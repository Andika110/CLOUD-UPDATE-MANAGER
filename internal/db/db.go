package db

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"
	"quinos-cloud-update-manager/internal/models"

	_ "modernc.org/sqlite"
)

type DB struct {
	db *sql.DB
	mu sync.Mutex
}

func InitDB(dbPath string) (*DB, error) {
	dir := filepath.Dir(dbPath)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create db directory: %w", err)
		}
	}

	dsn := fmt.Sprintf("%s?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)", dbPath)
	sqlDB, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Connection pool tuning for SQLite WAL mode
	sqlDB.SetMaxOpenConns(10)
	sqlDB.SetMaxIdleConns(5)
	sqlDB.SetConnMaxLifetime(time.Hour)

	d := &DB{db: sqlDB}
	if err := d.migrate(); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("migration failed: %w", err)
	}

	if err := d.seedDefaults(); err != nil {
		log.Printf("[DB] Warning during seeding: %v", err)
	}

	return d, nil
}

func (d *DB) Close() error {
	return d.db.Close()
}

func (d *DB) migrate() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	queries := []string{
		`CREATE TABLE IF NOT EXISTS outlets (
			id TEXT PRIMARY KEY,
			outlet_code TEXT UNIQUE NOT NULL,
			name TEXT NOT NULL,
			timezone TEXT NOT NULL DEFAULT 'Asia/Jakarta',
			api_key TEXT NOT NULL,
			release_group TEXT NOT NULL DEFAULT 'stable',
			status TEXT NOT NULL DEFAULT 'Active',
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		);`,
		`CREATE TABLE IF NOT EXISTS terminals (
			id TEXT PRIMARY KEY,
			outlet_id TEXT NOT NULL REFERENCES outlets(id) ON DELETE CASCADE,
			terminal_code TEXT NOT NULL,
			hostname TEXT NOT NULL,
			ip_address TEXT NOT NULL,
			os_info TEXT NOT NULL DEFAULT 'Windows',
			current_jar_version TEXT NOT NULL DEFAULT 'v2.4.2',
			agent_version TEXT NOT NULL DEFAULT 'v1.0.0',
			pos_running INTEGER NOT NULL DEFAULT 0,
			pos_pid INTEGER,
			target_path TEXT NOT NULL DEFAULT 'C:\\QuinosPOS\\app\\quinos.jar',
			status TEXT NOT NULL DEFAULT 'Offline',
			update_status TEXT NOT NULL DEFAULT 'IDLE',
			pending_command TEXT NOT NULL DEFAULT '',
			last_seen_at DATETIME,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(outlet_id, terminal_code)
		);`,
		`CREATE TABLE IF NOT EXISTS packages (
			id TEXT PRIMARY KEY,
			version_label TEXT NOT NULL,
			file_name TEXT NOT NULL,
			size_bytes INTEGER NOT NULL,
			sha256 TEXT NOT NULL,
			changelog TEXT NOT NULL DEFAULT '',
			release_channel TEXT NOT NULL DEFAULT 'stable',
			is_mandatory INTEGER NOT NULL DEFAULT 0,
			storage_path TEXT NOT NULL,
			uploaded_by TEXT NOT NULL DEFAULT 'admin',
			status TEXT NOT NULL DEFAULT 'Active',
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		);`,
		`CREATE TABLE IF NOT EXISTS deployments (
			id TEXT PRIMARY KEY,
			package_id TEXT NOT NULL REFERENCES packages(id),
			target_group TEXT NOT NULL,
			status TEXT NOT NULL DEFAULT 'Active',
			total_tasks INTEGER NOT NULL DEFAULT 0,
			success_count INTEGER NOT NULL DEFAULT 0,
			failed_count INTEGER NOT NULL DEFAULT 0,
			notes TEXT NOT NULL DEFAULT '',
			created_by TEXT NOT NULL DEFAULT 'admin',
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		);`,
		`CREATE TABLE IF NOT EXISTS deployment_tasks (
			id TEXT PRIMARY KEY,
			deployment_id TEXT NOT NULL REFERENCES deployments(id) ON DELETE CASCADE,
			terminal_id TEXT NOT NULL REFERENCES terminals(id) ON DELETE CASCADE,
			package_id TEXT NOT NULL REFERENCES packages(id),
			target_version TEXT NOT NULL,
			status TEXT NOT NULL DEFAULT 'PENDING',
			progress INTEGER NOT NULL DEFAULT 0,
			error_message TEXT NOT NULL DEFAULT '',
			started_at DATETIME,
			completed_at DATETIME
		);`,
		`CREATE TABLE IF NOT EXISTS audit_logs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			outlet_code TEXT NOT NULL DEFAULT '',
			terminal_code TEXT NOT NULL DEFAULT '',
			actor TEXT NOT NULL DEFAULT 'system',
			action TEXT NOT NULL,
			details TEXT NOT NULL DEFAULT '',
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		);`,
		`CREATE INDEX IF NOT EXISTS idx_terminals_outlet ON terminals(outlet_id);`,
		`CREATE INDEX IF NOT EXISTS idx_tasks_terminal ON deployment_tasks(terminal_id);`,
		`CREATE INDEX IF NOT EXISTS idx_tasks_deployment ON deployment_tasks(deployment_id);`,
	}

	for _, q := range queries {
		if _, err := d.db.Exec(q); err != nil {
			return fmt.Errorf("failed executing query (%s): %w", q, err)
		}
	}

	// Schema migrations
	_, _ = d.db.Exec("ALTER TABLE terminals ADD COLUMN idle_seconds INTEGER NOT NULL DEFAULT 0;")

	return nil
}

func (d *DB) seedDefaults() error {
	var count int
	if err := d.db.QueryRow("SELECT COUNT(*) FROM outlets").Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return nil
	}

	log.Printf("[DB] Initializing default outlet...")
	outID := uuid.New().String()
	_, err := d.db.Exec(`INSERT INTO outlets (id, outlet_code, name, timezone, api_key, release_group, status)
		VALUES (?, 'OUT-JKT-01', 'Quinos Main Store', 'Asia/Jakarta', 'key_senopati_secret123', 'stable', 'Active')`, outID)
	if err != nil {
		return err
	}

	d.AddAuditLog("", "", "SYSTEM", "DATABASE_INITIALIZED", "Cloud Update Manager initialized with clean production database")
	return nil
}

// GetOverview returns real-time dashboard statistics
func (d *DB) GetOverview() (*models.SystemOverview, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	overview := &models.SystemOverview{
		ServerTime:    time.Now().Format("2006-01-02 15:04:05 MST"),
		ServerVersion: "v1.0.0-CloudVPS",
		LatestVersion: "v2.4.2-STABLE",
	}

	_ = d.db.QueryRow("SELECT COUNT(*) FROM outlets").Scan(&overview.TotalOutlets)
	_ = d.db.QueryRow("SELECT COUNT(*) FROM outlets WHERE status = 'Active'").Scan(&overview.ActiveOutlets)
	_ = d.db.QueryRow("SELECT COUNT(*) FROM deployments WHERE status = 'Active'").Scan(&overview.ActiveDeployments)

	var latestVer string
	if err := d.db.QueryRow("SELECT version_label FROM packages WHERE status = 'Active' ORDER BY created_at DESC LIMIT 1").Scan(&latestVer); err == nil {
		overview.LatestVersion = latestVer
	}

	// Calculate terminals and online counts using Go time comparison (immune to timezone differences)
	rows, err := d.db.Query("SELECT pos_running, last_seen_at FROM terminals")
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			overview.TotalTerminals++
			var posRunning bool
			var lastSeen sql.NullTime
			if err := rows.Scan(&posRunning, &lastSeen); err == nil {
				if lastSeen.Valid && time.Since(lastSeen.Time) <= 45*time.Second {
					overview.OnlineTerminals++
					if posRunning {
						overview.POSRunningCount++
					}
				}
			}
		}
	}

	return overview, nil
}

// ListOutlets retrieves all outlets with aggregated stats
func (d *DB) ListOutlets() ([]models.Outlet, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	rows, err := d.db.Query(`
		SELECT id, outlet_code, name, timezone, api_key, release_group, status, created_at
		FROM outlets
		ORDER BY outlet_code ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []models.Outlet
	for rows.Next() {
		var o models.Outlet
		err := rows.Scan(
			&o.ID, &o.OutletCode, &o.Name, &o.Timezone, &o.APIKey, &o.ReleaseGroup, &o.Status, &o.CreatedAt,
		)
		if err != nil {
			return nil, err
		}

		// Count terminals for this outlet
		tRows, err := d.db.Query("SELECT last_seen_at FROM terminals WHERE outlet_id = ?", o.ID)
		if err == nil {
			for tRows.Next() {
				o.TerminalCount++
				var lastSeen sql.NullTime
				if err := tRows.Scan(&lastSeen); err == nil {
					if lastSeen.Valid && time.Since(lastSeen.Time) <= 45*time.Second {
						o.OnlineCount++
					}
				}
			}
			tRows.Close()
		}

		list = append(list, o)
	}
	return list, nil
}

// CreateOutlet adds a new outlet
func (d *DB) CreateOutlet(code, name, group, apiKey string) (*models.Outlet, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if apiKey == "" {
		apiKey = "key_" + uuid.New().String()[:12]
	}
	id := uuid.New().String()
	_, err := d.db.Exec(`INSERT INTO outlets (id, outlet_code, name, timezone, api_key, release_group, status)
		VALUES (?, ?, ?, 'Asia/Jakarta', ?, ?, 'Active')`, id, code, name, apiKey, group)
	if err != nil {
		return nil, err
	}

	return &models.Outlet{
		ID:           id,
		OutletCode:   code,
		Name:         name,
		Timezone:     "Asia/Jakarta",
		APIKey:       apiKey,
		ReleaseGroup: group,
		Status:       "Active",
		CreatedAt:    time.Now(),
	}, nil
}

// DeleteOutlet removes an outlet and its associated terminals
func (d *DB) DeleteOutlet(id string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	_, _ = d.db.Exec("DELETE FROM terminals WHERE outlet_id = ?", id)
	_, err := d.db.Exec("DELETE FROM outlets WHERE id = ?", id)
	return err
}

// DeleteTerminal removes a specific terminal
func (d *DB) DeleteTerminal(id string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	_, err := d.db.Exec("DELETE FROM terminals WHERE id = ?", id)
	return err
}

// ListTerminals returns terminals, optionally filtered by outlet_id
func (d *DB) ListTerminals(outletID string) ([]models.Terminal, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	query := `
		SELECT t.id, t.outlet_id, o.outlet_code, o.name, t.terminal_code, t.hostname, t.ip_address,
		       t.os_info, t.current_jar_version, t.agent_version, t.pos_running, t.pos_pid,
		       COALESCE(t.idle_seconds, 0), t.target_path, t.status, t.update_status, t.pending_command, t.last_seen_at, t.created_at
		FROM terminals t
		JOIN outlets o ON t.outlet_id = o.id
	`
	var args []interface{}
	if outletID != "" {
		query += " WHERE t.outlet_id = ?"
		args = append(args, outletID)
	}
	query += " ORDER BY o.outlet_code ASC, t.terminal_code ASC"

	rows, err := d.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []models.Terminal
	for rows.Next() {
		var t models.Terminal
		var posPID sql.NullInt64
		var lastSeen sql.NullTime

		err := rows.Scan(
			&t.ID, &t.OutletID, &t.OutletCode, &t.OutletName, &t.TerminalCode, &t.Hostname, &t.IPAddress,
			&t.OSInfo, &t.CurrentJARVersion, &t.AgentVersion, &t.POSRunning, &posPID,
			&t.IdleSeconds, &t.TargetPath, &t.Status, &t.UpdateStatus, &t.PendingCommand, &lastSeen, &t.CreatedAt,
		)
		if err != nil {
			return nil, err
		}
		if posPID.Valid {
			p := int(posPID.Int64)
			t.POSPID = &p
		}
		if lastSeen.Valid {
			t.LastSeenAt = &lastSeen.Time
			if time.Since(lastSeen.Time) <= 45*time.Second {
				t.Status = "Online"
			} else {
				t.Status = "Offline"
				t.POSRunning = false
			}
		} else {
			t.Status = "Offline"
			t.POSRunning = false
		}
		list = append(list, t)
	}
	return list, nil
}

// VerifyOutletAPIKey checks if the provided API key matches the outlet
func (d *DB) VerifyOutletAPIKey(outletCode, apiKey string) (string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	var outletID string
	err := d.db.QueryRow("SELECT id FROM outlets WHERE outlet_code = ? AND api_key = ? AND status = 'Active'", outletCode, apiKey).Scan(&outletID)
	if err != nil {
		return "", fmt.Errorf("invalid outlet credentials: %w", err)
	}
	return outletID, nil
}

// UpsertHeartbeat records heartbeat from an agent terminal, auto-registering outlet/terminal if needed
func (d *DB) UpsertHeartbeat(req models.HeartbeatRequest, apiKey string) (*models.Terminal, string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if req.OutletCode == "" {
		req.OutletCode = "OUT-01"
	}
	if req.TerminalCode == "" {
		if req.Hostname != "" {
			req.TerminalCode = req.Hostname
		} else if req.AgentID != "" {
			req.TerminalCode = req.AgentID
		} else {
			req.TerminalCode = "POS-01"
		}
	}
	if req.OSInfo == "" {
		req.OSInfo = "Windows"
	}
	if req.CurrentJARVersion == "" {
		req.CurrentJARVersion = "v2.4.2"
	}

	// 1. Find or auto-register outlet
	var outletID string
	err := d.db.QueryRow("SELECT id FROM outlets WHERE outlet_code = ?", req.OutletCode).Scan(&outletID)
	if err == sql.ErrNoRows {
		outletID = uuid.New().String()
		outletName := fmt.Sprintf("Cabang %s", req.OutletCode)
		if apiKey == "" {
			apiKey = "key_" + uuid.New().String()[:12]
		}
		_, err = d.db.Exec(`INSERT INTO outlets (id, outlet_code, name, timezone, api_key, release_group, status)
			VALUES (?, ?, ?, 'Asia/Jakarta', ?, 'stable', 'Active')`,
			outletID, req.OutletCode, outletName, apiKey)
		if err != nil {
			return nil, "", fmt.Errorf("failed auto-creating outlet: %w", err)
		}
		log.Printf("[DB] Auto-registered new outlet: %s (%s)", outletName, req.OutletCode)
	} else if err != nil {
		return nil, "", fmt.Errorf("db error checking outlet: %w", err)
	}

	now := time.Now()
	var termID, pendingCommand string
	err = d.db.QueryRow(`SELECT id, pending_command FROM terminals WHERE outlet_id = ? AND (terminal_code = ? OR hostname = ?)`, outletID, req.TerminalCode, req.Hostname).Scan(&termID, &pendingCommand)

	if err == sql.ErrNoRows {
		// Auto-register new terminal
		termID = uuid.New().String()
		_, err = d.db.Exec(`INSERT INTO terminals 
			(id, outlet_id, terminal_code, hostname, ip_address, os_info, current_jar_version, agent_version, pos_running, pos_pid, idle_seconds, target_path, status, update_status, last_seen_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'Online', ?, ?)`,
			termID, outletID, req.TerminalCode, req.Hostname, req.IPAddress, req.OSInfo,
			req.CurrentJARVersion, req.AgentVersion, req.POSRunning, req.POSPID, req.IdleSeconds, req.TargetPath,
			req.UpdateStatus, now)
		if err != nil {
			return nil, "", err
		}
		log.Printf("[DB] Auto-registered real terminal: %s (%s) IP: %s", req.TerminalCode, req.Hostname, req.IPAddress)
	} else if err == nil {
		// Update existing terminal
		_, err = d.db.Exec(`UPDATE terminals SET
			hostname = ?, ip_address = ?, os_info = ?, current_jar_version = ?, agent_version = ?,
			pos_running = ?, pos_pid = ?, idle_seconds = ?, target_path = ?, status = 'Online', update_status = ?,
			last_seen_at = ?
			WHERE id = ?`,
			req.Hostname, req.IPAddress, req.OSInfo, req.CurrentJARVersion, req.AgentVersion,
			req.POSRunning, req.POSPID, req.IdleSeconds, req.TargetPath, req.UpdateStatus, now, termID)
		if err != nil {
			return nil, "", err
		}
	} else {
		return nil, "", err
	}

	// Fetch updated terminal
	term := &models.Terminal{
		ID:                termID,
		OutletID:          outletID,
		TerminalCode:      req.TerminalCode,
		Hostname:          req.Hostname,
		IPAddress:         req.IPAddress,
		CurrentJARVersion: req.CurrentJARVersion,
		Status:            "Online",
		UpdateStatus:      req.UpdateStatus,
		LastSeenAt:        &now,
	}

	return term, pendingCommand, nil
}

// ClearPendingCommand resets pending command after agent consumed it
func (d *DB) ClearPendingCommand(terminalID string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	_, err := d.db.Exec("UPDATE terminals SET pending_command = '' WHERE id = ?", terminalID)
	return err
}

// SetTerminalPendingCommand sends a remote command to a terminal
func (d *DB) SetTerminalPendingCommand(terminalID string, cmd string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	_, err := d.db.Exec("UPDATE terminals SET pending_command = ? WHERE id = ?", cmd, terminalID)
	return err
}

// ListPackages returns all uploaded JAR releases
func (d *DB) ListPackages() ([]models.Package, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	rows, err := d.db.Query(`SELECT id, version_label, file_name, size_bytes, sha256, changelog, release_channel, is_mandatory, storage_path, uploaded_by, status, created_at FROM packages ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []models.Package
	for rows.Next() {
		var p models.Package
		var isMandatory int
		err := rows.Scan(
			&p.ID, &p.VersionLabel, &p.FileName, &p.SizeBytes, &p.SHA256, &p.Changelog,
			&p.ReleaseChannel, &isMandatory, &p.StoragePath, &p.UploadedBy, &p.Status, &p.CreatedAt,
		)
		if err != nil {
			return nil, err
		}
		p.IsMandatory = isMandatory == 1
		p.SizeMB = fmt.Sprintf("%.2f MB", float64(p.SizeBytes)/(1024*1024))
		list = append(list, p)
	}
	return list, nil
}

// GetPackage returns a package by ID
func (d *DB) GetPackage(id string) (*models.Package, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	var p models.Package
	var isMandatory int
	err := d.db.QueryRow(`SELECT id, version_label, file_name, size_bytes, sha256, changelog, release_channel, is_mandatory, storage_path, uploaded_by, status, created_at FROM packages WHERE id = ?`, id).
		Scan(&p.ID, &p.VersionLabel, &p.FileName, &p.SizeBytes, &p.SHA256, &p.Changelog, &p.ReleaseChannel, &isMandatory, &p.StoragePath, &p.UploadedBy, &p.Status, &p.CreatedAt)
	if err != nil {
		return nil, err
	}
	p.IsMandatory = isMandatory == 1
	p.SizeMB = fmt.Sprintf("%.2f MB", float64(p.SizeBytes)/(1024*1024))
	return &p, nil
}

// CreatePackage inserts a new package
func (d *DB) CreatePackage(p models.Package) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	isMandatory := 0
	if p.IsMandatory {
		isMandatory = 1
	}

	_, err := d.db.Exec(`INSERT INTO packages 
		(id, version_label, file_name, size_bytes, sha256, changelog, release_channel, is_mandatory, storage_path, uploaded_by, status)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		p.ID, p.VersionLabel, p.FileName, p.SizeBytes, p.SHA256, p.Changelog, p.ReleaseChannel, isMandatory, p.StoragePath, p.UploadedBy, p.Status)
	return err
}

// CreateDeployment triggers a rollout to targeted terminals
func (d *DB) CreateDeployment(pkgID, targetGroup, notes, createdBy string, forceClose bool) (*models.Deployment, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	depID := fmt.Sprintf("DEP-%s-%s", time.Now().Format("20060102"), uuid.New().String()[:6])

	// Find targeted terminals
	query := `SELECT t.id, t.terminal_code, o.outlet_code FROM terminals t JOIN outlets o ON t.outlet_id = o.id WHERE o.status = 'Active'`
	var args []interface{}

	if targetGroup == "canary" {
		query += " AND o.release_group = 'canary'"
	} else if targetGroup == "pilot" {
		query += " AND o.release_group IN ('canary', 'pilot')"
	} else if len(targetGroup) > 7 && targetGroup[:7] == "outlet:" {
		query += " AND o.outlet_code = ?"
		args = append(args, targetGroup[7:])
	}
	// if targetGroup == "all", no extra filter

	rows, err := d.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type targetItem struct {
		termID, termCode, outCode string
	}
	var targets []targetItem
	for rows.Next() {
		var it targetItem
		if err := rows.Scan(&it.termID, &it.termCode, &it.outCode); err == nil {
			targets = append(targets, it)
		}
	}

	if len(targets) == 0 {
		return nil, fmt.Errorf("no terminals found for target group: %s", targetGroup)
	}

	// Fetch package version
	var pkgVersion string
	_ = d.db.QueryRow("SELECT version_label FROM packages WHERE id = ?", pkgID).Scan(&pkgVersion)

	// Create Deployment record
	_, err = d.db.Exec(`INSERT INTO deployments (id, package_id, target_group, status, total_tasks, notes, created_by)
		VALUES (?, ?, ?, 'Active', ?, ?, ?)`, depID, pkgID, targetGroup, len(targets), notes, createdBy)
	if err != nil {
		return nil, err
	}

	// Create tasks for each terminal
	for _, t := range targets {
		taskID := uuid.New().String()
		_, _ = d.db.Exec(`INSERT INTO deployment_tasks (id, deployment_id, terminal_id, package_id, target_version, status, progress)
			VALUES (?, ?, ?, ?, ?, 'PENDING', 0)`, taskID, depID, t.termID, pkgID, pkgVersion)
		if forceClose {
			_, _ = d.db.Exec(`UPDATE terminals SET pending_command = 'CLOSE_POS' WHERE id = ?`, t.termID)
		}
	}

	return &models.Deployment{
		ID:             depID,
		PackageID:      pkgID,
		PackageVersion: pkgVersion,
		TargetGroup:    targetGroup,
		Status:         "Active",
		TotalTasks:     len(targets),
		Notes:          notes,
		CreatedBy:      createdBy,
		CreatedAt:      time.Now(),
	}, nil
}

// NotifyCashiersDeployment queues PROMPT_UPDATE (friendly dialog) to all active terminals in a deployment
func (d *DB) NotifyCashiersDeployment(depID string) (int, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	rows, err := d.db.Query(`
		SELECT DISTINCT terminal_id FROM deployment_tasks 
		WHERE deployment_id = ? AND status NOT IN ('SUCCESS', 'FAILED', 'CANCELLED', 'ROLLED_BACK')
	`, depID)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	var termIDs []string
	for rows.Next() {
		var tid string
		if err := rows.Scan(&tid); err == nil {
			termIDs = append(termIDs, tid)
		}
	}

	for _, tid := range termIDs {
		_, _ = d.db.Exec(`UPDATE terminals SET pending_command = 'PROMPT_UPDATE' WHERE id = ? AND (pending_command = '' OR pending_command IS NULL)`, tid)
	}

	return len(termIDs), nil
}

// ForceCloseDeploymentTerminals queues CLOSE_POS command to all active terminals in a deployment
func (d *DB) ForceCloseDeploymentTerminals(depID string) (int, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	rows, err := d.db.Query(`
		SELECT DISTINCT terminal_id FROM deployment_tasks 
		WHERE deployment_id = ? AND status NOT IN ('SUCCESS', 'FAILED', 'CANCELLED', 'ROLLED_BACK')
	`, depID)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	var termIDs []string
	for rows.Next() {
		var tid string
		if err := rows.Scan(&tid); err == nil {
			termIDs = append(termIDs, tid)
		}
	}

	for _, tid := range termIDs {
		_, _ = d.db.Exec(`UPDATE terminals SET pending_command = 'CLOSE_POS' WHERE id = ?`, tid)
	}

	return len(termIDs), nil
}

// CancelDeployment cancels an active deployment and sets remaining tasks to CANCELLED
func (d *DB) CancelDeployment(depID string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	_, err := d.db.Exec(`UPDATE deployments SET status = 'Cancelled' WHERE id = ?`, depID)
	if err != nil {
		return err
	}
	_, _ = d.db.Exec(`UPDATE deployment_tasks SET status = 'CANCELLED' WHERE deployment_id = ? AND status NOT IN ('SUCCESS', 'FAILED')`, depID)
	return nil
}

// ListDeployments returns all deployments with task counts and individual terminal tasks
func (d *DB) ListDeployments() ([]models.Deployment, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	rows, err := d.db.Query(`
		SELECT d.id, d.package_id, p.version_label, d.target_group, d.status, d.total_tasks,
		       d.success_count, d.failed_count, d.notes, d.created_by, d.created_at
		FROM deployments d
		JOIN packages p ON d.package_id = p.id
		ORDER BY d.created_at DESC LIMIT 50
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []models.Deployment
	for rows.Next() {
		var dep models.Deployment
		err := rows.Scan(
			&dep.ID, &dep.PackageID, &dep.PackageVersion, &dep.TargetGroup, &dep.Status,
			&dep.TotalTasks, &dep.SuccessCount, &dep.FailedCount, &dep.Notes, &dep.CreatedBy, &dep.CreatedAt,
		)
		if err != nil {
			return nil, err
		}
		list = append(list, dep)
	}

	// Fetch tasks for each deployment
	for i := range list {
		taskRows, err := d.db.Query(`
			SELECT dt.id, dt.deployment_id, dt.terminal_id, COALESCE(t.terminal_code, ''), COALESCE(o.outlet_code, ''), 
			       dt.package_id, dt.target_version, dt.status, dt.progress, COALESCE(dt.error_message, ''), 
			       dt.started_at, dt.completed_at
			FROM deployment_tasks dt
			LEFT JOIN terminals t ON dt.terminal_id = t.id
			LEFT JOIN outlets o ON t.outlet_id = o.id
			WHERE dt.deployment_id = ?
			ORDER BY dt.started_at ASC, dt.id ASC
		`, list[i].ID)
		if err == nil {
			for taskRows.Next() {
				var t models.DeploymentTask
				if err := taskRows.Scan(
					&t.ID, &t.DeploymentID, &t.TerminalID, &t.TerminalCode, &t.OutletCode,
					&t.PackageID, &t.TargetVersion, &t.Status, &t.Progress, &t.ErrorMessage,
					&t.StartedAt, &t.CompletedAt,
				); err == nil {
					list[i].Tasks = append(list[i].Tasks, t)
				}
			}
			taskRows.Close()
		}
	}

	return list, nil
}

// GetPendingTaskForTerminal returns an active update task for a terminal
func (d *DB) GetPendingTaskForTerminal(terminalID string) (*models.DeploymentTask, *models.Package, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	row := d.db.QueryRow(`
		SELECT dt.id, dt.deployment_id, dt.terminal_id, dt.package_id, dt.target_version, dt.status, dt.progress,
		       p.version_label, p.file_name, p.size_bytes, p.sha256, p.is_mandatory, p.storage_path
		FROM deployment_tasks dt
		JOIN packages p ON dt.package_id = p.id
		JOIN deployments d ON dt.deployment_id = d.id
		WHERE dt.terminal_id = ? AND dt.status IN ('PENDING', 'DOWNLOADING', 'VERIFYING', 'INSTALLING')
		  AND d.status = 'Active'
		ORDER BY dt.started_at ASC, dt.id ASC LIMIT 1
	`, terminalID)

	var t models.DeploymentTask
	var p models.Package
	var isMandatory int

	err := row.Scan(
		&t.ID, &t.DeploymentID, &t.TerminalID, &t.PackageID, &t.TargetVersion, &t.Status, &t.Progress,
		&p.VersionLabel, &p.FileName, &p.SizeBytes, &p.SHA256, &isMandatory, &p.StoragePath,
	)
	if err != nil {
		return nil, nil, err
	}

	p.ID = t.PackageID
	p.IsMandatory = isMandatory == 1
	return &t, &p, nil
}

// UpdateTaskProgress updates task status and terminal update_status
func (d *DB) UpdateTaskProgress(taskID, status string, progress int, errMsg string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	now := time.Now()
	var terminalID, depID, targetVer string
	err := d.db.QueryRow("SELECT terminal_id, deployment_id, target_version FROM deployment_tasks WHERE id = ?", taskID).Scan(&terminalID, &depID, &targetVer)
	if err != nil {
		return err
	}

	if status == "SUCCESS" {
		_, _ = d.db.Exec(`UPDATE deployment_tasks SET status = 'SUCCESS', progress = 100, completed_at = ?, error_message = '' WHERE id = ?`, now, taskID)
		_, _ = d.db.Exec(`UPDATE terminals SET update_status = 'SUCCESS', current_jar_version = ? WHERE id = ?`, targetVer, terminalID)
		_, _ = d.db.Exec(`UPDATE deployments SET success_count = success_count + 1 WHERE id = ?`, depID)
	} else if status == "FAILED" || status == "ROLLED_BACK" {
		_, _ = d.db.Exec(`UPDATE deployment_tasks SET status = ?, completed_at = ?, error_message = ? WHERE id = ?`, status, now, errMsg, taskID)
		_, _ = d.db.Exec(`UPDATE terminals SET update_status = ? WHERE id = ?`, status, terminalID)
		_, _ = d.db.Exec(`UPDATE deployments SET failed_count = failed_count + 1 WHERE id = ?`, depID)
	} else {
		// Progress update
		_, _ = d.db.Exec(`UPDATE deployment_tasks SET status = ?, progress = ?, started_at = COALESCE(started_at, ?) WHERE id = ?`, status, progress, now, taskID)
		_, _ = d.db.Exec(`UPDATE terminals SET update_status = ? WHERE id = ?`, status, terminalID)
	}

	return nil
}

// AddAuditLog inserts a security or deployment event
func (d *DB) AddAuditLog(outletCode, terminalCode, actor, action, details string) {
	d.mu.Lock()
	defer d.mu.Unlock()

	_, _ = d.db.Exec(`INSERT INTO audit_logs (outlet_code, terminal_code, actor, action, details) VALUES (?, ?, ?, ?, ?)`,
		outletCode, terminalCode, actor, action, details)
}

// ListAuditLogs retrieves recent logs
func (d *DB) ListAuditLogs(limit int) ([]models.AuditLog, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if limit <= 0 {
		limit = 100
	}
	rows, err := d.db.Query(`SELECT id, outlet_code, terminal_code, actor, action, details, created_at FROM audit_logs ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var logs []models.AuditLog
	for rows.Next() {
		var l models.AuditLog
		if err := rows.Scan(&l.ID, &l.OutletCode, &l.TerminalCode, &l.Actor, &l.Action, &l.Details, &l.CreatedAt); err == nil {
			logs = append(logs, l)
		}
	}
	return logs, nil
}

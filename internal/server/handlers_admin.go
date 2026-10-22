package server

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"quinos-cloud-update-manager/internal/models"
)

// handleWebDashboard serves the admin dashboard HTML file
func (s *Server) handleWebDashboard(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" && r.URL.Path != "/dashboard" {
		http.NotFound(w, r)
		return
	}

	htmlData, err := os.ReadFile(s.webHTMLPath)
	if err != nil {
		// Fallback minimal HTML if file not found
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprintf(w, `<!DOCTYPE html><html><head><title>Quinos Cloud Update Manager</title></head>
		<body style="font-family:sans-serif;padding:40px;background:#0f172a;color:#f8fafc;">
		<h1>Quinos Cloud Update Manager (VPS Edition)</h1>
		<p>Web Dashboard template loading from: <code>%s</code></p>
		<p>API Status: <strong>Online</strong></p>
		<p><a href="/api/v1/admin/overview" style="color:#6366f1;">View System Overview JSON</a></p>
		</body></html>`, s.webHTMLPath)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(htmlData)
}

// handleAdminOverview returns real-time dashboard counts and summary
func (s *Server) handleAdminOverview(w http.ResponseWriter, r *http.Request) {
	overview, err := s.db.GetOverview()
	if err != nil {
		http.Error(w, "Failed to get overview: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(overview)
}

// handleAdminOutlets handles listing, creating, and deleting store outlets
func (s *Server) handleAdminOutlets(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		outlets, err := s.db.ListOutlets()
		if err != nil {
			http.Error(w, "Failed to list outlets: "+err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(outlets)

	case http.MethodPost:
		var req struct {
			OutletCode   string `json:"outlet_code"`
			Name         string `json:"name"`
			ReleaseGroup string `json:"release_group"`
			APIKey       string `json:"api_key"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid JSON: "+err.Error(), http.StatusBadRequest)
			return
		}

		if req.OutletCode == "" || req.Name == "" {
			http.Error(w, "Missing outlet_code or name", http.StatusBadRequest)
			return
		}
		if req.ReleaseGroup == "" {
			req.ReleaseGroup = "stable"
		}

		outlet, err := s.db.CreateOutlet(req.OutletCode, req.Name, req.ReleaseGroup, req.APIKey)
		if err != nil {
			http.Error(w, "Failed to create outlet: "+err.Error(), http.StatusInternalServerError)
			return
		}

		s.db.AddAuditLog(req.OutletCode, "", "ADMIN", "OUTLET_CREATED", fmt.Sprintf("Created outlet %s (%s)", req.Name, req.OutletCode))

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(outlet)

	case http.MethodDelete:
		id := r.URL.Query().Get("id")
		if id == "" {
			http.Error(w, "Missing outlet id", http.StatusBadRequest)
			return
		}
		if err := s.db.DeleteOutlet(id); err != nil {
			http.Error(w, "Failed to delete outlet: "+err.Error(), http.StatusInternalServerError)
			return
		}
		s.db.AddAuditLog("", "", "ADMIN", "OUTLET_DELETED", fmt.Sprintf("Deleted outlet ID: %s", id))
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "deleted"})

	default:
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
	}
}

// handleAdminTerminals returns all terminals, optionally filtered by outlet_id, or deletes a terminal
func (s *Server) handleAdminTerminals(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		outletID := r.URL.Query().Get("outlet_id")
		terminals, err := s.db.ListTerminals(outletID)
		if err != nil {
			http.Error(w, "Failed to list terminals: "+err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(terminals)

	case http.MethodDelete:
		id := r.URL.Query().Get("id")
		if id == "" {
			http.Error(w, "Missing terminal id", http.StatusBadRequest)
			return
		}
		if err := s.db.DeleteTerminal(id); err != nil {
			http.Error(w, "Failed to delete terminal: "+err.Error(), http.StatusInternalServerError)
			return
		}
		s.db.AddAuditLog("", "", "ADMIN", "TERMINAL_DELETED", fmt.Sprintf("Deleted terminal ID: %s", id))
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "deleted"})

	default:
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
	}
}

// handleAdminPackages returns all uploaded JAR releases
func (s *Server) handleAdminPackages(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	packages, err := s.db.ListPackages()
	if err != nil {
		http.Error(w, "Failed to list packages: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(packages)
}

// handleAdminPackageUpload handles multipart form upload of a new Quinos JAR package
func (s *Server) handleAdminPackageUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	// 500 MB max in memory/temp disk
	if err := r.ParseMultipartForm(500 << 20); err != nil {
		http.Error(w, "Form parse error (max 500MB): "+err.Error(), http.StatusBadRequest)
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "File upload missing: "+err.Error(), http.StatusBadRequest)
		return
	}
	defer file.Close()

	versionLabel := strings.TrimSpace(r.FormValue("version_label"))
	if versionLabel == "" {
		versionLabel = fmt.Sprintf("v%s", time.Now().Format("2006.01.02"))
	}
	changelog := strings.TrimSpace(r.FormValue("changelog"))
	releaseChannel := strings.TrimSpace(r.FormValue("release_channel"))
	if releaseChannel == "" {
		releaseChannel = "stable"
	}
	isMandatory := r.FormValue("is_mandatory") == "true" || r.FormValue("is_mandatory") == "1"

	// Create storage file path
	cleanFilename := filepath.Base(header.Filename)
	pkgID := uuid.New().String()
	destFilename := fmt.Sprintf("quinos_%s_%s", pkgID[:8], cleanFilename)
	destPath := filepath.Join(s.storageDir, destFilename)

	destFile, err := os.Create(destPath)
	if err != nil {
		http.Error(w, "Failed to create artifact on disk: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer destFile.Close()

	// Compute SHA-256 while streaming to disk
	hasher := sha256.New()
	multiWriter := io.MultiWriter(destFile, hasher)

	writtenBytes, err := io.Copy(multiWriter, file)
	if err != nil {
		http.Error(w, "Failed saving file: "+err.Error(), http.StatusInternalServerError)
		return
	}

	sha256Hex := hex.EncodeToString(hasher.Sum(nil))

	pkg := models.Package{
		ID:             pkgID,
		VersionLabel:   versionLabel,
		FileName:       cleanFilename,
		SizeBytes:      writtenBytes,
		SHA256:         sha256Hex,
		Changelog:      changelog,
		ReleaseChannel: releaseChannel,
		IsMandatory:    isMandatory,
		StoragePath:    destPath,
		UploadedBy:     "admin",
		Status:         "Active",
		CreatedAt:      time.Now(),
	}

	if err := s.db.CreatePackage(pkg); err != nil {
		http.Error(w, "Failed to record package in database: "+err.Error(), http.StatusInternalServerError)
		return
	}

	log.Printf("[Upload] New package uploaded: %s (%s, %d bytes, sha256: %s)", versionLabel, cleanFilename, writtenBytes, sha256Hex)
	s.db.AddAuditLog("", "", "ADMIN", "PACKAGE_UPLOADED", fmt.Sprintf("Uploaded package %s (%s) - SHA256: %s", versionLabel, cleanFilename, sha256Hex))

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(pkg)
}

// handleAdminDeployments handles listing deployments and triggering new rollouts
func (s *Server) handleAdminDeployments(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		deployments, err := s.db.ListDeployments()
		if err != nil {
			http.Error(w, "Failed to list deployments: "+err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(deployments)

	case http.MethodPost:
		var req struct {
			PackageID   string `json:"package_id"`
			TargetGroup string `json:"target_group"` // "canary", "pilot", "all", or "outlet:OUT-XXX"
			Notes       string `json:"notes"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid JSON: "+err.Error(), http.StatusBadRequest)
			return
		}

		if req.PackageID == "" {
			http.Error(w, "Missing package_id", http.StatusBadRequest)
			return
		}
		if req.TargetGroup == "" {
			req.TargetGroup = "canary"
		}

		dep, err := s.db.CreateDeployment(req.PackageID, req.TargetGroup, req.Notes, "admin")
		if err != nil {
			http.Error(w, "Failed to create deployment: "+err.Error(), http.StatusBadRequest)
			return
		}

		s.db.AddAuditLog("", "", "ADMIN", "DEPLOYMENT_LAUNCHED", fmt.Sprintf("Started rollout %s to target group '%s' (%d terminals)", dep.ID, req.TargetGroup, dep.TotalTasks))

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(dep)

	default:
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
	}
}

// handleAdminTerminalCommand sends a remote command to a terminal
func (s *Server) handleAdminTerminalCommand(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		TerminalID string `json:"terminal_id"`
		Command    string `json:"command"` // "RESTART_POS", "CLOSE_POS", "CHECK_UPDATE", "TRIGGER_ROLLBACK"
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	if req.TerminalID == "" || req.Command == "" {
		http.Error(w, "Missing terminal_id or command", http.StatusBadRequest)
		return
	}

	if err := s.db.SetTerminalPendingCommand(req.TerminalID, req.Command); err != nil {
		http.Error(w, "Failed to set command: "+err.Error(), http.StatusInternalServerError)
		return
	}

	s.db.AddAuditLog("", "", "ADMIN", "REMOTE_COMMAND", fmt.Sprintf("Sent command '%s' to terminal ID %s", req.Command, req.TerminalID))

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{
		"status":  "ok",
		"message": fmt.Sprintf("Command '%s' queued for terminal", req.Command),
	})
}

// handleAdminLogs retrieves audit and security logs
func (s *Server) handleAdminLogs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	limitStr := r.URL.Query().Get("limit")
	limit := 100
	if limitStr != "" {
		if parsed, err := strconv.Atoi(limitStr); err == nil && parsed > 0 {
			limit = parsed
		}
	}

	logs, err := s.db.ListAuditLogs(limit)
	if err != nil {
		http.Error(w, "Failed to retrieve logs: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(logs)
}

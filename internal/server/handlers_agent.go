package server

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"quinos-cloud-update-manager/internal/models"
)

// handleAgentHeartbeat processes periodic agent status updates
func (s *Server) handleAgentHeartbeat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	apiKey := r.Header.Get("X-Terminal-Key")
	if apiKey == "" {
		apiKey = r.URL.Query().Get("api_key")
	}

	var req models.HeartbeatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Compatibility auto-fill from standard/LAN quinos-agent
	if req.CurrentJARVersion == "" && req.InstalledVersion != "" {
		req.CurrentJARVersion = req.InstalledVersion
	}
	if req.IPAddress == "" && req.IP != "" {
		req.IPAddress = req.IP
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
	if req.OutletCode == "" {
		req.OutletCode = "OUT-JKT-01"
	}
	if apiKey == "" {
		apiKey = "key_senopati_secret123"
	}

	// Extract real IP if behind Nginx reverse proxy
	realIP := r.Header.Get("X-Forwarded-For")
	if realIP == "" {
		realIP = r.Header.Get("X-Real-IP")
	}
	if realIP == "" {
		realIP = r.RemoteAddr
	}
	if req.IPAddress == "" {
		req.IPAddress = strings.Split(realIP, ":")[0]
	}

	term, pendingCmd, err := s.db.UpsertHeartbeat(req, apiKey)
	if err != nil {
		log.Printf("[Heartbeat] Error from %s/%s: %v", req.OutletCode, req.TerminalCode, err)
		http.Error(w, "Unauthorized or Database Error: "+err.Error(), http.StatusUnauthorized)
		return
	}

	// Check if this terminal has an active deployment task
	resp := models.HeartbeatResponse{
		Status:         "ok",
		ServerTime:     time.Now().Format("2006-01-02 15:04:05"),
		PendingCommand: pendingCmd,
	}

	// If there's a pending command, clear it from DB so it doesn't trigger twice
	if pendingCmd != "" {
		_ = s.db.ClearPendingCommand(term.ID)
	}

	task, pkg, err := s.db.GetPendingTaskForTerminal(term.ID)
	if err == nil && task != nil && pkg != nil {
		resp.HasUpdate = true
		resp.UpdateInfo = &struct {
			TaskID       string `json:"task_id"`
			PackageID    string `json:"package_id"`
			VersionLabel string `json:"version_label"`
			FileName     string `json:"file_name"`
			SizeBytes    int64  `json:"size_bytes"`
			SHA256       string `json:"sha256"`
			DownloadURL  string `json:"download_url"`
			IsMandatory  bool   `json:"is_mandatory"`
		}{
			TaskID:       task.ID,
			PackageID:    pkg.ID,
			VersionLabel: pkg.VersionLabel,
			FileName:     pkg.FileName,
			SizeBytes:    pkg.SizeBytes,
			SHA256:       pkg.SHA256,
			DownloadURL:  fmt.Sprintf("/api/v1/packages/%s/download", pkg.ID),
			IsMandatory:  pkg.IsMandatory,
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

// handleAgentTaskNext handles GET /api/v1/agents/tasks/next (used by quinos-agent)
func (s *Server) handleAgentTaskNext(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	agentID := r.URL.Query().Get("agent_id")
	terms, err := s.db.ListTerminals("")
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}

	var matchedTerm *models.Terminal
	for _, t := range terms {
		if t.Hostname == agentID || t.TerminalCode == agentID || strings.Contains(agentID, strings.ToLower(t.TerminalCode)) {
			matchedTerm = &t
			break
		}
	}

	if matchedTerm == nil && len(terms) > 0 {
		matchedTerm = &terms[0]
	}

	if matchedTerm == nil {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"has_task": false})
		return
	}

	task, pkg, err := s.db.GetPendingTaskForTerminal(matchedTerm.ID)
	if err != nil || task == nil || pkg == nil {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"has_task": false})
		return
	}

	type ClientInfo struct {
		TargetPath string `json:"target_path"`
	}
	type PackageInfo struct {
		ID               string `json:"id"`
		VersionLabel     string `json:"version_label"`
		OriginalFilename string `json:"original_filename"`
		SHA256           string `json:"sha256"`
		SizeBytes        int64  `json:"size_bytes"`
	}
	type TaskResponse struct {
		ID        string      `json:"id"`
		Client    ClientInfo  `json:"client"`
		PackageID string      `json:"package_id"`
		Package   PackageInfo `json:"package"`
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"has_task": true,
		"task": TaskResponse{
			ID:        task.ID,
			Client:    ClientInfo{TargetPath: matchedTerm.TargetPath},
			PackageID: pkg.ID,
			Package: PackageInfo{
				ID:               pkg.ID,
				VersionLabel:     pkg.VersionLabel,
				OriginalFilename: pkg.FileName,
				SHA256:           pkg.SHA256,
				SizeBytes:        pkg.SizeBytes,
			},
		},
	})
}

// handleAgentCheckUpdate explicitly checks for pending updates for an outlet/terminal
func (s *Server) handleAgentCheckUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	outletCode := r.Header.Get("X-Outlet-Code")
	if outletCode == "" {
		outletCode = r.URL.Query().Get("outlet_code")
	}
	terminalCode := r.URL.Query().Get("terminal_code")
	apiKey := r.Header.Get("X-Terminal-Key")
	if apiKey == "" {
		apiKey = r.URL.Query().Get("api_key")
	}

	outletID, err := s.db.VerifyOutletAPIKey(outletCode, apiKey)
	if err != nil {
		http.Error(w, "Unauthorized: "+err.Error(), http.StatusUnauthorized)
		return
	}

	terms, err := s.db.ListTerminals(outletID)
	if err != nil {
		http.Error(w, "Database error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	var targetTerm *models.Terminal
	for _, t := range terms {
		if t.TerminalCode == terminalCode {
			targetTerm = &t
			break
		}
	}

	if targetTerm == nil {
		http.Error(w, "Terminal not registered yet", http.StatusNotFound)
		return
	}

	task, pkg, err := s.db.GetPendingTaskForTerminal(targetTerm.ID)
	if err != nil || task == nil || pkg == nil {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"has_update": false,
			"message":    "System is up to date",
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"has_update":   true,
		"task_id":      task.ID,
		"package":      pkg,
		"download_url": fmt.Sprintf("/api/v1/packages/%s/download", pkg.ID),
	})
}

// handleAgentDownloadPackage serves the JAR file with HTTP Range support for resumable downloads
func (s *Server) handleAgentDownloadPackage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	pkgID := r.URL.Query().Get("id")
	// If path based: /api/v1/packages/{id}/download
	if pkgID == "" {
		parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
		for i, part := range parts {
			if part == "packages" && i+1 < len(parts) {
				pkgID = parts[i+1]
				break
			}
		}
	}

	if pkgID == "" {
		http.Error(w, "Missing package id", http.StatusBadRequest)
		return
	}

	pkg, err := s.db.GetPackage(pkgID)
	if err != nil {
		http.Error(w, "Package not found: "+err.Error(), http.StatusNotFound)
		return
	}

	// Resolve physical file path
	filePath := pkg.StoragePath
	if !filepath.IsAbs(filePath) {
		filePath = filepath.Join(s.storageDir, filepath.Base(filePath))
	}

	file, err := os.Open(filePath)
	if err != nil {
		http.Error(w, "Physical artifact file missing on server: "+err.Error(), http.StatusNotFound)
		return
	}
	defer file.Close()

	stat, err := file.Stat()
	if err != nil {
		http.Error(w, "Error reading file metadata: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", pkg.FileName))
	w.Header().Set("Content-Type", "application/java-archive")
	w.Header().Set("X-File-SHA256", pkg.SHA256)
	w.Header().Set("X-Package-SHA256", pkg.SHA256)
	w.Header().Set("X-Package-Version", pkg.VersionLabel)
	w.Header().Set("Accept-Ranges", "bytes")

	http.ServeContent(w, r, pkg.FileName, stat.ModTime(), file)
}

// handleAgentTaskReport receives progress and completion reports from agents
func (s *Server) handleAgentTaskReport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	// Check if path is /api/v1/tasks/{task_id}/status
	var taskID string
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	for i, part := range parts {
		if part == "tasks" && i+1 < len(parts) {
			taskID = parts[i+1]
			break
		}
	}

	var req struct {
		TaskID       string `json:"task_id"`
		Status       string `json:"status"`
		Progress     int    `json:"progress"`
		ErrorCode    string `json:"error_code,omitempty"`
		ErrorMessage string `json:"error_message,omitempty"`
		LogMessage   string `json:"log_message,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	if req.TaskID != "" {
		taskID = req.TaskID
	}

	if taskID == "" {
		http.Error(w, "Missing task_id", http.StatusBadRequest)
		return
	}

	errMsg := req.ErrorMessage
	if errMsg == "" && req.LogMessage != "" {
		errMsg = req.LogMessage
	}

	// Normalize status
	statusUpper := strings.ToUpper(req.Status)
	if statusUpper == "SUCCESS" {
		statusUpper = "SUCCESS"
	} else if strings.Contains(statusUpper, "FAIL") {
		statusUpper = "FAILED"
	} else if strings.Contains(statusUpper, "ROLL") {
		statusUpper = "ROLLED_BACK"
	}

	if err := s.db.UpdateTaskProgress(taskID, statusUpper, req.Progress, errMsg); err != nil {
		http.Error(w, "Failed to update task: "+err.Error(), http.StatusInternalServerError)
		return
	}

	log.Printf("[Task Report] Task %s: Status=%s Progress=%d%% Msg=%s", taskID, statusUpper, req.Progress, errMsg)
	s.db.AddAuditLog("", "", "AGENT", "TASK_STATUS_UPDATE", fmt.Sprintf("Task %s reported status: %s (%d%%)", taskID, statusUpper, req.Progress))

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{
		"status":  "ok",
		"message": "Task progress recorded",
	})
}

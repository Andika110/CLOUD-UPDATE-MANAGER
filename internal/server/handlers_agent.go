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

	if req.OutletCode == "" || req.TerminalCode == "" {
		http.Error(w, "Missing outlet_code or terminal_code", http.StatusBadRequest)
		return
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
			DownloadURL:  fmt.Sprintf("/api/v1/agent/packages/download?id=%s&task_id=%s", pkg.ID, task.ID),
			IsMandatory:  pkg.IsMandatory,
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
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
		"has_update": true,
		"task_id":    task.ID,
		"package":    pkg,
		"download_url": fmt.Sprintf("/api/v1/agent/packages/download?id=%s&task_id=%s", pkg.ID, task.ID),
	})
}

// handleAgentDownloadPackage serves the JAR file with HTTP Range support for resumable downloads
func (s *Server) handleAgentDownloadPackage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	pkgID := r.URL.Query().Get("id")
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

	// Set headers for file download and integrity verification
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", pkg.FileName))
	w.Header().Set("X-File-SHA256", pkg.SHA256)
	w.Header().Set("X-Package-Version", pkg.VersionLabel)
	w.Header().Set("Accept-Ranges", "bytes")

	// http.ServeContent handles HTTP Range Requests (resumable downloads) automatically!
	http.ServeContent(w, r, pkg.FileName, stat.ModTime(), file)
}

// handleAgentTaskReport receives progress and completion reports from agents
func (s *Server) handleAgentTaskReport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	var req models.TaskReportRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	if req.TaskID == "" {
		http.Error(w, "Missing task_id", http.StatusBadRequest)
		return
	}

	if err := s.db.UpdateTaskProgress(req.TaskID, req.Status, req.Progress, req.ErrorMessage); err != nil {
		http.Error(w, "Failed to update task: "+err.Error(), http.StatusInternalServerError)
		return
	}

	log.Printf("[Task Report] Task %s: Status=%s Progress=%d%% Error=%s", req.TaskID, req.Status, req.Progress, req.ErrorMessage)
	s.db.AddAuditLog("", "", "AGENT", "TASK_STATUS_UPDATE", fmt.Sprintf("Task %s reported status: %s (Progress: %d%%)", req.TaskID, req.Status, req.Progress))

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{
		"status":  "ok",
		"message": "Task progress recorded",
	})
}

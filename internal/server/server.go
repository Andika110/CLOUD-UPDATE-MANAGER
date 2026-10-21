package server

import (
	"log"
	"net/http"
	"os"

	"quinos-cloud-update-manager/internal/db"
)

type Server struct {
	db          *db.DB
	storageDir  string
	webHTMLPath string
	mux         *http.ServeMux
}

func NewServer(database *db.DB, storageDir, webHTMLPath string) *Server {
	if err := os.MkdirAll(storageDir, 0755); err != nil {
		log.Printf("[Server] Error creating storage directory: %v", err)
	}

	s := &Server{
		db:          database,
		storageDir:  storageDir,
		webHTMLPath: webHTMLPath,
		mux:         http.NewServeMux(),
	}
	s.registerRoutes()
	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// CORS Headers for Dashboard / API
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Terminal-Key, X-Outlet-Code, Range")
	w.Header().Set("Access-Control-Expose-Headers", "Content-Range, Content-Length, Accept-Ranges")

	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}

	s.mux.ServeHTTP(w, r)
}

func (s *Server) registerRoutes() {
	// Web Dashboard
	s.mux.HandleFunc("/", s.handleWebDashboard)

	// Agent / Terminal APIs
	s.mux.HandleFunc("/api/v1/agent/heartbeat", s.handleAgentHeartbeat)
	s.mux.HandleFunc("/api/v1/agent/check-update", s.handleAgentCheckUpdate)
	s.mux.HandleFunc("/api/v1/agent/packages/download", s.handleAgentDownloadPackage)
	s.mux.HandleFunc("/api/v1/agent/tasks/report", s.handleAgentTaskReport)

	// Admin Dashboard APIs
	s.mux.HandleFunc("/api/v1/admin/overview", s.handleAdminOverview)
	s.mux.HandleFunc("/api/v1/admin/outlets", s.handleAdminOutlets)
	s.mux.HandleFunc("/api/v1/admin/terminals", s.handleAdminTerminals)
	s.mux.HandleFunc("/api/v1/admin/packages", s.handleAdminPackages)
	s.mux.HandleFunc("/api/v1/admin/packages/upload", s.handleAdminPackageUpload)
	s.mux.HandleFunc("/api/v1/admin/deployments", s.handleAdminDeployments)
	s.mux.HandleFunc("/api/v1/admin/terminals/command", s.handleAdminTerminalCommand)
	s.mux.HandleFunc("/api/v1/admin/logs", s.handleAdminLogs)
}

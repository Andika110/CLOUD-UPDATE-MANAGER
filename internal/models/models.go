package models

import "time"

// Outlet represents a restaurant branch or store location
type Outlet struct {
	ID              string     `json:"id"`
	OutletCode      string     `json:"outlet_code"` // e.g. "OUT-JKT-01"
	Name            string     `json:"name"`        // e.g. "Quinos Flagship Senopati"
	Timezone        string     `json:"timezone"`    // e.g. "Asia/Jakarta"
	APIKey          string     `json:"api_key"`     // Auth token for all terminals in this outlet
	ReleaseGroup    string     `json:"release_group"` // "canary", "pilot", "stable"
	Status          string     `json:"status"`      // "Active", "Suspended"
	TerminalCount   int        `json:"terminal_count,omitempty"`
	OnlineCount     int        `json:"online_count,omitempty"`
	LastSeenAt      *time.Time `json:"last_seen_at,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
}

// Terminal represents an individual cashier / POS machine in an outlet
type Terminal struct {
	ID                string     `json:"id"`
	OutletID          string     `json:"outlet_id"`
	OutletCode        string     `json:"outlet_code,omitempty"`
	OutletName        string     `json:"outlet_name,omitempty"`
	TerminalCode      string     `json:"terminal_code"` // e.g. "POS-01"
	Hostname          string     `json:"hostname"`
	IPAddress         string     `json:"ip_address"`
	OSInfo            string     `json:"os_info"`
	CurrentJARVersion string     `json:"current_jar_version"`
	AgentVersion      string     `json:"agent_version"`
	POSRunning        bool       `json:"pos_running"`
	POSPID            *int       `json:"pos_pid,omitempty"`
	TargetPath        string     `json:"target_path"`
	Status            string     `json:"status"`        // "Online", "Offline"
	UpdateStatus      string     `json:"update_status"` // "IDLE", "DOWNLOADING", "VERIFYING", "INSTALLING", "SUCCESS", "FAILED", "ROLLED_BACK"
	PendingCommand    string     `json:"pending_command,omitempty"`
	LastSeenAt        *time.Time `json:"last_seen_at,omitempty"`
	CreatedAt         time.Time  `json:"created_at"`
}

// Package represents an uploaded Quinos JAR artifact
type Package struct {
	ID             string    `json:"id"`
	VersionLabel   string    `json:"version_label"`   // e.g. "v2.5.0-STABLE"
	FileName       string    `json:"file_name"`       // e.g. "quinos-pos.jar"
	SizeBytes      int64     `json:"size_bytes"`
	SizeMB         string    `json:"size_mb"`
	SHA256         string    `json:"sha256"`
	Changelog      string    `json:"changelog"`
	ReleaseChannel string    `json:"release_channel"` // "canary", "pilot", "stable"
	IsMandatory    bool      `json:"is_mandatory"`
	StoragePath    string    `json:"storage_path,omitempty"`
	UploadedBy     string    `json:"uploaded_by"`
	Status         string    `json:"status"`          // "Active", "Draft", "Archived"
	CreatedAt      time.Time `json:"created_at"`
}

// Deployment represents a rollout event
type Deployment struct {
	ID             string           `json:"id"`
	PackageID      string           `json:"package_id"`
	PackageVersion string           `json:"package_version,omitempty"`
	TargetGroup    string           `json:"target_group"`    // "canary", "pilot", "all", or "outlet:OUT-JKT-01"
	Status         string           `json:"status"`          // "Active", "Completed", "Cancelled", "Partial"
	TotalTasks     int              `json:"total_tasks"`
	SuccessCount   int              `json:"success_count"`
	FailedCount    int              `json:"failed_count"`
	Notes          string           `json:"notes"`
	CreatedBy      string           `json:"created_by"`
	Tasks          []DeploymentTask `json:"tasks,omitempty"`
	CreatedAt      time.Time        `json:"created_at"`
}

// DeploymentTask represents an update operation on a specific terminal
type DeploymentTask struct {
	ID            string     `json:"id"`
	DeploymentID  string     `json:"deployment_id"`
	TerminalID    string     `json:"terminal_id"`
	TerminalCode  string     `json:"terminal_code,omitempty"`
	OutletCode    string     `json:"outlet_code,omitempty"`
	PackageID     string     `json:"package_id"`
	TargetVersion string     `json:"target_version"`
	Status        string     `json:"status"` // "PENDING", "DOWNLOADING", "VERIFYING", "INSTALLING", "SUCCESS", "FAILED", "ROLLED_BACK"
	Progress      int        `json:"progress"`
	ErrorMessage  string     `json:"error_message,omitempty"`
	StartedAt     *time.Time `json:"started_at,omitempty"`
	CompletedAt   *time.Time `json:"completed_at,omitempty"`
}

// AuditLog records security and deployment events
type AuditLog struct {
	ID           int64     `json:"id"`
	OutletCode   string    `json:"outlet_code,omitempty"`
	TerminalCode string    `json:"terminal_code,omitempty"`
	Actor        string    `json:"actor"`
	Action       string    `json:"action"`
	Details      string    `json:"details"`
	CreatedAt    time.Time `json:"created_at"`
}

// SystemOverview holds dashboard summary statistics
type SystemOverview struct {
	TotalOutlets       int    `json:"total_outlets"`
	ActiveOutlets      int    `json:"active_outlets"`
	TotalTerminals     int    `json:"total_terminals"`
	OnlineTerminals    int    `json:"online_terminals"`
	POSRunningCount    int    `json:"pos_running_count"`
	ActiveDeployments  int    `json:"active_deployments"`
	LatestVersion      string `json:"latest_version"`
	ServerTime         string `json:"server_time"`
	ServerVersion      string `json:"server_version"`
}

// HeartbeatRequest payload sent by client agent
type HeartbeatRequest struct {
	OutletCode        string `json:"outlet_code"`
	TerminalCode      string `json:"terminal_code"`
	Hostname          string `json:"hostname"`
	IPAddress         string `json:"ip_address"`
	OSInfo            string `json:"os_info"`
	CurrentJARVersion string `json:"current_jar_version"`
	AgentVersion      string `json:"agent_version"`
	POSRunning        bool   `json:"pos_running"`
	POSPID            *int   `json:"pos_pid,omitempty"`
	TargetPath        string `json:"target_path"`
	UpdateStatus      string `json:"update_status,omitempty"`
}

// HeartbeatResponse returned to client agent
type HeartbeatResponse struct {
	Status         string `json:"status"` // "ok"
	ServerTime     string `json:"server_time"`
	PendingCommand string `json:"pending_command,omitempty"`
	HasUpdate      bool   `json:"has_update"`
	UpdateInfo     *struct {
		TaskID       string `json:"task_id"`
		PackageID    string `json:"package_id"`
		VersionLabel string `json:"version_label"`
		FileName     string `json:"file_name"`
		SizeBytes    int64  `json:"size_bytes"`
		SHA256       string `json:"sha256"`
		DownloadURL  string `json:"download_url"`
		IsMandatory  bool   `json:"is_mandatory"`
	} `json:"update_info,omitempty"`
}

// TaskReportRequest payload when agent reports progress or completion
type TaskReportRequest struct {
	TaskID       string `json:"task_id"`
	Status       string `json:"status"` // "DOWNLOADING", "VERIFYING", "INSTALLING", "SUCCESS", "FAILED", "ROLLED_BACK"
	Progress     int    `json:"progress"`
	ErrorMessage string `json:"error_message,omitempty"`
}

package process

// Process represents a managed process configuration
type Process struct {
	ID               int64  `json:"id"`
	Name             string `json:"name"`
	Command          string `json:"command"`
	Args             string `json:"args"`
	Dir              string `json:"dir"`
	Env              string `json:"env"` // JSON string of env vars
	AutoRestart      bool   `json:"auto_restart"`
	MaxRestarts      int    `json:"max_restarts"`
	RestartDelay     int    `json:"restart_delay"`   // seconds (base delay for backoff)
	StopTimeout      int    `json:"stop_timeout"`    // seconds, SIGTERM wait before SIGKILL (default 10)
	StartupTimeout   int    `json:"startup_timeout"` // seconds, max time for "starting" state (default 30)
	AutoStart        bool   `json:"auto_start"`
	LogFile          string `json:"log_file"`
	GroupID          int64  `json:"group_id"`
	RuntimeVersionID int64  `json:"runtime_version_id"` // FK → runtime_version.id; NOT NULL since Issue 02
	RuntimeLang      string `json:"runtime_lang"`       // joined: runtime_version.lang (read-only)
	RuntimeExact     string `json:"runtime_exact"`      // joined: runtime_version.exact (read-only)
	CreatedAt        string `json:"created_at"`
	UpdatedAt        string `json:"updated_at"`
}

// ProcessStatus represents the runtime status of a managed process
type ProcessStatus struct {
	ID         int64   `json:"id"`
	ProcessID  int64   `json:"process_id"`
	Status     string  `json:"status"` // running, stopped, error, starting, stopping
	PID        int     `json:"pid"`
	Uptime     int64   `json:"uptime"` // seconds
	Restarts   int     `json:"restarts"`
	CPUPercent float64 `json:"cpu_percent"`
	MemoryMB   float64 `json:"memory_mb"`
	ExitCode   int     `json:"exit_code"`
	LastStart  string  `json:"last_start"`
	LastError  string  `json:"last_error"`
	UpdatedAt  string  `json:"updated_at"`
}

// ProcessLog represents a log entry for a managed process
type ProcessLog struct {
	ID        int64  `json:"id"`
	ProcessID int64  `json:"process_id"`
	Type      string `json:"type"` // stdout, stderr, system
	Content   string `json:"content"`
	CreatedAt string `json:"created_at"`
}

// ProcessGroup represents a logical group of processes
type ProcessGroup struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	CreatedAt   string `json:"created_at"`
}

// ProcessWithStatus combines process config with its runtime status
type ProcessWithStatus struct {
	Process
	Status *ProcessStatus `json:"status,omitempty"`
	Group  *ProcessGroup  `json:"group,omitempty"`
}

// --- Request types ---

// CreateProcessRequest is the request body for creating a process
type CreateProcessRequest struct {
	Name             string `json:"name" binding:"required"`
	Command          string `json:"command" binding:"required"`
	Args             string `json:"args"`
	Dir              string `json:"dir"`
	Env              string `json:"env"`
	AutoRestart      *bool  `json:"auto_restart"`
	MaxRestarts      int    `json:"max_restarts"`
	RestartDelay     int    `json:"restart_delay"`
	StopTimeout      int    `json:"stop_timeout"`
	StartupTimeout   int    `json:"startup_timeout"`
	AutoStart        *bool  `json:"auto_start"`
	LogFile          string `json:"log_file"`
	GroupID          int64  `json:"group_id"`
	RuntimeVersionID int64  `json:"runtime_version_id" binding:"required,min=1"`
}

// UpdateProcessRequest is the request body for updating a process
type UpdateProcessRequest struct {
	Name             *string `json:"name"`
	Command          *string `json:"command"`
	Args             *string `json:"args"`
	Dir              *string `json:"dir"`
	Env              *string `json:"env"`
	AutoRestart      *bool   `json:"auto_restart"`
	MaxRestarts      *int    `json:"max_restarts"`
	RestartDelay     *int    `json:"restart_delay"`
	StopTimeout      *int    `json:"stop_timeout"`
	StartupTimeout   *int    `json:"startup_timeout"`
	AutoStart        *bool   `json:"auto_start"`
	LogFile          *string `json:"log_file"`
	GroupID          *int64  `json:"group_id"`
	RuntimeVersionID *int64  `json:"runtime_version_id"`
}

// CreateProcessGroupRequest is the request body for creating a process group
type CreateProcessGroupRequest struct {
	Name        string `json:"name" binding:"required"`
	Description string `json:"description"`
}

// UpdateProcessGroupRequest is the request body for updating a process group
type UpdateProcessGroupRequest struct {
	Name        *string `json:"name"`
	Description *string `json:"description"`
}

// BatchProcessIDs is the request body for batch operations
type BatchProcessIDs struct {
	IDs []int64 `json:"ids" binding:"required"`
}

// ProcessStats contains runtime resource statistics
type ProcessStats struct {
	CPUPercent float64 `json:"cpu_percent"`
	MemoryMB   float64 `json:"memory_mb"`
	PID        int     `json:"pid"`
	Uptime     int64   `json:"uptime"`
	Restarts   int     `json:"restarts"`
}

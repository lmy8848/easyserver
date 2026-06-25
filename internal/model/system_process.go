package model

// SystemProcess represents a running system process
type SystemProcess struct {
	PID        int     `json:"pid"`
	PPID       int     `json:"ppid"`
	Name       string  `json:"name"`
	User       string  `json:"user"`
	State      string  `json:"state"` // R=running, S=sleeping, D=disk-sleep, Z=zombie, T=stopped
	CPUPercent float64 `json:"cpu_percent"`
	MemoryMB   float64 `json:"memory_mb"`
	MemPercent float64 `json:"mem_percent"`
	StartTime  string  `json:"start_time"`
	Command    string  `json:"command"`
	Threads    int     `json:"threads"`
}

// SystemService represents a systemd service
type SystemService struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	ActiveState string `json:"active_state"` // active, inactive, failed, activating, deactivating
	SubState    string `json:"sub_state"`    // running, dead, exited, waiting, etc.
	PID         int    `json:"pid"`
	LoadState   string `json:"load_state"` // loaded, not-found
	Enabled     bool   `json:"enabled"`    // enabled/disabled in systemd
}

// SystemServiceAction represents an action on a systemd service
type SystemServiceAction struct {
	Action string `json:"action" binding:"required"` // start, stop, restart, enable, disable
}

// ServiceWhitelistEntry represents a whitelisted service for management
type ServiceWhitelistEntry struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	CreatedAt string `json:"created_at"`
}

// SystemOverview contains system-wide resource statistics
type SystemOverview struct {
	CPUUsage    float64          `json:"cpu_usage"`    // percent
	MemoryTotal int64            `json:"memory_total"` // MB
	MemoryUsed  int64            `json:"memory_used"`  // MB
	MemoryUsage float64          `json:"memory_usage"` // percent
	SwapTotal   int64            `json:"swap_total"`   // MB
	SwapUsed    int64            `json:"swap_used"`    // MB
	LoadAvg     [3]float64       `json:"load_avg"`     // 1min, 5min, 15min
	Uptime      int64            `json:"uptime"`       // seconds
	TopCPU      []SystemProcess  `json:"top_cpu"`      // top 5 by CPU
	TopMem      []SystemProcess  `json:"top_mem"`      // top 5 by memory
	TotalProcs  int              `json:"total_procs"`
	RunningProcs int             `json:"running_procs"`
}

// SystemProcessListRequest is the request for listing system processes
type SystemProcessListRequest struct {
	SortBy  string `form:"sort_by"`  // cpu, memory, pid, name
	Order   string `form:"order"`    // asc, desc
	Search  string `form:"search"`   // filter by name/command
	Limit   int    `form:"limit"`    // max results (default 100)
}

package systemprocess

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

// SystemProcessListRequest is the request for listing system processes
type SystemProcessListRequest struct {
	SortBy string `form:"sort_by"` // cpu, memory, pid, name
	Order  string `form:"order"`   // asc, desc
	Search string `form:"search"`  // filter by name/command
	Limit  int    `form:"limit"`   // max results (default 100)
}

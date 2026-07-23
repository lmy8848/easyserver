package monitor

import (
	"math"
)

// MonitorPoint represents a single monitoring data point
type MonitorPoint struct {
	ID           int64   `json:"id" db:"id"`
	CPUPercent   float64 `json:"cpu_percent" db:"cpu"`
	CPULoad1m    float64 `json:"cpu_load_1m" db:"cpu_load_1m"`
	CPULoad5m    float64 `json:"cpu_load_5m" db:"cpu_load_5m"`
	CPULoad15m   float64 `json:"cpu_load_15m" db:"cpu_load_15m"`
	MemTotal     uint64  `json:"mem_total" db:"mem_total"`
	MemUsed      uint64  `json:"mem_used" db:"mem_used"`
	MemPercent   float64 `json:"mem_percent" db:"mem_usage"`
	DiskTotal    uint64  `json:"disk_total" db:"disk_total"`
	DiskUsed     uint64  `json:"disk_used" db:"disk_used"`
	DiskPercent  float64 `json:"disk_percent" db:"disk_usage"`
	NetBytesSent uint64  `json:"net_bytes_sent" db:"net_bytes_sent"`
	NetBytesRecv uint64  `json:"net_bytes_recv" db:"net_bytes_recv"`
	SwapTotal    uint64  `json:"-" db:"-"`
	SwapUsed     uint64  `json:"-" db:"-"`
	Timestamp    string  `json:"timestamp" db:"timestamp"`
}

// DiskPartition represents a single disk partition
type DiskPartition struct {
	MountPoint   string  `json:"mount_point"`
	Device       string  `json:"device"`
	FSType       string  `json:"fs_type"`
	TotalBytes   uint64  `json:"total_bytes"`
	UsedBytes    uint64  `json:"used_bytes"`
	UsagePercent float64 `json:"usage_percent"`
}

// SystemInfo holds static system information
type SystemInfo struct {
	Hostname      string `json:"hostname"`
	OS            string `json:"os"`
	Kernel        string `json:"kernel"`
	Arch          string `json:"arch"`
	CPUCores      int    `json:"cpu_cores"`
	UptimeSeconds int64  `json:"uptime_seconds"`
}

// MonitorSnapshot is the API response format
type MonitorSnapshot struct {
	CPU struct {
		UsagePercent float64 `json:"usage_percent"`
		Load1m       float64 `json:"load_1m"`
		Load5m       float64 `json:"load_5m"`
		Load15m      float64 `json:"load_15m"`
	} `json:"cpu"`
	Memory struct {
		TotalBytes   uint64  `json:"total_bytes"`
		UsedBytes    uint64  `json:"used_bytes"`
		UsagePercent float64 `json:"usage_percent"`
	} `json:"memory"`
	Swap struct {
		TotalBytes   uint64  `json:"total_bytes"`
		UsedBytes    uint64  `json:"used_bytes"`
		UsagePercent float64 `json:"usage_percent"`
	} `json:"swap,omitempty"`
	Disk struct {
		MountPoint   string  `json:"mount_point"`
		TotalBytes   uint64  `json:"total_bytes"`
		UsedBytes    uint64  `json:"used_bytes"`
		UsagePercent float64 `json:"usage_percent"`
	} `json:"disk"`
	Network struct {
		BytesSent uint64 `json:"bytes_sent"`
		BytesRecv uint64 `json:"bytes_recv"`
	} `json:"network"`
	Partitions []DiskPartition `json:"partitions,omitempty"`
	System     *SystemInfo     `json:"system,omitempty"`
	Timestamp  string          `json:"timestamp"`
}

// ToSnapshot converts MonitorPoint to API response format
func (p *MonitorPoint) ToSnapshot() *MonitorSnapshot {
	s := &MonitorSnapshot{
		Timestamp: p.Timestamp,
	}

	s.CPU.UsagePercent = p.CPUPercent
	s.CPU.Load1m = p.CPULoad1m
	s.CPU.Load5m = p.CPULoad5m
	s.CPU.Load15m = p.CPULoad15m

	s.Memory.TotalBytes = p.MemTotal
	s.Memory.UsedBytes = p.MemUsed
	s.Memory.UsagePercent = p.MemPercent

	if p.SwapTotal > 0 {

		s.Swap = struct {
			TotalBytes   uint64  `json:"total_bytes"`
			UsedBytes    uint64  `json:"used_bytes"`
			UsagePercent float64 `json:"usage_percent"`
		}{
			TotalBytes:   p.SwapTotal,
			UsedBytes:    p.SwapUsed,
			UsagePercent: math.Round(float64(p.SwapUsed)/float64(p.SwapTotal)*100*100) / 100,
		}
	}

	s.Disk = struct {
		MountPoint   string  `json:"mount_point"`
		TotalBytes   uint64  `json:"total_bytes"`
		UsedBytes    uint64  `json:"used_bytes"`
		UsagePercent float64 `json:"usage_percent"`
	}{
		MountPoint:   "/",
		TotalBytes:   p.DiskTotal,
		UsedBytes:    p.DiskUsed,
		UsagePercent: p.DiskPercent,
	}

	s.Network.BytesSent = p.NetBytesSent
	s.Network.BytesRecv = p.NetBytesRecv

	return s
}

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

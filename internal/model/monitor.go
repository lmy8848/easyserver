package model

import "time"

// MonitorPoint represents a single monitoring data point
type MonitorPoint struct {
	ID           int64   `json:"id" db:"id"`
	CPUPercent   float64 `json:"cpu_percent" db:"cpu"`
	CPULoad1m    float64 `json:"cpu_load_1m" db:"cpu_load_1m"`
	CPULoad5m    float64 `json:"cpu_load_5m" db:"cpu_load_5m"`
	CPULoad15m   float64 `json:"cpu_load_15m" db:"cpu_load_15m"`
	MemTotal     uint64  `json:"mem_total" db:"mem_total"`
	MemUsed      uint64  `json:"mem_used" db:"mem_used"`
	MemAvailable uint64  `json:"mem_available" db:"mem_available"`
	MemPercent   float64 `json:"mem_percent" db:"mem_usage"`
	DiskTotal    uint64  `json:"disk_total" db:"disk_total"`
	DiskUsed     uint64  `json:"disk_used" db:"disk_used"`
	DiskFree     uint64  `json:"disk_free" db:"disk_free"`
	DiskPercent  float64 `json:"disk_percent" db:"disk_usage"`
	NetBytesSent uint64  `json:"net_bytes_sent" db:"net_bytes_sent"`
	NetBytesRecv uint64  `json:"net_bytes_recv" db:"net_bytes_recv"`
	NetPktsSent  uint64  `json:"net_packets_sent" db:"net_packets_sent"`
	NetPktsRecv  uint64  `json:"net_packets_recv" db:"net_packets_recv"`
	Timestamp    string  `json:"timestamp" db:"timestamp"`
}

// DiskPartition represents a single disk partition
type DiskPartition struct {
	MountPoint   string  `json:"mount_point"`
	Device       string  `json:"device"`
	FSType       string  `json:"fs_type"`
	TotalBytes   uint64  `json:"total_bytes"`
	UsedBytes    uint64  `json:"used_bytes"`
	FreeBytes    uint64  `json:"free_bytes"`
	UsagePercent float64 `json:"usage_percent"`
}

// SwapInfo represents swap memory info
type SwapInfo struct {
	TotalBytes   uint64  `json:"total_bytes"`
	UsedBytes    uint64  `json:"used_bytes"`
	FreeBytes    uint64  `json:"free_bytes"`
	UsagePercent float64 `json:"usage_percent"`
}

// ProcessInfo represents a top process
type ProcessInfo struct {
	PID        int     `json:"pid"`
	Name       string  `json:"name"`
	User       string  `json:"user"`
	CPUPercent float64 `json:"cpu_percent"`
	MemPercent float64 `json:"mem_percent"`
	MemBytes   uint64  `json:"mem_bytes"`
	State      string  `json:"state"`
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
	Swap *SwapInfo `json:"swap,omitempty"`
	Disk []struct {
		MountPoint   string  `json:"mount_point"`
		TotalBytes   uint64  `json:"total_bytes"`
		UsedBytes    uint64  `json:"used_bytes"`
		UsagePercent float64 `json:"usage_percent"`
	} `json:"disk"`
	Partitions []DiskPartition `json:"partitions,omitempty"`
	Network    struct {
		BytesSent uint64 `json:"bytes_sent"`
		BytesRecv uint64 `json:"bytes_recv"`
	} `json:"network"`
	System     *SystemInfo   `json:"system,omitempty"`
	TopProcess []ProcessInfo `json:"top_process,omitempty"`
	Timestamp  string        `json:"timestamp"`
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

	s.Disk = []struct {
		MountPoint   string  `json:"mount_point"`
		TotalBytes   uint64  `json:"total_bytes"`
		UsedBytes    uint64  `json:"used_bytes"`
		UsagePercent float64 `json:"usage_percent"`
	}{
		{
			MountPoint:   "/",
			TotalBytes:   p.DiskTotal,
			UsedBytes:    p.DiskUsed,
			UsagePercent: p.DiskPercent,
		},
	}

	s.Network.BytesSent = p.NetBytesSent
	s.Network.BytesRecv = p.NetBytesRecv

	return s
}

// MonitorData is the legacy format (keep for compatibility)
type MonitorData struct {
	ID        int64     `json:"id" db:"id"`
	CPU       float64   `json:"cpu" db:"cpu"`
	Memory    MemInfo   `json:"memory"`
	Disk      DiskInfo  `json:"disk"`
	Network   NetInfo   `json:"network"`
	Timestamp time.Time `json:"timestamp" db:"timestamp"`
}

type MemInfo struct {
	Total     uint64  `json:"total"`
	Used      uint64  `json:"used"`
	Available uint64  `json:"available"`
	Usage     float64 `json:"usage"`
}

type DiskInfo struct {
	Total uint64  `json:"total"`
	Used  uint64  `json:"used"`
	Free  uint64  `json:"free"`
	Usage float64 `json:"usage"`
}

type NetInfo struct {
	BytesSent   uint64 `json:"bytes_sent"`
	BytesRecv   uint64 `json:"bytes_recv"`
	PacketsSent uint64 `json:"packets_sent"`
	PacketsRecv uint64 `json:"packets_recv"`
}

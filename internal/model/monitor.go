package model

import "easyserver/internal/monitor"

// All types below are now defined in easyserver/internal/monitor.
// Kept as aliases for backward compatibility.

type MonitorPoint = monitor.MonitorPoint
type DiskPartition = monitor.DiskPartition
type SwapInfo = monitor.SwapInfo
type ProcessInfo = monitor.ProcessInfo
type SystemInfo = monitor.SystemInfo
type MonitorSnapshot = monitor.MonitorSnapshot
type MonitorData = monitor.MonitorData
type MemInfo = monitor.MemInfo
type DiskInfo = monitor.DiskInfo
type NetInfo = monitor.NetInfo

package service

import (
	"time"

	"easyserver/internal/monitor"
)

// MonitorService, MonitorHub, MonitorClient are now defined in easyserver/internal/monitor.
// Kept as aliases for backward compatibility.
type MonitorService = monitor.MonitorService
type MonitorHub = monitor.MonitorHub
type MonitorClient = monitor.MonitorClient

// NewMonitorService creates a new MonitorService.
func NewMonitorService(monitorRepo monitor.Repository, interval, retention time.Duration) *MonitorService {
	return monitor.NewMonitorService(monitorRepo, interval, retention)
}

// NewMonitorHub creates a new MonitorHub.
func NewMonitorHub() *MonitorHub {
	return monitor.NewMonitorHub()
}

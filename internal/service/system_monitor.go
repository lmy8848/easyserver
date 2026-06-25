package service

import (
	"context"
	"log"
	"time"

	"easyserver/internal/executor"
)

// SystemEventMonitor monitors system-level events (disk, memory, service failures)
// and logs them as audit events.
// Platform-specific implementations are in:
//   - system_monitor_linux.go
//   - system_monitor_windows.go
type SystemEventMonitor struct {
	auditService *AuditService
	executor     executor.CommandExecutor
	stopCh       chan struct{}
}

func NewSystemEventMonitor(auditService *AuditService, exec executor.CommandExecutor) *SystemEventMonitor {
	return &SystemEventMonitor{
		auditService: auditService,
		executor:     exec,
		stopCh:       make(chan struct{}),
	}
}

func (m *SystemEventMonitor) Start() {
	log.Println("system_monitor: starting system event monitor")

	// Monitor system events
	go m.monitorSystemEvents()
	go m.monitorServiceFailures()
}

func (m *SystemEventMonitor) Stop() {
	close(m.stopCh)
}

func (m *SystemEventMonitor) monitorSystemEvents() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			m.checkSystemEvents()
		case <-m.stopCh:
			return
		}
	}
}

// checkSystemEvents checks for system-level events that may need attention.
// Implementation is platform-specific.
func (m *SystemEventMonitor) checkSystemEvents() {
	// Check disk space
	m.checkDiskSpace()

	// Check memory usage
	m.checkMemoryUsage()
}

func (m *SystemEventMonitor) monitorServiceFailures() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			m.checkServiceFailures()
		case <-m.stopCh:
			return
		}
	}
}

// Platform-specific methods (defined in system_monitor_linux.go / system_monitor_windows.go):
//   - checkDiskSpace()
//   - checkMemoryUsage()
//   - checkServiceFailures()

// logEvent is a helper to log a system event (avoids import in platform files).
func (m *SystemEventMonitor) logEvent(ctx context.Context, action, detail string) {
	m.auditService.LogSystemEvent(ctx, action, detail)
}

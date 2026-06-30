package monitor

import (
	"context"
	"log"
	"time"

	"easyserver/internal/audit"
	"easyserver/internal/infra/executor"
)

// EventMonitor monitors system-level events (disk, memory, service failures)
// and logs them as audit events. Linux implementation lives in event_monitor_linux.go.
type EventMonitor struct {
	auditService *audit.Service
	executor     executor.CommandExecutor
	stopCh       chan struct{}
}

// NewEventMonitor creates a new EventMonitor.
func NewEventMonitor(auditService *audit.Service, exec executor.CommandExecutor) *EventMonitor {
	return &EventMonitor{
		auditService: auditService,
		executor:     exec,
		stopCh:       make(chan struct{}),
	}
}

// Start starts the event monitor.
func (m *EventMonitor) Start() {
	log.Println("system_monitor: starting system event monitor")

	go m.monitorSystemEvents()
	go m.monitorServiceFailures()
}

// Stop stops the event monitor.
func (m *EventMonitor) Stop() {
	close(m.stopCh)
}

func (m *EventMonitor) monitorSystemEvents() {
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

func (m *EventMonitor) checkSystemEvents() {
	m.checkDiskSpace()
	m.checkMemoryUsage()
}

func (m *EventMonitor) monitorServiceFailures() {
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

// logEvent is a helper to log a system event. summary is a human-readable Chinese description.
func (m *EventMonitor) logEvent(ctx context.Context, summary string) {
	m.auditService.LogSystemEvent(ctx, summary)
}

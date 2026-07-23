//go:build linux
// +build linux

package monitor

import (
	"bufio"
	"context"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"easyserver/internal/audit"
	"easyserver/internal/infra/executor"
)

// EventMonitor monitors system-level events (disk, memory, service failures)
// and logs them as audit events.
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

// checkDiskSpace checks disk usage on all mounted partitions.
// Alerts if any partition exceeds 90% usage.
func (m *EventMonitor) checkDiskSpace() {
	output, _, err := m.executor.RunCombined(nil, "df", "-h", "--output=target,pcent")
	if err != nil {
		log.Printf("system_monitor: failed to run df: %v", err)
		return
	}

	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	for scanner.Scan() {
		line := scanner.Text()
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}

		mountPoint := fields[0]
		usageStr := strings.TrimSuffix(fields[1], "%")
		usage, err := strconv.Atoi(usageStr)
		if err != nil {
			continue
		}

		if usage >= 90 {
			m.logEvent(context.Background(), "磁盘使用率告警："+mountPoint+" "+usageStr+"%")
		}
	}
}

// checkMemoryUsage checks memory usage from /proc/meminfo.
// Alerts if memory usage exceeds 90%.
func (m *EventMonitor) checkMemoryUsage() {
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return
	}

	mem := make(map[string]uint64)
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		valStr := strings.TrimSpace(parts[1])
		valStr = strings.TrimSuffix(valStr, " kB")
		valStr = strings.TrimSpace(valStr)
		val, err := strconv.ParseUint(valStr, 10, 64)
		if err != nil {
			continue
		}
		mem[key] = val * 1024 // Convert kB to bytes
	}

	total := mem["MemTotal"]
	available := mem["MemAvailable"]
	if total > 0 && available > 0 {
		used := total - available
		percent := float64(used) / float64(total) * 100
		if percent >= 90 {
			m.logEvent(context.Background(), "内存使用率告警："+strconv.FormatFloat(percent, 'f', 1, 64)+"%")
		}
	}
}

// checkServiceFailures checks for failed systemd services.
func (m *EventMonitor) checkServiceFailures() {
	output, _, err := m.executor.RunCombined(nil, "systemctl", "list-units", "--type=service", "--state=failed", "--no-pager", "--plain")
	if err != nil {
		return
	}

	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	for scanner.Scan() {
		line := scanner.Text()
		if strings.Contains(line, "failed") {
			fields := strings.Fields(line)
			if len(fields) >= 1 {
				serviceName := strings.TrimSuffix(fields[0], ".service")
				m.logEvent(context.Background(), "系统服务异常："+serviceName)
			}
		}
	}
}

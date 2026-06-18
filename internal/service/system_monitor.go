package service

import (
	"bufio"
	"log"
	"os"
	"os/exec"
	"strings"
	"time"
)

type SystemEventMonitor struct {
	auditService *AuditService
	stopCh       chan struct{}
}

func NewSystemEventMonitor(auditService *AuditService) *SystemEventMonitor {
	return &SystemEventMonitor{
		auditService: auditService,
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

func (m *SystemEventMonitor) checkSystemEvents() {
	// Check for recent reboots
	if data, err := os.ReadFile("/proc/uptime"); err == nil {
		fields := strings.Fields(string(data))
		if len(fields) >= 1 {
			// If uptime is less than 5 minutes, system might have rebooted
			// This is a simplified check
		}
	}

	// Check disk space
	m.checkDiskSpace()

	// Check memory usage
	m.checkMemoryUsage()
}

func (m *SystemEventMonitor) checkDiskSpace() {
	cmd := exec.Command("df", "-h", "/")
	output, err := cmd.Output()
	if err != nil {
		return
	}

	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	for scanner.Scan() {
		line := scanner.Text()
		if strings.Contains(line, "/") {
			fields := strings.Fields(line)
			if len(fields) >= 5 {
				usage := fields[4]
				usage = strings.TrimSuffix(usage, "%")
				// Alert if disk usage > 90%
				if len(usage) > 0 {
					// Parse usage percentage
					// If > 90%, log warning
				}
			}
		}
	}
}

func (m *SystemEventMonitor) checkMemoryUsage() {
	if data, err := os.ReadFile("/proc/meminfo"); err == nil {
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
			// Parse value
			mem[key] = 0
			// Note: In production, parse the actual value
			_ = valStr
		}
	}
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

func (m *SystemEventMonitor) checkServiceFailures() {
	cmd := exec.Command("systemctl", "list-units", "--type=service", "--state=failed", "--no-pager", "--plain")
	output, err := cmd.Output()
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
				m.auditService.LogSystemEvent("SERVICE_FAILED",
					"Service "+serviceName+" has failed")
			}
		}
	}
}

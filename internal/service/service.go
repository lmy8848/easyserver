//go:build linux
// +build linux

package service

import (
	"encoding/json"
	"fmt"
	"log"
	"os/exec"
	"strings"
	"sync"
	"time"
)

type ServiceInfo struct {
	Name          string  `json:"name"`
	Description   string  `json:"description"`
	State         string  `json:"state"`
	SubState      string  `json:"sub_state"`
	Enabled       bool    `json:"enabled"`
	PID           int     `json:"pid"`
	MemoryBytes   uint64  `json:"memory_bytes"`
	CPUPercent    float64 `json:"cpu_percent"`
	UptimeSeconds int64   `json:"uptime_seconds"`
}

type LogLine struct {
	Time     string `json:"time"`
	Message  string `json:"message"`
	Priority string `json:"priority"`
}

// journalEntry represents a journalctl JSON output line
type journalEntry struct {
	Message              string `json:"MESSAGE"`
	RealtimeTimestamp    string `json:"__REALTIME_TIMESTAMP"`
	Priority             string `json:"PRIORITY"`
	SyslogIdentifier    string `json:"SYSLOG_IDENTIFIER"`
	Transport            string `json:"_TRANSPORT"`
}

type ServiceManager struct {
	mu sync.RWMutex
}

func NewServiceManager() *ServiceManager {
	return &ServiceManager{}
}

// List returns all systemd services
func (m *ServiceManager) List() ([]ServiceInfo, error) {
	// Get list of services with detailed info in one call
	cmd := exec.Command("systemctl", "list-units", "--type=service", "--all", "--no-pager", "--plain", "--full")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to list services: %w", err)
	}

	// Get all enabled/disabled status in one call
	enabledMap := m.getAllEnabledStatus()

	var services []ServiceInfo
	lines := strings.Split(string(output), "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "UNIT") || strings.HasPrefix(line, "LOAD") {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}

		svc := ServiceInfo{
			Name:     strings.TrimSuffix(fields[0], ".service"),
			State:    fields[2],
			SubState: fields[3],
		}

		// Get description
		if len(fields) > 4 {
			svc.Description = strings.Join(fields[4:], " ")
		}

		// Check enabled status from map
		svc.Enabled = enabledMap[svc.Name]

		services = append(services, svc)
	}

	// Batch get PID and memory for all services
	m.batchGetDetailedInfo(services)

	return services, nil
}

// getAllEnabledStatus gets enabled status for all services in one call
func (m *ServiceManager) getAllEnabledStatus() map[string]bool {
	cmd := exec.Command("systemctl", "list-unit-files", "--type=service", "--no-pager", "--plain")
	output, err := cmd.Output()
	if err != nil {
		return nil
	}

	result := make(map[string]bool)
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "UNIT") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) >= 2 {
			name := strings.TrimSuffix(fields[0], ".service")
			result[name] = fields[1] == "enabled"
		}
	}
	return result
}

// batchGetDetailedInfo gets PID and memory for multiple services efficiently
func (m *ServiceManager) batchGetDetailedInfo(services []ServiceInfo) {
	if len(services) == 0 {
		return
	}

	// Build property query for all services
	args := []string{"show"}
	for _, svc := range services {
		args = append(args, svc.Name+".service")
	}
	args = append(args, "--property=Id,MainPID,MemoryCurrent,ActiveState")

	cmd := exec.Command("systemctl", args...)
	output, err := cmd.Output()
	if err != nil {
		return
	}

	// Parse output - each service block is separated by empty lines
	currentName := ""
	props := make(map[string]string)

	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			// Process accumulated properties
			if currentName != "" {
				for i := range services {
					if services[i].Name+".service" == currentName || services[i].Name == currentName {
						if v, ok := props["MainPID"]; ok {
							fmt.Sscanf(v, "%d", &services[i].PID)
						}
						if v, ok := props["MemoryCurrent"]; ok {
							fmt.Sscanf(v, "%d", &services[i].MemoryBytes)
						}
						break
					}
				}
			}
			currentName = ""
			props = make(map[string]string)
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			if parts[0] == "Id" {
				currentName = parts[1]
			}
			props[parts[0]] = parts[1]
		}
	}

	// Process last block
	if currentName != "" {
		for i := range services {
			if services[i].Name+".service" == currentName || services[i].Name == currentName {
				if v, ok := props["MainPID"]; ok {
					fmt.Sscanf(v, "%d", &services[i].PID)
				}
				if v, ok := props["MemoryCurrent"]; ok {
					fmt.Sscanf(v, "%d", &services[i].MemoryBytes)
				}
				break
			}
		}
	}
}

// Get returns info for a specific service
func (m *ServiceManager) Get(name string) (*ServiceInfo, error) {
	cmd := exec.Command("systemctl", "show", name+".service",
		"--property=ActiveState,SubState,MainPID,MemoryCurrent,Description")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get service info: %w", err)
	}

	svc := &ServiceInfo{
		Name: name,
	}

	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := parts[0]
		value := parts[1]

		switch key {
		case "ActiveState":
			svc.State = value
		case "SubState":
			svc.SubState = value
		case "MainPID":
			fmt.Sscanf(value, "%d", &svc.PID)
		case "MemoryCurrent":
			fmt.Sscanf(value, "%d", &svc.MemoryBytes)
		case "Description":
			svc.Description = value
		}
	}

	svc.Enabled = m.isEnabled(name)

	return svc, nil
}

// Start starts a service
func (m *ServiceManager) Start(name string) error {
	cmd := exec.Command("systemctl", "start", name+".service")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to start service: %s", string(output))
	}
	log.Printf("service: started %s", name)
	return nil
}

// Stop stops a service
func (m *ServiceManager) Stop(name string) error {
	cmd := exec.Command("systemctl", "stop", name+".service")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to stop service: %s", string(output))
	}
	log.Printf("service: stopped %s", name)
	return nil
}

// Restart restarts a service
func (m *ServiceManager) Restart(name string) error {
	cmd := exec.Command("systemctl", "restart", name+".service")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to restart service: %s", string(output))
	}
	log.Printf("service: restarted %s", name)
	return nil
}

// Enable enables a service for auto-start
func (m *ServiceManager) Enable(name string) error {
	cmd := exec.Command("systemctl", "enable", name+".service")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to enable service: %s", string(output))
	}
	log.Printf("service: enabled %s", name)
	return nil
}

// Disable disables a service from auto-start
func (m *ServiceManager) Disable(name string) error {
	cmd := exec.Command("systemctl", "disable", name+".service")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to disable service: %s", string(output))
	}
	log.Printf("service: disabled %s", name)
	return nil
}

// GetLogs returns recent logs for a service
func (m *ServiceManager) GetLogs(name string, tail int, since string) ([]LogLine, error) {
	args := []string{
		"-u", name + ".service",
		"--no-pager",
		"--output=json",
	}

	if tail > 0 {
		args = append(args, "-n", fmt.Sprintf("%d", tail))
	}

	if since != "" {
		args = append(args, "--since", since)
	}

	cmd := exec.Command("journalctl", args...)
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get logs: %w", err)
	}

	var logs []LogLine
	lines := strings.Split(string(output), "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Parse JSON log entry
		var entry journalEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			// If JSON parsing fails, use raw line
			logs = append(logs, LogLine{
				Message: line,
				Time:    time.Now().Format("2006-01-02 15:04:05"),
			})
			continue
		}

		// Convert timestamp (microseconds since epoch)
		logTime := ""
		if entry.RealtimeTimestamp != "" {
			var usec int64
			fmt.Sscanf(entry.RealtimeTimestamp, "%d", &usec)
			t := time.Unix(usec/1000000, (usec%1000000)*1000)
			logTime = t.Format("2006-01-02 15:04:05")
		}

		// Map priority to label
		priority := "info"
		switch entry.Priority {
		case "0":
			priority = "emerg"
		case "1":
			priority = "alert"
		case "2":
			priority = "crit"
		case "3":
			priority = "err"
		case "4":
			priority = "warn"
		case "5":
			priority = "notice"
		case "6":
			priority = "info"
		case "7":
			priority = "debug"
		}

		logs = append(logs, LogLine{
			Time:     logTime,
			Message:  entry.Message,
			Priority: priority,
		})
	}

	return logs, nil
}

// isEnabled checks if a service is enabled
func (m *ServiceManager) isEnabled(name string) bool {
	cmd := exec.Command("systemctl", "is-enabled", name+".service")
	output, err := cmd.Output()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(output)) == "enabled"
}

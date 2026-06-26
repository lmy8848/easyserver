package systemd

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"easyserver/internal/executor"
)

// ServiceInfo represents a systemd service.
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

// LogLine represents a log line from journalctl.
type LogLine struct {
	Time     string `json:"time"`
	Message  string `json:"message"`
	Priority string `json:"priority"`
}

// journalEntry represents a journalctl JSON output line.
type journalEntry struct {
	Message           string `json:"MESSAGE"`
	RealtimeTimestamp string `json:"__REALTIME_TIMESTAMP"`
	Priority          string `json:"PRIORITY"`
	SyslogIdentifier  string `json:"SYSLOG_IDENTIFIER"`
	Transport         string `json:"_TRANSPORT"`
}

// ServiceManager manages systemd services.
type ServiceManager struct {
	mu       sync.RWMutex
	executor executor.CommandExecutor
}

// NewServiceManager creates a new ServiceManager.
func NewServiceManager(exec executor.CommandExecutor) *ServiceManager {
	return &ServiceManager{executor: exec}
}

// List returns all systemd services.
func (m *ServiceManager) List(ctx context.Context) ([]ServiceInfo, error) {
	stdout, _, exitCode, err := m.executor.Run(ctx, "systemctl", "list-units", "--type=service", "--all", "--no-pager", "--plain", "--full")
	if err != nil || exitCode != 0 {
		return nil, fmt.Errorf("failed to list services: %w", err)
	}

	enabledMap := m.getAllEnabledStatus(ctx)

	var services []ServiceInfo
	lines := strings.Split(stdout, "\n")

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

		if len(fields) > 4 {
			svc.Description = strings.Join(fields[4:], " ")
		}

		svc.Enabled = enabledMap[svc.Name]
		services = append(services, svc)
	}

	m.batchGetDetailedInfo(ctx, services)

	return services, nil
}

// getAllEnabledStatus gets enabled status for all services in one call.
func (m *ServiceManager) getAllEnabledStatus(ctx context.Context) map[string]bool {
	stdout, _, exitCode, err := m.executor.Run(ctx, "systemctl", "list-unit-files", "--type=service", "--no-pager", "--plain")
	if err != nil || exitCode != 0 {
		return nil
	}

	result := make(map[string]bool)
	lines := strings.Split(stdout, "\n")
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

// batchGetDetailedInfo gets PID and memory for multiple services efficiently.
func (m *ServiceManager) batchGetDetailedInfo(ctx context.Context, services []ServiceInfo) {
	if len(services) == 0 {
		return
	}

	args := []string{"show"}
	for _, svc := range services {
		args = append(args, svc.Name+".service")
	}
	args = append(args, "--property=Id,MainPID,MemoryCurrent,ActiveState")

	stdout, _, exitCode, err := m.executor.Run(ctx, "systemctl", args...)
	if err != nil || exitCode != 0 {
		return
	}

	currentName := ""
	props := make(map[string]string)

	lines := strings.Split(stdout, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
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

// Get returns info for a specific service.
func (m *ServiceManager) Get(ctx context.Context, name string) (*ServiceInfo, error) {
	stdout, _, exitCode, err := m.executor.Run(ctx, "systemctl", "show", name+".service",
		"--property=ActiveState,SubState,MainPID,MemoryCurrent,Description")
	if err != nil || exitCode != 0 {
		return nil, fmt.Errorf("failed to get service info: %w", err)
	}

	svc := &ServiceInfo{
		Name: name,
	}

	lines := strings.Split(stdout, "\n")
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

	svc.Enabled = m.isEnabled(ctx, name)

	return svc, nil
}

// Start starts a service.
func (m *ServiceManager) Start(ctx context.Context, name string) error {
	if err := m.requireServiceExists(ctx, name); err != nil {
		return err
	}

	info, err := m.Get(ctx, name)
	if err != nil {
		return fmt.Errorf("failed to get service state: %w", err)
	}
	if info.State == "active" {
		return fmt.Errorf("service %s is already running", name)
	}

	output, exitCode, err := m.executor.RunCombined(ctx, "systemctl", "start", name+".service")
	if err != nil || exitCode != 0 {
		return fmt.Errorf("failed to start service: %s", output)
	}
	log.Printf("service: started %s", name)
	return nil
}

// Stop stops a service.
func (m *ServiceManager) Stop(ctx context.Context, name string) error {
	if err := m.requireServiceExists(ctx, name); err != nil {
		return err
	}

	info, err := m.Get(ctx, name)
	if err != nil {
		return fmt.Errorf("failed to get service state: %w", err)
	}
	if info.State == "inactive" || info.State == "failed" {
		return fmt.Errorf("service %s is already stopped", name)
	}

	output, exitCode, err := m.executor.RunCombined(ctx, "systemctl", "stop", name+".service")
	if err != nil || exitCode != 0 {
		return fmt.Errorf("failed to stop service: %s", output)
	}
	log.Printf("service: stopped %s", name)
	return nil
}

// Restart restarts a service.
func (m *ServiceManager) Restart(ctx context.Context, name string) error {
	if err := m.requireServiceExists(ctx, name); err != nil {
		return err
	}

	output, exitCode, err := m.executor.RunCombined(ctx, "systemctl", "restart", name+".service")
	if err != nil || exitCode != 0 {
		return fmt.Errorf("failed to restart service: %s", output)
	}
	log.Printf("service: restarted %s", name)
	return nil
}

// Enable enables a service for auto-start.
func (m *ServiceManager) Enable(ctx context.Context, name string) error {
	if err := m.requireServiceExists(ctx, name); err != nil {
		return err
	}

	if m.isEnabled(ctx, name) {
		return fmt.Errorf("service %s is already enabled", name)
	}

	output, exitCode, err := m.executor.RunCombined(ctx, "systemctl", "enable", name+".service")
	if err != nil || exitCode != 0 {
		return fmt.Errorf("failed to enable service: %s", output)
	}
	log.Printf("service: enabled %s", name)
	return nil
}

// Disable disables a service from auto-start.
func (m *ServiceManager) Disable(ctx context.Context, name string) error {
	if err := m.requireServiceExists(ctx, name); err != nil {
		return err
	}

	if !m.isEnabled(ctx, name) {
		return fmt.Errorf("service %s is already disabled", name)
	}

	output, exitCode, err := m.executor.RunCombined(ctx, "systemctl", "disable", name+".service")
	if err != nil || exitCode != 0 {
		return fmt.Errorf("failed to disable service: %s", output)
	}
	log.Printf("service: disabled %s", name)
	return nil
}

// GetLogs returns recent logs for a service.
func (m *ServiceManager) GetLogs(ctx context.Context, name string, tail int, since string) ([]LogLine, error) {
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

	stdout, _, exitCode, err := m.executor.Run(ctx, "journalctl", args...)
	if err != nil || exitCode != 0 {
		return nil, fmt.Errorf("failed to get logs: %w", err)
	}

	var logs []LogLine
	lines := strings.Split(stdout, "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		var entry journalEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			logs = append(logs, LogLine{
				Message: line,
				Time:    time.Now().Format("2006-01-02 15:04:05"),
			})
			continue
		}

		logTime := ""
		if entry.RealtimeTimestamp != "" {
			var usec int64
			fmt.Sscanf(entry.RealtimeTimestamp, "%d", &usec)
			t := time.Unix(usec/1000000, (usec%1000000)*1000)
			logTime = t.Format("2006-01-02 15:04:05")
		}

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

// isEnabled checks if a service is enabled.
func (m *ServiceManager) isEnabled(ctx context.Context, name string) bool {
	stdout, _, exitCode, err := m.executor.Run(ctx, "systemctl", "is-enabled", name+".service")
	if err != nil || exitCode != 0 {
		return false
	}
	return strings.TrimSpace(stdout) == "enabled"
}

// serviceExists checks if a service unit exists on the system.
func (m *ServiceManager) serviceExists(ctx context.Context, name string) bool {
	_, exitCode, err := m.executor.RunCombined(ctx, "systemctl", "cat", name+".service")
	return err == nil && exitCode == 0
}

// requireServiceExists returns an error if the service does not exist.
func (m *ServiceManager) requireServiceExists(ctx context.Context, name string) error {
	if !m.serviceExists(ctx, name) {
		return fmt.Errorf("service %s does not exist", name)
	}
	return nil
}

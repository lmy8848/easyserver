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
)

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
			m.logEvent(context.Background(), "DISK_HIGH_USAGE",
				"Disk usage on "+mountPoint+" is "+usageStr+"%")
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
			m.logEvent(context.Background(), "MEMORY_HIGH_USAGE",
				"Memory usage is "+strconv.FormatFloat(percent, 'f', 1, 64)+"%")
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
				m.logEvent(context.Background(), "SERVICE_FAILED",
					"Service "+serviceName+" has failed")
			}
		}
	}
}

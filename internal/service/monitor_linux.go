//go:build linux
// +build linux

package service

import (
	"bufio"
	"fmt"
	"log"
	"math"
	"os"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"syscall"

	"easyserver/internal/model"
)

func (s *MonitorService) readCPU(p *model.MonitorPoint) {
	data, err := os.ReadFile("/proc/stat")
	if err != nil {
		log.Printf("monitor: failed to read /proc/stat: %v", err)
		return
	}

	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "cpu ") {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < 5 {
			return
		}

		var total uint64
		for i := 1; i < len(fields); i++ {
			v, _ := strconv.ParseUint(fields[i], 10, 64)
			total += v
		}

		idle, _ := strconv.ParseUint(fields[4], 10, 64)

		if s.prevTotal > 0 {
			diffTotal := total - s.prevTotal
			diffIdle := idle - s.prevIdle
			if diffTotal > 0 {
				p.CPUPercent = math.Round((1-float64(diffIdle)/float64(diffTotal))*100*100) / 100
			}
		}

		s.prevIdle = idle
		s.prevTotal = total
		return
	}
}

func (s *MonitorService) readLoad(p *model.MonitorPoint) {
	data, err := os.ReadFile("/proc/loadavg")
	if err != nil {
		return
	}

	fields := strings.Fields(string(data))
	if len(fields) < 3 {
		return
	}

	p.CPULoad1m, _ = strconv.ParseFloat(fields[0], 64)
	p.CPULoad5m, _ = strconv.ParseFloat(fields[1], 64)
	p.CPULoad15m, _ = strconv.ParseFloat(fields[2], 64)
}

func (s *MonitorService) readMemory(p *model.MonitorPoint) {
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		log.Printf("monitor: failed to read /proc/meminfo: %v", err)
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
		val, _ := strconv.ParseUint(valStr, 10, 64)
		mem[key] = val * 1024
	}

	p.MemTotal = mem["MemTotal"]
	p.MemAvailable = mem["MemAvailable"]
	if p.MemTotal > 0 {
		p.MemUsed = p.MemTotal - mem["MemFree"] - mem["Buffers"] - mem["Cached"]
		p.MemPercent = math.Round(float64(p.MemUsed)/float64(p.MemTotal)*100*100) / 100
	}
}

func (s *MonitorService) readDisk(p *model.MonitorPoint) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs("/", &stat); err != nil {
		return
	}

	p.DiskTotal = stat.Blocks * uint64(stat.Bsize)
	p.DiskFree = stat.Bfree * uint64(stat.Bsize)
	p.DiskUsed = p.DiskTotal - p.DiskFree
	if p.DiskTotal > 0 {
		p.DiskPercent = math.Round(float64(p.DiskUsed)/float64(p.DiskTotal)*100*100) / 100
	}
}

func (s *MonitorService) readNetwork(p *model.MonitorPoint) {
	data, err := os.ReadFile("/proc/net/dev")
	if err != nil {
		log.Printf("monitor: failed to read /proc/net/dev: %v", err)
		return
	}

	var totalSent, totalRecv, totalPktsSent, totalPktsRecv uint64
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.Contains(line, ":") {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		iface := strings.TrimSpace(parts[0])
		if strings.HasPrefix(iface, "lo") {
			continue
		}

		fields := strings.Fields(parts[1])
		if len(fields) < 10 {
			continue
		}

		recvBytes, _ := strconv.ParseUint(fields[0], 10, 64)
		recvPkts, _ := strconv.ParseUint(fields[1], 10, 64)
		sentBytes, _ := strconv.ParseUint(fields[8], 10, 64)
		sentPkts, _ := strconv.ParseUint(fields[9], 10, 64)

		totalSent += sentBytes
		totalRecv += recvBytes
		totalPktsSent += sentPkts
		totalPktsRecv += recvPkts
	}

	if s.prevSent > 0 {
		p.NetBytesSent = totalSent - s.prevSent
		p.NetBytesRecv = totalRecv - s.prevRecv
		p.NetPktsSent = totalPktsSent - s.prevPktsSent
		p.NetPktsRecv = totalPktsRecv - s.prevPktsRecv
	}

	s.prevSent = totalSent
	s.prevRecv = totalRecv
	s.prevPktsSent = totalPktsSent
	s.prevPktsRecv = totalPktsRecv
}

func (s *MonitorService) readSystemInfo() *model.SystemInfo {
	info := &model.SystemInfo{
		Arch:     runtime.GOARCH,
		CPUCores: runtime.NumCPU(),
	}

	// Hostname
	if data, err := os.ReadFile("/proc/sys/kernel/hostname"); err == nil {
		info.Hostname = strings.TrimSpace(string(data))
	}

	// Kernel version
	if data, err := os.ReadFile("/proc/sys/kernel/osrelease"); err == nil {
		info.Kernel = strings.TrimSpace(string(data))
	}

	// OS name from /etc/os-release
	if data, err := os.ReadFile("/etc/os-release"); err == nil {
		scanner := bufio.NewScanner(strings.NewReader(string(data)))
		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, "PRETTY_NAME=") {
				val := strings.TrimPrefix(line, "PRETTY_NAME=")
				val = strings.Trim(val, "\"")
				info.OS = val
				break
			}
		}
	}
	if info.OS == "" {
		info.OS = "Linux"
	}

	// Uptime from /proc/uptime
	if data, err := os.ReadFile("/proc/uptime"); err == nil {
		fields := strings.Fields(string(data))
		if len(fields) >= 1 {
			if uptime, err := strconv.ParseFloat(fields[0], 64); err == nil {
				info.UptimeSeconds = int64(uptime)
			}
		}
	}

	return info
}

// readSwap reads swap memory info from /proc/meminfo
func (s *MonitorService) readSwap() *model.SwapInfo {
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return nil
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
		val, _ := strconv.ParseUint(valStr, 10, 64)
		mem[key] = val * 1024
	}

	swap := &model.SwapInfo{
		TotalBytes: mem["SwapTotal"],
		FreeBytes:  mem["SwapFree"],
	}
	swap.UsedBytes = swap.TotalBytes - swap.FreeBytes
	if swap.TotalBytes > 0 {
		swap.UsagePercent = math.Round(float64(swap.UsedBytes)/float64(swap.TotalBytes)*100*100) / 100
	}

	return swap
}

// readPartitions reads all disk partitions from /proc/mounts
func (s *MonitorService) readPartitions() []model.DiskPartition {
	data, err := os.ReadFile("/proc/mounts")
	if err != nil {
		return nil
	}

	var partitions []model.DiskPartition
	seen := make(map[string]bool)

	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		line := scanner.Text()
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}

		device := fields[0]
		mountPoint := fields[1]
		fsType := fields[2]

		// Skip virtual/special filesystems
		if strings.HasPrefix(device, "/dev/") == false {
			continue
		}
		// Skip duplicates
		if seen[mountPoint] {
			continue
		}
		seen[mountPoint] = true

		var stat syscall.Statfs_t
		if err := syscall.Statfs(mountPoint, &stat); err != nil {
			continue
		}

		total := stat.Blocks * uint64(stat.Bsize)
		free := stat.Bfree * uint64(stat.Bsize)
		used := total - free
		var percent float64
		if total > 0 {
			percent = math.Round(float64(used)/float64(total)*100*100) / 100
		}

		partitions = append(partitions, model.DiskPartition{
			MountPoint:   mountPoint,
			Device:       device,
			FSType:       fsType,
			TotalBytes:   total,
			UsedBytes:    used,
			FreeBytes:    free,
			UsagePercent: percent,
		})
	}

	return partitions
}

// readTopProcesses reads top 8 processes by CPU+Memory usage
func (s *MonitorService) readTopProcesses() []model.ProcessInfo {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil
	}

	var processes []model.ProcessInfo

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		pid, err := strconv.Atoi(entry.Name())
		if err != nil {
			continue
		}

		// Read /proc/[pid]/stat
		statData, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
		if err != nil {
			continue
		}

		statFields := strings.Fields(string(statData))
		if len(statFields) < 24 {
			continue
		}

		// Parse process name (field 1, in parentheses)
		name := statFields[1]
		name = strings.TrimPrefix(name, "(")
		name = strings.TrimSuffix(name, ")")

		// State (field 2)
		state := statFields[2]

		// User time (field 13) and system time (field 14) in clock ticks
		utime, _ := strconv.ParseUint(statFields[13], 10, 64)
		stime, _ := strconv.ParseUint(statFields[14], 10, 64)
		totalTime := utime + stime

		// RSS (field 23) in pages
		rssPages, _ := strconv.ParseUint(statFields[23], 10, 64)
		pageSize := uint64(os.Getpagesize())
		memBytes := rssPages * pageSize

		// Read /proc/[pid]/status for user
		user := "root"
		statusData, err := os.ReadFile(fmt.Sprintf("/proc/%d/status", pid))
		if err == nil {
			statusScanner := bufio.NewScanner(strings.NewReader(string(statusData)))
			for statusScanner.Scan() {
				line := statusScanner.Text()
				if strings.HasPrefix(line, "Uid:") {
					uidFields := strings.Fields(line)
					if len(uidFields) >= 2 {
						uid := uidFields[1]
						// Try to resolve username
						if passwdData, err := os.ReadFile("/etc/passwd"); err == nil {
							passScanner := bufio.NewScanner(strings.NewReader(string(passwdData)))
							for passScanner.Scan() {
								passLine := passScanner.Text()
								passFields := strings.Split(passLine, ":")
								if len(passFields) >= 3 && passFields[2] == uid {
									user = passFields[0]
									break
								}
							}
						}
					}
					break
				}
			}
		}

		// Calculate CPU percent (rough estimate based on total time)
		// This is a simplified version - for accurate %, we'd need delta tracking per PID
		cpuPercent := float64(totalTime) / 100.0 // Simplified

		// Get total memory for percentage
		var memPercent float64
		if memData, err := os.ReadFile("/proc/meminfo"); err == nil {
			memScanner := bufio.NewScanner(strings.NewReader(string(memData)))
			for memScanner.Scan() {
				line := memScanner.Text()
				if strings.HasPrefix(line, "MemTotal:") {
					fields := strings.Fields(line)
					if len(fields) >= 2 {
						val, _ := strconv.ParseUint(fields[1], 10, 64)
						totalMem := val * 1024
						if totalMem > 0 {
							memPercent = math.Round(float64(memBytes)/float64(totalMem)*100*100) / 100
						}
					}
					break
				}
			}
		}

		processes = append(processes, model.ProcessInfo{
			PID:        pid,
			Name:       name,
			User:       user,
			CPUPercent: cpuPercent,
			MemPercent: memPercent,
			MemBytes:   memBytes,
			State:      state,
		})
	}

	// Sort by memory usage (most used first), take top 8
	sort.Slice(processes, func(i, j int) bool {
		return processes[i].MemBytes > processes[j].MemBytes
	})

	if len(processes) > 8 {
		processes = processes[:8]
	}

	return processes
}

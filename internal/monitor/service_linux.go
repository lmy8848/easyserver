//go:build linux
// +build linux

package monitor

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
)

func (s *MonitorService) readCPU(p *MonitorPoint) {
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

func (s *MonitorService) readLoad(p *MonitorPoint) {
	data, err := os.ReadFile("/proc/loadavg")
	if err != nil {
		log.Printf("monitor: failed to read /proc/loadavg: %v", err)
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

func (s *MonitorService) readMemory(p *MonitorPoint) {
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
		// 同样的 uint64 回绕风险：极端情况下 buffers/cached 报告异常会让减法回绕，
		// 写库时被 database/sql 以"高位为 1"拒收。
		used := mem["MemFree"] + mem["Buffers"] + mem["Cached"]
		p.MemUsed = deltaU64(p.MemTotal, used)
		p.MemPercent = math.Round(float64(p.MemUsed)/float64(p.MemTotal)*100*100) / 100
	}
}

func (s *MonitorService) readDisk(p *MonitorPoint) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs("/", &stat); err != nil {
		log.Printf("monitor: failed to statfs /: %v", err)
		return
	}

	p.DiskTotal = stat.Blocks * uint64(stat.Bsize)
	p.DiskFree = stat.Bfree * uint64(stat.Bsize)
	p.DiskUsed = p.DiskTotal - p.DiskFree
	if p.DiskTotal > 0 {
		p.DiskPercent = math.Round(float64(p.DiskUsed)/float64(p.DiskTotal)*100*100) / 100
	}
}

func (s *MonitorService) readNetwork(p *MonitorPoint) {
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
		// 计数器可能因网卡重置/移除而回退；uint64 直减会回绕成天文数字（高位为 1），
		// database/sql 拒绝带高位的 uint64。当前值小于上次值时按 0 处理。
		p.NetBytesSent = deltaU64(totalSent, s.prevSent)
		p.NetBytesRecv = deltaU64(totalRecv, s.prevRecv)
		p.NetPktsSent = deltaU64(totalPktsSent, s.prevPktsSent)
		p.NetPktsRecv = deltaU64(totalPktsRecv, s.prevPktsRecv)
	}

	s.prevSent = totalSent
	s.prevRecv = totalRecv
	s.prevPktsSent = totalPktsSent
	s.prevPktsRecv = totalPktsRecv
}

func deltaU64(cur, prev uint64) uint64 {
	if cur < prev {
		return 0
	}
	return cur - prev
}

func (s *MonitorService) readSystemInfo() *SystemInfo {
	info := &SystemInfo{
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
func (s *MonitorService) readSwap() *SwapInfo {
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

	swap := &SwapInfo{
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
func (s *MonitorService) readPartitions() []DiskPartition {
	data, err := os.ReadFile("/proc/mounts")
	if err != nil {
		return nil
	}

	var partitions []DiskPartition
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

		partitions = append(partitions, DiskPartition{
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
func (s *MonitorService) readTopProcesses() []ProcessInfo {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil
	}

	// 性能优化：预加载 uid 缓存（只读一次 /etc/passwd）
	s.ensureUIDCache()

	// 性能优化：只读一次 /proc/meminfo 获取总内存
	var totalMem uint64
	if memData, err := os.ReadFile("/proc/meminfo"); err == nil {
		scanner := bufio.NewScanner(strings.NewReader(string(memData)))
		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, "MemTotal:") {
				fields := strings.Fields(line)
				if len(fields) >= 2 {
					val, _ := strconv.ParseUint(fields[1], 10, 64)
					totalMem = val * 1024
				}
				break
			}
		}
	}

	var processes []ProcessInfo

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

		// 进程名(comm)在括号里，可能含空格(如 "npm start")。用最后一个 ')'
		// 定位 comm 结束，避免 strings.Fields 把名字拆开导致后续字段错位
		// （否则 RSS 会读到 vsize 等错误字段，内存显示成 TB 级）。
		statStr := string(statData)
		firstParen := strings.IndexByte(statStr, '(')
		lastParen := strings.LastIndexByte(statStr, ')')
		if firstParen < 0 || lastParen < 0 || lastParen <= firstParen {
			continue
		}
		name := statStr[firstParen+1 : lastParen]
		// rest 从 state 开始，rest[k] 对应原 stat 字段 k+3
		restFields := strings.Fields(statStr[lastParen+1:])
		if len(restFields) < 22 { // 至少到 rss(原 field 24 = rest[21])
			continue
		}

		// State (原 field 3 = rest[0])
		state := restFields[0]

		// User time (原 field 14 = rest[11]) and system time (原 field 15 = rest[12]) in clock ticks
		utime, _ := strconv.ParseUint(restFields[11], 10, 64)
		stime, _ := strconv.ParseUint(restFields[12], 10, 64)
		totalTime := utime + stime

		// RSS (原 field 24 = rest[21]) in pages
		rssPages, _ := strconv.ParseUint(restFields[21], 10, 64)
		pageSize := uint64(os.Getpagesize())
		memBytes := rssPages * pageSize

		// Read /proc/[pid]/status for user（使用缓存）
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
						if name, ok := s.uidCache[uid]; ok {
							user = name
						}
					}
					break
				}
			}
		}

		// Calculate CPU percent (rough estimate based on total time)
		cpuPercent := float64(totalTime) / 100.0 // Simplified

		// Get memory percentage（使用预加载的 totalMem）
		var memPercent float64
		if totalMem > 0 {
			memPercent = math.Round(float64(memBytes)/float64(totalMem)*100*100) / 100
		}

		processes = append(processes, ProcessInfo{
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

// ensureUIDCache 确保 uid→username 缓存已加载（只读一次 /etc/passwd）
func (s *MonitorService) ensureUIDCache() {
	s.processMu.RLock()
	if len(s.uidCache) > 0 {
		s.processMu.RUnlock()
		return
	}
	s.processMu.RUnlock()

	s.processMu.Lock()
	defer s.processMu.Unlock()
	// Double-check after acquiring write lock
	if len(s.uidCache) > 0 {
		return
	}
	passwdData, err := os.ReadFile("/etc/passwd")
	if err != nil {
		return
	}
	scanner := bufio.NewScanner(strings.NewReader(string(passwdData)))
	for scanner.Scan() {
		fields := strings.Split(scanner.Text(), ":")
		if len(fields) >= 3 {
			s.uidCache[fields[2]] = fields[0]
		}
	}
}

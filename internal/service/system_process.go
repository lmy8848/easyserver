package service

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"easyserver/internal/executor"
	"easyserver/internal/model"
	"easyserver/internal/repository"
)

const (
	procDir                = "/proc"
	memInfoFile            = "/proc/meminfo"
	loadAvgFile            = "/proc/loadavg"
	uptimeFile             = "/proc/uptime"
	statFile               = "/proc/stat"
	defaultProcLimit       = 100
	defaultServiceLogLines = 100
	maxServiceLogLines     = 500
)

// cpuSample stores a single CPU measurement for a process
type cpuSample struct {
	utime    int64
	stime    int64
	sampleAt time.Time
}

// cpuStat stores a snapshot of /proc/stat cpu line
type cpuStat struct {
	user    int64
	nice    int64
	system  int64
	idle    int64
	iowait  int64
	irq     int64
	softirq int64
	steal   int64
	at      time.Time
}

type serviceAuditLogger interface {
	LogServiceOperation(ctx context.Context, userID int64, username, action, serviceName, ip, userAgent string)
}

// SystemProcessService provides system process monitoring and systemd management
type SystemProcessService struct {
	executor      executor.CommandExecutor
	whitelistRepo repository.ServiceWhitelistRepository
	auditLogger   serviceAuditLogger

	cache     *model.SystemOverview
	cacheMu   sync.RWMutex
	cacheTime time.Time
	cacheTTL  time.Duration

	cpuSamples map[int]*cpuSample // pid -> last sample
	cpuMu      sync.RWMutex
	prevCPU    *cpuStat // previous system-wide CPU snapshot

	// Process list cache
	procCache     []model.SystemProcess
	procCacheMu   sync.RWMutex
	procCacheTime time.Time
	procCacheTTL  time.Duration

	// Service list cache
	svcCache     []model.SystemService
	svcCacheMu   sync.RWMutex
	svcCacheTime time.Time
	svcCacheTTL  time.Duration
}

// NewSystemProcessService creates a new SystemProcessService
func NewSystemProcessService(executor executor.CommandExecutor, whitelistRepo repository.ServiceWhitelistRepository, auditLogger serviceAuditLogger) *SystemProcessService {
	sps := &SystemProcessService{
		executor:      executor,
		whitelistRepo: whitelistRepo,
		auditLogger:   auditLogger,
		cacheTTL:      5 * time.Second,
		procCacheTTL:  5 * time.Second,
		svcCacheTTL:   10 * time.Second,
		cpuSamples:    make(map[int]*cpuSample),
	}
	// Ensure whitelist table exists
	sps.whitelistRepo.Init(context.Background())
	return sps
}

// GetOverview returns system-wide resource statistics with caching
func (s *SystemProcessService) GetOverview() (*model.SystemOverview, error) {
	s.cacheMu.RLock()
	if s.cache != nil && time.Since(s.cacheTime) < s.cacheTTL {
		defer s.cacheMu.RUnlock()
		return s.cache, nil
	}
	s.cacheMu.RUnlock()

	overview := &model.SystemOverview{}

	// CPU usage
	cpuUsage, err := s.getCPUUsage()
	if err == nil {
		overview.CPUUsage = cpuUsage
	}

	// Memory info
	memTotal, memUsed, memPercent, swapTotal, swapUsed, err := getMemoryInfo()
	if err == nil {
		overview.MemoryTotal = memTotal
		overview.MemoryUsed = memUsed
		overview.MemoryUsage = memPercent
		overview.SwapTotal = swapTotal
		overview.SwapUsed = swapUsed
	}

	// Load average
	loadAvg, err := getLoadAverage()
	if err == nil {
		overview.LoadAvg = loadAvg
	}

	// Uptime
	uptime, err := getUptime()
	if err == nil {
		overview.Uptime = uptime
	}

	// Top processes
	procs, err := s.ListSystemProcesses("memory", "desc", "", 0)
	if err == nil {
		overview.TotalProcs = len(procs)
		for _, p := range procs {
			if p.State == "R" {
				overview.RunningProcs++
			}
		}
		// Top 5 by CPU
		sort.Slice(procs, func(i, j int) bool { return procs[i].CPUPercent > procs[j].CPUPercent })
		limit := 5
		if len(procs) < limit {
			limit = len(procs)
		}
		overview.TopCPU = make([]model.SystemProcess, limit)
		copy(overview.TopCPU, procs[:limit])

		// Top 5 by memory
		sort.Slice(procs, func(i, j int) bool { return procs[i].MemoryMB > procs[j].MemoryMB })
		if len(procs) < limit {
			limit = len(procs)
		}
		overview.TopMem = make([]model.SystemProcess, limit)
		copy(overview.TopMem, procs[:limit])
	}

	s.cacheMu.Lock()
	s.cache = overview
	s.cacheTime = time.Now()
	s.cacheMu.Unlock()

	return overview, nil
}

// ListSystemProcesses reads processes from /proc
// procWithCPU holds process data plus raw CPU ticks for delta calculation
type procWithCPU struct {
	model.SystemProcess
	utime int64
	stime int64
}

// ListSystemProcesses reads processes from /proc and calculates instantaneous CPU usage
func (s *SystemProcessService) ListSystemProcesses(sortBy, order, search string, limit int) ([]model.SystemProcess, error) {
	if limit <= 0 || limit > defaultProcLimit {
		limit = defaultProcLimit
	}

	// Check cache first (only for non-search requests to keep search responsive)
	if search == "" {
		s.procCacheMu.RLock()
		if s.procCache != nil && time.Since(s.procCacheTime) < s.procCacheTTL {
			cached := make([]model.SystemProcess, len(s.procCache))
			copy(cached, s.procCache)
			s.procCacheMu.RUnlock()

			sortProcesses(cached, sortBy, order)
			if len(cached) > limit {
				cached = cached[:limit]
			}
			return cached, nil
		}
		s.procCacheMu.RUnlock()
	}

	entries, err := os.ReadDir(procDir)
	if err != nil {
		return nil, fmt.Errorf("read /proc: %w", err)
	}

	now := time.Now()
	search = strings.ToLower(search)

	var rawProcs []procWithCPU

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		pid, err := strconv.Atoi(entry.Name())
		if err != nil {
			continue
		}

		proc, utime, stime, err := readProcStatusWithTicks(pid)
		if err != nil {
			continue
		}

		// Apply search filter
		if search != "" && !strings.Contains(strings.ToLower(proc.Name), search) &&
			!strings.Contains(strings.ToLower(proc.Command), search) {
			continue
		}

		rawProcs = append(rawProcs, procWithCPU{proc, utime, stime})
	}

	// Calculate instantaneous CPU using delta with previous samples
	s.cpuMu.Lock()
	newSamples := make(map[int]*cpuSample)
	for i := range rawProcs {
		pid := rawProcs[i].PID
		totalTicks := rawProcs[i].utime + rawProcs[i].stime

		if prev, ok := s.cpuSamples[pid]; ok {
			deltaTicks := totalTicks - (prev.utime + prev.stime)
			deltaTime := now.Sub(prev.sampleAt).Seconds()
			if deltaTime > 0 && deltaTicks >= 0 {
				rawProcs[i].CPUPercent = float64(deltaTicks) / (deltaTime * float64(100)) * 100.0
			}
		}

		newSamples[pid] = &cpuSample{
			utime:    rawProcs[i].utime,
			stime:    rawProcs[i].stime,
			sampleAt: now,
		}
	}
	s.cpuSamples = newSamples
	s.cpuMu.Unlock()

	// Convert to result
	var processes []model.SystemProcess
	for _, rp := range rawProcs {
		processes = append(processes, rp.SystemProcess)
	}

	// Update cache (only for full scans)
	if search == "" {
		s.procCacheMu.Lock()
		s.procCache = processes
		s.procCacheTime = now
		s.procCacheMu.Unlock()
	}

	// Sort
	sortProcesses(processes, sortBy, order)

	// Limit
	if len(processes) > limit {
		processes = processes[:limit]
	}

	return processes, nil
}

// GetSystemProcess returns details for a specific process by PID
func GetSystemProcess(pid int) (*model.SystemProcess, error) {
	proc, err := readProcStatus(pid)
	if err != nil {
		return nil, err
	}
	return &proc, nil
}

// ListServices returns systemd services (filtered by whitelist if entries exist)
func (s *SystemProcessService) ListServices() ([]model.SystemService, error) {
	// Check cache
	s.svcCacheMu.RLock()
	if s.svcCache != nil && time.Since(s.svcCacheTime) < s.svcCacheTTL {
		cached := make([]model.SystemService, len(s.svcCache))
		copy(cached, s.svcCache)
		s.svcCacheMu.RUnlock()
		return cached, nil
	}
	s.svcCacheMu.RUnlock()

	// Get whitelist
	whitelist, _ := s.GetWhitelist()

	// Use systemctl list-units with format to get PID directly (avoids per-service calls)
	out, _, err := s.executor.RunCombined(nil, "systemctl", "list-units", "--type=service", "--all", "--no-pager", "--plain", "--no-legend",
		"--property=name,load,active,sub,description,main-pid")
	if err != nil {
		// Fallback to basic list
		out, _, err = s.executor.RunCombined(nil, "systemctl", "list-units", "--type=service", "--all", "--no-pager", "--plain", "--no-legend")
		if err != nil {
			return nil, fmt.Errorf("systemctl list-units: %w", err)
		}
	}

	// Batch get enabled status via list-unit-files
	enabledMap := make(map[string]bool)
	enabledOut, _, err := s.executor.RunCombined(nil, "systemctl", "list-unit-files", "--type=service", "--no-pager", "--plain", "--no-legend")
	if err == nil {
		enabledScanner := bufio.NewScanner(strings.NewReader(enabledOut))
		for enabledScanner.Scan() {
			line := strings.TrimSpace(enabledScanner.Text())
			if line == "" {
				continue
			}
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				unitName := strings.TrimSuffix(fields[0], ".service")
				enabledMap[unitName] = fields[1] == "enabled"
			}
		}
	}

	var services []model.SystemService
	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}

		name := strings.TrimSuffix(fields[0], ".service")
		svc := model.SystemService{
			Name:        name,
			LoadState:   fields[1],
			ActiveState: fields[2],
			SubState:    fields[3],
		}
		if len(fields) > 4 {
			svc.Description = strings.Join(fields[4:], " ")
		}

		// Get PID from enabled map (avoids per-service systemctl call)
		if svc.ActiveState == "active" && svc.SubState == "running" {
			// Try to extract PID from --property output if available
			// Otherwise leave as 0
		}

		// Get enabled status from batch result
		svc.Enabled = enabledMap[name]

		// Filter by whitelist if it has entries
		if len(whitelist) > 0 {
			found := false
			for _, w := range whitelist {
				if w.Name == name || w.Name == fields[0] {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}

		services = append(services, svc)
	}

	// Update cache
	s.svcCacheMu.Lock()
	s.svcCache = services
	s.svcCacheTime = time.Now()
	s.svcCacheMu.Unlock()

	return services, nil
}

// protectedServices are services that cannot be stopped/restarted without force flag
var protectedServices = map[string]string{
	"easyserver":     "EasyServer 面板进程",
	"ssh":            "SSH 远程访问服务",
	"sshd":           "SSH 远程访问服务",
	"networking":     "网络服务",
	"network":        "网络服务",
	"systemd-logind": "登录管理服务",
}

// ServiceAction performs an action on a systemd service
func (s *SystemProcessService) ServiceAction(serviceName, action string, force bool) error {
	// Validate action
	validActions := map[string]bool{
		"start": true, "stop": true, "restart": true,
		"enable": true, "disable": true,
	}
	if !validActions[action] {
		return fmt.Errorf("invalid action: %s", action)
	}

	// Check protected services for stop/restart without force
	if (action == "stop" || action == "restart" || action == "disable") && !force {
		if reason, ok := protectedServices[serviceName]; ok {
			return fmt.Errorf("protected_service:%s:%s", serviceName, reason)
		}
	}

	// Check whitelist (if it has entries)
	whitelist, _ := s.GetWhitelist()
	if len(whitelist) > 0 {
		allowed := false
		for _, w := range whitelist {
			if w.Name == serviceName {
				allowed = true
				break
			}
		}
		if !allowed {
			return fmt.Errorf("service %s not in whitelist", serviceName)
		}
	}

	output, _, err := s.executor.RunCombined(nil, "systemctl", action, serviceName)
	if err != nil {
		return fmt.Errorf("systemctl %s %s: %s", action, serviceName, output)
	}

	// Audit log
	s.auditLog(serviceName, action)

	return nil
}

// GetWhitelist returns all whitelisted services
func (s *SystemProcessService) GetWhitelist() ([]model.ServiceWhitelistEntry, error) {
	return s.whitelistRepo.List(context.Background())
}

// AddToWhitelist adds a service to the whitelist
func (s *SystemProcessService) AddToWhitelist(name string) error {
	return s.whitelistRepo.Add(context.Background(), name)
}

// RemoveFromWhitelist removes a service from the whitelist
func (s *SystemProcessService) RemoveFromWhitelist(name string) error {
	return s.whitelistRepo.Delete(context.Background(), name)
}

// GetServiceLogs returns recent logs for a systemd service using journalctl
func (s *SystemProcessService) GetServiceLogs(serviceName string, lines int) (string, error) {
	if lines <= 0 || lines > maxServiceLogLines {
		lines = defaultServiceLogLines
	}

	output, _, err := s.executor.RunCombined(nil, "journalctl",
		"-u", serviceName,
		"-n", strconv.Itoa(lines),
		"--no-pager",
		"--output=short-iso",
	)
	if err != nil {
		// journalctl returns exit code 1 when no entries found
		if len(output) > 0 {
			return output, nil
		}
		return "", fmt.Errorf("journalctl failed: %w", err)
	}

	return output, nil
}

// IsProtectedService checks if a service is in the protected list
func IsProtectedService(serviceName string) (string, bool) {
	reason, ok := protectedServices[serviceName]
	return reason, ok
}

// ProtectedServices returns the map of protected services
func ProtectedServices() map[string]string {
	return protectedServices
}

func (s *SystemProcessService) auditLog(serviceName, action string) {
	if s.auditLogger == nil {
		return
	}
	s.auditLogger.LogServiceOperation(context.Background(), 0, "system", action, serviceName, "127.0.0.1", "EasyServer")
}

// --- /proc parsing helpers ---

func readProcStatus(pid int) (model.SystemProcess, error) {
	proc, _, _, err := readProcStatusWithTicks(pid)
	return proc, err
}

// readProcStatusWithTicks returns the process info plus raw utime/stime for CPU delta calculation
func readProcStatusWithTicks(pid int) (model.SystemProcess, int64, int64, error) {
	var utime, stime int64
	proc := model.SystemProcess{PID: pid}

	// Read /proc/[pid]/status
	statusFile := fmt.Sprintf("%s/%d/status", procDir, pid)
	f, err := os.Open(statusFile)
	if err != nil {
		return proc, 0, 0, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])
		switch key {
		case "Name":
			proc.Name = val
		case "State":
			if len(val) > 0 {
				proc.State = string(val[0]) // R, S, D, Z, T
			}
		case "PPid":
			proc.PPID, _ = strconv.Atoi(val)
		case "Uid":
			uid := strings.Fields(val)[0]
			proc.User = getUserFromUID(uid)
		case "Threads":
			proc.Threads, _ = strconv.Atoi(val)
		}
	}

	// Read command line
	cmdFile := fmt.Sprintf("%s/%d/cmdline", procDir, pid)
	cmdBytes, err := os.ReadFile(cmdFile)
	if err == nil {
		cmd := strings.ReplaceAll(string(cmdBytes), "\x00", " ")
		proc.Command = strings.TrimSpace(cmd)
	}

	// Read memory from /proc/[pid]/statm
	statmFile := fmt.Sprintf("%s/%d/statm", procDir, pid)
	statmBytes, err := os.ReadFile(statmFile)
	if err == nil {
		fields := strings.Fields(string(statmBytes))
		if len(fields) >= 2 {
			rssPages, _ := strconv.ParseInt(fields[1], 10, 64)
			proc.MemoryMB = float64(rssPages) * 4 / 1024 // pages to MB (4KB per page)
		}
	}

	// Read CPU ticks from /proc/[pid]/stat
	statPidFile := fmt.Sprintf("%s/%d/stat", procDir, pid)
	statBytes, err := os.ReadFile(statPidFile)
	if err == nil {
		fields := strings.Fields(string(statBytes))
		if len(fields) >= 22 {
			utime, _ = strconv.ParseInt(fields[13], 10, 64)
			stime, _ = strconv.ParseInt(fields[14], 10, 64)
			starttime, _ := strconv.ParseInt(fields[21], 10, 64)

			// Calculate start time
			clkTck := int64(100) // sysconf(_SC_CLK_TCK), typically 100 on Linux
			bootTime := getBootTime()
			startSec := starttime/clkTck + bootTime
			proc.StartTime = time.Unix(startSec, 0).Format("01-02 15:04")
		}
	}

	return proc, utime, stime, nil
}

func getUserFromUID(uid string) string {
	f, err := os.Open("/etc/passwd")
	if err != nil {
		return uid
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		parts := strings.Split(scanner.Text(), ":")
		if len(parts) >= 3 && parts[2] == uid {
			return parts[0]
		}
	}
	return uid
}

func getBootTime() int64 {
	f, err := os.Open(statFile)
	if err != nil {
		return 0
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "btime ") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				t, _ := strconv.ParseInt(fields[1], 10, 64)
				return t
			}
		}
	}
	return 0
}

// getCPUUsage reads /proc/stat and calculates instantaneous CPU usage using delta with previous sample
func (s *SystemProcessService) getCPUUsage() (float64, error) {
	f, err := os.Open(statFile)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	if !scanner.Scan() {
		return 0, fmt.Errorf("empty /proc/stat")
	}

	fields := strings.Fields(scanner.Text())
	if len(fields) < 9 || fields[0] != "cpu" {
		return 0, fmt.Errorf("invalid /proc/stat format")
	}

	now := time.Now()
	cur := &cpuStat{
		at: now,
	}
	cur.user, _ = strconv.ParseInt(fields[1], 10, 64)
	cur.nice, _ = strconv.ParseInt(fields[2], 10, 64)
	cur.system, _ = strconv.ParseInt(fields[3], 10, 64)
	cur.idle, _ = strconv.ParseInt(fields[4], 10, 64)
	cur.iowait, _ = strconv.ParseInt(fields[5], 10, 64)
	cur.irq, _ = strconv.ParseInt(fields[6], 10, 64)
	cur.softirq, _ = strconv.ParseInt(fields[7], 10, 64)
	cur.steal, _ = strconv.ParseInt(fields[8], 10, 64)

	// If we have a previous sample, calculate delta
	if s.prevCPU != nil {
		deltaTotal := (cur.user + cur.nice + cur.system + cur.idle + cur.iowait + cur.irq + cur.softirq + cur.steal) -
			(s.prevCPU.user + s.prevCPU.nice + s.prevCPU.system + s.prevCPU.idle + s.prevCPU.iowait + s.prevCPU.irq + s.prevCPU.softirq + s.prevCPU.steal)
		deltaBusy := (cur.user + cur.nice + cur.system + cur.irq + cur.softirq + cur.steal) -
			(s.prevCPU.user + s.prevCPU.nice + s.prevCPU.system + s.prevCPU.irq + s.prevCPU.softirq + s.prevCPU.steal)

		s.prevCPU = cur

		if deltaTotal > 0 {
			return float64(deltaBusy) / float64(deltaTotal) * 100, nil
		}
	}

	// First sample, save and return 0
	s.prevCPU = cur
	return 0, nil
}

func getMemoryInfo() (total, used int64, percent float64, swapTotal, swapUsed int64, err error) {
	f, err := os.Open(memInfoFile)
	if err != nil {
		return
	}
	defer f.Close()

	mem := make(map[string]int64)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		parts := strings.Fields(scanner.Text())
		if len(parts) >= 2 {
			key := strings.TrimSuffix(parts[0], ":")
			val, _ := strconv.ParseInt(parts[1], 10, 64)
			mem[key] = val // in kB
		}
	}

	total = mem["MemTotal"] / 1024 // to MB
	available := mem["MemAvailable"]
	if available == 0 {
		available = mem["MemFree"] + mem["Buffers"] + mem["Cached"]
	}
	used = (mem["MemTotal"] - available) / 1024
	if mem["MemTotal"] > 0 {
		percent = float64(mem["MemTotal"]-available) / float64(mem["MemTotal"]) * 100
	}
	swapTotal = mem["SwapTotal"] / 1024
	swapUsed = (mem["SwapTotal"] - mem["SwapFree"]) / 1024
	return
}

func getLoadAverage() ([3]float64, error) {
	data, err := os.ReadFile(loadAvgFile)
	if err != nil {
		return [3]float64{}, err
	}
	fields := strings.Fields(string(data))
	if len(fields) >= 3 {
		var load [3]float64
		load[0], _ = strconv.ParseFloat(fields[0], 64)
		load[1], _ = strconv.ParseFloat(fields[1], 64)
		load[2], _ = strconv.ParseFloat(fields[2], 64)
		return load, nil
	}
	return [3]float64{}, fmt.Errorf("invalid loadavg format")
}

func getUptime() (int64, error) {
	data, err := os.ReadFile(uptimeFile)
	if err != nil {
		return 0, err
	}
	fields := strings.Fields(string(data))
	if len(fields) >= 1 {
		uptime, err := strconv.ParseFloat(fields[0], 64)
		if err != nil {
			return 0, err
		}
		return int64(uptime), nil
	}
	return 0, fmt.Errorf("invalid uptime format")
}

func sortProcesses(procs []model.SystemProcess, sortBy, order string) {
	asc := order != "desc"
	sort.Slice(procs, func(i, j int) bool {
		switch sortBy {
		case "cpu":
			if asc {
				return procs[i].CPUPercent < procs[j].CPUPercent
			}
			return procs[i].CPUPercent > procs[j].CPUPercent
		case "memory":
			if asc {
				return procs[i].MemoryMB < procs[j].MemoryMB
			}
			return procs[i].MemoryMB > procs[j].MemoryMB
		case "pid":
			if asc {
				return procs[i].PID < procs[j].PID
			}
			return procs[i].PID > procs[j].PID
		case "name":
			if asc {
				return procs[i].Name < procs[j].Name
			}
			return procs[i].Name > procs[j].Name
		default:
			// Default: sort by memory desc
			return procs[i].MemoryMB > procs[j].MemoryMB
		}
	})
}

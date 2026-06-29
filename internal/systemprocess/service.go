package systemprocess

import (
	"bufio"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	procDir          = "/proc"
	statFile         = "/proc/stat"
	defaultProcLimit = 100
)

// cpuSample stores a single CPU measurement for a process
type cpuSample struct {
	utime    int64
	stime    int64
	sampleAt time.Time
}

// Service provides system process monitoring.
type Service struct {
	cpuSamples map[int]*cpuSample // pid -> last sample
	cpuMu      sync.RWMutex

	// Process list cache
	procCache     []SystemProcess
	procCacheMu   sync.RWMutex
	procCacheTime time.Time
	procCacheTTL  time.Duration
}

// NewService creates a new systemprocess.Service.
func NewService() *Service {
	return &Service{
		procCacheTTL: 5 * time.Second,
		cpuSamples:   make(map[int]*cpuSample),
	}
}

// procWithCPU holds process data plus raw CPU ticks for delta calculation.
type procWithCPU struct {
	SystemProcess
	utime int64
	stime int64
}

// ListSystemProcesses reads processes from /proc and calculates instantaneous CPU usage.
func (s *Service) ListSystemProcesses(sortBy, order, search string, limit int) ([]SystemProcess, error) {
	if limit <= 0 || limit > defaultProcLimit {
		limit = defaultProcLimit
	}

	// Check cache first (only for non-search requests to keep search responsive)
	if search == "" {
		s.procCacheMu.RLock()
		if s.procCache != nil && time.Since(s.procCacheTime) < s.procCacheTTL {
			cached := make([]SystemProcess, len(s.procCache))
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
				if rawProcs[i].CPUPercent > 100.0 {
					rawProcs[i].CPUPercent = 100.0
				}
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
	processes := make([]SystemProcess, 0)
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

// GetSystemProcess returns details for a specific process by PID.
func GetSystemProcess(pid int) (*SystemProcess, error) {
	proc, err := readProcStatus(pid)
	if err != nil {
		return nil, err
	}
	return &proc, nil
}

// --- /proc parsing helpers ---

func readProcStatus(pid int) (SystemProcess, error) {
	proc, _, _, err := readProcStatusWithTicks(pid)
	return proc, err
}

// readProcStatusWithTicks returns the process info plus raw utime/stime for CPU delta calculation.
func readProcStatusWithTicks(pid int) (SystemProcess, int64, int64, error) {
	var utime, stime int64
	proc := SystemProcess{PID: pid}

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

func sortProcesses(procs []SystemProcess, sortBy, order string) {
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

package process

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"sync"
	"syscall"
	"time"

	"easyserver/internal/infra"
	"easyserver/internal/infra/executor"
	"easyserver/internal/runtimeenv"
)

// --- Constants ---
const (
	defaultStopTimeoutSec  = 10  // seconds, SIGTERM wait before SIGKILL
	maxBackoffSec          = 300 // 5 minutes, cap for exponential backoff
	defaultRestartDelaySec = 5   // seconds, base delay for restart
	defaultMaxRestarts     = 10
	defaultStartupTimeout  = 30 // seconds
)

// managedProcess tracks a running process instance
type managedProcess struct {
	ID        int64
	proc      executor.Process
	cancel    context.CancelFunc
	startedAt time.Time
	mu        sync.Mutex
}

// Service is the core service for managing background processes
type Service struct {
	repo     Repository
	executor executor.CommandExecutor

	processes map[int64]*managedProcess
	mu        sync.RWMutex
	stopCh    chan struct{}
}

// NewService creates a new Service.
// It auto-starts processes that have auto_start=1.
func NewService(repo Repository, exec executor.CommandExecutor) *Service {
	s := &Service{
		repo:     repo,
		executor: exec,

		processes: make(map[int64]*managedProcess),
		stopCh:    make(chan struct{}),
	}
	// Auto-start enabled processes
	go s.autoStartAll()
	return s
}

// Shutdown stops all managed processes gracefully
func (s *Service) Shutdown() {
	close(s.stopCh)
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, mp := range s.processes {
		s.stopProcess(mp, defaultStopTimeoutSec)
	}
}

// --- CRUD operations ---

// List returns all process configurations with their runtime status
func (s *Service) List(ctx context.Context) ([]ProcessWithStatus, error) {
	processes, err := s.repo.ListProcesses(ctx)
	if err != nil {
		return nil, err
	}

	result := make([]ProcessWithStatus, 0)
	for _, p := range processes {
		pws := ProcessWithStatus{Process: p}
		pws.Status = s.getStatus(ctx, p.ID)
		pws.Group = s.getGroup(ctx, p.GroupID)
		result = append(result, pws)
	}
	return result, nil
}

// Get returns a single process by ID
func (s *Service) Get(ctx context.Context, id int64) (*ProcessWithStatus, error) {
	p, err := s.repo.GetProcessByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get process: %w", err)
	}
	if p == nil {
		return nil, nil
	}

	return &ProcessWithStatus{
		Process: *p,
		Status:  s.getStatus(ctx, p.ID),
		Group:   s.getGroup(ctx, p.GroupID),
	}, nil
}

// Create adds a new process configuration
func (s *Service) Create(ctx context.Context, req *CreateProcessRequest) (*Process, error) {
	// When RuntimeVersionID is 0 (website-linked or system-level process),
	// skip the runtime status check — the command runs on $PATH directly.
	// Otherwise, refuse to bind to a runtime that's not actually usable.
	if req.RuntimeVersionID > 0 {
		status, err := s.repo.GetRuntimeVersionStatus(ctx, req.RuntimeVersionID)
		if err != nil {
			return nil, fmt.Errorf("validate runtime_version: %w", err)
		}
		if status != "installed" {
			return nil, fmt.Errorf("runtime_version %d is %q, only 'installed' runtimes can be bound", req.RuntimeVersionID, status)
		}
	}

	autoRestart := true
	if req.AutoRestart != nil {
		autoRestart = *req.AutoRestart
	}
	autoStart := false
	if req.AutoStart != nil {
		autoStart = *req.AutoStart
	}
	maxRestarts := req.MaxRestarts
	if maxRestarts <= 0 {
		maxRestarts = defaultMaxRestarts
	}
	restartDelay := req.RestartDelay
	if restartDelay <= 0 {
		restartDelay = defaultRestartDelaySec
	}
	stopTimeout := req.StopTimeout
	if stopTimeout <= 0 {
		stopTimeout = defaultStopTimeoutSec
	}
	startupTimeout := req.StartupTimeout
	if startupTimeout <= 0 {
		startupTimeout = defaultStartupTimeout
	}

	p := &Process{
		Name:             req.Name,
		Command:          req.Command,
		Args:             req.Args,
		Dir:              req.Dir,
		Env:              req.Env,
		AutoRestart:      autoRestart,
		MaxRestarts:      maxRestarts,
		RestartDelay:     restartDelay,
		StopTimeout:      stopTimeout,
		StartupTimeout:   startupTimeout,
		AutoStart:        autoStart,
		LogFile:          req.LogFile,
		GroupID:          req.GroupID,
		RuntimeVersionID: req.RuntimeVersionID,
	}

	id, err := s.repo.CreateProcess(ctx, p)
	if err != nil {
		return nil, fmt.Errorf("create process: %w", err)
	}
	p.ID = id
	return p, nil
}

// Update modifies an existing process configuration
func (s *Service) Update(ctx context.Context, id int64, req *UpdateProcessRequest) error {
	// Check if process is running — don't allow changing critical fields while running
	s.mu.RLock()
	_, running := s.processes[id]
	s.mu.RUnlock()

	if running && req.Command != nil {
		return fmt.Errorf("cannot change command while process is running, stop it first")
	}
	if running && req.RuntimeVersionID != nil {
		// Runtime change only takes effect on next Start; refuse mid-flight to
		// avoid the subtle UX where the user thinks they switched node 18 → 20
		// but the running process keeps using 18 until restart.
		return fmt.Errorf("cannot change runtime_version_id while process is running, stop it first")
	}

	// Symmetry with Create: refuse to bind to a non-installed runtime.
	if req.RuntimeVersionID != nil {
		status, err := s.repo.GetRuntimeVersionStatus(ctx, *req.RuntimeVersionID)
		if err != nil {
			return fmt.Errorf("validate runtime_version: %w", err)
		}
		if status != "installed" {
			return fmt.Errorf("runtime_version %d is not installed (status=%s)", *req.RuntimeVersionID, status)
		}
	}

	return s.repo.UpdateProcess(ctx, id, req)
}

// Delete removes a process and its status/logs
func (s *Service) Delete(ctx context.Context, id int64) error {
	// Stop if running
	s.mu.RLock()
	mp, running := s.processes[id]
	s.mu.RUnlock()
	if running {
		s.stopProcess(mp, defaultStopTimeoutSec)
		s.mu.Lock()
		delete(s.processes, id)
		s.mu.Unlock()
	}

	if err := s.repo.DeleteProcess(ctx, id); err != nil {
		return fmt.Errorf("delete process: %w", err)
	}
	return nil
}

// --- Lifecycle operations ---

// Start launches a process
func (s *Service) Start(ctx context.Context, id int64) error {
	s.mu.Lock()
	if _, exists := s.processes[id]; exists {
		s.mu.Unlock()
		return fmt.Errorf("process %d is already running", id)
	}
	s.mu.Unlock()

	p, err := s.Get(ctx, id)
	if err != nil || p == nil {
		return fmt.Errorf("process %d not found", id)
	}

	// Update status to starting
	s.updateStatus(ctx, id, "starting", 0, 0, "")

	// Build options
	args := parseArgs(p.Args)
	opts := executor.StartOptions{
		Setpgid: true,
	}
	if p.Dir != "" {
		opts.WorkDir = p.Dir
	}
	if p.Env != "" && p.Env != "{}" {
		envMap := make(map[string]string)
		if err := json.Unmarshal([]byte(p.Env), &envMap); err == nil {
			for k, v := range envMap {
				opts.Env = append(opts.Env, fmt.Sprintf("%s=%s", k, v))
			}
		}
	}

	// When RuntimeVersionID is 0 (website-linked or system-level process),
	// run the command directly on $PATH without mise wrapping. Otherwise,
	// wrap in `mise exec <lang>@<exact> --` so binaries resolve via the
	// pinned runtime version (see ADR-0002 §4 and Issue 06).
	var execCmd string
	var execArgs []string
	if p.RuntimeVersionID > 0 {
		miseTool, ok := runtimeenv.MiseToolFor(p.RuntimeLang)
		if !ok {
			s.updateStatus(ctx, id, "failed", 0, 0, "unsupported runtime: "+p.RuntimeLang)
			return fmt.Errorf("unsupported runtime lang %q for process %d", p.RuntimeLang, id)
		}
		execCmd = "/usr/local/bin/mise"
		execArgs = append([]string{"exec", miseTool + "@" + p.RuntimeExact, "--", p.Command}, args...)
		opts.Env = append(opts.Env, "MISE_DATA_DIR=/var/lib/easyserver/mise")
	} else {
		execCmd = p.Command
		execArgs = args
	}

	// Start process - 用 context.Background() 而非请求 ctx，避免 API 返回后
	// 请求 context 取消导致进程被 exec.CommandContext Kill（表现为启动后立即 exit 1）
	proc, err := s.executor.Start(context.Background(), opts, execCmd, execArgs...)
	if err != nil {
		s.updateStatus(ctx, id, "failed", 0, 0, err.Error())
		return fmt.Errorf("failed to start process: %w", err)
	}

	// Set up stdout/stderr capture
	stdout, _ := proc.StdoutPipe()
	stderr, _ := proc.StderrPipe()

	mpCtx, cancel := context.WithCancel(context.Background())
	mp := &managedProcess{
		ID:        id,
		proc:      proc,
		cancel:    cancel,
		startedAt: time.Now(),
	}

	s.mu.Lock()
	s.processes[id] = mp
	s.mu.Unlock()

	// Update status to running
	s.updateStatus(ctx, id, "running", proc.Pid(), 0, "")
	s.addLog(ctx, id, "system", fmt.Sprintf("Process started (PID: %d)", proc.Pid()))

	// Start log capture goroutines
	go s.captureOutput(mpCtx, id, stdout, "stdout")
	go s.captureOutput(mpCtx, id, stderr, "stderr")

	// Wait for process to exit in background
	startupTimeout := p.StartupTimeout
	if startupTimeout <= 0 {
		startupTimeout = defaultStartupTimeout
	}
	go s.waitForExit(mpCtx, mp, p.AutoRestart, p.MaxRestarts, p.RestartDelay, startupTimeout)

	return nil
}

// Stop terminates a running process
func (s *Service) Stop(ctx context.Context, id int64) error {
	s.mu.RLock()
	mp, exists := s.processes[id]
	s.mu.RUnlock()

	if !exists {
		// Check if process exists in DB
		p, _ := s.Get(ctx, id)
		if p == nil {
			return fmt.Errorf("process %d not found", id)
		}
		return fmt.Errorf("process %d is not running", id)
	}

	// Get process config for stopTimeout
	p, _ := s.Get(ctx, id)
	stopTimeout := defaultStopTimeoutSec
	if p != nil && p.StopTimeout > 0 {
		stopTimeout = p.StopTimeout
	}

	s.updateStatus(ctx, id, "stopping", mp.proc.Pid(), 0, "")
	s.addLog(ctx, id, "system", "Stopping process...")

	s.stopProcess(mp, stopTimeout)

	s.mu.Lock()
	delete(s.processes, id)
	s.mu.Unlock()

	s.updateStatus(ctx, id, "stopped", 0, 0, "")
	s.addLog(ctx, id, "system", "Process stopped")
	return nil
}

// Restart stops and starts a process
func (s *Service) Restart(ctx context.Context, id int64) error {
	// Check if currently running
	s.mu.RLock()
	_, running := s.processes[id]
	s.mu.RUnlock()

	if running {
		if err := s.Stop(ctx, id); err != nil {
			return fmt.Errorf("stop during restart: %w", err)
		}
		// Brief pause before restart
		time.Sleep(500 * time.Millisecond)
	}
	return s.Start(ctx, id)
}

// GetStats returns runtime statistics for a process
func (s *Service) GetStats(ctx context.Context, id int64) (*ProcessStats, error) {
	status := s.getStatus(ctx, id)
	if status == nil {
		return nil, nil
	}

	stats := &ProcessStats{
		CPUPercent: status.CPUPercent,
		MemoryMB:   status.MemoryMB,
		PID:        status.PID,
		Uptime:     status.Uptime,
		Restarts:   status.Restarts,
	}

	// If running, compute uptime
	s.mu.RLock()
	mp, running := s.processes[id]
	s.mu.RUnlock()

	if running && mp.proc != nil {
		mp.mu.Lock()
		stats.Uptime = int64(time.Since(mp.startedAt).Seconds())
		mp.mu.Unlock()
	}

	return stats, nil
}

// GetLogs returns process log entries
func (s *Service) GetLogs(ctx context.Context, processID int64, limit, offset int) ([]ProcessLog, int, error) {
	if limit <= 0 {
		limit = 50
	}
	return s.repo.ListLogs(ctx, processID, limit, offset)
}

// --- Batch operations ---

// BatchStart starts multiple processes
func (s *Service) BatchStart(ctx context.Context, ids []int64) ([]int64, []int64, error) {
	var started, failed []int64
	for _, id := range ids {
		if err := s.Start(ctx, id); err != nil {
			failed = append(failed, id)
			log.Printf("process: batch start %d failed: %v", id, err)
		} else {
			started = append(started, id)
		}
	}
	return started, failed, nil
}

// BatchStop stops multiple processes
func (s *Service) BatchStop(ctx context.Context, ids []int64) ([]int64, []int64, error) {
	var stopped, failed []int64
	for _, id := range ids {
		if err := s.Stop(ctx, id); err != nil {
			failed = append(failed, id)
		} else {
			stopped = append(stopped, id)
		}
	}
	return stopped, failed, nil
}

// BatchRestart restarts multiple processes
func (s *Service) BatchRestart(ctx context.Context, ids []int64) ([]int64, []int64, error) {
	var restarted, failed []int64
	for _, id := range ids {
		if err := s.Restart(ctx, id); err != nil {
			failed = append(failed, id)
		} else {
			restarted = append(restarted, id)
		}
	}
	return restarted, failed, nil
}

// --- Group operations ---

// ListGroups returns all process groups
func (s *Service) ListGroups(ctx context.Context) ([]ProcessGroup, error) {
	return s.repo.ListGroups(ctx)
}

// GetGroup returns a process group by ID
func (s *Service) GetGroup(ctx context.Context, id int64) (*ProcessGroup, error) {
	return s.repo.GetGroup(ctx, id)
}

// CreateGroup creates a new process group
func (s *Service) CreateGroup(ctx context.Context, req *CreateProcessGroupRequest) (*ProcessGroup, error) {
	id, err := s.repo.CreateGroup(ctx, req.Name, req.Description)
	if err != nil {
		return nil, err
	}
	return &ProcessGroup{ID: id, Name: req.Name, Description: req.Description}, nil
}

// UpdateGroup updates a process group
func (s *Service) UpdateGroup(ctx context.Context, id int64, req *UpdateProcessGroupRequest) error {
	return s.repo.UpdateGroup(ctx, id, req)
}

// DeleteGroup deletes a process group
func (s *Service) DeleteGroup(ctx context.Context, id int64) error {
	return s.repo.DeleteGroup(ctx, id)
}

// --- Export/Import ---

// Export returns JSON representation of all process configs
func (s *Service) Export(ctx context.Context) ([]Process, error) {
	return s.repo.ListProcesses(ctx)
}

// Import imports process configurations from JSON
func (s *Service) Import(ctx context.Context, processes []Process) (int, error) {
	count := 0
	for _, p := range processes {
		req := &CreateProcessRequest{
			Name:             p.Name,
			Command:          p.Command,
			Args:             p.Args,
			Dir:              p.Dir,
			Env:              p.Env,
			AutoRestart:      &p.AutoRestart,
			MaxRestarts:      p.MaxRestarts,
			RestartDelay:     p.RestartDelay,
			AutoStart:        &p.AutoStart,
			LogFile:          p.LogFile,
			GroupID:          p.GroupID,
			RuntimeVersionID: p.RuntimeVersionID,
		}
		if _, err := s.Create(ctx, req); err != nil {
			log.Printf("process: import %s failed: %v", p.Name, err)
			continue
		}
		count++
	}
	return count, nil
}

// --- Internal helpers ---

func (s *Service) stopProcess(mp *managedProcess, stopTimeout int) {
	mp.cancel()
	if mp.proc != nil {
		// SIGTERM first for graceful shutdown
		mp.proc.Signal(syscall.SIGTERM)
		done := make(chan error, 1)
		infra.Go(func() {
			done <- mp.proc.Wait()
		})
		// Wait configured timeout, then SIGKILL
		if stopTimeout <= 0 {
			stopTimeout = defaultStopTimeoutSec
		}
		select {
		case <-done:
			// Process exited gracefully
		case <-time.After(time.Duration(stopTimeout) * time.Second):
			mp.proc.Kill()
			<-done // Wait for SIGKILL to take effect
		}
	}
}

func (s *Service) waitForExit(ctx context.Context, mp *managedProcess, autoRestart bool, maxRestarts, restartDelay, startupTimeout int) {
	startTime := time.Now()
	err := mp.proc.Wait()
	exitCode := 0
	if err != nil {
		// Process exited with error
		exitCode = 1
	}

	s.mu.Lock()
	delete(s.processes, mp.ID)
	s.mu.Unlock()

	select {
	case <-ctx.Done():
		// Context cancelled (manual stop)
		return
	default:
	}

	// Update status
	status := "stopped"
	errMsg := ""
	if exitCode != 0 {
		status = "error"
		errMsg = fmt.Sprintf("exit code %d", exitCode)
	}
	s.updateStatus(context.Background(), mp.ID, status, 0, exitCode, errMsg)

	// Check if should auto-restart with exponential backoff
	if autoRestart && exitCode != 0 {
		// Check if this was a startup failure (exited within startupTimeout)
		runtime := time.Since(startTime).Seconds()
		isStartupFailure := runtime < float64(startupTimeout)

		st := s.getStatus(context.Background(), mp.ID)
		if st != nil && st.Restarts < maxRestarts {
			// Exponential backoff: base * 2^restarts, capped at 5 minutes
			// Cap shift to prevent integer overflow
			shift := uint(st.Restarts)
			if shift > 10 {
				shift = 10 // 2^10 = 1024, safe for int
			}
			backoff := restartDelay * (1 << shift)
			maxBackoff := maxBackoffSec
			if backoff > maxBackoff {
				backoff = maxBackoff
			}

			// For startup failures, use longer backoff to prevent restart loops
			if isStartupFailure {
				backoff = backoff * 2
				s.addLog(context.Background(), mp.ID, "system",
					fmt.Sprintf("Startup failure detected (exited in %.0fs < %ds), auto-restarting in %ds (attempt %d/%d)",
						runtime, startupTimeout, backoff, st.Restarts+1, maxRestarts))
			} else {
				s.addLog(context.Background(), mp.ID, "system",
					fmt.Sprintf("Process exited with code %d, auto-restarting in %ds (attempt %d/%d)",
						exitCode, backoff, st.Restarts+1, maxRestarts))
			}

			time.Sleep(time.Duration(backoff) * time.Second)

			// Increment restart counter
			s.repo.IncrementRestarts(context.Background(), mp.ID)

			// Clear exit code and error before restart
			s.repo.ClearExitInfo(context.Background(), mp.ID)

			if err := s.Start(context.Background(), mp.ID); err != nil {
				s.addLog(context.Background(), mp.ID, "system",
					fmt.Sprintf("Auto-restart failed: %v", err))
				s.updateStatus(context.Background(), mp.ID, "error", 0, 0, err.Error())
			}
		} else {
			s.addLog(context.Background(), mp.ID, "system",
				fmt.Sprintf("Max restarts (%d) reached, giving up", maxRestarts))
		}
	}
}

func (s *Service) captureOutput(ctx context.Context, processID int64, r io.Reader, logType string) {
	if r == nil {
		return
	}
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024) // 1MB max line
	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return
		default:
		}
		s.addLog(ctx, processID, logType, scanner.Text())
	}
}

func (s *Service) updateStatus(ctx context.Context, processID int64, status string, pid int, exitCode int, lastError string) {
	s.repo.UpsertStatus(ctx, processID, status, pid, exitCode, lastError)
}

func (s *Service) addLog(ctx context.Context, processID int64, logType, content string) {
	s.repo.AppendLog(ctx, processID, logType, content)
}

func (s *Service) getStatus(ctx context.Context, processID int64) *ProcessStatus {
	st, _ := s.repo.GetStatus(ctx, processID)
	return st
}

func (s *Service) getGroup(ctx context.Context, groupID int64) *ProcessGroup {
	if groupID == 0 {
		return nil
	}
	g, _ := s.repo.GetGroup(ctx, groupID)
	return g
}

func (s *Service) autoStartAll() {
	// Wait briefly for DB to be ready
	time.Sleep(2 * time.Second)

	ctx := context.Background()
	ids, err := s.repo.GetAutoStartIDs(ctx)
	if err != nil {
		log.Printf("process: auto-start query failed: %v", err)
		return
	}

	for _, id := range ids {
		if err := s.Start(ctx, id); err != nil {
			log.Printf("process: auto-start %d failed: %v", id, err)
		}
	}
}

func parseArgs(args string) []string {
	if args == "" {
		return nil
	}
	// Simple space-split for now; could be enhanced with proper shell parsing
	var result []string
	current := ""
	inQuote := false
	for _, c := range args {
		switch c {
		case '"':
			inQuote = !inQuote
		case ' ':
			if inQuote {
				current += string(c)
			} else if current != "" {
				result = append(result, current)
				current = ""
			}
		default:
			current += string(c)
		}
	}
	if current != "" {
		result = append(result, current)
	}
	return result
}

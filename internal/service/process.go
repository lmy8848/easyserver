package service

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

	"easyserver/internal/executor"
	"easyserver/internal/model"
	"easyserver/internal/repository"
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

// ProcessManager is the core service for managing background processes
type ProcessManager struct {
	repo     repository.ProcessRepository
	executor executor.CommandExecutor

	processes map[int64]*managedProcess
	mu        sync.RWMutex
	stopCh    chan struct{}
}

// NewProcessManager creates a new ProcessManager.
// It auto-starts processes that have auto_start=1.
func NewProcessManager(repo repository.ProcessRepository, exec executor.CommandExecutor) *ProcessManager {
	pm := &ProcessManager{
		repo:     repo,
		executor: exec,

		processes: make(map[int64]*managedProcess),
		stopCh:    make(chan struct{}),
	}
	// Auto-start enabled processes
	go pm.autoStartAll()
	return pm
}

// Shutdown stops all managed processes gracefully
func (pm *ProcessManager) Shutdown() {
	close(pm.stopCh)
	pm.mu.Lock()
	defer pm.mu.Unlock()
	for _, mp := range pm.processes {
		pm.stopProcess(mp, defaultStopTimeoutSec) // default for shutdown
	}
}

// --- CRUD operations ---

// List returns all process configurations with their runtime status
func (pm *ProcessManager) List(ctx context.Context) ([]model.ProcessWithStatus, error) {
	processes, err := pm.repo.ListProcesses(ctx)
	if err != nil {
		return nil, err
	}

	var result []model.ProcessWithStatus
	for _, p := range processes {
		pws := model.ProcessWithStatus{Process: p}
		pws.Status = pm.getStatus(ctx, p.ID)
		pws.Group = pm.getGroup(ctx, p.GroupID)
		result = append(result, pws)
	}
	return result, nil
}

// Get returns a single process by ID
func (pm *ProcessManager) Get(ctx context.Context, id int64) (*model.ProcessWithStatus, error) {
	p, err := pm.repo.GetProcessByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get process: %w", err)
	}
	if p == nil {
		return nil, nil
	}

	return &model.ProcessWithStatus{
		Process: *p,
		Status:  pm.getStatus(ctx, p.ID),
		Group:   pm.getGroup(ctx, p.GroupID),
	}, nil
}

// Create adds a new process configuration
func (pm *ProcessManager) Create(ctx context.Context, req *model.CreateProcessRequest) (*model.Process, error) {
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

	p := &model.Process{
		Name:           req.Name,
		Command:        req.Command,
		Args:           req.Args,
		Dir:            req.Dir,
		Env:            req.Env,
		AutoRestart:    autoRestart,
		MaxRestarts:    maxRestarts,
		RestartDelay:   restartDelay,
		StopTimeout:    stopTimeout,
		StartupTimeout: startupTimeout,
		AutoStart:      autoStart,
		LogFile:        req.LogFile,
		GroupID:        req.GroupID,
	}

	id, err := pm.repo.CreateProcess(ctx, p)
	if err != nil {
		return nil, fmt.Errorf("create process: %w", err)
	}
	p.ID = id
	return p, nil
}

// Update modifies an existing process configuration
func (pm *ProcessManager) Update(ctx context.Context, id int64, req *model.UpdateProcessRequest) error {
	// Check if process is running — don't allow changing critical fields while running
	pm.mu.RLock()
	_, running := pm.processes[id]
	pm.mu.RUnlock()

	if running && req.Command != nil {
		return fmt.Errorf("cannot change command while process is running, stop it first")
	}

	return pm.repo.UpdateProcess(ctx, id, req)
}

// Delete removes a process and its status/logs
func (pm *ProcessManager) Delete(ctx context.Context, id int64) error {
	// Stop if running
	pm.mu.RLock()
	mp, running := pm.processes[id]
	pm.mu.RUnlock()
	if running {
		pm.stopProcess(mp, defaultStopTimeoutSec)
		pm.mu.Lock()
		delete(pm.processes, id)
		pm.mu.Unlock()
	}

	if err := pm.repo.DeleteProcess(ctx, id); err != nil {
		return fmt.Errorf("delete process: %w", err)
	}
	return nil
}

// --- Lifecycle operations ---

// Start launches a process
func (pm *ProcessManager) Start(ctx context.Context, id int64) error {
	pm.mu.Lock()
	if _, exists := pm.processes[id]; exists {
		pm.mu.Unlock()
		return fmt.Errorf("process %d is already running", id)
	}
	pm.mu.Unlock()

	p, err := pm.Get(ctx, id)
	if err != nil || p == nil {
		return fmt.Errorf("process %d not found", id)
	}

	// Update status to starting
	pm.updateStatus(ctx, id, "starting", 0, 0, "")

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

	// Start process
	proc, err := pm.executor.Start(ctx, opts, p.Command, args...)
	if err != nil {
		pm.updateStatus(ctx, id, "failed", 0, 0, err.Error())
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

	pm.mu.Lock()
	pm.processes[id] = mp
	pm.mu.Unlock()

	// Update status to running
	pm.updateStatus(ctx, id, "running", proc.Pid(), 0, "")
	pm.addLog(ctx, id, "system", fmt.Sprintf("Process started (PID: %d)", proc.Pid()))

	// Start log capture goroutines
	go pm.captureOutput(mpCtx, id, stdout, "stdout")
	go pm.captureOutput(mpCtx, id, stderr, "stderr")

	// Wait for process to exit in background
	startupTimeout := p.StartupTimeout
	if startupTimeout <= 0 {
		startupTimeout = defaultStartupTimeout
	}
	go pm.waitForExit(mpCtx, mp, p.AutoRestart, p.MaxRestarts, p.RestartDelay, startupTimeout)

	return nil
}

// Stop terminates a running process
func (pm *ProcessManager) Stop(ctx context.Context, id int64) error {
	pm.mu.RLock()
	mp, exists := pm.processes[id]
	pm.mu.RUnlock()

	if !exists {
		return fmt.Errorf("process %d is not running", id)
	}

	// Get process config for stopTimeout
	p, _ := pm.Get(ctx, id)
	stopTimeout := defaultStopTimeoutSec
	if p != nil && p.StopTimeout > 0 {
		stopTimeout = p.StopTimeout
	}

	pm.updateStatus(ctx, id, "stopping", mp.proc.Pid(), 0, "")
	pm.addLog(ctx, id, "system", "Stopping process...")

	pm.stopProcess(mp, stopTimeout)

	pm.mu.Lock()
	delete(pm.processes, id)
	pm.mu.Unlock()

	pm.updateStatus(ctx, id, "stopped", 0, 0, "")
	pm.addLog(ctx, id, "system", "Process stopped")
	return nil
}

// Restart stops and starts a process
func (pm *ProcessManager) Restart(ctx context.Context, id int64) error {
	// Check if currently running
	pm.mu.RLock()
	_, running := pm.processes[id]
	pm.mu.RUnlock()

	if running {
		if err := pm.Stop(ctx, id); err != nil {
			return fmt.Errorf("stop during restart: %w", err)
		}
		// Brief pause before restart
		time.Sleep(500 * time.Millisecond)
	}
	return pm.Start(ctx, id)
}

// GetStats returns runtime statistics for a process
func (pm *ProcessManager) GetStats(ctx context.Context, id int64) (*model.ProcessStats, error) {
	status := pm.getStatus(ctx, id)
	if status == nil {
		return nil, nil
	}

	stats := &model.ProcessStats{
		CPUPercent: status.CPUPercent,
		MemoryMB:   status.MemoryMB,
		PID:        status.PID,
		Uptime:     status.Uptime,
		Restarts:   status.Restarts,
	}

	// If running, compute uptime
	pm.mu.RLock()
	mp, running := pm.processes[id]
	pm.mu.RUnlock()

	if running && mp.proc != nil {
		mp.mu.Lock()
		stats.Uptime = int64(time.Since(mp.startedAt).Seconds())
		mp.mu.Unlock()
	}

	return stats, nil
}

// GetLogs returns process log entries
func (pm *ProcessManager) GetLogs(ctx context.Context, processID int64, limit, offset int) ([]model.ProcessLog, int, error) {
	if limit <= 0 {
		limit = 50
	}
	return pm.repo.ListLogs(ctx, processID, limit, offset)
}

// --- Batch operations ---

// BatchStart starts multiple processes
func (pm *ProcessManager) BatchStart(ctx context.Context, ids []int64) ([]int64, []int64, error) {
	var started, failed []int64
	for _, id := range ids {
		if err := pm.Start(ctx, id); err != nil {
			failed = append(failed, id)
			log.Printf("process: batch start %d failed: %v", id, err)
		} else {
			started = append(started, id)
		}
	}
	return started, failed, nil
}

// BatchStop stops multiple processes
func (pm *ProcessManager) BatchStop(ctx context.Context, ids []int64) ([]int64, []int64, error) {
	var stopped, failed []int64
	for _, id := range ids {
		if err := pm.Stop(ctx, id); err != nil {
			failed = append(failed, id)
		} else {
			stopped = append(stopped, id)
		}
	}
	return stopped, failed, nil
}

// BatchRestart restarts multiple processes
func (pm *ProcessManager) BatchRestart(ctx context.Context, ids []int64) ([]int64, []int64, error) {
	var restarted, failed []int64
	for _, id := range ids {
		if err := pm.Restart(ctx, id); err != nil {
			failed = append(failed, id)
		} else {
			restarted = append(restarted, id)
		}
	}
	return restarted, failed, nil
}

// --- Group operations ---

// ListGroups returns all process groups
func (pm *ProcessManager) ListGroups(ctx context.Context) ([]model.ProcessGroup, error) {
	return pm.repo.ListGroups(ctx)
}

// CreateGroup creates a new process group
func (pm *ProcessManager) CreateGroup(ctx context.Context, req *model.CreateProcessGroupRequest) (*model.ProcessGroup, error) {
	id, err := pm.repo.CreateGroup(ctx, req.Name, req.Description)
	if err != nil {
		return nil, err
	}
	return &model.ProcessGroup{ID: id, Name: req.Name, Description: req.Description}, nil
}

// UpdateGroup updates a process group
func (pm *ProcessManager) UpdateGroup(ctx context.Context, id int64, req *model.UpdateProcessGroupRequest) error {
	return pm.repo.UpdateGroup(ctx, id, req)
}

// DeleteGroup deletes a process group
func (pm *ProcessManager) DeleteGroup(ctx context.Context, id int64) error {
	return pm.repo.DeleteGroup(ctx, id)
}

// --- Export/Import ---

// Export returns JSON representation of all process configs
func (pm *ProcessManager) Export(ctx context.Context) ([]model.Process, error) {
	return pm.repo.ListProcesses(ctx)
}

// Import imports process configurations from JSON
func (pm *ProcessManager) Import(ctx context.Context, processes []model.Process) (int, error) {
	count := 0
	for _, p := range processes {
		req := &model.CreateProcessRequest{
			Name:         p.Name,
			Command:      p.Command,
			Args:         p.Args,
			Dir:          p.Dir,
			Env:          p.Env,
			AutoRestart:  &p.AutoRestart,
			MaxRestarts:  p.MaxRestarts,
			RestartDelay: p.RestartDelay,
			AutoStart:    &p.AutoStart,
			LogFile:      p.LogFile,
			GroupID:      p.GroupID,
		}
		if _, err := pm.Create(ctx, req); err != nil {
			log.Printf("process: import %s failed: %v", p.Name, err)
			continue
		}
		count++
	}
	return count, nil
}

// --- Internal helpers ---

func (pm *ProcessManager) stopProcess(mp *managedProcess, stopTimeout int) {
	mp.cancel()
	if mp.proc != nil {
		// SIGTERM first for graceful shutdown
		mp.proc.Signal(syscall.SIGTERM)
		done := make(chan error, 1)
		go func() {
			done <- mp.proc.Wait()
		}()
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

func (pm *ProcessManager) waitForExit(ctx context.Context, mp *managedProcess, autoRestart bool, maxRestarts, restartDelay, startupTimeout int) {
	startTime := time.Now()
	err := mp.proc.Wait()
	exitCode := 0
	if err != nil {
		// Process exited with error
		exitCode = 1
	}

	pm.mu.Lock()
	delete(pm.processes, mp.ID)
	pm.mu.Unlock()

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
	pm.updateStatus(context.Background(), mp.ID, status, 0, exitCode, errMsg)

	// Check if should auto-restart with exponential backoff
	if autoRestart && exitCode != 0 {
		// Check if this was a startup failure (exited within startupTimeout)
		runtime := time.Since(startTime).Seconds()
		isStartupFailure := runtime < float64(startupTimeout)

		status := pm.getStatus(context.Background(), mp.ID)
		if status != nil && status.Restarts < maxRestarts {
			// Exponential backoff: base * 2^restarts, capped at 5 minutes
			// Cap shift to prevent integer overflow
			shift := uint(status.Restarts)
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
				pm.addLog(context.Background(), mp.ID, "system",
					fmt.Sprintf("Startup failure detected (exited in %.0fs < %ds), auto-restarting in %ds (attempt %d/%d)",
						runtime, startupTimeout, backoff, status.Restarts+1, maxRestarts))
			} else {
				pm.addLog(context.Background(), mp.ID, "system",
					fmt.Sprintf("Process exited with code %d, auto-restarting in %ds (attempt %d/%d)",
						exitCode, backoff, status.Restarts+1, maxRestarts))
			}

			time.Sleep(time.Duration(backoff) * time.Second)

			// Increment restart counter
			pm.repo.IncrementRestarts(context.Background(), mp.ID)

			// Clear exit code and error before restart
			pm.repo.ClearExitInfo(context.Background(), mp.ID)

			if err := pm.Start(context.Background(), mp.ID); err != nil {
				pm.addLog(context.Background(), mp.ID, "system",
					fmt.Sprintf("Auto-restart failed: %v", err))
				pm.updateStatus(context.Background(), mp.ID, "error", 0, 0, err.Error())
			}
		} else {
			pm.addLog(context.Background(), mp.ID, "system",
				fmt.Sprintf("Max restarts (%d) reached, giving up", maxRestarts))
		}
	}
}

func (pm *ProcessManager) captureOutput(ctx context.Context, processID int64, r io.Reader, logType string) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024) // 1MB max line
	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return
		default:
		}
		pm.addLog(ctx, processID, logType, scanner.Text())
	}
}

func (pm *ProcessManager) updateStatus(ctx context.Context, processID int64, status string, pid int, exitCode int, lastError string) {
	pm.repo.UpsertStatus(ctx, processID, status, pid, exitCode, lastError)
}

func (pm *ProcessManager) addLog(ctx context.Context, processID int64, logType, content string) {
	pm.repo.AppendLog(ctx, processID, logType, content)
}

func (pm *ProcessManager) getStatus(ctx context.Context, processID int64) *model.ProcessStatus {
	s, _ := pm.repo.GetStatus(ctx, processID)
	return s
}

func (pm *ProcessManager) getGroup(ctx context.Context, groupID int64) *model.ProcessGroup {
	if groupID == 0 {
		return nil
	}
	g, _ := pm.repo.GetGroup(ctx, groupID)
	return g
}

func (pm *ProcessManager) autoStartAll() {
	// Wait briefly for DB to be ready
	time.Sleep(2 * time.Second)

	ctx := context.Background()
	ids, err := pm.repo.GetAutoStartIDs(ctx)
	if err != nil {
		log.Printf("process: auto-start query failed: %v", err)
		return
	}

	for _, id := range ids {
		if err := pm.Start(ctx, id); err != nil {
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

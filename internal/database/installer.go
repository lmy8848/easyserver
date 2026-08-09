package database

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
)

// installLog is the in-memory line buffer for one installation. The background
// installer appends to it; SSE subscribers replay everything written so far
// (cursor-based) and then receive live lines.
type installLog struct {
	mu    sync.Mutex
	lines []string
}

func (l *installLog) append(line string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	// Keep a bounded history (same ceiling as service logs).
	if len(l.lines) >= maxLogLines {
		l.lines = append(l.lines[:0], l.lines[len(l.lines)-maxLogLines*3/4:]...)
	}
	l.lines = append(l.lines, line)
}

// Tail returns lines from index from (0-based); callers keep their own cursor
// so they never see a line twice. It returns the new end index.
func (l *installLog) Tail(from int) ([]string, int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if from < 0 || from >= len(l.lines) {
		return nil, len(l.lines)
	}
	return l.lines[from:], len(l.lines)
}

// installTask is one in-flight installation. The instance row's status
// (provisioning → running | failed) is the durable record; this struct carries
// the volatile log buffer and completion signal.
type installTask struct {
	instanceID int64
	log        *installLog
	done       chan struct{}
	err        error
	finishedAt time.Time // set when the install completes; stale tasks are pruned
}

// InstallTask is the handler-facing view of a task (safe to hand out of the
// package): replayable log + completion channel + final error.
type InstallTask struct {
	Log   *installLog
	done  chan struct{}
	errFn func() error
}

// Done is closed when the install finishes (success or failure).
func (t *InstallTask) Done() <-chan struct{} { return t.done }

// Err returns the final error, nil on success.
func (t *InstallTask) Err() error { return t.errFn() }

// installer runs database installations in the background. One install per
// engine at a time (serialized); the instance row is created before the
// goroutine starts, so a page refresh shows "provisioning" and can re-open the
// log stream. Service restart abandons in-flight tasks (their rows stay
// provisioning/failed for manual cleanup).
type installer struct {
	mu    sync.Mutex
	busy  map[DBType]bool
	tasks map[int64]*installTask
}

func newInstaller() *installer {
	return &installer{
		busy:  make(map[DBType]bool),
		tasks: make(map[int64]*installTask),
	}
}

// begin claims the engine for one install. Returns an error if another install
// on the same engine is still running.
func (in *installer) begin(dbType DBType) error {
	in.mu.Lock()
	defer in.mu.Unlock()
	in.pruneDone(installTaskTTL)
	if in.busy[dbType] {
		return fmt.Errorf("该引擎已有安装正在进行，请稍后再试")
	}
	in.busy[dbType] = true
	return nil
}

func (in *installer) end(dbType DBType) {
	in.mu.Lock()
	defer in.mu.Unlock()
	in.busy[dbType] = false
}

// start registers the task and launches the install goroutine. The task stays
// in the map after completion so its log stays replayable (the "继续查看日志"
// flow after refresh/close); only the engine's busy slot is released.
func (in *installer) start(dbType DBType, instanceID int64, run func(log *installLog) error) *installTask {
	task := &installTask{
		instanceID: instanceID,
		log:        &installLog{},
		done:       make(chan struct{}),
	}
	in.mu.Lock()
	in.tasks[instanceID] = task
	in.mu.Unlock()

	go func() {
		defer close(task.done)
		defer in.end(dbType)
		task.err = run(task.log)
		in.mu.Lock()
		task.finishedAt = time.Now()
		in.mu.Unlock()
	}()

	return task
}

// pruneDone drops finished tasks older than ttl. Done tasks are deliberately
// kept (see start) so their logs stay viewable; installs are rare, so pruning
// on each new install keeps the map bounded without a background sweeper.
func (in *installer) pruneDone(ttl time.Duration) {
	cutoff := time.Now().Add(-ttl)
	for id, t := range in.tasks {
		if !t.finishedAt.IsZero() && t.finishedAt.Before(cutoff) {
			delete(in.tasks, id)
		}
	}
}

// get returns the task for an instance, if it still exists.
func (in *installer) get(instanceID int64) (*installTask, bool) {
	in.mu.Lock()
	defer in.mu.Unlock()
	t, ok := in.tasks[instanceID]
	return t, ok
}

// writeLog splits multi-line command output into individual log lines.
func writeLog(log *installLog, msg string) {
	for _, line := range strings.Split(msg, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			log.append(line)
		}
	}
}

// installInstance runs the container creation pipeline and reports progress
// into log. rt is an install-scoped runtime whose command output is hooked into
// log. On failure the container is removed and the row flips to "failed" (the
// row itself stays, so the failure and its log remain visible).
func (s *Service) installInstance(ctx context.Context, dbType DBType, v *DBInstance, spec ContainerSpec, password string, rt DatabaseRuntime, log *installLog) error {
	fail := func(msg string, err error) error {
		_ = rt.Remove(ctx, v.ContainerEngine, v.ContainerID)
		_ = s.repo.UpdateInstanceStatus(ctx, v.ID, "failed")
		log.append("❌ " + msg + ": " + err.Error())
		return err
	}

	log.append("开始安装 " + v.Image + " ...")
	if err := rt.Create(ctx, spec); err != nil {
		return fail("创建容器失败", err)
	}
	log.append("容器已创建")

	if dbType == DBTypeRedis {
		log.append("写入 Redis 配置...")
		if err := seedRedisConfig(ctx, rt, v.ContainerEngine, v.ContainerID, password); err != nil {
			return fail("写入 Redis 配置失败", err)
		}
	}

	log.append("启动容器...")
	if err := rt.Start(ctx, v.ContainerEngine, v.ContainerID); err != nil {
		return fail("启动容器失败", err)
	}
	log.append("等待数据库就绪（最长 2 分钟）...")
	if _, err := waitForHealthy(ctx, rt, v.ContainerEngine, v.ContainerID, 2*time.Minute); err != nil {
		return fail("数据库未就绪", err)
	}
	log.append("✅ 安装完成，数据库已就绪")
	if err := s.repo.UpdateInstanceStatus(ctx, v.ID, "running"); err != nil {
		return err
	}
	return nil
}

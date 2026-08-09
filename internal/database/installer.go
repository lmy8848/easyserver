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

// installTask is one in-flight installation. No database row exists while it
// runs — the task is the only record of the install until it completes (the
// row is written on success as "running", on failure as "failed").
type installTask struct {
	installID  string
	engine     DBType
	version    string
	image      string
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
// engine at a time (serialized); no instance row exists until the install
// completes, so a refresh shows no new instance — only the "正在安装" entry from
// the active list. Service restart abandons in-flight tasks (their logs are
// lost; no half-installed row is left behind).
type installer struct {
	mu    sync.Mutex
	busy  map[DBType]bool
	tasks map[string]*installTask
}

func newInstaller() *installer {
	return &installer{
		busy:  make(map[DBType]bool),
		tasks: make(map[string]*installTask),
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

// InstallMeta carries the display info a task records for the active-install
// list and the log modal title.
type InstallMeta struct {
	Version string
	Image   string
}

// start registers the task and launches the install goroutine. The task stays
// in the map after completion so its log stays replayable (the "继续查看日志"
// flow after refresh/close); only the engine's busy slot is released.
func (in *installer) start(dbType DBType, installID string, meta InstallMeta, run func(log *installLog) error) *installTask {
	task := &installTask{
		installID: installID,
		engine:    dbType,
		version:   meta.Version,
		image:     meta.Image,
		log:       &installLog{},
		done:      make(chan struct{}),
	}
	in.mu.Lock()
	in.tasks[installID] = task
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

// get returns the task for an install id, if it still exists.
func (in *installer) get(installID string) (*installTask, bool) {
	in.mu.Lock()
	defer in.mu.Unlock()
	t, ok := in.tasks[installID]
	return t, ok
}

// ActiveInstall is the handler-facing view of an in-progress install — what the
// front-end needs to show the "正在安装" entry and re-open its log.
type ActiveInstall struct {
	InstallID string `json:"install_id"`
	Engine    string `json:"engine"`
	Version   string `json:"version"`
	Image     string `json:"image"`
}

// Active returns installs still running (not yet done). Finished tasks stay in
// the map (log replay) but drop out of Active.
func (in *installer) Active() []ActiveInstall {
	in.mu.Lock()
	defer in.mu.Unlock()
	var out []ActiveInstall
	for _, t := range in.tasks {
		select {
		case <-t.done:
			continue
		default:
		}
		out = append(out, ActiveInstall{InstallID: t.installID, Engine: string(t.engine), Version: t.version, Image: t.image})
	}
	return out
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
// log. The instance row is written only when the install terminates — "running"
// on success, "failed" on error (the failed row stays so the failure and its
// log remain visible and the container can be cleaned up). While installing,
// no row exists; the in-memory task is the only record.
func (s *Service) installInstance(ctx context.Context, dbType DBType, version, image, engineName, containerID, volumeName, bindAddress string, port int, password string, spec ContainerSpec, rt DatabaseRuntime, log *installLog) error {
	writeRow := func(status string) {
		v := &DBInstance{DBType: dbType, Version: version, Port: port, Status: status,
			ContainerEngine: engineName, Image: image, ContainerID: containerID,
			VolumeName: volumeName, ConfigDir: spec.ConfigDir, BindAddress: bindAddress, AdminPassword: password}
		if _, err := s.repo.CreateInstance(ctx, v); err != nil {
			log.append("❌ 写入实例记录失败: " + err.Error())
		}
	}
	fail := func(msg string, err error) error {
		_ = rt.Remove(ctx, engineName, containerID)
		writeRow("failed")
		log.append("❌ " + msg + ": " + err.Error())
		return err
	}

	log.append("开始安装 " + image + " ...")
	if err := rt.Create(ctx, spec); err != nil {
		return fail("创建容器失败", err)
	}
	log.append("容器已创建")

	if dbType == DBTypeRedis {
		log.append("写入 Redis 配置...")
		if err := seedRedisConfig(ctx, rt, engineName, containerID, password); err != nil {
			return fail("写入 Redis 配置失败", err)
		}
	}

	log.append("启动容器...")
	if err := rt.Start(ctx, engineName, containerID); err != nil {
		return fail("启动容器失败", err)
	}
	log.append("等待数据库就绪（最长 2 分钟）...")
	if _, err := waitForHealthy(ctx, rt, engineName, containerID, 2*time.Minute); err != nil {
		return fail("数据库未就绪", err)
	}
	log.append("✅ 安装完成，数据库已就绪")
	writeRow("running")
	return nil
}

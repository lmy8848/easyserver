package plugin

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"easyserver/internal/infra/errx"
)

// Supervisor 托管插件进程：拉起（注入 token 环境）→ 等 socket → 健康检查 →
// 崩溃指数退避重启 → 窗口内失败过多转 disabled。进程崩溃绝不波及主面板
// （文档 §12.2）。
type Supervisor struct {
	repo        PluginRepository
	hostSocket  string // Host API socket 路径（注入插件环境）
	socketDir   string
	pluginsDir  string

	mu       sync.Mutex
	procs    map[string]*pfProcess
	health   *http.Client // unix socket HTTP 客户端
	stopping bool
}

type pfProcess struct {
	manifest  *Manifest
	token     string
	cmd       *exec.Cmd
	done      chan error
	cancel    context.CancelFunc
	failCount int
	lastFail  time.Time
}

// NewSupervisor 创建进程托管器。socketDir / pluginsDir 通常用包级常量，
// 测试可注入临时目录。
func NewSupervisor(repo PluginRepository, hostSocket, socketDir, pluginsDir string) *Supervisor {
	tr := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, "unix", hostSocket)
		},
	}
	return &Supervisor{
		repo:       repo,
		hostSocket: hostSocket,
		socketDir:  socketDir,
		pluginsDir: pluginsDir,
		procs:      map[string]*pfProcess{},
		health:     &http.Client{Transport: tr, Timeout: 3 * time.Second},
	}
}

// CleanupOrphans 启动时清理：删除残留 socket 文件，杀掉 cmdline 含插件
// 目录的孤儿进程（文档 §12.2）。
func (s *Supervisor) CleanupOrphans(ctx context.Context) {
	entries, err := os.ReadDir(s.socketDir)
	if err == nil {
		for _, e := range entries {
			if strings.HasSuffix(e.Name(), ".sock") {
				p := filepath.Join(s.socketDir, e.Name())
				if err := os.Remove(p); err == nil {
					log.Printf("plugin: 清理残留 socket %s", p)
				}
			}
		}
	}
	killOrphansByCmdline(s.pluginsDir)
}

// killOrphansByCmdline 扫描 /proc，杀掉可执行路径位于插件目录下的残留进程。
// 仅 Linux（面板目标平台）。
func killOrphansByCmdline(pluginsDir string) {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return
	}
	for _, e := range entries {
		if !isAllDigits(e.Name()) {
			continue
		}
		cmdline, err := os.ReadFile("/proc/" + e.Name() + "/cmdline")
		if err != nil {
			continue
		}
		argv0 := strings.TrimRight(string(cmdline), "\x00")
		if argv0 != "" && strings.HasPrefix(argv0, pluginsDir+"/") {
			pid, _ := strconv.Atoi(e.Name())
			if p, err := os.FindProcess(pid); err == nil {
				_ = p.Kill()
				log.Printf("plugin: 清理孤儿进程 pid=%d path=%s", pid, argv0)
			}
		}
	}
}

func isAllDigits(s string) bool {
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return len(s) > 0
}

// EnsureRunning 拉起插件进程并等待其 socket 就绪 + 健康检查通过。
// 幂等：已在运行则直接返回。
func (s *Supervisor) EnsureRunning(ctx context.Context, m *Manifest, token string) error {
	s.mu.Lock()
	if s.stopping {
		s.mu.Unlock()
		return errx.Internal("插件管理器正在关闭")
	}
	if p, ok := s.procs[m.Name]; ok && p.cmd != nil && p.cmd.Process != nil {
		s.mu.Unlock()
		return nil
	}
	s.mu.Unlock()

	if err := os.MkdirAll(s.socketDir, 0700); err != nil {
		return errx.Internal("创建插件 socket 目录失败: %w", err)
	}
	dir := filepath.Join(s.pluginsDir, m.Name)
	sockPath := filepath.Join(s.socketDir, m.Runtime.Socket)

	_ = os.Remove(sockPath) // 残留 socket 会导致监听失败
	execPath := filepath.Join(dir, m.Runtime.Exec)
	if _, err := os.Stat(execPath); err != nil {
		return errx.Internal("插件可执行文件不存在: %s", execPath)
	}

	pctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(pctx, execPath, m.Runtime.Args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"EASYSERVER_PLUGIN_TOKEN="+token,
		"EASYSERVER_HOST_SOCKET="+s.hostSocket,
		"EASYSERVER_PLUGIN_ID="+m.Name,
	)
	for k, v := range m.Runtime.Env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	if err := cmd.Start(); err != nil {
		cancel()
		return errx.Internal("拉起插件进程失败: %w", err)
	}

	p := &pfProcess{manifest: m, token: token, cmd: cmd, done: make(chan error, 1), cancel: cancel}
	s.mu.Lock()
	s.procs[m.Name] = p
	s.mu.Unlock()
	_ = s.repo.SetPluginStatus(ctx, m.Name, StateStarting, cmd.Process.Pid, "")

	// 等待 socket 就绪（≤ startup_timeout）
	deadline := time.Now().Add(m.Runtime.StartupTimeout)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(sockPath); err == nil {
			if err := s.healthOK(ctx, m); err == nil {
				s.mu.Lock()
				p.failCount = 0
				s.mu.Unlock()
				_ = s.repo.SetPluginStatus(ctx, m.Name, StateRunning, cmd.Process.Pid, "")
				go s.watchExit(p)
				go s.healthLoop(p)
				log.Printf("plugin: %s 已运行 pid=%d", m.Name, cmd.Process.Pid)
				return nil
			}
		}
		select {
		case err := <-p.done:
			cancel()
			_ = s.repo.SetPluginStatus(ctx, m.Name, StateError, 0, err.Error())
			return errx.Internal("插件 %s 启动即退出: %v", m.Name, err)
		case <-ctx.Done():
			cancel()
			return ctx.Err()
		case <-time.After(200 * time.Millisecond):
		}
	}
	cancel()
	_ = s.repo.SetPluginStatus(ctx, m.Name, StateError, 0, "startup_timeout")
	return errx.Internal("插件 %s 在 %s 内未就绪", m.Name, m.Runtime.StartupTimeout)
}

// healthOK 通过插件 socket 调 /health。
func (s *Supervisor) healthOK(ctx context.Context, m *Manifest) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://plugin"+m.Health.Path, nil)
	if err != nil {
		return err
	}
	resp, err := s.health.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 500 {
		return fmt.Errorf("health %d", resp.StatusCode)
	}
	return nil
}

// watchExit 监听进程退出并按退避策略重启（文档 §12.2：1s 起步指数退避，
// 10 分钟窗口失败 >5 次转 disabled 停止重启）。
func (s *Supervisor) watchExit(p *pfProcess) {
	err := p.cmd.Wait()
	p.done <- err

	if s.isStopping() {
		return
	}
	ctx := context.Background()
	_ = s.repo.SetPluginStatus(ctx, p.manifest.Name, StateDegraded, 0, fmt.Sprintf("exited: %v", err))
	_ = s.repo.IncRestartCount(ctx, p.manifest.Name)

	s.mu.Lock()
	p.failCount++
	now := time.Now()
	if p.lastFail.IsZero() || now.Sub(p.lastFail) > 10*time.Minute {
		p.failCount = 1
	}
	p.lastFail = now
	tooMany := p.failCount > 5
	s.mu.Unlock()

	if tooMany {
		_ = s.repo.SetPluginStatus(ctx, p.manifest.Name, StateDisabled, 0, "连续失败过多，已停止重启")
		log.Printf("plugin: %s 10 分钟内失败超过 5 次，停止重启", p.manifest.Name)
		return
	}
	delay := backoffDelay(p.failCount)
	log.Printf("plugin: %s 退出（%v），%s 后第 %d 次重启", p.manifest.Name, err, delay, p.failCount)
	time.AfterFunc(delay, func() {
		if s.isStopping() {
			return
		}
		// token 不变；重放 EnsureRunning（manifest 从 repo 记录的 manifest_path 重新加载）
		m, err := loadManifestFor(p.manifest.Name)
		if err != nil {
			log.Printf("plugin: %s 重启失败: %v", p.manifest.Name, err)
			return
		}
		if err := s.EnsureRunning(context.Background(), m, p.token); err != nil {
			log.Printf("plugin: %s 重启失败: %v", p.manifest.Name, err)
		}
	})
}

// loadManifestFor 从插件目录重新加载 manifest（重启路径使用）。
// manager（PR-2）接入后由 PluginManager 提供。
func loadManifestFor(name string) (*Manifest, error) {
	dir := filepath.Join(PluginsDir, name)
	return LoadManifest(filepath.Join(dir, "plugin.yaml"), dir)
}

func backoffDelay(failCount int) time.Duration {
	d := time.Second
	for i := 1; i < failCount && d < 60*time.Second; i++ {
		d *= 2
	}
	if d > 60*time.Second {
		d = 60 * time.Second
	}
	return d
}

// healthLoop 周期健康检查：连续 FailThreshold 次失败 → degraded 并重启。
func (s *Supervisor) healthLoop(p *pfProcess) {
	interval := p.manifest.Health.Interval
	if interval <= 0 {
		interval = 30 * time.Second
	}
	t := time.NewTicker(interval)
	defer t.Stop()
	fails := 0
	for range t.C {
		s.mu.Lock()
		cur, ok := s.procs[p.manifest.Name]
		if !ok || cur != p || s.stopping {
			s.mu.Unlock()
			return
		}
		s.mu.Unlock()

		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		err := s.healthOK(ctx, p.manifest)
		cancel()
		if err == nil {
			fails = 0
			_ = s.repo.SetPluginStatus(context.Background(), p.manifest.Name, StateRunning, p.cmd.Process.Pid, "")
			continue
		}
		fails++
		if fails == 1 {
			_ = s.repo.SetPluginStatus(context.Background(), p.manifest.Name, StateDegraded, p.cmd.Process.Pid, err.Error())
		}
		if fails >= p.manifest.Health.FailThreshold {
			log.Printf("plugin: %s 健康检查连续失败 %d 次，重启", p.manifest.Name, fails)
			_ = p.cancel() // 触发 watchExit 的退避重启路径
			return
		}
	}
}

// StopPlugin 停止插件进程（SIGTERM 10s grace → SIGKILL）并清理状态。
func (s *Supervisor) StopPlugin(ctx context.Context, name string) error {
	s.mu.Lock()
	p, ok := s.procs[name]
	delete(s.procs, name)
	s.mu.Unlock()
	if !ok {
		return nil // 本就未运行
	}
	_ = s.repo.SetPluginStatus(ctx, name, StateDisabled, 0, "")
	if p.cmd.Process == nil {
		return nil
	}
	_ = p.cmd.Process.Signal(os.Interrupt)
	done := make(chan struct{})
	go func() { _, _ = p.cmd.Process.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		_ = p.cmd.Process.Kill()
	}
	_ = os.Remove(filepath.Join(s.socketDir, p.manifest.Runtime.Socket))
	return nil
}

// StopAll 面板关停时调用：对所有运行中的插件发出停止（最多等 10s）。
func (s *Supervisor) StopAll(ctx context.Context) {
	s.mu.Lock()
	s.stopping = true
	names := make([]string, 0, len(s.procs))
	for n := range s.procs {
		names = append(names, n)
	}
	s.mu.Unlock()
	for _, n := range names {
		_ = s.StopPlugin(ctx, n)
	}
}

func (s *Supervisor) isStopping() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.stopping
}

// GenerateToken 生成插件 token（32 字节随机 hex）。
func GenerateToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

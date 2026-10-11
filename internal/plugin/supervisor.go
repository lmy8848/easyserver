package plugin

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"easyserver/internal/infra/errx"
)

// Supervisor 托管插件进程：拉起（注入 token 环境）→ 等 socket → 健康检查 →
// 崩溃指数退避重启 → 窗口内失败过多转 disabled。进程崩溃绝不波及主面板
// （文档 §12.2）。
type Supervisor struct {
	repo       PluginRepository
	hostSocket string
	socketDir  string
	pluginsDir string

	mu       sync.Mutex
	procs    map[string]*pfProcess
	stopping bool
}

type pfProcess struct {
	manifest *Manifest
	token    string
	client   *http.Client
	cmd      *exec.Cmd
	cancel   context.CancelFunc // CommandContext 取消（健康失败时杀进程）
	exited   chan struct{}      // 进程退出时关闭
	exitErr  error

	stopOnce sync.Once   // StopPlugin 主动停止标记
	stopped  atomic.Bool // true 时退出监视跳过状态写库（避免覆盖 disabled）
	restarts atomic.Int32

	stateMu sync.Mutex
	state   string // 进程内状态缓存，避免并发写库互相覆盖
}

// setState 仅在状态变化时写库（healthLoop / 退出监视 / EnsureRunning 并发存在）。
func (s *Supervisor) setState(ctx context.Context, p *pfProcess, status, lastErr string, pid int) {
	p.stateMu.Lock()
	if p.state == status {
		p.stateMu.Unlock()
		return
	}
	p.state = status
	p.stateMu.Unlock()
	_ = s.repo.SetPluginStatus(ctx, p.manifest.Name, status, pid, lastErr)
}

// NewSupervisor 创建进程托管器。socketDir / pluginsDir 通常用包级常量，
// 测试可注入临时目录。
func NewSupervisor(repo PluginRepository, hostSocket, socketDir, pluginsDir string) *Supervisor {
	return &Supervisor{
		repo:       repo,
		hostSocket: hostSocket,
		socketDir:  socketDir,
		pluginsDir: pluginsDir,
		procs:      map[string]*pfProcess{},
	}
}

// pluginHealthClient 返回拨向插件 socket 的 HTTP 客户端。
func pluginHealthClient(socketPath string) *http.Client {
	tr := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, "unix", socketPath)
		},
	}
	return &http.Client{Transport: tr, Timeout: 3 * time.Second}
}

// CleanupOrphans 启动时清理：删除残留 socket 文件，杀掉可执行路径位于
// 插件目录下的孤儿进程（文档 §12.2）。
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

// killOrphansByCmdline 扫描 /proc，杀掉 cmdline 落在插件目录下的残留进程。
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
	if s == "" {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// EnsureRunning 拉起插件进程并等待 socket 就绪 + 健康检查通过。幂等。
func (s *Supervisor) EnsureRunning(ctx context.Context, m *Manifest, token string) error {
	s.mu.Lock()
	if s.stopping {
		s.mu.Unlock()
		return errx.Internal("插件管理器正在关闭")
	}
	if p, ok := s.procs[m.Name]; ok && !p.stopped.Load() {
		select {
		case <-p.exited: // 已死，走重建
		default:
			s.mu.Unlock()
			return nil
		}
	}
	s.mu.Unlock()

	if err := os.MkdirAll(s.socketDir, 0700); err != nil {
		return errx.Internal("创建插件 socket 目录失败: %w", err)
	}
	dir := filepath.Join(s.pluginsDir, m.Name)
	sockPath := filepath.Join(s.socketDir, m.Runtime.Socket)
	_ = os.Remove(sockPath)

	execPath := filepath.Join(dir, m.Runtime.Exec)
	if _, err := os.Stat(execPath); err != nil {
		return errx.Internal("插件可执行文件不存在: %s", execPath)
	}

	m.ManifestDefaults() // 防御：绕过 LoadManifest 构造的 manifest 也保证健康检查字段完整
	p := &pfProcess{
		manifest: m,
		token:    token,
		client:   pluginHealthClient(sockPath),
		exited:   make(chan struct{}),
	}
	s.setState(ctx, p, StateStarting, "", 0)

	pctx, cancel := context.WithCancel(ctx)
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
		s.setState(ctx, p, StateError, err.Error(), 0)
		return errx.Internal("拉起插件进程失败: %w", err)
	}
	p.cmd = cmd
	p.cancel = cancel
	s.mu.Lock()
	s.procs[m.Name] = p
	s.mu.Unlock()

	// 退出监视：非主动停止的退出 → degraded + 指数退避重启
	go func() { //nolint:contextcheck // 退出监视独立于启动请求的上下文，状态写库用 Background
		err := cmd.Wait()
		p.exitErr = err
		close(p.exited)
		if p.stopped.Load() || s.isStopping() {
			return
		}
		s.setState(context.Background(), p, StateDegraded, fmt.Sprintf("exited: %v", err), 0)
		s.restartWithBackoff(p)
	}()

	// 等待 socket 就绪 + 健康通过（≤ startup_timeout）
	deadline := time.Now().Add(m.Runtime.StartupTimeout)
	for time.Now().Before(deadline) {
		select {
		case <-p.exited:
			cancel()
			s.setState(ctx, p, StateError, fmt.Sprintf("启动即退出: %v", p.exitErr), 0)
			return errx.Internal("插件 %s 启动即退出: %v", m.Name, p.exitErr)
		default:
		}
		if _, err := os.Stat(sockPath); err == nil {
			if err := s.healthOK(ctx, p); err == nil {
				s.setState(ctx, p, StateRunning, "", cmd.Process.Pid)
				go s.healthLoop(p)
				return nil
			}
		}
		select {
		case <-p.exited:
			cancel()
			s.setState(ctx, p, StateError, fmt.Sprintf("启动即退出: %v", p.exitErr), 0)
			return errx.Internal("插件 %s 启动即退出: %v", m.Name, p.exitErr)
		case <-ctx.Done():
			cancel()
			return ctx.Err()
		case <-time.After(200 * time.Millisecond):
		}
	}
	cancel()
	s.setState(ctx, p, StateError, "startup_timeout", 0)
	return errx.Internal("插件 %s 在 %s 内未就绪", m.Name, m.Runtime.StartupTimeout)
}

// restartWithBackoff 指数退避重启（1s 起步 60s 封顶；10 分钟窗口 >5 次停止）。
//
//nolint:contextcheck // 重启生命周期独立于触发退出的请求上下文，必须用 Background
func (s *Supervisor) restartWithBackoff(p *pfProcess) {
	ctx := context.Background()
	_ = s.repo.IncRestartCount(ctx, p.manifest.Name)
	n := p.restarts.Add(1)
	if n > 5 {
		s.setState(context.Background(), p, StateDisabled, "10 分钟窗口内失败超过 5 次，已停止重启", 0)
		log.Printf("plugin: %s 连续失败超过 5 次，停止重启", p.manifest.Name)
		return
	}
	delay := time.Second
	for i := 1; i < int(n) && delay < 60*time.Second; i++ {
		delay *= 2
	}
	log.Printf("plugin: %s 退出（%v），%s 后第 %d 次重启", p.manifest.Name, p.exitErr, delay, n)
	time.AfterFunc(delay, func() {
		if s.isStopping() || p.stopped.Load() {
			return
		}
		if err := s.EnsureRunning(context.Background(), p.manifest, p.token); err != nil {
			log.Printf("plugin: %s 重启失败: %v", p.manifest.Name, err)
		}
	})
}

// healthOK 通过插件 socket 调 /health。
func (s *Supervisor) healthOK(ctx context.Context, p *pfProcess) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://plugin"+p.manifest.Health.Path, nil)
	if err != nil {
		return err
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 500 {
		return fmt.Errorf("health %d", resp.StatusCode)
	}
	return nil
}

// healthLoop 周期健康检查：连续 FailThreshold 次失败 → 重启进程。
//
//nolint:contextcheck // 健康循环是独立长驻 goroutine，状态写库用 Background
func (s *Supervisor) healthLoop(p *pfProcess) {
	interval := p.manifest.Health.Interval
	if interval <= 0 {
		interval = 30 * time.Second
	}
	t := time.NewTicker(interval)
	defer t.Stop()
	fails := 0
	for range t.C {
		select {
		case <-p.exited:
			return
		default:
		}
		if s.isStopping() {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		err := s.healthOK(ctx, p)
		cancel()
		if err == nil {
			s.setState(context.Background(), p, StateRunning, "", p.cmd.Process.Pid)
			fails = 0
			continue
		}
		fails++
		if fails >= p.manifest.Health.FailThreshold {
			log.Printf("plugin: %s 健康检查连续失败 %d 次，重启", p.manifest.Name, fails)
			p.cancel() // 退出监视器会走退避重启
			return
		}
	}
}

// RunningEndpoint 返回运行中插件的 socket 路径与请求超时；ok=false 表示未运行。
func (s *Supervisor) RunningEndpoint(name string) (socketPath string, timeout time.Duration, ok bool) {
	s.mu.Lock()
	p, exists := s.procs[name]
	s.mu.Unlock()
	if !exists || p.stopped.Load() {
		return "", 0, false
	}
	select {
	case <-p.exited:
		return "", 0, false
	default:
	}
	t := p.manifest.Runtime.RequestTimeout
	if t <= 0 {
		t = 30 * time.Second
	}
	return filepath.Join(s.socketDir, p.manifest.Runtime.Socket), t, true
}

// StopPlugin 停止插件进程（SIGINT 10s grace → Kill）并等待完全退出。
func (s *Supervisor) StopPlugin(ctx context.Context, name string) error {
	s.mu.Lock()
	p, ok := s.procs[name]
	if ok {
		delete(s.procs, name)
	}
	s.mu.Unlock()
	if !ok {
		return nil
	}
	p.stopOnce.Do(func() {
		p.stopped.Store(true)
		if p.cmd.Process != nil {
			_ = p.cmd.Process.Signal(os.Interrupt)
		}
	})
	select {
	case <-p.exited:
	case <-time.After(10 * time.Second):
		if p.cmd.Process != nil {
			_ = p.cmd.Process.Kill()
			<-p.exited
		}
	}
	s.setState(ctx, p, StateDisabled, "", 0)
	_ = os.Remove(filepath.Join(s.socketDir, p.manifest.Runtime.Socket))
	return nil
}

func (s *Supervisor) isStopping() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.stopping
}

// GenerateToken 生成插件 token（32 字节随机 hex，每次拉起注入，不持久化）。
func GenerateToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// StopAll 面板关停时调用：对所有运行中的插件发出停止。
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

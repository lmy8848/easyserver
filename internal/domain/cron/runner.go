package cron

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"log"
	"os/exec"
	"sync"
	"syscall"
	"time"

	"easyserver/internal/infra"
	"easyserver/internal/util"
)

// RunningScript 表示一个正在运行的脚本实例。
// 进程独立于任何 WS 连接生命周期：WS 断开只取消订阅，不 Kill 进程。
type RunningScript struct {
	scriptID  int64
	cmd       *exec.Cmd
	stdout    io.ReadCloser
	stderr    io.ReadCloser
	startTime time.Time

	mu   sync.Mutex
	subs map[chan []byte]struct{} // 订阅者（WS 连接）
	done chan struct{}            // 进程退出时关闭
	once sync.Once                // 保证 done 只 close 一次

	journalCh chan string    // 待写入 journald 的行队列
	catCmd    *exec.Cmd      // systemd-cat 进程（nil = 未启用持久化）
	catStdinW io.WriteCloser // systemd-cat stdin writer
}

// setpgidCmd 构造独立进程组的 *exec.Cmd（背景：脚本进程与 systemd-cat 都要
// 能整体 Kill 而不波及面板进程）。仅在 runner 与 cron service 启动子进程处使用。
func setpgidCmd(name string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(context.Background(), name, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	return cmd
}

// Running 是运行中脚本的对外摘要，供 REST 查询前端显示标记。
type Running struct {
	ID        int64  `json:"id"`
	StartedAt string `json:"started_at"`
}

// ScriptRunner 管理运行中的脚本进程。单实例：同一脚本同时只允许一个运行实例，
// 再次 Start 复用已有实例。
type ScriptRunner struct {
	mu      sync.RWMutex
	running map[int64]*RunningScript
}

// NewScriptRunner 创建 ScriptRunner。
func NewScriptRunner() *ScriptRunner {
	return &ScriptRunner{
		running: make(map[int64]*RunningScript),
	}
}

// Get 返回运行中的脚本实例；未运行返回 ok=false。
func (r *ScriptRunner) Get(scriptID int64) (*RunningScript, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	rs, ok := r.running[scriptID]
	return rs, ok
}

// RunningIDs 返回所有运行中脚本 id。
func (r *ScriptRunner) RunningIDs() []int64 {
	r.mu.RLock()
	defer r.mu.RUnlock()
	ids := make([]int64, 0, len(r.running))
	for id := range r.running {
		ids = append(ids, id)
	}
	return ids
}

// List 返回运行中脚本摘要列表。
func (r *ScriptRunner) List() []Running {
	r.mu.RLock()
	defer r.mu.RUnlock()
	list := make([]Running, 0, len(r.running))
	for id, rs := range r.running {
		list = append(list, Running{ID: id, StartedAt: rs.startTime.Format(util.TimeLayout)})
	}
	return list
}

// Start 启动并注册一个脚本执行。若该脚本已在运行，直接返回现有实例（单实例）。
// 进程独立于调用方生命周期，由 runner 持有，进程退出后自动从 map 移除。
// 注意：必须用 context.Background() 而非调用方 ctx —— WebSocket 断开时请求 ctx 会被 cancel，
// exec.CommandContext 会随之 SIGKILL 进程，导致刷新页面后脚本被误杀。
func (r *ScriptRunner) Start(script *Script) (*RunningScript, error) {
	r.mu.Lock()
	if rs, ok := r.running[script.ID]; ok {
		r.mu.Unlock()
		return rs, nil // 已运行，复用现有实例
	}

	// systemd-cat 持久化管道：脚本输出落 journald（identifier=easyserver-script-<name>）。
	var catCmd *exec.Cmd
	var catStdinW io.WriteCloser
	if cmd := setpgidCmd("systemd-cat", "-t", "easyserver-script-"+script.Name); cmd != nil {
		if w, wErr := cmd.StdinPipe(); wErr == nil {
			if err := cmd.Start(); err == nil {
				catCmd = cmd
				catStdinW = w
			} else {
				log.Printf("启动 systemd-cat 失败（日志不入 journald）: %v", err)
			}
		} else {
			log.Printf("获取 systemd-cat stdin 失败: %v", wErr)
		}
	}

	// 主执行进程：直接 exec.CommandContext 拿 *exec.Cmd，先取 stdout/stderr pipe 再 Start。
	// 注意 pipe 必须在 Start 之前设置，否则报 "StdoutPipe after process started"。
	cmd := setpgidCmd(script.Path)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		if catCmd != nil {
			_ = catCmd.Process.Kill()
		}
		r.mu.Unlock()
		return nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		if catCmd != nil {
			_ = catCmd.Process.Kill()
		}
		r.mu.Unlock()
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		if catCmd != nil {
			_ = catCmd.Process.Kill()
		}
		r.mu.Unlock()
		return nil, err
	}

	rs := &RunningScript{
		scriptID:  script.ID,
		cmd:       cmd,
		stdout:    stdout,
		stderr:    stderr,
		startTime: time.Now(),
		subs:      make(map[chan []byte]struct{}),
		done:      make(chan struct{}),
		journalCh: make(chan string, 256),
		catCmd:    catCmd,
		catStdinW: catStdinW,
	}
	r.running[script.ID] = rs
	r.mu.Unlock()

	rs.runPumps()
	// 进程退出：关闭 done → 从 map 移除 → 清理 journal 写入。
	infra.Go(func() {
		_ = cmd.Wait()
		if rs.catStdinW != nil {
			close(rs.journalCh)
		}
		rs.once.Do(func() { close(rs.done) })
		if rs.catCmd != nil {
			_ = rs.catCmd.Process.Kill()
		}
		r.mu.Lock()
		if cur, ok := r.running[script.ID]; ok && cur == rs {
			delete(r.running, script.ID)
		}
		r.mu.Unlock()
	})
	return rs, nil
}

// Stop 杀掉脚本进程并从 map 移除。进程组随之终止（setpgid）。
func (r *ScriptRunner) Stop(scriptID int64) {
	r.mu.Lock()
	rs, ok := r.running[scriptID]
	if ok {
		delete(r.running, scriptID)
	}
	r.mu.Unlock()
	if ok && rs.cmd.Process != nil {
		_ = rs.cmd.Process.Kill()
	}
}

// Subscribe 注册一个订阅者，返回日志 chan 和取消函数。
// 取消只注销订阅，不 Kill 进程。
func (rs *RunningScript) Subscribe() (chan []byte, func()) {
	ch := make(chan []byte, 128)
	rs.mu.Lock()
	rs.subs[ch] = struct{}{}
	rs.mu.Unlock()

	cancel := func() {
		rs.mu.Lock()
		delete(rs.subs, ch)
		rs.mu.Unlock()
	}
	return ch, cancel
}

// Done 返回进程退出信号（只关闭一次）。
func (rs *RunningScript) Done() <-chan struct{} { return rs.done }

// Cmd 返回底层 *exec.Cmd，供读退出码。
func (rs *RunningScript) Cmd() *exec.Cmd { return rs.cmd }

// StartedAtStr 返回启动时间字符串。
func (rs *RunningScript) StartedAtStr() string { return rs.startTime.Format(util.TimeLayout) }

// runPumps 启动 stdout/stderr pump goroutine：读进程输出 → 写 journald + 广播订阅者。
// 任一 pump 结束即视为进程接近退出，由 Wait goroutine 完成清理。
func (rs *RunningScript) runPumps() {
	stdout := rs.stdout
	stderr := rs.stderr

	// journald 写入与实时流解耦：独立 goroutine 消费 channel，避免
	// systemd-cat 消费慢/挂起时阻塞 pump（否则实时日志会卡住）。
	if rs.catStdinW != nil {
		infra.Go(func() {
			for line := range rs.journalCh {
				_, _ = rs.catStdinW.Write([]byte(line + "\n"))
			}
			rs.catStdinW.Close() // channel 关闭后：关 stdin 让 systemd-cat 落盘退出
		})
	}

	pump := func(r io.Reader, stream string) {
		if r == nil {
			return
		}
		scanner := bufio.NewScanner(r)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024) // 行缓冲最大 1MB
		for scanner.Scan() {
			line := scanner.Text()
			now := time.Now().Format(util.TimeLayout)
			msg, _ := json.Marshal(map[string]any{
				"type": "log",
				"data": map[string]any{
					"stream":  stream,
					"message": line,
					"time":    now,
				},
			})
			// 落 journald：非阻塞投递，放不下直接丢弃，绝不阻塞实时广播。
			if rs.catStdinW != nil {
				select {
				case rs.journalCh <- line:
				default:
				}
			}
			rs.broadcast(msg)
		}
	}

	infra.Go(func() { pump(stdout, "stdout") })
	infra.Go(func() { pump(stderr, "stderr") })
}

// broadcast 把一条日志 JSON 投递给所有订阅者（非阻塞，满则丢弃）。
func (rs *RunningScript) broadcast(msg []byte) {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	for ch := range rs.subs {
		select {
		case ch <- msg:
		default:
		}
	}
}

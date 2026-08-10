package task

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

// Status 是任务的可观察状态。终态只有 succeeded / failed / canceled 三种，
// 无排队态（并发超限直接拒绝，不排队）。
type Status string

const (
	StatusRunning   Status = "running"
	StatusSucceeded Status = "succeeded"
	StatusFailed    Status = "failed"
	StatusCanceled  Status = "canceled"
)

// ErrKeyBusy 表示同 key 已有正在运行的任务（去重拒绝）。调用方可用 errors.Is
// 识别并翻译成领域文案（如数据库安装的"已有安装正在进行"）。
var ErrKeyBusy = errors.New("task key is busy")

// Options 是任务级能力开关，全部按需开启（零值 = 关闭）：
// Timeout=0 无超时、MaxRetries=0 不重试、RetryInterval 仅在重试时使用。
type Options struct {
	Timeout       time.Duration // 0 = 无超时；超时归入失败并触发重试
	MaxRetries    int           // 0 = 不重试；失败重试次数（不含首次）
	RetryInterval time.Duration // 重试固定间隔，默认 3s
}

// TaskLog 是任务的可选日志附件：内存环形缓冲 + 游标回放。订阅者先回放已缓冲
// 行（Tail 从游标起）再收实时行；不接收日志的任务不产生任何流式成本。
type TaskLog struct {
	mu    sync.Mutex
	lines []string
}

const maxLogLines = 5000

// Append 追加一行日志。环形缓冲：超过上限时丢弃最旧的 1/4。
func (l *TaskLog) Append(line string) {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(l.lines) >= maxLogLines {
		l.lines = append(l.lines[:0], l.lines[len(l.lines)-maxLogLines*3/4:]...)
	}
	l.lines = append(l.lines, line)
}

// Tail 返回从游标 from 起的行（0 基）；调用方持自己的游标所以每行只见一次。
// 返回新游标位置（即已见过的行尾）。
func (l *TaskLog) Tail(from int) ([]string, int) {
	if l == nil {
		return nil, 0
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if from < 0 || from >= len(l.lines) {
		return nil, len(l.lines)
	}
	return l.lines[from:], len(l.lines)
}

// Task 是任务执行器对一次后台执行的句柄：完成信号 + 最终错误 + 状态 + 可选日志。
// key 既是去重键也是查找句柄（同一 key 同时只运行一个任务，因此 key 唯一标识
// 一次执行）。
type Task struct {
	key    string
	status Status
	err    error
	log    *TaskLog
	done   chan struct{}

	mu       sync.Mutex
	cancelFn context.CancelFunc // Cancel() 触发；nil = 任务已完成或无需取消
}

// Done 在任务完成（成功/失败/取消）时关闭。
func (t *Task) Done() <-chan struct{} { return t.done }

// Key 返回任务的去重键/句柄。
func (t *Task) Key() string { return t.key }

// Err 返回最终错误：成功为 nil，失败/取消为最后一次尝试的错误。
func (t *Task) Err() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.err
}

// Status 返回当前状态。
func (t *Task) Status() Status {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.status
}

// Log 返回任务日志（仅 StartWithLog 注入）；无日志任务返回 nil。
func (t *Task) Log() *TaskLog {
	return t.log
}

// Manager 是后台任务执行器：同 key 去重、全局并发上限、任务级超时/重试/取消。
// key 是唯一的注册维度（去重键 = 查找句柄）：succeeded 完成即清；failed/canceled
// 保留至同 key 下一次 Start（覆盖），因此 map 天然有界，无 TTL 清理器。
type Manager struct {
	sem chan struct{}

	mu    sync.Mutex
	byKey map[string]*Task
}

// NewManager 创建执行器，concurrency 为全局并发上限（同时运行的任务数）。
func NewManager(concurrency int) *Manager {
	if concurrency < 1 {
		concurrency = 1
	}
	return &Manager{
		sem:   make(chan struct{}, concurrency),
		byKey: make(map[string]*Task),
	}
}

// Start 启动一个无日志任务。key 是去重键也是查找句柄——同一 key 同时只运行一个
// 任务；重复提交同步返回错误。任务体从调用方的 ctx 脱离（后台执行，由本执行器
// 持有生命周期），fn 在派生 ctx 上运行。
func (m *Manager) Start(key string, opts Options, fn func(ctx context.Context) error) (*Task, error) {
	return m.start(key, opts, fn, nil)
}

// StartWithLog 同 Start，但给任务体注入一个 TaskLog（可选日志附件）。任务日志
// 通过游标回放（Tail），供 SSE 等订阅者先回放再收实时行。
func (m *Manager) StartWithLog(key string, opts Options, fn func(ctx context.Context, log *TaskLog) error) (*Task, error) {
	if fn == nil {
		return nil, fmt.Errorf("task fn is required")
	}
	return m.start(key, opts, nil, fn)
}

func (m *Manager) start(key string, opts Options, plain func(ctx context.Context) error, withLog func(ctx context.Context, log *TaskLog) error) (*Task, error) {
	if key == "" {
		return nil, fmt.Errorf("task key is required")
	}
	if plain == nil && withLog == nil {
		return nil, fmt.Errorf("task fn is required")
	}
	if opts.RetryInterval == 0 {
		opts.RetryInterval = 3 * time.Second
	}

	// 同 key 去重 + 并发上限：同步检查，超限/去重直接返回错误（不排队）。
	// 已完成的同 key 终态（failed/canceled）可被覆盖——"失败保留至重装"。
	m.mu.Lock()
	if old, busy := m.byKey[key]; busy {
		old.mu.Lock()
		s := old.status
		old.mu.Unlock()
		if s == StatusRunning {
			m.mu.Unlock()
			return nil, fmt.Errorf("%w: %s", ErrKeyBusy, key)
		}
	}
	select {
	case m.sem <- struct{}{}:
	default:
		m.mu.Unlock()
		return nil, fmt.Errorf("后台任务并发数已达上限，请稍后再试")
	}

	var log *TaskLog
	var fn func(ctx context.Context) error
	if withLog != nil {
		log = &TaskLog{}
		fn = func(ctx context.Context) error { return withLog(ctx, log) }
	} else {
		fn = plain
	}
	tk := &Task{
		key:    key,
		status: StatusRunning,
		log:    log,
		done:   make(chan struct{}),
	}
	m.byKey[key] = tk
	m.mu.Unlock()

	go m.run(tk, opts, fn)
	return tk, nil
}

// run 是执行循环。语义要点：
//   - 用户取消优先：canceled 是唯一非重试终态；
//   - 超时归入失败并触发重试（每次尝试拿全新 ctx）；
//   - 重试期间状态仍为 running（任务在 Active 视角不消失）。
func (m *Manager) run(tk *Task, opts Options, fn func(ctx context.Context) error) {
	// 该任务的执行 ctx 独立于调用方：从 background 派生，Cancel 时取消。
	ctx, cancel := context.WithCancel(context.Background())
	tk.mu.Lock()
	tk.cancelFn = cancel
	tk.mu.Unlock()

	// 释放并发槽位；成功即清（失败/取消保留至同 key 下次 Start）。
	defer func() {
		<-m.sem
		m.mu.Lock()
		tk.mu.Lock()
		status := tk.status
		tk.mu.Unlock()
		if status == StatusSucceeded {
			// 只清仍指向本任务的记录：成功与同 key 重装可能交错（close(done) 后
			// Start 已注册新任务），旧任务的清理不得删掉新任务的注册。
			if m.byKey[tk.key] == tk {
				delete(m.byKey, tk.key)
			}
		}
		m.mu.Unlock()
	}()

	maxAttempts := opts.MaxRetries + 1
	for attempt := 0; ; attempt++ {
		// 单次尝试在独立闭包内执行，保证带 Timeout 时的 cancel 总是被调用。
		err := func() error {
			attemptCtx := ctx
			if opts.Timeout > 0 {
				var cancel context.CancelFunc
				attemptCtx, cancel = context.WithTimeout(ctx, opts.Timeout)
				defer cancel()
			}
			return fn(attemptCtx)
		}()

		// 用户取消优先：父 ctx 取消（仅来自 Cancel）→ 终态 canceled，绝不重试。
		if ctx.Err() == context.Canceled {
			tk.mu.Lock()
			tk.status = StatusCanceled
			tk.err = err
			tk.mu.Unlock()
			close(tk.done)
			return
		}

		// 成功：成功即清（defer 里处理），终态 succeeded。
		if err == nil {
			tk.mu.Lock()
			tk.status = StatusSucceeded
			tk.mu.Unlock()
			close(tk.done)
			return
		}

		// 失败（含超时→DeadlineExceeded，归入 failed）：还有重试次数则重试。
		if attempt+1 >= maxAttempts {
			tk.mu.Lock()
			tk.status = StatusFailed
			tk.err = err
			tk.mu.Unlock()
			close(tk.done)
			return
		}

		// 真正要重试才打分隔行——MaxRetries=0 不产生任何额外日志（迁移零行为
		// 变化），且"第 N/M 次尝试失败"是在下一次尝试前打的。
		if tk.log != nil {
			tk.log.Append(fmt.Sprintf("第 %d/%d 次尝试失败: %v，%s 后重试", attempt+1, maxAttempts, err, opts.RetryInterval))
		}

		// 重试间隔：期间状态仍为 running；等待时监听取消。
		select {
		case <-ctx.Done():
			tk.mu.Lock()
			tk.status = StatusCanceled
			tk.err = context.Canceled
			tk.mu.Unlock()
			close(tk.done)
			return
		case <-time.After(opts.RetryInterval):
		}
	}
}

// Cancel 取消一个正在运行的任务（用户主动取消）。返回 false 表示 key 未知或任务
// 已完成。取消优先于超时/重试：canceled 是唯一非重试终态。
func (m *Manager) Cancel(key string) bool {
	m.mu.Lock()
	tk, ok := m.byKey[key]
	m.mu.Unlock()
	if !ok {
		return false
	}
	select {
	case <-tk.done:
		return false // 已完成，无需取消
	default:
	}
	tk.mu.Lock()
	fn := tk.cancelFn
	tk.mu.Unlock()
	if fn != nil {
		fn()
	}
	return true
}

// Get 返回任务句柄。返回 ok=false 表示不存在（可能成功即清，或从未存在）。
func (m *Manager) Get(key string) (*Task, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	tk, ok := m.byKey[key]
	return tk, ok
}

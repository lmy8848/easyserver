package cron

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"easyserver/internal/domain/notification"
	"easyserver/internal/infra/config"
	"easyserver/internal/infra/mise"
)

// cronWatchInterval 是定时任务失败巡检间隔。
const cronWatchInterval = 5 * time.Minute

// 脚本/任务命令的落盘目录，由面板根（config.DataRoot）派生。集中定义，
// 避免路径常量散落各文件。
const (
	// scriptsDir 是脚本落盘目录（脚本内容存储，DB 仅存元数据）。
	scriptsDir = config.DataRoot + "/scripts"
	// taskCommandDir 是任务命令落盘目录，与脚本库分离（见 timer_manager.go）。
	taskCommandDir = scriptsDir + "/tasks"
)

// Service 管理定时任务与脚本/文档：任务 CRUD/状态/日志委托给 TimerManager，脚本/文档走 SQLite。
type Service struct {
	repo Repository
	tm   *TimerManager

	mu        sync.Mutex
	lastSeen  map[string]string // task name → LastResult，翻转检测基线
	notifSink notification.Sink
}

// NewService creates a new cron Service.
func NewService(repo Repository, provider mise.Provider, runtime RuntimeLookup) *Service {
	return NewServiceWithSink(repo, provider, runtime, nil)
}

// NewServiceWithSink 在 NewService 基础上附加通知 sink（nil 时失败巡检不发送）。
// runtime 提供"已安装运行环境"校验（ADR-0009），由 router 注入 runtimeenv.Service。
func NewServiceWithSink(repo Repository, provider mise.Provider, runtime RuntimeLookup, sink notification.Sink) *Service {
	return &Service{
		repo:      repo,
		tm:        NewTimerManager(provider, runtime),
		lastSeen:  make(map[string]string),
		notifSink: sink,
	}
}

func (s *Service) List(ctx context.Context) ([]CronTask, error) {
	return s.tm.List(ctx)
}

func (s *Service) Get(ctx context.Context, name string) (*CronTask, error) {
	return s.tm.Get(ctx, name)
}

func (s *Service) Create(ctx context.Context, task *CronTask) error {
	return s.tm.Create(ctx, task)
}

func (s *Service) Update(ctx context.Context, task *CronTask) error {
	return s.tm.Update(ctx, task)
}

func (s *Service) Delete(ctx context.Context, name string) error {
	return s.tm.Delete(ctx, name)
}

func (s *Service) Enable(ctx context.Context, name string) error {
	return s.tm.Enable(ctx, name)
}

func (s *Service) Disable(ctx context.Context, name string) error {
	return s.tm.Disable(ctx, name)
}

func (s *Service) RunNow(ctx context.Context, name string) error {
	return s.tm.RunNow(ctx, name)
}

func (s *Service) GetRuns(ctx context.Context, name string, limit int) ([]CronRun, error) {
	return s.tm.GetRuns(ctx, name, limit)
}

func (s *Service) ListScripts(ctx context.Context) ([]Script, error) {
	return s.repo.ListScripts(ctx)
}

func (s *Service) GetScript(ctx context.Context, id int64) (*Script, error) {
	script, err := s.repo.GetScript(ctx, id)
	if err != nil {
		return nil, err
	}
	content, err := s.repo.ReadScriptFile(id)
	if err != nil {
		return nil, err
	}
	script.Content = content
	return script, nil
}

// CreateScript 先写元数据行拿 ID，再原样写内容文件（含用户自带的 shebang，
// 执行靠文件首行解释器决定，不额外补）；文件写失败回滚记录。
func (s *Service) CreateScript(ctx context.Context, script *Script) error {
	if err := s.repo.CreateScript(ctx, script); err != nil {
		return err
	}
	if err := s.repo.WriteScriptFile(script.ID, script.Content); err != nil {
		_ = s.repo.DeleteScript(ctx, script.ID)
		return fmt.Errorf("写脚本文件失败（已回滚记录）: %w", err)
	}
	script.Path = scriptFilePath(script.ID)
	return nil
}

// UpdateScript 更新元数据；提供新内容则原样重写文件。
func (s *Service) UpdateScript(ctx context.Context, script *Script) error {
	if err := s.repo.UpdateScript(ctx, script); err != nil {
		return err
	}
	if script.Content != "" {
		if err := s.repo.WriteScriptFile(script.ID, script.Content); err != nil {
			return fmt.Errorf("重写脚本文件失败: %w", err)
		}
	}
	return nil
}

func (s *Service) DeleteScript(ctx context.Context, id int64) error {
	if err := s.repo.DeleteScript(ctx, id); err != nil {
		return err
	}
	return s.repo.DeleteScriptFile(id)
}

// --- 失败巡检 ---

// StartWatcher 启动定时任务失败巡检：每 cronWatchInterval 扫描一次全部任务的
// LastResult，翻转（success → failed）才发站内通知。stopCh 关闭时退出。
// 首次扫描只建基线不通知，存量失败不刷屏；持续失败不重复（无翻转不触发）。
func (s *Service) StartWatcher(stopCh <-chan struct{}) {
	go func() {
		ticker := time.NewTicker(cronWatchInterval)
		defer ticker.Stop()
		for {
			select {
			case <-stopCh:
				return
			case <-ticker.C:
				s.sweep()
			}
		}
	}()
}

// sweep 执行一次巡检：列出全部任务，与上次结果比对，翻转才通知。
func (s *Service) sweep() {
	tasks, err := s.List(context.Background())
	if err != nil {
		log.Printf("cron: watcher list tasks: %v", err)
		return
	}
	changed := s.reconcile(taskResults(tasks))
	for _, f := range changed {
		s.notifyTaskFailed(f)
	}
}

// taskResults 抽取任务的 name → LastResult 映射。
func taskResults(tasks []CronTask) map[string]string {
	m := make(map[string]string, len(tasks))
	for _, t := range tasks {
		m[t.Name] = t.LastResult
	}
	return m
}

// taskFailedResult 判定 systemd 的 Result 属性是否代表执行失败。LastResult 是
// systemctl show 的 Result 值原样（timer_manager.go），成功为 "success"；失败
// 是 exit-code / signal / timeout / start-limit-hit 等，绝不等于字面 "failed"。
func taskFailedResult(result string) bool {
	switch result {
	case "success", "":
		return false
	default:
		return true
	}
}

// reconcile 是纯状态机：拿当前结果与 lastSeen 基线比对，返回本次发生
// 非失败 → 失败翻转的任务。**副作用只有更新基线**，通知由调用方发。
//
// 规则：
//   - 首次见到（基线无此任务）→ 只建基线，不通知（存量失败不刷屏）
//   - 基线为失败（持续失败）→ 不通知
//   - 基线非失败、当前失败 → 翻转，通知（记录基线为失败）
//   - 基线为失败、当前非失败（恢复）→ 更新基线，不通知
func (s *Service) reconcile(curr map[string]string) []string {
	s.mu.Lock()
	defer s.mu.Unlock()

	var failed []string
	for name, result := range curr {
		prev, seen := s.lastSeen[name]
		if !seen {
			s.lastSeen[name] = result
			continue
		}
		if result != prev {
			// 状态变了：当前失败才算值得通知的事件
			s.lastSeen[name] = result
			if !taskFailedResult(prev) && taskFailedResult(result) {
				failed = append(failed, name)
			}
		}
	}
	return failed
}

// notifyTaskFailed 向站内通知投递任务失败。sink 未接线或发送失败都只记日志。
func (s *Service) notifyTaskFailed(name string) {
	if s.notifSink == nil {
		return
	}
	if _, err := s.notifSink.CreateIfNotExists(notification.CreateNotificationRequest{
		Type:    "cron",
		Title:   "定时任务失败：" + name,
		Message: fmt.Sprintf("定时任务 %s 上次执行失败，请检查任务日志。", name),
		Level:   "error",
	}); err != nil {
		log.Printf("cron: notify task failed %q: %v", name, err)
	}
}

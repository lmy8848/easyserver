package plugin

import (
	"context"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/robfig/cron/v3"
)

// serviceScheduler 运行插件 manifest.services 声明的定时任务（文档 §5）：
// 面板按 5 段 cron 表达式调度，以 `sh -c <command>` 执行，全程审计 + 超时。
// 调度与插件启停绑定；服务命令由管理员显式安装的插件声明，等价面板自身
// 定时任务的可信度。
type serviceScheduler struct {
	mu   sync.Mutex
	cron *cron.Cron
	jobs map[string][]cron.EntryID // 插件名 → 已注册的 entry 列表
}

func newServiceScheduler() *serviceScheduler {
	return &serviceScheduler{
		cron: cron.New(),
		jobs: map[string][]cron.EntryID{},
	}
}

func (s *serviceScheduler) start() { s.cron.Start() }

func (s *serviceScheduler) stop() { s.cron.Stop() }

// schedule 注册插件声明的全部 services；重复调用先移除旧任务（幂等）。
func (s *serviceScheduler) schedule(m *Manager, name string, mft *Manifest) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, id := range s.jobs[name] {
		s.cron.Remove(id)
	}
	delete(s.jobs, name)
	for _, svc := range mft.Contributions.Services {
		cmd := strings.TrimSpace(svc.Command)
		if cmd == "" {
			logManagerf(" %s 服务 %s 命令为空，跳过", name, svc.ID)
			continue
		}
		id, err := s.cron.AddFunc(svc.Schedule, func() {
			runServiceCommand(m, name, cmd)
		})
		if err != nil {
			logManagerf(" %s 服务 %s 调度表达式无效: %v", name, svc.ID, err)
			continue
		}
		s.jobs[name] = append(s.jobs[name], id)
	}
}

// unschedule 移除插件的全部定时任务（停止/禁用/卸载时调用）。
func (s *serviceScheduler) unschedule(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, id := range s.jobs[name] {
		s.cron.Remove(id)
	}
	delete(s.jobs, name)
}

// runServiceCommand 执行插件声明的服务命令（sh -c，审计 + 超时 + 输出日志）。
func runServiceCommand(m *Manager, pluginName, command string) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	_ = m.AuditWrite(ctx, pluginName, "service: "+command)
	out, err := exec.CommandContext(ctx, "sh", "-c", command).CombinedOutput()
	if err != nil {
		logManagerf(" %s 服务命令执行失败: %v", pluginName, err)
	}
	if s := strings.TrimSpace(string(out)); s != "" {
		logManagerf(" %s 服务输出: %s", pluginName, s)
	}
}

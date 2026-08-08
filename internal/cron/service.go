package cron

import (
	"context"
	"fmt"

	"easyserver/internal/infra/executor"
	"easyserver/internal/infra/mise"
)

// Service 管理定时任务与脚本/文档：任务 CRUD/状态/日志委托给 TimerManager，脚本/文档走 SQLite。
type Service struct {
	repo Repository
	tm   *TimerManager
}

// NewService creates a new cron Service.
func NewService(repo Repository, exec executor.CommandExecutor, provider mise.Provider) *Service {
	return &Service{
		repo: repo,
		tm:   NewTimerManager(exec, provider, repo, repo),
	}
}

func (s *Service) List(ctx context.Context) ([]CronTask, error) {
	return s.tm.List(ctx)
}

func (s *Service) Get(ctx context.Context, name string) (*CronTask, error) {
	return s.tm.Get(ctx, name)
}

func (s *Service) Create(ctx context.Context, task *CronTask) error {
	if err := s.resolveScriptCommand(ctx, task); err != nil {
		return err
	}
	return s.tm.Create(ctx, task)
}

func (s *Service) Update(ctx context.Context, task *CronTask) error {
	if err := s.resolveScriptCommand(ctx, task); err != nil {
		return err
	}
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

func (s *Service) GetLogs(ctx context.Context, name string, tail int) ([]LogLine, error) {
	return s.tm.GetLogs(ctx, name, tail)
}

// resolveScriptCommand 任务引用脚本时，把 ExecStart 指向脚本落盘文件
// （脚本文件带语言 shebang，mise exec 提供运行时 PATH 供解释器解析）。
func (s *Service) resolveScriptCommand(ctx context.Context, task *CronTask) error {
	if task.ScriptID <= 0 {
		return nil
	}
	// 校验脚本存在，避免创建指向不存在文件的 unit。
	if _, err := s.repo.GetScript(ctx, int64(task.ScriptID)); err != nil {
		return fmt.Errorf("脚本 %d 不存在", task.ScriptID)
	}
	task.Command = scriptFilePath(int64(task.ScriptID))
	return nil
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

// CreateScript 先写元数据行拿 ID，再写带 shebang 的内容文件；文件写失败回滚记录。
func (s *Service) CreateScript(ctx context.Context, script *Script) error {
	if err := s.repo.CreateScript(ctx, script); err != nil {
		return err
	}
	if err := s.repo.WriteScriptFile(script.ID, withShebang(script.Content, script.Language)); err != nil {
		_ = s.repo.DeleteScript(ctx, script.ID)
		return fmt.Errorf("写脚本文件失败（已回滚记录）: %w", err)
	}
	return nil
}

// UpdateScript 更新元数据；提供新内容则重写带 shebang 的文件。
func (s *Service) UpdateScript(ctx context.Context, script *Script) error {
	if err := s.repo.UpdateScript(ctx, script); err != nil {
		return err
	}
	if script.Content != "" {
		if err := s.repo.WriteScriptFile(script.ID, withShebang(script.Content, script.Language)); err != nil {
			return fmt.Errorf("重写脚本文件失败: %w", err)
		}
	}
	return nil
}

// withShebang 按语言补 shebang 行，使落盘脚本可执行。
func withShebang(content, language string) string {
	shebang := map[string]string{
		"sh":      "#!/bin/sh",
		"bash":    "#!/bin/bash",
		"python":  "#!/usr/bin/env python3",
		"python3": "#!/usr/bin/env python3",
		"":        "#!/bin/sh",
	}[language]
	return shebang + "\n" + content
}

func (s *Service) DeleteScript(ctx context.Context, id int64) error {
	if err := s.repo.DeleteScript(ctx, id); err != nil {
		return err
	}
	return s.repo.DeleteScriptFile(id)
}

func (s *Service) SeedDefaultDocs(ctx context.Context) error {
	count, err := s.repo.CountDocs(ctx)
	if err != nil {
		return err
	}
	if count > 0 {
		return nil
	}

	defaultDocs := []CronDoc{
		{
			Title:     "调度频率",
			SortOrder: 1,
			Content: `## 调度频率

定时任务使用预设频率表单，无需手工编写调度表达式：

| 频率 | 说明 |
|------|------|
| 每 N 分钟 | 按分钟步长触发，如每 5 分钟 |
| 每 N 小时 | 按小时步长触发，如每 3 小时 |
| 每天 | 每天固定时间触发 |
| 每周 | 每周固定几天 + 时间触发 |
| 每月 | 每月固定日 + 时间触发 |

选择频率后，系统会自动生成对应的执行计划，并在任务列表中显示下次执行时间。`,
		},
		{
			Title:     "持久化执行",
			SortOrder: 2,
			Content: `## 持久化执行

开启后，若系统在预定触发时间处于关机或休眠状态，错过的执行计划将在下次开机时自动补齐执行。
适合日志轮转、数据备份等不宜漏跑的周期性任务。默认关闭（严格按计划时间执行）。`,
		},
		{
			Title:     "重试与超时",
			SortOrder: 3,
			Content: `## 重试与超时

- 任务失败后会自动重试，达到最大重试次数后停止。
- 超时时间用于限制单次执行的时长，卡住的任务不会无限运行。
- 运行时的日志会自动记录，可在任务详情页查看。`,
		},
		{
			Title:     "Shell 脚本技巧",
			SortOrder: 4,
			Content: `## Shell 脚本常用技巧

~~~bash
#!/bin/bash
set -e    # 遇到错误立即退出
set -u    # 使用未定义变量报错
set -o pipefail  # 管道中任何命令失败都算失败
~~~

### 防止重复执行
~~~bash
LOCK_FILE="/tmp/myscript.lock"
if [ -f "$LOCK_FILE" ]; then
    echo "脚本正在运行，退出"
    exit 1
fi
trap "rm -f $LOCK_FILE" EXIT
touch "$LOCK_FILE"
~~~

### 超时控制
~~~bash
timeout 300 long_running_command  # 5 分钟超时
~~~`,
		},
	}

	return s.repo.BatchCreateDocs(ctx, defaultDocs)
}

func (s *Service) ListDocs(ctx context.Context) ([]CronDoc, error) {
	return s.repo.ListDocs(ctx)
}

func (s *Service) GetDoc(ctx context.Context, id int64) (*CronDoc, error) {
	return s.repo.GetDoc(ctx, id)
}

func (s *Service) UpdateDoc(ctx context.Context, doc *CronDoc) error {
	return s.repo.UpdateDoc(ctx, doc)
}

func (s *Service) CreateDoc(ctx context.Context, doc *CronDoc) error {
	return s.repo.CreateDoc(ctx, doc)
}

func (s *Service) DeleteDoc(ctx context.Context, id int64) error {
	return s.repo.DeleteDoc(ctx, id)
}

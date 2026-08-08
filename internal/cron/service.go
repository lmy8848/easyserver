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
// 同时提供执行命令与脚本时，执行命令优先，脚本忽略。
func (s *Service) resolveScriptCommand(ctx context.Context, task *CronTask) error {
	if task.ScriptID <= 0 || task.Command != "" {
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

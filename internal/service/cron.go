package service

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"easyserver/internal/executor"
	"easyserver/internal/model"
	"easyserver/internal/repository"
)

// CronService manages cron tasks and their execution
type CronService struct {
	repo     repository.CronRepository
	executor executor.CommandExecutor
}

// NewCronService creates a new CronService
func NewCronService(repo repository.CronRepository, exec executor.CommandExecutor) *CronService {
	return &CronService{repo: repo, executor: exec}
}

// List returns all cron tasks
func (s *CronService) List(ctx context.Context) ([]model.CronTask, error) {
	return s.repo.ListTasks(ctx)
}

// Get returns a cron task by ID
func (s *CronService) Get(ctx context.Context, id int64) (*model.CronTask, error) {
	return s.repo.GetTask(ctx, id)
}

// Create creates a new cron task
func (s *CronService) Create(ctx context.Context, task *model.CronTask) error {
	return s.repo.CreateTask(ctx, task)
}

// Update updates an existing cron task
func (s *CronService) Update(ctx context.Context, task *model.CronTask) error {
	return s.repo.UpdateTask(ctx, task)
}

// Delete deletes a cron task and its logs
func (s *CronService) Delete(ctx context.Context, id int64) error {
	return s.repo.DeleteTask(ctx, id)
}

// Enable enables a cron task
func (s *CronService) Enable(ctx context.Context, id int64) error {
	return s.repo.EnableTask(ctx, id)
}

// Disable disables a cron task
func (s *CronService) Disable(ctx context.Context, id int64) error {
	return s.repo.DisableTask(ctx, id)
}

// RunNow executes a cron task immediately (async)
// Returns error only if task cannot be started (not found, already running)
// Actual execution happens in a background goroutine
func (s *CronService) RunNow(ctx context.Context, id int64) error {
	// Check task exists
	task, err := s.Get(ctx, id)
	if err != nil {
		return fmt.Errorf("task not found: %w", err)
	}

	// Atomic concurrency guard: only proceed if not already running
	ok, err := s.repo.SetTaskRunning(ctx, id)
	if err != nil {
		return fmt.Errorf("failed to update task status: %w", err)
	}
	if !ok {
		return fmt.Errorf("task is already running")
	}

	// Execute asynchronously
	go s.executeTask(task)

	return nil
}

// executeTask runs the actual task execution in a background goroutine
func (s *CronService) executeTask(task *model.CronTask) {
	// Use background context - not tied to HTTP request
	ctx := context.Background()

	var output []byte
	var runErr error
	attempts := 0
	maxRetry := task.MaxRetry
	if maxRetry < 0 {
		maxRetry = 0
	}
	if maxRetry > 10 {
		maxRetry = 10
	}

	start := time.Now()

	for attempts <= maxRetry {
		attempts++

		// Create context with timeout if specified
		runCtx := ctx
		var cancel context.CancelFunc
		if task.Timeout > 0 {
			timeout := task.Timeout
			if timeout > 86400 {
				timeout = 86400
			}
			runCtx, cancel = context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
		}

		// Build command
		var command string
		if task.ScriptID > 0 {
			script, scriptErr := s.repo.GetScript(ctx, int64(task.ScriptID))
			if scriptErr != nil {
				if cancel != nil {
					cancel()
				}
				output = []byte(fmt.Sprintf("script not found: %v", scriptErr))
				runErr = scriptErr
				break
			}
			// Validate script content: reject null bytes
			if strings.ContainsRune(script.Content, '\x00') {
				if cancel != nil {
					cancel()
				}
				output = []byte("script content contains null byte")
				runErr = fmt.Errorf("invalid script content")
				break
			}
			command = script.Content
		} else {
			// Validate command: reject null bytes and enforce max length
			if strings.ContainsRune(task.Command, '\x00') {
				if cancel != nil {
					cancel()
				}
				output = []byte("command contains null byte")
				runErr = fmt.Errorf("invalid command")
				break
			}
			const maxCmdLen = 8192
			if len(task.Command) > maxCmdLen {
				if cancel != nil {
					cancel()
				}
				output = []byte(fmt.Sprintf("command exceeds maximum length (%d bytes)", maxCmdLen))
				runErr = fmt.Errorf("command too long")
				break
			}
			command = task.Command
		}

		// Build options
		opts := executor.CommandOptions{}
		if task.WorkDir != "" {
			if _, err := os.Stat(task.WorkDir); err == nil {
				opts.WorkDir = task.WorkDir
			} else {
				log.Printf("cron: work_dir %s does not exist, using default", task.WorkDir)
			}
		}
		if task.EnvVars != "" {
			opts.Env = parseEnvVars(task.EnvVars)
		}

		outputStr, _, runErr := s.executor.RunWithOptions(runCtx, opts, "sh", "-c", command)
		output = []byte(outputStr)
		if cancel != nil {
			cancel()
		}
		if runErr == nil {
			break
		}

		if attempts <= maxRetry {
			time.Sleep(time.Second * time.Duration(attempts))
		}
	}

	duration := int(time.Since(start).Milliseconds())

	status := "success"
	if runErr != nil {
		status = "failed"
		if len(output) == 0 {
			output = []byte(runErr.Error())
		} else {
			output = append(output, []byte("\n"+runErr.Error())...)
		}
	}

	// Truncate output
	outputStr := string(output)
	const maxOutputLen = 10000
	if len(outputStr) > maxOutputLen {
		outputStr = outputStr[:maxOutputLen] + "\n... (truncated)"
	}

	// Insert log
	if err := s.repo.CreateLog(ctx, task.ID, status, outputStr, duration); err != nil {
		log.Printf("cron: failed to insert log for task %d: %v", task.ID, err)
	}

	// Update task status
	const maxResultLen = 500
	if err := s.repo.UpdateTaskResult(ctx, task.ID, status, truncate(outputStr, maxResultLen)); err != nil {
		log.Printf("cron: failed to update task %d status: %v", task.ID, err)
	}

	log.Printf("cron: task %d (%s) completed with status=%s in %dms", task.ID, task.Name, status, duration)
}

// parseEnvVars parses environment variables from a newline-separated KEY=VALUE string.
// Validates that keys contain only safe characters (alphanumeric, underscore) and
// rejects lines with null bytes or control characters to prevent injection.
func parseEnvVars(envStr string) []string {
	var envs []string
	for _, line := range strings.Split(envStr, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Reject null bytes and control characters
		if strings.ContainsRune(line, '\x00') {
			continue
		}
		// Split on first '=' only (SplitN with n=2)
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := parts[0]
		if key == "" {
			continue
		}
		// Validate key: only alphanumeric, underscore, dot (POSIX convention)
		validKey := true
		for _, ch := range key {
			if !((ch >= 'A' && ch <= 'Z') || (ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9') || ch == '_' || ch == '.') {
				validKey = false
				break
			}
		}
		if !validKey {
			log.Printf("cron: skipping env var with invalid key: %q", key)
			continue
		}
		envs = append(envs, line)
	}
	return envs
}

// GetLogs returns execution logs for a cron task
func (s *CronService) GetLogs(ctx context.Context, taskID int64, limit int) ([]model.CronLog, error) {
	if limit <= 0 {
		limit = 50
	}
	return s.repo.GetLogs(ctx, taskID, limit)
}

// ListEnabled returns all enabled cron tasks
func (s *CronService) ListEnabled(ctx context.Context) ([]model.CronTask, error) {
	return s.repo.ListEnabledTasks(ctx)
}

// SyncToSystemCrontab writes enabled tasks to the system crontab
func (s *CronService) SyncToSystemCrontab(ctx context.Context) error {
	tasks, err := s.ListEnabled(ctx)
	if err != nil {
		return err
	}

	var lines []string
	lines = append(lines, "# EasyServer managed cron tasks - DO NOT EDIT MANUALLY")
	for _, t := range tasks {
		lines = append(lines, fmt.Sprintf("%s root %s # easycron:%d:%s",
			t.Schedule, t.Command, t.ID, t.Name))
	}

	content := strings.Join(lines, "\n") + "\n"
	crontabPath := "/etc/cron.d/easyserver"
	if err := os.WriteFile(crontabPath, []byte(content), 0644); err != nil {
		return fmt.Errorf("write crontab file: %w", err)
	}
	return nil
}

// RemoveFromSystemCrontab removes a task from the system crontab
func (s *CronService) RemoveFromSystemCrontab(ctx context.Context, taskID int64) error {
	// Re-sync all tasks (simple approach)
	return s.SyncToSystemCrontab(ctx)
}

// ListScripts returns all scripts
func (s *CronService) ListScripts(ctx context.Context) ([]model.Script, error) {
	return s.repo.ListScripts(ctx)
}

// GetScript returns a script by ID
func (s *CronService) GetScript(ctx context.Context, id int64) (*model.Script, error) {
	return s.repo.GetScript(ctx, id)
}

// CreateScript creates a new script
func (s *CronService) CreateScript(ctx context.Context, script *model.Script) error {
	return s.repo.CreateScript(ctx, script)
}

// UpdateScript updates an existing script
func (s *CronService) UpdateScript(ctx context.Context, script *model.Script) error {
	return s.repo.UpdateScript(ctx, script)
}

// DeleteScript deletes a script by ID
func (s *CronService) DeleteScript(ctx context.Context, id int64) error {
	return s.repo.DeleteScript(ctx, id)
}

// ============================================================
// Cron documentation management
// ============================================================

// SeedDefaultDocs inserts default documentation if table is empty
func (s *CronService) SeedDefaultDocs(ctx context.Context) error {
	count, err := s.repo.CountDocs(ctx)
	if err != nil {
		return err
	}
	if count > 0 {
		return nil
	}

	defaultDocs := []model.CronDoc{
		{
			Title:     "Cron 表达式基础",
			SortOrder: 1,
			Content: `## Cron 表达式格式

一个 Cron 表达式由 5 个字段组成，用空格分隔：

| 字段 | 含义 | 范围 | 允许的特殊字符 |
|------|------|------|----------------|
| 分钟 | 0-59 | * , - / | |
| 小时 | 0-23 | * , - / | |
| 日 | 1-31 | * , - / | |
| 月 | 1-12 | * , - / | |
| 星期 | 0-7 (0和7都是周日) | * , - / | |

## 特殊字符说明

| 字符 | 含义 | 示例 |
|------|------|------|
| * | 任意值 | * * * * * 每分钟 |
| , | 列举多个值 | 0 9,18 * * * 每天9点和18点 |
| - | 范围 | 0 9 * * 1-5 工作日9点 |
| / | 步长 | */5 * * * * 每5分钟 |`,
		},
		{
			Title:     "常用表达式示例",
			SortOrder: 2,
			Content: `## 常用 Cron 表达式

| 表达式 | 说明 |
|--------|------|
| * * * * * | 每分钟执行 |
| */5 * * * * | 每 5 分钟执行 |
| 0 * * * * | 每小时整点执行 |
| 0 2 * * * | 每天凌晨 2:00 执行 |
| 0 9 * * 1-5 | 工作日每天 9:00 执行 |
| 0 0 * * 0 | 每周日午夜执行 |
| 0 0 1 * * | 每月 1 号午夜执行 |
| 0 0 1 1 * | 每年 1 月 1 日午夜执行 |
| 30 4 1,15 * * | 每月 1 号和 15 号 4:30 执行 |
| 0 */2 * * * | 每 2 小时执行 |
| 0 8-17 * * 1-5 | 工作日 8:00-17:00 每小时执行 |`,
		},
		{
			Title:     "高级用法",
			SortOrder: 3,
			Content: "## 高级 Cron 技巧\n\n" +
				"### 组合使用\n" +
				"- 0 9,12,18 * * * — 每天 9:00、12:00、18:00 执行\n" +
				"- 0 0 * * 1,3,5 — 每周一、三、五午夜执行\n" +
				"- */10 8-18 * * 1-5 — 工作日 8:00-18:00 每 10 分钟执行\n\n" +
				"### 常见场景\n" +
				"- 数据库备份: 0 2 * * * — 每天凌晨 2 点\n" +
				"- 日志清理: 0 0 * * 0 — 每周日午夜\n" +
				"- 健康检查: */5 * * * * — 每 5 分钟\n" +
				"- 报表生成: 0 8 1 * * — 每月 1 号早上 8 点\n" +
				"- 缓存刷新: 0 */6 * * * — 每 6 小时\n\n" +
				"### 注意事项\n" +
				"- 星期几的字段中，0 和 7 都表示周日\n" +
				"- 避免在整点(:00)执行大量任务，错开几分钟\n" +
				"- 生产环境建议加上随机偏移，避免同时执行",
		},
		{
			Title:     "Shell 脚本技巧",
			SortOrder: 4,
			Content: "## Shell 脚本常用技巧\n\n" +
				"### 错误处理\n" +
				"```bash\n" +
				"#!/bin/bash\n" +
				"set -e  # 遇到错误立即退出\n" +
				"set -u  # 使用未定义变量报错\n" +
				"set -o pipefail  # 管道中任何命令失败都算失败\n" +
				"```\n\n" +
				"### 日志记录\n" +
				"```bash\n" +
				"LOG_FILE=\"/var/log/myscript.log\"\n" +
				"log() {\n" +
				"    echo \"[$(date '+%Y-%m-%d %H:%M:%S')] $1\" | tee -a \"$LOG_FILE\"\n" +
				"}\n" +
				"log \"任务开始\"\n" +
				"# ... 执行任务 ...\n" +
				"log \"任务完成\"\n" +
				"```\n\n" +
				"### 锁机制（防止重复执行）\n" +
				"```bash\n" +
				"LOCK_FILE=\"/tmp/myscript.lock\"\n" +
				"if [ -f \"$LOCK_FILE\" ]; then\n" +
				"    echo \"脚本正在运行，退出\"\n" +
				"    exit 1\n" +
				"fi\n" +
				"trap \"rm -f $LOCK_FILE\" EXIT\n" +
				"touch \"$LOCK_FILE\"\n" +
				"```\n\n" +
				"### 超时控制\n" +
				"```bash\n" +
				"timeout 300 long_running_command  # 5 分钟超时\n" +
				"```",
		},
	}

	return s.repo.BatchCreateDocs(ctx, defaultDocs)
}

// ListDocs returns all documentation sections
func (s *CronService) ListDocs(ctx context.Context) ([]model.CronDoc, error) {
	return s.repo.ListDocs(ctx)
}

// GetDoc returns a documentation section by ID
func (s *CronService) GetDoc(ctx context.Context, id int64) (*model.CronDoc, error) {
	return s.repo.GetDoc(ctx, id)
}

// UpdateDoc updates a documentation section
func (s *CronService) UpdateDoc(ctx context.Context, doc *model.CronDoc) error {
	return s.repo.UpdateDoc(ctx, doc)
}

// CreateDoc creates a new documentation section
func (s *CronService) CreateDoc(ctx context.Context, doc *model.CronDoc) error {
	return s.repo.CreateDoc(ctx, doc)
}

// DeleteDoc deletes a documentation section
func (s *CronService) DeleteDoc(ctx context.Context, id int64) error {
	return s.repo.DeleteDoc(ctx, id)
}

func truncate(s string, maxLen int) string {
	if len(s) > maxLen {
		return s[:maxLen] + "..."
	}
	return s
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

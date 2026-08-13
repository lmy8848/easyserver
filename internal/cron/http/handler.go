package http

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"easyserver/internal/cron"
	"easyserver/internal/httpx"
	"easyserver/internal/httpx/middleware"
	"easyserver/internal/infra/apperror"
	"easyserver/internal/infra/executor"

	"github.com/gin-gonic/gin"
)

// CronHandler handles cron task API requests
type CronHandler struct {
	cronService *cron.Service
	executor    executor.CommandExecutor
	runner      *cron.ScriptRunner
}

// NewCronHandler creates a new CronHandler
func NewCronHandler(cronService *cron.Service, exec executor.CommandExecutor) *CronHandler {
	return &CronHandler{
		cronService: cronService,
		executor:    exec,
		runner:      cron.NewScriptRunner(exec),
	}
}

// ListTasks returns all cron tasks
func (h *CronHandler) ListTasks(c *gin.Context) {
	tasks, err := h.cronService.List(c.Request.Context())
	if err != nil {
		c.Error(apperror.WrapError(err))
		return
	}
	httpx.Success(c, tasks)
}

// GetTask returns a cron task by name
func (h *CronHandler) GetTask(c *gin.Context) {
	name := c.Param("name")
	task, err := h.cronService.Get(c.Request.Context(), name)
	if err != nil {
		c.Error(apperror.ErrNotFound.WithMessage("任务不存在"))
		return
	}
	httpx.Success(c, task)
}

// CreateTask creates a new cron task
func (h *CronHandler) CreateTask(c *gin.Context) {
	var req cron.CreateCronTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperror.ErrBadRequest.Wrap(err))
		return
	}

	middleware.AuditSummary(c, "创建定时任务 "+req.Name)

	// 名称：不允许换行/前缀注入，且需唯一
	if strings.ContainsAny(req.Name, "\r\n") {
		c.Error(apperror.ErrBadRequest.WithMessage("名称不允许包含换行符"))
		return
	}
	if err := checkTaskNameUnique(c.Request.Context(), h.cronService, req.Name, ""); err != nil {
		c.Error(err)
		return
	}

	// 调度表达式（OnCalendar），前端预设频率或手写均转为它
	onCalendar := strings.TrimSpace(req.Schedule)
	if onCalendar == "" {
		c.Error(apperror.ErrBadRequest.WithMessage("调度表达式不能为空"))
		return
	}
	if err := validateOnCalendar(h.executor, onCalendar); err != nil {
		c.Error(apperror.ErrBadRequest.WithMessage("无效的调度表达式: " + err.Error()))
		return
	}

	// Validate: command must be provided
	if req.Command == "" {
		c.Error(apperror.ErrBadRequest.WithMessage("必须提供执行命令"))
		return
	}
	// Validate timeout and retry bounds（0 = 不超时 / 不重试）
	if req.Timeout < 0 || req.Timeout > 86400 {
		c.Error(apperror.ErrBadRequest.WithMessage("超时时间必须在 0 到 86400 秒之间"))
		return
	}
	if req.MaxRetry < 0 || req.MaxRetry > 10 {
		c.Error(apperror.ErrBadRequest.WithMessage("最大重试次数必须在 0 到 10 之间"))
		return
	}
	if err := validateEnvVars(req.EnvVars); err != nil {
		c.Error(apperror.ErrBadRequest.Wrap(err))
		return
	}
	if err := validateWorkDir(req.WorkDir); err != nil {
		c.Error(apperror.ErrBadRequest.Wrap(err))
		return
	}

	task := &cron.CronTask{
		Name:        req.Name,
		Command:     req.Command,
		Schedule:    onCalendar,
		Description: req.Description,
		Persistent:  req.Persistent,
		Enabled:     true,
		Timeout:     req.Timeout,
		MaxRetry:    req.MaxRetry,
		EnvVars:     req.EnvVars,
		WorkDir:     req.WorkDir,
		Runtime:     req.Runtime,
	}

	if err := h.cronService.Create(c.Request.Context(), task); err != nil {
		c.Error(apperror.WrapError(err))
		return
	}
	httpx.Success(c, task)
}

// UpdateTask updates an existing cron task
func (h *CronHandler) UpdateTask(c *gin.Context) {
	name := c.Param("name")
	task, err := h.cronService.Get(c.Request.Context(), name)
	if err != nil {
		c.Error(apperror.ErrNotFound.WithMessage("任务不存在"))
		return
	}

	var req cron.UpdateCronTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperror.ErrBadRequest.Wrap(err))
		return
	}

	middleware.AuditSummary(c, "更新定时任务 "+task.Name)
	// Apply partial updates
	if req.Name != nil {
		if strings.ContainsAny(*req.Name, "\r\n") {
			c.Error(apperror.ErrBadRequest.WithMessage("名称不允许包含换行符"))
			return
		}
		if err := checkTaskNameUnique(c.Request.Context(), h.cronService, *req.Name, task.Name); err != nil {
			c.Error(err)
			return
		}
		task.Name = *req.Name
	}
	if req.Description != nil {
		task.Description = *req.Description
	}
	if req.Persistent != nil {
		task.Persistent = *req.Persistent
	}
	if req.Enabled != nil {
		task.Enabled = *req.Enabled
	}
	if req.Schedule != nil {
		onCalendar := strings.TrimSpace(*req.Schedule)
		if onCalendar == "" {
			c.Error(apperror.ErrBadRequest.WithMessage("调度表达式不能为空"))
			return
		}
		if err := validateOnCalendar(h.executor, onCalendar); err != nil {
			c.Error(apperror.ErrBadRequest.WithMessage("无效的调度表达式: " + err.Error()))
			return
		}
		task.Schedule = onCalendar
	}
	if req.Command != nil {
		task.Command = *req.Command
	}
	if req.Timeout != nil {
		if *req.Timeout < 0 || *req.Timeout > 86400 {
			c.Error(apperror.ErrBadRequest.WithMessage("超时时间必须在 0 到 86400 秒之间"))
			return
		}
		task.Timeout = *req.Timeout
	}
	if req.MaxRetry != nil {
		if *req.MaxRetry < 0 || *req.MaxRetry > 10 {
			c.Error(apperror.ErrBadRequest.WithMessage("最大重试次数必须在 0 到 10 之间"))
			return
		}
		task.MaxRetry = *req.MaxRetry
	}
	if req.EnvVars != nil {
		if err := validateEnvVars(*req.EnvVars); err != nil {
			c.Error(apperror.ErrBadRequest.Wrap(err))
			return
		}
		task.EnvVars = *req.EnvVars
	}
	if req.WorkDir != nil {
		if err := validateWorkDir(*req.WorkDir); err != nil {
			c.Error(apperror.ErrBadRequest.Wrap(err))
			return
		}
		task.WorkDir = *req.WorkDir
	}
	if req.Runtime != nil {
		task.Runtime = *req.Runtime
	}

	// Validate command required
	if task.Command == "" {
		c.Error(apperror.ErrBadRequest.WithMessage("必须提供执行命令"))
		return
	}

	if err := h.cronService.Update(c.Request.Context(), task); err != nil {
		c.Error(apperror.WrapError(err))
		return
	}
	httpx.Success(c, task)
}

// DeleteTask deletes a cron task
func (h *CronHandler) DeleteTask(c *gin.Context) {
	name := c.Param("name")
	task, err := h.cronService.Get(c.Request.Context(), name)
	if err != nil {
		c.Error(apperror.ErrNotFound.WithMessage("任务不存在"))
		return
	}
	middleware.AuditSummary(c, "删除定时任务 "+task.Name)
	if err := h.cronService.Delete(c.Request.Context(), name); err != nil {
		c.Error(apperror.WrapError(err))
		return
	}
	httpx.Success(c, gin.H{"message": "任务已删除"})
}

// EnableTask enables a cron task
func (h *CronHandler) EnableTask(c *gin.Context) {
	name := c.Param("name")
	task, err := h.cronService.Get(c.Request.Context(), name)
	if err != nil {
		c.Error(apperror.ErrNotFound.WithMessage("任务不存在"))
		return
	}
	middleware.AuditSummary(c, "启用定时任务 "+task.Name)
	if err := h.cronService.Enable(c.Request.Context(), name); err != nil {
		c.Error(apperror.WrapError(err))
		return
	}
	httpx.Success(c, gin.H{"message": "任务已启用"})
}

// DisableTask disables a cron task
func (h *CronHandler) DisableTask(c *gin.Context) {
	name := c.Param("name")
	task, err := h.cronService.Get(c.Request.Context(), name)
	if err != nil {
		c.Error(apperror.ErrNotFound.WithMessage("任务不存在"))
		return
	}
	middleware.AuditSummary(c, "禁用定时任务 "+task.Name)
	if err := h.cronService.Disable(c.Request.Context(), name); err != nil {
		c.Error(apperror.WrapError(err))
		return
	}
	httpx.Success(c, gin.H{"message": "任务已禁用"})
}

// RunTask executes a cron task immediately
func (h *CronHandler) RunTask(c *gin.Context) {
	name := c.Param("name")
	task, err := h.cronService.Get(c.Request.Context(), name)
	if err != nil {
		c.Error(apperror.ErrNotFound.WithMessage("任务不存在"))
		return
	}
	middleware.AuditSummary(c, "立即执行定时任务 "+task.Name)
	if err := h.cronService.RunNow(c.Request.Context(), name); err != nil {
		c.Error(apperror.WrapError(err))
		return
	}
	httpx.Success(c, gin.H{"message": "任务已执行"})
}

// GetTaskRuns returns execution runs (grouped by invocation) for a cron task
func (h *CronHandler) GetTaskRuns(c *gin.Context) {
	name := c.Param("name")
	limit := 50
	if l := c.Query("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 500 {
			limit = parsed
		}
	}
	runs, err := h.cronService.GetRuns(c.Request.Context(), name, limit)
	if err != nil {
		c.Error(apperror.WrapError(err))
		return
	}
	httpx.Success(c, runs)
}

// ListScripts returns all scripts
func (h *CronHandler) ListScripts(c *gin.Context) {
	scripts, err := h.cronService.ListScripts(c.Request.Context())
	if err != nil {
		c.Error(apperror.WrapError(err))
		return
	}
	httpx.Success(c, scripts)
}

// GetScript returns a script by ID
func (h *CronHandler) GetScript(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.Error(apperror.ErrBadRequest.WithMessage("无效的脚本ID"))
		return
	}
	script, err := h.cronService.GetScript(c.Request.Context(), id)
	if err != nil {
		c.Error(apperror.ErrNotFound.WithMessage("脚本不存在"))
		return
	}
	httpx.Success(c, script)
}

// ScriptLogs 是 /cron/scripts/:id/logs 的总入口：带 ?stream=1 走实时 SSE，否则返回历史日志 REST。
func (h *CronHandler) ScriptLogs(c *gin.Context) {
	if c.Query("stream") == "1" {
		h.RunScriptSSE(c)
		return
	}
	h.GetScriptLogs(c)
}

// GetScriptLogs 返回脚本的历史执行日志（journald，identifier=easyserver-script-<name>）。
// 日志按脚本存（跨多次执行），返回最近 limit 条，供刷新后回看。
func (h *CronHandler) GetScriptLogs(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		c.Error(apperror.ErrBadRequest.WithMessage("无效的脚本ID"))
		return
	}
	script, err := h.cronService.GetScript(c.Request.Context(), id)
	if err != nil {
		c.Error(apperror.ErrNotFound.WithMessage("脚本不存在"))
		return
	}

	limit := 200
	if l := c.Query("limit"); l != "" {
		if parsed, aErr := strconv.Atoi(l); aErr == nil && parsed > 0 && parsed <= 1000 {
			limit = parsed
		}
	}

	args := []string{
		"--identifier=" + "easyserver-script-" + script.Name,
		"--no-pager",
		"--output=json",
		"-n", strconv.Itoa(limit),
	}
	stdout, _, exitCode, err := h.executor.Run(c.Request.Context(), "journalctl", args...)
	if err != nil || exitCode != 0 {
		c.Error(apperror.ErrInternal.WithMessage("读取历史日志失败: " + stdout))
		return
	}

	logs := parseScriptJournalLogs(stdout)
	httpx.Success(c, logs)
}

// parseScriptJournalLogs 解析 journalctl JSON 输出为 ScriptLogLine 列表。
// journald 不区分 stdout/stderr，统一 stream 为 stdout。
func parseScriptJournalLogs(stdout string) []cron.ScriptLogLine {
	var logs []cron.ScriptLogLine
	for line := range strings.SplitSeq(stdout, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var entry struct {
			Message           string `json:"MESSAGE"`
			RealtimeTimestamp string `json:"__REALTIME_TIMESTAMP"`
		}
		if err := json.Unmarshal([]byte(line), &entry); err != nil || entry.Message == "" {
			continue
		}
		logs = append(logs, cron.ScriptLogLine{
			Stream:  "stdout",
			Message: entry.Message,
			Time:    formatJournalTimestamp(entry.RealtimeTimestamp),
		})
	}
	return logs
}

// formatJournalTimestamp 把 journald 微秒级 __REALTIME_TIMESTAMP 转成 "2006-01-02 15:04:05"。
func formatJournalTimestamp(realtime string) string {
	if realtime == "" {
		return time.Now().Format("2006-01-02 15:04:05")
	}
	var usec int64
	if _, err := fmt.Sscanf(realtime, "%d", &usec); err != nil {
		return time.Now().Format("2006-01-02 15:04:05")
	}
	return time.Unix(usec/1e6, (usec%1e6)*1000).Format("2006-01-02 15:04:05")
}

// CreateScript creates a new script
func (h *CronHandler) CreateScript(c *gin.Context) {
	var req cron.CreateScriptRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperror.ErrBadRequest.Wrap(err))
		return
	}

	middleware.AuditSummary(c, "创建脚本 "+req.Name)
	if strings.TrimSpace(req.Content) == "" {
		c.Error(apperror.ErrBadRequest.WithMessage("脚本内容不能为空"))
		return
	}

	existingScripts, err := h.cronService.ListScripts(c.Request.Context())
	if err != nil {
		c.Error(apperror.ErrInternal.WithMessage("检查脚本名称失败: " + err.Error()))
		return
	}
	for _, s := range existingScripts {
		if s.Name == req.Name {
			c.Error(apperror.ErrBadRequest.WithMessage("脚本名称已存在"))
			return
		}
	}

	script := &cron.Script{
		Name:        req.Name,
		Description: req.Description,
		Content:     req.Content,
	}
	if err := h.cronService.CreateScript(c.Request.Context(), script); err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			c.Error(apperror.ErrConflict.WithMessage("脚本名称已存在"))
			return
		}
		c.Error(apperror.WrapError(err))
		return
	}
	httpx.Success(c, script)
}

// UpdateScript updates an existing script
func (h *CronHandler) UpdateScript(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.Error(apperror.ErrBadRequest.WithMessage("无效的脚本ID"))
		return
	}
	script, err := h.cronService.GetScript(c.Request.Context(), id)
	if err != nil {
		c.Error(apperror.ErrNotFound.WithMessage("脚本不存在"))
		return
	}

	var req cron.UpdateScriptRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperror.ErrBadRequest.Wrap(err))
		return
	}

	middleware.AuditSummary(c, "更新脚本 "+script.Name)
	if req.Name != nil {
		existingScripts, listErr := h.cronService.ListScripts(c.Request.Context())
		if listErr != nil {
			c.Error(apperror.ErrInternal.WithMessage("检查脚本名称失败: " + listErr.Error()))
			return
		}
		for _, s := range existingScripts {
			if s.ID != id && s.Name == *req.Name {
				c.Error(apperror.ErrBadRequest.WithMessage("脚本名称已存在"))
				return
			}
		}
		script.Name = *req.Name
	}
	if req.Description != nil {
		script.Description = *req.Description
	}
	if req.Content != nil {
		if strings.TrimSpace(*req.Content) == "" {
			c.Error(apperror.ErrBadRequest.WithMessage("脚本内容不能为空"))
			return
		}
		script.Content = *req.Content
	}

	if err := h.cronService.UpdateScript(c.Request.Context(), script); err != nil {
		c.Error(apperror.WrapError(err))
		return
	}
	httpx.Success(c, script)
}

// DeleteScript deletes a script
func (h *CronHandler) DeleteScript(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.Error(apperror.ErrBadRequest.WithMessage("无效的脚本ID"))
		return
	}
	script, err := h.cronService.GetScript(c.Request.Context(), id)
	if err != nil {
		c.Error(apperror.ErrNotFound.WithMessage("脚本不存在"))
		return
	}
	middleware.AuditSummary(c, "删除脚本 "+script.Name)
	// Check for dependent tasks
	tasks, listErr := h.cronService.List(c.Request.Context())
	if listErr != nil {
		c.Error(apperror.ErrInternal.WithMessage("检查依赖任务失败: " + listErr.Error()))
		return
	}
	for _, t := range tasks {
		if t.Command == cron.ScriptPath(id) {
			c.Error(apperror.ErrConflict.WithMessage(fmt.Sprintf("脚本被任务 '%s' 使用，请先移除引用", t.Name)))
			return
		}
	}
	if err := h.cronService.DeleteScript(c.Request.Context(), id); err != nil {
		c.Error(apperror.WrapError(err))
		return
	}
	httpx.Success(c, gin.H{"message": "脚本已删除"})
}

// RunScriptSSE 通过 Server-Sent Events 单向推送运行中脚本的实时 stdout/stderr。
// 启动/停止已独立为 REST（POST /cron/scripts/:id/run、/stop），此处**只订阅、不启动**。
// 脚本未运行时返回 404，前端自行决定是否先调 REST 启动。
func (h *CronHandler) RunScriptSSE(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		c.Error(apperror.ErrBadRequest.WithMessage("无效的脚本ID"))
		return
	}
	if _, err := h.cronService.GetScript(c.Request.Context(), id); err != nil {
		c.Error(apperror.ErrNotFound.WithMessage("脚本不存在"))
		return
	}
	rs, ok := h.runner.Get(id)
	if !ok {
		c.Error(apperror.ErrNotFound.WithMessage("脚本未在运行"))
		return
	}

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		c.Error(apperror.ErrInternal.WithMessage("当前连接不支持流式输出"))
		return
	}
	// 连接建立即 flush 一次，确保客户端立即收到响应头。
	fmt.Fprint(c.Writer, ": connected\n\n")
	flusher.Flush()

	sub, cancel := rs.Subscribe()
	defer cancel() // 只注销订阅，不 Kill 进程

	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	doneCh := rs.Done()
	for {
		select {
		case msg := <-sub:
			if _, err := fmt.Fprintf(c.Writer, "data: %s\n\n", msg); err != nil {
				return
			}
			flusher.Flush()
		case <-doneCh:
			// 脚本已退出，发退出码。
			exitCode := 0
			if cmd := rs.Cmd(); cmd != nil && cmd.ProcessState != nil {
				exitCode = cmd.ProcessState.ExitCode()
			}
			exitMsg, _ := json.Marshal(map[string]any{
				"type": "exit",
				"code": exitCode,
			})
			if _, err := fmt.Fprintf(c.Writer, "data: %s\n\n", exitMsg); err != nil {
				return
			}
			flusher.Flush()
			return
		case <-ticker.C:
			// 心跳注释行，避免连接被中间层空闲断开。
			if _, err := fmt.Fprint(c.Writer, ": ping\n\n"); err != nil {
				return
			}
			flusher.Flush()
		case <-c.Request.Context().Done():
			// 客户端断开（fetch abort）时取消 request context。
			return
		}
	}
}

// RunScript 启动一个脚本执行（独立于 WS 订阅）。单实例：已运行则复用，不重复启动。
func (h *CronHandler) RunScript(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		c.Error(apperror.ErrBadRequest.WithMessage("无效的脚本ID"))
		return
	}
	script, err := h.cronService.GetScript(c.Request.Context(), id)
	if err != nil {
		c.Error(apperror.ErrNotFound.WithMessage("脚本不存在"))
		return
	}
	middleware.AuditSummary(c, "执行脚本 "+script.Name)
	if _, err := h.runner.Start(script); err != nil {
		c.Error(apperror.ErrInternal.WithMessage("启动脚本失败: " + err.Error()))
		return
	}
	httpx.Success(c, gin.H{"message": "已启动"})
}

// GetRunningScripts 返回运行中脚本 id 列表，供前端显示「运行中」标记。
func (h *CronHandler) GetRunningScripts(c *gin.Context) {
	httpx.Success(c, h.runner.RunningIDs())
}

// StopScript 停止一个正在运行的脚本（列表上的「停止」按钮调用）。
func (h *CronHandler) StopScript(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		c.Error(apperror.ErrBadRequest.WithMessage("无效的脚本ID"))
		return
	}
	if _, ok := h.runner.Get(id); !ok {
		c.Error(apperror.ErrNotFound.WithMessage("脚本未在运行"))
		return
	}
	middleware.AuditSummary(c, "停止脚本执行")
	h.runner.Stop(id)
	httpx.Success(c, gin.H{"message": "已停止"})
}

// checkTaskNameUnique 校验任务名唯一（排除 excludeName，用于编辑场景）。
func checkTaskNameUnique(ctx context.Context, svc *cron.Service, name, excludeName string) error {
	existing, err := svc.List(ctx)
	if err != nil {
		return apperror.ErrInternal.WithMessage("检查任务名称失败: " + err.Error())
	}
	for _, t := range existing {
		if t.Name == name && t.Name != excludeName {
			return apperror.ErrBadRequest.WithMessage("任务名称已存在")
		}
	}
	return nil
}

// validateEnvVars 校验环境变量格式：每行非空须为 KEY=VALUE，且 KEY 非空。
func validateEnvVars(envStr string) error {
	for line := range strings.SplitSeq(envStr, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		k, _, ok := strings.Cut(line, "=")
		if !ok || strings.TrimSpace(k) == "" {
			return fmt.Errorf("环境变量 %q 格式错误，应为 KEY=VALUE", line)
		}
	}
	return nil
}

// validateWorkDir 校验工作目录：提供时须为绝对路径。
func validateWorkDir(dir string) error {
	if dir == "" {
		return nil
	}
	if !filepath.IsAbs(dir) {
		return errors.New("工作目录必须是绝对路径")
	}
	return nil
}

// validateOnCalendar 用 systemd-analyze calendar 校验 OnCalendar 表达式。
func validateOnCalendar(exec executor.CommandExecutor, expr string) error {
	_, _, exitCode, err := exec.Run(context.Background(), "systemd-analyze", "calendar", expr)
	if err != nil || exitCode != 0 {
		return fmt.Errorf("systemd 无法解析 %q", expr)
	}
	return nil
}

func RegisterRoutes(protected *gin.RouterGroup, wsGroup *gin.RouterGroup, cronService *cron.Service, exec executor.CommandExecutor) {
	handler := NewCronHandler(cronService, exec)

	protected.GET("/cron/tasks", handler.ListTasks)
	protected.POST("/cron/tasks", handler.CreateTask)
	protected.GET("/cron/tasks/:name", handler.GetTask)
	protected.PUT("/cron/tasks/:name", handler.UpdateTask)
	protected.DELETE("/cron/tasks/:name", handler.DeleteTask)
	protected.POST("/cron/tasks/:name/enable", handler.EnableTask)
	protected.POST("/cron/tasks/:name/disable", handler.DisableTask)
	protected.POST("/cron/tasks/:name/run", handler.RunTask)
	protected.GET("/cron/tasks/:name/runs", handler.GetTaskRuns)
	protected.GET("/cron/scripts", handler.ListScripts)
	protected.POST("/cron/scripts", handler.CreateScript)
	protected.GET("/cron/scripts/running", handler.GetRunningScripts)
	protected.POST("/cron/scripts/:id/run", handler.RunScript)
	protected.GET("/cron/scripts/:id", handler.GetScript)
	protected.PUT("/cron/scripts/:id", handler.UpdateScript)
	protected.DELETE("/cron/scripts/:id", handler.DeleteScript)
	protected.POST("/cron/scripts/:id/stop", handler.StopScript)

	protected.GET("/cron/scripts/:id/logs", handler.ScriptLogs)
}

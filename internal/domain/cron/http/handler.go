package http

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"easyserver/internal/domain/cron"
	"easyserver/internal/httpx"
	"easyserver/internal/httpx/middleware"
	"easyserver/internal/infra/errx"
	"easyserver/internal/util"

	"github.com/gin-gonic/gin"
)

// CronHandler handles cron task API requests
type CronHandler struct {
	cronService *cron.Service
	runner      *cron.ScriptRunner
}

// NewCronHandler creates a new CronHandler
func NewCronHandler(cronService *cron.Service) *CronHandler {
	return &CronHandler{
		cronService: cronService,
		runner:      cron.NewScriptRunner(),
	}
}

// ListTasks returns all cron tasks
func (h *CronHandler) ListTasks(c *gin.Context) (any, error) {
	tasks, err := h.cronService.List(c.Request.Context())
	if err != nil {
		return nil, err
	}
	return httpx.Paginate(tasks, httpx.ParsePagination(c, 50, 200)), nil
}

// GetTask returns a cron task by name
func (h *CronHandler) GetTask(c *gin.Context) (any, error) {
	name := c.Param("name")
	task, err := h.cronService.Get(c.Request.Context(), name)
	if err != nil {
		return nil, errx.NotFound("任务不存在")
	}
	return task, nil
}

// CreateTask creates a new cron task
func (h *CronHandler) CreateTask(c *gin.Context) (any, error) {
	var req cron.CreateCronTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		return nil, errx.BadRequest("invalid request: %w", err)
	}

	middleware.AuditSummary(c, "创建定时任务 "+req.Name)

	// 名称：不允许换行/前缀注入，且需唯一
	if strings.ContainsAny(req.Name, "\r\n") {
		return nil, errx.BadRequest("名称不允许包含换行符")
	}
	if err := checkTaskNameUnique(c.Request.Context(), h.cronService, req.Name, ""); err != nil {
		return nil, err
	}

	// 调度表达式（OnCalendar），前端预设频率或手写均转为它
	onCalendar := strings.TrimSpace(req.Schedule)
	if onCalendar == "" {
		return nil, errx.BadRequest("调度表达式不能为空")
	}
	if err := validateOnCalendar(onCalendar); err != nil {
		return nil, errx.BadRequest("无效的调度表达式: %w", err)
	}

	// Validate: command must be provided
	if req.Command == "" {
		return nil, errx.BadRequest("必须提供执行命令")
	}
	// Validate timeout and retry bounds（0 = 不超时 / 不重试）
	if req.Timeout < 0 || req.Timeout > 86400 {
		return nil, errx.BadRequest("超时时间必须在 0 到 86400 秒之间")
	}
	if req.MaxRetry < 0 || req.MaxRetry > 10 {
		return nil, errx.BadRequest("最大重试次数必须在 0 到 10 之间")
	}
	if err := validateEnvVars(req.EnvVars); err != nil {
		return nil, errx.BadRequest("%w", err)
	}
	if err := validateWorkDir(req.WorkDir); err != nil {
		return nil, errx.BadRequest("%w", err)
	}

	task := &cron.CronTask{
		Name:        req.Name,
		Description: req.Description,
		Schedule:    onCalendar,
		Persistent:  req.Persistent,
		Command:     req.Command,
		WorkDir:     req.WorkDir,
		Runtime:     req.Runtime,
		EnvVars:     req.EnvVars,
		Timeout:     req.Timeout,
		MaxRetry:    req.MaxRetry,
		Enabled:     true,
	}

	if err := h.cronService.Create(c.Request.Context(), task); err != nil {
		return nil, err
	}

	return task, nil
}

// UpdateTask updates an existing cron task
func (h *CronHandler) UpdateTask(c *gin.Context) (any, error) {
	name := c.Param("name")
	oldTask, err := h.cronService.Get(c.Request.Context(), name)
	if err != nil {
		return nil, errx.NotFound("任务不存在")
	}

	var req cron.UpdateCronTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		return nil, errx.BadRequest("invalid request: %w", err)
	}

	middleware.AuditSummary(c, "更新定时任务 "+name)

	// Validate name if changing
	if req.Name != nil && *req.Name != name {
		if strings.ContainsAny(*req.Name, "\r\n") {
			return nil, errx.BadRequest("名称不允许包含换行符")
		}
		if err := checkTaskNameUnique(c.Request.Context(), h.cronService, *req.Name, name); err != nil {
			return nil, err
		}
	}

	task := *oldTask

	if req.Name != nil {
		task.Name = *req.Name
	}
	if req.Description != nil {
		task.Description = *req.Description
	}
	if req.Schedule != nil {
		onCalendar := strings.TrimSpace(*req.Schedule)
		if onCalendar == "" {
			return nil, errx.BadRequest("调度表达式不能为空")
		}
		if err := validateOnCalendar(onCalendar); err != nil {
			return nil, errx.BadRequest("无效的调度表达式: %w", err)
		}
		task.Schedule = onCalendar
	}
	if req.Persistent != nil {
		task.Persistent = *req.Persistent
	}
	if req.Enabled != nil {
		task.Enabled = *req.Enabled
	}
	if req.Command != nil {
		task.Command = *req.Command
	}
	if req.Timeout != nil {
		if *req.Timeout < 0 || *req.Timeout > 86400 {
			return nil, errx.BadRequest("超时时间必须在 0 到 86400 秒之间")
		}
		task.Timeout = *req.Timeout
	}
	if req.MaxRetry != nil {
		if *req.MaxRetry < 0 || *req.MaxRetry > 10 {
			return nil, errx.BadRequest("最大重试次数必须在 0 到 10 之间")
		}
		task.MaxRetry = *req.MaxRetry
	}
	if req.EnvVars != nil {
		if err := validateEnvVars(*req.EnvVars); err != nil {
			return nil, errx.BadRequest("%w", err)
		}
		task.EnvVars = *req.EnvVars
	}
	if req.WorkDir != nil {
		if err := validateWorkDir(*req.WorkDir); err != nil {
			return nil, errx.BadRequest("%w", err)
		}
		task.WorkDir = *req.WorkDir
	}
	if req.Runtime != nil {
		task.Runtime = *req.Runtime
	}

	if task.Command == "" {
		return nil, errx.BadRequest("必须提供执行命令")
	}

	if err := h.cronService.Update(c.Request.Context(), &task); err != nil {
		return nil, err
	}

	return task, nil
}

// DeleteTask deletes a cron task
func (h *CronHandler) DeleteTask(c *gin.Context) (any, error) {
	name := c.Param("name")
	if _, err := h.cronService.Get(c.Request.Context(), name); err != nil {
		return nil, errx.NotFound("任务不存在")
	}
	middleware.AuditSummary(c, "删除定时任务 "+name)
	if err := h.cronService.Delete(c.Request.Context(), name); err != nil {
		return nil, err
	}
	return gin.H{"message": "任务已删除"}, nil
}

// EnableTask enables a cron task
func (h *CronHandler) EnableTask(c *gin.Context) (any, error) {
	name := c.Param("name")
	if _, err := h.cronService.Get(c.Request.Context(), name); err != nil {
		return nil, errx.NotFound("任务不存在")
	}
	middleware.AuditSummary(c, "启用定时任务 "+name)
	if err := h.cronService.Enable(c.Request.Context(), name); err != nil {
		return nil, err
	}
	return gin.H{"message": "任务已启用"}, nil
}

// DisableTask disables a cron task
func (h *CronHandler) DisableTask(c *gin.Context) (any, error) {
	name := c.Param("name")
	if _, err := h.cronService.Get(c.Request.Context(), name); err != nil {
		return nil, errx.NotFound("任务不存在")
	}
	middleware.AuditSummary(c, "禁用定时任务 "+name)
	if err := h.cronService.Disable(c.Request.Context(), name); err != nil {
		return nil, err
	}
	return gin.H{"message": "任务已禁用"}, nil
}

// RunTask manually triggers a cron task
func (h *CronHandler) RunTask(c *gin.Context) (any, error) {
	name := c.Param("name")
	if _, err := h.cronService.Get(c.Request.Context(), name); err != nil {
		return nil, errx.NotFound("任务不存在")
	}
	middleware.AuditSummary(c, "手动触发定时任务 "+name)
	if err := h.cronService.RunNow(c.Request.Context(), name); err != nil {
		return nil, err
	}
	return gin.H{"message": "任务已触发"}, nil
}

// GetTaskRuns returns execution history for a task
func (h *CronHandler) GetTaskRuns(c *gin.Context) (any, error) {
	name := c.Param("name")
	since := c.Query("since")
	until := c.Query("until")
	p := httpx.ParsePagination(c, 20, 100)
	runs, err := h.cronService.GetRuns(c.Request.Context(), name, 2000, since, until)
	if err != nil {
		return nil, err
	}
	return httpx.Paginate(runs, p), nil
}

// ========== Script Management (独立脚本库) ==========

// ListScripts returns all saved scripts
func (h *CronHandler) ListScripts(c *gin.Context) (any, error) {
	scripts, err := h.cronService.ListScripts(c.Request.Context())
	if err != nil {
		return nil, err
	}
	return httpx.Paginate(scripts, httpx.ParsePagination(c, 50, 200)), nil
}

// GetScript returns a script by ID
func (h *CronHandler) GetScript(c *gin.Context) (any, error) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		return nil, errx.BadRequest("无效的脚本ID")
	}
	script, err := h.cronService.GetScript(c.Request.Context(), id)
	if err != nil {
		return nil, errx.NotFound("脚本不存在")
	}
	return script, nil
}

// ScriptLogs 是 /cron/scripts/:id/logs 的总入口：带 ?stream=1 走实时 SSE，否则返回历史日志 REST。
func (h *CronHandler) ScriptLogs(c *gin.Context) (any, error) {
	if c.Query("stream") == "1" {
		return h.RunScriptSSE(c)
	}
	return h.GetScriptLogs(c)
}

// GetScriptLogs 返回脚本的历史执行日志（journald，identifier=easyserver-script-<name>）。
// 日志按脚本存（跨多次执行），返回最近 limit 条，供刷新后回看。
func (h *CronHandler) GetScriptLogs(c *gin.Context) (any, error) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		return nil, errx.BadRequest("无效的脚本ID")
	}
	script, err := h.cronService.GetScript(c.Request.Context(), id)
	if err != nil {
		return nil, errx.NotFound("脚本不存在")
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
	stdout, err := exec.CommandContext(c.Request.Context(), "journalctl", args...).Output()
	if err != nil {
		return nil, errx.Internal("读取历史日志失败: %s", string(stdout))
	}

	logs := parseScriptJournalLogs(string(stdout))
	return logs, nil
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

// formatJournalTimestamp 把 journald 微秒级 __REALTIME_TIMESTAMP 转成 util.TimeLayout。
func formatJournalTimestamp(realtime string) string {
	var usec int64
	if _, err := fmt.Sscanf(realtime, "%d", &usec); err == nil {
		return util.UnixMicros(usec).Format(util.TimeLayout)
	}
	return time.Now().Format(util.TimeLayout)
}

// CreateScript creates a new script
func (h *CronHandler) CreateScript(c *gin.Context) (any, error) {
	var req struct {
		Name        string `json:"name" binding:"required"`
		Content     string `json:"content"`
		Description string `json:"description"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		return nil, errx.BadRequest("invalid request: %w", err)
	}

	middleware.AuditSummary(c, "创建脚本 "+req.Name)

	if strings.TrimSpace(req.Content) == "" {
		return nil, errx.BadRequest("脚本内容不能为空")
	}

	// Check name unique
	existing, err := h.cronService.ListScripts(c.Request.Context())
	if err != nil {
		return nil, errx.Internal("检查脚本名称失败: %w", err)
	}
	for _, s := range existing {
		if s.Name == req.Name {
			return nil, errx.Conflict("脚本名称已存在")
		}
	}

	script := &cron.Script{
		Name:        req.Name,
		Content:     req.Content,
		Description: req.Description,
	}

	if err := h.cronService.CreateScript(c.Request.Context(), script); err != nil {
		if strings.Contains(err.Error(), "UNIQUE") || strings.Contains(err.Error(), "already exists") {
			return nil, errx.Conflict("脚本名称已存在")
		}
		return nil, err
	}

	return script, nil
}

// UpdateScript updates an existing script
func (h *CronHandler) UpdateScript(c *gin.Context) (any, error) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		return nil, errx.BadRequest("无效的脚本ID")
	}
	oldScript, err := h.cronService.GetScript(c.Request.Context(), id)
	if err != nil {
		return nil, errx.NotFound("脚本不存在")
	}

	var req struct {
		Name        *string `json:"name"`
		Content     *string `json:"content"`
		Description *string `json:"description"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		return nil, errx.BadRequest("invalid request: %w", err)
	}

	middleware.AuditSummary(c, "更新脚本 "+oldScript.Name)

	// If name changes, check uniqueness
	if req.Name != nil && *req.Name != oldScript.Name {
		existing, listErr := h.cronService.ListScripts(c.Request.Context())
		if listErr != nil {
			return nil, errx.Internal("检查脚本名称失败: %w", listErr)
		}
		for _, s := range existing {
			if s.Name == *req.Name && s.ID != id {
				return nil, errx.BadRequest("脚本名称已存在")
			}
		}
	}

	script := *oldScript
	if req.Name != nil {
		script.Name = *req.Name
	}
	if req.Content != nil {
		if strings.TrimSpace(*req.Content) == "" {
			return nil, errx.BadRequest("脚本内容不能为空")
		}
		script.Content = *req.Content
	}
	if req.Description != nil {
		script.Description = *req.Description
	}

	if err := h.cronService.UpdateScript(c.Request.Context(), &script); err != nil {
		return nil, err
	}

	return script, nil
}

// DeleteScript deletes a script
func (h *CronHandler) DeleteScript(c *gin.Context) (any, error) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		return nil, errx.BadRequest("无效的脚本ID")
	}
	script, err := h.cronService.GetScript(c.Request.Context(), id)
	if err != nil {
		return nil, errx.NotFound("脚本不存在")
	}
	middleware.AuditSummary(c, "删除脚本 "+script.Name)
	if err := h.cronService.DeleteScript(c.Request.Context(), id); err != nil {
		return nil, err
	}
	return gin.H{"message": "脚本已删除"}, nil
}

// RunScriptSSE 通过 Server-Sent Events 单向推送脚本实时输出。
// 客户端 fetch /cron/scripts/:id/logs?stream=1 建立长连接，服务端按行 push。
// 仅订阅当前正在运行的脚本输出；若脚本未在运行，返回 404 让前端回落到历史日志。
func (h *CronHandler) RunScriptSSE(c *gin.Context) (any, error) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		return nil, errx.BadRequest("无效的脚本ID")
	}
	if _, err := h.cronService.GetScript(c.Request.Context(), id); err != nil {
		return nil, errx.NotFound("脚本不存在")
	}
	rs, ok := h.runner.Get(id)
	if !ok {
		return nil, errx.NotFound("脚本未在运行")
	}

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		return nil, errx.Internal("当前连接不支持流式输出")
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
				return nil, nil
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
				return nil, nil
			}
			flusher.Flush()
			return nil, nil
		case <-ticker.C:
			// 心跳注释行，避免连接被中间层空闲断开。
			if _, err := fmt.Fprint(c.Writer, ": ping\n\n"); err != nil {
				return nil, nil
			}
			flusher.Flush()
		case <-c.Request.Context().Done():
			// 客户端断开（fetch abort）时取消 request context。
			return nil, nil
		}
	}
}

// RunScript 启动一个脚本执行（独立于 WS 订阅）。单实例：已运行则复用，不重复启动。
func (h *CronHandler) RunScript(c *gin.Context) (any, error) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		return nil, errx.BadRequest("无效的脚本ID")
	}
	script, err := h.cronService.GetScript(c.Request.Context(), id)
	if err != nil {
		return nil, errx.NotFound("脚本不存在")
	}
	middleware.AuditSummary(c, "执行脚本 "+script.Name)
	if _, err := h.runner.Start(script); err != nil {
		return nil, errx.Internal("启动脚本失败: %w", err)
	}
	return gin.H{"message": "已启动"}, nil
}

// GetRunningScripts 返回运行中脚本 id 列表，供前端显示「运行中」标记。
func (h *CronHandler) GetRunningScripts(c *gin.Context) (any, error) {
	return h.runner.RunningIDs(), nil
}

// StopScript 停止一个正在运行的脚本（列表上的「停止」按钮调用）。
func (h *CronHandler) StopScript(c *gin.Context) (any, error) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		return nil, errx.BadRequest("无效的脚本ID")
	}
	if _, err := h.cronService.GetScript(c.Request.Context(), id); err != nil {
		return nil, errx.NotFound("脚本不存在")
	}
	if _, ok := h.runner.Get(id); !ok {
		return nil, errx.NotFound("脚本未在运行")
	}
	middleware.AuditSummary(c, "停止脚本执行")
	h.runner.Stop(id)
	return gin.H{"message": "已停止"}, nil
}

// checkTaskNameUnique 校验任务名唯一（排除 excludeName，用于编辑场景）。
func checkTaskNameUnique(ctx context.Context, svc *cron.Service, name, excludeName string) error {
	existing, err := svc.List(ctx)
	if err != nil {
		return errx.Internal("检查任务名称失败: %w", err)
	}
	for _, t := range existing {
		if t.Name == name && t.Name != excludeName {
			return errx.BadRequest("任务名称已存在")
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
func validateOnCalendar(expr string) error {
	_, err := exec.CommandContext(context.Background(), "systemd-analyze", "calendar", expr).Output()
	if err != nil {
		return fmt.Errorf("systemd 无法解析 %q", expr)
	}
	return nil
}

func RegisterRoutes(protected *gin.RouterGroup, wsGroup *gin.RouterGroup, cronService *cron.Service) {
	handler := NewCronHandler(cronService)

	protected.GET("/cron/tasks", httpx.H(handler.ListTasks))
	protected.POST("/cron/tasks", httpx.H(handler.CreateTask))
	protected.GET("/cron/tasks/:name", httpx.H(handler.GetTask))
	protected.PUT("/cron/tasks/:name", httpx.H(handler.UpdateTask))
	protected.DELETE("/cron/tasks/:name", httpx.H(handler.DeleteTask))
	protected.POST("/cron/tasks/:name/enable", httpx.H(handler.EnableTask))
	protected.POST("/cron/tasks/:name/disable", httpx.H(handler.DisableTask))
	protected.POST("/cron/tasks/:name/run", httpx.H(handler.RunTask))
	protected.GET("/cron/tasks/:name/runs", httpx.H(handler.GetTaskRuns))
	protected.GET("/cron/scripts", httpx.H(handler.ListScripts))
	protected.POST("/cron/scripts", httpx.H(handler.CreateScript))
	protected.GET("/cron/scripts/running", httpx.H(handler.GetRunningScripts))
	protected.POST("/cron/scripts/:id/run", httpx.H(handler.RunScript))
	protected.GET("/cron/scripts/:id", httpx.H(handler.GetScript))
	protected.PUT("/cron/scripts/:id", httpx.H(handler.UpdateScript))
	protected.DELETE("/cron/scripts/:id", httpx.H(handler.DeleteScript))
	protected.POST("/cron/scripts/:id/stop", httpx.H(handler.StopScript))

	protected.GET("/cron/scripts/:id/logs", httpx.H(handler.ScriptLogs))
}

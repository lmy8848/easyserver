package api

import (
	"context"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"easyserver/internal/cron"
	"easyserver/internal/httpx/middleware"
	"easyserver/internal/infra/executor"

	"github.com/gin-gonic/gin"
)

// CronHandler handles cron task API requests
type CronHandler struct {
	cronService *cron.Service
	executor    executor.CommandExecutor
}

// NewCronHandler creates a new CronHandler
func NewCronHandler(cronService *cron.Service, exec executor.CommandExecutor) *CronHandler {
	return &CronHandler{cronService: cronService, executor: exec}
}

// ListTasks returns all cron tasks
func (h *CronHandler) ListTasks(c *gin.Context) {
	tasks, err := h.cronService.List(c.Request.Context())
	if err != nil {
		c.Error(WrapError(err))
		return
	}
	Success(c, tasks)
}

// GetTask returns a cron task by ID
func (h *CronHandler) GetTask(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.Error(ErrBadRequest.WithMessage("无效的任务ID"))
		return
	}
	task, err := h.cronService.Get(c.Request.Context(), id)
	if err != nil {
		c.Error(ErrNotFound.WithMessage("任务不存在"))
		return
	}
	Success(c, task)
}

// CreateTask creates a new cron task
func (h *CronHandler) CreateTask(c *gin.Context) {
	var req cron.CreateCronTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(ErrBadRequest.Wrap(err))
		return
	}

	middleware.AuditSummary(c, "创建定时任务 "+req.Name)
	// Validate cron expression (5 fields: minute hour day month weekday)
	parts := strings.Fields(req.Schedule)
	if len(parts) != 5 {
		c.Error(ErrBadRequest.WithMessage("无效的 cron 表达式: 需要 5 个字段 (分 时 日 月 周)"))
		return
	}
	if err := validateCronFieldRanges(parts); err != nil {
		c.Error(ErrBadRequest.WithMessage("无效的 cron 表达式: " + err.Error()))
		return
	}

	// Reject control chars in fields that get rendered verbatim into the
	// /etc/cron.d/easyserver line — a stray '\n' would silently inject an
	// extra cron entry that the UI cannot see or remove.
	if strings.ContainsAny(req.Name, "\r\n") || strings.ContainsAny(req.Command, "\r\n") {
		c.Error(ErrBadRequest.WithMessage("名称和命令不允许包含换行符"))
		return
	}

	// Validate: either command or script_id must be provided
	if req.Command == "" && req.ScriptID == 0 {
		c.Error(ErrBadRequest.WithMessage("必须提供命令或脚本ID"))
		return
	}

	// Validate timeout and retry bounds
	if req.Timeout < 0 || req.Timeout > 86400 {
		c.Error(ErrBadRequest.WithMessage("超时时间必须在 0 到 86400 秒之间"))
		return
	}
	if req.MaxRetry < 0 || req.MaxRetry > 10 {
		c.Error(ErrBadRequest.WithMessage("最大重试次数必须在 0 到 10 之间"))
		return
	}

	// Validate name uniqueness
	existing, err := h.cronService.List(c.Request.Context())
	if err != nil {
		c.Error(ErrInternal.WithMessage("检查任务名称失败: " + err.Error()))
		return
	}
	for _, t := range existing {
		if t.Name == req.Name {
			c.Error(ErrBadRequest.WithMessage("任务名称已存在"))
			return
		}
	}

	task := &cron.CronTask{
		Name:             req.Name,
		Command:          req.Command,
		Schedule:         req.Schedule,
		Description:      req.Description,
		Enabled:          true,
		Status:           "idle",
		ScriptID:         req.ScriptID,
		Timeout:          req.Timeout,
		MaxRetry:         req.MaxRetry,
		EnvVars:          req.EnvVars,
		WorkDir:          req.WorkDir,
		RuntimeVersionID: req.RuntimeVersionID,
	}

	if err := h.cronService.Create(c.Request.Context(), task); err != nil {
		c.Error(WrapError(err))
		return
	}
	Success(c, task)
}

// UpdateTask updates an existing cron task
func (h *CronHandler) UpdateTask(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.Error(ErrBadRequest.WithMessage("无效的任务ID"))
		return
	}

	task, err := h.cronService.Get(c.Request.Context(), id)
	if err != nil {
		c.Error(ErrNotFound.WithMessage("任务不存在"))
		return
	}

	var req cron.UpdateCronTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(ErrBadRequest.Wrap(err))
		return
	}

	middleware.AuditSummary(c, "更新定时任务 "+task.Name)
	// Apply partial updates
	if req.Name != nil {
		if strings.ContainsAny(*req.Name, "\r\n") {
			c.Error(ErrBadRequest.WithMessage("名称不允许包含换行符"))
			return
		}
		// Check name uniqueness (exclude current task)
		existing, listErr := h.cronService.List(c.Request.Context())
		if listErr != nil {
			c.Error(ErrInternal.WithMessage("检查任务名称失败: " + listErr.Error()))
			return
		}
		for _, t := range existing {
			if t.ID != id && t.Name == *req.Name {
				c.Error(ErrBadRequest.WithMessage("任务名称已存在"))
				return
			}
		}
		task.Name = *req.Name
	}
	if req.Command != nil {
		if strings.ContainsAny(*req.Command, "\r\n") {
			c.Error(ErrBadRequest.WithMessage("命令不允许包含换行符"))
			return
		}
		task.Command = *req.Command
	}
	if req.Schedule != nil {
		parts := strings.Fields(*req.Schedule)
		if len(parts) != 5 {
			c.Error(ErrBadRequest.WithMessage("无效的 cron 表达式: 需要 5 个字段"))
			return
		}
		if err := validateCronFieldRanges(parts); err != nil {
			c.Error(ErrBadRequest.WithMessage("无效的 cron 表达式: " + err.Error()))
			return
		}
		task.Schedule = *req.Schedule
	}
	if req.Description != nil {
		task.Description = *req.Description
	}
	if req.ScriptID != nil {
		task.ScriptID = *req.ScriptID
	}
	if req.Timeout != nil {
		if *req.Timeout < 0 || *req.Timeout > 86400 {
			c.Error(ErrBadRequest.WithMessage("超时时间必须在 0 到 86400 秒之间"))
			return
		}
		task.Timeout = *req.Timeout
	}
	if req.MaxRetry != nil {
		if *req.MaxRetry < 0 || *req.MaxRetry > 10 {
			c.Error(ErrBadRequest.WithMessage("最大重试次数必须在 0 到 10 之间"))
			return
		}
		task.MaxRetry = *req.MaxRetry
	}
	if req.EnvVars != nil {
		task.EnvVars = *req.EnvVars
	}
	if req.WorkDir != nil {
		task.WorkDir = *req.WorkDir
	}
	if req.RuntimeVersionID != nil {
		task.RuntimeVersionID = *req.RuntimeVersionID
	}

	// Validate command/script_id relationship
	if task.Command == "" && task.ScriptID == 0 {
		c.Error(ErrBadRequest.WithMessage("必须提供命令或脚本ID"))
		return
	}

	if err := h.cronService.Update(c.Request.Context(), task); err != nil {
		c.Error(WrapError(err))
		return
	}
	Success(c, task)
}

// DeleteTask deletes a cron task
func (h *CronHandler) DeleteTask(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.Error(ErrBadRequest.WithMessage("无效的任务ID"))
		return
	}
	// Check existence
	task, err := h.cronService.Get(c.Request.Context(), id)
	if err != nil {
		c.Error(ErrNotFound.WithMessage("任务不存在"))
		return
	}
	middleware.AuditSummary(c, "删除定时任务 "+task.Name)
	if err := h.cronService.Delete(c.Request.Context(), id); err != nil {
		c.Error(WrapError(err))
		return
	}
	Success(c, gin.H{"message": "任务已删除"})
}

// EnableTask enables a cron task
func (h *CronHandler) EnableTask(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.Error(ErrBadRequest.WithMessage("无效的任务ID"))
		return
	}
	// Check existence
	task, err := h.cronService.Get(c.Request.Context(), id)
	if err != nil {
		c.Error(ErrNotFound.WithMessage("任务不存在"))
		return
	}
	middleware.AuditSummary(c, "启用定时任务 "+task.Name)
	if err := h.cronService.Enable(c.Request.Context(), id); err != nil {
		c.Error(WrapError(err))
		return
	}
	Success(c, gin.H{"message": "任务已启用"})
}

// DisableTask disables a cron task
func (h *CronHandler) DisableTask(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.Error(ErrBadRequest.WithMessage("无效的任务ID"))
		return
	}
	// Check existence
	task, err := h.cronService.Get(c.Request.Context(), id)
	if err != nil {
		c.Error(ErrNotFound.WithMessage("任务不存在"))
		return
	}
	middleware.AuditSummary(c, "禁用定时任务 "+task.Name)
	if err := h.cronService.Disable(c.Request.Context(), id); err != nil {
		c.Error(WrapError(err))
		return
	}
	Success(c, gin.H{"message": "任务已禁用"})
}

// RunTask executes a cron task immediately
func (h *CronHandler) RunTask(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.Error(ErrBadRequest.WithMessage("无效的任务ID"))
		return
	}
	task, err := h.cronService.Get(c.Request.Context(), id)
	if err != nil {
		c.Error(ErrNotFound.WithMessage("任务不存在"))
		return
	}
	middleware.AuditSummary(c, "立即执行定时任务 "+task.Name)
	if err := h.cronService.RunNow(c.Request.Context(), id); err != nil {
		c.Error(WrapError(err))
		return
	}
	Success(c, gin.H{"message": "任务已执行"})
}

// GetTaskLogs returns execution logs for a cron task
func (h *CronHandler) GetTaskLogs(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.Error(ErrBadRequest.WithMessage("无效的任务ID"))
		return
	}

	limit := 50
	if l := c.Query("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 200 {
			limit = parsed
		}
	}

	logs, err := h.cronService.GetLogs(c.Request.Context(), id, limit)
	if err != nil {
		c.Error(WrapError(err))
		return
	}
	Success(c, logs)
}

// ListScripts returns all scripts
func (h *CronHandler) ListScripts(c *gin.Context) {
	scripts, err := h.cronService.ListScripts(c.Request.Context())
	if err != nil {
		c.Error(WrapError(err))
		return
	}
	Success(c, scripts)
}

// GetScript returns a script by ID
func (h *CronHandler) GetScript(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.Error(ErrBadRequest.WithMessage("无效的脚本ID"))
		return
	}
	script, err := h.cronService.GetScript(c.Request.Context(), id)
	if err != nil {
		c.Error(ErrNotFound.WithMessage("脚本不存在"))
		return
	}
	Success(c, script)
}

// CreateScript creates a new script
func (h *CronHandler) CreateScript(c *gin.Context) {
	var req cron.CreateScriptRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(ErrBadRequest.Wrap(err))
		return
	}

	middleware.AuditSummary(c, "创建脚本 "+req.Name)
	// Validate and set default language
	language := req.Language
	if language == "" {
		language = "sh"
	}

	// Validate language is supported
	if err := validateScriptLanguage(language); err != nil {
		c.Error(ErrBadRequest.Wrap(err))
		return
	}

	// Check language interpreter exists on server
	if err := h.checkInterpreterInstalled(language); err != nil {
		c.Error(ErrBadRequest.WithMessage(fmt.Sprintf("language '%s' is not installed: %v", language, err)))
		return
	}

	// Validate content is not empty
	if strings.TrimSpace(req.Content) == "" {
		c.Error(ErrBadRequest.WithMessage("脚本内容不能为空"))
		return
	}

	// Validate name uniqueness
	existingScripts, err := h.cronService.ListScripts(c.Request.Context())
	if err != nil {
		c.Error(ErrInternal.WithMessage("检查脚本名称失败: " + err.Error()))
		return
	}
	for _, s := range existingScripts {
		if s.Name == req.Name {
			c.Error(ErrBadRequest.WithMessage("脚本名称已存在"))
			return
		}
	}

	script := &cron.Script{
		Name:        req.Name,
		Description: req.Description,
		Content:     req.Content,
		Language:    language,
	}

	if err := h.cronService.CreateScript(c.Request.Context(), script); err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			c.Error(ErrConflict.WithMessage("脚本名称已存在"))
			return
		}
		c.Error(WrapError(err))
		return
	}
	Success(c, script)
}

// validateScriptLanguage checks if the language is supported
func validateScriptLanguage(language string) error {
	supported := map[string]bool{
		"sh":      true,
		"bash":    true,
		"python":  true,
		"python3": true,
	}
	if !supported[language] {
		return fmt.Errorf("unsupported language '%s', supported: sh, bash, python, python3", language)
	}
	return nil
}

// checkInterpreterInstalled verifies the language interpreter exists on the server
func (h *CronHandler) checkInterpreterInstalled(language string) error {
	path, err := h.executor.LookPath(language)
	if err != nil {
		return fmt.Errorf("interpreter '%s' not found in PATH", language)
	}

	// Verify it's actually executable
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("interpreter '%s' not accessible: %v", language, err)
	}

	return nil
}

// UpdateScript updates an existing script
func (h *CronHandler) UpdateScript(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.Error(ErrBadRequest.WithMessage("无效的脚本ID"))
		return
	}

	script, err := h.cronService.GetScript(c.Request.Context(), id)
	if err != nil {
		c.Error(ErrNotFound.WithMessage("脚本不存在"))
		return
	}

	var req cron.UpdateScriptRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(ErrBadRequest.Wrap(err))
		return
	}

	middleware.AuditSummary(c, "更新脚本 "+script.Name)
	if req.Name != nil {
		// Check name uniqueness (exclude current script)
		existingScripts, listErr := h.cronService.ListScripts(c.Request.Context())
		if listErr != nil {
			c.Error(ErrInternal.WithMessage("检查脚本名称失败: " + listErr.Error()))
			return
		}
		for _, s := range existingScripts {
			if s.ID != id && s.Name == *req.Name {
				c.Error(ErrBadRequest.WithMessage("脚本名称已存在"))
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
			c.Error(ErrBadRequest.WithMessage("脚本内容不能为空"))
			return
		}
		script.Content = *req.Content
	}
	if req.Language != nil {
		if err := validateScriptLanguage(*req.Language); err != nil {
			c.Error(ErrBadRequest.Wrap(err))
			return
		}
		if err := h.checkInterpreterInstalled(*req.Language); err != nil {
			c.Error(ErrBadRequest.WithMessage(fmt.Sprintf("language '%s' is not installed: %v", *req.Language, err)))
			return
		}
		script.Language = *req.Language
	}

	if err := h.cronService.UpdateScript(c.Request.Context(), script); err != nil {
		c.Error(WrapError(err))
		return
	}
	Success(c, script)
}

// DeleteScript deletes a script
func (h *CronHandler) DeleteScript(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.Error(ErrBadRequest.WithMessage("无效的脚本ID"))
		return
	}
	// Check existence
	script, err := h.cronService.GetScript(c.Request.Context(), id)
	if err != nil {
		c.Error(ErrNotFound.WithMessage("脚本不存在"))
		return
	}
	middleware.AuditSummary(c, "删除脚本 "+script.Name)
	// Check for dependent tasks
	tasks, listErr := h.cronService.List(c.Request.Context())
	if listErr != nil {
		c.Error(ErrInternal.WithMessage("检查依赖任务失败: " + listErr.Error()))
		return
	}
	for _, t := range tasks {
		if int64(t.ScriptID) == id {
			c.Error(ErrConflict.WithMessage(fmt.Sprintf("脚本被任务 '%s' 使用，请先移除引用", t.Name)))
			return
		}
	}
	if err := h.cronService.DeleteScript(c.Request.Context(), id); err != nil {
		c.Error(WrapError(err))
		return
	}
	Success(c, gin.H{"message": "脚本已删除"})
}

// GetPresets returns common cron expression presets
func (h *CronHandler) GetPresets(c *gin.Context) {
	presets := []gin.H{
		{"label": "每分钟", "value": "* * * * *", "description": "每分钟执行"},
		{"label": "每5分钟", "value": "*/5 * * * *", "description": "每5分钟执行"},
		{"label": "每小时", "value": "0 * * * *", "description": "每小时整点执行"},
		{"label": "每天凌晨2点", "value": "0 2 * * *", "description": "每天凌晨2点执行"},
		{"label": "每天凌晨3点", "value": "0 3 * * *", "description": "每天凌晨3点执行"},
		{"label": "每周一凌晨2点", "value": "0 2 * * 1", "description": "每周一凌晨2点执行"},
		{"label": "每月1号凌晨2点", "value": "0 2 1 * *", "description": "每月1号凌晨2点执行"},
		{"label": "每小时第5分钟", "value": "5 * * * *", "description": "每小时的第5分钟执行"},
		{"label": "工作日9点", "value": "0 9 * * 1-5", "description": "工作日每天9点执行"},
		{"label": "工作日18点", "value": "0 18 * * 1-5", "description": "工作日每天18点执行"},
	}
	Success(c, presets)
}

// DescribeSchedule returns a human-readable description of a cron expression
func (h *CronHandler) DescribeSchedule(c *gin.Context) {
	schedule := c.Query("schedule")
	if schedule == "" {
		c.Error(ErrBadRequest.WithMessage("schedule 参数不能为空"))
		return
	}
	desc := describeCronExpression(schedule)
	Success(c, gin.H{"description": desc})
}

// GetNextRuns returns the next N execution times for a cron expression
func (h *CronHandler) GetNextRuns(c *gin.Context) {
	schedule := c.Query("schedule")
	if schedule == "" {
		c.Error(ErrBadRequest.WithMessage("schedule 参数不能为空"))
		return
	}

	runs, err := calculateNextRuns(schedule, 5)
	if err != nil {
		c.Error(ErrBadRequest.WithMessage("无效的 cron 表达式: " + err.Error()))
		return
	}
	Success(c, gin.H{"next_runs": runs})
}

// describeCronExpression parses a 5-field cron expression and returns a Chinese description
func describeCronExpression(schedule string) string {
	parts := strings.Fields(schedule)
	if len(parts) != 5 {
		return "无效的 cron 表达式"
	}

	minute := parts[0]
	hour := parts[1]
	dayOfMonth := parts[2]
	month := parts[3]
	weekday := parts[4]

	weekdayNames := map[string]string{
		"0": "周日", "1": "周一", "2": "周二", "3": "周三",
		"4": "周四", "5": "周五", "6": "周六", "7": "周日",
	}

	// Every minute
	if minute == "*" && hour == "*" && dayOfMonth == "*" && month == "*" && weekday == "*" {
		return "每分钟执行"
	}

	// Every N minutes
	if strings.HasPrefix(minute, "*/") && hour == "*" && dayOfMonth == "*" && month == "*" && weekday == "*" {
		n := strings.TrimPrefix(minute, "*/")
		return fmt.Sprintf("每 %s 分钟执行", n)
	}

	// Every hour at exact time
	if minute != "*" && hour == "*" && dayOfMonth == "*" && month == "*" && weekday == "*" {
		return fmt.Sprintf("每小时的第 %s 分钟执行", minute)
	}

	// Specific time every day
	if minute != "*" && hour != "*" && dayOfMonth == "*" && month == "*" && weekday == "*" {
		return fmt.Sprintf("每天 %s:%s 执行", padZero(hour), padZero(minute))
	}

	// Specific day of month
	if minute != "*" && hour != "*" && dayOfMonth != "*" && month == "*" && weekday == "*" {
		return fmt.Sprintf("每月 %s 号 %s:%s 执行", dayOfMonth, padZero(hour), padZero(minute))
	}

	// Specific weekday(s)
	if minute != "*" && hour != "*" && dayOfMonth == "*" && month == "*" && weekday != "*" {
		weekdayDesc := formatWeekday(weekday, weekdayNames)
		return fmt.Sprintf("%s %s:%s 执行", weekdayDesc, padZero(hour), padZero(minute))
	}

	// Every hour exact
	if minute == "0" && hour == "*" && dayOfMonth == "*" && month == "*" && weekday == "*" {
		return "每小时整点执行"
	}

	// Specific month
	if minute != "*" && hour != "*" && month != "*" {
		monthDesc := fmt.Sprintf("%s 月", month)
		if dayOfMonth != "*" {
			monthDesc += fmt.Sprintf(" %s 号", dayOfMonth)
		}
		if weekday != "*" {
			monthDesc += " " + formatWeekday(weekday, weekdayNames)
		}
		return fmt.Sprintf("%s %s:%s 执行", monthDesc, padZero(hour), padZero(minute))
	}

	// Fallback
	return fmt.Sprintf("%s %s %s %s %s", minute, hour, dayOfMonth, month, weekday)
}

func padZero(s string) string {
	if len(s) == 1 {
		return "0" + s
	}
	return s
}

func formatWeekday(wd string, names map[string]string) string {
	// Handle ranges like 1-5
	if strings.Contains(wd, "-") {
		parts := strings.SplitN(wd, "-", 2)
		if name1, ok1 := names[parts[0]]; ok1 {
			if name2, ok2 := names[parts[1]]; ok2 {
				return fmt.Sprintf("%s到%s", name1, name2)
			}
		}
		return wd
	}
	// Handle lists like 1,3,5
	if strings.Contains(wd, ",") {
		items := strings.Split(wd, ",")
		var names_list []string
		for _, item := range items {
			if name, ok := names[strings.TrimSpace(item)]; ok {
				names_list = append(names_list, name)
			} else {
				names_list = append(names_list, item)
			}
		}
		return strings.Join(names_list, "、")
	}
	if name, ok := names[wd]; ok {
		return name
	}
	return wd
}

// calculateNextRuns calculates the next N execution times for a 5-field cron expression
func calculateNextRuns(schedule string, count int) ([]string, error) {
	parts := strings.Fields(schedule)
	if len(parts) != 5 {
		return nil, fmt.Errorf("expected 5 fields, got %d", len(parts))
	}

	minutes, err := parseCronField(parts[0], 0, 59)
	if err != nil {
		return nil, fmt.Errorf("invalid minute field: %v", err)
	}
	hours, err := parseCronField(parts[1], 0, 23)
	if err != nil {
		return nil, fmt.Errorf("invalid hour field: %v", err)
	}
	days, err := parseCronField(parts[2], 1, 31)
	if err != nil {
		return nil, fmt.Errorf("invalid day field: %v", err)
	}
	months, err := parseCronField(parts[3], 1, 12)
	if err != nil {
		return nil, fmt.Errorf("invalid month field: %v", err)
	}
	weekdays, err := parseCronField(parts[4], 0, 7)
	if err != nil {
		return nil, fmt.Errorf("invalid weekday field: %v", err)
	}

	// Normalize weekday 7 -> 0 (both mean Sunday)
	normalizedWeekdays := make(map[int]bool)
	for w := range weekdays {
		if w == 7 {
			normalizedWeekdays[0] = true
		} else {
			normalizedWeekdays[w] = true
		}
	}

	now := time.Now()
	var results []string
	t := now.Add(time.Minute).Truncate(time.Minute) // Start from next minute

	maxIterations := 366 * 24 * 60 // Safety limit: 1 year of minutes
	for i := 0; i < maxIterations && len(results) < count; i++ {
		_, mOk := minutes[t.Minute()]
		_, hOk := hours[t.Hour()]
		_, moOk := months[int(t.Month())]
		_, dOk := days[t.Day()]
		_, wOk := normalizedWeekdays[int(t.Weekday())]

		if mOk && hOk && moOk && (dOk || wOk) {
			results = append(results, t.Format("2006-01-02 15:04 (Mon)"))
		}
		t = t.Add(time.Minute)
	}

	return results, nil
}

// validateCronFieldRanges validates that each cron field is within valid ranges
func validateCronFieldRanges(parts []string) error {
	ranges := []struct {
		min, max int
		name     string
	}{
		{0, 59, "分钟"},
		{0, 23, "小时"},
		{1, 31, "日"},
		{1, 12, "月"},
		{0, 7, "星期"},
	}
	for i, part := range parts {
		if part == "*" || strings.Contains(part, "/") || strings.Contains(part, "-") || strings.Contains(part, ",") {
			continue // complex expressions validated by parseCronField
		}
		val, err := strconv.Atoi(part)
		if err != nil {
			return fmt.Errorf("%s字段值无效: %s", ranges[i].name, part)
		}
		if val < ranges[i].min || val > ranges[i].max {
			return fmt.Errorf("%s字段值 %d 超出范围 [%d, %d]", ranges[i].name, val, ranges[i].min, ranges[i].max)
		}
	}
	return nil
}

// parseCronField parses a single cron field and returns the set of valid values
func parseCronField(field string, min, max int) (map[int]bool, error) {
	result := make(map[int]bool)

	// Wildcard
	if field == "*" {
		for i := min; i <= max; i++ {
			result[i] = true
		}
		return result, nil
	}

	// Step values: */N or range/N
	if strings.Contains(field, "/") {
		stepParts := strings.SplitN(field, "/", 2)
		step, err := strconv.Atoi(stepParts[1])
		if err != nil || step <= 0 {
			return nil, fmt.Errorf("invalid step value")
		}

		rangeMin, rangeMax := min, max
		if stepParts[0] != "*" {
			rangeVals, err := parseCronField(stepParts[0], min, max)
			if err != nil {
				return nil, err
			}
			// Find min and max from the range
			rangeMin = max
			rangeMax = min
			for v := range rangeVals {
				if v < rangeMin {
					rangeMin = v
				}
				if v > rangeMax {
					rangeMax = v
				}
			}
		}

		for i := rangeMin; i <= rangeMax; i += step {
			result[i] = true
		}
		return result, nil
	}

	// Comma-separated values
	for _, part := range strings.Split(field, ",") {
		part = strings.TrimSpace(part)

		// Range: N-M
		if strings.Contains(part, "-") {
			rangeParts := strings.SplitN(part, "-", 2)
			start, err := strconv.Atoi(rangeParts[0])
			if err != nil {
				return nil, fmt.Errorf("invalid range start: %s", rangeParts[0])
			}
			end, err := strconv.Atoi(rangeParts[1])
			if err != nil {
				return nil, fmt.Errorf("invalid range end: %s", rangeParts[1])
			}
			for i := start; i <= end; i++ {
				if i >= min && i <= max {
					result[i] = true
				}
			}
			continue
		}

		// Single value
		val, err := strconv.Atoi(part)
		if err != nil {
			return nil, fmt.Errorf("invalid value: %s", part)
		}
		if val >= min && val <= max {
			result[val] = true
		}
	}

	return result, nil
}

// ============================================================
// Cron documentation handlers
// ============================================================

// ListDocs returns all cron documentation sections
func (h *CronHandler) ListDocs(c *gin.Context) {
	docs, err := h.cronService.ListDocs(c.Request.Context())
	if err != nil {
		c.Error(WrapError(err))
		return
	}
	Success(c, docs)
}

// GetDoc returns a cron documentation section by ID
func (h *CronHandler) GetDoc(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.Error(ErrBadRequest.WithMessage("无效的文档ID"))
		return
	}
	doc, err := h.cronService.GetDoc(c.Request.Context(), id)
	if err != nil {
		c.Error(ErrNotFound.WithMessage("文档不存在"))
		return
	}
	Success(c, doc)
}

// CreateDoc creates a new cron documentation section
func (h *CronHandler) CreateDoc(c *gin.Context) {
	var req struct {
		Title     string `json:"title" binding:"required"`
		Content   string `json:"content" binding:"required"`
		SortOrder int    `json:"sort_order"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(ErrBadRequest.Wrap(err))
		return
	}

	middleware.AuditSummary(c, "创建定时任务文档 "+req.Title)
	doc := &cron.CronDoc{
		Title:     req.Title,
		Content:   req.Content,
		SortOrder: req.SortOrder,
	}
	if err := h.cronService.CreateDoc(c.Request.Context(), doc); err != nil {
		c.Error(WrapError(err))
		return
	}
	Success(c, doc)
}

// UpdateDoc updates an existing cron documentation section
func (h *CronHandler) UpdateDoc(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.Error(ErrBadRequest.WithMessage("无效的文档ID"))
		return
	}
	doc, err := h.cronService.GetDoc(c.Request.Context(), id)
	if err != nil {
		c.Error(ErrNotFound.WithMessage("文档不存在"))
		return
	}
	var req struct {
		Title     *string `json:"title"`
		Content   *string `json:"content"`
		SortOrder *int    `json:"sort_order"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(ErrBadRequest.Wrap(err))
		return
	}

	middleware.AuditSummary(c, "更新定时任务文档 "+doc.Title)
	if req.Title != nil {
		doc.Title = *req.Title
	}
	if req.Content != nil {
		doc.Content = *req.Content
	}
	if req.SortOrder != nil {
		doc.SortOrder = *req.SortOrder
	}
	if err := h.cronService.UpdateDoc(c.Request.Context(), doc); err != nil {
		c.Error(WrapError(err))
		return
	}
	Success(c, doc)
}

// DeleteDoc deletes a cron documentation section
func (h *CronHandler) DeleteDoc(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.Error(ErrBadRequest.WithMessage("无效的文档ID"))
		return
	}
	doc, err := h.cronService.GetDoc(c.Request.Context(), id)
	if err != nil {
		c.Error(ErrNotFound.WithMessage("文档不存在"))
		return
	}
	middleware.AuditSummary(c, "删除定时任务文档 "+doc.Title)
	if err := h.cronService.DeleteDoc(c.Request.Context(), id); err != nil {
		c.Error(WrapError(err))
		return
	}
	Success(c, gin.H{"message": "文档已删除"})
}

func registerCronRoutes(protected *gin.RouterGroup, cronService *cron.Service, exec executor.CommandExecutor) {
	// Seed default documentation (tables managed by migration system)
	if err := cronService.SeedDefaultDocs(context.Background()); err != nil {
		log.Printf("WARNING: seed default cron docs failed: %v", err)
	}
	handler := NewCronHandler(cronService, exec)

	protected.GET("/cron/presets", handler.GetPresets)
	protected.GET("/cron/describe", handler.DescribeSchedule)
	protected.GET("/cron/next-runs", handler.GetNextRuns)
	protected.GET("/cron/tasks", handler.ListTasks)
	protected.POST("/cron/tasks", handler.CreateTask)
	protected.GET("/cron/tasks/:id", handler.GetTask)
	protected.PUT("/cron/tasks/:id", handler.UpdateTask)
	protected.DELETE("/cron/tasks/:id", handler.DeleteTask)
	protected.POST("/cron/tasks/:id/enable", handler.EnableTask)
	protected.POST("/cron/tasks/:id/disable", handler.DisableTask)
	protected.POST("/cron/tasks/:id/run", handler.RunTask)
	protected.GET("/cron/tasks/:id/logs", handler.GetTaskLogs)
	protected.GET("/cron/scripts", handler.ListScripts)
	protected.POST("/cron/scripts", handler.CreateScript)
	protected.GET("/cron/scripts/:id", handler.GetScript)
	protected.PUT("/cron/scripts/:id", handler.UpdateScript)
	protected.DELETE("/cron/scripts/:id", handler.DeleteScript)
	protected.GET("/cron/docs", handler.ListDocs)
	protected.POST("/cron/docs", handler.CreateDoc)
	protected.GET("/cron/docs/:id", handler.GetDoc)
	protected.PUT("/cron/docs/:id", handler.UpdateDoc)
	protected.DELETE("/cron/docs/:id", handler.DeleteDoc)
}

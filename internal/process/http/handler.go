package http

import (
	"strconv"

	"easyserver/internal/httpx"
	"easyserver/internal/httpx/middleware"
	"easyserver/internal/infra/apperror"
	"easyserver/internal/process"

	"github.com/gin-gonic/gin"
)

// ProcessHandler handles process guardian API requests
type ProcessHandler struct {
	pm *process.Service
}

// NewProcessHandler creates a new ProcessHandler
func NewProcessHandler(pm *process.Service) *ProcessHandler {
	return &ProcessHandler{pm: pm}
}

// ListProcesses returns all processes with status
func (h *ProcessHandler) ListProcesses(c *gin.Context) {
	processes, err := h.pm.List(c.Request.Context())
	if err != nil {
		c.Error(apperror.WrapError(err))
		return
	}
	httpx.Success(c, processes)
}

// GetProcess returns a single process by ID
func (h *ProcessHandler) GetProcess(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.Error(apperror.ErrBadRequest.WithMessage("无效的进程ID"))
		return
	}
	p, err := h.pm.Get(c.Request.Context(), id)
	if err != nil {
		c.Error(apperror.WrapError(err))
		return
	}
	if p == nil {
		c.Error(apperror.ErrNotFound.WithMessage("进程不存在"))
		return
	}
	httpx.Success(c, p)
}

// CreateProcess creates a new process configuration
func (h *ProcessHandler) CreateProcess(c *gin.Context) {
	var req process.CreateProcessRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperror.ErrBadRequest.WithMessage("参数错误: " + err.Error()))
		return
	}

	middleware.AuditSummary(c, "创建进程 "+req.Name)
	p, err := h.pm.Create(c.Request.Context(), &req)
	if err != nil {
		c.Error(apperror.WrapError(err))
		return
	}
	httpx.Success(c, p)
}

// UpdateProcess updates a process configuration
func (h *ProcessHandler) UpdateProcess(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.Error(apperror.ErrBadRequest.WithMessage("无效的进程ID"))
		return
	}
	var req process.UpdateProcessRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperror.ErrBadRequest.WithMessage("参数错误: " + err.Error()))
		return
	}

	middleware.AuditSummary(c, "更新进程 "+strconv.FormatInt(id, 10))
	if err := h.pm.Update(c.Request.Context(), id, &req); err != nil {
		c.Error(apperror.WrapError(err))
		return
	}
	httpx.Success(c, gin.H{"message": "更新成功"})
}

// DeleteProcess deletes a process configuration
func (h *ProcessHandler) DeleteProcess(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.Error(apperror.ErrBadRequest.WithMessage("无效的进程ID"))
		return
	}
	middleware.AuditSummary(c, "删除进程 "+strconv.FormatInt(id, 10))
	if err := h.pm.Delete(c.Request.Context(), id); err != nil {
		c.Error(apperror.WrapError(err))
		return
	}
	httpx.Success(c, gin.H{"message": "删除成功"})
}

// StartProcess launches a process
func (h *ProcessHandler) StartProcess(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.Error(apperror.ErrBadRequest.WithMessage("无效的进程ID"))
		return
	}
	middleware.AuditSummary(c, "启动进程 "+strconv.FormatInt(id, 10))
	if err := h.pm.Start(c.Request.Context(), id); err != nil {
		c.Error(apperror.WrapError(err))
		return
	}
	httpx.Success(c, gin.H{"message": "启动成功"})
}

// StopProcess terminates a running process
func (h *ProcessHandler) StopProcess(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.Error(apperror.ErrBadRequest.WithMessage("无效的进程ID"))
		return
	}
	middleware.AuditSummary(c, "停止进程 "+strconv.FormatInt(id, 10))
	if err := h.pm.Stop(c.Request.Context(), id); err != nil {
		c.Error(apperror.WrapError(err))
		return
	}
	httpx.Success(c, gin.H{"message": "停止成功"})
}

// RestartProcess restarts a process
func (h *ProcessHandler) RestartProcess(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.Error(apperror.ErrBadRequest.WithMessage("无效的进程ID"))
		return
	}
	middleware.AuditSummary(c, "重启进程 "+strconv.FormatInt(id, 10))
	if err := h.pm.Restart(c.Request.Context(), id); err != nil {
		c.Error(apperror.WrapError(err))
		return
	}
	httpx.Success(c, gin.H{"message": "重启成功"})
}

// GetProcessLogs returns process log entries
func (h *ProcessHandler) GetProcessLogs(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.Error(apperror.ErrBadRequest.WithMessage("无效的进程ID"))
		return
	}
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	logs, total, err := h.pm.GetLogs(c.Request.Context(), id, limit, offset)
	if err != nil {
		c.Error(apperror.WrapError(err))
		return
	}
	httpx.SuccessPaginated(c, int64(total), logs)
}

// GetProcessStats returns runtime resource stats
func (h *ProcessHandler) GetProcessStats(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.Error(apperror.ErrBadRequest.WithMessage("无效的进程ID"))
		return
	}
	stats, err := h.pm.GetStats(c.Request.Context(), id)
	if err != nil {
		c.Error(apperror.WrapError(err))
		return
	}
	httpx.Success(c, stats)
}

// BatchStart starts multiple processes
func (h *ProcessHandler) BatchStart(c *gin.Context) {
	var req process.BatchProcessIDs
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperror.ErrBadRequest.WithMessage("参数错误: " + err.Error()))
		return
	}

	middleware.AuditSummary(c, "批量启动进程 "+strconv.Itoa(len(req.IDs))+" 个")
	started, failed, err := h.pm.BatchStart(c.Request.Context(), req.IDs)
	if err != nil {
		c.Error(apperror.WrapError(err))
		return
	}
	httpx.Success(c, gin.H{"started": started, "failed": failed})
}

// BatchStop stops multiple processes
func (h *ProcessHandler) BatchStop(c *gin.Context) {
	var req process.BatchProcessIDs
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperror.ErrBadRequest.WithMessage("参数错误: " + err.Error()))
		return
	}

	middleware.AuditSummary(c, "批量停止进程 "+strconv.Itoa(len(req.IDs))+" 个")
	stopped, failed, err := h.pm.BatchStop(c.Request.Context(), req.IDs)
	if err != nil {
		c.Error(apperror.WrapError(err))
		return
	}
	httpx.Success(c, gin.H{"stopped": stopped, "failed": failed})
}

// BatchRestart restarts multiple processes
func (h *ProcessHandler) BatchRestart(c *gin.Context) {
	var req process.BatchProcessIDs
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperror.ErrBadRequest.WithMessage("参数错误: " + err.Error()))
		return
	}

	middleware.AuditSummary(c, "批量重启进程 "+strconv.Itoa(len(req.IDs))+" 个")
	restarted, failed, err := h.pm.BatchRestart(c.Request.Context(), req.IDs)
	if err != nil {
		c.Error(apperror.WrapError(err))
		return
	}
	httpx.Success(c, gin.H{"restarted": restarted, "failed": failed})
}

// --- Group handlers ---

// ListGroups returns all process groups
func (h *ProcessHandler) ListGroups(c *gin.Context) {
	groups, err := h.pm.ListGroups(c.Request.Context())
	if err != nil {
		c.Error(apperror.WrapError(err))
		return
	}
	httpx.Success(c, groups)
}

// GetGroup returns a process group by ID
func (h *ProcessHandler) GetGroup(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.Error(apperror.ErrBadRequest.WithMessage("无效的分组ID"))
		return
	}
	g, err := h.pm.GetGroup(c.Request.Context(), id)
	if err != nil {
		c.Error(apperror.WrapError(err))
		return
	}
	if g == nil {
		c.Error(apperror.ErrNotFound.WithMessage("分组不存在"))
		return
	}
	httpx.Success(c, g)
}

// CreateGroup creates a new process group
func (h *ProcessHandler) CreateGroup(c *gin.Context) {
	var req process.CreateProcessGroupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperror.ErrBadRequest.WithMessage("参数错误: " + err.Error()))
		return
	}

	middleware.AuditSummary(c, "创建进程分组 "+req.Name)
	g, err := h.pm.CreateGroup(c.Request.Context(), &req)
	if err != nil {
		c.Error(apperror.WrapError(err))
		return
	}
	httpx.Success(c, g)
}

// UpdateGroup updates a process group
func (h *ProcessHandler) UpdateGroup(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.Error(apperror.ErrBadRequest.WithMessage("无效的分组ID"))
		return
	}
	var req process.UpdateProcessGroupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperror.ErrBadRequest.WithMessage("参数错误: " + err.Error()))
		return
	}

	middleware.AuditSummary(c, "更新进程分组 "+strconv.FormatInt(id, 10))
	if err := h.pm.UpdateGroup(c.Request.Context(), id, &req); err != nil {
		c.Error(apperror.WrapError(err))
		return
	}
	httpx.Success(c, gin.H{"message": "更新成功"})
}

// DeleteGroup deletes a process group
func (h *ProcessHandler) DeleteGroup(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.Error(apperror.ErrBadRequest.WithMessage("无效的分组ID"))
		return
	}
	middleware.AuditSummary(c, "删除进程分组 "+strconv.FormatInt(id, 10))
	if err := h.pm.DeleteGroup(c.Request.Context(), id); err != nil {
		c.Error(apperror.WrapError(err))
		return
	}
	httpx.Success(c, gin.H{"message": "删除成功"})
}

// ExportProcesses exports all process configs as JSON
func (h *ProcessHandler) ExportProcesses(c *gin.Context) {
	processes, err := h.pm.Export(c.Request.Context())
	if err != nil {
		c.Error(apperror.WrapError(err))
		return
	}
	httpx.Success(c, processes)
}

// ImportProcesses imports process configs from JSON
func (h *ProcessHandler) ImportProcesses(c *gin.Context) {
	var processes []process.Process
	if err := c.ShouldBindJSON(&processes); err != nil {
		c.Error(apperror.ErrBadRequest.WithMessage("参数错误: " + err.Error()))
		return
	}

	middleware.AuditSummary(c, "导入进程 "+strconv.Itoa(len(processes))+" 个")
	count, err := h.pm.Import(c.Request.Context(), processes)
	if err != nil {
		c.Error(apperror.WrapError(err))
		return
	}
	httpx.Success(c, gin.H{"imported": count})
}

// RegisterRoutes registers process guardian API routes
func RegisterRoutes(protected *gin.RouterGroup, pm *process.Service) {
	handler := NewProcessHandler(pm)

	// Process CRUD
	protected.GET("/processes", handler.ListProcesses)
	protected.POST("/processes", handler.CreateProcess)
	protected.GET("/processes/:id", handler.GetProcess)
	protected.PUT("/processes/:id", handler.UpdateProcess)
	protected.DELETE("/processes/:id", handler.DeleteProcess)

	// Process lifecycle
	protected.POST("/processes/:id/start", handler.StartProcess)
	protected.POST("/processes/:id/stop", handler.StopProcess)
	protected.POST("/processes/:id/restart", handler.RestartProcess)

	// Process logs and stats
	protected.GET("/processes/:id/logs", handler.GetProcessLogs)
	protected.GET("/processes/:id/stats", handler.GetProcessStats)

	// Batch operations
	protected.POST("/processes/batch/start", handler.BatchStart)
	protected.POST("/processes/batch/stop", handler.BatchStop)
	protected.POST("/processes/batch/restart", handler.BatchRestart)

	// Process groups
	protected.GET("/process-groups", handler.ListGroups)
	protected.POST("/process-groups", handler.CreateGroup)
	protected.GET("/process-groups/:id", handler.GetGroup)
	protected.PUT("/process-groups/:id", handler.UpdateGroup)
	protected.DELETE("/process-groups/:id", handler.DeleteGroup)

	// Import/Export
	protected.GET("/processes/export", handler.ExportProcesses)
	protected.POST("/processes/import", handler.ImportProcesses)
}

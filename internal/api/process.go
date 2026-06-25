package api

import (
	"strconv"

	"easyserver/internal/model"
	"easyserver/internal/service"

	"github.com/gin-gonic/gin"
)

// ProcessHandler handles process guardian API requests
type ProcessHandler struct {
	pm *service.ProcessManager
}

// NewProcessHandler creates a new ProcessHandler
func NewProcessHandler(pm *service.ProcessManager) *ProcessHandler {
	return &ProcessHandler{pm: pm}
}

// ListProcesses returns all processes with status
func (h *ProcessHandler) ListProcesses(c *gin.Context) {
	processes, err := h.pm.List(c.Request.Context())
	if err != nil {
		InternalError(c, err.Error())
		return
	}
	Success(c, processes)
}

// GetProcess returns a single process by ID
func (h *ProcessHandler) GetProcess(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		BadRequest(c, "无效的进程ID")
		return
	}
	p, err := h.pm.Get(c.Request.Context(), id)
	if err != nil {
		InternalError(c, err.Error())
		return
	}
	if p == nil {
		NotFound(c, "进程不存在")
		return
	}
	Success(c, p)
}

// CreateProcess creates a new process configuration
func (h *ProcessHandler) CreateProcess(c *gin.Context) {
	var req model.CreateProcessRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequest(c, "参数错误: "+err.Error())
		return
	}
	p, err := h.pm.Create(c.Request.Context(), &req)
	if err != nil {
		InternalError(c, err.Error())
		return
	}
	Success(c, p)
}

// UpdateProcess updates a process configuration
func (h *ProcessHandler) UpdateProcess(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		BadRequest(c, "无效的进程ID")
		return
	}
	var req model.UpdateProcessRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequest(c, "参数错误: "+err.Error())
		return
	}
	if err := h.pm.Update(c.Request.Context(), id, &req); err != nil {
		InternalError(c, err.Error())
		return
	}
	Success(c, gin.H{"message": "更新成功"})
}

// DeleteProcess deletes a process configuration
func (h *ProcessHandler) DeleteProcess(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		BadRequest(c, "无效的进程ID")
		return
	}
	if err := h.pm.Delete(c.Request.Context(), id); err != nil {
		InternalError(c, err.Error())
		return
	}
	Success(c, gin.H{"message": "删除成功"})
}

// StartProcess launches a process
func (h *ProcessHandler) StartProcess(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		BadRequest(c, "无效的进程ID")
		return
	}
	if err := h.pm.Start(c.Request.Context(), id); err != nil {
		InternalError(c, err.Error())
		return
	}
	Success(c, gin.H{"message": "启动成功"})
}

// StopProcess terminates a running process
func (h *ProcessHandler) StopProcess(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		BadRequest(c, "无效的进程ID")
		return
	}
	if err := h.pm.Stop(c.Request.Context(), id); err != nil {
		InternalError(c, err.Error())
		return
	}
	Success(c, gin.H{"message": "停止成功"})
}

// RestartProcess restarts a process
func (h *ProcessHandler) RestartProcess(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		BadRequest(c, "无效的进程ID")
		return
	}
	if err := h.pm.Restart(c.Request.Context(), id); err != nil {
		InternalError(c, err.Error())
		return
	}
	Success(c, gin.H{"message": "重启成功"})
}

// GetProcessLogs returns process log entries
func (h *ProcessHandler) GetProcessLogs(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		BadRequest(c, "无效的进程ID")
		return
	}
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	logs, total, err := h.pm.GetLogs(c.Request.Context(), id, limit, offset)
	if err != nil {
		InternalError(c, err.Error())
		return
	}
	SuccessPaginated(c, int64(total), logs)
}

// GetProcessStats returns runtime resource stats
func (h *ProcessHandler) GetProcessStats(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		BadRequest(c, "无效的进程ID")
		return
	}
	stats, err := h.pm.GetStats(c.Request.Context(), id)
	if err != nil {
		InternalError(c, err.Error())
		return
	}
	Success(c, stats)
}

// BatchStart starts multiple processes
func (h *ProcessHandler) BatchStart(c *gin.Context) {
	var req model.BatchProcessIDs
	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequest(c, "参数错误: "+err.Error())
		return
	}
	started, failed, err := h.pm.BatchStart(c.Request.Context(), req.IDs)
	if err != nil {
		InternalError(c, err.Error())
		return
	}
	Success(c, gin.H{"started": started, "failed": failed})
}

// BatchStop stops multiple processes
func (h *ProcessHandler) BatchStop(c *gin.Context) {
	var req model.BatchProcessIDs
	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequest(c, "参数错误: "+err.Error())
		return
	}
	stopped, failed, err := h.pm.BatchStop(c.Request.Context(), req.IDs)
	if err != nil {
		InternalError(c, err.Error())
		return
	}
	Success(c, gin.H{"stopped": stopped, "failed": failed})
}

// BatchRestart restarts multiple processes
func (h *ProcessHandler) BatchRestart(c *gin.Context) {
	var req model.BatchProcessIDs
	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequest(c, "参数错误: "+err.Error())
		return
	}
	restarted, failed, err := h.pm.BatchRestart(c.Request.Context(), req.IDs)
	if err != nil {
		InternalError(c, err.Error())
		return
	}
	Success(c, gin.H{"restarted": restarted, "failed": failed})
}

// --- Group handlers ---

// ListGroups returns all process groups
func (h *ProcessHandler) ListGroups(c *gin.Context) {
	groups, err := h.pm.ListGroups(c.Request.Context())
	if err != nil {
		InternalError(c, err.Error())
		return
	}
	Success(c, groups)
}

// CreateGroup creates a new process group
func (h *ProcessHandler) CreateGroup(c *gin.Context) {
	var req model.CreateProcessGroupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequest(c, "参数错误: "+err.Error())
		return
	}
	g, err := h.pm.CreateGroup(c.Request.Context(), &req)
	if err != nil {
		InternalError(c, err.Error())
		return
	}
	Success(c, g)
}

// UpdateGroup updates a process group
func (h *ProcessHandler) UpdateGroup(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		BadRequest(c, "无效的分组ID")
		return
	}
	var req model.UpdateProcessGroupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequest(c, "参数错误: "+err.Error())
		return
	}
	if err := h.pm.UpdateGroup(c.Request.Context(), id, &req); err != nil {
		InternalError(c, err.Error())
		return
	}
	Success(c, gin.H{"message": "更新成功"})
}

// DeleteGroup deletes a process group
func (h *ProcessHandler) DeleteGroup(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		BadRequest(c, "无效的分组ID")
		return
	}
	if err := h.pm.DeleteGroup(c.Request.Context(), id); err != nil {
		InternalError(c, err.Error())
		return
	}
	Success(c, gin.H{"message": "删除成功"})
}

// ExportProcesses exports all process configs as JSON
func (h *ProcessHandler) ExportProcesses(c *gin.Context) {
	processes, err := h.pm.Export(c.Request.Context())
	if err != nil {
		InternalError(c, err.Error())
		return
	}
	Success(c, processes)
}

// ImportProcesses imports process configs from JSON
func (h *ProcessHandler) ImportProcesses(c *gin.Context) {
	var processes []model.Process
	if err := c.ShouldBindJSON(&processes); err != nil {
		BadRequest(c, "参数错误: "+err.Error())
		return
	}
	count, err := h.pm.Import(c.Request.Context(), processes)
	if err != nil {
		InternalError(c, err.Error())
		return
	}
	Success(c, gin.H{"imported": count})
}
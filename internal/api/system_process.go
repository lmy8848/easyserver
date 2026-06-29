package api

import (
	"fmt"
	"strconv"

	"easyserver/internal/systemprocess"

	"github.com/gin-gonic/gin"
)

// SystemProcessHandler handles system process monitoring API requests.
type SystemProcessHandler struct {
	sps *systemprocess.Service
}

// NewSystemProcessHandler creates a new SystemProcessHandler.
func NewSystemProcessHandler(sps *systemprocess.Service) *SystemProcessHandler {
	return &SystemProcessHandler{sps: sps}
}

// ListSystemProcesses returns all running system processes.
func (h *SystemProcessHandler) ListSystemProcesses(c *gin.Context) {
	sortBy := c.DefaultQuery("sort_by", "memory")
	order := c.DefaultQuery("order", "desc")
	search := c.Query("search")
	limitStr := c.DefaultQuery("limit", "100")
	limit, _ := strconv.Atoi(limitStr)

	processes, err := h.sps.ListSystemProcesses(sortBy, order, search, limit)
	if err != nil {
		c.Error(WrapError(err))
		return
	}
	Success(c, processes)
}

// GetSystemProcess returns details for a specific process.
func (h *SystemProcessHandler) GetSystemProcess(c *gin.Context) {
	pidStr := c.Param("pid")
	pid, err := strconv.Atoi(pidStr)
	if err != nil {
		c.Error(ErrBadRequest.WithMessage("无效的PID"))
		return
	}

	proc, err := systemprocess.GetSystemProcess(pid)
	if err != nil {
		c.Error(ErrNotFound.WithMessage(fmt.Sprintf("进程 %d 不存在", pid)))
		return
	}
	Success(c, proc)
}

func registerSystemProcessRoutes(protected *gin.RouterGroup, sps *systemprocess.Service) {
	handler := NewSystemProcessHandler(sps)

	sysGroup := protected.Group("/system")
	{
		// System processes (read-only monitoring)
		sysGroup.GET("/processes", handler.ListSystemProcesses)
		sysGroup.GET("/processes/:pid", handler.GetSystemProcess)
	}
}

package api

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"easyserver/internal/systemprocess"

	"github.com/gin-gonic/gin"
)

// SystemProcessHandler handles system process monitoring and service management API requests.
type SystemProcessHandler struct {
	sps *systemprocess.Service
}

// NewSystemProcessHandler creates a new SystemProcessHandler.
func NewSystemProcessHandler(sps *systemprocess.Service) *SystemProcessHandler {
	return &SystemProcessHandler{sps: sps}
}

// GetSystemOverview returns system-wide resource statistics.
func (h *SystemProcessHandler) GetSystemOverview(c *gin.Context) {
	overview, err := h.sps.GetOverview()
	if err != nil {
		InternalError(c, err.Error())
		return
	}
	Success(c, overview)
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
		InternalError(c, err.Error())
		return
	}
	Success(c, processes)
}

// GetSystemProcess returns details for a specific process.
func (h *SystemProcessHandler) GetSystemProcess(c *gin.Context) {
	pidStr := c.Param("pid")
	pid, err := strconv.Atoi(pidStr)
	if err != nil {
		BadRequest(c, "无效的PID")
		return
	}

	proc, err := systemprocess.GetSystemProcess(pid)
	if err != nil {
		NotFound(c, fmt.Sprintf("进程 %d 不存在", pid))
		return
	}
	Success(c, proc)
}

// ListSystemServices returns systemd services.
func (h *SystemProcessHandler) ListSystemServices(c *gin.Context) {
	services, err := h.sps.ListServices()
	if err != nil {
		InternalError(c, err.Error())
		return
	}
	Success(c, services)
}

// ServiceAction performs an action on a systemd service.
func (h *SystemProcessHandler) ServiceAction(c *gin.Context) {
	name := c.Param("name")
	if name == "" {
		BadRequest(c, "服务名不能为空")
		return
	}

	var req struct {
		Action string `json:"action" binding:"required"`
		Force  bool   `json:"force"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequest(c, "无效的请求参数")
		return
	}

	if err := h.sps.ServiceAction(name, req.Action, req.Force); err != nil {
		// Check for protected service error
		if strings.HasPrefix(err.Error(), "protected_service:") {
			parts := strings.SplitN(err.Error(), ":", 3)
			if len(parts) >= 3 {
				c.JSON(http.StatusOK, gin.H{
					"code":    40300,
					"message": fmt.Sprintf("受保护的服务: %s (%s)，请使用强制操作", parts[1], parts[2]),
					"data": gin.H{
						"protected": true,
						"service":   parts[1],
						"reason":    parts[2],
					},
				})
				return
			}
		}
		InternalError(c, err.Error())
		return
	}
	Success(c, gin.H{"message": fmt.Sprintf("服务 %s %s 成功", name, req.Action)})
}

// GetServiceLogs returns recent logs for a systemd service.
func (h *SystemProcessHandler) GetServiceLogs(c *gin.Context) {
	name := c.Param("name")
	if name == "" {
		BadRequest(c, "服务名不能为空")
		return
	}

	linesStr := c.DefaultQuery("lines", "100")
	lines, _ := strconv.Atoi(linesStr)

	logs, err := h.sps.GetServiceLogs(name, lines)
	if err != nil {
		InternalError(c, err.Error())
		return
	}
	Success(c, gin.H{"logs": logs, "service": name})
}

// ListProtectedServices returns the list of protected services.
func (h *SystemProcessHandler) ListProtectedServices(c *gin.Context) {
	protected := make([]gin.H, 0)
	for name, reason := range systemprocess.ProtectedServices() {
		protected = append(protected, gin.H{"name": name, "reason": reason})
	}
	Success(c, protected)
}

// ListWhitelist returns the service whitelist.
func (h *SystemProcessHandler) ListWhitelist(c *gin.Context) {
	entries, err := h.sps.GetWhitelist()
	if err != nil {
		InternalError(c, err.Error())
		return
	}
	Success(c, entries)
}

// AddToWhitelist adds a service to the whitelist.
func (h *SystemProcessHandler) AddToWhitelist(c *gin.Context) {
	var req struct {
		Name string `json:"name" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequest(c, "服务名不能为空")
		return
	}

	if err := h.sps.AddToWhitelist(req.Name); err != nil {
		InternalError(c, err.Error())
		return
	}
	Success(c, gin.H{"message": fmt.Sprintf("已添加 %s 到白名单", req.Name)})
}

// RemoveFromWhitelist removes a service from the whitelist.
func (h *SystemProcessHandler) RemoveFromWhitelist(c *gin.Context) {
	name := c.Param("name")
	if name == "" {
		BadRequest(c, "服务名不能为空")
		return
	}

	if err := h.sps.RemoveFromWhitelist(name); err != nil {
		InternalError(c, err.Error())
		return
	}
	Success(c, gin.H{"message": fmt.Sprintf("已从白名单移除 %s", name)})
}

func registerSystemProcessRoutes(protected *gin.RouterGroup, sps *systemprocess.Service) {
	handler := NewSystemProcessHandler(sps)

	sysGroup := protected.Group("/system")
	{
		// System overview
		sysGroup.GET("/overview", handler.GetSystemOverview)

		// System processes (read-only monitoring)
		sysGroup.GET("/processes", handler.ListSystemProcesses)
		sysGroup.GET("/processes/:pid", handler.GetSystemProcess)

		// System services (systemd management)
		sysGroup.GET("/services", handler.ListSystemServices)
		sysGroup.POST("/services/:name/action", handler.ServiceAction)
		sysGroup.GET("/services/:name/logs", handler.GetServiceLogs)

		// Protected services info
		sysGroup.GET("/services/protected", handler.ListProtectedServices)

		// Service whitelist
		sysGroup.GET("/services/whitelist", handler.ListWhitelist)
		sysGroup.POST("/services/whitelist", handler.AddToWhitelist)
		sysGroup.DELETE("/services/whitelist/:name", handler.RemoveFromWhitelist)
	}
}

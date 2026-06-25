package api

import (
	"easyserver/internal/service"

	"github.com/gin-gonic/gin"
)

// registerProcessRoutes registers process guardian routes
func registerProcessRoutes(protected *gin.RouterGroup, pm *service.ProcessManager) {
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
	protected.PUT("/process-groups/:id", handler.UpdateGroup)
	protected.DELETE("/process-groups/:id", handler.DeleteGroup)

	// Import/Export
	protected.GET("/processes/export", handler.ExportProcesses)
	protected.POST("/processes/import", handler.ImportProcesses)
}
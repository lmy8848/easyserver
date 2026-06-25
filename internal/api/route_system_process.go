package api

import (
	"easyserver/internal/service"

	"github.com/gin-gonic/gin"
)

// registerSystemProcessRoutes registers system process monitoring and service management routes
func registerSystemProcessRoutes(protected *gin.RouterGroup, sps *service.SystemProcessService) {
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

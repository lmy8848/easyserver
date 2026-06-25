package api

import (
	"easyserver/internal/executor"
	"easyserver/internal/service"

	"github.com/gin-gonic/gin"
)

// registerMonitorRoutes registers monitor-related routes
func registerMonitorRoutes(protected *gin.RouterGroup, wsGroup *gin.RouterGroup, monitorService *service.MonitorService, jwtSecret string, allowedOrigins []string, devMode bool) {
	handler := NewMonitorHandler(monitorService, jwtSecret, allowedOrigins, devMode)
	protected.GET("/monitor/stats", handler.HandleStats)
	protected.GET("/monitor/history", handler.HandleHistory)
	wsGroup.GET("/monitor", handler.HandleWebSocket)
}

// registerServiceRoutes registers service management routes
func registerServiceRoutes(protected *gin.RouterGroup, wsGroup *gin.RouterGroup, jwtSecret string, allowedOrigins []string, devMode bool) {
	handler := NewServiceHandler(jwtSecret, allowedOrigins, devMode)
	protected.GET("/services", handler.List)
	protected.GET("/services/:name", handler.Get)
	protected.GET("/services/:name/logs", handler.GetLogs)
	protected.POST("/services/:name/start", handler.Start)
	protected.POST("/services/:name/stop", handler.Stop)
	protected.POST("/services/:name/restart", handler.Restart)
	protected.POST("/services/:name/enable", handler.Enable)
	protected.POST("/services/:name/disable", handler.Disable)
	wsGroup.GET("/services/:name/logs", handler.HandleLogsWebSocket)
}

// registerTerminalRoutes registers terminal routes
func registerTerminalRoutes(protected *gin.RouterGroup, wsGroup *gin.RouterGroup, jwtSecret string, auditService *service.AuditService, exec executor.CommandExecutor, allowedOrigins []string, devMode bool) {
	protected.GET("/terminal/:id", func(c *gin.Context) {
		Success(c, nil)
	})
	handler := NewTerminalHandler(jwtSecret, auditService, exec, allowedOrigins, devMode)
	wsGroup.GET("/terminal/:id", handler.HandleWebSocket)
}

// registerSystemRoutes registers system routes
func registerSystemRoutes(protected *gin.RouterGroup) {
	handler := NewSystemHandler()
	protected.GET("/system/ssh-logins", handler.GetSSHLogins)
	protected.GET("/system/ssh-config", handler.GetSystemSSHConfig)
	protected.GET("/system/check-port", handler.CheckPort)
	protected.GET("/system/check-ports", handler.CheckPorts)
}

package api

import (
	"easyserver/internal/audit"
	"easyserver/internal/infra/executor"
	"easyserver/internal/terminal"

	"github.com/gin-gonic/gin"
)

// registerTerminalRoutes registers terminal routes
func registerTerminalRoutes(protected *gin.RouterGroup, wsGroup *gin.RouterGroup, terminalManager *terminal.Manager, jwtSecret string, auditService *audit.Service, allowedOrigins []string, devMode bool) {
	protected.GET("/terminal/:id", func(c *gin.Context) {
		Success(c, nil)
	})
	handler := NewTerminalHandler(terminalManager, jwtSecret, auditService, allowedOrigins, devMode)
	wsGroup.GET("/terminal/:id", handler.HandleWebSocket)
}

// registerSystemRoutes registers system routes
func registerSystemRoutes(protected *gin.RouterGroup, exec executor.CommandExecutor) {
	handler := NewSystemHandler(exec)
	protected.GET("/system/ssh-logins", handler.GetSSHLogins)
	protected.GET("/system/ssh-config", handler.GetSystemSSHConfig)
	protected.GET("/system/check-port", handler.CheckPort)
	protected.GET("/system/check-ports", handler.CheckPorts)
}

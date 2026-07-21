package api

import (
	"easyserver/internal/infra/executor"

	"github.com/gin-gonic/gin"
)

// registerSystemRoutes registers system routes
func registerSystemRoutes(protected *gin.RouterGroup, exec executor.CommandExecutor) {
	handler := NewSystemHandler(exec)
	protected.GET("/system/ssh-logins", handler.GetSSHLogins)
	protected.GET("/system/ssh-config", handler.GetSystemSSHConfig)
	protected.GET("/system/check-port", handler.CheckPort)
	protected.GET("/system/check-ports", handler.CheckPorts)
}

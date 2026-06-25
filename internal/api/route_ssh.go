package api

import (
	"easyserver/internal/service"

	"github.com/gin-gonic/gin"
)

// registerSSHRoutes registers SSH management routes
func registerSSHRoutes(protected *gin.RouterGroup, sshService *service.SSHConfigService) {
	handler := NewSSHHandler(sshService)

	// SSH Config
	protected.GET("/ssh/config", handler.GetConfig)
	protected.PUT("/ssh/config", handler.SaveConfig)
	protected.POST("/ssh/config/test", handler.TestConfig)
	protected.POST("/ssh/config/reload", handler.ReloadSSH)

	// SSH Sessions
	protected.GET("/ssh/sessions", handler.GetSessions)
	protected.POST("/ssh/sessions/:pid/kill", handler.KillSession)

	// SSH Login History
	protected.GET("/ssh/logins", handler.GetLoginHistory)
}

package api

import (
	"easyserver/internal/config"
	"easyserver/internal/service"

	"github.com/gin-gonic/gin"
)

// registerSettingsRoutes registers settings-related routes
func registerSettingsRoutes(protected *gin.RouterGroup, cfg *config.Config, configPath string, alertService *service.AlertService) {
	handler := NewSettingsHandler(cfg, configPath, alertService)
	protected.GET("/settings", handler.GetSettings)
	protected.GET("/settings/system", handler.GetSystemInfo)
	protected.PUT("/settings/server", handler.UpdateServerConfig)
	protected.PUT("/settings/auth", handler.UpdateAuthConfig)
	protected.PUT("/settings/monitor", handler.UpdateMonitorConfig)
	protected.PUT("/settings/audit", handler.UpdateAuditConfig)
	protected.PUT("/settings/notify", handler.UpdateNotifyConfig)
	protected.POST("/settings/notify/test", handler.TestWebhook)
	protected.GET("/alerts/rules", handler.GetAlertRules)
	protected.PUT("/alerts/rules", handler.UpdateAlertRules)
	protected.PUT("/settings/cloud", handler.UpdateCloudConfig)
	protected.POST("/settings/cloud/test", handler.TestCloudConnection)
	protected.POST("/settings/restart", handler.RestartPanel)
}

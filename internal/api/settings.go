package api

import (
	"easyserver/internal/config"

	"github.com/gin-gonic/gin"
)

type SettingsHandler struct {
	cfg *config.Config
}

func NewSettingsHandler(cfg *config.Config) *SettingsHandler {
	return &SettingsHandler{cfg: cfg}
}

// GetSettings returns current settings
func (h *SettingsHandler) GetSettings(c *gin.Context) {
	Success(c, gin.H{
		"server": gin.H{
			"port":          h.cfg.Server.Port,
			"host":          h.cfg.Server.Host,
			"serve_frontend": h.cfg.Server.ServeFrontend,
			"tls_enabled":   h.cfg.Server.TLS.Enabled,
		},
		"auth": gin.H{
			"session_timeout":   h.cfg.Auth.SessionTimeout.String(),
			"idle_timeout":      h.cfg.Auth.IdleTimeout.String(),
			"max_login_attempts": h.cfg.Auth.MaxLoginAttempts,
			"lockout_duration":  h.cfg.Auth.LockoutDuration.String(),
			"rate_limit":        h.cfg.Auth.RateLimit,
			"rate_interval":     h.cfg.Auth.RateInterval.String(),
		},
		"monitor": gin.H{
			"history_retention": h.cfg.Monitor.HistoryRetention.String(),
			"collect_interval":  h.cfg.Monitor.CollectInterval.String(),
		},
		"database": gin.H{
			"path": h.cfg.Database.Path,
		},
		"audit": gin.H{
			"enabled":  h.cfg.Audit.Enabled,
			"log_path": h.cfg.Audit.LogPath,
		},
		"tencentcloud": gin.H{
			"enabled":    h.cfg.TencentCloud.Enabled,
			"region":     h.cfg.TencentCloud.Region,
			"instance_id": h.cfg.TencentCloud.InstanceID,
		},
	})
}

// GetSystemInfo returns system information
func (h *SettingsHandler) GetSystemInfo(c *gin.Context) {
	Success(c, gin.H{
		"version": "1.0.0",
		"go_version": "1.24.4",
		"platform": "linux/amd64",
	})
}

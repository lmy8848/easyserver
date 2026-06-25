package api

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"easyserver/internal/config"
	"easyserver/internal/executor"
	"easyserver/internal/model"
	"easyserver/internal/service"

	"github.com/gin-gonic/gin"
)

// Version information set via ldflags at build time.
// Example: go build -ldflags "-X easyserver/internal/api.Version=1.0.0"
var (
	Version   = "dev"
	GoVersion = runtime.Version()
)

type SettingsHandler struct {
	cfg          *config.Config
	cfgMu        sync.RWMutex
	configPath   string
	alertService *service.AlertService
	executor     executor.CommandExecutor
}

func NewSettingsHandler(cfg *config.Config, configPath string, alertService *service.AlertService, exec executor.CommandExecutor) *SettingsHandler {
	return &SettingsHandler{
		cfg:          cfg,
		configPath:   configPath,
		alertService: alertService,
		executor:     exec,
	}
}

// maskWebhookURL partially masks a webhook URL for display, showing only the scheme and host.
func maskWebhookURL(rawURL string) string {
	if rawURL == "" {
		return ""
	}
	// Show scheme://host/*** to indicate it's configured without leaking the path/params
	for i, ch := range rawURL {
		if ch == '/' && i > 0 && rawURL[i-1] == '/' {
			// Found "//", find the next "/" after host
			for j := i + 2; j < len(rawURL); j++ {
				if rawURL[j] == '/' || rawURL[j] == '?' || rawURL[j] == '#' {
					return rawURL[:j] + "/***"
				}
			}
			// No path after host
			return rawURL
		}
	}
	return "***"
}

// GetSettings returns current settings (sensitive fields are masked)
func (h *SettingsHandler) GetSettings(c *gin.Context) {
	h.cfgMu.RLock()
	defer h.cfgMu.RUnlock()

	// Mask database path: show only the filename
	dbPath := h.cfg.Database.Path
	if idx := strings.LastIndex(dbPath, "/"); idx >= 0 && idx < len(dbPath)-1 {
		dbPath = "/***/" + dbPath[idx+1:]
	} else if dbPath != "" {
		dbPath = "***"
	}

	// Mask webhook URL: show only scheme + host
	webhookURL := maskWebhookURL(h.cfg.Notify.WebhookURL)

	Success(c, gin.H{
		"server": gin.H{
			"port":           h.cfg.Server.Port,
			"host":           h.cfg.Server.Host,
			"serve_frontend": h.cfg.Server.ServeFrontend,
			"tls_enabled":    h.cfg.Server.TLS.Enabled,
		},
		"auth": gin.H{
			"session_timeout":    h.cfg.Auth.SessionTimeout.String(),
			"idle_timeout":       h.cfg.Auth.IdleTimeout.String(),
			"max_login_attempts": h.cfg.Auth.MaxLoginAttempts,
			"lockout_duration":   h.cfg.Auth.LockoutDuration.String(),
			"rate_limit":         h.cfg.Auth.RateLimit,
			"rate_interval":      h.cfg.Auth.RateInterval.String(),
		},
		"monitor": gin.H{
			"history_retention": h.cfg.Monitor.HistoryRetention.String(),
			"collect_interval":  h.cfg.Monitor.CollectInterval.String(),
		},
		"database": gin.H{
			"path": dbPath,
		},
		"audit": gin.H{
			"enabled":  h.cfg.Audit.Enabled,
			"log_path": h.cfg.Audit.LogPath,
		},
		"notify": gin.H{
			"enabled":     h.cfg.Notify.Enabled,
			"webhook_url": webhookURL,
		},
		"tencentcloud": gin.H{
			"enabled":     h.cfg.TencentCloud.Enabled,
			"region":      h.cfg.TencentCloud.Region,
			"instance_id": h.cfg.TencentCloud.InstanceID,
			"has_secret":  h.cfg.TencentCloud.SecretID != "" && h.cfg.TencentCloud.SecretKey != "",
		},
	})
}

// UpdateCloudConfig updates Tencent Cloud configuration
func (h *SettingsHandler) UpdateCloudConfig(c *gin.Context) {
	h.cfgMu.Lock()
	defer h.cfgMu.Unlock()
	var req struct {
		Enabled    *bool   `json:"enabled"`
		SecretID   *string `json:"secret_id"`
		SecretKey  *string `json:"secret_key"`
		Region     *string `json:"region"`
		InstanceID *string `json:"instance_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequest(c, err.Error())
		return
	}

	// Validate region
	validRegions := map[string]bool{
		"ap-guangzhou":     true,
		"ap-shanghai":      true,
		"ap-beijing":       true,
		"ap-nanjing":       true,
		"ap-chengdu":       true,
		"ap-chongqing":     true,
		"ap-hongkong":      true,
		"ap-singapore":     true,
		"ap-tokyo":         true,
		"na-siliconvalley": true,
		"eu-frankfurt":     true,
	}

	if req.Enabled != nil {
		h.cfg.TencentCloud.Enabled = *req.Enabled
	}
	if req.SecretID != nil {
		h.cfg.TencentCloud.SecretID = *req.SecretID
	}
	if req.SecretKey != nil {
		h.cfg.TencentCloud.SecretKey = *req.SecretKey
	}
	if req.Region != nil {
		if !validRegions[*req.Region] {
			BadRequest(c, fmt.Sprintf("无效的区域: %s", *req.Region))
			return
		}
		h.cfg.TencentCloud.Region = *req.Region
	}
	if req.InstanceID != nil {
		h.cfg.TencentCloud.InstanceID = *req.InstanceID
	}

	// Save to config file
	if err := h.saveConfig(); err != nil {
		InternalError(c, fmt.Sprintf("保存配置失败: %v", err))
		return
	}

	Success(c, gin.H{"message": "云配置已更新"})
}

// UpdateServerConfig updates server configuration
func (h *SettingsHandler) UpdateServerConfig(c *gin.Context) {
	h.cfgMu.Lock()
	defer h.cfgMu.Unlock()
	var req struct {
		Port          *int    `json:"port"`
		Host          *string `json:"host"`
		ServeFrontend *bool   `json:"serve_frontend"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequest(c, err.Error())
		return
	}

	requiresRestart := false

	if req.Port != nil {
		if *req.Port < 1 || *req.Port > 65535 {
			BadRequest(c, "端口必须在 1 到 65535 之间")
			return
		}
		// Warn about privileged ports
		if *req.Port < 1024 {
			// Privileged ports require root or special capabilities
			log.Printf("WARNING: Port %d is a privileged port, may require root privileges", *req.Port)
		}
		h.cfg.Server.Port = *req.Port
		requiresRestart = true
	}
	if req.Host != nil {
		host := strings.TrimSpace(*req.Host)
		if host == "" {
			BadRequest(c, "主机不能为空")
			return
		}
		if len(host) > 253 {
			BadRequest(c, "主机名过长（最多 253 个字符）")
			return
		}
		// Warn about localhost-only binding
		if host == "127.0.0.1" || host == "::1" {
			log.Printf("WARNING: Host set to %s, panel will only be accessible from localhost", host)
		}
		h.cfg.Server.Host = host
		requiresRestart = true
	}
	if req.ServeFrontend != nil {
		if !*req.ServeFrontend {
			log.Printf("WARNING: Frontend serving disabled, panel UI will not be accessible via browser")
		}
		h.cfg.Server.ServeFrontend = *req.ServeFrontend
		requiresRestart = true
	}

	// Save to config file
	if err := h.saveConfig(); err != nil {
		InternalError(c, fmt.Sprintf("保存配置失败: %v", err))
		return
	}

	Success(c, gin.H{
		"message":          "服务器配置已更新",
		"requires_restart": requiresRestart,
	})
}

// UpdateAuthConfig updates authentication configuration
func (h *SettingsHandler) UpdateAuthConfig(c *gin.Context) {
	h.cfgMu.Lock()
	defer h.cfgMu.Unlock()
	var req struct {
		SessionTimeout   *string `json:"session_timeout"`
		IdleTimeout      *string `json:"idle_timeout"`
		MaxLoginAttempts *int    `json:"max_login_attempts"`
		LockoutDuration  *string `json:"lockout_duration"`
		RateLimit        *int    `json:"rate_limit"`
		RateInterval     *string `json:"rate_interval"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequest(c, err.Error())
		return
	}

	if req.SessionTimeout != nil {
		d, err := time.ParseDuration(*req.SessionTimeout)
		if err != nil {
			BadRequest(c, fmt.Sprintf("无效的 session_timeout: %v", err))
			return
		}
		// Minimum 5 minutes to prevent lockout
		if d < 5*time.Minute {
			BadRequest(c, "session_timeout 至少为 5 分钟")
			return
		}
		h.cfg.Auth.SessionTimeout = d
	}
	if req.IdleTimeout != nil {
		d, err := time.ParseDuration(*req.IdleTimeout)
		if err != nil {
			BadRequest(c, fmt.Sprintf("无效的 idle_timeout: %v", err))
			return
		}
		// Minimum 1 minute to prevent lockout
		if d < 1*time.Minute {
			BadRequest(c, "idle_timeout 至少为 1 分钟")
			return
		}
		h.cfg.Auth.IdleTimeout = d
	}
	if req.MaxLoginAttempts != nil {
		if *req.MaxLoginAttempts < 3 || *req.MaxLoginAttempts > 100 {
			BadRequest(c, "max_login_attempts 必须在 3 到 100 之间")
			return
		}
		h.cfg.Auth.MaxLoginAttempts = *req.MaxLoginAttempts
	}
	if req.LockoutDuration != nil {
		d, err := time.ParseDuration(*req.LockoutDuration)
		if err != nil {
			BadRequest(c, fmt.Sprintf("无效的 lockout_duration: %v", err))
			return
		}
		// Minimum 1 minute, maximum 24 hours
		if d < 1*time.Minute {
			BadRequest(c, "lockout_duration 至少为 1 分钟")
			return
		}
		if d > 24*time.Hour {
			BadRequest(c, "lockout_duration 不能超过 24 小时")
			return
		}
		h.cfg.Auth.LockoutDuration = d
	}
	if req.RateLimit != nil {
		if *req.RateLimit < 10 {
			BadRequest(c, "rate_limit 至少为 10")
			return
		}
		h.cfg.Auth.RateLimit = *req.RateLimit
	}
	if req.RateInterval != nil {
		d, err := time.ParseDuration(*req.RateInterval)
		if err != nil {
			BadRequest(c, fmt.Sprintf("无效的 rate_interval: %v", err))
			return
		}
		if d < 1*time.Second {
			BadRequest(c, "rate_interval 至少为 1 秒")
			return
		}
		h.cfg.Auth.RateInterval = d
	}

	// Save to config file
	if err := h.saveConfig(); err != nil {
		InternalError(c, fmt.Sprintf("保存配置失败: %v", err))
		return
	}

	Success(c, gin.H{"message": "认证配置已更新"})
}

// UpdateMonitorConfig updates monitor configuration
func (h *SettingsHandler) UpdateMonitorConfig(c *gin.Context) {
	h.cfgMu.Lock()
	defer h.cfgMu.Unlock()
	var req struct {
		HistoryRetention *string `json:"history_retention"`
		CollectInterval  *string `json:"collect_interval"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequest(c, err.Error())
		return
	}

	if req.HistoryRetention != nil {
		d, err := time.ParseDuration(*req.HistoryRetention)
		if err != nil {
			BadRequest(c, fmt.Sprintf("无效的 history_retention: %v", err))
			return
		}
		if d < 1*time.Minute {
			BadRequest(c, "history_retention 至少为 1 分钟")
			return
		}
		if d > 8760*time.Hour {
			BadRequest(c, "history_retention 不能超过 1 年")
			return
		}
		h.cfg.Monitor.HistoryRetention = d
	}
	if req.CollectInterval != nil {
		d, err := time.ParseDuration(*req.CollectInterval)
		if err != nil {
			BadRequest(c, fmt.Sprintf("无效的 collect_interval: %v", err))
			return
		}
		if d < 1*time.Second {
			BadRequest(c, "collect_interval 至少为 1 秒")
			return
		}
		if d > 1*time.Hour {
			BadRequest(c, "collect_interval 不能超过 1 小时")
			return
		}
		h.cfg.Monitor.CollectInterval = d
	}

	// Save to config file
	if err := h.saveConfig(); err != nil {
		InternalError(c, fmt.Sprintf("保存配置失败: %v", err))
		return
	}

	Success(c, gin.H{"message": "监控配置已更新"})
}

// UpdateAuditConfig updates audit configuration
func (h *SettingsHandler) UpdateAuditConfig(c *gin.Context) {
	h.cfgMu.Lock()
	defer h.cfgMu.Unlock()
	var req struct {
		Enabled *bool `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequest(c, err.Error())
		return
	}

	if req.Enabled != nil {
		h.cfg.Audit.Enabled = *req.Enabled
	}

	// Save to config file
	if err := h.saveConfig(); err != nil {
		InternalError(c, fmt.Sprintf("保存配置失败: %v", err))
		return
	}

	Success(c, gin.H{"message": "审计配置已更新"})
}

// validateWebhookURL validates a webhook URL format
func validateWebhookURL(rawURL string) error {
	if rawURL == "" {
		return fmt.Errorf("webhook URL cannot be empty")
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid webhook URL: %v", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("webhook URL must use http or https scheme")
	}
	if u.Host == "" {
		return fmt.Errorf("webhook URL must have a valid host")
	}
	return nil
}

// UpdateNotifyConfig updates notification configuration
func (h *SettingsHandler) UpdateNotifyConfig(c *gin.Context) {
	h.cfgMu.Lock()
	defer h.cfgMu.Unlock()
	var req struct {
		Enabled    *bool   `json:"enabled"`
		WebhookURL *string `json:"webhook_url"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequest(c, err.Error())
		return
	}

	if req.Enabled != nil {
		h.cfg.Notify.Enabled = *req.Enabled
	}
	if req.WebhookURL != nil {
		webhookURL := strings.TrimSpace(*req.WebhookURL)
		if webhookURL != "" {
			if err := validateWebhookURL(webhookURL); err != nil {
				BadRequest(c, err.Error())
				return
			}
		}
		h.cfg.Notify.WebhookURL = webhookURL
	}

	// Save to config file
	if err := h.saveConfig(); err != nil {
		InternalError(c, fmt.Sprintf("保存配置失败: %v", err))
		return
	}

	Success(c, gin.H{"message": "通知配置已更新"})
}

// TestWebhook sends a test notification to the configured webhook
func (h *SettingsHandler) TestWebhook(c *gin.Context) {
	h.cfgMu.RLock()
	defer h.cfgMu.RUnlock()
	if h.cfg.Notify.WebhookURL == "" {
		BadRequest(c, "请先配置 Webhook URL")
		return
	}

	if err := validateWebhookURL(h.cfg.Notify.WebhookURL); err != nil {
		BadRequest(c, err.Error())
		return
	}

	notifyService := service.NewNotifyService(h.cfg.Notify.WebhookURL, true)
	if err := notifyService.TestWebhook(); err != nil {
		Error(c, http.StatusUnprocessableEntity, CodeBadRequest, fmt.Sprintf("测试通知失败: %v", err))
		return
	}

	Success(c, gin.H{"message": "测试通知已发送"})
}

// TestCloudConnection tests the Tencent Cloud connection
func (h *SettingsHandler) TestCloudConnection(c *gin.Context) {
	h.cfgMu.RLock()
	defer h.cfgMu.RUnlock()
	if h.cfg.TencentCloud.SecretID == "" || h.cfg.TencentCloud.SecretKey == "" {
		BadRequest(c, "请先配置 SecretID 和 SecretKey")
		return
	}

	cloudService, err := service.NewCloudService(
		h.cfg.TencentCloud.SecretID,
		h.cfg.TencentCloud.SecretKey,
		h.cfg.TencentCloud.Region,
		h.cfg.TencentCloud.InstanceID,
	)
	if err != nil {
		InternalError(c, fmt.Sprintf("创建云客户端失败: %v", err))
		return
	}

	// Try to get instances to verify connection
	instances, err := cloudService.GetInstances(c.Request.Context())
	if err != nil {
		InternalError(c, fmt.Sprintf("连接失败: %v", err))
		return
	}

	Success(c, gin.H{
		"message":        "连接成功",
		"instance_count": len(instances),
	})
}

// saveConfig saves the current config to the config.yaml file
func (h *SettingsHandler) saveConfig() error {
	return config.Save(h.cfg, h.configPath)
}

// GetAlertRules returns the current alert rules
func (h *SettingsHandler) GetAlertRules(c *gin.Context) {
	h.cfgMu.RLock()
	defer h.cfgMu.RUnlock()
	Success(c, gin.H{"rules": h.cfg.Alerts.Rules})
}

// UpdateAlertRules updates the alert rules configuration
func (h *SettingsHandler) UpdateAlertRules(c *gin.Context) {
	h.cfgMu.Lock()
	defer h.cfgMu.Unlock()
	var req struct {
		Rules []config.AlertRuleConfig `json:"rules"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequest(c, "无效的请求: "+err.Error())
		return
	}

	// Limit number of rules to prevent abuse
	if len(req.Rules) > 50 {
		BadRequest(c, "告警规则过多（最多 50 条）")
		return
	}

	// Validate rules
	for _, rule := range req.Rules {
		if rule.Name == "" {
			BadRequest(c, "规则名称不能为空")
			return
		}
		validMetrics := map[string]bool{
			"cpu_percent": true, "mem_percent": true, "disk_percent": true,
			"load_1m": true, "load_5m": true, "load_15m": true,
		}
		if !validMetrics[rule.Metric] {
			BadRequest(c, "无效的指标: "+rule.Metric)
			return
		}
		if rule.Threshold <= 0 || rule.Threshold > 100 {
			BadRequest(c, "阈值必须在 0 到 100 之间")
			return
		}
		if rule.Duration < 0 {
			BadRequest(c, "持续时间不能为负数")
			return
		}
	}

	h.cfg.Alerts.Rules = req.Rules
	if err := h.saveConfig(); err != nil {
		InternalError(c, "保存配置失败: "+err.Error())
		return
	}

	// Update AlertService at runtime
	if h.alertService != nil {
		var alertRules []model.AlertRule
		for i, rule := range req.Rules {
			alertRules = append(alertRules, model.AlertRule{
				ID:        int64(i + 1),
				Name:      rule.Name,
				Metric:    rule.Metric,
				Threshold: rule.Threshold,
				Duration:  rule.Duration,
				Enabled:   rule.Enabled,
			})
		}
		h.alertService.SetRules(alertRules)
	}

	Success(c, gin.H{"message": "告警规则已更新", "rules": h.cfg.Alerts.Rules})
}

// GetSystemInfo returns system information
func (h *SettingsHandler) GetSystemInfo(c *gin.Context) {
	Success(c, gin.H{
		"version":    Version,
		"go_version": GoVersion,
		"platform":   fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH),
	})
}

// RestartPanel restarts the backend service
func (h *SettingsHandler) RestartPanel(c *gin.Context) {
	// Validate TLS configuration before restart
	if h.cfg.Server.TLS.Enabled {
		if h.cfg.Server.TLS.CertFile == "" || h.cfg.Server.TLS.KeyFile == "" {
			BadRequest(c, "TLS 已启用但未配置证书/密钥文件")
			return
		}
		// Verify cert file exists
		if _, err := os.Stat(h.cfg.Server.TLS.CertFile); os.IsNotExist(err) {
			BadRequest(c, fmt.Sprintf("TLS 证书文件不存在: %s", h.cfg.Server.TLS.CertFile))
			return
		}
		// Verify key file exists
		if _, err := os.Stat(h.cfg.Server.TLS.KeyFile); os.IsNotExist(err) {
			BadRequest(c, fmt.Sprintf("TLS 密钥文件不存在: %s", h.cfg.Server.TLS.KeyFile))
			return
		}
	}

	// Return success first, then restart
	go func() {
		time.Sleep(1 * time.Second)
		log.Println("settings: restarting panel...")

		// Resolve the actual executable path
		execPath, err := os.Executable()
		if err != nil {
			log.Printf("settings: failed to resolve executable path: %v", err)
			return
		}
		execPath, err = filepath.EvalSymlinks(execPath)
		if err != nil {
			log.Printf("settings: failed to resolve symlink: %v", err)
			return
		}

		// Build restart command using actual paths (shell-escaped to prevent injection)
		safeConfigPath := "'" + strings.ReplaceAll(h.configPath, "'", "'\\''") + "'"
		safeExecPath := "'" + strings.ReplaceAll(execPath, "'", "'\\''") + "'"
		cmdStr := fmt.Sprintf("sleep 2 && nohup %s -config %s > /dev/null 2>&1 &",
			safeExecPath, safeConfigPath)
		if _, _, err := h.executor.RunCombined(context.Background(), "sh", "-c", cmdStr); err != nil {
			log.Printf("settings: failed to start restart command: %v", err)
			return
		}

		time.Sleep(500 * time.Millisecond)
		os.Exit(0)
	}()
	Success(c, gin.H{"message": "面板正在重启..."})
}

func registerSettingsRoutes(protected *gin.RouterGroup, cfg *config.Config, configPath string, alertService *service.AlertService, exec executor.CommandExecutor) {
	handler := NewSettingsHandler(cfg, configPath, alertService, exec)
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

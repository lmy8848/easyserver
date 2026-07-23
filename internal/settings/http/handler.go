package http

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"easyserver/internal/alert"
	"easyserver/internal/cloud"
	"easyserver/internal/httpx"
	"easyserver/internal/httpx/middleware"
	"easyserver/internal/infra"
	"easyserver/internal/infra/apperror"
	"easyserver/internal/infra/config"
	"easyserver/internal/infra/executor"
	"easyserver/internal/notify"

	"github.com/gin-gonic/gin"
)

type MonitorUpdater interface {
	SetInterval(interval time.Duration)
	SetRetention(retention time.Duration)
}

type SettingsHandler struct {
	cfg            *config.Config
	cfgMu          sync.RWMutex
	configPath     string
	alertService   *alert.Service
	monitorService MonitorUpdater
	executor       executor.CommandExecutor
	sig            *infra.Signal
}

func NewSettingsHandler(cfg *config.Config, configPath string, alertService *alert.Service, exec executor.CommandExecutor, sig *infra.Signal) *SettingsHandler {
	return &SettingsHandler{
		cfg:          cfg,
		configPath:   configPath,
		alertService: alertService,
		executor:     exec,
		sig:          sig,
	}
}

func (h *SettingsHandler) SetMonitorService(m MonitorUpdater) {
	h.monitorService = m
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

	httpx.Success(c, gin.H{
		"server": gin.H{
			"port":           h.cfg.Server.Port,
			"host":           h.cfg.Server.Host,
			"serve_frontend": h.cfg.Server.ServeFrontend,
			"tls": gin.H{
				"enabled":   h.cfg.Server.TLS.Enabled,
				"cert_info": certInfoFromConfig(h.cfg),
			},
			"domain":               h.cfg.Server.Domain,
			"redirect_mode":        h.cfg.Server.RedirectMode,
			"www_handling":         h.cfg.Server.WwwHandling,
			"assets_rate_limit":    h.cfg.Server.AssetsRateLimit,
			"assets_rate_interval": h.cfg.Server.AssetsRateInterval.String(),
			"max_upload_size":      h.cfg.Server.MaxUploadSize,
			"turnstile": gin.H{
				"site_key":            h.cfg.Server.Turnstile.SiteKey,
				"secret_key":          h.cfg.Server.Turnstile.SecretKey,
				"enable_login":        h.cfg.Server.Turnstile.EnableLogin,
				"enable_qr_login":     h.cfg.Server.Turnstile.EnableQRLogin,
				"enable_public_share": h.cfg.Server.Turnstile.EnablePublicShare,
			},
		},
		"auth": gin.H{
			"session_timeout":       h.cfg.Auth.SessionTimeout.String(),
			"idle_timeout":          h.cfg.Auth.IdleTimeout.String(),
			"max_login_attempts":    h.cfg.Auth.MaxLoginAttempts,
			"lockout_duration":      h.cfg.Auth.LockoutDuration.String(),
			"rate_limit":            h.cfg.Auth.RateLimit,
			"rate_interval":         h.cfg.Auth.RateInterval.String(),
			"login_rate_limit":      h.cfg.Auth.LoginRateLimit,
			"login_rate_interval":   h.cfg.Auth.LoginRateInterval.String(),
			"allow_multi_session":   h.cfg.Auth.AllowMultiSession,
			"mobile_device_binding": h.cfg.Auth.MobileDeviceBinding,
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
		c.Error(apperror.ErrBadRequest.Wrap(err))
		return
	}

	middleware.AuditSummary(c, "更新云配置")

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
			c.Error(apperror.ErrBadRequest.WithMessage(fmt.Sprintf("无效的区域: %s", *req.Region)))
			return
		}
		h.cfg.TencentCloud.Region = *req.Region
	}
	if req.InstanceID != nil {
		h.cfg.TencentCloud.InstanceID = *req.InstanceID
	}

	// Save to config file
	if err := h.saveConfig(); err != nil {
		c.Error(apperror.ErrInternal.WithMessage(fmt.Sprintf("保存配置失败: %v", err)))
		return
	}

	httpx.Success(c, gin.H{"message": "云配置已更新"})
}

// UpdateServerConfig updates server configuration
func (h *SettingsHandler) UpdateServerConfig(c *gin.Context) {
	h.cfgMu.Lock()
	defer h.cfgMu.Unlock()
	var req struct {
		Port               *int    `json:"port"`
		Host               *string `json:"host"`
		ServeFrontend      *bool   `json:"serve_frontend"`
		Domain             *string `json:"domain"`
		RedirectMode       *string `json:"redirect_mode"`
		WwwHandling        *string `json:"www_handling"`
		MaxUploadSize      *int64  `json:"max_upload_size"`
		AssetsRateLimit    *int    `json:"assets_rate_limit"`
		AssetsRateInterval *string `json:"assets_rate_interval"`
		Turnstile          *struct {
			SiteKey           *string `json:"site_key"`
			SecretKey         *string `json:"secret_key"`
			EnableLogin       *bool   `json:"enable_login"`
			EnableQRLogin     *bool   `json:"enable_qr_login"`
			EnablePublicShare *bool   `json:"enable_public_share"`
		} `json:"turnstile"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperror.ErrBadRequest.Wrap(err))
		return
	}

	middleware.AuditSummary(c, "更新服务器配置")

	requiresRestart := false

	if req.Port != nil {
		if *req.Port < 1 || *req.Port > 65535 {
			c.Error(apperror.ErrBadRequest.WithMessage("端口必须在 1 到 65535 之间"))
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
			c.Error(apperror.ErrBadRequest.WithMessage("主机不能为空"))
			return
		}
		if len(host) > 253 {
			c.Error(apperror.ErrBadRequest.WithMessage("主机名过长（最多 253 个字符）"))
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
	if req.Domain != nil {
		h.cfg.Server.Domain = strings.TrimSpace(*req.Domain)
	}
	if req.RedirectMode != nil {
		h.cfg.Server.RedirectMode = *req.RedirectMode
	}
	if req.WwwHandling != nil {
		h.cfg.Server.WwwHandling = *req.WwwHandling
	}
	if req.MaxUploadSize != nil {
		if *req.MaxUploadSize < 0 {
			c.Error(apperror.ErrBadRequest.WithMessage("max_upload_size 不能为负数"))
			return
		}
		if *req.MaxUploadSize > 4<<30 { // 4 GB max
			c.Error(apperror.ErrBadRequest.WithMessage("max_upload_size 不能超过 4GB"))
			return
		}
		h.cfg.Server.MaxUploadSize = *req.MaxUploadSize
	}
	if req.AssetsRateLimit != nil {
		if *req.AssetsRateLimit < 100 || *req.AssetsRateLimit > 100000 {
			c.Error(apperror.ErrBadRequest.WithMessage("assets_rate_limit 必须在 100 到 100000 之间"))
			return
		}
		h.cfg.Server.AssetsRateLimit = *req.AssetsRateLimit
	}
	if req.AssetsRateInterval != nil {
		d, err := time.ParseDuration(*req.AssetsRateInterval)
		if err != nil {
			c.Error(apperror.ErrBadRequest.WithMessage(fmt.Sprintf("无效的 assets_rate_interval: %v", err)))
			return
		}
		if d < 1*time.Second || d > 1*time.Hour {
			c.Error(apperror.ErrBadRequest.WithMessage("assets_rate_interval 必须在 1s 到 1h 之间"))
			return
		}
		h.cfg.Server.AssetsRateInterval = d
	}
	if req.Turnstile != nil {
		if req.Turnstile.SiteKey != nil {
			h.cfg.Server.Turnstile.SiteKey = *req.Turnstile.SiteKey
		}
		if req.Turnstile.SecretKey != nil {
			h.cfg.Server.Turnstile.SecretKey = *req.Turnstile.SecretKey
		}
		if req.Turnstile.EnableLogin != nil {
			h.cfg.Server.Turnstile.EnableLogin = *req.Turnstile.EnableLogin
		}
		if req.Turnstile.EnableQRLogin != nil {
			h.cfg.Server.Turnstile.EnableQRLogin = *req.Turnstile.EnableQRLogin
		}
		if req.Turnstile.EnablePublicShare != nil {
			h.cfg.Server.Turnstile.EnablePublicShare = *req.Turnstile.EnablePublicShare
		}
	}

	// Save to config file
	if err := h.saveConfig(); err != nil {
		c.Error(apperror.ErrInternal.WithMessage(fmt.Sprintf("保存配置失败: %v", err)))
		return
	}

	// Sync assets rate limiter at runtime
	if req.AssetsRateLimit != nil || req.AssetsRateInterval != nil {
		if rl := middleware.GetRateLimiter("assets"); rl != nil {
			rl.UpdateRate(h.cfg.Server.AssetsRateLimit, h.cfg.Server.AssetsRateInterval)
		}
	}

	httpx.Success(c, gin.H{
		"message":          "服务器配置已更新",
		"requires_restart": requiresRestart,
	})
}

// tlsCertInfo holds parsed certificate metadata for API responses.
type tlsCertInfo struct {
	Domain    string `json:"domain"`
	Issuer    string `json:"issuer"`
	ExpiresAt string `json:"expires_at"`
}

// UpdateTLSConfig updates TLS certificate configuration.
// Accepts PEM-encoded cert/key content, validates the pair, writes to disk,
// and updates the config file. Requires restart to take effect.
func (h *SettingsHandler) UpdateTLSConfig(c *gin.Context) {
	h.cfgMu.Lock()
	defer h.cfgMu.Unlock()

	var req struct {
		Enabled     *bool   `json:"enabled"`
		CertContent *string `json:"cert_content"`
		KeyContent  *string `json:"key_content"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperror.ErrBadRequest.Wrap(err))
		return
	}

	middleware.AuditSummary(c, "更新 TLS 配置")

	if req.Enabled == nil {
		c.Error(apperror.ErrBadRequest.WithMessage("缺少 enabled 字段"))
		return
	}

	// If disabling, just update the flag
	if !*req.Enabled {
		h.cfg.Server.TLS.Enabled = false
		if err := h.saveConfig(); err != nil {
			c.Error(apperror.ErrInternal.WithMessage(fmt.Sprintf("保存配置失败: %v", err)))
			return
		}
		httpx.Success(c, gin.H{
			"message":          "TLS 已禁用",
			"requires_restart": true,
			"cert_info":        nil,
		})
		return
	}

	// Enabling TLS — cert and key content are required
	if req.CertContent == nil || req.KeyContent == nil ||
		strings.TrimSpace(*req.CertContent) == "" || strings.TrimSpace(*req.KeyContent) == "" {
		c.Error(apperror.ErrBadRequest.WithMessage("启用 TLS 需要提供证书和私钥内容"))
		return
	}

	// Validate PEM format and that cert matches key
	certPEM := []byte(strings.TrimSpace(*req.CertContent))
	keyPEM := []byte(strings.TrimSpace(*req.KeyContent))

	if _, err := tls.X509KeyPair(certPEM, keyPEM); err != nil {
		c.Error(apperror.ErrBadRequest.WithMessage(fmt.Sprintf("证书与私钥不配对或格式无效: %v", err)))
		return
	}

	// Determine cert storage directory (next to config file)
	configDir := filepath.Dir(h.configPath)
	certDir := filepath.Join(configDir, "certs")
	if err := os.MkdirAll(certDir, 0700); err != nil {
		c.Error(apperror.ErrInternal.WithMessage(fmt.Sprintf("创建证书目录失败: %v", err)))
		return
	}

	certPath := filepath.Join(certDir, "server.crt")
	keyPath := filepath.Join(certDir, "server.key")

	// Atomic write: write to temp file then rename
	certTmp := certPath + ".tmp"
	if err := os.WriteFile(certTmp, certPEM, 0644); err != nil {
		c.Error(apperror.ErrInternal.WithMessage(fmt.Sprintf("写入证书文件失败: %v", err)))
		return
	}
	if err := os.Rename(certTmp, certPath); err != nil {
		os.Remove(certTmp)
		c.Error(apperror.ErrInternal.WithMessage(fmt.Sprintf("更新证书文件失败: %v", err)))
		return
	}

	keyTmp := keyPath + ".tmp"
	if err := os.WriteFile(keyTmp, keyPEM, 0600); err != nil {
		c.Error(apperror.ErrInternal.WithMessage(fmt.Sprintf("写入私钥文件失败: %v", err)))
		return
	}
	if err := os.Rename(keyTmp, keyPath); err != nil {
		os.Remove(keyTmp)
		c.Error(apperror.ErrInternal.WithMessage(fmt.Sprintf("更新私钥文件失败: %v", err)))
		return
	}

	// Update config
	h.cfg.Server.TLS.Enabled = true
	h.cfg.Server.TLS.CertFile = certPath
	h.cfg.Server.TLS.KeyFile = keyPath

	if err := h.saveConfig(); err != nil {
		c.Error(apperror.ErrInternal.WithMessage(fmt.Sprintf("保存配置失败: %v", err)))
		return
	}

	// Parse cert info for response
	certInfo := parseCertInfo(certPEM)

	httpx.Success(c, gin.H{
		"message":          "TLS 证书已更新，需要重启面板生效",
		"requires_restart": true,
		"cert_info":        certInfo,
	})
}

// parseCertInfo extracts domain, issuer, and expiry from a PEM-encoded certificate.
func parseCertInfo(certPEM []byte) *tlsCertInfo {
	block, _ := pem.Decode(certPEM)
	if block == nil {
		return nil
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil
	}
	domain := cert.Subject.CommonName
	if len(cert.DNSNames) > 0 {
		domain = cert.DNSNames[0]
	}
	return &tlsCertInfo{
		Domain:    domain,
		Issuer:    cert.Issuer.CommonName,
		ExpiresAt: cert.NotAfter.Format(time.RFC3339),
	}
}

// certInfoFromConfig loads and parses the currently configured TLS certificate.
func certInfoFromConfig(cfg *config.Config) *tlsCertInfo {
	if !cfg.Server.TLS.Enabled || cfg.Server.TLS.CertFile == "" {
		return nil
	}
	data, err := os.ReadFile(cfg.Server.TLS.CertFile)
	if err != nil {
		return nil
	}
	return parseCertInfo(data)
}

// UpdateAuthConfig updates authentication configuration
func (h *SettingsHandler) UpdateAuthConfig(c *gin.Context) {
	h.cfgMu.Lock()
	defer h.cfgMu.Unlock()
	var req struct {
		SessionTimeout      *string `json:"session_timeout"`
		IdleTimeout         *string `json:"idle_timeout"`
		MaxLoginAttempts    *int    `json:"max_login_attempts"`
		LockoutDuration     *string `json:"lockout_duration"`
		RateLimit           *int    `json:"rate_limit"`
		RateInterval        *string `json:"rate_interval"`
		LoginRateLimit      *int    `json:"login_rate_limit"`
		LoginRateInterval   *string `json:"login_rate_interval"`
		AllowMultiSession   *bool   `json:"allow_multi_session"`
		MobileDeviceBinding *bool   `json:"mobile_device_binding"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperror.ErrBadRequest.Wrap(err))
		return
	}

	middleware.AuditSummary(c, "更新认证配置")

	if req.SessionTimeout != nil {
		d, err := time.ParseDuration(*req.SessionTimeout)
		if err != nil {
			c.Error(apperror.ErrBadRequest.WithMessage(fmt.Sprintf("无效的 session_timeout: %v", err)))
			return
		}
		// Minimum 5 minutes to prevent lockout
		if d < 5*time.Minute {
			c.Error(apperror.ErrBadRequest.WithMessage("session_timeout 至少为 5 分钟"))
			return
		}
		h.cfg.Auth.SessionTimeout = d
	}
	if req.IdleTimeout != nil {
		d, err := time.ParseDuration(*req.IdleTimeout)
		if err != nil {
			c.Error(apperror.ErrBadRequest.WithMessage(fmt.Sprintf("无效的 idle_timeout: %v", err)))
			return
		}
		// Minimum 1 minute to prevent lockout
		if d < 1*time.Minute {
			c.Error(apperror.ErrBadRequest.WithMessage("idle_timeout 至少为 1 分钟"))
			return
		}
		h.cfg.Auth.IdleTimeout = d
	}
	if req.MaxLoginAttempts != nil {
		if *req.MaxLoginAttempts < 3 || *req.MaxLoginAttempts > 100 {
			c.Error(apperror.ErrBadRequest.WithMessage("max_login_attempts 必须在 3 到 100 之间"))
			return
		}
		h.cfg.Auth.MaxLoginAttempts = *req.MaxLoginAttempts
	}
	if req.LockoutDuration != nil {
		d, err := time.ParseDuration(*req.LockoutDuration)
		if err != nil {
			c.Error(apperror.ErrBadRequest.WithMessage(fmt.Sprintf("无效的 lockout_duration: %v", err)))
			return
		}
		// Minimum 1 minute, maximum 24 hours
		if d < 1*time.Minute {
			c.Error(apperror.ErrBadRequest.WithMessage("lockout_duration 至少为 1 分钟"))
			return
		}
		if d > 24*time.Hour {
			c.Error(apperror.ErrBadRequest.WithMessage("lockout_duration 不能超过 24 小时"))
			return
		}
		h.cfg.Auth.LockoutDuration = d
	}
	if req.RateLimit != nil {
		if *req.RateLimit < 10 {
			c.Error(apperror.ErrBadRequest.WithMessage("rate_limit 至少为 10"))
			return
		}
		h.cfg.Auth.RateLimit = *req.RateLimit
	}
	if req.RateInterval != nil {
		d, err := time.ParseDuration(*req.RateInterval)
		if err != nil {
			c.Error(apperror.ErrBadRequest.WithMessage(fmt.Sprintf("无效的 rate_interval: %v", err)))
			return
		}
		if d < 1*time.Second {
			c.Error(apperror.ErrBadRequest.WithMessage("rate_interval 至少为 1 秒"))
			return
		}
		h.cfg.Auth.RateInterval = d
	}

	if req.LoginRateLimit != nil {
		if *req.LoginRateLimit < 1 || *req.LoginRateLimit > 100 {
			c.Error(apperror.ErrBadRequest.WithMessage("login_rate_limit 必须在 1 到 100 之间"))
			return
		}
		h.cfg.Auth.LoginRateLimit = *req.LoginRateLimit
	}
	if req.LoginRateInterval != nil {
		d, err := time.ParseDuration(*req.LoginRateInterval)
		if err != nil {
			c.Error(apperror.ErrBadRequest.WithMessage(fmt.Sprintf("无效的 login_rate_interval: %v", err)))
			return
		}
		if d < 1*time.Second || d > 1*time.Hour {
			c.Error(apperror.ErrBadRequest.WithMessage("login_rate_interval 必须在 1s 到 1h 之间"))
			return
		}
		h.cfg.Auth.LoginRateInterval = d
	}
	if req.AllowMultiSession != nil {
		h.cfg.Auth.AllowMultiSession = *req.AllowMultiSession
	}
	if req.MobileDeviceBinding != nil {
		h.cfg.Auth.MobileDeviceBinding = *req.MobileDeviceBinding
	}

	// Sync API rate limiter at runtime
	if req.RateLimit != nil || req.RateInterval != nil {
		if rl := middleware.GetRateLimiter("api"); rl != nil {
			rl.UpdateRate(h.cfg.Auth.RateLimit, h.cfg.Auth.RateInterval)
		}
	}
	// Sync login rate limiter at runtime
	if req.LoginRateLimit != nil || req.LoginRateInterval != nil {
		if rl := middleware.GetRateLimiter("login"); rl != nil {
			rl.UpdateRate(h.cfg.Auth.LoginRateLimit, h.cfg.Auth.LoginRateInterval)
		}
	}

	// Save to config file
	if err := h.saveConfig(); err != nil {
		c.Error(apperror.ErrInternal.WithMessage(fmt.Sprintf("保存配置失败: %v", err)))
		return
	}

	httpx.Success(c, gin.H{"message": "认证配置已更新"})
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
		c.Error(apperror.ErrBadRequest.Wrap(err))
		return
	}

	middleware.AuditSummary(c, "更新监控配置")

	if req.HistoryRetention != nil {
		d, err := time.ParseDuration(*req.HistoryRetention)
		if err != nil {
			c.Error(apperror.ErrBadRequest.WithMessage(fmt.Sprintf("无效的 history_retention: %v", err)))
			return
		}
		if d < 1*time.Minute {
			c.Error(apperror.ErrBadRequest.WithMessage("history_retention 至少为 1 分钟"))
			return
		}
		if d > 8760*time.Hour {
			c.Error(apperror.ErrBadRequest.WithMessage("history_retention 不能超过 1 年"))
			return
		}
		h.cfg.Monitor.HistoryRetention = d
		if h.monitorService != nil {
			h.monitorService.SetRetention(d)
		}
	}
	if req.CollectInterval != nil {
		d, err := time.ParseDuration(*req.CollectInterval)
		if err != nil {
			c.Error(apperror.ErrBadRequest.WithMessage(fmt.Sprintf("无效的 collect_interval: %v", err)))
			return
		}
		if d < 1*time.Second {
			c.Error(apperror.ErrBadRequest.WithMessage("collect_interval 至少为 1 秒"))
			return
		}
		if d > 1*time.Hour {
			c.Error(apperror.ErrBadRequest.WithMessage("collect_interval 不能超过 1 小时"))
			return
		}
		h.cfg.Monitor.CollectInterval = d
		if h.monitorService != nil {
			h.monitorService.SetInterval(d)
		}
	}

	// Save to config file
	if err := h.saveConfig(); err != nil {
		c.Error(apperror.ErrInternal.WithMessage(fmt.Sprintf("保存配置失败: %v", err)))
		return
	}

	httpx.Success(c, gin.H{"message": "监控配置已更新"})
}

// UpdateAuditConfig updates audit configuration
func (h *SettingsHandler) UpdateAuditConfig(c *gin.Context) {
	h.cfgMu.Lock()
	defer h.cfgMu.Unlock()
	var req struct {
		Enabled *bool `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperror.ErrBadRequest.Wrap(err))
		return
	}

	middleware.AuditSummary(c, "更新审计配置")

	if req.Enabled != nil {
		h.cfg.Audit.Enabled = *req.Enabled
	}

	// Save to config file
	if err := h.saveConfig(); err != nil {
		c.Error(apperror.ErrInternal.WithMessage(fmt.Sprintf("保存配置失败: %v", err)))
		return
	}

	httpx.Success(c, gin.H{"message": "审计配置已更新"})
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
		c.Error(apperror.ErrBadRequest.Wrap(err))
		return
	}

	middleware.AuditSummary(c, "更新通知配置")

	if req.Enabled != nil {
		h.cfg.Notify.Enabled = *req.Enabled
	}
	if req.WebhookURL != nil {
		webhookURL := strings.TrimSpace(*req.WebhookURL)
		if webhookURL != "" {
			if err := validateWebhookURL(webhookURL); err != nil {
				c.Error(apperror.ErrBadRequest.Wrap(err))
				return
			}
		}
		h.cfg.Notify.WebhookURL = webhookURL
	}

	// Save to config file
	if err := h.saveConfig(); err != nil {
		c.Error(apperror.ErrInternal.WithMessage(fmt.Sprintf("保存配置失败: %v", err)))
		return
	}

	httpx.Success(c, gin.H{"message": "通知配置已更新"})
}

// TestWebhook sends a test notification to the configured webhook
func (h *SettingsHandler) TestWebhook(c *gin.Context) {
	middleware.AuditSummary(c, "测试通知 Webhook")
	h.cfgMu.RLock()
	defer h.cfgMu.RUnlock()
	if h.cfg.Notify.WebhookURL == "" {
		c.Error(apperror.ErrBadRequest.WithMessage("请先配置 Webhook URL"))
		return
	}

	if err := validateWebhookURL(h.cfg.Notify.WebhookURL); err != nil {
		c.Error(apperror.ErrBadRequest.Wrap(err))
		return
	}

	notifyService := notify.NewService(h.cfg.Notify.WebhookURL, true)
	if err := notifyService.TestWebhook(); err != nil {
		c.Error(apperror.ErrInternal.WithMessage(fmt.Sprintf("测试通知失败: %v", err)))
		return
	}

	httpx.Success(c, gin.H{"message": "测试通知已发送"})
}

// TestCloudConnection tests the Tencent Cloud connection
func (h *SettingsHandler) TestCloudConnection(c *gin.Context) {
	middleware.AuditSummary(c, "测试云连接")
	h.cfgMu.RLock()
	defer h.cfgMu.RUnlock()
	if h.cfg.TencentCloud.SecretID == "" || h.cfg.TencentCloud.SecretKey == "" {
		c.Error(apperror.ErrBadRequest.WithMessage("请先配置 SecretID 和 SecretKey"))
		return
	}

	cloudService, err := cloud.NewService(
		h.cfg.TencentCloud.SecretID,
		h.cfg.TencentCloud.SecretKey,
		h.cfg.TencentCloud.Region,
		h.cfg.TencentCloud.InstanceID,
	)
	if err != nil {
		c.Error(apperror.ErrInternal.WithMessage(fmt.Sprintf("创建云客户端失败: %v", err)))
		return
	}

	// Try to get instances to verify connection
	instances, err := cloudService.GetInstances(c.Request.Context())
	if err != nil {
		c.Error(apperror.ErrInternal.WithMessage(fmt.Sprintf("连接失败: %v", err)))
		return
	}

	httpx.Success(c, gin.H{
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
	httpx.Success(c, gin.H{"rules": h.cfg.Alerts.Rules})
}

// UpdateAlertRules updates the alert rules configuration
func (h *SettingsHandler) UpdateAlertRules(c *gin.Context) {
	h.cfgMu.Lock()
	defer h.cfgMu.Unlock()
	var req struct {
		Rules []config.AlertRuleConfig `json:"rules"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperror.ErrBadRequest.WithMessage("无效的请求: " + err.Error()))
		return
	}

	middleware.AuditSummary(c, "更新告警规则")

	// Limit number of rules to prevent abuse
	if len(req.Rules) > 50 {
		c.Error(apperror.ErrBadRequest.WithMessage("告警规则过多（最多 50 条）"))
		return
	}

	// Validate rules
	for _, rule := range req.Rules {
		if rule.Name == "" {
			c.Error(apperror.ErrBadRequest.WithMessage("规则名称不能为空"))
			return
		}
		validMetrics := map[string]bool{
			"cpu_percent": true, "mem_percent": true, "disk_percent": true,
			"load_1m": true, "load_5m": true, "load_15m": true,
		}
		if !validMetrics[rule.Metric] {
			c.Error(apperror.ErrBadRequest.WithMessage("无效的指标: " + rule.Metric))
			return
		}
		if rule.Threshold <= 0 || rule.Threshold > 100 {
			c.Error(apperror.ErrBadRequest.WithMessage("阈值必须在 0 到 100 之间"))
			return
		}
		if rule.Duration < 0 {
			c.Error(apperror.ErrBadRequest.WithMessage("持续时间不能为负数"))
			return
		}
	}

	h.cfg.Alerts.Rules = req.Rules
	if err := h.saveConfig(); err != nil {
		c.Error(apperror.ErrInternal.WithMessage("保存配置失败: " + err.Error()))
		return
	}

	// Update AlertService at runtime
	if h.alertService != nil {
		var alertRules []alert.AlertRule
		for i, rule := range req.Rules {
			alertRules = append(alertRules, alert.AlertRule{
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

	httpx.Success(c, gin.H{"message": "告警规则已更新", "rules": h.cfg.Alerts.Rules})
}

// GetSystemInfo returns system information
func (h *SettingsHandler) GetSystemInfo(c *gin.Context) {
	httpx.Success(c, gin.H{
		"version": infra.Version,
	})
}

// RestartPanel restarts the backend service.
// When force=true (e.g. port change), the listener is closed so the child
// process creates a fresh one on the new address.
func (h *SettingsHandler) RestartPanel(c *gin.Context) {
	middleware.AuditSummary(c, "重启面板")

	var req struct {
		Force *bool `json:"force"`
	}
	_ = c.ShouldBindJSON(&req) // optional body, ignore parse errors
	force := req.Force != nil && *req.Force

	// Return success first, then restart.
	infra.Go(func() {
		time.Sleep(1 * time.Second)
		h.sig.Request(infra.RestartOpts{
			ConfigPath: h.configPath,
			DevMode:    h.cfg.Server.DevMode,
			Force:      force,
		})
	})
	httpx.Success(c, gin.H{"message": "面板正在重启..."})
}

func RegisterRoutes(protected *gin.RouterGroup, cfg *config.Config, configPath string, alertService *alert.Service, monitorSvc MonitorUpdater, exec executor.CommandExecutor, sig *infra.Signal) {
	handler := NewSettingsHandler(cfg, configPath, alertService, exec, sig)
	handler.SetMonitorService(monitorSvc)
	protected.GET("/settings", handler.GetSettings)
	protected.GET("/settings/system", handler.GetSystemInfo)
	protected.PUT("/settings/server", handler.UpdateServerConfig)
	protected.PUT("/settings/tls", handler.UpdateTLSConfig)
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

package http

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"easyserver/internal/alert"
	"easyserver/internal/cloud"
	"easyserver/internal/httpx"
	"easyserver/internal/httpx/middleware"
	"easyserver/internal/infra"
	"easyserver/internal/infra/apperror"
	"easyserver/internal/infra/config"
	"easyserver/internal/notify"

	"github.com/gin-gonic/gin"
)

type MonitorUpdater interface {
	SetInterval(interval time.Duration)
	SetRetention(retention time.Duration)
}

type SettingsHandler struct {
	store          *config.Store
	alertService   *alert.Service
	monitorService MonitorUpdater
	sig            *infra.Signal
}

func NewSettingsHandler(store *config.Store, alertService *alert.Service, sig *infra.Signal) *SettingsHandler {
	return &SettingsHandler{
		store:        store,
		alertService: alertService,
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
	cfg := h.store.Get()

	// Mask database path: show only the filename
	dbPath := cfg.Database.Path
	if idx := strings.LastIndex(dbPath, "/"); idx >= 0 && idx < len(dbPath)-1 {
		dbPath = "/***/" + dbPath[idx+1:]
	} else if dbPath != "" {
		dbPath = "***"
	}

	// Mask webhook URL: show only scheme + host
	webhookURL := maskWebhookURL(cfg.Notify.WebhookURL)

	httpx.Success(c, gin.H{
		"server": gin.H{
			"port":           cfg.Server.Port,
			"host":           cfg.Server.Host,
			"serve_frontend": cfg.Server.ServeFrontend,
			"tls": gin.H{
				"enabled":   cfg.Server.TLS.Enabled,
				"cert_info": certInfoFromConfig(cfg),
			},
			"domain":               cfg.Server.Domain,
			"redirect_mode":        cfg.Server.RedirectMode,
			"www_handling":         cfg.Server.WwwHandling,
			"assets_rate_limit":    cfg.Server.AssetsRateLimit,
			"assets_rate_interval": cfg.Server.AssetsRateInterval.String(),
			"max_upload_size":      cfg.Server.MaxUploadSize,
			"turnstile": gin.H{
				"site_key":            cfg.Server.Turnstile.SiteKey,
				"secret_key":          cfg.Server.Turnstile.SecretKey,
				"enable_login":        cfg.Server.Turnstile.EnableLogin,
				"enable_qr_login":     cfg.Server.Turnstile.EnableQRLogin,
				"enable_public_share": cfg.Server.Turnstile.EnablePublicShare,
			},
		},
		"auth": gin.H{
			"session_timeout":       int(cfg.Auth.SessionTimeout.Seconds()),
			"idle_timeout":          int(cfg.Auth.IdleTimeout.Seconds()),
			"max_login_attempts":    cfg.Auth.MaxLoginAttempts,
			"lockout_duration":      int(cfg.Auth.LockoutDuration.Seconds()),
			"rate_limit":            cfg.Auth.RateLimit,
			"rate_interval":         int(cfg.Auth.RateInterval.Seconds()),
			"login_rate_limit":      cfg.Auth.LoginRateLimit,
			"login_rate_interval":   int(cfg.Auth.LoginRateInterval.Seconds()),
			"allow_multi_session":   cfg.Auth.AllowMultiSession,
			"mobile_device_binding": cfg.Auth.MobileDeviceBinding,
		},
		"monitor": gin.H{
			"history_retention": int(cfg.Monitor.HistoryRetention.Hours()),
			"collect_interval":  int(cfg.Monitor.CollectInterval.Seconds()),
		},
		"database": gin.H{
			"path": dbPath,
		},
		"audit": gin.H{
			"enabled":  cfg.Audit.Enabled,
			"log_path": cfg.Audit.LogPath,
		},
		"notify": gin.H{
			"enabled":     cfg.Notify.Enabled,
			"webhook_url": webhookURL,
		},
		"tencentcloud": gin.H{
			"enabled":     cfg.TencentCloud.Enabled,
			"region":      cfg.TencentCloud.Region,
			"instance_id": cfg.TencentCloud.InstanceID,
			"has_secret":  cfg.TencentCloud.SecretID != "" && cfg.TencentCloud.SecretKey != "",
		},
		"features": gin.H{
			"file_preview": cfg.Features.FilePreview,
			"fim":          cfg.Features.FIM,
		},
	})
}

// UpdateFeaturesConfig updates optional feature toggles.
func (h *SettingsHandler) UpdateFeaturesConfig(c *gin.Context) {
	var req struct {
		FilePreview *bool `json:"file_preview"`
		FIM         *bool `json:"fim"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperror.ErrBadRequest.Wrap(err))
		return
	}
	middleware.AuditSummary(c, "更新功能开关")
	h.store.Update(func(cfg *config.Config) {
		if req.FilePreview != nil {
			cfg.Features.FilePreview = *req.FilePreview
		}
		if req.FIM != nil {
			cfg.Features.FIM = *req.FIM
		}
	})
	if err := h.saveConfig(); err != nil {
		c.Error(apperror.ErrInternal.WithMessage(fmt.Sprintf("保存配置失败: %v", err)))
		return
	}
	httpx.Success(c, gin.H{"message": "功能开关已更新"})
}

// UpdateCloudConfig updates Tencent Cloud configuration
func (h *SettingsHandler) UpdateCloudConfig(c *gin.Context) {
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

	if req.Region != nil && !validRegions[*req.Region] {
		c.Error(apperror.ErrBadRequest.WithMessage("无效的区域: " + *req.Region))
		return
	}

	h.store.Update(func(cfg *config.Config) {
		if req.Enabled != nil {
			cfg.TencentCloud.Enabled = *req.Enabled
		}
		if req.SecretID != nil {
			cfg.TencentCloud.SecretID = *req.SecretID
		}
		if req.SecretKey != nil {
			cfg.TencentCloud.SecretKey = *req.SecretKey
		}
		if req.Region != nil {
			cfg.TencentCloud.Region = *req.Region
		}
		if req.InstanceID != nil {
			cfg.TencentCloud.InstanceID = *req.InstanceID
		}
	})

	// Save to config file
	if err := h.saveConfig(); err != nil {
		c.Error(apperror.ErrInternal.WithMessage(fmt.Sprintf("保存配置失败: %v", err)))
		return
	}

	httpx.Success(c, gin.H{"message": "云配置已更新"})
}

// UpdateServerConfig updates server configuration
func (h *SettingsHandler) UpdateServerConfig(c *gin.Context) {
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

	// ---- 校验（基于请求体，不依赖当前配置，提前统一做） ----
	if req.Port != nil && (*req.Port < 1 || *req.Port > 65535) {
		c.Error(apperror.ErrBadRequest.WithMessage("端口必须在 1 到 65535 之间"))
		return
	}
	// Warn about privileged ports
	if req.Port != nil && *req.Port < 1024 {
		log.Printf("WARNING: Port %d is a privileged port, may require root privileges", *req.Port)
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
	}
	if req.MaxUploadSize != nil && (*req.MaxUploadSize < 0 || *req.MaxUploadSize > 4<<30) {
		c.Error(apperror.ErrBadRequest.WithMessage("max_upload_size 必须在 0 到 4GB 之间"))
		return
	}
	if req.AssetsRateLimit != nil && (*req.AssetsRateLimit < 100 || *req.AssetsRateLimit > 100000) {
		c.Error(apperror.ErrBadRequest.WithMessage("assets_rate_limit 必须在 100 到 100000 之间"))
		return
	}
	var assetsRateInterval time.Duration
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
		assetsRateInterval = d
	}

	if req.ServeFrontend != nil && !*req.ServeFrontend {
		log.Printf("WARNING: Frontend serving disabled, panel UI will not be accessible via browser")
	}
	// ---- 应用修改（副本上原子替换，读方看到完整一致的新配置） ----
	h.store.Update(func(cfg *config.Config) {
		if req.Port != nil {
			cfg.Server.Port = *req.Port
		}
		if req.Host != nil {
			cfg.Server.Host = strings.TrimSpace(*req.Host)
		}
		if req.ServeFrontend != nil {
			cfg.Server.ServeFrontend = *req.ServeFrontend
		}
		if req.Domain != nil {
			cfg.Server.Domain = strings.TrimSpace(*req.Domain)
		}
		if req.RedirectMode != nil {
			cfg.Server.RedirectMode = *req.RedirectMode
		}
		if req.WwwHandling != nil {
			cfg.Server.WwwHandling = *req.WwwHandling
		}
		if req.MaxUploadSize != nil {
			cfg.Server.MaxUploadSize = *req.MaxUploadSize
		}
		if req.AssetsRateLimit != nil {
			cfg.Server.AssetsRateLimit = *req.AssetsRateLimit
		}
		if req.AssetsRateInterval != nil {
			cfg.Server.AssetsRateInterval = assetsRateInterval
		}
		if req.Turnstile != nil {
			if req.Turnstile.SiteKey != nil {
				cfg.Server.Turnstile.SiteKey = *req.Turnstile.SiteKey
			}
			if req.Turnstile.SecretKey != nil {
				cfg.Server.Turnstile.SecretKey = *req.Turnstile.SecretKey
			}
			if req.Turnstile.EnableLogin != nil {
				cfg.Server.Turnstile.EnableLogin = *req.Turnstile.EnableLogin
			}
			if req.Turnstile.EnableQRLogin != nil {
				cfg.Server.Turnstile.EnableQRLogin = *req.Turnstile.EnableQRLogin
			}
			if req.Turnstile.EnablePublicShare != nil {
				cfg.Server.Turnstile.EnablePublicShare = *req.Turnstile.EnablePublicShare
			}
		}
	})

	// 重启类字段（端口/监听地址/前端托管）变化时提示重启
	requiresRestart := req.Port != nil || req.Host != nil || req.ServeFrontend != nil

	// Save to config file
	if err := h.saveConfig(); err != nil {
		c.Error(apperror.ErrInternal.WithMessage(fmt.Sprintf("保存配置失败: %v", err)))
		return
	}

	// Sync assets rate limiter at runtime
	if req.AssetsRateLimit != nil || req.AssetsRateInterval != nil {
		if rl := middleware.GetRateLimiter("assets"); rl != nil {
			cur := h.store.Get()
			rl.UpdateRate(cur.Server.AssetsRateLimit, cur.Server.AssetsRateInterval)
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
		h.store.Update(func(cfg *config.Config) {
			cfg.Server.TLS.Enabled = false
		})
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
	configDir := filepath.Dir(h.store.Get().Path)
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
	h.store.Update(func(cfg *config.Config) {
		cfg.Server.TLS.Enabled = true
		cfg.Server.TLS.CertFile = certPath
		cfg.Server.TLS.KeyFile = keyPath
	})

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
	var req struct {
		SessionTimeout      *int  `json:"session_timeout"`
		IdleTimeout         *int  `json:"idle_timeout"`
		MaxLoginAttempts    *int  `json:"max_login_attempts"`
		LockoutDuration     *int  `json:"lockout_duration"`
		RateLimit           *int  `json:"rate_limit"`
		RateInterval        *int  `json:"rate_interval"`
		LoginRateLimit      *int  `json:"login_rate_limit"`
		LoginRateInterval   *int  `json:"login_rate_interval"`
		AllowMultiSession   *bool `json:"allow_multi_session"`
		MobileDeviceBinding *bool `json:"mobile_device_binding"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperror.ErrBadRequest.Wrap(err))
		return
	}

	middleware.AuditSummary(c, "更新认证配置")

	// ---- 校验（基于请求体） ----
	if req.SessionTimeout != nil && *req.SessionTimeout < 300 {
		c.Error(apperror.ErrBadRequest.WithMessage("session_timeout 至少为 300 秒（5分钟）"))
		return
	}
	if req.IdleTimeout != nil && *req.IdleTimeout < 60 {
		c.Error(apperror.ErrBadRequest.WithMessage("idle_timeout 至少为 60 秒（1分钟）"))
		return
	}
	if req.MaxLoginAttempts != nil && (*req.MaxLoginAttempts < 3 || *req.MaxLoginAttempts > 100) {
		c.Error(apperror.ErrBadRequest.WithMessage("max_login_attempts 必须在 3 到 100 之间"))
		return
	}
	if req.LockoutDuration != nil && (*req.LockoutDuration < 60 || *req.LockoutDuration > 86400) {
		c.Error(apperror.ErrBadRequest.WithMessage("lockout_duration 必须在 60 秒到 86400 秒之间"))
		return
	}
	if req.RateLimit != nil && *req.RateLimit < 10 {
		c.Error(apperror.ErrBadRequest.WithMessage("rate_limit 至少为 10"))
		return
	}
	if req.RateInterval != nil && *req.RateInterval < 1 {
		c.Error(apperror.ErrBadRequest.WithMessage("rate_interval 至少为 1 秒"))
		return
	}
	if req.LoginRateLimit != nil && (*req.LoginRateLimit < 1 || *req.LoginRateLimit > 100) {
		c.Error(apperror.ErrBadRequest.WithMessage("login_rate_limit 必须在 1 到 100 之间"))
		return
	}
	if req.LoginRateInterval != nil && (*req.LoginRateInterval < 1 || *req.LoginRateInterval > 3600) {
		c.Error(apperror.ErrBadRequest.WithMessage("login_rate_interval 必须在 1 秒到 3600 秒之间"))
		return
	}

	// ---- 应用修改 ----
	h.store.Update(func(cfg *config.Config) {
		if req.SessionTimeout != nil {
			cfg.Auth.SessionTimeout = time.Duration(*req.SessionTimeout) * time.Second
		}
		if req.IdleTimeout != nil {
			cfg.Auth.IdleTimeout = time.Duration(*req.IdleTimeout) * time.Second
		}
		if req.MaxLoginAttempts != nil {
			cfg.Auth.MaxLoginAttempts = *req.MaxLoginAttempts
		}
		if req.LockoutDuration != nil {
			cfg.Auth.LockoutDuration = time.Duration(*req.LockoutDuration) * time.Second
		}
		if req.RateLimit != nil {
			cfg.Auth.RateLimit = *req.RateLimit
		}
		if req.RateInterval != nil {
			cfg.Auth.RateInterval = time.Duration(*req.RateInterval) * time.Second
		}
		if req.LoginRateLimit != nil {
			cfg.Auth.LoginRateLimit = *req.LoginRateLimit
		}
		if req.LoginRateInterval != nil {
			cfg.Auth.LoginRateInterval = time.Duration(*req.LoginRateInterval) * time.Second
		}
		if req.AllowMultiSession != nil {
			cfg.Auth.AllowMultiSession = *req.AllowMultiSession
		}
		if req.MobileDeviceBinding != nil {
			cfg.Auth.MobileDeviceBinding = *req.MobileDeviceBinding
		}
	})

	// Sync API rate limiter at runtime
	if req.RateLimit != nil || req.RateInterval != nil {
		if rl := middleware.GetRateLimiter("api"); rl != nil {
			cur := h.store.Get()
			rl.UpdateRate(cur.Auth.RateLimit, cur.Auth.RateInterval)
		}
	}
	// Sync login rate limiter at runtime
	if req.LoginRateLimit != nil || req.LoginRateInterval != nil {
		if rl := middleware.GetRateLimiter("login"); rl != nil {
			cur := h.store.Get()
			rl.UpdateRate(cur.Auth.LoginRateLimit, cur.Auth.LoginRateInterval)
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
	var req struct {
		HistoryRetention *int `json:"history_retention"`
		CollectInterval  *int `json:"collect_interval"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperror.ErrBadRequest.Wrap(err))
		return
	}

	middleware.AuditSummary(c, "更新监控配置")

	// ---- 校验（基于请求体） ----
	if req.HistoryRetention != nil && (*req.HistoryRetention < 24 || *req.HistoryRetention > 8760) {
		c.Error(apperror.ErrBadRequest.WithMessage("history_retention 必须在 24 小时到 8760 小时（1年）之间"))
		return
	}
	if req.CollectInterval != nil && (*req.CollectInterval < 1 || *req.CollectInterval > 300) {
		c.Error(apperror.ErrBadRequest.WithMessage("collect_interval 必须在 1 秒到 300 秒（5分钟）之间"))
		return
	}

	// ---- 应用修改 ----
	var retention, interval time.Duration
	h.store.Update(func(cfg *config.Config) {
		if req.HistoryRetention != nil {
			retention = time.Duration(*req.HistoryRetention) * time.Hour
			cfg.Monitor.HistoryRetention = retention
		}
		if req.CollectInterval != nil {
			interval = time.Duration(*req.CollectInterval) * time.Second
			cfg.Monitor.CollectInterval = interval
		}
	})
	// 运行时热更新监控节律（monitor 服务内部原子更新，无需重启）
	if req.HistoryRetention != nil && h.monitorService != nil {
		h.monitorService.SetRetention(retention)
	}
	if req.CollectInterval != nil && h.monitorService != nil {
		h.monitorService.SetInterval(interval)
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
	var req struct {
		Enabled *bool `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperror.ErrBadRequest.Wrap(err))
		return
	}

	middleware.AuditSummary(c, "更新审计配置")

	h.store.Update(func(cfg *config.Config) {
		if req.Enabled != nil {
			cfg.Audit.Enabled = *req.Enabled
		}
	})

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
		return errors.New("webhook URL cannot be empty")
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid webhook URL: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return errors.New("webhook URL must use http or https scheme")
	}
	if u.Host == "" {
		return errors.New("webhook URL must have a valid host")
	}
	return nil
}

// UpdateNotifyConfig updates notification configuration
func (h *SettingsHandler) UpdateNotifyConfig(c *gin.Context) {
	var req struct {
		Enabled    *bool   `json:"enabled"`
		WebhookURL *string `json:"webhook_url"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperror.ErrBadRequest.Wrap(err))
		return
	}

	middleware.AuditSummary(c, "更新通知配置")

	if req.WebhookURL != nil && strings.TrimSpace(*req.WebhookURL) != "" {
		if err := validateWebhookURL(strings.TrimSpace(*req.WebhookURL)); err != nil {
			c.Error(apperror.ErrBadRequest.Wrap(err))
			return
		}
	}

	h.store.Update(func(cfg *config.Config) {
		if req.Enabled != nil {
			cfg.Notify.Enabled = *req.Enabled
		}
		if req.WebhookURL != nil {
			cfg.Notify.WebhookURL = strings.TrimSpace(*req.WebhookURL)
		}
	})

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
	cfg := h.store.Get()
	if cfg.Notify.WebhookURL == "" {
		c.Error(apperror.ErrBadRequest.WithMessage("请先配置 Webhook URL"))
		return
	}

	if err := validateWebhookURL(cfg.Notify.WebhookURL); err != nil {
		c.Error(apperror.ErrBadRequest.Wrap(err))
		return
	}

	notifyService := notify.NewService(h.store)
	if err := notifyService.TestWebhook(); err != nil {
		c.Error(apperror.ErrInternal.WithMessage(fmt.Sprintf("测试通知失败: %v", err)))
		return
	}

	httpx.Success(c, gin.H{"message": "测试通知已发送"})
}

// TestCloudConnection tests the Tencent Cloud connection
func (h *SettingsHandler) TestCloudConnection(c *gin.Context) {
	middleware.AuditSummary(c, "测试云连接")
	cfg := h.store.Get()
	if cfg.TencentCloud.SecretID == "" || cfg.TencentCloud.SecretKey == "" {
		c.Error(apperror.ErrBadRequest.WithMessage("请先配置 SecretID 和 SecretKey"))
		return
	}

	cloudService, err := cloud.NewService(
		cfg.TencentCloud.SecretID,
		cfg.TencentCloud.SecretKey,
		cfg.TencentCloud.Region,
		cfg.TencentCloud.InstanceID,
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

// saveConfig saves the current config back to its source file (cfg.Path).
func (h *SettingsHandler) saveConfig() error {
	return config.Save(h.store.Get())
}

// GetAlertRules returns the current alert rules
func (h *SettingsHandler) GetAlertRules(c *gin.Context) {
	httpx.Success(c, gin.H{"rules": h.store.Get().Alerts.Rules})
}

// UpdateAlertRules updates the alert rules configuration
func (h *SettingsHandler) UpdateAlertRules(c *gin.Context) {
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

	h.store.Update(func(cfg *config.Config) {
		cfg.Alerts.Rules = req.Rules
	})
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

	httpx.Success(c, gin.H{"message": "告警规则已更新", "rules": h.store.Get().Alerts.Rules})
}

// GetSystemInfo returns system information
func (h *SettingsHandler) GetSystemInfo(c *gin.Context) {
	httpx.Success(c, gin.H{
		"version":  infra.DisplayVersion(),
		"build_id": infra.BuildID,
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
	cfg := h.store.Get()
	infra.Go(func() {
		time.Sleep(1 * time.Second)
		h.sig.Request(infra.RestartOpts{
			ConfigPath: cfg.Path,
			DevMode:    cfg.Server.DevMode,
			Force:      force,
		})
	})
	httpx.Success(c, gin.H{"message": "面板正在重启..."})
}

func RegisterRoutes(protected *gin.RouterGroup, store *config.Store, alertService *alert.Service, monitorSvc MonitorUpdater, sig *infra.Signal) {
	handler := NewSettingsHandler(store, alertService, sig)
	handler.SetMonitorService(monitorSvc)
	protected.GET("/settings", handler.GetSettings)
	protected.GET("/settings/system", handler.GetSystemInfo)
	protected.PUT("/settings/server", handler.UpdateServerConfig)
	protected.PUT("/settings/tls", handler.UpdateTLSConfig)
	protected.PUT("/settings/auth", handler.UpdateAuthConfig)
	protected.PUT("/settings/monitor", handler.UpdateMonitorConfig)
	protected.PUT("/settings/audit", handler.UpdateAuditConfig)
	protected.PUT("/settings/notify", handler.UpdateNotifyConfig)
	protected.PUT("/settings/features", handler.UpdateFeaturesConfig)
	protected.POST("/settings/notify/test", handler.TestWebhook)
	protected.GET("/alerts/rules", handler.GetAlertRules)
	protected.PUT("/alerts/rules", handler.UpdateAlertRules)
	protected.PUT("/settings/cloud", handler.UpdateCloudConfig)
	protected.POST("/settings/cloud/test", handler.TestCloudConnection)
	protected.POST("/settings/restart", handler.RestartPanel)
}

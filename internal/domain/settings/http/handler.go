package http

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"log"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"easyserver/internal/domain/alert"
	"easyserver/internal/domain/notify"
	"easyserver/internal/httpx"
	"easyserver/internal/httpx/middleware"
	"easyserver/internal/infra"
	"easyserver/internal/infra/config"
	"easyserver/internal/infra/errx"
	"easyserver/internal/infra/logger"

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
func (h *SettingsHandler) GetSettings(c *gin.Context) (any, error) {
	cfg := h.store.Get()

	// Mask webhook URL: show only scheme + host
	webhookURL := maskWebhookURL(cfg.Notify.WebhookURL)

	return gin.H{
		"server": gin.H{
			"port": cfg.Server.Port,
			"host": cfg.Server.Host,
			"tls": gin.H{
				"enabled":   cfg.Server.TLS.Enabled,
				"cert_file": cfg.Server.TLS.CertFile,
				"key_file":  cfg.Server.TLS.KeyFile,
				"cert_info": certInfoFromConfig(cfg),
			},
			"domain":               cfg.Server.Domain,
			"force_domain":         cfg.Server.ForceDomain,
			"allowed_origins":      cfg.Server.AllowedOrigins,
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
			"session_timeout":          int(cfg.Auth.SessionTimeout.Seconds()),
			"idle_timeout":             int(cfg.Auth.IdleTimeout.Seconds()),
			"max_login_attempts":       cfg.Auth.MaxLoginAttempts,
			"lockout_duration":         int(cfg.Auth.LockoutDuration.Seconds()),
			"rate_limit":               cfg.Auth.RateLimit,
			"rate_interval":            int(cfg.Auth.RateInterval.Seconds()),
			"login_rate_limit":         cfg.Auth.LoginRateLimit,
			"login_rate_interval":      int(cfg.Auth.LoginRateInterval.Seconds()),
			"allow_multi_session":      cfg.Auth.AllowMultiSession,
			"ip_whitelist":             cfg.Auth.IPWhitelist,
			"session_cleanup_interval": int(cfg.Auth.SessionCleanupInterval.Seconds()),
		},
		"monitor": gin.H{
			"history_retention": int(cfg.Monitor.HistoryRetention.Hours()),
			"collect_interval":  int(cfg.Monitor.CollectInterval.Seconds()),
		},
		"audit": gin.H{
			"retention_days": cfg.Audit.RetentionDays,
		},
		"notify": gin.H{
			"enabled":     cfg.Notify.Enabled,
			"webhook_url": webhookURL,
		},
		"features": gin.H{
			"fim": cfg.Features.FIM,
		},
		"logs": gin.H{
			"level":       cfg.Logs.Level,
			"path":        logger.LogPath(),
			"format":      cfg.Logs.Format,
			"max_size_mb": cfg.Logs.MaxSizeMB,
			"max_files":   cfg.Logs.MaxFiles,
		},
	}, nil
}

// UpdateFeaturesConfig updates optional feature toggles.
func (h *SettingsHandler) UpdateFeaturesConfig(c *gin.Context) (any, error) {
	req, err := httpx.BindJSON[struct {
		FIM *bool `json:"fim"`
	}](c)
	if err != nil {
		return nil, err
	}
	middleware.AuditSummary(c, "更新功能开关")
	h.store.Update(func(cfg *config.Config) {
		if req.FIM != nil {
			cfg.Features.FIM = *req.FIM
		}
	})
	if err := h.saveConfig(); err != nil {
		return nil, errx.Internal("保存配置失败: %w", err)
	}
	return gin.H{"message": "功能开关已更新"}, nil
}

// UpdateServerConfig updates server configuration
func (h *SettingsHandler) UpdateServerConfig(c *gin.Context) (any, error) {
	req, err := httpx.BindJSON[struct {
		Port               *int      `json:"port"`
		Host               *string   `json:"host"`
		Domain             *string   `json:"domain"`
		ForceDomain        *bool     `json:"force_domain"`
		AllowedOrigins     *[]string `json:"allowed_origins"`
		MaxUploadSize      *int64    `json:"max_upload_size"`
		AssetsRateLimit    *int      `json:"assets_rate_limit"`
		AssetsRateInterval *string   `json:"assets_rate_interval"`
		Turnstile          *struct {
			SiteKey           *string `json:"site_key"`
			SecretKey         *string `json:"secret_key"`
			EnableLogin       *bool   `json:"enable_login"`
			EnableQRLogin     *bool   `json:"enable_qr_login"`
			EnablePublicShare *bool   `json:"enable_public_share"`
		} `json:"turnstile"`
	}](c)
	if err != nil {
		return nil, err
	}

	middleware.AuditSummary(c, "更新服务器配置")

	// ---- 校验（基于请求体，不依赖当前配置，提前统一做） ----
	if req.Port != nil && (*req.Port < 1 || *req.Port > 65535) {
		return nil, errx.BadRequest("端口必须在 1 到 65535 之间")
	}
	// Warn about privileged ports
	if req.Port != nil && *req.Port < 1024 {
		log.Printf("WARNING: Port %d is a privileged port, may require root privileges", *req.Port)
	}
	if req.Host != nil {
		host := strings.TrimSpace(*req.Host)
		if host == "" {
			return nil, errx.BadRequest("主机不能为空")
		}
		if len(host) > 253 {
			return nil, errx.BadRequest("主机名过长（最多 253 个字符）")
		}
		// Warn about localhost-only binding
		if host == "127.0.0.1" || host == "::1" {
			log.Printf("WARNING: Host set to %s, panel will only be accessible from localhost", host)
		}
	}
	if req.MaxUploadSize != nil && (*req.MaxUploadSize < 0 || *req.MaxUploadSize > 4<<30) {
		return nil, errx.BadRequest("max_upload_size 必须在 0 到 4GB 之间")
	}
	if req.AssetsRateLimit != nil && (*req.AssetsRateLimit < 100 || *req.AssetsRateLimit > 100000) {
		return nil, errx.BadRequest("assets_rate_limit 必须在 100 到 100000 之间")
	}
	var assetsRateInterval time.Duration
	if req.AssetsRateInterval != nil {
		d, err := time.ParseDuration(*req.AssetsRateInterval)
		if err != nil {
			return nil, errx.BadRequest("无效的 assets_rate_interval: %w", err)
		}
		if d < 1*time.Second || d > 1*time.Hour {
			return nil, errx.BadRequest("assets_rate_interval 必须在 1s 到 1h 之间")
		}
		assetsRateInterval = d
	}

	// ---- 应用修改（副本上原子替换，读方看到完整一致的新配置） ----
	h.store.Update(func(cfg *config.Config) {
		if req.Port != nil {
			cfg.Server.Port = *req.Port
		}
		if req.Host != nil {
			cfg.Server.Host = strings.TrimSpace(*req.Host)
		}
		if req.Domain != nil {
			cfg.Server.Domain = strings.TrimSpace(*req.Domain)
		}
		if req.ForceDomain != nil {
			cfg.Server.ForceDomain = *req.ForceDomain
		}
		if req.AllowedOrigins != nil {
			cleaned := make([]string, 0, len(*req.AllowedOrigins))
			for _, o := range *req.AllowedOrigins {
				if o = strings.TrimSpace(o); o != "" {
					cleaned = append(cleaned, o)
				}
			}
			cfg.Server.AllowedOrigins = cleaned
		}
		if req.MaxUploadSize != nil {
			cfg.Server.MaxUploadSize = *req.MaxUploadSize
		}
		if req.AssetsRateLimit != nil {
			cfg.Server.AssetsRateLimit = *req.AssetsRateLimit
		}
		if req.AssetsRateInterval != nil {
			cfg.Server.AssetsRateInterval = config.Duration(assetsRateInterval)
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

	// 重启类字段（端口/监听地址）变化时提示重启
	requiresRestart := req.Port != nil || req.Host != nil

	// Save to config file
	if err := h.saveConfig(); err != nil {
		return nil, errx.Internal("保存配置失败: %w", err)
	}

	// Sync assets rate limiter at runtime
	if req.AssetsRateLimit != nil || req.AssetsRateInterval != nil {
		if rl := middleware.GetRateLimiter("assets"); rl != nil {
			cur := h.store.Get()
			rl.UpdateRate(cur.Server.AssetsRateLimit, cur.Server.AssetsRateInterval.Duration())
		}
	}

	return gin.H{
		"message":          "服务器配置已更新",
		"requires_restart": requiresRestart,
	}, nil
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
func (h *SettingsHandler) UpdateTLSConfig(c *gin.Context) (any, error) {
	req, err := httpx.BindJSON[struct {
		Enabled     *bool   `json:"enabled"`
		CertContent *string `json:"cert_content"`
		KeyContent  *string `json:"key_content"`
	}](c)
	if err != nil {
		return nil, err
	}

	middleware.AuditSummary(c, "更新 TLS 配置")

	if req.Enabled == nil {
		return nil, errx.BadRequest("缺少 enabled 字段")
	}

	// If disabling, just update the flag
	if !*req.Enabled {
		h.store.Update(func(cfg *config.Config) {
			cfg.Server.TLS.Enabled = false
		})
		if err := h.saveConfig(); err != nil {
			return nil, errx.Internal("保存配置失败: %w", err)
		}
		return gin.H{
			"message":          "TLS 已禁用",
			"requires_restart": true,
			"cert_info":        nil,
		}, nil
	}

	// Enabling TLS — cert and key content are required
	if req.CertContent == nil || req.KeyContent == nil ||
		strings.TrimSpace(*req.CertContent) == "" || strings.TrimSpace(*req.KeyContent) == "" {
		return nil, errx.BadRequest("启用 TLS 需要提供证书和私钥内容")
	}

	// Validate PEM format and that cert matches key
	certPEM := []byte(strings.TrimSpace(*req.CertContent))
	keyPEM := []byte(strings.TrimSpace(*req.KeyContent))

	if _, err := tls.X509KeyPair(certPEM, keyPEM); err != nil {
		return nil, errx.BadRequest("证书与私钥不配对或格式无效: %w", err)
	}

	// Determine cert storage directory (next to config file)
	configDir := filepath.Dir(h.store.Get().Path)
	certDir := filepath.Join(configDir, "certs")
	if err := os.MkdirAll(certDir, 0700); err != nil {
		return nil, errx.Internal("创建证书目录失败: %w", err)
	}

	certPath := filepath.Join(certDir, "server.crt")
	keyPath := filepath.Join(certDir, "server.key")

	// Atomic write: write to temp file then rename
	certTmp := certPath + ".tmp"
	if err := os.WriteFile(certTmp, certPEM, 0644); err != nil {
		return nil, errx.Internal("写入证书文件失败: %w", err)
	}
	if err := os.Rename(certTmp, certPath); err != nil {
		os.Remove(certTmp)
		return nil, errx.Internal("更新证书文件失败: %w", err)
	}

	keyTmp := keyPath + ".tmp"
	if err := os.WriteFile(keyTmp, keyPEM, 0600); err != nil {
		return nil, errx.Internal("写入私钥文件失败: %w", err)
	}
	if err := os.Rename(keyTmp, keyPath); err != nil {
		os.Remove(keyTmp)
		return nil, errx.Internal("更新私钥文件失败: %w", err)
	}

	// Update config
	h.store.Update(func(cfg *config.Config) {
		cfg.Server.TLS.Enabled = true
		cfg.Server.TLS.CertFile = certPath
		cfg.Server.TLS.KeyFile = keyPath
	})

	if err := h.saveConfig(); err != nil {
		return nil, errx.Internal("保存配置失败: %w", err)
	}

	// Parse cert info for response
	certInfo := parseCertInfo(certPEM)

	return gin.H{
		"message":          "TLS 证书已更新，需要重启面板生效",
		"requires_restart": true,
		"cert_info":        certInfo,
	}, nil
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
func (h *SettingsHandler) UpdateAuthConfig(c *gin.Context) (any, error) {
	req, err := httpx.BindJSON[struct {
		SessionTimeout         *int      `json:"session_timeout"`
		IdleTimeout            *int      `json:"idle_timeout"`
		MaxLoginAttempts       *int      `json:"max_login_attempts"`
		LockoutDuration        *int      `json:"lockout_duration"`
		RateLimit              *int      `json:"rate_limit"`
		RateInterval           *int      `json:"rate_interval"`
		LoginRateLimit         *int      `json:"login_rate_limit"`
		LoginRateInterval      *int      `json:"login_rate_interval"`
		AllowMultiSession      *bool     `json:"allow_multi_session"`
		IPWhitelist            *[]string `json:"ip_whitelist"`
		SessionCleanupInterval *int      `json:"session_cleanup_interval"`
	}](c)
	if err != nil {
		return nil, err
	}

	middleware.AuditSummary(c, "更新认证配置")

	// ---- 校验（基于请求体） ----
	if req.SessionTimeout != nil && *req.SessionTimeout < 300 {
		return nil, errx.BadRequest("session_timeout 至少为 300 秒（5分钟）")
	}
	if req.IdleTimeout != nil && *req.IdleTimeout < 60 {
		return nil, errx.BadRequest("idle_timeout 至少为 60 秒（1分钟）")
	}
	if req.MaxLoginAttempts != nil && (*req.MaxLoginAttempts < 3 || *req.MaxLoginAttempts > 100) {
		return nil, errx.BadRequest("max_login_attempts 必须在 3 到 100 之间")
	}
	if req.LockoutDuration != nil && (*req.LockoutDuration < 60 || *req.LockoutDuration > 86400) {
		return nil, errx.BadRequest("lockout_duration 必须在 60 秒到 86400 秒之间")
	}
	if req.RateLimit != nil && *req.RateLimit < 10 {
		return nil, errx.BadRequest("rate_limit 至少为 10")
	}
	if req.RateInterval != nil && *req.RateInterval < 1 {
		return nil, errx.BadRequest("rate_interval 至少为 1 秒")
	}
	if req.LoginRateLimit != nil && (*req.LoginRateLimit < 1 || *req.LoginRateLimit > 100) {
		return nil, errx.BadRequest("login_rate_limit 必须在 1 到 100 之间")
	}
	if req.LoginRateInterval != nil && (*req.LoginRateInterval < 1 || *req.LoginRateInterval > 3600) {
		return nil, errx.BadRequest("login_rate_interval 必须在 1 秒到 3600 秒之间")
	}
	if req.SessionCleanupInterval != nil && (*req.SessionCleanupInterval < 60 || *req.SessionCleanupInterval > 86400) {
		return nil, errx.BadRequest("session_cleanup_interval 必须在 60 秒到 86400 秒之间")
	}
	if req.IPWhitelist != nil {
		for _, entry := range *req.IPWhitelist {
			entry = strings.TrimSpace(entry)
			if entry == "" || !validIPOrCIDR(entry) {
				return nil, errx.BadRequest("无效的 IP 白名单项: %s", entry)
			}
		}
	}

	// ---- 应用修改 ----
	h.store.Update(func(cfg *config.Config) {
		if req.SessionTimeout != nil {
			cfg.Auth.SessionTimeout = config.Duration(time.Duration(*req.SessionTimeout) * time.Second)
		}
		if req.IdleTimeout != nil {
			cfg.Auth.IdleTimeout = config.Duration(time.Duration(*req.IdleTimeout) * time.Second)
		}
		if req.MaxLoginAttempts != nil {
			cfg.Auth.MaxLoginAttempts = *req.MaxLoginAttempts
		}
		if req.LockoutDuration != nil {
			cfg.Auth.LockoutDuration = config.Duration(time.Duration(*req.LockoutDuration) * time.Second)
		}
		if req.RateLimit != nil {
			cfg.Auth.RateLimit = *req.RateLimit
		}
		if req.RateInterval != nil {
			cfg.Auth.RateInterval = config.Duration(time.Duration(*req.RateInterval) * time.Second)
		}
		if req.LoginRateLimit != nil {
			cfg.Auth.LoginRateLimit = *req.LoginRateLimit
		}
		if req.LoginRateInterval != nil {
			cfg.Auth.LoginRateInterval = config.Duration(time.Duration(*req.LoginRateInterval) * time.Second)
		}
		if req.AllowMultiSession != nil {
			cfg.Auth.AllowMultiSession = *req.AllowMultiSession
		}
		if req.IPWhitelist != nil {
			cleaned := make([]string, 0, len(*req.IPWhitelist))
			for _, e := range *req.IPWhitelist {
				if e = strings.TrimSpace(e); e != "" {
					cleaned = append(cleaned, e)
				}
			}
			cfg.Auth.IPWhitelist = cleaned
		}
		if req.SessionCleanupInterval != nil {
			cfg.Auth.SessionCleanupInterval = config.Duration(time.Duration(*req.SessionCleanupInterval) * time.Second)
		}
	})

	// 运行中热更新全局 IP 白名单（无需重启）
	if req.IPWhitelist != nil {
		if wl := middleware.GetIPWhitelist(); wl != nil {
			wl.Update(h.store.Get().Auth.IPWhitelist)
		}
	}

	// Sync API rate limiter at runtime
	if req.RateLimit != nil || req.RateInterval != nil {
		if rl := middleware.GetRateLimiter("api"); rl != nil {
			cur := h.store.Get()
			rl.UpdateRate(cur.Auth.RateLimit, cur.Auth.RateInterval.Duration())
		}
	}
	// Sync login rate limiter at runtime
	if req.LoginRateLimit != nil || req.LoginRateInterval != nil {
		if rl := middleware.GetRateLimiter("login"); rl != nil {
			cur := h.store.Get()
			rl.UpdateRate(cur.Auth.LoginRateLimit, cur.Auth.LoginRateInterval.Duration())
		}
	}

	// Save to config file
	if err := h.saveConfig(); err != nil {
		return nil, errx.Internal("保存配置失败: %w", err)
	}

	return gin.H{"message": "认证配置已更新"}, nil
}

// UpdateMonitorConfig updates monitor configuration
func (h *SettingsHandler) UpdateMonitorConfig(c *gin.Context) (any, error) {
	req, err := httpx.BindJSON[struct {
		HistoryRetention *int `json:"history_retention"`
		CollectInterval  *int `json:"collect_interval"`
	}](c)
	if err != nil {
		return nil, err
	}

	middleware.AuditSummary(c, "更新监控配置")

	// ---- 校验（基于请求体） ----
	if req.HistoryRetention != nil && (*req.HistoryRetention < 24 || *req.HistoryRetention > 8760) {
		return nil, errx.BadRequest("history_retention 必须在 24 小时到 8760 小时（1年）之间")
	}
	if req.CollectInterval != nil && (*req.CollectInterval < 1 || *req.CollectInterval > 300) {
		return nil, errx.BadRequest("collect_interval 必须在 1 秒到 300 秒（5分钟）之间")
	}

	// ---- 应用修改 ----
	var retention, interval time.Duration
	h.store.Update(func(cfg *config.Config) {
		if req.HistoryRetention != nil {
			retention = time.Duration(*req.HistoryRetention) * time.Hour
			cfg.Monitor.HistoryRetention = config.Duration(retention)
		}
		if req.CollectInterval != nil {
			interval = time.Duration(*req.CollectInterval) * time.Second
			cfg.Monitor.CollectInterval = config.Duration(interval)
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
		return nil, errx.Internal("保存配置失败: %w", err)
	}

	return gin.H{"message": "监控配置已更新"}, nil
}

// UpdateAuditConfig updates audit configuration
func (h *SettingsHandler) UpdateAuditConfig(c *gin.Context) (any, error) {
	req, err := httpx.BindJSON[struct {
		RetentionDays *int `json:"retention_days"`
	}](c)
	if err != nil {
		return nil, err
	}
	if req.RetentionDays != nil && (*req.RetentionDays < 1 || *req.RetentionDays > 3650) {
		return nil, errx.BadRequest("保留天数必须在 1 到 3650 天之间")
	}

	middleware.AuditSummary(c, "更新审计配置")

	h.store.Update(func(cfg *config.Config) {
		if req.RetentionDays != nil {
			cfg.Audit.RetentionDays = *req.RetentionDays
		}
	})

	// Save to config file
	if err := h.saveConfig(); err != nil {
		return nil, errx.Internal("保存配置失败: %w", err)
	}

	return gin.H{"message": "审计配置已更新"}, nil
}

// validIPOrCIDR 校验单个 IP 白名单项是合法 IP 或 CIDR。
func validIPOrCIDR(s string) bool {
	if strings.Contains(s, "/") {
		_, _, err := net.ParseCIDR(s)
		return err == nil
	}
	return net.ParseIP(s) != nil
}

// validateWebhookURL validates a webhook URL format
func validateWebhookURL(rawURL string) error {
	if rawURL == "" {
		return errx.BadRequest("webhook URL cannot be empty")
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return errx.BadRequest("invalid webhook URL: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return errx.BadRequest("webhook URL must use http or https scheme")
	}
	if u.Host == "" {
		return errx.BadRequest("webhook URL must have a valid host")
	}
	return nil
}

// UpdateNotifyConfig updates notification configuration
func (h *SettingsHandler) UpdateNotifyConfig(c *gin.Context) (any, error) {
	req, err := httpx.BindJSON[struct {
		Enabled    *bool   `json:"enabled"`
		WebhookURL *string `json:"webhook_url"`
	}](c)
	if err != nil {
		return nil, err
	}

	middleware.AuditSummary(c, "更新通知配置")

	if req.WebhookURL != nil && strings.TrimSpace(*req.WebhookURL) != "" {
		if err := validateWebhookURL(strings.TrimSpace(*req.WebhookURL)); err != nil {
			return nil, err
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
		return nil, errx.Internal("保存配置失败: %w", err)
	}

	return gin.H{"message": "通知配置已更新"}, nil
}

// TestWebhook sends a test notification to the configured webhook
func (h *SettingsHandler) TestWebhook(c *gin.Context) (any, error) {
	middleware.AuditSummary(c, "测试通知 Webhook")
	cfg := h.store.Get()
	if cfg.Notify.WebhookURL == "" {
		return nil, errx.BadRequest("请先配置 Webhook URL")
	}

	if err := validateWebhookURL(cfg.Notify.WebhookURL); err != nil {
		return nil, err
	}

	notifyService := notify.NewService(h.store)
	if err := notifyService.TestWebhook(); err != nil {
		return nil, errx.Internal("测试通知失败: %w", err)
	}

	return gin.H{"message": "测试通知已发送"}, nil
}

// UpdateLogsConfig updates the global log configuration (level/format/rotation).
// 等级通过 logger.SetLevel 运行时立即生效，其余字段持久化到 config.toml，重启后生效。
func (h *SettingsHandler) UpdateLogsConfig(c *gin.Context) (any, error) {
	req, err := httpx.BindJSON[struct {
		Level     *string `json:"level"`
		Format    *string `json:"format"`
		MaxSizeMB *int    `json:"max_size_mb"`
		MaxFiles  *int    `json:"max_files"`
	}](c)
	if err != nil {
		return nil, err
	}

	middleware.AuditSummary(c, "更新日志配置")

	if req.Level != nil {
		if err := logger.SetLevel(*req.Level); err != nil {
			return nil, errx.BadRequest("%v", err)
		}
	}
	if req.Format != nil {
		if *req.Format != "text" && *req.Format != "json" {
			return nil, errx.BadRequest("无效的日志格式 %q，可选 text|json", *req.Format)
		}
	}
	if req.MaxSizeMB != nil && (*req.MaxSizeMB < 1 || *req.MaxSizeMB > 1024) {
		return nil, errx.BadRequest("max_size_mb 必须在 1 到 1024 之间")
	}
	if req.MaxFiles != nil && (*req.MaxFiles < 1 || *req.MaxFiles > 10) {
		return nil, errx.BadRequest("max_files 必须在 1 到 10 之间")
	}

	h.store.Update(func(cfg *config.Config) {
		if req.Level != nil {
			cfg.Logs.Level = *req.Level
		}
		if req.Format != nil {
			cfg.Logs.Format = *req.Format
		}
		if req.MaxSizeMB != nil {
			cfg.Logs.MaxSizeMB = *req.MaxSizeMB
		}
		if req.MaxFiles != nil {
			cfg.Logs.MaxFiles = *req.MaxFiles
		}
	})

	if err := h.saveConfig(); err != nil {
		return nil, errx.Internal("保存配置失败: %w", err)
	}

	return gin.H{"message": "日志配置已更新", "level": logger.GetLevel()}, nil
}

// saveConfig saves the current config back to its source file (cfg.Path).
func (h *SettingsHandler) saveConfig() error {
	return config.Save(h.store.Get())
}

// GetAlertRules returns the current alert rules
func (h *SettingsHandler) GetAlertRules(c *gin.Context) (any, error) {
	return gin.H{"rules": h.store.Get().Alerts.Rules}, nil
}

// UpdateAlertRules updates the alert rules configuration
func (h *SettingsHandler) UpdateAlertRules(c *gin.Context) (any, error) {
	req, err := httpx.BindJSON[struct {
		Rules []config.AlertRuleConfig `json:"rules"`
	}](c)
	if err != nil {
		return nil, err
	}

	middleware.AuditSummary(c, "更新告警规则")

	// Limit number of rules to prevent abuse
	if len(req.Rules) > 50 {
		return nil, errx.BadRequest("告警规则过多（最多 50 条）")
	}

	// Validate rules
	for _, rule := range req.Rules {
		if rule.Name == "" {
			return nil, errx.BadRequest("规则名称不能为空")
		}
		validMetrics := map[string]bool{
			"cpu_percent": true, "mem_percent": true, "disk_percent": true,
			"load_1m": true, "load_5m": true, "load_15m": true,
		}
		if !validMetrics[rule.Metric] {
			return nil, errx.BadRequest("无效的指标: %s", rule.Metric)
		}
		if rule.Threshold <= 0 || rule.Threshold > 100 {
			return nil, errx.BadRequest("阈值必须在 0 到 100 之间")
		}
		if rule.Duration < 0 {
			return nil, errx.BadRequest("持续时间不能为负数")
		}
	}

	h.store.Update(func(cfg *config.Config) {
		cfg.Alerts.Rules = req.Rules
	})
	if err := h.saveConfig(); err != nil {
		return nil, errx.Internal("保存配置失败: %w", err)
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

	return gin.H{"message": "告警规则已更新", "rules": h.store.Get().Alerts.Rules}, nil
}

// GetSystemInfo returns system information
func (h *SettingsHandler) GetSystemInfo(c *gin.Context) (any, error) {
	return gin.H{
		"version":  infra.DisplayVersion(),
		"build_id": infra.BuildID,
	}, nil
}

// CheckUpdate checks GitHub for the latest release.
func (h *SettingsHandler) CheckUpdate(c *gin.Context) (any, error) {
	info, err := infra.CheckUpdate(c.Request.Context())
	if err != nil {
		return nil, errx.Internal("%w", err)
	}
	return info, nil
}

// RestartPanel restarts the backend service.
// When force=true (e.g. port change), the listener is closed so the child
// process creates a fresh one on the new address.
func (h *SettingsHandler) RestartPanel(c *gin.Context) (any, error) {
	middleware.AuditSummary(c, "重启面板")

	req, _ := httpx.BindJSON[struct {
		Force *bool `json:"force"`
	}](c) // optional body, ignore parse errors
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
	return gin.H{"message": "面板正在重启..."}, nil
}

func RegisterRoutes(protected *gin.RouterGroup, store *config.Store, alertService *alert.Service, monitorSvc MonitorUpdater, sig *infra.Signal) {
	handler := NewSettingsHandler(store, alertService, sig)
	handler.SetMonitorService(monitorSvc)
	protected.GET("/settings", httpx.H(handler.GetSettings))
	protected.GET("/settings/system", httpx.H(handler.GetSystemInfo))
	protected.GET("/settings/check-update", httpx.H(handler.CheckUpdate))
	protected.PUT("/settings/server", httpx.H(handler.UpdateServerConfig))
	protected.PUT("/settings/tls", httpx.H(handler.UpdateTLSConfig))
	protected.PUT("/settings/auth", httpx.H(handler.UpdateAuthConfig))
	protected.PUT("/settings/monitor", httpx.H(handler.UpdateMonitorConfig))
	protected.PUT("/settings/audit", httpx.H(handler.UpdateAuditConfig))
	protected.PUT("/settings/notify", httpx.H(handler.UpdateNotifyConfig))
	protected.PUT("/settings/features", httpx.H(handler.UpdateFeaturesConfig))
	protected.PUT("/settings/logs", httpx.H(handler.UpdateLogsConfig))
	protected.POST("/settings/notify/test", httpx.H(handler.TestWebhook))
	protected.GET("/alerts/rules", httpx.H(handler.GetAlertRules))
	protected.PUT("/alerts/rules", httpx.H(handler.UpdateAlertRules))
	protected.POST("/settings/restart", httpx.H(handler.RestartPanel))
}

package config

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"time"
)

// Validate 检查配置的安全性与合理性。dev 模式下多数错误降级为警告
// （log.Println 后继续），非 dev 模式返回 error 由调用方决定是否中止。
// 空密钥、缺失必填路径等严重问题无论何种模式都返回 error。
func Validate(cfg *Config, devMode bool) error {
	// JWT secret - empty always rejected
	if cfg.Auth.JWTSecret == "" {
		return errors.New("JWT secret is empty. Set a strong secret via EASYSERVER_JWT_SECRET env var (32+ chars) or auth.jwt_secret in config.toml")
	}
	if err := rejectOrWarn(devMode, len(cfg.Auth.JWTSecret) < 32,
		"JWT secret must be at least 32 bytes. Use: EASYSERVER_JWT_SECRET=$(openssl rand -base64 32)"); err != nil {
		return err
	}

	knownWeakJWT := []string{
		"easyserver-secret-key-change-me",
		"change-me-to-a-random-secret",
		"change-me-to-a-random-secret-at-least-32-bytes-long",
		"secret",
		"password",
		"12345678901234567890123456789012",
	}
	for _, weak := range knownWeakJWT {
		if cfg.Auth.JWTSecret == weak {
			if err := rejectOrWarn(devMode, true,
				"JWT secret is set to a well-known default value."); err != nil {
				return err
			}
			break
		}
	}
	if len(cfg.Auth.JWTSecret) >= 32 && uniformString(cfg.Auth.JWTSecret) {
		if err := rejectOrWarn(devMode, true,
			"JWT secret has no entropy (all same character)."); err != nil {
			return err
		}
	}

	if cfg.Plugins.SecretsKey == "" {
		return errors.New("plugins.secrets_key is empty. It is auto-generated on first start - check file permissions")
	}

	// trusted_proxies：非法 CIDR 直接拒绝；显式信任所有代理（0.0.0.0/0、::/0）
	// 等于把 ClientIP 交给客户端自报，限流/IP 白名单全部可绕，非 dev 拒绝启动
	for _, p := range cfg.Server.TrustedProxies {
		if p == "0.0.0.0/0" || p == "::/0" {
			if err := rejectOrWarn(devMode, true,
				"server.trusted_proxies trusts ALL proxies - IP-based rate limiting and IP whitelist can be bypassed by a spoofed X-Forwarded-For"); err != nil {
				return err
			}
			continue
		}
		if _, _, err := net.ParseCIDR(p); err != nil && net.ParseIP(p) == nil {
			return fmt.Errorf("server.trusted_proxies entry %q is not a valid IP or CIDR", p)
		}
	}

	// Turnstile 一致性：开启任一开关就必须配置秘钥，否则验证组件照常渲染
	// 而后端全放行（fail-closed 兜底下则表现为无提示的全量拒绝，无法登录）
	turnstileOn := cfg.Server.Turnstile.EnableLogin ||
		cfg.Server.Turnstile.EnableQRLogin ||
		cfg.Server.Turnstile.EnablePublicShare
	if err := rejectOrWarn(devMode, turnstileOn && cfg.Server.Turnstile.SecretKey == "",
		"server.turnstile: switches are enabled but secret_key is empty - verification will fail"); err != nil {
		return err
	}

	// Rate limit guards
	if err := rejectOrWarn(devMode, cfg.Auth.RateLimit < 10 || cfg.Auth.RateLimit > 100000,
		"auth.rate_limit must be between 10 and 100000"); err != nil {
		return err
	}
	if err := rejectOrWarn(devMode, cfg.Auth.RateInterval.Duration() < time.Second || cfg.Auth.RateInterval.Duration() > time.Hour,
		"auth.rate_interval must be between 1s and 1h"); err != nil {
		return err
	}

	if err := rejectOrWarn(devMode, cfg.Auth.LoginRateLimit < 1 || cfg.Auth.LoginRateLimit > 100,
		"auth.login_rate_limit must be between 1 and 100"); err != nil {
		return err
	}
	if err := rejectOrWarn(devMode, cfg.Auth.LoginRateInterval.Duration() < time.Second || cfg.Auth.LoginRateInterval.Duration() > time.Hour,
		"auth.login_rate_interval must be between 1s and 1h"); err != nil {
		return err
	}

	if err := rejectOrWarn(devMode, cfg.Server.AssetsRateLimit < 100 || cfg.Server.AssetsRateLimit > 100000,
		"server.assets_rate_limit must be between 100 and 100000"); err != nil {
		return err
	}
	if err := rejectOrWarn(devMode, cfg.Server.AssetsRateInterval.Duration() < time.Second || cfg.Server.AssetsRateInterval.Duration() > time.Hour,
		"server.assets_rate_interval must be between 1s and 1h"); err != nil {
		return err
	}

	// File manager
	if cfg.FileManager.BasePath == "" {
		return errors.New("filemanager.base_path is required")
	}
	if cfg.FileManager.BasePath == "/" {
		log.Println("WARNING: FileManager BasePath is '/' (full root access).")
	}

	// TLS: when enabled, verify the cert/key pair is loadable and not expired.
	// dev mode degrades to a warning; production rejects startup/restart.
	if cfg.Server.TLS.Enabled {
		if cfg.Server.TLS.CertFile == "" || cfg.Server.TLS.KeyFile == "" {
			if err := rejectOrWarn(devMode, true,
				"TLS 已启用但未配置证书/密钥文件"); err != nil {
				return err
			}
		} else if err := ValidateTLSCert(cfg.Server.TLS.CertFile, cfg.Server.TLS.KeyFile); err != nil {
			if err := rejectOrWarn(devMode, true, err.Error()); err != nil {
				return err
			}
		}
	}

	// 日志配置：非法等级/格式直接拒绝（避免静默回退使实际等级与配置不符）。
	if err := validateLogConfig(cfg); err != nil {
		return err
	}

	return nil
}

// validateLogConfig 校验日志等级与格式；非法值返回描述性错误。
func validateLogConfig(cfg *Config) error {
	if !validLogLevel(cfg.Logs.Level) {
		return fmt.Errorf("logs.level 无效: %q（可选 debug|info|warn|error）", cfg.Logs.Level)
	}
	if !validLogFormat(cfg.Logs.Format) {
		return fmt.Errorf("logs.format 无效: %q（可选 text|json）", cfg.Logs.Format)
	}
	return nil
}

// validLogLevel 校验日志等级枚举（与 internal/infra/logger.parseLevel 保持一致，
// 避免非法等级被静默回退导致实际等级与配置不符）。
func validLogLevel(s string) bool {
	switch s {
	case "debug", "info", "warn", "warning", "error", "":
		return true
	}
	return false
}

// validLogFormat 校验日志格式枚举。
func validLogFormat(s string) bool {
	switch s {
	case "text", "json", "":
		return true
	}
	return false
}

// ValidateTLSCert loads and validates a TLS certificate/key pair:
//   - both files are readable
//   - PEM parses correctly
//   - cert and key match (tls.LoadX509KeyPair)
//   - cert has not expired
//
// Returns nil if valid, a descriptive error otherwise. Shared by startup
// (config.Validate) and hot-restart (settings handler) so both paths enforce
// the same level of checking.
func ValidateTLSCert(certFile, keyFile string) error {
	if _, err := tls.LoadX509KeyPair(certFile, keyFile); err != nil {
		return fmt.Errorf("TLS 证书/密钥无效: %w", err)
	}
	// LoadX509KeyPair does not check expiry; parse the leaf cert explicitly.
	data, err := os.ReadFile(certFile)
	if err != nil {
		return fmt.Errorf("读取 TLS 证书失败: %w", err)
	}
	block, _ := pem.Decode(data)
	if block == nil {
		return errors.New("TLS 证书不是有效的 PEM 格式")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return fmt.Errorf("解析 TLS 证书失败: %w", err)
	}
	if time.Now().After(cert.NotAfter) {
		return fmt.Errorf("TLS 证书已于 %s 过期", cert.NotAfter.Format("2006-01-02"))
	}
	return nil
}

// rejectOrWarn 在 fail 时：dev 模式打印警告返回 nil，非 dev 返回 error。
func rejectOrWarn(devMode, fail bool, msg string) error {
	if !fail {
		return nil
	}
	if devMode {
		log.Println("WARNING:", msg, "(allowed only in dev mode)")
		return nil
	}
	return errors.New(msg)
}

// uniformString 判断字符串是否所有字符相同（用于检测无熵密钥）。
func uniformString(s string) bool {
	for i := 1; i < len(s); i++ {
		if s[i] != s[0] {
			return false
		}
	}
	return true
}

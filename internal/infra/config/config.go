package config

import (
	"os"
	"strconv"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server       ServerConfig       `yaml:"server"`
	Auth         AuthConfig         `yaml:"auth"`
	Monitor      MonitorConfig      `yaml:"monitor"`
	Alerts       AlertConfig        `yaml:"alerts"`
	Database     DatabaseConfig     `yaml:"database"`
	Audit        AuditConfig        `yaml:"audit"`
	FileManager  FileManagerConfig  `yaml:"filemanager"`
	TencentCloud TencentCloudConfig `yaml:"tencentcloud"`
	Deploy       DeployConfig       `yaml:"deploy"`
	Notify       NotifyConfig       `yaml:"notify"`
}

type NotifyConfig struct {
	WebhookURL string `yaml:"webhook_url"` // 钉钉/飞书/企微 Webhook URL
	Enabled    bool   `yaml:"enabled"`
}

type ServerConfig struct {
	Port           int      `yaml:"port"`
	Host           string   `yaml:"host"`
	ServeFrontend  bool     `yaml:"serve_frontend"`
	AllowedOrigins []string `yaml:"allowed_origins"`
	DevMode        bool     `yaml:"dev_mode"`
	Domain         string   `yaml:"domain"`
	// TrustedProxies is the list of trusted reverse-proxy CIDRs whose
	// X-Forwarded-For is honored by c.ClientIP(). Default ["127.0.0.1"] (same-
	// host nginx). Set to the CDN ranges (e.g. Cloudflare) when fronted by one.
	// Empty/nil disables XFF trust entirely (ClientIP uses RemoteAddr).
	TrustedProxies     []string      `yaml:"trusted_proxies"`
	RedirectMode       string        `yaml:"redirect_mode"` // "off" | "ip_only" | "non_matching"
	WwwHandling        string        `yaml:"www_handling"`  // "off" | "force_www" | "remove_www"
	TLS                TLSConfig     `yaml:"tls"`
	AssetsRateLimit    int           `yaml:"assets_rate_limit"`
	AssetsRateInterval time.Duration `yaml:"assets_rate_interval"`
	MaxUploadSize      int64         `yaml:"max_upload_size"` // bytes, 0 = use default (512MB)
}

type TLSConfig struct {
	Enabled  bool   `yaml:"enabled"`
	CertFile string `yaml:"cert_file"`
	KeyFile  string `yaml:"key_file"`
}

type AuthConfig struct {
	JWTSecret              string        `yaml:"jwt_secret"`
	SessionTimeout         time.Duration `yaml:"session_timeout"`
	IdleTimeout            time.Duration `yaml:"idle_timeout"`
	MaxLoginAttempts       int           `yaml:"max_login_attempts"`
	LockoutDuration        time.Duration `yaml:"lockout_duration"`
	RateLimit              int           `yaml:"rate_limit"`
	RateInterval           time.Duration `yaml:"rate_interval"`
	LoginRateLimit         int           `yaml:"login_rate_limit"`
	LoginRateInterval      time.Duration `yaml:"login_rate_interval"`
	IPWhitelist            []string      `yaml:"ip_whitelist"`
	SessionCleanupInterval time.Duration `yaml:"session_cleanup_interval"`
}

type MonitorConfig struct {
	HistoryRetention time.Duration `yaml:"history_retention"`
	CollectInterval  time.Duration `yaml:"collect_interval"`
}

type AlertConfig struct {
	Rules []AlertRuleConfig `yaml:"rules"`
}

type AlertRuleConfig struct {
	Name      string  `yaml:"name" json:"name"`
	Metric    string  `yaml:"metric" json:"metric"`
	Threshold float64 `yaml:"threshold" json:"threshold"`
	Duration  int     `yaml:"duration" json:"duration"`
	Enabled   bool    `yaml:"enabled" json:"enabled"`
}

type DatabaseConfig struct {
	Path string `yaml:"path"`
}

type AuditConfig struct {
	Enabled       bool   `yaml:"enabled"`
	LogPath       string `yaml:"log_path"`
	RetentionDays int    `yaml:"retention_days"`
}

type FileManagerConfig struct {
	BasePath string `yaml:"base_path"`
}

type TencentCloudConfig struct {
	Enabled    bool   `yaml:"enabled"`
	SecretID   string `yaml:"secret_id"`
	SecretKey  string `yaml:"secret_key"`
	Region     string `yaml:"region"`
	InstanceID string `yaml:"instance_id"`
}

type DeployConfig struct {
	EncryptionKey string `yaml:"encryption_key"`
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	cfg := &Config{
		Server: ServerConfig{
			Port:               8080,
			Host:               "0.0.0.0",
			AssetsRateLimit:    5000,
			AssetsRateInterval: time.Minute,
		},
		Auth: AuthConfig{
			SessionTimeout:         24 * time.Hour,
			IdleTimeout:            30 * time.Minute,
			MaxLoginAttempts:       5,
			LockoutDuration:        15 * time.Minute,
			RateLimit:              1000,
			RateInterval:           time.Minute,
			LoginRateLimit:         10,
			LoginRateInterval:      time.Minute,
			SessionCleanupInterval: 5 * time.Minute,
		},
		Monitor: MonitorConfig{
			HistoryRetention: 24 * time.Hour,
			CollectInterval:  time.Second,
		},
		Database: DatabaseConfig{
			Path: "./data/easyserver.db",
		},
		Audit: AuditConfig{
			Enabled:       true,
			LogPath:       "./data/audit.log",
			RetentionDays: 90,
		},
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, err
	}

	// Merge defaults for fields not present in YAML (yaml.Unmarshal zeros them)
	cfg.mergeDefaults()

	// Override with environment variables
	cfg.applyEnvOverrides()

	return cfg, nil
}

// mergeDefaults restores default values for int/duration fields that were
// zeroed by yaml.Unmarshal when the YAML key is absent.
func (c *Config) mergeDefaults() {
	if c.Server.AssetsRateLimit == 0 {
		c.Server.AssetsRateLimit = 5000
	}
	if c.Server.AssetsRateInterval == 0 {
		c.Server.AssetsRateInterval = time.Minute
	}
	if c.Auth.RateLimit == 0 {
		c.Auth.RateLimit = 1000
	}
	if c.Auth.RateInterval == 0 {
		c.Auth.RateInterval = time.Minute
	}
	if c.Auth.LoginRateLimit == 0 {
		c.Auth.LoginRateLimit = 10
	}
	if c.Auth.LoginRateInterval == 0 {
		c.Auth.LoginRateInterval = time.Minute
	}
	if c.Auth.SessionCleanupInterval == 0 {
		c.Auth.SessionCleanupInterval = 5 * time.Minute
	}
}

func (c *Config) applyEnvOverrides() {
	// JWT Secret
	if v := os.Getenv("EASYSERVER_JWT_SECRET"); v != "" {
		c.Auth.JWTSecret = v
	}

	// Server
	if v := os.Getenv("EASYSERVER_PORT"); v != "" {
		if port, err := strconv.Atoi(v); err == nil {
			c.Server.Port = port
		}
	}
	if v := os.Getenv("EASYSERVER_HOST"); v != "" {
		c.Server.Host = v
	}

	// TLS
	if v := os.Getenv("EASYSERVER_TLS_ENABLED"); v == "true" {
		c.Server.TLS.Enabled = true
	}
	if v := os.Getenv("EASYSERVER_TLS_CERT_FILE"); v != "" {
		c.Server.TLS.CertFile = v
	}
	if v := os.Getenv("EASYSERVER_TLS_KEY_FILE"); v != "" {
		c.Server.TLS.KeyFile = v
	}

	// Database
	if v := os.Getenv("EASYSERVER_DB_PATH"); v != "" {
		c.Database.Path = v
	}

	// Tencent Cloud
	if v := os.Getenv("EASYSERVER_TENCENTCLOUD_ENABLED"); v == "true" {
		c.TencentCloud.Enabled = true
	}
	if v := os.Getenv("EASYSERVER_TENCENTCLOUD_SECRET_ID"); v != "" {
		c.TencentCloud.SecretID = v
	}
	if v := os.Getenv("EASYSERVER_TENCENTCLOUD_SECRET_KEY"); v != "" {
		c.TencentCloud.SecretKey = v
	}
	if v := os.Getenv("EASYSERVER_TENCENTCLOUD_REGION"); v != "" {
		c.TencentCloud.Region = v
	}
	if v := os.Getenv("EASYSERVER_TENCENTCLOUD_INSTANCE_ID"); v != "" {
		c.TencentCloud.InstanceID = v
	}

	// Deploy
	if v := os.Getenv("EASYSERVER_ENCRYPTION_KEY"); v != "" {
		c.Deploy.EncryptionKey = v
	}
}

// Save writes the configuration to a YAML file
func Save(cfg *Config, path string) error {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}

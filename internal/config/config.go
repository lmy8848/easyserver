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
	Port           int       `yaml:"port"`
	Host           string    `yaml:"host"`
	ServeFrontend  bool      `yaml:"serve_frontend"`
	AllowedOrigins []string  `yaml:"allowed_origins"`
	DevMode        bool      `yaml:"dev_mode"`
	TLS            TLSConfig `yaml:"tls"`
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
	Name      string  `yaml:"name"`
	Metric    string  `yaml:"metric"`
	Threshold float64 `yaml:"threshold"`
	Duration  int     `yaml:"duration"`
	Enabled   bool    `yaml:"enabled"`
}

type DatabaseConfig struct {
	Path string `yaml:"path"`
}

type AuditConfig struct {
	Enabled      bool   `yaml:"enabled"`
	LogPath      string `yaml:"log_path"`
	RetentionDays int   `yaml:"retention_days"`
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
			Port: 8080,
			Host: "0.0.0.0",
		},
		Auth: AuthConfig{
			SessionTimeout:         24 * time.Hour,
			IdleTimeout:            30 * time.Minute,
			MaxLoginAttempts:       5,
			LockoutDuration:        15 * time.Minute,
			RateLimit:              100,
			RateInterval:           time.Minute,
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

	// Override with environment variables
	cfg.applyEnvOverrides()

	return cfg, nil
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

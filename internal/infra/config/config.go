package config

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"easyserver/internal/util"

	"github.com/pelletier/go-toml/v2"
)

// DataRoot 是面板私有工作根目录（Panel Root，固定常量，不支持配置修改）。
// 各领域子目录（mise / scripts / db）由代码从它派生拼接，禁止在代码中
// 散落硬编码 /opt/easyserver 字面量。
//
// 设计意图：整个面板的私有数据自包含在一个目录下，卸载面板 = 删除该目录，
// 零系统残留。改动该常量 = 全新安装语义（存量内容不迁移）。
const DataRoot = "/opt/easyserver"

// Path 记录配置文件在磁盘上的实际路径（--config 决定），Load 时填充。
// 不参与 TOML/JSON 序列化：它是来源元数据，不是配置项。需要路径的地方
// （写回 config.Save、推导配置目录、热重启传参、FIM 默认监视项）统一从
// 它取，避免把 configPath 作为独立参数在调用链里层层传递。
type Config struct {
	Path        string            `toml:"-" json:"-"`
	Server      ServerConfig      `toml:"server"`
	Auth        AuthConfig        `toml:"auth"`
	Monitor     MonitorConfig     `toml:"monitor"`
	Alerts      AlertConfig       `toml:"alerts"`
	Audit       AuditConfig       `toml:"audit"`
	FileManager FileManagerConfig `toml:"filemanager"`
	Notify      NotifyConfig      `toml:"notify"`
	Logs        LogsConfig        `toml:"logs"`
	Features    FeaturesConfig    `toml:"features"`
}

// FeaturesConfig holds optional feature toggles. Disabled by default to save
// resources; admin enables per-feature from the panel settings.
type FeaturesConfig struct {
	FIM bool `toml:"fim"` // file integrity monitoring
}

type NotifyConfig struct {
	WebhookURL string `toml:"webhook_url"` // 钉钉/飞书/企微 Webhook URL
	Enabled    bool   `toml:"enabled"`
}

type ServerConfig struct {
	Port           int      `toml:"port"`
	Host           string   `toml:"host"`
	AllowedOrigins []string `toml:"allowed_origins"`
	DevMode        bool     `toml:"dev_mode"`
	Domain         string   `toml:"domain"`
	ForceDomain    bool     `toml:"force_domain"`
	// TrustedProxies is the list of trusted reverse-proxy CIDRs whose
	// X-Forwarded-For is honored by c.ClientIP(). Default ["127.0.0.1"] (same-
	// host nginx). Set to the CDN ranges (e.g. Cloudflare) when fronted by one.
	// Empty/nil disables XFF trust entirely (ClientIP uses RemoteAddr).
	TrustedProxies     []string        `toml:"trusted_proxies"`
	TLS                TLSConfig       `toml:"tls"`
	Turnstile          TurnstileConfig `toml:"turnstile"`
	AssetsRateLimit    int             `toml:"assets_rate_limit"`
	AssetsRateInterval Duration        `toml:"assets_rate_interval"`
	MaxUploadSize      int64           `toml:"max_upload_size"` // bytes, 0 = use default (512MB)
}

// TurnstileConfig holds Cloudflare Turnstile settings.
type TurnstileConfig struct {
	SiteKey           string `toml:"site_key"`
	SecretKey         string `toml:"secret_key"`
	EnableLogin       bool   `toml:"enable_login"`
	EnableQRLogin     bool   `toml:"enable_qr_login"`
	EnablePublicShare bool   `toml:"enable_public_share"`
}

type TLSConfig struct {
	Enabled  bool   `toml:"enabled"`
	CertFile string `toml:"cert_file"`
	KeyFile  string `toml:"key_file"`
}

type AuthConfig struct {
	JWTSecret              string   `toml:"jwt_secret"`
	SessionTimeout         Duration `toml:"session_timeout"`
	IdleTimeout            Duration `toml:"idle_timeout"`
	MaxLoginAttempts       int      `toml:"max_login_attempts"`
	LockoutDuration        Duration `toml:"lockout_duration"`
	RateLimit              int      `toml:"rate_limit"`
	RateInterval           Duration `toml:"rate_interval"`
	LoginRateLimit         int      `toml:"login_rate_limit"`
	LoginRateInterval      Duration `toml:"login_rate_interval"`
	IPWhitelist            []string `toml:"ip_whitelist"`
	SessionCleanupInterval Duration `toml:"session_cleanup_interval"`
	AllowMultiSession      bool     `toml:"allow_multi_session"`
}

type MonitorConfig struct {
	HistoryRetention Duration `toml:"history_retention"`
	CollectInterval  Duration `toml:"collect_interval"`
}

type AlertConfig struct {
	Rules []AlertRuleConfig `toml:"rules"`
}

type AlertRuleConfig struct {
	Name      string  `toml:"name" json:"name"`
	Metric    string  `toml:"metric" json:"metric"`
	Threshold float64 `toml:"threshold" json:"threshold"`
	Duration  int     `toml:"duration" json:"duration"`
	Enabled   bool    `toml:"enabled" json:"enabled"`
}

type AuditConfig struct {
	RetentionDays int `toml:"retention_days"`
}

type FileManagerConfig struct {
	BasePath string `toml:"base_path"`
}

// LogsConfig 控制全局运行日志：文件落盘（应用根目录）、分级、源码定位、轮转。
// 等级可在面板设置中运行时修改并持久化（SetLevel 即时生效，重启读本配置生效）。
type LogsConfig struct {
	Level     string `toml:"level"`       // debug|info|warn|error，默认 info
	Path      string `toml:"path"`        // 日志文件路径；空 = DataRoot/easyserver.log
	Format    string `toml:"format"`      // text|json，默认 text
	MaxSizeMB int    `toml:"max_size_mb"` // 单文件轮转阈值(MB)，默认 10
	MaxFiles  int    `toml:"max_files"`   // 保留轮转文件数(.1 .2…)，默认 3
}

// Duration is a wrapper around time.Duration that supports TOML string parsing (e.g. "24h", "30m", "3s").
type Duration time.Duration

func (d Duration) Duration() time.Duration {
	return time.Duration(d)
}

func (d Duration) String() string {
	return time.Duration(d).String()
}

func (d Duration) Seconds() float64 {
	return time.Duration(d).Seconds()
}

func (d Duration) Hours() float64 {
	return time.Duration(d).Hours()
}

func (d Duration) Minutes() float64 {
	return time.Duration(d).Minutes()
}

func (d *Duration) UnmarshalText(text []byte) error {
	s := string(text)
	if s == "" {
		*d = 0
		return nil
	}
	dur, err := time.ParseDuration(s)
	if err != nil {
		return err
	}
	*d = Duration(dur)
	return nil
}

func (d Duration) MarshalText() ([]byte, error) {
	return []byte(marshalDuration(time.Duration(d))), nil
}

// marshalDuration formats a duration dropping zero sub-units, e.g.:
//   - 24h0m0s  → "24h"
//   - 1h30m0s  → "1h30m"
//   - 15m0s    → "15m"
//   - 3s       → "3s"
func marshalDuration(d time.Duration) string {
	if d == 0 {
		return "0s"
	}
	neg := d < 0
	if neg {
		d = -d
	}

	h := d / time.Hour
	d %= time.Hour
	m := d / time.Minute
	d %= time.Minute
	s := d / time.Second
	sub := d % time.Second

	var result string
	if sub != 0 {
		// sub-second component: fall back to stdlib
		result = (h*time.Hour + m*time.Minute + s*time.Second + sub).String()
	} else {
		if h > 0 {
			result += fmt.Sprintf("%dh", h)
		}
		if m > 0 {
			result += fmt.Sprintf("%dm", m)
		}
		if s > 0 {
			result += fmt.Sprintf("%ds", s)
		}
	}

	if neg {
		return "-" + result
	}
	return result
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	cfg := &Config{Path: path}
	if err := toml.Unmarshal(data, cfg); err != nil {
		return nil, err
	}

	// Merge defaults for fields not present in TOML (toml.Unmarshal zeros them)
	generated := cfg.mergeDefaults()

	// Override with environment variables
	cfg.applyEnvOverrides()

	// If crypto secrets were auto-generated on first run, persist them back to file so they survive restarts
	if generated && path != "" {
		_ = Save(cfg)
	}

	return cfg, nil
}

// generateRandomSecret generates a cryptographically secure hex-encoded random string.
func generateRandomSecret(numBytes int) string {
	s, err := util.RandomHex(numBytes)
	if err != nil {
		panic("crypto/rand read failed: " + err.Error())
	}
	return s
}

// mergeDefaults restores default values for fields that were
// zeroed by toml.Unmarshal when the TOML key is absent. Returns true if any
// sensitive secret was auto-generated.
func (c *Config) mergeDefaults() bool {
	generated := false
	if c.Auth.JWTSecret == "" {
		c.Auth.JWTSecret = generateRandomSecret(32)
		generated = true
	}
	if c.Server.Port == 0 {
		c.Server.Port = 8080
	}
	if c.Server.Host == "" {
		c.Server.Host = "0.0.0.0"
	}
	if c.Server.AssetsRateLimit == 0 {
		c.Server.AssetsRateLimit = 5000
	}
	if c.Server.AssetsRateInterval == 0 {
		c.Server.AssetsRateInterval = Duration(time.Minute)
	}
	if c.Auth.SessionTimeout == 0 {
		c.Auth.SessionTimeout = Duration(24 * time.Hour)
	}
	if c.Auth.IdleTimeout == 0 {
		c.Auth.IdleTimeout = Duration(24 * time.Hour)
	}
	if c.Auth.MaxLoginAttempts == 0 {
		c.Auth.MaxLoginAttempts = 5
	}
	if c.Auth.LockoutDuration == 0 {
		c.Auth.LockoutDuration = Duration(15 * time.Minute)
	}
	if c.Auth.RateLimit == 0 {
		c.Auth.RateLimit = 1000
	}
	if c.Auth.RateInterval == 0 {
		c.Auth.RateInterval = Duration(time.Minute)
	}
	if c.Auth.LoginRateLimit == 0 {
		c.Auth.LoginRateLimit = 60
	}
	if c.Auth.LoginRateInterval == 0 {
		c.Auth.LoginRateInterval = Duration(time.Minute)
	}
	if c.Auth.SessionCleanupInterval == 0 {
		c.Auth.SessionCleanupInterval = Duration(5 * time.Minute)
	}
	if c.Monitor.CollectInterval == 0 {
		c.Monitor.CollectInterval = Duration(3 * time.Second)
	}
	if c.Monitor.HistoryRetention == 0 {
		c.Monitor.HistoryRetention = Duration(7 * 24 * time.Hour)
	}
	if c.Audit.RetentionDays == 0 {
		c.Audit.RetentionDays = 90
	}
	if c.Logs.Level == "" {
		c.Logs.Level = "info"
	}
	if c.Logs.Format == "" {
		c.Logs.Format = "text"
	}
	if c.Logs.MaxSizeMB == 0 {
		c.Logs.MaxSizeMB = 10
	}
	if c.Logs.MaxFiles == 0 {
		c.Logs.MaxFiles = 3
	}
	return generated
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
}

// Save writes the configuration back to its source file (cfg.Path, set by Load).
func Save(cfg *Config) error {
	data, err := toml.Marshal(cfg)
	if err != nil {
		return err
	}
	return os.WriteFile(cfg.Path, data, 0600)
}

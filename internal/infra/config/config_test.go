package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadValidTOML(t *testing.T) {
	tomlContent := `
[server]
port = 9090
host = "127.0.0.1"
dev_mode = true
allowed_origins = ["http://localhost:3000"]
domain = "example.com"
force_domain = true
trusted_proxies = ["10.0.0.0/8"]
assets_rate_limit = 2000
assets_rate_interval = "30s"
max_upload_size = 1048576

[server.tls]
enabled = true
cert_file = "/path/to/cert.pem"
key_file = "/path/to/key.pem"

[server.turnstile]
site_key = "turnstile-site-key"
secret_key = "turnstile-secret-key"
enable_login = true
enable_qr_login = true
enable_public_share = true

[auth]
jwt_secret = "test-secret-at-least-32-bytes-long-12345"
session_timeout = "12h"
idle_timeout = "1h"
max_login_attempts = 3
lockout_duration = "10m"
rate_limit = 500
rate_interval = "30s"
login_rate_limit = 30
login_rate_interval = "30s"
ip_whitelist = ["192.168.1.1"]
session_cleanup_interval = "2m"
allow_multi_session = true

[monitor]
history_retention = "48h"
collect_interval = "5s"

[audit]
retention_days = 30

[filemanager]
base_path = "/tmp/data"

[tencentcloud]
enabled = true
secret_id = "tc-id"
secret_key = "tc-key"
region = "ap-beijing"
instance_id = "lhins-123"

[notify]
enabled = true
webhook_url = "https://example.com/webhook"

[features]
fim = true

[[alerts.rules]]
name = "CPU Alert"
metric = "cpu_percent"
threshold = 85.5
duration = 120
enabled = true
`

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.toml")
	if err := os.WriteFile(configPath, []byte(tomlContent), 0600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if cfg.Server.Port != 9090 {
		t.Errorf("Port = %d, want 9090", cfg.Server.Port)
	}
	if cfg.Server.Host != "127.0.0.1" {
		t.Errorf("Host = %s, want 127.0.0.1", cfg.Server.Host)
	}
	if !cfg.Server.ForceDomain {
		t.Errorf("ForceDomain = false, want true")
	}
	if cfg.Auth.SessionTimeout != Duration(12*time.Hour) {
		t.Errorf("SessionTimeout = %v, want 12h", cfg.Auth.SessionTimeout)
	}
	if cfg.Auth.IdleTimeout != Duration(1*time.Hour) {
		t.Errorf("IdleTimeout = %v, want 1h", cfg.Auth.IdleTimeout)
	}
	if cfg.Monitor.CollectInterval != Duration(5*time.Second) {
		t.Errorf("CollectInterval = %v, want 5s", cfg.Monitor.CollectInterval)
	}
	if cfg.Monitor.HistoryRetention != Duration(48*time.Hour) {
		t.Errorf("HistoryRetention = %v, want 48h", cfg.Monitor.HistoryRetention)
	}
	if cfg.Audit.RetentionDays != 30 {
		t.Errorf("Audit.RetentionDays = %d, want 30", cfg.Audit.RetentionDays)
	}
	if len(cfg.Alerts.Rules) != 1 || cfg.Alerts.Rules[0].Name != "CPU Alert" {
		t.Errorf("Alerts.Rules = %+v, want 1 rule 'CPU Alert'", cfg.Alerts.Rules)
	}
	if !cfg.Features.FIM {
		t.Errorf("Features.FIM not enabled: %+v", cfg.Features)
	}
}

func TestAutoGenerateSecretsAndPersist(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.toml")

	// Config without jwt_secret
	initial := `
[server]
port = 8080
`
	if err := os.WriteFile(configPath, []byte(initial), 0600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if len(cfg.Auth.JWTSecret) < 32 {
		t.Errorf("Expected generated JWTSecret >= 32 chars, got %d", len(cfg.Auth.JWTSecret))
	}

	// Verify persistence
	reloaded, err := Load(configPath)
	if err != nil {
		t.Fatalf("Reload: %v", err)
	}

	if reloaded.Auth.JWTSecret != cfg.Auth.JWTSecret {
		t.Errorf("Reloaded JWTSecret = %s, want %s (persistent)", reloaded.Auth.JWTSecret, cfg.Auth.JWTSecret)
	}
}

func TestSaveAndReloadRoundtrip(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.toml")

	initial := `
[server]
port = 8888
host = "0.0.0.0"

[auth]
jwt_secret = "initial-secret-at-least-32-bytes-long"
session_timeout = "24h"

[audit]
retention_days = 90
`
	if err := os.WriteFile(configPath, []byte(initial), 0600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	// Modify config
	cfg.Server.Port = 8000
	cfg.Auth.SessionTimeout = Duration(48 * time.Hour)
	cfg.Features.FIM = true
	cfg.Audit.RetentionDays = 60

	if err := Save(cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Reload
	reloaded, err := Load(configPath)
	if err != nil {
		t.Fatalf("Reload: %v", err)
	}

	if reloaded.Server.Port != 8000 {
		t.Errorf("Reloaded Port = %d, want 8000", reloaded.Server.Port)
	}
	if reloaded.Auth.SessionTimeout != Duration(48*time.Hour) {
		t.Errorf("Reloaded SessionTimeout = %v, want 48h", reloaded.Auth.SessionTimeout)
	}
	if !reloaded.Features.FIM {
		t.Errorf("Reloaded Features.FIM = false, want true")
	}
	if reloaded.Audit.RetentionDays != 60 {
		t.Errorf("Reloaded Audit.RetentionDays = %d, want 60", reloaded.Audit.RetentionDays)
	}
}

func TestApplyEnvOverrides(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.toml")

	initial := `
[server]
port = 8080
host = "127.0.0.1"

[auth]
jwt_secret = "file-secret-at-least-32-bytes-long"
`
	if err := os.WriteFile(configPath, []byte(initial), 0600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	t.Setenv("EASYSERVER_PORT", "9999")
	t.Setenv("EASYSERVER_JWT_SECRET", "env-secret-at-least-32-bytes-long")
	t.Setenv("EASYSERVER_HOST", "0.0.0.0")

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if cfg.Server.Port != 9999 {
		t.Errorf("Port = %d, want 9999 (from env)", cfg.Server.Port)
	}
	if cfg.Server.Host != "0.0.0.0" {
		t.Errorf("Host = %s, want 0.0.0.0 (from env)", cfg.Server.Host)
	}
	if cfg.Auth.JWTSecret != "env-secret-at-least-32-bytes-long" {
		t.Errorf("JWTSecret = %s, want env value", cfg.Auth.JWTSecret)
	}
}

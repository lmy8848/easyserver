package plugin

import (
	"context"
)

// PluginRecord 是 plugins 表的行结构（注册状态与运行时元数据）。
type PluginRecord struct {
	ID                  int64    `json:"id"`
	Name                string   `json:"name"`
	Version             string   `json:"version"`
	APIVersion          int      `json:"api_version"`
	ManifestPath        string   `json:"manifest_path"`
	Enabled             bool     `json:"enabled"`
	Status              string   `json:"status"`
	PID                 int      `json:"pid"`
	PermissionsApproved []string `json:"permissions_approved"`
	RestartCount        int      `json:"restart_count"`
	LastError           string   `json:"last_error"`
	LastSeen            string   `json:"last_seen"`
	SocketPath          string   `json:"socket_path"`
	CreatedAt           string   `json:"created_at"`
	UpdatedAt           string   `json:"updated_at"`
}

// PluginRepository 持久化插件注册状态、配置 KV 与密钥。
type PluginRepository interface {
	UpsertPlugin(ctx context.Context, rec *PluginRecord) error
	GetPlugin(ctx context.Context, name string) (*PluginRecord, error)
	ListPlugins(ctx context.Context) ([]PluginRecord, error)
	SetPluginStatus(ctx context.Context, name, status string, pid int, lastErr string) error
	SetPluginEnabled(ctx context.Context, name string, enabled bool, approved []string) error
	IncRestartCount(ctx context.Context, name string) error
	DeletePlugin(ctx context.Context, name string) error

	GetConfig(ctx context.Context, plugin, key string) (string, error)
	SetConfig(ctx context.Context, plugin, key, value string) error
	ListConfig(ctx context.Context, plugin string) (map[string]string, error)
	DeleteConfig(ctx context.Context, plugin, key string) error

	SetSecret(ctx context.Context, plugin, key, plaintext string) error
	GetSecret(ctx context.Context, plugin, key string) (string, error)
	DeleteSecret(ctx context.Context, plugin, key string) error
}

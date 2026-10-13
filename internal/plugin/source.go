package plugin

import (
	"context"
	"time"
)

// PluginSource 是插件安装源（市场入口）的记录：一个暴露 index.json 的 HTTP 端点。
type PluginSource struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	URL       string    `json:"url"`
	Enabled   bool      `json:"enabled"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// SourceRepository 持久化插件源清单。
type SourceRepository interface {
	ListSources(ctx context.Context) ([]PluginSource, error)
	AddSource(ctx context.Context, name, url string) (*PluginSource, error)
	DeleteSource(ctx context.Context, id int64) error
	SetSourceEnabled(ctx context.Context, id int64, enabled bool) error
}

// SourceCatalog 是插件源 index.json 的解析结果（源协议，文档 §插件源）。
type SourceCatalog struct {
	Name    string             `json:"name"`
	Plugins []SourcePluginItem `json:"plugins"`
}

// SourcePluginItem 是源中单个插件的条目。
type SourcePluginItem struct {
	Name        string `json:"name"`
	Version     string `json:"version"`
	Description string `json:"description"`
	DownloadURL string `json:"download_url"`
}

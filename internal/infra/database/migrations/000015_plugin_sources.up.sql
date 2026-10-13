-- 插件源：插件安装源（市场）入口。源是暴露 index.json 的 HTTP 端点，
-- 面板据此浏览插件清单并从源下载安装。为后期插件源市场打基础。
CREATE TABLE IF NOT EXISTS plugin_sources (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT UNIQUE NOT NULL,
    url TEXT NOT NULL,
    enabled INTEGER DEFAULT 1,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_plugin_sources_enabled ON plugin_sources(enabled);

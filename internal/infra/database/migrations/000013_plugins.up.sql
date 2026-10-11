-- 插件系统基座：插件注册状态 / 插件配置 KV / 插件密钥
CREATE TABLE IF NOT EXISTS plugins (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT UNIQUE NOT NULL,
    version TEXT NOT NULL,
    api_version INTEGER NOT NULL,
    manifest_path TEXT NOT NULL,
    enabled INTEGER DEFAULT 0,
    status TEXT DEFAULT 'registered',
    pid INTEGER DEFAULT 0,
    permissions_approved TEXT,
    restart_count INTEGER DEFAULT 0,
    last_error TEXT,
    last_seen DATETIME,
    socket_path TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_plugins_status ON plugins(status);

CREATE TABLE plugin_config (
    plugin_name TEXT NOT NULL,
    key TEXT NOT NULL,
    value TEXT,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (plugin_name, key)
);

CREATE TABLE plugin_secrets (
    plugin_name TEXT NOT NULL,
    key TEXT NOT NULL,
    encrypted_value TEXT NOT NULL,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (plugin_name, key)
);

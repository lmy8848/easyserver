-- EasyServer Initial Schema
-- 后续结构变更请新增 00000N_*.up.sql / .down.sql，不要改本文件。

PRAGMA foreign_keys = ON;

-- =============================================
-- 1. Core user tables (no dependencies)
-- =============================================

CREATE TABLE IF NOT EXISTS users (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    username TEXT UNIQUE NOT NULL,
    password_hash TEXT NOT NULL,
    role TEXT NOT NULL DEFAULT 'viewer',
    must_change_pass INTEGER DEFAULT 0,
    last_login DATETIME,
    last_login_ip TEXT DEFAULT '',
    login_attempts INTEGER DEFAULT 0,
    locked_until DATETIME,
    expires_at DATETIME,
    ip_whitelist TEXT DEFAULT '',
    totp_secret TEXT DEFAULT '',
    totp_enabled INTEGER DEFAULT 0,
    totp_backup_codes TEXT DEFAULT '[]',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS token_blacklist (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL,
    token TEXT NOT NULL,
    expires_at DATETIME NOT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_token_blacklist_user ON token_blacklist(user_id);
CREATE INDEX IF NOT EXISTS idx_token_blacklist_expires ON token_blacklist(expires_at);

CREATE TABLE IF NOT EXISTS audit_logs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER,
    username TEXT,
    action TEXT NOT NULL,
    resource TEXT,
    detail TEXT,
    ip TEXT,
    user_agent TEXT,
    type TEXT NOT NULL DEFAULT 'operation',  -- operation | request
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_audit_created_at ON audit_logs(created_at);
CREATE INDEX IF NOT EXISTS idx_audit_logs_type ON audit_logs(type);
CREATE INDEX IF NOT EXISTS idx_audit_logs_action ON audit_logs(action);

-- =============================================
-- 2. System monitoring (no dependencies)
-- =============================================

CREATE TABLE IF NOT EXISTS monitor_data (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    cpu REAL,
    cpu_load_1m REAL DEFAULT 0,
    cpu_load_5m REAL DEFAULT 0,
    cpu_load_15m REAL DEFAULT 0,
    mem_total INTEGER,
    mem_used INTEGER,
    mem_available INTEGER,
    mem_usage REAL,
    disk_total INTEGER,
    disk_used INTEGER,
    disk_free INTEGER,
    disk_usage REAL,
    net_bytes_sent INTEGER,
    net_bytes_recv INTEGER,
    net_packets_sent INTEGER,
    net_packets_recv INTEGER,
    timestamp DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_monitor_timestamp ON monitor_data(timestamp);

-- =============================================
-- 3. Notifications (no dependencies)
-- =============================================

CREATE TABLE IF NOT EXISTS notifications (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    type TEXT NOT NULL,           -- alert/security/deploy/cron/update/system
    title TEXT NOT NULL,
    message TEXT NOT NULL,
    level TEXT DEFAULT 'info',    -- info/warning/error
    is_read INTEGER DEFAULT 0,
    metadata TEXT,                -- JSON: 关联资源ID等
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_notifications_type ON notifications(type);
CREATE INDEX IF NOT EXISTS idx_notifications_read ON notifications(is_read);
CREATE INDEX IF NOT EXISTS idx_notifications_created ON notifications(created_at DESC);

-- =============================================
-- 4. Runtime versions (no dependencies)
-- =============================================

CREATE TABLE IF NOT EXISTS runtime_version (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    lang TEXT NOT NULL CHECK(lang IN ('node', 'python', 'go', 'java', 'php')),
    major TEXT NOT NULL,
    exact TEXT NOT NULL,
    installed_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    status TEXT DEFAULT 'installed',
    progress INTEGER NOT NULL DEFAULT 0,
    progress_step TEXT NOT NULL DEFAULT '',
    logs TEXT NOT NULL DEFAULT '',
    error_message TEXT NOT NULL DEFAULT ''
);

-- =============================================
-- 5. Environment configs (no dependencies)
-- =============================================

CREATE TABLE IF NOT EXISTS env_configs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL UNIQUE,
    value TEXT NOT NULL,
    enabled INTEGER DEFAULT 1,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_env_configs_name ON env_configs(name);

CREATE TABLE IF NOT EXISTS path_entries (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    path TEXT NOT NULL UNIQUE,
    enabled INTEGER DEFAULT 1,
    order_num INTEGER DEFAULT 0,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_path_entries_path ON path_entries(path);

-- =============================================
-- 6. Deploy tables (server -> tasks -> versions)
-- =============================================

CREATE TABLE IF NOT EXISTS deploy_servers (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    host TEXT NOT NULL,
    port INTEGER DEFAULT 22,
    username TEXT NOT NULL,
    auth_type TEXT CHECK(auth_type IN ('password', 'key')),
    auth_data TEXT,
    status TEXT DEFAULT 'unknown',
    last_ping TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS deploy_tasks (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    server_id INTEGER REFERENCES deploy_servers(id),
    name TEXT NOT NULL,
    type TEXT CHECK(type IN ('sync', 'command', 'rollback')),
    source_path TEXT,
    dest_path TEXT,
    command TEXT,
    status TEXT DEFAULT 'pending',
    result TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_deploy_tasks_server ON deploy_tasks(server_id);

CREATE TABLE IF NOT EXISTS deploy_versions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    server_id INTEGER REFERENCES deploy_servers(id),
    task_id INTEGER REFERENCES deploy_tasks(id),
    version TEXT NOT NULL,
    files TEXT,
    backup_path TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_deploy_versions_server ON deploy_versions(server_id);

-- =============================================
-- 7. Web server & website tables
-- =============================================

CREATE TABLE IF NOT EXISTS web_servers (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL UNIQUE,
    display_name TEXT NOT NULL,
    description TEXT DEFAULT '',
    install_cmd TEXT DEFAULT '',
    uninstall_cmd TEXT DEFAULT '',
    config_path TEXT DEFAULT '',
    config_file TEXT DEFAULT '',
    sites_available TEXT DEFAULT '',
    sites_enabled TEXT DEFAULT '',
    service_name TEXT DEFAULT '',
    binary_path TEXT DEFAULT '',
    default_port INTEGER DEFAULT 80,
    log_dir TEXT DEFAULT '',
    status TEXT DEFAULT 'not_installed',
    version TEXT DEFAULT '',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_web_servers_name ON web_servers(name);

CREATE TABLE IF NOT EXISTS websites (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    web_server_id INTEGER NOT NULL DEFAULT 0,
    name TEXT NOT NULL,
    domain TEXT NOT NULL UNIQUE,
    root_path TEXT NOT NULL,
    port INTEGER DEFAULT 80,
    project_type TEXT DEFAULT 'static',
    app_port INTEGER DEFAULT 0,
    ssl_enabled INTEGER DEFAULT 0,
    ssl_cert_path TEXT DEFAULT '',
    ssl_key_path TEXT DEFAULT '',
    proxy_enabled INTEGER DEFAULT 0,
    proxy_pass TEXT DEFAULT '',
    custom_config TEXT DEFAULT '',
    access_log TEXT DEFAULT '',
    error_log TEXT DEFAULT '',
    status TEXT DEFAULT 'active',
    build_command TEXT DEFAULT '',
    start_command TEXT DEFAULT '',
    runtime_version_id INTEGER DEFAULT 0,
    process_id INTEGER DEFAULT 0,
    config_options TEXT DEFAULT '',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_websites_domain ON websites(domain);
CREATE INDEX IF NOT EXISTS idx_websites_server ON websites(web_server_id);

-- =============================================
-- 8. Database server tables (server -> versions -> databases)
-- =============================================

CREATE TABLE IF NOT EXISTS db_servers (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL UNIQUE,
    display_name TEXT NOT NULL,
    description TEXT DEFAULT '',
    default_port INTEGER DEFAULT 0,
    status TEXT DEFAULT 'not_installed',
    version TEXT DEFAULT '',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS db_versions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    db_server_id INTEGER NOT NULL DEFAULT 0,
    version TEXT NOT NULL,
    service_name TEXT DEFAULT '',
    config_file TEXT DEFAULT '',
    data_dir TEXT DEFAULT '',
    port INTEGER DEFAULT 0,
    status TEXT DEFAULT 'stopped',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(db_server_id, version)
);

CREATE INDEX IF NOT EXISTS idx_db_versions_server ON db_versions(db_server_id);

CREATE TABLE IF NOT EXISTS databases (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    db_server_id INTEGER NOT NULL DEFAULT 0,
    db_version_id INTEGER NOT NULL DEFAULT 0,
    name TEXT NOT NULL,
    charset TEXT DEFAULT 'utf8mb4',
    description TEXT DEFAULT '',
    size_bytes INTEGER DEFAULT 0,
    status TEXT DEFAULT 'active',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_databases_server ON databases(db_server_id);
CREATE INDEX IF NOT EXISTS idx_databases_version ON databases(db_version_id);

CREATE TABLE IF NOT EXISTS db_users (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    db_server_id INTEGER NOT NULL DEFAULT 0,
    username TEXT NOT NULL,
    password TEXT DEFAULT '',
    host TEXT DEFAULT 'localhost',
    privileges TEXT DEFAULT '',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_db_users_server ON db_users(db_server_id);

CREATE TABLE IF NOT EXISTS db_backups (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    db_server_id INTEGER NOT NULL,
    db_version_id INTEGER NOT NULL,
    database_id INTEGER DEFAULT 0,
    database_name TEXT NOT NULL,
    backup_type TEXT NOT NULL DEFAULT 'manual',
    file_path TEXT NOT NULL,
    file_size INTEGER DEFAULT 0,
    status TEXT DEFAULT 'completed',
    error_message TEXT DEFAULT '',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_db_backups_database_id ON db_backups(database_id);

-- =============================================
-- 9. Firewall rules (no dependencies)
-- =============================================

CREATE TABLE IF NOT EXISTS firewall_rules (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    chain TEXT NOT NULL DEFAULT 'INPUT',
    protocol TEXT NOT NULL DEFAULT 'tcp',
    port TEXT DEFAULT '',
    action TEXT NOT NULL DEFAULT 'ACCEPT',
    source TEXT DEFAULT '',
    target TEXT DEFAULT '',
    enabled INTEGER DEFAULT 1,
    priority INTEGER DEFAULT 0,
    ip_version TEXT DEFAULT 'ipv4',
    remark TEXT DEFAULT '',
    created_at TEXT DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_firewall_rules_chain ON firewall_rules(chain);
CREATE INDEX IF NOT EXISTS idx_firewall_rules_enabled ON firewall_rules(enabled);

-- =============================================
-- 10. Cron & script tables
-- =============================================

CREATE TABLE IF NOT EXISTS cron_tasks (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    command TEXT NOT NULL,
    schedule TEXT NOT NULL,
    description TEXT DEFAULT '',
    enabled INTEGER DEFAULT 1,
    status TEXT DEFAULT 'idle',
    last_run TEXT DEFAULT '',
    last_result TEXT DEFAULT '',
    next_run TEXT DEFAULT '',
    script_id INTEGER DEFAULT 0,
    timeout INTEGER DEFAULT 0,
    max_retry INTEGER DEFAULT 0,
    env_vars TEXT DEFAULT '',
    work_dir TEXT DEFAULT '',
    runtime_version_id INTEGER NOT NULL REFERENCES runtime_version(id),
    created_at TEXT DEFAULT (datetime('now')),
    updated_at TEXT DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_cron_tasks_enabled ON cron_tasks(enabled);

CREATE TABLE IF NOT EXISTS cron_logs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    task_id INTEGER NOT NULL,
    status TEXT NOT NULL,
    output TEXT DEFAULT '',
    duration INTEGER DEFAULT 0,
    created_at TEXT DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_cron_logs_task_id ON cron_logs(task_id);

CREATE TABLE IF NOT EXISTS scripts (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL UNIQUE,
    description TEXT DEFAULT '',
    content TEXT NOT NULL,
    language TEXT DEFAULT 'sh',
    created_at TEXT DEFAULT (datetime('now')),
    updated_at TEXT DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS cron_docs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    title TEXT NOT NULL,
    content TEXT NOT NULL,
    sort_order INTEGER DEFAULT 0,
    created_at TEXT DEFAULT (datetime('now')),
    updated_at TEXT DEFAULT (datetime('now'))
);

-- =============================================
-- 11. QR login (no dependencies)
-- =============================================

CREATE TABLE IF NOT EXISTS qr_login_sessions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    qr_token TEXT NOT NULL UNIQUE,
    status TEXT NOT NULL DEFAULT 'pending',  -- pending | confirmed | cancelled
    user_id INTEGER DEFAULT 0,
    web_token TEXT DEFAULT '',               -- 签发给 Web 的 JWT，领取后删除
    user_json TEXT DEFAULT '',               -- {user, must_change_pass} JSON，领取后删除
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    expires_at DATETIME NOT NULL,
    confirmed_at DATETIME
);

CREATE INDEX IF NOT EXISTS idx_qr_login_token ON qr_login_sessions(qr_token);
CREATE INDEX IF NOT EXISTS idx_qr_login_status ON qr_login_sessions(status);

-- =============================================
-- 12. File sharing (no dependencies)
-- =============================================

CREATE TABLE IF NOT EXISTS file_shares (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    file_path TEXT NOT NULL,
    file_name TEXT NOT NULL,
    file_size INTEGER DEFAULT 0,
    token TEXT NOT NULL UNIQUE,
    password TEXT DEFAULT '',
    expires_at DATETIME,
    max_downloads INTEGER DEFAULT 0,
    download_count INTEGER DEFAULT 0,
    created_by INTEGER NOT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_file_shares_token ON file_shares(token);
CREATE INDEX IF NOT EXISTS idx_file_shares_created_by ON file_shares(created_by);

-- =============================================
-- 13. File Integrity Monitoring (no dependencies)
-- =============================================

CREATE TABLE IF NOT EXISTS fim_baseline (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    path TEXT NOT NULL UNIQUE,
    hash TEXT NOT NULL,
    size INTEGER NOT NULL,
    mtime TEXT NOT NULL,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS fim_changes (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    path TEXT NOT NULL,
    change_type TEXT NOT NULL,   -- modified / added / deleted
    old_hash TEXT,
    new_hash TEXT,
    detected_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_fim_changes_path ON fim_changes(path);
CREATE INDEX IF NOT EXISTS idx_fim_changes_detected ON fim_changes(detected_at);

-- =============================================
-- 14. Website security (depends on websites)
-- =============================================

CREATE TABLE IF NOT EXISTS website_security_config (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    website_id INTEGER NOT NULL UNIQUE,
    rate_limit_enabled BOOLEAN DEFAULT 0,
    rate_limit_rate INTEGER DEFAULT 10,
    rate_limit_burst INTEGER DEFAULT 20,
    limit_conn INTEGER DEFAULT 100,
    auto_ban_enabled BOOLEAN DEFAULT 0,
    auto_ban_threshold INTEGER DEFAULT 100,
    auto_ban_404_threshold INTEGER DEFAULT 50,
    auto_ban_duration INTEGER DEFAULT 3600,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (website_id) REFERENCES websites(id)
);

CREATE TABLE IF NOT EXISTS website_banned_ip (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    website_id INTEGER,
    ip TEXT NOT NULL,
    reason TEXT,
    source TEXT DEFAULT 'auto',
    expires_at DATETIME,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (website_id) REFERENCES websites(id)
);

CREATE INDEX IF NOT EXISTS idx_website_banned_ip_ip ON website_banned_ip(ip);
CREATE INDEX IF NOT EXISTS idx_website_banned_ip_website ON website_banned_ip(website_id);
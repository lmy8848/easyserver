-- EasyServer Initial Schema Migration
-- Generated from codebase: database.go + service/*.go InitTables()
-- 27 tables total

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

-- =============================================
-- 2. Session & auth tables (depend on users)
-- =============================================

CREATE TABLE IF NOT EXISTS sessions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    token TEXT UNIQUE NOT NULL,
    user_id INTEGER NOT NULL,
    username TEXT NOT NULL,
    role TEXT NOT NULL,
    ip TEXT DEFAULT '',
    user_agent TEXT DEFAULT '',
    last_active DATETIME DEFAULT CURRENT_TIMESTAMP,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    expires_at DATETIME NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_sessions_user ON sessions(user_id);
CREATE INDEX IF NOT EXISTS idx_sessions_token ON sessions(token);
CREATE INDEX IF NOT EXISTS idx_sessions_last_active ON sessions(last_active);

CREATE TABLE IF NOT EXISTS user_activities (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL,
    username TEXT NOT NULL,
    action TEXT NOT NULL,
    ip TEXT DEFAULT '',
    user_agent TEXT DEFAULT '',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_user_activities_user ON user_activities(user_id);
CREATE INDEX IF NOT EXISTS idx_user_activities_created ON user_activities(created_at);

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
    signature TEXT DEFAULT '',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_audit_created_at ON audit_logs(created_at);

-- =============================================
-- 3. System monitoring (no dependencies)
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
-- 4. Runtime environments (no dependencies)
-- =============================================

CREATE TABLE IF NOT EXISTS runtime_environments (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    version TEXT NOT NULL,
    path TEXT NOT NULL,
    is_default INTEGER DEFAULT 0,
    status TEXT DEFAULT 'installed',
    progress INTEGER DEFAULT 0,
    progress_step TEXT DEFAULT '',
    logs TEXT DEFAULT '',
    error_message TEXT DEFAULT '',
    installed_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(name, version)
);

CREATE INDEX IF NOT EXISTS idx_runtime_name ON runtime_environments(name);

CREATE TABLE IF NOT EXISTS runtime_versions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    version TEXT NOT NULL,
    lts INTEGER DEFAULT 0,
    stable INTEGER DEFAULT 1,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(name, version)
);

CREATE INDEX IF NOT EXISTS idx_runtime_versions_name ON runtime_versions(name);

-- =============================================
-- 5. Environment configs (no dependencies)
-- =============================================

CREATE TABLE IF NOT EXISTS env_configs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    value TEXT NOT NULL,
    runtime_id INTEGER DEFAULT 0,
    is_global INTEGER DEFAULT 0,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(name, runtime_id)
);

CREATE INDEX IF NOT EXISTS idx_env_configs_runtime ON env_configs(runtime_id);

CREATE TABLE IF NOT EXISTS path_entries (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    path TEXT NOT NULL,
    runtime_id INTEGER DEFAULT 0,
    is_global INTEGER DEFAULT 0,
    order_num INTEGER DEFAULT 0,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(path, runtime_id)
);

CREATE INDEX IF NOT EXISTS idx_path_entries_runtime ON path_entries(runtime_id);

CREATE TABLE IF NOT EXISTS global_configs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    category TEXT NOT NULL,
    key TEXT NOT NULL,
    value TEXT NOT NULL,
    description TEXT DEFAULT '',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(category, key)
);

CREATE INDEX IF NOT EXISTS idx_global_configs_category ON global_configs(category);

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
    created_at TEXT DEFAULT (datetime('now')),
    updated_at TEXT DEFAULT (datetime('now'))
);

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
-- 11. Package manager (depends on runtime_environments)
-- =============================================

CREATE TABLE IF NOT EXISTS packages (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    runtime_id INTEGER NOT NULL,
    name TEXT NOT NULL,
    version TEXT NOT NULL,
    scope TEXT DEFAULT 'global',
    source TEXT NOT NULL,
    installed_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(runtime_id, name, scope)
);

CREATE INDEX IF NOT EXISTS idx_packages_runtime ON packages(runtime_id);

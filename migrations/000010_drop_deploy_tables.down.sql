-- 回滚：重新创建 deploy 相关表
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

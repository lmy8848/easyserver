-- EasyServer Process Guardian Migration
-- 4 tables for process management

PRAGMA foreign_keys = ON;

-- Process configuration
CREATE TABLE IF NOT EXISTS processes (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL UNIQUE,
    command TEXT NOT NULL,
    args TEXT DEFAULT '',
    dir TEXT DEFAULT '',
    env TEXT DEFAULT '{}',
    auto_restart INTEGER DEFAULT 1,
    max_restarts INTEGER DEFAULT 10,
    restart_delay INTEGER DEFAULT 5,
    auto_start INTEGER DEFAULT 0,
    log_file TEXT DEFAULT '',
    group_id INTEGER DEFAULT 0,
    created_at TEXT DEFAULT (datetime('now')),
    updated_at TEXT DEFAULT (datetime('now'))
);

-- Process runtime status
CREATE TABLE IF NOT EXISTS process_status (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    process_id INTEGER NOT NULL UNIQUE,
    status TEXT DEFAULT 'stopped',
    pid INTEGER DEFAULT 0,
    uptime INTEGER DEFAULT 0,
    restarts INTEGER DEFAULT 0,
    cpu_percent REAL DEFAULT 0,
    memory_mb REAL DEFAULT 0,
    exit_code INTEGER DEFAULT 0,
    last_start TEXT,
    last_error TEXT,
    updated_at TEXT DEFAULT (datetime('now')),
    FOREIGN KEY (process_id) REFERENCES processes(id) ON DELETE CASCADE
);

-- Process logs
CREATE TABLE IF NOT EXISTS process_logs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    process_id INTEGER NOT NULL,
    type TEXT NOT NULL,
    content TEXT NOT NULL,
    created_at TEXT DEFAULT (datetime('now')),
    FOREIGN KEY (process_id) REFERENCES processes(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_process_logs_process_id ON process_logs(process_id);
CREATE INDEX IF NOT EXISTS idx_process_logs_created_at ON process_logs(created_at);

-- Process groups
CREATE TABLE IF NOT EXISTS process_groups (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL UNIQUE,
    description TEXT DEFAULT '',
    created_at TEXT DEFAULT (datetime('now'))
);
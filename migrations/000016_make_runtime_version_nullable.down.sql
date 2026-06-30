-- Revert: make runtime_version_id NOT NULL again

PRAGMA foreign_keys = OFF;

CREATE TABLE IF NOT EXISTS processes_old (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL UNIQUE,
    command TEXT NOT NULL,
    args TEXT DEFAULT '',
    dir TEXT DEFAULT '',
    env TEXT DEFAULT '{}',
    auto_restart INTEGER DEFAULT 1,
    max_restarts INTEGER DEFAULT 10,
    restart_delay INTEGER DEFAULT 5,
    stop_timeout INTEGER DEFAULT 10,
    startup_timeout INTEGER DEFAULT 30,
    auto_start INTEGER DEFAULT 0,
    log_file TEXT DEFAULT '',
    group_id INTEGER DEFAULT 0,
    runtime_version_id INTEGER NOT NULL REFERENCES runtime_version(id),
    created_at TEXT DEFAULT (datetime('now')),
    updated_at TEXT DEFAULT (datetime('now'))
);

INSERT INTO processes_old SELECT * FROM processes;
DROP TABLE processes;
ALTER TABLE processes_old RENAME TO processes;

PRAGMA foreign_keys = ON;

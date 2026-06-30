-- Make runtime_version_id nullable for website-linked processes
-- Website start commands (e.g. "npm start", "java -jar app.jar") don't always
-- need a specific runtime version — they use whatever is on $PATH.

PRAGMA foreign_keys = OFF;

CREATE TABLE IF NOT EXISTS processes_new (
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
    runtime_version_id INTEGER REFERENCES runtime_version(id),
    created_at TEXT DEFAULT (datetime('now')),
    updated_at TEXT DEFAULT (datetime('now'))
);

INSERT INTO processes_new SELECT * FROM processes;
DROP TABLE processes;
ALTER TABLE processes_new RENAME TO processes;

PRAGMA foreign_keys = ON;

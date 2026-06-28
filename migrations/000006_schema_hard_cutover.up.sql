-- Hard cutover for schema and runtime management

CREATE TABLE IF NOT EXISTS runtime_version (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    lang TEXT NOT NULL CHECK(lang IN ('node', 'python', 'go', 'java', 'php')),
    major TEXT NOT NULL,
    exact TEXT NOT NULL,
    installed_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    status TEXT DEFAULT 'installed'
);

CREATE TABLE IF NOT EXISTS global_default (
    lang TEXT PRIMARY KEY CHECK(lang IN ('node', 'python', 'go', 'java', 'php')),
    runtime_version_id INTEGER NOT NULL,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (runtime_version_id) REFERENCES runtime_version(id)
);

CREATE TABLE IF NOT EXISTS runtime_mirror (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    lang TEXT NOT NULL CHECK(lang IN ('node', 'python', 'go', 'java', 'php')),
    env_key TEXT NOT NULL,
    env_value TEXT NOT NULL,
    enabled INTEGER DEFAULT 1,
    source TEXT NOT NULL CHECK(source IN ('seed', 'user')),
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

ALTER TABLE processes ADD COLUMN runtime_version_id INTEGER NOT NULL REFERENCES runtime_version(id);
ALTER TABLE cron_tasks ADD COLUMN runtime_version_id INTEGER NOT NULL REFERENCES runtime_version(id);

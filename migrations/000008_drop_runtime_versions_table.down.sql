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

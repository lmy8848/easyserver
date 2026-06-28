-- 回滚：重建 packages 表（不恢复历史数据）
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

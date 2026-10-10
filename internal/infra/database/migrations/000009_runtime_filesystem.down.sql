-- Rollback: restore the runtime_version table and the websites column.
-- 目录扫描数据不会回填（面板重装后从 installs/ 重新扫描，ADR-0009 手动迁移惯例）。

PRAGMA foreign_keys = OFF;

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

ALTER TABLE websites ADD COLUMN runtime_version_id INTEGER DEFAULT 0;

PRAGMA foreign_keys = ON;

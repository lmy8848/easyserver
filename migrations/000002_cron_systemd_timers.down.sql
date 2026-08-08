-- 回滚 000002：还原 cron_tasks / cron_logs 表与 scripts.content 列。
-- 面板无部署，回滚仅用于开发环境重建，不承载迁移旧数据。

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

CREATE TABLE IF NOT EXISTS cron_docs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    title TEXT NOT NULL,
    content TEXT NOT NULL,
    sort_order INTEGER DEFAULT 0,
    created_at TEXT DEFAULT (datetime('now')),
    updated_at TEXT DEFAULT (datetime('now'))
);

ALTER TABLE scripts ADD COLUMN content TEXT NOT NULL DEFAULT '';
CREATE TABLE env_configs_old (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    value TEXT NOT NULL,
    runtime_id INTEGER DEFAULT 0,
    is_global INTEGER DEFAULT 0,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(name, runtime_id)
);
INSERT INTO env_configs_old (id, name, value, is_global, created_at, updated_at)
SELECT id, name, value, 1, created_at, updated_at FROM env_configs;
DROP TABLE env_configs;
ALTER TABLE env_configs_old RENAME TO env_configs;
CREATE INDEX idx_env_configs_runtime ON env_configs(runtime_id);

CREATE TABLE path_entries_old (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    path TEXT NOT NULL,
    runtime_id INTEGER DEFAULT 0,
    is_global INTEGER DEFAULT 0,
    order_num INTEGER DEFAULT 0,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(path, runtime_id)
);
INSERT INTO path_entries_old (id, path, is_global, order_num, created_at)
SELECT id, path, 1, order_num, created_at FROM path_entries;
DROP TABLE path_entries;
ALTER TABLE path_entries_old RENAME TO path_entries;
CREATE INDEX idx_path_entries_runtime ON path_entries(runtime_id);

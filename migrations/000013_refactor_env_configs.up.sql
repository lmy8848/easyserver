-- Create new env_configs table
CREATE TABLE env_configs_new (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL UNIQUE,
    value TEXT NOT NULL,
    enabled INTEGER DEFAULT 1,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Copy global configurations
INSERT INTO env_configs_new (id, name, value, created_at, updated_at)
SELECT MAX(id), name, value, created_at, updated_at FROM env_configs 
WHERE is_global = 1 OR runtime_id = 0
GROUP BY name;

DROP TABLE env_configs;
ALTER TABLE env_configs_new RENAME TO env_configs;
CREATE INDEX idx_env_configs_name ON env_configs(name);

-- Create new path_entries table
CREATE TABLE path_entries_new (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    path TEXT NOT NULL UNIQUE,
    enabled INTEGER DEFAULT 1,
    order_num INTEGER DEFAULT 0,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Copy global path entries
INSERT INTO path_entries_new (id, path, order_num, created_at)
SELECT MAX(id), path, order_num, created_at FROM path_entries 
WHERE is_global = 1 OR runtime_id = 0
GROUP BY path;

DROP TABLE path_entries;
ALTER TABLE path_entries_new RENAME TO path_entries;
CREATE INDEX idx_path_entries_path ON path_entries(path);

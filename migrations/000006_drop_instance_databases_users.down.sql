-- Rollback: restore the persisted logical database/user catalog tables and the
-- database_id FK on instance_backups. Persisted rows are not recoverable — they
-- were live engine state, so the tables are recreated empty.

PRAGMA foreign_keys = OFF;

CREATE TABLE instance_databases (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    instance_id INTEGER NOT NULL REFERENCES database_instances(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    charset TEXT DEFAULT 'utf8mb4',
    description TEXT DEFAULT '',
    size_bytes INTEGER DEFAULT 0,
    status TEXT DEFAULT 'active',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(instance_id, name)
);

CREATE INDEX idx_instance_databases_instance ON instance_databases(instance_id);

CREATE TABLE instance_users (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    instance_id INTEGER NOT NULL REFERENCES database_instances(id) ON DELETE CASCADE,
    username TEXT NOT NULL,
    password TEXT DEFAULT '',
    host TEXT DEFAULT '%',
    privileges TEXT DEFAULT '',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(instance_id, username, host)
);

CREATE TABLE instance_backups_new (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    instance_id INTEGER NOT NULL REFERENCES database_instances(id) ON DELETE CASCADE,
    database_id INTEGER REFERENCES instance_databases(id) ON DELETE CASCADE,
    database_name TEXT NOT NULL,
    backup_type TEXT DEFAULT 'manual',
    file_path TEXT NOT NULL,
    file_size INTEGER DEFAULT 0,
    status TEXT DEFAULT 'pending',
    error_message TEXT DEFAULT '',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

INSERT INTO instance_backups_new
    (id, instance_id, database_name, backup_type, file_path, file_size, status, error_message, created_at)
SELECT id, instance_id, database_name, backup_type, file_path, file_size, status, error_message, created_at
FROM instance_backups;

DROP TABLE instance_backups;
ALTER TABLE instance_backups_new RENAME TO instance_backups;
CREATE INDEX idx_instance_backups_database ON instance_backups(database_id);

PRAGMA foreign_keys = ON;

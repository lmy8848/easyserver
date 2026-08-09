-- Logical databases and users are now live engine state, queried in real time
-- (SHOW DATABASES / mysql.user / pg_database / pg_roles). The panel no longer
-- persists a mirror of them — instance_databases and instance_users go away.
--
-- instance_backups keeps its database_name column (backups are files we own),
-- but the database_id FK column (which referenced instance_databases) is
-- dropped; backups are now addressed by (instance_id, database_name).

PRAGMA foreign_keys = OFF;

CREATE TABLE instance_backups_new (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    instance_id INTEGER NOT NULL REFERENCES database_instances(id) ON DELETE CASCADE,
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
CREATE INDEX idx_instance_backups_name ON instance_backups(database_name);

DROP TABLE instance_databases;
DROP TABLE instance_users;

PRAGMA foreign_keys = ON;

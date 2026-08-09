-- Remove the database_engines directory table. The engine is just an enum
-- (db_type) on each instance row; the aggregate status/version summary is
-- recomputed in memory on read instead of persisted.
--
-- database_instances changes:
--   engine_id INTEGER NOT NULL REFERENCES database_engines(id)  ->  db_type TEXT NOT NULL
--   container_name TEXT NOT NULL UNIQUE                          ->  removed (container_id is the only reference)
--
-- id values are preserved so instance_databases/users/backups FK references
-- are unaffected. Existing rows keep their container_id value (a container
-- name token), which the container runtimes resolve just like an id.

PRAGMA foreign_keys = OFF;

CREATE TABLE database_instances_new (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    db_type TEXT NOT NULL CHECK(db_type IN ('mysql','postgresql','redis')),
    version TEXT NOT NULL,
    runtime TEXT NOT NULL DEFAULT 'docker' CHECK(runtime IN ('docker','podman')),
    image TEXT NOT NULL,
    container_id TEXT NOT NULL UNIQUE,
    volume_name TEXT NOT NULL UNIQUE,
    config_dir TEXT DEFAULT '',
    bind_address TEXT NOT NULL DEFAULT '127.0.0.1',
    port INTEGER NOT NULL,
    admin_password TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'provisioning',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- INNER JOIN: an orphan engine_id makes the migration fail loudly instead of
-- silently corrupting db_type (foreign keys guarantee it should not exist).
INSERT INTO database_instances_new
    (id, db_type, version, runtime, image, container_id, volume_name, config_dir,
     bind_address, port, admin_password, status, created_at, updated_at)
SELECT i.id, e.name, i.version, i.runtime, i.image, i.container_id, i.volume_name, i.config_dir,
       i.bind_address, i.port, i.admin_password, i.status, i.created_at, i.updated_at
FROM database_instances i
JOIN database_engines e ON i.engine_id = e.id;

DROP TABLE database_instances;
ALTER TABLE database_instances_new RENAME TO database_instances;
CREATE INDEX idx_database_instances_db_type ON database_instances(db_type);
DROP TABLE database_engines;

PRAGMA foreign_keys = ON;
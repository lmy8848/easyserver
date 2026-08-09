-- Database management rebuilt around container-backed Database Instances.
--
-- Final shape (no intermediate states):
--   database_instances — one row per container-backed instance; the engine is
--     an enum (db_type), the container is addressed by container_id.
--   instance_backups    — backup files we own (files are ours, so they are
--     persisted; addressed by (instance_id, database_name)).
-- Logical databases and users are NOT stored: they are live engine state,
-- queried in real time (SHOW DATABASES / mysql.user / pg_database / pg_roles).
--
-- The pre-container dbserver catalog tables are intentionally discarded; host
-- database processes and their data are never touched.

PRAGMA foreign_keys = OFF;

DROP TABLE IF EXISTS db_backups;
DROP TABLE IF EXISTS db_users;
DROP TABLE IF EXISTS databases;
DROP TABLE IF EXISTS db_versions;
DROP TABLE IF EXISTS db_servers;

CREATE TABLE database_instances (
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

CREATE INDEX idx_database_instances_db_type ON database_instances(db_type);

CREATE TABLE instance_backups (
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

CREATE INDEX idx_instance_backups_name ON instance_backups(database_name);

PRAGMA foreign_keys = ON;

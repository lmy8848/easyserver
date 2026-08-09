-- Database management is rebuilt around container-backed Database Instances.
-- Existing database management metadata is intentionally discarded; host
-- database processes and their data are never touched by this migration.

PRAGMA foreign_keys = OFF;

DROP TABLE IF EXISTS db_backups;
DROP TABLE IF EXISTS db_users;
DROP TABLE IF EXISTS databases;
DROP TABLE IF EXISTS db_versions;
DROP TABLE IF EXISTS db_servers;

CREATE TABLE database_engines (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL UNIQUE,
    display_name TEXT NOT NULL,
    description TEXT DEFAULT '',
    default_port INTEGER NOT NULL,
    status TEXT NOT NULL DEFAULT 'not_installed',
    version TEXT DEFAULT '',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE database_instances (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    engine_id INTEGER NOT NULL REFERENCES database_engines(id),
    version TEXT NOT NULL,
    runtime TEXT NOT NULL DEFAULT 'docker' CHECK(runtime IN ('docker', 'podman')),
    image TEXT NOT NULL,
    container_name TEXT NOT NULL UNIQUE,
    container_id TEXT DEFAULT '',
    volume_name TEXT NOT NULL UNIQUE,
    config_dir TEXT DEFAULT '',
    bind_address TEXT NOT NULL DEFAULT '127.0.0.1',
    port INTEGER NOT NULL,
    admin_user TEXT NOT NULL DEFAULT '',
    admin_password TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'provisioning',
    health_status TEXT NOT NULL DEFAULT 'starting',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_database_instances_engine ON database_instances(engine_id);

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

CREATE TABLE instance_backups (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    instance_id INTEGER NOT NULL REFERENCES database_instances(id) ON DELETE CASCADE,
    database_id INTEGER NOT NULL REFERENCES instance_databases(id) ON DELETE CASCADE,
    database_name TEXT NOT NULL,
    backup_type TEXT DEFAULT 'manual',
    file_path TEXT NOT NULL,
    file_size INTEGER DEFAULT 0,
    status TEXT DEFAULT 'pending',
    error_message TEXT DEFAULT '',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_instance_backups_database ON instance_backups(database_id);

PRAGMA foreign_keys = ON;

-- Best-effort down migration: recreate database_engines (seeded with the three
-- predefined engines) and restore database_instances.engine_id + container_name.
-- The engine aggregate status/version summary is recomputed from the instances
-- at the time of the rollback.

PRAGMA foreign_keys = OFF;

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

INSERT INTO database_engines (name, display_name, description, default_port) VALUES
    ('mysql', 'MySQL', '最流行的关系型数据库，广泛用于 Web 应用', 3306),
    ('postgresql', 'PostgreSQL', '功能强大的开源关系型数据库', 5432),
    ('redis', 'Redis', '高性能内存数据库，用于缓存和消息队列', 6379);

CREATE TABLE database_instances_new (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    engine_id INTEGER NOT NULL REFERENCES database_engines(id),
    version TEXT NOT NULL,
    runtime TEXT NOT NULL DEFAULT 'docker' CHECK(runtime IN ('docker','podman')),
    image TEXT NOT NULL,
    container_name TEXT NOT NULL UNIQUE,
    container_id TEXT DEFAULT '',
    volume_name TEXT NOT NULL UNIQUE,
    config_dir TEXT DEFAULT '',
    bind_address TEXT NOT NULL DEFAULT '127.0.0.1',
    port INTEGER NOT NULL,
    admin_password TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'provisioning',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- container_name is restored as a copy of container_id (the managed token).
INSERT INTO database_instances_new
    (id, engine_id, version, runtime, image, container_name, container_id, volume_name, config_dir,
     bind_address, port, admin_password, status, created_at, updated_at)
SELECT i.id, e.id, i.version, i.runtime, i.image, i.container_id, i.container_id, i.volume_name, i.config_dir,
       i.bind_address, i.port, i.admin_password, i.status, i.created_at, i.updated_at
FROM database_instances i
JOIN database_engines e ON i.db_type = e.name;

DROP TABLE database_instances;
ALTER TABLE database_instances_new RENAME TO database_instances;
CREATE INDEX idx_database_instances_engine ON database_instances(engine_id);

PRAGMA foreign_keys = ON;
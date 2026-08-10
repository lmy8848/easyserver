-- Add container_port: the port the database engine actually listens on INSIDE
-- the container. It is always the engine default (MySQL 3306 / PostgreSQL 5432
-- / Redis 6379); the user-selected port (port column) is only the host mapping
-- (HostPort). The pre-fix containerSpec mapped the user port 1:1 onto the
-- container port (`--publish X:X`), so for X != engine default the host port
-- pointed at a container port where nothing listens.
--
-- Backfill: old instances whose host port equals the engine default have a
-- self-consistent mapping (the engine listens on the default inside the
-- container, and the host default maps to it) — they are directly connectable
-- and get container_port backfilled. Instances with a broken mapping keep
-- container_port = 0, meaning the direct channel is unavailable and the CLI
-- channel (docker exec) remains the only path; the UI surfaces a hint to change
-- the port so the instance can move to direct connection.

ALTER TABLE database_instances ADD COLUMN container_port INTEGER NOT NULL DEFAULT 0;

UPDATE database_instances SET container_port = 3306 WHERE db_type = 'mysql' AND port = 3306;
UPDATE database_instances SET container_port = 5432 WHERE db_type = 'postgresql' AND port = 5432;
UPDATE database_instances SET container_port = 6379 WHERE db_type = 'redis' AND port = 6379;

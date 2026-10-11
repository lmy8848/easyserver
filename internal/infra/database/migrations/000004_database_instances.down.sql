-- Rollback: drop the container-backed database tables, returning to the
-- 000003 state (no database management tables).

PRAGMA foreign_keys = OFF;
DROP TABLE IF EXISTS instance_backups;
DROP TABLE IF EXISTS database_instances;
PRAGMA foreign_keys = ON;

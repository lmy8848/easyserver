-- Rollback: drop the container_port column, returning to the 000004 shape.
ALTER TABLE database_instances DROP COLUMN container_port;

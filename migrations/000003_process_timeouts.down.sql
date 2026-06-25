-- Remove timeout columns
ALTER TABLE processes DROP COLUMN stop_timeout;
ALTER TABLE processes DROP COLUMN startup_timeout;

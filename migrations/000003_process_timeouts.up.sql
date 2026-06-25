-- Add new columns for graceful shutdown, startup timeout
ALTER TABLE processes ADD COLUMN stop_timeout INTEGER DEFAULT 10;
ALTER TABLE processes ADD COLUMN startup_timeout INTEGER DEFAULT 30;

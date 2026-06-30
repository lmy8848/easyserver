-- Add runtime_version_id to websites for process guardian linking
ALTER TABLE websites ADD COLUMN runtime_version_id INTEGER DEFAULT 0;

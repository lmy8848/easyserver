-- Issue 02 hard cutover left runtime_version with only 6 columns, but the
-- service / repo code reads/writes progress, progress_step, logs and
-- error_message on it. HealState and ListAll throw "no such column".
-- Add them here rather than rewriting migration 6 (already applied).

ALTER TABLE runtime_version ADD COLUMN progress INTEGER NOT NULL DEFAULT 0;
ALTER TABLE runtime_version ADD COLUMN progress_step TEXT NOT NULL DEFAULT '';
ALTER TABLE runtime_version ADD COLUMN logs TEXT NOT NULL DEFAULT '';
ALTER TABLE runtime_version ADD COLUMN error_message TEXT NOT NULL DEFAULT '';

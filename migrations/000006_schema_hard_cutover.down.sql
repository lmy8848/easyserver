-- Revert Hard cutover

ALTER TABLE processes DROP COLUMN runtime_version_id;
ALTER TABLE cron_tasks DROP COLUMN runtime_version_id;

DROP TABLE IF EXISTS runtime_mirror;
DROP TABLE IF EXISTS global_default;
DROP TABLE IF EXISTS runtime_version;

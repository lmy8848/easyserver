-- 000005_add_missing_indexes.down.sql

DROP INDEX IF EXISTS idx_db_backups_database_id;
DROP INDEX IF EXISTS idx_firewall_rules_chain;
DROP INDEX IF EXISTS idx_firewall_rules_enabled;
DROP INDEX IF EXISTS idx_audit_logs_action;
DROP INDEX IF EXISTS idx_cron_tasks_enabled;

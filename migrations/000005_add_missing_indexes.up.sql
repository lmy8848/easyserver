-- 添加缺失索引：db_backups, firewall_rules, audit_logs, cron_tasks
-- 000005_add_missing_indexes.up.sql

-- db_backups: 按 database_id 查询备份列表
CREATE INDEX IF NOT EXISTS idx_db_backups_database_id ON db_backups(database_id);

-- firewall_rules: 按 chain/enabled 查询规则
CREATE INDEX IF NOT EXISTS idx_firewall_rules_chain ON firewall_rules(chain);
CREATE INDEX IF NOT EXISTS idx_firewall_rules_enabled ON firewall_rules(enabled);

-- audit_logs: 按 action 做 DISTINCT 查询
CREATE INDEX IF NOT EXISTS idx_audit_logs_action ON audit_logs(action);

-- cron_tasks: 按 enabled/status 筛选
CREATE INDEX IF NOT EXISTS idx_cron_tasks_enabled ON cron_tasks(enabled);

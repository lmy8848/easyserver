-- Rollback the entire initial schema. DROP order is dependency-reversed
-- (children before parents) so FOREIGN KEY constraints don't block it.

PRAGMA foreign_keys = OFF;

DROP TABLE IF EXISTS website_banned_ip;
DROP TABLE IF EXISTS website_security_config;
DROP TABLE IF EXISTS fim_changes;
DROP TABLE IF EXISTS fim_baseline;
DROP TABLE IF EXISTS file_shares;
DROP TABLE IF EXISTS qr_login_sessions;
DROP TABLE IF EXISTS cron_docs;
DROP TABLE IF EXISTS scripts;
DROP TABLE IF EXISTS cron_logs;
DROP TABLE IF EXISTS cron_tasks;
DROP TABLE IF EXISTS firewall_rules;
DROP TABLE IF EXISTS db_backups;
DROP TABLE IF EXISTS db_users;
DROP TABLE IF EXISTS databases;
DROP TABLE IF EXISTS db_versions;
DROP TABLE IF EXISTS db_servers;
DROP TABLE IF EXISTS websites;
DROP TABLE IF EXISTS web_servers;
DROP TABLE IF EXISTS deploy_versions;
DROP TABLE IF EXISTS deploy_tasks;
DROP TABLE IF EXISTS deploy_servers;
DROP TABLE IF EXISTS path_entries;
DROP TABLE IF EXISTS env_configs;
DROP TABLE IF EXISTS runtime_version;
DROP TABLE IF EXISTS notifications;
DROP TABLE IF EXISTS monitor_data;
DROP TABLE IF EXISTS audit_logs;
DROP TABLE IF EXISTS token_blacklist;
DROP TABLE IF EXISTS users;

PRAGMA foreign_keys = ON;
-- EasyServer Initial Schema Rollback
-- Drops all 27 tables in reverse dependency order

PRAGMA foreign_keys = OFF;

-- 11. Package manager
DROP TABLE IF EXISTS packages;

-- 10. Cron & script tables
DROP TABLE IF EXISTS cron_docs;
DROP TABLE IF EXISTS scripts;
DROP TABLE IF EXISTS cron_logs;
DROP TABLE IF EXISTS cron_tasks;

-- 9. Firewall rules
DROP TABLE IF EXISTS firewall_rules;

-- 8. Database server tables
DROP TABLE IF EXISTS db_backups;
DROP TABLE IF EXISTS db_users;
DROP TABLE IF EXISTS databases;
DROP TABLE IF EXISTS db_versions;
DROP TABLE IF EXISTS db_servers;

-- 7. Web server & website tables
DROP TABLE IF EXISTS websites;
DROP TABLE IF EXISTS web_servers;

-- 6. Deploy tables
DROP TABLE IF EXISTS deploy_versions;
DROP TABLE IF EXISTS deploy_tasks;
DROP TABLE IF EXISTS deploy_servers;

-- 5. Environment configs
DROP TABLE IF EXISTS global_configs;
DROP TABLE IF EXISTS path_entries;
DROP TABLE IF EXISTS env_configs;

-- 4. Runtime environments
DROP TABLE IF EXISTS runtime_versions;
DROP TABLE IF EXISTS runtime_environments;

-- 3. System monitoring
DROP TABLE IF EXISTS monitor_data;

-- 2. Session & auth tables
DROP TABLE IF EXISTS audit_logs;
DROP TABLE IF EXISTS token_blacklist;
DROP TABLE IF EXISTS user_activities;
DROP TABLE IF EXISTS sessions;

-- 1. Core user tables
DROP TABLE IF EXISTS users;

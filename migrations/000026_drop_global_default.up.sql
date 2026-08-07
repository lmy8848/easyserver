-- global_default 表已无实际用途：config.toml 不再生成 [tools] 段，
-- Process/Cron/Systemd 均用 mise exec <lang>@<exact> 显式指定版本。
DROP TABLE IF EXISTS global_default;
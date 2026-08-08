-- 定时任务承载从系统 crond 迁移到 systemd timers（ADR-0004）。
--
-- - cron_tasks / cron_logs 表删除：timer unit 是唯一权威，日志走 journald。
-- - scripts 表降为元数据：删除 content 列，脚本内容落盘
--   /opt/easyserver/scripts/<id>.<ext>（由 cron 包的脚本仓库读写文件）。
--   已有脚本内容在迁移时手动落盘，代码不自动迁移（见 ADR-0004.Consequences）。
-- - SQLite 3.35+ 支持 ALTER TABLE DROP COLUMN。

DROP TABLE IF EXISTS cron_tasks;
DROP TABLE IF EXISTS cron_logs;

-- 帮助手册改为前端内置（无需后端存储），删除 cron_docs 表。
DROP TABLE IF EXISTS cron_docs;

ALTER TABLE scripts DROP COLUMN content;
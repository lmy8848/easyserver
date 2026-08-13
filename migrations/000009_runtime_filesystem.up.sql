-- 运行环境权威从 runtime_version 表迁移到 installs/ 文件系统（ADR-0009）。
--
-- - runtime_version 表删除：安装产物（目录 + 完成标记）是唯一权威，
--   installing/failed/进度/日志等过程态只存 task 执行器内存，重启即失。
-- - websites.runtime_version_id 列删除：无 FK、从未参与执行（仅元数据），
--   面板不解析此绑定。

PRAGMA foreign_keys = OFF;

DROP TABLE IF EXISTS runtime_version;

ALTER TABLE websites DROP COLUMN runtime_version_id;

PRAGMA foreign_keys = ON;

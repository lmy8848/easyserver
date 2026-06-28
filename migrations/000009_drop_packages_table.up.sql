-- 删除 packages 表
-- 包安装信息以系统包管理器（npm/pip 等）的实时输出为准，
-- 应用层不再持久化包列表，避免 DB 与磁盘不一致。

DROP INDEX IF EXISTS idx_packages_runtime;
DROP TABLE IF EXISTS packages;

-- Drop legacy process guardian tables.
-- 进程守护已迁移到 systemd unit 文件方案（见 .scratch/process-systemd-merge/PRD.md）。
-- 配置权威由 /etc/systemd/system/easyserver-*.service 承载，日志走 journald，
-- 状态走 systemctl show。旧表数据不迁移，直接清空。

DROP TABLE IF EXISTS process_logs;
DROP TABLE IF EXISTS process_status;
DROP TABLE IF EXISTS process_groups;
DROP TABLE IF EXISTS processes;

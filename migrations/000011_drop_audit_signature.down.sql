-- 回滚：恢复 signature 列（不再写入，仅恢复结构兼容历史）。
ALTER TABLE audit_logs ADD COLUMN signature TEXT DEFAULT '';

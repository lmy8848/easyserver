-- 不可回滚：旧表数据已删除，down 无法恢复。
-- 占位文件保持 migration 配对约定；如需回滚请从备份恢复 DB。
SELECT 1;

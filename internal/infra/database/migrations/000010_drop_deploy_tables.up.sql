-- 移除部署同步功能相关的三张数据表与关联索引
DROP TABLE IF EXISTS deploy_versions;
DROP TABLE IF EXISTS deploy_tasks;
DROP TABLE IF EXISTS deploy_servers;

-- 审计日志按类型拆分：operation（业务操作）/ request（HTTP 请求）
-- operation = 各 LogXxx 业务方法写入（带 FILE_/SECURITY_/SYSTEM_/SERVICE_/TERMINAL_ 等语义前缀）
-- request    = 全局中间件记录的 HTTP 写请求（action 为 HTTP method）
ALTER TABLE audit_logs ADD COLUMN type TEXT NOT NULL DEFAULT 'operation';
UPDATE audit_logs SET type='request' WHERE action IN ('GET','POST','PUT','DELETE','PATCH','HEAD','OPTIONS');
CREATE INDEX IF NOT EXISTS idx_audit_logs_type ON audit_logs(type);

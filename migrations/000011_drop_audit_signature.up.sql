-- 移除 audit_logs.signature 列：HMAC 签名机制已废弃。
-- 原签名防篡改无效：签名密钥与审计日志同存于一个 SQLite 库，能写日志者即可重算合法签名；
-- 且单行 HMAC 无法检测整行删除。重建表删除该列。
CREATE TABLE audit_logs_new (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER,
    username TEXT,
    action TEXT NOT NULL,
    resource TEXT,
    detail TEXT,
    ip TEXT,
    user_agent TEXT,
    type TEXT NOT NULL DEFAULT 'operation',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
INSERT INTO audit_logs_new (id, user_id, username, action, resource, detail, ip, user_agent, type, created_at)
    SELECT id, user_id, username, action, resource, detail, ip, user_agent, type, created_at FROM audit_logs;
DROP TABLE audit_logs;
ALTER TABLE audit_logs_new RENAME TO audit_logs;
CREATE INDEX IF NOT EXISTS idx_audit_created_at ON audit_logs(created_at);
CREATE INDEX IF NOT EXISTS idx_audit_logs_type ON audit_logs(type);
CREATE INDEX IF NOT EXISTS idx_audit_logs_action ON audit_logs(action);

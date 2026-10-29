-- 移动端直连通道的设备公钥：设备首次通过面板（CF）注册 Ed25519 公钥，
-- 之后直连端口上的每个请求都用私钥签名，服务端据此识别设备与所属用户。
CREATE TABLE IF NOT EXISTS device_keys (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    device_id TEXT NOT NULL UNIQUE,
    user_id INTEGER NOT NULL,
    public_key TEXT NOT NULL,          -- base64 标准编码的 32 字节 Ed25519 公钥
    name TEXT DEFAULT '',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    last_seen_at DATETIME,
    revoked_at DATETIME
);
CREATE INDEX idx_device_keys_user ON device_keys(user_id);

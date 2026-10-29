-- 移动端刷新令牌：不透明随机串只存 SHA-256 哈希，按设备绑定，支持轮换与整链吊销。
CREATE TABLE IF NOT EXISTS refresh_tokens (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    token_hash TEXT NOT NULL UNIQUE,
    family_id TEXT NOT NULL,
    user_id INTEGER NOT NULL,
    device_id TEXT NOT NULL,
    expires_at DATETIME NOT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    rotated_from TEXT,                -- 由哪个 token_hash 轮换而来（重放检测）
    rotated_at DATETIME,
    revoked_at DATETIME
);
CREATE INDEX idx_refresh_tokens_user ON refresh_tokens(user_id);
CREATE INDEX idx_refresh_tokens_family ON refresh_tokens(family_id);
CREATE INDEX idx_refresh_tokens_device ON refresh_tokens(device_id);

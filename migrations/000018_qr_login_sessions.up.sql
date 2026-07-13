-- QR login sessions: Web 面板显码、手机扫码确认的一次性登录会话
CREATE TABLE IF NOT EXISTS qr_login_sessions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    qr_token TEXT NOT NULL UNIQUE,
    status TEXT NOT NULL DEFAULT 'pending',  -- pending | confirmed | cancelled
    user_id INTEGER DEFAULT 0,
    web_token TEXT DEFAULT '',               -- 签发给 Web 的 JWT，领取后删除
    user_json TEXT DEFAULT '',               -- {user, must_change_pass} JSON，领取后删除
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    expires_at DATETIME NOT NULL,
    confirmed_at DATETIME
);
CREATE INDEX IF NOT EXISTS idx_qr_login_token ON qr_login_sessions(qr_token);
CREATE INDEX IF NOT EXISTS idx_qr_login_status ON qr_login_sessions(status);

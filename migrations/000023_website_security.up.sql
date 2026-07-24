-- Website security: per-website rate-limit config + IP ban records.
CREATE TABLE IF NOT EXISTS website_security_config (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    website_id INTEGER NOT NULL UNIQUE,
    rate_limit_enabled BOOLEAN DEFAULT 0,
    rate_limit_rate INTEGER DEFAULT 10,
    rate_limit_burst INTEGER DEFAULT 20,
    limit_conn INTEGER DEFAULT 100,
    auto_ban_enabled BOOLEAN DEFAULT 0,
    auto_ban_threshold INTEGER DEFAULT 100,
    auto_ban_404_threshold INTEGER DEFAULT 50,
    auto_ban_duration INTEGER DEFAULT 3600,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (website_id) REFERENCES websites(id)
);

CREATE TABLE IF NOT EXISTS website_banned_ip (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    website_id INTEGER,
    ip TEXT NOT NULL,
    reason TEXT,
    source TEXT DEFAULT 'auto',
    expires_at DATETIME,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (website_id) REFERENCES websites(id)
);

CREATE INDEX IF NOT EXISTS idx_website_banned_ip_ip ON website_banned_ip(ip);
CREATE INDEX IF NOT EXISTS idx_website_banned_ip_website ON website_banned_ip(website_id);

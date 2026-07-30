CREATE TABLE IF NOT EXISTS sessions (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	token TEXT NOT NULL UNIQUE,
	user_id INTEGER NOT NULL,
	username TEXT NOT NULL,
	role TEXT NOT NULL DEFAULT 'admin',
	ip TEXT DEFAULT '',
	user_agent TEXT DEFAULT '',
	client_type TEXT DEFAULT 'web',
	device_id TEXT DEFAULT '',
	device_info TEXT DEFAULT '',
	last_active DATETIME DEFAULT CURRENT_TIMESTAMP,
	expires_at DATETIME,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_sessions_user ON sessions(user_id);
CREATE INDEX IF NOT EXISTS idx_sessions_token ON sessions(token);
CREATE INDEX IF NOT EXISTS idx_sessions_last_active ON sessions(last_active);

CREATE TABLE IF NOT EXISTS user_activities (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	user_id INTEGER NOT NULL,
	username TEXT NOT NULL,
	action TEXT NOT NULL,
	ip TEXT DEFAULT '',
	user_agent TEXT DEFAULT '',
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_user_activities_user ON user_activities(user_id);
CREATE INDEX IF NOT EXISTS idx_user_actions_action ON user_activities(action);

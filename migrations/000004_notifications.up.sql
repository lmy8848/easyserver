-- 通知表
CREATE TABLE IF NOT EXISTS notifications (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  type TEXT NOT NULL,           -- alert/security/deploy/cron/update/system
  title TEXT NOT NULL,
  message TEXT NOT NULL,
  level TEXT DEFAULT 'info',    -- info/warning/error
  is_read INTEGER DEFAULT 0,
  metadata TEXT,                -- JSON: 关联资源ID等
  created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_notifications_type ON notifications(type);
CREATE INDEX IF NOT EXISTS idx_notifications_read ON notifications(is_read);
CREATE INDEX IF NOT EXISTS idx_notifications_created ON notifications(created_at DESC);

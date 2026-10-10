-- Rollback: 重新加回 users 表的两个死列（SQLite 仅支持 ADD COLUMN）。

ALTER TABLE users ADD COLUMN expires_at DATETIME;
ALTER TABLE users ADD COLUMN ip_whitelist TEXT DEFAULT '';

-- Sessions are now managed in-memory; drop the persistent table.
DROP INDEX IF EXISTS idx_sessions_user;
DROP INDEX IF EXISTS idx_sessions_token;
DROP INDEX IF EXISTS idx_sessions_last_active;
DROP TABLE IF EXISTS sessions;

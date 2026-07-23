-- File Integrity Monitoring (FIM): baseline hashes + detected changes.
-- CREATE TABLE IF NOT EXISTS is idempotent, no migration hook needed.
CREATE TABLE IF NOT EXISTS fim_baseline (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  path TEXT NOT NULL UNIQUE,
  hash TEXT NOT NULL,
  size INTEGER NOT NULL,
  mtime TEXT NOT NULL,
  updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS fim_changes (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  path TEXT NOT NULL,
  change_type TEXT NOT NULL,   -- modified / added / deleted
  old_hash TEXT,
  new_hash TEXT,
  detected_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_fim_changes_path ON fim_changes(path);
CREATE INDEX IF NOT EXISTS idx_fim_changes_detected ON fim_changes(detected_at);

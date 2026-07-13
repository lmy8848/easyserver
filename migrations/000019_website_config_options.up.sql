-- file_shares table for external file sharing.
-- The websites.process_id / config_options columns are added idempotently by the
-- version-19 pre-migration hook in migrate.go (ALTER TABLE ADD COLUMN is not
-- idempotent in SQLite, so we check pragma_table_info first).
CREATE TABLE IF NOT EXISTS file_shares (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    file_path TEXT NOT NULL,
    file_name TEXT NOT NULL,
    file_size INTEGER DEFAULT 0,
    token TEXT NOT NULL UNIQUE,
    password TEXT DEFAULT '',
    expires_at DATETIME,
    max_downloads INTEGER DEFAULT 0,
    download_count INTEGER DEFAULT 0,
    created_by INTEGER NOT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_file_shares_token ON file_shares(token);
CREATE INDEX IF NOT EXISTS idx_file_shares_created_by ON file_shares(created_by);

-- Add process_id and config_options columns to websites
ALTER TABLE websites ADD COLUMN process_id INTEGER DEFAULT 0;
ALTER TABLE websites ADD COLUMN config_options TEXT DEFAULT '';

-- Create file_shares table for external file sharing
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

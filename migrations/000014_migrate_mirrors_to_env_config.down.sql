-- Recreate runtime_mirror table (best-effort: cannot recover original rows
-- after DROP, but restores the schema so older binaries don't crash on boot).
CREATE TABLE IF NOT EXISTS runtime_mirror (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    lang TEXT NOT NULL CHECK(lang IN ('node', 'python', 'go', 'java', 'php')),
    env_key TEXT NOT NULL,
    env_value TEXT NOT NULL,
    enabled INTEGER DEFAULT 1,
    source TEXT NOT NULL CHECK(source IN ('seed', 'user')),
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Remove mirror env vars that were migrated into env_configs, so a
-- downgrade + re-upgrade doesn't double-insert. Names match the catalog
-- mirror env keys.
DELETE FROM env_configs
WHERE name IN ('MISE_NODE_MIRROR_URL', 'MISE_GO_DOWNLOAD_MIRROR', 'PYTHON_BUILD_MIRROR_URL');

CREATE TABLE IF NOT EXISTS global_default (
    lang TEXT PRIMARY KEY CHECK(lang IN ('node', 'python', 'go', 'java', 'php')),
    runtime_version_id INTEGER NOT NULL,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (runtime_version_id) REFERENCES runtime_version(id)
);
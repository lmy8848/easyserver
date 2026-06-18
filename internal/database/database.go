package database

import (
	"database/sql"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

func Init(dbPath string) (*sql.DB, error) {
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}

	db, err := sql.Open("sqlite", dbPath+"?_journal_mode=WAL&_busy_timeout=5000&_txlock=immediate")
	if err != nil {
		return nil, err
	}

	// Set connection pool settings for better concurrency
	db.SetMaxOpenConns(1) // SQLite only supports one writer at a time
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(time.Hour)

	if err := db.Ping(); err != nil {
		return nil, err
	}

	if err := createTables(db); err != nil {
		return nil, err
	}

	return db, nil
}

func createTables(db *sql.DB) error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS users (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			username TEXT UNIQUE NOT NULL,
			password_hash TEXT NOT NULL,
			role TEXT NOT NULL DEFAULT 'viewer',
			must_change_pass INTEGER DEFAULT 0,
			last_login DATETIME,
			last_login_ip TEXT DEFAULT '',
			login_attempts INTEGER DEFAULT 0,
			locked_until DATETIME,
			expires_at DATETIME,
			ip_whitelist TEXT DEFAULT '',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS user_activities (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL,
			username TEXT NOT NULL,
			action TEXT NOT NULL,
			ip TEXT DEFAULT '',
			user_agent TEXT DEFAULT '',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_user_activities_user ON user_activities(user_id)`,
		`CREATE INDEX IF NOT EXISTS idx_user_activities_created ON user_activities(created_at)`,
		`CREATE TABLE IF NOT EXISTS sessions (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			token TEXT UNIQUE NOT NULL,
			user_id INTEGER NOT NULL,
			username TEXT NOT NULL,
			role TEXT NOT NULL,
			ip TEXT DEFAULT '',
			user_agent TEXT DEFAULT '',
			last_active DATETIME DEFAULT CURRENT_TIMESTAMP,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			expires_at DATETIME NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_sessions_user ON sessions(user_id)`,
		`CREATE INDEX IF NOT EXISTS idx_sessions_token ON sessions(token)`,
		`CREATE INDEX IF NOT EXISTS idx_sessions_last_active ON sessions(last_active)`,
		`CREATE TABLE IF NOT EXISTS monitor_data (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			cpu REAL,
			cpu_load_1m REAL DEFAULT 0,
			cpu_load_5m REAL DEFAULT 0,
			cpu_load_15m REAL DEFAULT 0,
			mem_total INTEGER,
			mem_used INTEGER,
			mem_available INTEGER,
			mem_usage REAL,
			disk_total INTEGER,
			disk_used INTEGER,
			disk_free INTEGER,
			disk_usage REAL,
			net_bytes_sent INTEGER,
			net_bytes_recv INTEGER,
			net_packets_sent INTEGER,
			net_packets_recv INTEGER,
			timestamp DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS audit_logs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER,
			username TEXT,
			action TEXT NOT NULL,
			resource TEXT,
			detail TEXT,
			ip TEXT,
			user_agent TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS deploy_servers (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			host TEXT NOT NULL,
			port INTEGER DEFAULT 22,
			username TEXT NOT NULL,
			auth_type TEXT CHECK(auth_type IN ('password', 'key')),
			auth_data TEXT,
			status TEXT DEFAULT 'unknown',
			last_ping TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS deploy_tasks (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			server_id INTEGER REFERENCES deploy_servers(id),
			name TEXT NOT NULL,
			type TEXT CHECK(type IN ('sync', 'command', 'rollback')),
			source_path TEXT,
			dest_path TEXT,
			command TEXT,
			status TEXT DEFAULT 'pending',
			result TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS deploy_versions (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			server_id INTEGER REFERENCES deploy_servers(id),
			task_id INTEGER REFERENCES deploy_tasks(id),
			version TEXT NOT NULL,
			files TEXT,
			backup_path TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_monitor_timestamp ON monitor_data(timestamp)`,
		`CREATE INDEX IF NOT EXISTS idx_audit_created_at ON audit_logs(created_at)`,
		`CREATE INDEX IF NOT EXISTS idx_deploy_tasks_server ON deploy_tasks(server_id)`,
		`CREATE INDEX IF NOT EXISTS idx_deploy_versions_server ON deploy_versions(server_id)`,
		`CREATE TABLE IF NOT EXISTS token_blacklist (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL,
			token TEXT NOT NULL,
			expires_at DATETIME NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_token_blacklist_user ON token_blacklist(user_id)`,
		`CREATE INDEX IF NOT EXISTS idx_token_blacklist_expires ON token_blacklist(expires_at)`,
		`CREATE TABLE IF NOT EXISTS runtime_environments (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			version TEXT NOT NULL,
			path TEXT NOT NULL,
			is_default INTEGER DEFAULT 0,
			status TEXT DEFAULT 'installed',
			installed_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(name, version)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_runtime_name ON runtime_environments(name)`,
	}

	for _, q := range queries {
		if _, err := db.Exec(q); err != nil {
			return err
		}
	}

	return nil
}

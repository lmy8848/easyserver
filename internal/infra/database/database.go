package database

import (
	"database/sql"
	"fmt"
	"log"
	"net/url"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

func Init(dbPath string) (*sql.DB, error) {
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}

	// Build DSN using url.Parse for safe parameter encoding
	dsn := &url.URL{
		Path: dbPath,
	}
	params := dsn.Query()
	params.Set("_journal_mode", "WAL")
	params.Set("_busy_timeout", "5000")
	params.Set("_txlock", "immediate")
	dsn.RawQuery = params.Encode()

	db, err := sql.Open("sqlite", dsn.String())
	if err != nil {
		return nil, err
	}

	// Set connection pool settings for better concurrency
	db.SetMaxOpenConns(1) // SQLite only supports one writer at a time
	db.SetMaxIdleConns(1)

	if err := db.Ping(); err != nil {
		return nil, err
	}

	// Enable foreign key enforcement (SQLite has it OFF by default)
	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		return nil, fmt.Errorf("failed to enable foreign keys: %w", err)
	}

	// Try migration-based initialization first
	// Resolve migrations directory relative to the executable
	exePath, err := os.Executable()
	migrationsDir := "migrations"
	if err == nil {
		migrationsDir = filepath.Join(filepath.Dir(exePath), "migrations")
	}
	if err := Migrate(db, migrationsDir); err != nil {
		// Fallback to legacy initialization if migrations directory not found
		log.Printf("migrate: falling back to legacy init: %v", err)
		if err := createTables(db); err != nil {
			return nil, err
		}
		if err := migrateDatabase(db); err != nil {
			return nil, fmt.Errorf("database migration failed: %w", err)
		}
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
			client_type TEXT NOT NULL DEFAULT 'web',
			device_id TEXT NOT NULL DEFAULT '',
			device_info TEXT NOT NULL DEFAULT '',
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
			type TEXT NOT NULL DEFAULT 'operation',
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

// migrateDatabase handles schema migrations for existing databases
func migrateDatabase(db *sql.DB) error {
	migrations := []struct {
		column string
		query  string
		table  string
	}{
		{"totp_secret", "ALTER TABLE users ADD COLUMN totp_secret TEXT DEFAULT ''", "users"},
		{"totp_enabled", "ALTER TABLE users ADD COLUMN totp_enabled INTEGER DEFAULT 0", "users"},
		{"totp_backup_codes", "ALTER TABLE users ADD COLUMN totp_backup_codes TEXT DEFAULT '[]'", "users"},
	}

	for _, m := range migrations {
		// Check if column already exists
		var exists bool
		err := db.QueryRow(`
			SELECT COUNT(*) > 0 FROM pragma_table_info(?) WHERE name = ?
		`, m.table, m.column).Scan(&exists)
		if err != nil {
			return fmt.Errorf("check column %s: %w", m.column, err)
		}

		if !exists {
			if _, err := db.Exec(m.query); err != nil {
				return fmt.Errorf("add column %s: %w", m.column, err)
			}
		}
	}

	return nil
}

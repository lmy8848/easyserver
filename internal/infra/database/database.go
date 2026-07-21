package database

import (
	"database/sql"
	"fmt"
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
		return nil, fmt.Errorf("migrate: %w", err)
	}

	return db, nil
}

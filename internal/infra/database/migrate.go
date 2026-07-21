package database

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Migrate runs all pending database migrations
func Migrate(db *sql.DB, migrationsDir string) error {
	// Create migrations tracking table
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version INTEGER PRIMARY KEY,
			name TEXT NOT NULL,
			applied_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)
	`); err != nil {
		return fmt.Errorf("create migrations table: %w", err)
	}

	// Get applied migrations
	applied, err := getAppliedMigrations(db)
	if err != nil {
		return fmt.Errorf("get applied migrations: %w", err)
	}

	// Read migration files
	files, err := os.ReadDir(migrationsDir)
	if err != nil {
		if os.IsNotExist(err) {
			log.Printf("migrate: no migrations directory found, skipping")
			return nil
		}
		return fmt.Errorf("read migrations dir: %w", err)
	}

	// Filter and sort .up.sql files
	var upFiles []string
	for _, f := range files {
		if strings.HasSuffix(f.Name(), ".up.sql") {
			upFiles = append(upFiles, f.Name())
		}
	}
	sort.Strings(upFiles)

	isFreshInstall := len(applied) == 0

	// Run pending migrations
	for _, name := range upFiles {
		version := extractVersion(name)
		if applied[version] {
			continue
		}

		var hook func(*sql.Tx) error
		if version == 6 && !isFreshInstall {
			hook = func(tx *sql.Tx) error {
				return performHardCutoverBackup(tx, db, migrationsDir)
			}
		}
		// Version 19 (website_config_options): add websites.process_id /
		// config_options idempotently. ALTER TABLE ADD COLUMN is not idempotent
		// in SQLite, so existing deployments that already have the columns (from
		// the legacy createTables fallback) must not re-run the ALTER.
		if version == 19 {
			hook = ensureWebsitesColumns
		}
		// Version 20 (sessions_client_device): add sessions.client_type /
		// device_id / device_info idempotently for mobile single-device binding.
		if version == 20 {
			hook = ensureSessionsColumns
		}

		log.Printf("migrate: running migration %s", name)
		if err := runMigration(db, filepath.Join(migrationsDir, name), version, name, hook); err != nil {
			return fmt.Errorf("run migration %s: %w", name, err)
		}
		log.Printf("migrate: applied %s", name)
	}

	return nil
}

// getAppliedMigrations returns a map of applied migration versions
func getAppliedMigrations(db *sql.DB) (map[int]bool, error) {
	rows, err := db.Query("SELECT version FROM schema_migrations")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	applied := make(map[int]bool)
	for rows.Next() {
		var version int
		if err := rows.Scan(&version); err != nil {
			return nil, err
		}
		applied[version] = true
	}
	return applied, nil
}

// extractVersion extracts the version number from migration filename
// e.g., "000001_init_schema.up.sql" -> 1
func extractVersion(name string) int {
	var version int
	fmt.Sscanf(name, "%d", &version)
	return version
}

// stripLeadingComments removes leading comment lines and blank lines from a SQL statement
func stripLeadingComments(stmt string) string {
	lines := strings.Split(stmt, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "--") {
			continue
		}
		return trimmed
	}
	return ""
}

// runMigration executes a single migration file
func runMigration(db *sql.DB, path string, version int, name string, preTxHook func(*sql.Tx) error) error {
	content, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	// Split by semicolons and execute each statement
	statements := splitStatements(string(content))

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if preTxHook != nil {
		if err := preTxHook(tx); err != nil {
			return err
		}
	}

	for _, stmt := range statements {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" {
			continue
		}
		// Check if statement is only comments (no actual SQL)
		cleaned := stripLeadingComments(stmt)
		if cleaned == "" {
			continue
		}
		if _, err := tx.Exec(stmt); err != nil {
			return fmt.Errorf("exec statement: %w\nStatement: %s", err, stmt[:min(100, len(stmt))])
		}
	}

	// Record migration
	if _, err := tx.Exec("INSERT INTO schema_migrations (version, name) VALUES (?, ?)", version, name); err != nil {
		return err
	}

	return tx.Commit()
}

// ensureWebsitesColumns idempotently adds websites.process_id and
// websites.config_options. Used as the version-19 pre-migration hook so that
// deployments which already have the columns (legacy createTables fallback) and
// fresh installs both converge safely.
func ensureWebsitesColumns(tx *sql.Tx) error {
	cols := []struct{ name, def string }{
		{"process_id", "INTEGER DEFAULT 0"},
		{"config_options", "TEXT DEFAULT ''"},
	}
	for _, c := range cols {
		var cnt int
		if err := tx.QueryRow(
			"SELECT COUNT(*) FROM pragma_table_info('websites') WHERE name = ?", c.name,
		).Scan(&cnt); err != nil {
			return fmt.Errorf("check column %s: %w", c.name, err)
		}
		if cnt > 0 {
			continue // column already exists, skip ALTER
		}
		if _, err := tx.Exec(
			fmt.Sprintf("ALTER TABLE websites ADD COLUMN %s %s", c.name, c.def),
		); err != nil {
			return fmt.Errorf("add column %s: %w", c.name, err)
		}
	}
	return nil
}

// ensureSessionsColumns idempotently adds sessions.client_type / device_id /
// device_info. Used as the version-20 pre-migration hook so that deployments
// which already have the columns (legacy createTables fallback) and fresh
// installs both converge safely.
func ensureSessionsColumns(tx *sql.Tx) error {
	cols := []struct{ name, def string }{
		{"client_type", "TEXT NOT NULL DEFAULT 'web'"},
		{"device_id", "TEXT NOT NULL DEFAULT ''"},
		{"device_info", "TEXT NOT NULL DEFAULT ''"},
	}
	for _, c := range cols {
		var cnt int
		if err := tx.QueryRow(
			"SELECT COUNT(*) FROM pragma_table_info('sessions') WHERE name = ?", c.name,
		).Scan(&cnt); err != nil {
			return fmt.Errorf("check column %s: %w", c.name, err)
		}
		if cnt > 0 {
			continue // column already exists, skip ALTER
		}
		if _, err := tx.Exec(
			fmt.Sprintf("ALTER TABLE sessions ADD COLUMN %s %s", c.name, c.def),
		); err != nil {
			return fmt.Errorf("add column %s: %w", c.name, err)
		}
	}
	return nil
}

// splitStatements splits SQL content by semicolons, respecting quoted strings
// and -- line comments (semicolons inside comments or quotes do not split).
func splitStatements(content string) []string {
	var statements []string
	var current strings.Builder
	inQuote := false
	quoteChar := byte(0)
	inComment := false // -- line comment

	for i := 0; i < len(content); i++ {
		ch := content[i]

		// Detect start of -- line comment (outside quotes)
		if !inQuote && !inComment && ch == '-' && i+1 < len(content) && content[i+1] == '-' {
			inComment = true
		}

		if inComment {
			current.WriteByte(ch)
			if ch == '\n' {
				inComment = false
			}
			continue
		}

		if !inQuote && (ch == '\'' || ch == '"') {
			inQuote = true
			quoteChar = ch
		} else if inQuote && ch == quoteChar {
			inQuote = false
		}

		if ch == ';' && !inQuote {
			stmt := current.String()
			if strings.TrimSpace(stmt) != "" {
				statements = append(statements, stmt)
			}
			current.Reset()
		} else {
			current.WriteByte(ch)
		}
	}

	// Last statement without semicolon
	if stmt := current.String(); strings.TrimSpace(stmt) != "" {
		statements = append(statements, stmt)
	}

	return statements
}

package database

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"log"
	"sort"
	"strings"
)

//go:embed all:migrations
var migrationsFS embed.FS

// Migrate runs all pending migrations from the schema embedded in the binary.
// The schema must travel with the binary: official install/upgrade flows ship
// only the executable, and an exe-adjacent migrations dir silently degrades to
// "no migrations found" on a fresh install.
func Migrate(db *sql.DB) error {
	sub, err := fs.Sub(migrationsFS, "migrations")
	if err != nil {
		return fmt.Errorf("resolve embedded migrations: %w", err)
	}
	return migrateFS(db, sub)
}

// migrateFS applies every *.up.sql in fsys (sorted by name) that is not yet
// recorded in schema_migrations. Each file runs in a transaction together with
// its version record, so a failed migration leaves no partial state.
func migrateFS(db *sql.DB, fsys fs.FS) error {
	ctx := context.Background()

	// Create migrations tracking table
	if _, err := db.ExecContext(ctx, `
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

	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return fmt.Errorf("read migrations: %w", err)
	}

	var names []string
	maxFileVersion := 0
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".up.sql") {
			continue
		}
		names = append(names, name)
		if v := extractVersion(name); v > maxFileVersion {
			maxFileVersion = v
		}
	}
	sort.Strings(names)

	// 历史安装（迁移链被压缩重排前）的 schema_migrations 里可能记录着高于
	// 当前嵌入链的版本号；该形态下同名新迁移会因版本号碰撞被静默跳过。
	// 出现即大声提示运维核对 schema，绝不静默继续。
	maxApplied := 0
	for v := range applied {
		if v > maxApplied {
			maxApplied = v
		}
	}
	if maxApplied > maxFileVersion {
		log.Printf("migrate: WARNING: schema_migrations max version %d exceeds highest embedded migration %d — "+
			"legacy pre-squash install, newer migrations numbered <= %d were skipped. Verify schema consistency manually.",
			maxApplied, maxFileVersion, maxFileVersion)
	}

	// Run pending migrations
	for _, name := range names {
		version := extractVersion(name)
		if applied[version] {
			continue
		}

		log.Printf("migrate: running migration %s", name)
		content, err := fs.ReadFile(fsys, name)
		if err != nil {
			return fmt.Errorf("read migration %s: %w", name, err)
		}
		if err := runMigration(db, string(content), version, name); err != nil {
			return fmt.Errorf("run migration %s: %w", name, err)
		}
		log.Printf("migrate: applied %s", name)
	}

	return nil
}

// getAppliedMigrations returns a map of applied migration versions
func getAppliedMigrations(db *sql.DB) (map[int]bool, error) {
	ctx := context.Background()
	rows, err := db.QueryContext(ctx, "SELECT version FROM schema_migrations")
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
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return applied, nil
}

// extractVersion extracts the version number from migration filename
// e.g., "000001_init_schema.up.sql" -> 1
func extractVersion(name string) int {
	var version int
	// 解析失败时 version 保持 0，符合"无版本号即 0"的兜底语义
	_, _ = fmt.Sscanf(name, "%d", &version)
	return version
}

// stripLeadingComments removes leading comment lines and blank lines from a SQL statement
func stripLeadingComments(stmt string) string {
	lines := strings.SplitSeq(stmt, "\n")
	for line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "--") {
			continue
		}
		return trimmed
	}
	return ""
}

// runMigration executes a single migration's SQL statements and records its version.
func runMigration(db *sql.DB, content string, version int, name string) error {
	ctx := context.Background()

	// Split by semicolons and execute each statement
	statements := splitStatements(content)

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

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
		if _, err := tx.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("exec statement: %w\nStatement: %s", err, stmt[:min(100, len(stmt))])
		}
	}

	// Record migration
	if _, err := tx.ExecContext(ctx, "INSERT INTO schema_migrations (version, name) VALUES (?, ?)", version, name); err != nil {
		return err
	}

	return tx.Commit()
}

// splitStatements splits SQL content by semicolons, respecting quoted strings
// and -- line comments (semicolons inside comments or quotes do not split).
func splitStatements(content string) []string {
	var statements []string
	var current strings.Builder
	inQuote := false
	quoteChar := byte(0)
	inComment := false // -- line comment

	for i := range len(content) {
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

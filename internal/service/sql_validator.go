package service

import (
	"fmt"
	"regexp"
	"strings"
)

// ValidationResult represents a validation result
type ValidationResult struct {
	Valid   bool   `json:"valid"`
	Message string `json:"message"`
	SQL     string `json:"sql,omitempty"`
}

// SQLValidator validates SQL statements without executing them
type SQLValidator struct {
	builder *SQLBuilder
}

func NewSQLValidator(dbType DBType) *SQLValidator {
	return &SQLValidator{builder: NewSQLBuilder(dbType)}
}

// ValidateIdentifier validates a table or column name
func (v *SQLValidator) ValidateIdentifier(name string) *ValidationResult {
	if len(name) == 0 {
		return &ValidationResult{Valid: false, Message: "identifier cannot be empty"}
	}
	if len(name) > 64 {
		return &ValidationResult{Valid: false, Message: "identifier too long (max 64 chars)"}
	}
	for _, c := range name {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_' || c == '-') {
			return &ValidationResult{Valid: false, Message: fmt.Sprintf("invalid character '%c' in identifier", c)}
		}
	}
	return &ValidationResult{Valid: true}
}

// ValidateDatabaseName validates a database name
func (v *SQLValidator) ValidateDatabaseName(name string) *ValidationResult {
	return v.ValidateIdentifier(name)
}

// ValidateTableName validates a table name
func (v *SQLValidator) ValidateTableName(name string) *ValidationResult {
	return v.ValidateIdentifier(name)
}

// ValidateUsername validates a username
func (v *SQLValidator) ValidateUsername(name string) *ValidationResult {
	if len(name) == 0 {
		return &ValidationResult{Valid: false, Message: "username cannot be empty"}
	}
	if len(name) > 32 {
		return &ValidationResult{Valid: false, Message: "username too long (max 32 chars)"}
	}
	return v.ValidateIdentifier(name)
}

// ValidateHost validates a host
func (v *SQLValidator) ValidateHost(host string) *ValidationResult {
	if host == "" {
		return &ValidationResult{Valid: false, Message: "host cannot be empty"}
	}
	if host == "%" || host == "localhost" {
		return &ValidationResult{Valid: true}
	}
	// Allow IP addresses and hostnames
	for _, c := range host {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '.' || c == '-' || c == ':') {
			return &ValidationResult{Valid: false, Message: fmt.Sprintf("invalid character '%c' in host", c)}
		}
	}
	return &ValidationResult{Valid: true}
}

// ValidateCharset validates a charset
func (v *SQLValidator) ValidateCharset(charset string) *ValidationResult {
	validCharsets := map[string]bool{
		"utf8mb4": true, "utf8": true, "latin1": true,
		"ascii": true, "gbk": true, "big5": true,
	}
	if charset == "" {
		return &ValidationResult{Valid: true, Message: "will use default charset"}
	}
	if !validCharsets[charset] {
		return &ValidationResult{Valid: false, Message: fmt.Sprintf("unsupported charset: %s", charset)}
	}
	return &ValidationResult{Valid: true}
}

// ValidatePrivilege validates a privilege string
func (v *SQLValidator) ValidatePrivilege(priv string) *ValidationResult {
	validPrivileges := map[string]bool{
		"ALL PRIVILEGES": true, "SELECT": true, "INSERT": true,
		"UPDATE": true, "DELETE": true, "CREATE": true, "DROP": true,
		"INDEX": true, "ALTER": true, "EXECUTE": true,
	}
	priv = strings.TrimSpace(priv)
	if !validPrivileges[priv] {
		return &ValidationResult{Valid: false, Message: fmt.Sprintf("invalid privilege: %s", priv)}
	}
	return &ValidationResult{Valid: true}
}

// ValidateInsert validates an INSERT operation
func (v *SQLValidator) ValidateInsert(table string, data map[string]interface{}, tableInfo *TableInfo) *ValidationResult {
	if r := v.ValidateTableName(table); !r.Valid {
		return r
	}
	if len(data) == 0 {
		return &ValidationResult{Valid: false, Message: "no data to insert"}
	}

	// Validate column names
	for col := range data {
		if r := v.ValidateIdentifier(col); !r.Valid {
			return &ValidationResult{Valid: false, Message: fmt.Sprintf("invalid column '%s': %s", col, r.Message)}
		}
	}

	// Generate preview SQL
	sql := v.builder.BuildInsert(table, data, tableInfo)
	return &ValidationResult{Valid: true, Message: "valid", SQL: sql}
}

// ValidateUpdate validates an UPDATE operation
func (v *SQLValidator) ValidateUpdate(table string, data map[string]interface{}, pkCol string, pkVal interface{}) *ValidationResult {
	if r := v.ValidateTableName(table); !r.Valid {
		return r
	}
	if r := v.ValidateIdentifier(pkCol); !r.Valid {
		return r
	}
	if len(data) == 0 {
		return &ValidationResult{Valid: false, Message: "no data to update"}
	}
	if pkVal == nil {
		return &ValidationResult{Valid: false, Message: "primary key value is required"}
	}

	sql := v.builder.BuildUpdate(table, data, pkCol, pkVal)
	return &ValidationResult{Valid: true, Message: "valid", SQL: sql}
}

// ValidateDelete validates a DELETE operation
func (v *SQLValidator) ValidateDelete(table string, pkCol string, pkVal interface{}) *ValidationResult {
	if r := v.ValidateTableName(table); !r.Valid {
		return r
	}
	if r := v.ValidateIdentifier(pkCol); !r.Valid {
		return r
	}
	if pkVal == nil {
		return &ValidationResult{Valid: false, Message: "primary key value is required"}
	}

	sql := v.builder.BuildDelete(table, pkCol, pkVal)
	return &ValidationResult{Valid: true, Message: "valid", SQL: sql}
}

// stripLeadingComments removes leading SQL comments and whitespace from a statement.
// Handles both single-line (-- ...) and block (/* ... */) comments.
func stripLeadingComments(sql string) string {
	for {
		sql = strings.TrimSpace(sql)
		if strings.HasPrefix(sql, "--") {
			// Single-line comment: skip to end of line
			if idx := strings.Index(sql, "\n"); idx >= 0 {
				sql = sql[idx+1:]
			} else {
				// Entire string is a comment
				return ""
			}
		} else if strings.HasPrefix(sql, "/*") {
			// Block comment: skip to closing */
			if idx := strings.Index(sql, "*/"); idx >= 0 {
				sql = sql[idx+2:]
			} else {
				// Unclosed block comment
				return ""
			}
		} else {
			break
		}
	}
	return sql
}

// getFirstKeyword extracts the first 1-3 significant keywords from SQL,
// skipping comments and whitespace. Returns uppercased keyword(s).
func getFirstKeyword(sql string) string {
	cleaned := stripLeadingComments(sql)
	if cleaned == "" {
		return ""
	}
	upper := strings.ToUpper(cleaned)

	// Take up to first 3 words (enough for "DROP TABLE IF")
	words := strings.Fields(upper)
	if len(words) == 0 {
		return ""
	}
	if len(words) > 3 {
		words = words[:3]
	}
	return strings.Join(words, " ")
}

// validateSingleStatement validates a single SQL statement against the blocklist.
func validateSingleStatement(sql string) *ValidationResult {
	upper := strings.ToUpper(sql)

	// --- Prefix blocklist (dangerous at start of statement) ---
	prefixBlocked := []string{
		"DROP DATABASE", "DROP SCHEMA",
		"DROP TABLE",
		"TRUNCATE",
		"GRANT",
		"REVOKE",
		"ALTER USER", "ALTER SYSTEM",
		"CREATE USER", "DROP USER",
		"LOAD DATA", "LOAD FILE", "INTO OUTFILE", "INTO DUMPFILE",
	}
	for _, b := range prefixBlocked {
		if strings.HasPrefix(upper, b) {
			return &ValidationResult{Valid: false, Message: fmt.Sprintf("%s is not allowed", b)}
		}
	}

	// --- Pattern-based checks: dangerous SQL without WHERE clause ---
	// DELETE without WHERE
	if strings.HasPrefix(upper, "DELETE") {
		if !strings.Contains(upper, " WHERE ") {
			return &ValidationResult{Valid: false, Message: "DELETE without WHERE clause is not allowed"}
		}
	}

	// UPDATE without WHERE
	if strings.HasPrefix(upper, "UPDATE") {
		if !strings.Contains(upper, " WHERE ") {
			return &ValidationResult{Valid: false, Message: "UPDATE without WHERE clause is not allowed"}
		}
	}

	// Block SET PASSWORD or ALTER ... PASSWORD
	if strings.Contains(upper, "SET PASSWORD") || strings.Contains(upper, "IDENTIFIED BY") {
		return &ValidationResult{Valid: false, Message: "password modification is not allowed"}
	}

	// Block SLEEP() and BENCHMARK() DoS functions
	if strings.Contains(upper, "SLEEP(") || strings.Contains(upper, "BENCHMARK(") {
		return &ValidationResult{Valid: false, Message: "SLEEP/BENCHMARK functions are not allowed"}
	}

	// Block INTO OUTFILE / INTO DUMPFILE (file write)
	if strings.Contains(upper, "INTO OUTFILE") || strings.Contains(upper, "INTO DUMPFILE") {
		return &ValidationResult{Valid: false, Message: "writing to filesystem is not allowed"}
	}

	return nil // no violation
}

// ValidateSQL validates a raw SQL statement for safety.
// It strips leading comments, splits by semicolons, and checks each statement
// against a blocklist of dangerous operations.
func (v *SQLValidator) ValidateSQL(sql string) *ValidationResult {
	sql = strings.TrimSpace(sql)
	if sql == "" {
		return &ValidationResult{Valid: false, Message: "SQL cannot be empty"}
	}

	// Split by semicolons and validate each statement individually
	statements := strings.Split(sql, ";")
	for _, stmt := range statements {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" {
			continue
		}

		// Strip leading comments to get the real statement content
		cleanedStmt := stripLeadingComments(stmt)
		if cleanedStmt == "" {
			continue // statement was only comments
		}

		// Validate the statement with comments stripped for pattern matching
		if result := validateSingleStatement(cleanedStmt); result != nil {
			return result
		}
	}

	// Ensure trailing semicolon
	if !strings.HasSuffix(sql, ";") {
		sql += ";"
	}

	return &ValidationResult{Valid: true, Message: "valid", SQL: sql}
}

// ParseTableInfo parses DESCRIBE output into TableInfo
func ParseTableInfo(dbType DBType, tableName string, describeOutput string) *TableInfo {
	info := &TableInfo{Name: tableName, Columns: []ColumnInfo{}}
	lines := strings.Split(strings.TrimSpace(describeOutput), "\n")

	switch dbType {
	case DBTypeMySQL:
		for i, line := range lines {
			if i == 0 { // header
				continue
			}
			fields := strings.Split(line, "\t")
			if len(fields) >= 2 {
				col := ColumnInfo{
					Name:       fields[0],
					Type:       fields[1],
					IsNullable: len(fields) < 3 || fields[2] == "YES",
				}
				if len(fields) >= 4 {
					col.IsPrimaryKey = fields[3] == "PRI"
					if col.IsPrimaryKey {
						info.PrimaryKey = col.Name
					}
				}
				if len(fields) >= 6 {
					col.IsAutoIncr = strings.Contains(fields[5], "auto_increment")
				}
				if len(fields) >= 5 && fields[4] != "NULL" && fields[4] != "" {
					col.HasDefault = true
					col.DefaultValue = fields[4]
				}
				info.Columns = append(info.Columns, col)
			}
		}
	case DBTypePostgreSQL:
		for i, line := range lines {
			if i < 2 { // header + separator
				continue
			}
			fields := strings.Split(line, "|")
			if len(fields) >= 2 {
				col := ColumnInfo{
					Name:       strings.TrimSpace(fields[0]),
					Type:       strings.TrimSpace(fields[1]),
					IsNullable: len(fields) < 3 || strings.TrimSpace(fields[2]) == "YES",
				}
				if len(fields) >= 4 && strings.TrimSpace(fields[3]) != "" {
					col.HasDefault = true
					col.DefaultValue = strings.TrimSpace(fields[3])
				}
				if len(fields) >= 5 {
					col.IsPrimaryKey = strings.TrimSpace(fields[4]) == "YES"
					if col.IsPrimaryKey {
						info.PrimaryKey = col.Name
					}
				}
				// Detect SERIAL (auto-increment in PostgreSQL)
				if strings.Contains(col.Type, "serial") {
					col.IsAutoIncr = true
				}
				info.Columns = append(info.Columns, col)
			}
		}
	}

	return info
}

// regexp for detecting auto-increment in MySQL DESCRIBE output
var autoIncrRegexp = regexp.MustCompile(`(?i)auto_increment`)

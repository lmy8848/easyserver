package database

import (
	"fmt"
	"regexp"
	"strings"
)

// --- 名称/字符集校验 ---

const (
	maxDBNameLen   = 64
	maxUsernameLen = 32
	maxHostLen     = 255
	defaultCharset = "utf8mb4"
)

var validCharsets = map[string]bool{
	"utf8mb4": true, "utf8": true, "latin1": true,
	"ascii": true, "gbk": true, "big5": true,
}

// --- 名称/标识符校验 ---
//
// 这些校验由 SQLBuilder 的 Build* 方法内嵌调用（构建即校验），也供路由层
// 的路径参数校验复用（isValidDBName）。

var tableNameRegexp = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)

// isValidTableName checks table/column name validity.
func isValidTableName(name string) bool {
	return name != "" && len(name) <= 64 && tableNameRegexp.MatchString(name)
}

// isValidColumnName checks a column name for use in DML statements. Columns
// may carry a hyphen, unlike table names.
func isValidColumnName(name string) error {
	if len(name) == 0 {
		return fmt.Errorf("identifier cannot be empty")
	}
	if len(name) > 64 {
		return fmt.Errorf("identifier too long (max 64 chars)")
	}
	for _, c := range name {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_' || c == '-') {
			return fmt.Errorf("invalid character '%c' in identifier", c)
		}
	}
	return nil
}

func isValidDBName(name string) bool {
	if len(name) == 0 || len(name) > maxDBNameLen {
		return false
	}
	for _, c := range name {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_' || c == '-' || c == '.') {
			return false
		}
	}
	return true
}

func isValidUsername(name string) bool {
	if len(name) == 0 || len(name) > maxUsernameLen {
		return false
	}
	for _, c := range name {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_' || c == '-' || c == '.') {
			return false
		}
	}
	return true
}

func isValidHost(host string) bool {
	if len(host) == 0 || len(host) > maxHostLen {
		return false
	}
	if host == "%" || host == "localhost" {
		return true
	}
	for _, c := range host {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '.' || c == '-' || c == ':') {
			return false
		}
	}
	return true
}

func isValidCharset(charset string) bool {
	return validCharsets[charset]
}

// allowedColumnTypes is the whitelist of column types BuildCreateTable accepts —
// a union of the MySQL and PostgreSQL type sets (SERIAL/UUID/JSONB/BYTEA are PG,
// YEAR/TINYINT/DATETIME are MySQL). Each engine rejects what it doesn't know;
// the panel just guards against injection-shaped input. DOUBLE PRECISION covers
// MySQL's DOUBLE alias too.
var allowedColumnTypes = map[string]bool{
	"INT": true, "INTEGER": true, "TINYINT": true, "SMALLINT": true, "MEDIUMINT": true, "BIGINT": true,
	"FLOAT": true, "DOUBLE": true, "DOUBLE PRECISION": true, "DECIMAL": true, "NUMERIC": true, "REAL": true,
	"VARCHAR": true, "CHAR": true, "CHARACTER": true, "CHARACTER VARYING": true,
	"TEXT": true, "TINYTEXT": true, "MEDIUMTEXT": true, "LONGTEXT": true,
	"BLOB": true, "TINYBLOB": true, "MEDIUMBLOB": true, "LONGBLOB": true, "BINARY": true, "VARBINARY": true, "BYTEA": true,
	"DATE": true, "TIME": true, "TIMETZ": true, "DATETIME": true, "TIMESTAMP": true, "TIMESTAMPTZ": true, "YEAR": true, "INTERVAL": true,
	"BOOLEAN": true, "BOOL": true, "BIT": true,
	"JSON": true, "ENUM": true, "SET": true, "MONEY": true,
	"SERIAL": true, "BIGSERIAL": true, "SMALLSERIAL": true,
	"UUID": true, "JSONB": true,
}

// --- 权限白名单 ---

// ValidMySQLPrivileges is the whitelist of valid MySQL privileges
var ValidMySQLPrivileges = map[string]bool{
	"ALL": true, "ALL PRIVILEGES": true,
	"SELECT": true, "INSERT": true, "UPDATE": true, "DELETE": true,
	"CREATE": true, "DROP": true, "RELOAD": true, "SHUTDOWN": true,
	"PROCESS": true, "FILE": true, "REFERENCES": true, "INDEX": true,
	"ALTER": true, "SHOW DATABASES": true, "SUPER": true,
	"CREATE TEMPORARY TABLES": true, "LOCK TABLES": true,
	"EXECUTE": true, "REPLICATION SLAVE": true, "REPLICATION CLIENT": true,
	"CREATE VIEW": true, "SHOW VIEW": true, "CREATE ROUTINE": true,
	"ALTER ROUTINE": true, "CREATE USER": true, "EVENT": true,
	"TRIGGER": true, "CREATE TABLESPACE": true,
}

// ValidPostgreSQLPrivileges is the whitelist of valid PostgreSQL privileges
var ValidPostgreSQLPrivileges = map[string]bool{
	"ALL": true, "ALL PRIVILEGES": true,
	"SELECT": true, "INSERT": true, "UPDATE": true, "DELETE": true,
	"TRUNCATE": true, "REFERENCES": true, "TRIGGER": true,
	"CREATE": true, "CONNECT": true, "TEMPORARY": true, "TEMP": true,
	"EXECUTE": true, "USAGE": true,
}

// ValidatePrivileges validates that privileges string only contains valid privilege names.
// Returns empty string if invalid privileges are found.
func ValidatePrivileges(dbType DBType, privileges string) string {
	validMap := ValidMySQLPrivileges
	if dbType == DBTypePostgreSQL {
		validMap = ValidPostgreSQLPrivileges
	}

	// Split by comma and validate each privilege
	parts := strings.Split(privileges, ",")
	var valid []string
	for _, p := range parts {
		p = strings.TrimSpace(strings.ToUpper(p))
		if p == "" {
			continue
		}
		if !validMap[p] {
			return "" // Invalid privilege found
		}
		valid = append(valid, p)
	}

	if len(valid) == 0 {
		return ""
	}
	return strings.Join(valid, ", ")
}

// --- SQL 语句安全闸门 ---

// ValidateSQL validates a raw SQL statement for safety before execution (the
// ExecuteSQL entry point). It is not a per-operation validator — the Build*
// methods already validate the DML they generate; this guards free-form input.
func ValidateSQL(sql string) *ValidationResult {
	sql = strings.TrimSpace(sql)
	if sql == "" {
		return &ValidationResult{Valid: false, Message: "SQL cannot be empty"}
	}

	statements := strings.Split(sql, ";")
	for _, stmt := range statements {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" {
			continue
		}

		cleanedStmt := stripLeadingComments(stmt)
		if cleanedStmt == "" {
			continue
		}

		if result := validateSingleStatement(cleanedStmt); result != nil {
			return result
		}
	}

	if !strings.HasSuffix(sql, ";") {
		sql += ";"
	}

	return &ValidationResult{Valid: true, Message: "valid", SQL: sql}
}

func stripLeadingComments(sql string) string {
	for {
		sql = strings.TrimSpace(sql)
		if strings.HasPrefix(sql, "--") {
			if idx := strings.Index(sql, "\n"); idx >= 0 {
				sql = sql[idx+1:]
			} else {
				return ""
			}
		} else if strings.HasPrefix(sql, "/*") {
			if idx := strings.Index(sql, "*/"); idx >= 0 {
				sql = sql[idx+2:]
			} else {
				return ""
			}
		} else {
			break
		}
	}
	return sql
}

func validateSingleStatement(sql string) *ValidationResult {
	upper := strings.ToUpper(sql)

	prefixBlocked := []string{
		"DROP DATABASE", "DROP SCHEMA",
		"DROP TABLE", "DROP INDEX",
		"TRUNCATE",
		"GRANT",
		"REVOKE",
		"ALTER USER", "ALTER SYSTEM", "ALTER TABLE",
		"CREATE USER", "DROP USER",
		"CREATE DATABASE", "CREATE SCHEMA",
		"LOAD DATA", "LOAD FILE", "INTO OUTFILE", "INTO DUMPFILE",
		"SET PASSWORD", "ALTER DATABASE",
	}
	for _, b := range prefixBlocked {
		if strings.HasPrefix(upper, b) {
			return &ValidationResult{Valid: false, Message: fmt.Sprintf("%s is not allowed", b)}
		}
	}

	if strings.HasPrefix(upper, "DELETE") {
		if !strings.Contains(upper, " WHERE ") {
			return &ValidationResult{Valid: false, Message: "DELETE without WHERE clause is not allowed"}
		}
	}

	if strings.HasPrefix(upper, "UPDATE") {
		if !strings.Contains(upper, " WHERE ") {
			return &ValidationResult{Valid: false, Message: "UPDATE without WHERE clause is not allowed"}
		}
	}

	if strings.Contains(upper, "SET PASSWORD") || strings.Contains(upper, "IDENTIFIED BY") {
		return &ValidationResult{Valid: false, Message: "password modification is not allowed"}
	}

	if strings.Contains(upper, "SLEEP(") || strings.Contains(upper, "BENCHMARK(") {
		return &ValidationResult{Valid: false, Message: "SLEEP/BENCHMARK functions are not allowed"}
	}

	if strings.Contains(upper, "INTO OUTFILE") || strings.Contains(upper, "INTO DUMPFILE") {
		return &ValidationResult{Valid: false, Message: "writing to filesystem is not allowed"}
	}

	// Block system catalog access
	if strings.Contains(upper, "INFORMATION_SCHEMA") {
		return &ValidationResult{Valid: false, Message: "accessing INFORMATION_SCHEMA is not allowed"}
	}

	// Block MySQL system database (mysql.user etc.) for ALL statement types,
	// including SELECT — it exposes password hashes and user/host grants.
	if strings.Contains(upper, "MYSQL.") {
		return &ValidationResult{Valid: false, Message: "accessing mysql system database is not allowed"}
	}

	// Block PostgreSQL system catalogs
	if strings.Contains(upper, "PG_") || strings.Contains(upper, "PGCATALOG") {
		return &ValidationResult{Valid: false, Message: "accessing system catalogs is not allowed"}
	}

	// Block comment-based bypass attempts
	if strings.Contains(sql, "/*") || strings.Contains(sql, "*/") {
		return &ValidationResult{Valid: false, Message: "block comments are not allowed"}
	}

	return nil
}

// --- 错误脱敏 ---

var pathPattern = regexp.MustCompile(`(?:/[\w.-]+){2,}`)

// SanitizeSQLError strips sensitive information (file paths) from SQL error output.
func SanitizeSQLError(raw string) string {
	lines := strings.Split(raw, "\n")
	var sanitized []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		line = pathPattern.ReplaceAllString(line, "[...]")
		sanitized = append(sanitized, line)
	}
	return strings.Join(sanitized, "\n")
}

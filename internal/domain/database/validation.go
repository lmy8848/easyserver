package database

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// copyProgramRe 命中 PostgreSQL 的 COPY ... TO/FROM PROGRAM（以服务器权限
// 执行任意 OS 命令，等效主机 RCE）
var copyProgramRe = regexp.MustCompile(`\bCOPY\b.*\bPROGRAM\b`)

// --- 名称/字符集校验 ---

const (
	maxDBNameLen   = 64
	maxUsernameLen = 32
	maxHostLen     = 255
	defaultCharset = "utf8mb4"
)

// --- 名称/标识符校验 ---
//
// 这些校验由 SQLBuilder 的 Build* 方法内嵌调用（构建即校验），也供路由层
// 的路径参数校验复用（isValidDBName）。

var (
	tableNameRegexp  = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]{0,63}$`)
	columnNameRegexp = regexp.MustCompile(`^[a-zA-Z0-9_\-]{1,64}$`)
	dbNameRegexp     = regexp.MustCompile(`^[a-zA-Z0-9_.\-]{1,64}$`)
	usernameRegexp   = regexp.MustCompile(`^[a-zA-Z0-9_.\-]{1,32}$`)
	hostRegexp       = regexp.MustCompile(`^[a-zA-Z0-9_.\-:%]{1,255}$`)
	charsetRegexp    = regexp.MustCompile(`^[a-zA-Z0-9_\-]{1,64}$`)
	collationRegexp  = regexp.MustCompile(`^[a-zA-Z0-9_.\-]{1,64}$`)
)

// IsValidTableName checks table name validity.
func IsValidTableName(name string) bool {
	return tableNameRegexp.MatchString(name)
}

func isValidTableName(name string) bool {
	return IsValidTableName(name)
}

// isValidColumnName checks a column name for use in DML statements. Columns
// may carry a hyphen, unlike table names.
func isValidColumnName(name string) error {
	if !columnNameRegexp.MatchString(name) {
		return fmt.Errorf("invalid column identifier: %s", name)
	}
	return nil
}

// IsValidDBName checks database name validity.
func IsValidDBName(name string) bool {
	return dbNameRegexp.MatchString(name)
}

func isValidDBName(name string) bool {
	return IsValidDBName(name)
}

func isValidUsername(name string) bool {
	return usernameRegexp.MatchString(name)
}

func isValidHost(host string) bool {
	return hostRegexp.MatchString(host)
}

func isValidCharset(charset string) bool {
	return charsetRegexp.MatchString(charset)
}

func isValidCollation(collation string) bool {
	return collationRegexp.MatchString(collation)
}

// --- 权限校验 ---

var privilegeRegexp = regexp.MustCompile(`^[a-zA-Z0-9_\s]{1,64}$`)

// ValidatePrivileges validates privilege names format to prevent SQL injection,
// leaving actual privilege enforcement to the database engine.
func ValidatePrivileges(dbType DBType, privileges string) string {
	// Split by comma and validate each privilege syntax
	parts := strings.Split(privileges, ",")
	var valid []string
	for _, p := range parts {
		p = strings.TrimSpace(strings.ToUpper(p))
		if p == "" {
			continue
		}
		if !privilegeRegexp.MatchString(p) {
			return "" // Invalid privilege format / injection attempt
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

	// 仅允许单条语句：多语句会绕过按前缀/首语句的所有限制
	// （此前 MySQL DSN multiStatements=true 使 "SELECT 1; DELETE ..." 直接执行）
	if err := countStatements(sql); err != nil {
		return &ValidationResult{Valid: false, Message: err.Error()}
	}

	cleanedStmt := stripLeadingComments(sql)
	if cleanedStmt == "" {
		return &ValidationResult{Valid: false, Message: "SQL cannot be empty"}
	}
	if result := validateSingleStatement(cleanedStmt); result != nil {
		return result
	}

	return &ValidationResult{Valid: true, Message: "valid", SQL: sql}
}

// countStatements 检查输入是否只包含一条语句：统计字符串/标识符引用之外的
// 分号数量（末尾分号在统计前剥离，属于常见书写习惯）。
func countStatements(sql string) error {
	trimmed := strings.TrimRight(strings.TrimSpace(sql), "; \t\r\n")
	inSingle, inDouble, inBacktick := false, false, false
	for i := 0; i < len(trimmed); i++ {
		switch c := trimmed[i]; {
		case c == '\'' && !inDouble && !inBacktick:
			// MySQL 单引号内 '' 转义与 \' 转义
			if inSingle && i+1 < len(trimmed) && trimmed[i+1] == '\'' {
				i++
				continue
			}
			if inSingle && i > 0 && trimmed[i-1] == '\\' {
				continue
			}
			inSingle = !inSingle
		case c == '"' && !inSingle && !inBacktick:
			inDouble = !inDouble
		case c == '`' && !inSingle && !inDouble:
			inBacktick = !inBacktick
		case c == '\\' && (inSingle || inDouble):
			i++ // 跳过转义字符
		case c == ';' && !inSingle && !inDouble && !inBacktick:
			return errors.New("only a single SQL statement is allowed")
		}
	}
	return nil
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

	// IDENTIFIED WITH 覆盖 MySQL 8 的 ALTER USER ... IDENTIFIED WITH xxx BY '...'，
	// 只拦 IDENTIFIED BY 会漏掉该写法
	if strings.Contains(upper, "SET PASSWORD") || strings.Contains(upper, "IDENTIFIED BY") || strings.Contains(upper, "IDENTIFIED WITH") {
		return &ValidationResult{Valid: false, Message: "password modification is not allowed"}
	}

	// PostgreSQL COPY ... TO/FROM PROGRAM 以服务器权限执行任意 OS 命令
	if copyProgramRe.MatchString(upper) {
		return &ValidationResult{Valid: false, Message: "COPY TO/FROM PROGRAM is not allowed"}
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

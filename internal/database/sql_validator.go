package database

import (
	"fmt"
	"regexp"
	"strings"
)

// SQLValidator validates SQL statements without executing them.
type SQLValidator struct {
	builder *SQLBuilder
}

// NewSQLValidator creates a new SQLValidator.
func NewSQLValidator(dbType DBType) *SQLValidator {
	return &SQLValidator{builder: NewSQLBuilder(dbType)}
}

// ValidateIdentifier validates a table or column name.
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

// ValidateTableName validates a table name.
func (v *SQLValidator) ValidateTableName(name string) *ValidationResult {
	return v.ValidateIdentifier(name)
}

// ValidateInsert validates an INSERT operation.
func (v *SQLValidator) ValidateInsert(table string, data map[string]interface{}, tableInfo *TableInfo) *ValidationResult {
	if r := v.ValidateTableName(table); !r.Valid {
		return r
	}
	if len(data) == 0 {
		return &ValidationResult{Valid: false, Message: "no data to insert"}
	}

	for col := range data {
		if r := v.ValidateIdentifier(col); !r.Valid {
			return &ValidationResult{Valid: false, Message: fmt.Sprintf("invalid column '%s': %s", col, r.Message)}
		}
	}

	sql := v.builder.BuildInsert(table, data, tableInfo)
	return &ValidationResult{Valid: true, Message: "valid", SQL: sql}
}

// ValidateUpdate validates an UPDATE operation.
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

// ValidateDelete validates a DELETE operation.
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

// ValidateSQL validates a raw SQL statement for safety.
func (v *SQLValidator) ValidateSQL(sql string) *ValidationResult {
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

// tableInfoFromQuery converts a structured describe query result (name, type,
// nullable, default, pk-flag per column) into TableInfo. It is the direct-channel
// equivalent of the old DESCRIBE-text parser — the driver returns the describe
// rows structurally, so no text parsing is needed.
func tableInfoFromQuery(dbType DBType, tableName string, res *QueryResult) *TableInfo {
	info := &TableInfo{Name: tableName, Columns: []ColumnInfo{}}
	// Map column name → row index so both mysql DESCRIBE and the pg
	// information_schema query (which has its own header order) work.
	colIdx := map[string]int{}
	for i, c := range res.Columns {
		colIdx[c.Name] = i
	}
	nameIdx := colIdx["Field"]
	if _, ok := colIdx["column_name"]; ok {
		nameIdx = colIdx["column_name"]
	}
	if nameIdx < 0 && len(res.Columns) == 0 {
		return info
	}
	hasType := colIdx["Type"] >= 0
	if _, ok := colIdx["data_type"]; ok {
		hasType = true
	}
	typeIdx := colIdx["Type"]
	if v, ok := colIdx["data_type"]; ok {
		typeIdx = v
	}
	for _, row := range res.Rows {
		if nameIdx >= len(row) || row[nameIdx] == nil {
			continue
		}
		col := ColumnInfo{Name: str(row, nameIdx)}
		if hasType {
			col.Type = str(row, typeIdx)
		}
		col.IsNullable = !isTruthy(row, colIdx["Null"]) // mysql: "NO" → not nullable
		if v, ok := colIdx["is_nullable"]; ok {
			col.IsNullable = strings.EqualFold(str(row, v), "yes")
		}
		if v, ok := colIdx["Key"]; ok && strings.EqualFold(str(row, v), "pri") {
			col.IsPrimaryKey = true
			info.PrimaryKey = col.Name
		}
		if v, ok := colIdx["is_primary"]; ok && isTruthy(row, v) {
			col.IsPrimaryKey = true
			info.PrimaryKey = col.Name
		}
		if v, ok := colIdx["Extra"]; ok && strings.Contains(strings.ToLower(str(row, v)), "auto_increment") {
			col.IsAutoIncr = true
		}
		if v, ok := colIdx["Default"]; ok && row[v] != nil {
			col.HasDefault = true
			col.DefaultValue = str(row, v)
		}
		if v, ok := colIdx["column_default"]; ok && row[v] != nil {
			col.HasDefault = true
			col.DefaultValue = str(row, v)
		}
		info.Columns = append(info.Columns, col)
	}
	return info
}

// isTruthy reports whether a cell is a truthy value: non-nil and not one of the
// common falsy spellings.
func isTruthy(row []any, i int) bool {
	if i < 0 || i >= len(row) || row[i] == nil {
		return false
	}
	switch v := row[i].(type) {
	case string:
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "", "no", "false", "off", "0":
			return false
		}
	case []byte:
		return isTruthy([]any{v}, 0) == true
	case bool:
		return v
	}
	return true
}

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

var tableNameRegexp = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)

// allowedColumnTypes is the whitelist of column types CreateTable accepts. Types
// that appear only in one engine (SERIAL/UUID/JSONB are PG, YEAR is MySQL) are
// listed anyway — the engine rejects what it doesn't know, the panel just guards
// against injection-shaped input.
var allowedColumnTypes = map[string]bool{
	"INT": true, "INTEGER": true, "TINYINT": true, "SMALLINT": true, "MEDIUMINT": true, "BIGINT": true,
	"FLOAT": true, "DOUBLE": true, "DECIMAL": true, "NUMERIC": true, "REAL": true,
	"VARCHAR": true, "CHAR": true, "TEXT": true, "TINYTEXT": true, "MEDIUMTEXT": true, "LONGTEXT": true,
	"BLOB": true, "TINYBLOB": true, "MEDIUMBLOB": true, "LONGBLOB": true, "BINARY": true, "VARBINARY": true,
	"DATE": true, "TIME": true, "DATETIME": true, "TIMESTAMP": true, "YEAR": true,
	"BOOLEAN": true, "BOOL": true, "BIT": true,
	"JSON": true, "ENUM": true, "SET": true,
	"SERIAL": true, "BIGSERIAL": true, "SMALLSERIAL": true,
	"UUID": true, "JSONB": true,
}

// ValidateTableName checks table/column name validity.
func ValidateTableName(name string) bool {
	return name != "" && len(name) <= 64 && tableNameRegexp.MatchString(name)
}

// --- Validation helpers ---

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

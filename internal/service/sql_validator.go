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

// ValidateSQL validates a raw SQL statement for safety
func (v *SQLValidator) ValidateSQL(sql string) *ValidationResult {
	sql = strings.TrimSpace(sql)
	if sql == "" {
		return &ValidationResult{Valid: false, Message: "SQL cannot be empty"}
	}

	// Block dangerous statements
	upper := strings.ToUpper(sql)
	blocked := []string{
		"DROP DATABASE", "DROP SCHEMA",
		"TRUNCATE", "GRANT ALL",
	}
	for _, b := range blocked {
		if strings.HasPrefix(upper, b) {
			return &ValidationResult{Valid: false, Message: fmt.Sprintf("%s is not allowed", b)}
		}
	}

	// Basic syntax check: must end with semicolon
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

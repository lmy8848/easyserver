package database_mgmt

import (
	"fmt"
	"strings"
)

// SQLBuilder generates SQL statements per database type.
type SQLBuilder struct {
	dbType DBType
}

// NewSQLBuilder creates a new SQLBuilder.
func NewSQLBuilder(dbType DBType) *SQLBuilder {
	return &SQLBuilder{dbType: dbType}
}

// QuoteIdentifier quotes a table/column identifier.
func (b *SQLBuilder) QuoteIdentifier(name string) string {
	switch b.dbType {
	case DBTypeMySQL:
		return "`" + strings.ReplaceAll(name, "`", "``") + "`"
	case DBTypePostgreSQL:
		return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
	}
	return name
}

// EscapeString escapes a string value for use in SQL.
func (b *SQLBuilder) EscapeString(s string) string {
	switch b.dbType {
	case DBTypeMySQL:
		s = strings.ReplaceAll(s, `\`, `\\`)
		s = strings.ReplaceAll(s, `'`, `\'`)
		s = strings.ReplaceAll(s, `"`, `\"`)
		s = strings.ReplaceAll(s, "\n", `\n`)
		s = strings.ReplaceAll(s, "\r", `\r`)
		s = strings.ReplaceAll(s, "\t", `\t`)
		s = strings.ReplaceAll(s, "\x00", `\0`)
		return s
	case DBTypePostgreSQL:
		return strings.ReplaceAll(s, "'", "''")
	}
	return s
}

// BuildInsert generates an INSERT statement.
func (b *SQLBuilder) BuildInsert(table string, data map[string]interface{}, tableInfo *TableInfo) string {
	var cols []string
	var vals []string

	for col, val := range data {
		if tableInfo != nil {
			for _, ci := range tableInfo.Columns {
				if ci.Name == col && ci.IsAutoIncr && (val == nil || val == "") {
					goto skip
				}
			}
		}
		cols = append(cols, b.QuoteIdentifier(col))
		vals = append(vals, b.formatValue(val))
	skip:
	}

	return fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s);",
		b.QuoteIdentifier(table),
		strings.Join(cols, ", "),
		strings.Join(vals, ", "))
}

// BuildUpdate generates an UPDATE statement.
func (b *SQLBuilder) BuildUpdate(table string, data map[string]interface{}, pkCol string, pkVal interface{}) string {
	var sets []string
	for col, val := range data {
		if col == pkCol {
			continue
		}
		sets = append(sets, fmt.Sprintf("%s = %s", b.QuoteIdentifier(col), b.formatValue(val)))
	}

	return fmt.Sprintf("UPDATE %s SET %s WHERE %s = %s;",
		b.QuoteIdentifier(table),
		strings.Join(sets, ", "),
		b.QuoteIdentifier(pkCol),
		b.formatValue(pkVal))
}

// BuildDelete generates a DELETE statement.
func (b *SQLBuilder) BuildDelete(table string, pkCol string, pkVal interface{}) string {
	return fmt.Sprintf("DELETE FROM %s WHERE %s = %s;",
		b.QuoteIdentifier(table),
		b.QuoteIdentifier(pkCol),
		b.formatValue(pkVal))
}

// BuildSelect generates a SELECT statement with pagination.
func (b *SQLBuilder) BuildSelect(table string, columns []string, page, pageSize int) string {
	cols := "*"
	if len(columns) > 0 {
		quoted := make([]string, len(columns))
		for i, c := range columns {
			quoted[i] = b.QuoteIdentifier(c)
		}
		cols = strings.Join(quoted, ", ")
	}

	offset := (page - 1) * pageSize
	return fmt.Sprintf("SELECT %s FROM %s LIMIT %d OFFSET %d;",
		cols, b.QuoteIdentifier(table), pageSize, offset)
}

// BuildCount generates a COUNT query.
func (b *SQLBuilder) BuildCount(table string) string {
	return fmt.Sprintf("SELECT COUNT(*) FROM %s;", b.QuoteIdentifier(table))
}

// BuildListTables generates a query to list tables.
func (b *SQLBuilder) BuildListTables() string {
	switch b.dbType {
	case DBTypeMySQL:
		return "SHOW TABLES;"
	case DBTypePostgreSQL:
		return "SELECT tablename FROM pg_tables WHERE schemaname = 'public' ORDER BY tablename;"
	}
	return ""
}

// BuildDescribeTable generates a query to describe table structure.
// Table name must be validated before calling this function.
func (b *SQLBuilder) BuildDescribeTable(table string) string {
	switch b.dbType {
	case DBTypeMySQL:
		return fmt.Sprintf("DESCRIBE %s;", b.QuoteIdentifier(table))
	case DBTypePostgreSQL:
		// Use QuoteIdentifier for safe table name quoting
		quotedTable := b.QuoteIdentifier(table)
		return fmt.Sprintf(`SELECT column_name, data_type, is_nullable, column_default,
			CASE WHEN column_name IN (
				SELECT a.attname FROM pg_index i
				JOIN pg_attribute a ON a.attrelid = i.indrelid AND a.attnum = ANY(i.indkey)
				WHERE i.indrelid = %s::regclass AND i.indisprimary
			) THEN 'YES' ELSE 'NO' END as is_primary
			FROM information_schema.columns
			WHERE table_name = $1 ORDER BY ordinal_position;`, quotedTable)
	}
	return ""
}

// BuildCreateDatabase generates a CREATE DATABASE statement.
func (b *SQLBuilder) BuildCreateDatabase(name string, charset string) string {
	switch b.dbType {
	case DBTypeMySQL:
		if charset == "" {
			charset = "utf8mb4"
		}
		return fmt.Sprintf("CREATE DATABASE `%s` CHARACTER SET %s;", name, charset)
	case DBTypePostgreSQL:
		encoding := "UTF8"
		if charset == "latin1" {
			encoding = "LATIN1"
		}
		return fmt.Sprintf(`CREATE DATABASE "%s" ENCODING '%s';`, name, encoding)
	}
	return ""
}

// BuildDropDatabase generates a DROP DATABASE statement.
func (b *SQLBuilder) BuildDropDatabase(name string) string {
	switch b.dbType {
	case DBTypeMySQL:
		return fmt.Sprintf("DROP DATABASE `%s`;", name)
	case DBTypePostgreSQL:
		return fmt.Sprintf(`DROP DATABASE "%s";`, name)
	}
	return ""
}

// BuildCreateUser generates a CREATE USER statement.
func (b *SQLBuilder) BuildCreateUser(username, password, host string) string {
	switch b.dbType {
	case DBTypeMySQL:
		return fmt.Sprintf("CREATE USER '%s'@'%s' IDENTIFIED BY '%s';",
			b.EscapeString(username), b.EscapeString(host), b.EscapeString(password))
	case DBTypePostgreSQL:
		return fmt.Sprintf(`CREATE USER "%s" WITH PASSWORD '%s';`,
			strings.ReplaceAll(username, `"`, `""`), b.EscapeString(password))
	}
	return ""
}

// BuildDropUser generates a DROP USER statement.
func (b *SQLBuilder) BuildDropUser(username, host string) string {
	switch b.dbType {
	case DBTypeMySQL:
		return fmt.Sprintf("DROP USER '%s'@'%s';", b.EscapeString(username), b.EscapeString(host))
	case DBTypePostgreSQL:
		return fmt.Sprintf(`DROP USER "%s";`, strings.ReplaceAll(username, `"`, `""`))
	}
	return ""
}

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

// BuildGrant generates a GRANT statement.
// Privileges must be validated before calling this function.
func (b *SQLBuilder) BuildGrant(privileges, database, username, host string) string {
	// Validate privileges
	validatedPrivs := ValidatePrivileges(b.dbType, privileges)
	if validatedPrivs == "" {
		return "" // Invalid privileges
	}

	switch b.dbType {
	case DBTypeMySQL:
		return fmt.Sprintf("GRANT %s ON %s.* TO '%s'@'%s'; FLUSH PRIVILEGES;",
			validatedPrivs, b.QuoteIdentifier(database), b.EscapeString(username), b.EscapeString(host))
	case DBTypePostgreSQL:
		return fmt.Sprintf(`GRANT %s ON DATABASE %s TO %s;`,
			validatedPrivs, b.QuoteIdentifier(database), b.QuoteIdentifier(username))
	}
	return ""
}

func (b *SQLBuilder) formatValue(val interface{}) string {
	if val == nil {
		return "NULL"
	}
	switch v := val.(type) {
	case string:
		return "'" + b.EscapeString(v) + "'"
	case int, int64, float64:
		return fmt.Sprintf("%v", v)
	case bool:
		if v {
			switch b.dbType {
			case DBTypeMySQL:
				return "1"
			case DBTypePostgreSQL:
				return "true"
			}
		}
		switch b.dbType {
		case DBTypeMySQL:
			return "0"
		case DBTypePostgreSQL:
			return "false"
		}
	}
	return "'" + b.EscapeString(fmt.Sprintf("%v", val)) + "'"
}

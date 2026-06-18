package service

import (
	"fmt"
	"strings"
)

// DBType represents the database engine type
type DBType string

const (
	DBTypeMySQL      DBType = "mysql"
	DBTypePostgreSQL DBType = "postgresql"
	DBTypeRedis      DBType = "redis"
)

// ColumnInfo represents a column's metadata
type ColumnInfo struct {
	Name         string
	Type         string
	IsPrimaryKey bool
	IsAutoIncr   bool
	HasDefault   bool
	DefaultValue string
	IsNullable   bool
}

// TableInfo represents a table's metadata
type TableInfo struct {
	Name       string
	PrimaryKey string
	Columns    []ColumnInfo
}

// SQLBuilder generates SQL statements per database type
type SQLBuilder struct {
	dbType DBType
}

func NewSQLBuilder(dbType DBType) *SQLBuilder {
	return &SQLBuilder{dbType: dbType}
}

// QuoteIdentifier quotes a table/column identifier
func (b *SQLBuilder) QuoteIdentifier(name string) string {
	switch b.dbType {
	case DBTypeMySQL:
		return "`" + strings.ReplaceAll(name, "`", "``") + "`"
	case DBTypePostgreSQL:
		return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
	}
	return name
}

// EscapeString escapes a string value for use in SQL
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

// BuildInsert generates an INSERT statement
// If data omits auto-increment fields, they are excluded
func (b *SQLBuilder) BuildInsert(table string, data map[string]interface{}, tableInfo *TableInfo) string {
	var cols []string
	var vals []string

	for col, val := range data {
		// Skip auto-increment fields if value is nil/empty
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

// BuildUpdate generates an UPDATE statement
func (b *SQLBuilder) BuildUpdate(table string, data map[string]interface{}, pkCol string, pkVal interface{}) string {
	var sets []string
	for col, val := range data {
		if col == pkCol {
			continue // don't update primary key
		}
		sets = append(sets, fmt.Sprintf("%s = %s", b.QuoteIdentifier(col), b.formatValue(val)))
	}

	return fmt.Sprintf("UPDATE %s SET %s WHERE %s = %s;",
		b.QuoteIdentifier(table),
		strings.Join(sets, ", "),
		b.QuoteIdentifier(pkCol),
		b.formatValue(pkVal))
}

// BuildDelete generates a DELETE statement
func (b *SQLBuilder) BuildDelete(table string, pkCol string, pkVal interface{}) string {
	return fmt.Sprintf("DELETE FROM %s WHERE %s = %s;",
		b.QuoteIdentifier(table),
		b.QuoteIdentifier(pkCol),
		b.formatValue(pkVal))
}

// BuildSelect generates a SELECT statement with pagination
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

// BuildCount generates a COUNT query
func (b *SQLBuilder) BuildCount(table string) string {
	return fmt.Sprintf("SELECT COUNT(*) FROM %s;", b.QuoteIdentifier(table))
}

// BuildListTables generates a query to list tables
func (b *SQLBuilder) BuildListTables() string {
	switch b.dbType {
	case DBTypeMySQL:
		return "SHOW TABLES;"
	case DBTypePostgreSQL:
		return "SELECT tablename FROM pg_tables WHERE schemaname = 'public' ORDER BY tablename;"
	}
	return ""
}

// BuildDescribeTable generates a query to describe table structure
func (b *SQLBuilder) BuildDescribeTable(table string) string {
	switch b.dbType {
	case DBTypeMySQL:
		return fmt.Sprintf("DESCRIBE `%s`;", table)
	case DBTypePostgreSQL:
		return fmt.Sprintf(`SELECT column_name, data_type, is_nullable, column_default,
			CASE WHEN column_name IN (
				SELECT a.attname FROM pg_index i
				JOIN pg_attribute a ON a.attrelid = i.indrelid AND a.attnum = ANY(i.indkey)
				WHERE i.indrelid = '%s'::regclass AND i.indisprimary
			) THEN 'YES' ELSE 'NO' END as is_primary
			FROM information_schema.columns
			WHERE table_name = '%s' ORDER BY ordinal_position;`, table, table)
	}
	return ""
}

// formatValue formats a value for SQL
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

// BuildDropDatabase generates a DROP DATABASE statement
func (b *SQLBuilder) BuildDropDatabase(name string) string {
	switch b.dbType {
	case DBTypeMySQL:
		return fmt.Sprintf("DROP DATABASE `%s`;", name)
	case DBTypePostgreSQL:
		return fmt.Sprintf(`DROP DATABASE "%s";`, name)
	}
	return ""
}

// BuildCreateUser generates a CREATE USER statement
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

// BuildDropUser generates a DROP USER statement
func (b *SQLBuilder) BuildDropUser(username, host string) string {
	switch b.dbType {
	case DBTypeMySQL:
		return fmt.Sprintf("DROP USER '%s'@'%s';", b.EscapeString(username), b.EscapeString(host))
	case DBTypePostgreSQL:
		return fmt.Sprintf(`DROP USER "%s";`, strings.ReplaceAll(username, `"`, `""`))
	}
	return ""
}

// BuildGrant generates a GRANT statement
func (b *SQLBuilder) BuildGrant(privileges, database, username, host string) string {
	switch b.dbType {
	case DBTypeMySQL:
		return fmt.Sprintf("GRANT %s ON `%s`.* TO '%s'@'%s'; FLUSH PRIVILEGES;",
			privileges, database, b.EscapeString(username), b.EscapeString(host))
	case DBTypePostgreSQL:
		return fmt.Sprintf(`GRANT %s ON DATABASE "%s" TO "%s";`,
			privileges, database, strings.ReplaceAll(username, `"`, `""`))
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

package database

import (
	"fmt"
	"strings"
)

// SQLBuilder generates SQL statements per database type. Every Build* method
// validates its inputs (names, non-empty data, pk presence) and returns an
// error instead of producing an unsafe or meaningless statement — callers don't
// need a separate validation pass.
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

// placeholder returns the parameter placeholder for the n-th argument: `?` for
// MySQL, `$n` for PostgreSQL. The driver channel binds these parameters.
func (b *SQLBuilder) placeholder(n int) string {
	if b.dbType == DBTypePostgreSQL {
		return fmt.Sprintf("$%d", n)
	}
	return "?"
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

// BuildInsert generates an INSERT statement (validated table/column names,
// non-empty data).
func (b *SQLBuilder) BuildInsert(table string, data map[string]interface{}, tableInfo *TableInfo) (string, error) {
	if !isValidTableName(table) {
		return "", fmt.Errorf("无效的表名")
	}
	if len(data) == 0 {
		return "", fmt.Errorf("no data to insert")
	}
	for col := range data {
		if err := isValidColumnName(col); err != nil {
			return "", fmt.Errorf("invalid column '%s': %s", col, err)
		}
	}

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
		strings.Join(vals, ", ")), nil
}

// BuildInsertParams is the parameterized form of BuildInsert: the SQL carries
// placeholders and the values come back as args for driver binding. The dry-run
// preview still uses BuildInsert so the user sees rendered SQL.
func (b *SQLBuilder) BuildInsertParams(table string, data map[string]interface{}, tableInfo *TableInfo) (string, []any, error) {
	if !isValidTableName(table) {
		return "", nil, fmt.Errorf("无效的表名")
	}
	if len(data) == 0 {
		return "", nil, fmt.Errorf("no data to insert")
	}
	for col := range data {
		if err := isValidColumnName(col); err != nil {
			return "", nil, fmt.Errorf("invalid column '%s': %s", col, err)
		}
	}

	var cols []string
	var ph []string
	var args []any
	n := 0
	for col, val := range data {
		if tableInfo != nil {
			for _, ci := range tableInfo.Columns {
				if ci.Name == col && ci.IsAutoIncr && (val == nil || val == "") {
					goto skip
				}
			}
		}
		cols = append(cols, b.QuoteIdentifier(col))
		n++
		ph = append(ph, b.placeholder(n))
		args = append(args, val)
	skip:
	}
	return fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s);",
		b.QuoteIdentifier(table), strings.Join(cols, ", "), strings.Join(ph, ", ")), args, nil
}

// BuildUpdate generates an UPDATE statement (validated table/primary-key names,
// non-empty data, non-nil pk value).
func (b *SQLBuilder) BuildUpdate(table string, data map[string]interface{}, pkCol string, pkVal interface{}) (string, error) {
	if !isValidTableName(table) {
		return "", fmt.Errorf("无效的表名")
	}
	if err := isValidColumnName(pkCol); err != nil {
		return "", fmt.Errorf("invalid primary key column '%s': %s", pkCol, err)
	}
	if len(data) == 0 {
		return "", fmt.Errorf("no data to update")
	}
	if pkVal == nil {
		return "", fmt.Errorf("primary key value is required")
	}

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
		b.formatValue(pkVal)), nil
}

// BuildUpdateParams is the parameterized form of BuildUpdate.
func (b *SQLBuilder) BuildUpdateParams(table string, data map[string]interface{}, pkCol string, pkVal interface{}) (string, []any, error) {
	if !isValidTableName(table) {
		return "", nil, fmt.Errorf("无效的表名")
	}
	if err := isValidColumnName(pkCol); err != nil {
		return "", nil, fmt.Errorf("invalid primary key column '%s': %s", pkCol, err)
	}
	if len(data) == 0 {
		return "", nil, fmt.Errorf("no data to update")
	}
	if pkVal == nil {
		return "", nil, fmt.Errorf("primary key value is required")
	}

	var sets []string
	var args []any
	n := 0
	for col, val := range data {
		if col == pkCol {
			continue
		}
		n++
		sets = append(sets, fmt.Sprintf("%s = %s", b.QuoteIdentifier(col), b.placeholder(n)))
		args = append(args, val)
	}
	n++
	return fmt.Sprintf("UPDATE %s SET %s WHERE %s = %s;",
		b.QuoteIdentifier(table), strings.Join(sets, ", "), b.QuoteIdentifier(pkCol), b.placeholder(n)), append(args, pkVal), nil
}

// BuildDelete generates a DELETE statement (validated table/primary-key names,
// non-nil pk value).
func (b *SQLBuilder) BuildDelete(table string, pkCol string, pkVal interface{}) (string, error) {
	if !isValidTableName(table) {
		return "", fmt.Errorf("无效的表名")
	}
	if err := isValidColumnName(pkCol); err != nil {
		return "", fmt.Errorf("invalid primary key column '%s': %s", pkCol, err)
	}
	if pkVal == nil {
		return "", fmt.Errorf("primary key value is required")
	}
	return fmt.Sprintf("DELETE FROM %s WHERE %s = %s;",
		b.QuoteIdentifier(table),
		b.QuoteIdentifier(pkCol),
		b.formatValue(pkVal)), nil
}

// BuildDeleteParams is the parameterized form of BuildDelete.
func (b *SQLBuilder) BuildDeleteParams(table string, pkCol string, pkVal interface{}) (string, []any, error) {
	if !isValidTableName(table) {
		return "", nil, fmt.Errorf("无效的表名")
	}
	if err := isValidColumnName(pkCol); err != nil {
		return "", nil, fmt.Errorf("invalid primary key column '%s': %s", pkCol, err)
	}
	if pkVal == nil {
		return "", nil, fmt.Errorf("primary key value is required")
	}
	return fmt.Sprintf("DELETE FROM %s WHERE %s = %s;",
		b.QuoteIdentifier(table), b.QuoteIdentifier(pkCol), b.placeholder(1)), []any{pkVal}, nil
}

// BuildSelect generates a SELECT statement with pagination (validated table
// name — it is quoted into the statement).
func (b *SQLBuilder) BuildSelect(table string, columns []string, page, pageSize int) (string, error) {
	if !isValidTableName(table) {
		return "", fmt.Errorf("无效的表名")
	}
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
		cols, b.QuoteIdentifier(table), pageSize, offset), nil
}

// BuildCount generates a COUNT query (validated table name).
func (b *SQLBuilder) BuildCount(table string) (string, error) {
	if !isValidTableName(table) {
		return "", fmt.Errorf("无效的表名")
	}
	return fmt.Sprintf("SELECT COUNT(*) FROM %s;", b.QuoteIdentifier(table)), nil
}

// BuildListTables generates a query to list tables in the current database.
func (b *SQLBuilder) BuildListTables() string {
	switch b.dbType {
	case DBTypeMySQL:
		return "SHOW TABLES;"
	case DBTypePostgreSQL:
		return "SELECT tablename FROM pg_tables WHERE schemaname = 'public' ORDER BY tablename;"
	}
	return ""
}

// BuildDescribeTable generates a query to describe table structure (validated
// table name — it is interpolated into the statement).
func (b *SQLBuilder) BuildDescribeTable(table string) (string, error) {
	if !isValidTableName(table) {
		return "", fmt.Errorf("无效的表名")
	}
	switch b.dbType {
	case DBTypeMySQL:
		return fmt.Sprintf("DESCRIBE %s;", b.QuoteIdentifier(table)), nil
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
			WHERE table_name = $1 ORDER BY ordinal_position;`, quotedTable), nil
	}
	return "", fmt.Errorf("unsupported db type: %s", b.dbType)
}

// BuildCreateTable generates a CREATE TABLE statement for the columns given.
// Column types are checked against the allowedColumnTypes whitelist; names are
// validated and quoted per engine.
func (b *SQLBuilder) BuildCreateTable(tableName string, columns []TableColumn) (string, error) {
	if !isValidTableName(tableName) {
		return "", fmt.Errorf("无效的表名")
	}
	for _, col := range columns {
		baseType := strings.ToUpper(strings.Split(col.Type, "(")[0])
		baseType = strings.TrimSpace(baseType)
		if !allowedColumnTypes[baseType] {
			return "", fmt.Errorf("不支持的列类型: %s", col.Type)
		}
		if !isValidTableName(col.Name) {
			return "", fmt.Errorf("无效的列名: %s", col.Name)
		}
	}

	switch b.dbType {
	case DBTypeMySQL:
		var parts []string
		for _, col := range columns {
			p := []string{fmt.Sprintf("`%s`", col.Name), col.Type}
			if col.IsPrimary {
				p = append(p, "PRIMARY KEY")
			}
			if col.AutoIncr {
				p = append(p, "AUTO_INCREMENT")
			}
			if !col.Nullable {
				p = append(p, "NOT NULL")
			}
			parts = append(parts, strings.Join(p, " "))
		}
		return fmt.Sprintf("CREATE TABLE `%s` (%s) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;", tableName, strings.Join(parts, ", ")), nil
	case DBTypePostgreSQL:
		var parts []string
		for _, col := range columns {
			p := []string{fmt.Sprintf("\"%s\"", col.Name), col.Type}
			if col.IsPrimary {
				p = append(p, "PRIMARY KEY")
			}
			if col.AutoIncr {
				p = []string{fmt.Sprintf("\"%s\"", col.Name), "SERIAL", "PRIMARY KEY"}
			}
			if !col.Nullable && !col.IsPrimary {
				p = append(p, "NOT NULL")
			}
			parts = append(parts, strings.Join(p, " "))
		}
		return fmt.Sprintf("CREATE TABLE \"%s\" (%s);", tableName, strings.Join(parts, ", ")), nil
	}
	return "", fmt.Errorf("不支持的数据库类型")
}

// BuildDropTable generates a DROP TABLE statement (validated table name).
func (b *SQLBuilder) BuildDropTable(tableName string) (string, error) {
	if !isValidTableName(tableName) {
		return "", fmt.Errorf("无效的表名")
	}
	switch b.dbType {
	case DBTypeMySQL:
		return fmt.Sprintf("DROP TABLE `%s`;", tableName), nil
	case DBTypePostgreSQL:
		return fmt.Sprintf("DROP TABLE \"%s\";", tableName), nil
	}
	return "", fmt.Errorf("不支持的数据库类型")
}

// BuildCreateDatabase generates a CREATE DATABASE statement (validated name
// and charset).
func (b *SQLBuilder) BuildCreateDatabase(name string, charset string) (string, error) {
	if !isValidDBName(name) {
		return "", fmt.Errorf("invalid database name")
	}
	if charset == "" {
		charset = defaultCharset
	}
	if !isValidCharset(charset) {
		return "", fmt.Errorf("invalid charset: %s", charset)
	}
	switch b.dbType {
	case DBTypeMySQL:
		return fmt.Sprintf("CREATE DATABASE `%s` CHARACTER SET %s;", name, charset), nil
	case DBTypePostgreSQL:
		encoding := "UTF8"
		if charset == "latin1" {
			encoding = "LATIN1"
		}
		return fmt.Sprintf(`CREATE DATABASE "%s" ENCODING '%s';`, name, encoding), nil
	}
	return "", fmt.Errorf("unsupported db type: %s", b.dbType)
}

// BuildDropDatabase generates a DROP DATABASE statement (validated name).
func (b *SQLBuilder) BuildDropDatabase(name string) (string, error) {
	if !isValidDBName(name) {
		return "", fmt.Errorf("invalid database name")
	}
	switch b.dbType {
	case DBTypeMySQL:
		return fmt.Sprintf("DROP DATABASE `%s`;", name), nil
	case DBTypePostgreSQL:
		return fmt.Sprintf(`DROP DATABASE "%s";`, name), nil
	}
	return "", fmt.Errorf("unsupported db type: %s", b.dbType)
}

// BuildCreateUser generates a CREATE USER statement (validated username/host;
// empty host falls back to localhost).
func (b *SQLBuilder) BuildCreateUser(username, password, host string) (string, error) {
	if !isValidUsername(username) {
		return "", fmt.Errorf("invalid username: only alphanumeric, underscore, hyphen, dot allowed (max %d chars)", maxUsernameLen)
	}
	if host == "" {
		host = "localhost"
	}
	if !isValidHost(host) {
		return "", fmt.Errorf("invalid host")
	}
	switch b.dbType {
	case DBTypeMySQL:
		return fmt.Sprintf("CREATE USER '%s'@'%s' IDENTIFIED BY '%s';",
			b.EscapeString(username), b.EscapeString(host), b.EscapeString(password)), nil
	case DBTypePostgreSQL:
		return fmt.Sprintf(`CREATE USER "%s" WITH PASSWORD '%s';`,
			strings.ReplaceAll(username, `"`, `""`), b.EscapeString(password)), nil
	}
	return "", fmt.Errorf("unsupported db type: %s", b.dbType)
}

// BuildDropUser generates a DROP USER statement (validated username; host is
// checked for MySQL, where users are address-scoped).
func (b *SQLBuilder) BuildDropUser(username, host string) (string, error) {
	if !isValidUsername(username) {
		return "", fmt.Errorf("invalid username")
	}
	switch b.dbType {
	case DBTypeMySQL:
		if !isValidHost(host) {
			return "", fmt.Errorf("invalid host")
		}
		return fmt.Sprintf("DROP USER '%s'@'%s';", b.EscapeString(username), b.EscapeString(host)), nil
	case DBTypePostgreSQL:
		return fmt.Sprintf(`DROP USER "%s";`, strings.ReplaceAll(username, `"`, `""`)), nil
	}
	return "", fmt.Errorf("unsupported db type: %s", b.dbType)
}

// BuildGrant generates a GRANT statement. Database name and privileges are
// validated here — invalid input yields an error, not a statement.
func (b *SQLBuilder) BuildGrant(privileges, database, username, host string) (string, error) {
	if !isValidDBName(database) {
		return "", fmt.Errorf("invalid database name")
	}
	// ValidatePrivileges is the per-engine whitelist (MySQL and PG differ, e.g.
	// INDEX exists only for MySQL).
	validatedPrivs := ValidatePrivileges(b.dbType, privileges)
	if validatedPrivs == "" {
		return "", fmt.Errorf("invalid privileges: %s", privileges)
	}

	switch b.dbType {
	case DBTypeMySQL:
		return fmt.Sprintf("GRANT %s ON %s.* TO '%s'@'%s'; FLUSH PRIVILEGES;",
			validatedPrivs, b.QuoteIdentifier(database), b.EscapeString(username), b.EscapeString(host)), nil
	case DBTypePostgreSQL:
		return fmt.Sprintf(`GRANT %s ON DATABASE %s TO %s;`,
			validatedPrivs, b.QuoteIdentifier(database), b.QuoteIdentifier(username)), nil
	}
	return "", fmt.Errorf("unsupported db type: %s", b.dbType)
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

// --- describe 结果解析 ---

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

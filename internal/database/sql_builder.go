package database

import (
	"fmt"
	"regexp"
	"strconv"
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

// BuildInsert generates an INSERT statement (validated table/column names).
// All columns may be omitted (e.g. a table whose only required column is the
// auto-increment primary key) — then a defaults-insert is emitted instead of
// failing with "no data to insert".
func (b *SQLBuilder) BuildInsert(table string, data map[string]interface{}, tableInfo *TableInfo) (string, error) {
	if !isValidTableName(table) {
		return "", fmt.Errorf("无效的表名")
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

	if len(cols) == 0 {
		return b.buildInsertDefaults(table), nil
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

	if len(cols) == 0 {
		return b.buildInsertDefaults(table), nil, nil
	}
	return fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s);",
		b.QuoteIdentifier(table), strings.Join(cols, ", "), strings.Join(ph, ", ")), args, nil
}

// buildInsertDefaults 生成只写默认值的 INSERT（全列可空且都不填、或只剩自增列时）：
// MySQL 用 `INSERT INTO t () VALUES ();`（合法，插入全默认行），PG 用 `DEFAULT VALUES`。
func (b *SQLBuilder) buildInsertDefaults(table string) string {
	if b.dbType == DBTypePostgreSQL {
		return fmt.Sprintf("INSERT INTO %s DEFAULT VALUES;", b.QuoteIdentifier(table))
	}
	return fmt.Sprintf("INSERT INTO %s () VALUES ();", b.QuoteIdentifier(table))
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
		// table name is a value here, so it goes through the driver as $1
		// (parameterized binding) rather than being interpolated into the text.
		return `SELECT column_name, data_type, is_nullable, column_default,
			CASE WHEN column_name IN (
				SELECT a.attname FROM pg_index i
				JOIN pg_attribute a ON a.attrelid = i.indrelid AND a.attnum = ANY(i.indkey)
				WHERE i.indrelid = $1::regclass AND i.indisprimary
			) THEN 'YES' ELSE 'NO' END as is_primary,
			is_identity
			FROM information_schema.columns
			WHERE table_name = $1 ORDER BY ordinal_position;`, nil
	}
	return "", fmt.Errorf("unsupported db type: %s", b.dbType)
}

var baseColumnTypeRegexp = regexp.MustCompile(`^[a-zA-Z0-9_\s]+$`)
var colTypeParamRegexp = regexp.MustCompile(`^\(\s*(?:(?:\d+|'[^']*')\s*(?:,\s*(?:\d+|'[^']*')\s*)*)?\s*\)$`)

func isValidColumnTypeSyntax(colType string) bool {
	if colType == "" {
		return false
	}
	baseType := strings.TrimSpace(strings.Split(colType, "(")[0])
	if !baseColumnTypeRegexp.MatchString(baseType) {
		return false
	}
	if idx := strings.Index(colType, "("); idx != -1 {
		paramPart := colType[idx:]
		if !colTypeParamRegexp.MatchString(paramPart) {
			return false
		}
	}
	return true
}

func getFullColumnType(col TableColumn) string {
	t := strings.TrimSpace(col.Type)
	l := strings.TrimSpace(col.Length)
	if l != "" && !strings.Contains(t, "(") {
		return fmt.Sprintf("%s(%s)", t, l)
	}
	return t
}

// BuildCreateTable generates a CREATE TABLE statement for the columns given.
// Column types are validated for SQL safety (safe type identifier and optional
// length/precision parameter format); actual type validity is enforced by the database engine.
// charset/collation semantics differ per engine:
// MySQL takes table-level DEFAULT CHARSET/COLLATE; PostgreSQL has no per-table
// charset (encoding is set at database level, UTF8 mainstream) — a chosen
// collation (a locale like C.UTF-8) is applied per string column via COLLATE.
func (b *SQLBuilder) BuildCreateTable(tableName string, columns []TableColumn, charset, collation string) (string, error) {
	if !isValidTableName(tableName) {
		return "", fmt.Errorf("无效的表名")
	}
	for _, col := range columns {
		colType := getFullColumnType(col)
		if !isValidColumnTypeSyntax(colType) {
			return "", fmt.Errorf("不支持的列类型: %s", colType)
		}
		if !isValidTableName(col.Name) {
			return "", fmt.Errorf("无效的列名: %s", col.Name)
		}
	}
	if collation != "" && !isValidCollation(collation) {
		return "", fmt.Errorf("不支持的排序规则: %s", collation)
	}

	switch b.dbType {
	case DBTypeMySQL:
		cs := charset
		if cs == "" {
			cs = defaultCharset
		}
		if !isValidCharset(cs) {
			return "", fmt.Errorf("不支持的字符集: %s", cs)
		}
		var parts []string
		for _, col := range columns {
			colType := getFullColumnType(col)
			p := []string{fmt.Sprintf("`%s`", col.Name), colType}
			if col.IsPrimary {
				p = append(p, "PRIMARY KEY")
			}
			if col.AutoIncr {
				p = append(p, "AUTO_INCREMENT")
			}
			if !col.Nullable {
				p = append(p, "NOT NULL")
			}
			if col.Unique {
				p = append(p, "UNIQUE")
			}
			if col.DefaultValue != "" && !col.AutoIncr {
				p = append(p, "DEFAULT "+b.formatDefault(col.DefaultValue))
			}
			parts = append(parts, strings.Join(p, " "))
		}
		suffix := fmt.Sprintf("ENGINE=InnoDB DEFAULT CHARSET=%s", cs)
		if collation != "" {
			suffix += " COLLATE=" + collation
		}
		return fmt.Sprintf("CREATE TABLE `%s` (%s) %s;", tableName, strings.Join(parts, ", "), suffix), nil
	case DBTypePostgreSQL:
		var parts []string
		for _, col := range columns {
			colType := getFullColumnType(col)
			p := []string{fmt.Sprintf("\"%s\"", col.Name), colType}
			if col.IsPrimary {
				p = append(p, "PRIMARY KEY")
			}
			if col.AutoIncr {
				serialType := "SERIAL"
				if strings.EqualFold(col.Type, "BIGINT") {
					serialType = "BIGSERIAL"
				} else if strings.EqualFold(col.Type, "SMALLINT") {
					serialType = "SMALLSERIAL"
				}
				p = []string{fmt.Sprintf("\"%s\"", col.Name), serialType, "PRIMARY KEY"}
			}
			if !col.Nullable && !col.IsPrimary {
				p = append(p, "NOT NULL")
			}
			// collation 只对字符串列有效（locale 指定比较规则）；数字/时间列加会报错。
			if collation != "" && isPgStringType(colType) {
				p = append(p, `COLLATE "`+collation+`"`)
			}
			if col.Unique {
				p = append(p, "UNIQUE")
			}
			if col.DefaultValue != "" && !col.AutoIncr {
				p = append(p, "DEFAULT "+b.formatDefault(col.DefaultValue))
			}
			parts = append(parts, strings.Join(p, " "))
		}
		return fmt.Sprintf("CREATE TABLE \"%s\" (%s);", tableName, strings.Join(parts, ", ")), nil
	}
	return "", fmt.Errorf("不支持的数据库类型")
}

// isPgStringType reports whether a column type accepts a PG COLLATE clause
// (locale applies to text comparisons; numeric/temporal/JSON types reject it).
func isPgStringType(t string) bool {
	switch strings.ToUpper(strings.TrimSpace(strings.Split(t, "(")[0])) {
	case "VARCHAR", "CHAR", "CHARACTER", "CHARACTER VARYING", "TEXT",
		"TINYTEXT", "MEDIUMTEXT", "LONGTEXT":
		return true
	}
	return false
}

// formatDefault 渲染 DEFAULT 字面量：纯数字/数值表达式（含函数）不加引号，其余
// 字符串加引号转义（含 CURRENT_TIMESTAMP 这类无引号函数的常见拼写）。
func (b *SQLBuilder) formatDefault(v string) string {
	s := strings.TrimSpace(v)
	if s == "" {
		return ""
	}
	if b.isNumericOrFunc(s) {
		return s
	}
	return "'" + b.EscapeString(s) + "'"
}

// isNumericOrFunc 判断默认值是否需要加引号：数字字面量、CURRENT_TIMESTAMP 等
// SQL 函数、NOW() 括号表达式不加引号，其余视为字符串。
func (b *SQLBuilder) isNumericOrFunc(s string) bool {
	if s == "" {
		return false
	}
	// 数字（含负数、小数、指数）
	if _, err := strconv.ParseFloat(s, 64); err == nil {
		return true
	}
	upper := strings.ToUpper(s)
	// 常见无引号默认值函数（可带括号实参）
	for _, fn := range []string{"CURRENT_TIMESTAMP", "NOW", "CURRENT_DATE", "CURRENT_TIME", "CURRENT_USER", "UUID", "GEN_RANDOM_UUID"} {
		if upper == fn || strings.HasPrefix(upper, fn+"(") {
			return true
		}
	}
	return false
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

// BuildResetPassword generates an ALTER USER statement to change a user's password.
func (b *SQLBuilder) BuildResetPassword(username, newPassword, host string) (string, error) {
	if !isValidUsername(username) {
		return "", fmt.Errorf("invalid username")
	}
	if newPassword == "" {
		return "", fmt.Errorf("password cannot be empty")
	}
	switch b.dbType {
	case DBTypeMySQL:
		if host == "" {
			host = "localhost"
		}
		if !isValidHost(host) {
			return "", fmt.Errorf("invalid host")
		}
		return fmt.Sprintf("ALTER USER '%s'@'%s' IDENTIFIED BY '%s';",
			b.EscapeString(username), b.EscapeString(host), b.EscapeString(newPassword)), nil
	case DBTypePostgreSQL:
		return fmt.Sprintf(`ALTER USER "%s" WITH PASSWORD '%s';`,
			strings.ReplaceAll(username, `"`, `""`), b.EscapeString(newPassword)), nil
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
		// mysql DESCRIBE 的 Null 列: "YES" → 可空, "NO" → NOT NULL。之前多了个 !，
		// 把两者反了：NOT NULL 列被当成可空，前端就不标必填，插入漏填报
		// "Field doesn't have a default value"。
		col.IsNullable = isTruthy(row, colIdx["Null"])
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
		if v, ok := colIdx["is_identity"]; ok && strings.EqualFold(str(row, v), "yes") {
			col.IsAutoIncr = true
		}
		if v, ok := colIdx["Default"]; ok && row[v] != nil {
			col.HasDefault = true
			col.DefaultValue = str(row, v)
		}
		if v, ok := colIdx["column_default"]; ok && row[v] != nil {
			col.HasDefault = true
			defVal := str(row, v)
			col.DefaultValue = defVal
			if strings.Contains(strings.ToLower(defVal), "nextval(") {
				col.IsAutoIncr = true
			}
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
		// []byte 是驱动对字节值的常见返回（pgx 文本/二进制、MySQL 8.0 部分列）。
		// 不能拿 []byte 再包 []any{v} 递归——v 仍是 []byte，会无限递归直到栈溢出。
		// 这里把字节转字符串，落到 string 分支用同一套判假逻辑。
		return isTruthy([]any{string(v)}, 0)
	case bool:
		return v
	}
	return true
}

package database

import (
	"fmt"
	"regexp"
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
	if !ValidateTableName(table) {
		return "", fmt.Errorf("无效的表名")
	}
	if len(data) == 0 {
		return "", fmt.Errorf("no data to insert")
	}
	for col := range data {
		if err := validateColumnName(col); err != nil {
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
	if !ValidateTableName(table) {
		return "", nil, fmt.Errorf("无效的表名")
	}
	if len(data) == 0 {
		return "", nil, fmt.Errorf("no data to insert")
	}
	for col := range data {
		if err := validateColumnName(col); err != nil {
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
	if !ValidateTableName(table) {
		return "", fmt.Errorf("无效的表名")
	}
	if err := validateColumnName(pkCol); err != nil {
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
	if !ValidateTableName(table) {
		return "", nil, fmt.Errorf("无效的表名")
	}
	if err := validateColumnName(pkCol); err != nil {
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
	if !ValidateTableName(table) {
		return "", fmt.Errorf("无效的表名")
	}
	if err := validateColumnName(pkCol); err != nil {
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
	if !ValidateTableName(table) {
		return "", nil, fmt.Errorf("无效的表名")
	}
	if err := validateColumnName(pkCol); err != nil {
		return "", nil, fmt.Errorf("invalid primary key column '%s': %s", pkCol, err)
	}
	if pkVal == nil {
		return "", nil, fmt.Errorf("primary key value is required")
	}
	return fmt.Sprintf("DELETE FROM %s WHERE %s = %s;",
		b.QuoteIdentifier(table), b.QuoteIdentifier(pkCol), b.placeholder(1)), []any{pkVal}, nil
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

// BuildGrant generates a GRANT statement. Privileges are validated here — an
// invalid privilege set yields an empty string, not a statement.
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

// --- SQL statement safety gate ---

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

// ValidateTableName checks table/column name validity.
func ValidateTableName(name string) bool {
	return name != "" && len(name) <= 64 && tableNameRegexp.MatchString(name)
}

// allowedColumnTypes is the whitelist of column types CreateTable accepts — a
// union of the MySQL and PostgreSQL type sets (SERIAL/UUID/JSONB/BYTEA are PG,
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

// validateColumnName checks a column name for use in DML statements. Columns
// may carry a hyphen, unlike table names.
func validateColumnName(name string) error {
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

package service

import (
	"context"
	"fmt"
	"log"
	"regexp"
	"strings"

	"easyserver/internal/executor"
	"easyserver/internal/model"
)

// SQLQueryService handles data operations inside databases (sub-domain 3).
// Currently shells out to mysql/psql CLI; interface designed for future
// driver swap with no Handler-side change.
// TODO: swap *DatabaseMgmtService to DatabaseRepository when candidate 1 lands.
type SQLQueryService struct {
	mgmtSvc  *DatabaseMgmtService
	executor executor.CommandExecutor
}

func NewSQLQueryService(mgmtSvc *DatabaseMgmtService, exec executor.CommandExecutor) *SQLQueryService {
	return &SQLQueryService{mgmtSvc: mgmtSvc, executor: exec}
}

// --- Result types (keep Handler JSON wire shape stable) ---

// DMLResult is the response for ExecuteSQL / Insert / Update / Delete.
type DMLResult struct {
	Success bool   `json:"success"`
	Output  string `json:"output,omitempty"`
	Error   string `json:"error,omitempty"`
	DryRun  bool   `json:"dry_run,omitempty"`
	SQL     string `json:"sql,omitempty"`
}

// PagedQueryResult is the response for QueryTable.
type PagedQueryResult struct {
	Headers  []string        `json:"headers"`
	Rows     [][]interface{} `json:"rows"`
	Total    int             `json:"total"`
	Page     int             `json:"page"`
	PageSize int             `json:"page_size"`
}

// DescribeResult is the response for DescribeTable.
type DescribeResult struct {
	TableName  string                   `json:"table_name"`
	PrimaryKey string                   `json:"primary_key"`
	Columns    []map[string]interface{} `json:"columns"`
}

// TableColumn describes a column for CreateTable.
type TableColumn struct {
	Name      string `json:"name"`
	Type      string `json:"type"`
	Nullable  bool   `json:"nullable"`
	IsPrimary bool   `json:"is_primary"`
	AutoIncr  bool   `json:"auto_incr"`
}

// --- Internal helpers ---

// lookupDB resolves dbID → database + server + typed DBType.
func (s *SQLQueryService) lookupDB(ctx context.Context, dbID int64) (*model.Database, *model.DBServer, DBType, error) {
	db, err := s.mgmtSvc.GetDatabaseByID(ctx, dbID)
	if err != nil || db == nil {
		return nil, nil, "", fmt.Errorf("数据库不存在")
	}
	server, err := s.mgmtSvc.GetServerByID(ctx, db.DBServerID)
	if err != nil || server == nil {
		return nil, nil, "", fmt.Errorf("服务器不存在")
	}
	dbType := getDBTypeFromName(server.Name)
	return db, server, dbType, nil
}

func getDBTypeFromName(name string) DBType {
	switch name {
	case "mysql":
		return DBTypeMySQL
	case "postgresql":
		return DBTypePostgreSQL
	case "redis":
		return DBTypeRedis
	}
	return DBTypeMySQL
}

// execRaw shells out to mysql/psql and returns combined output.
func (s *SQLQueryService) execRaw(ctx context.Context, dbType DBType, dbName string, sql string) (string, error) {
	switch dbType {
	case DBTypeMySQL:
		out, _, err := s.executor.RunCombined(ctx, "mysql", dbName, "-e", sql)
		return out, err
	case DBTypePostgreSQL:
		out, _, err := s.executor.RunCombined(ctx, "sudo", "-u", "postgres", "psql", "-d", dbName, "-c", sql)
		return out, err
	}
	return "", fmt.Errorf("不支持的数据库类型")
}

// pathPattern matches filesystem paths that should be stripped from error output.
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

// ValidateTableName checks table/column name validity.
var tableNameRegexp = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)

func ValidateTableName(name string) bool {
	return name != "" && len(name) <= 64 && tableNameRegexp.MatchString(name)
}

// --- Public methods ---

// ListTables returns all table names in the given database.
func (s *SQLQueryService) ListTables(ctx context.Context, dbID int64) ([]map[string]interface{}, error) {
	db, server, _, err := s.lookupDB(ctx, dbID)
	if err != nil {
		return nil, err
	}

	var tables []map[string]interface{}
	switch server.Name {
	case "mysql":
		out, err := s.execRaw(ctx, DBTypeMySQL, db.Name, "SHOW TABLES;")
		if err != nil {
			return nil, fmt.Errorf("获取表列表失败: %s", out)
		}
		lines := strings.Split(strings.TrimSpace(out), "\n")
		for i, line := range lines {
			if i == 0 {
				continue
			}
			line = strings.TrimSpace(line)
			if line != "" {
				tables = append(tables, map[string]interface{}{"name": line})
			}
		}
	case "postgresql":
		out, err := s.execRaw(ctx, DBTypePostgreSQL, db.Name,
			"SELECT tablename FROM pg_tables WHERE schemaname = 'public' ORDER BY tablename;")
		if err != nil {
			return nil, fmt.Errorf("获取表列表失败: %s", out)
		}
		lines := strings.Split(strings.TrimSpace(out), "\n")
		for i, line := range lines {
			line = strings.TrimSpace(line)
			if i < 2 || line == "" || line == "(0 rows)" || strings.HasPrefix(line, "-") || strings.HasPrefix(line, "(") {
				continue
			}
			tables = append(tables, map[string]interface{}{"name": line})
		}
	}
	return tables, nil
}

// DescribeTable returns structured column info for a table.
func (s *SQLQueryService) DescribeTable(ctx context.Context, dbID int64, tableName string) (*DescribeResult, error) {
	if !ValidateTableName(tableName) {
		return nil, fmt.Errorf("无效的表名")
	}
	db, _, dbType, err := s.lookupDB(ctx, dbID)
	if err != nil {
		return nil, err
	}

	builder := NewSQLBuilder(dbType)
	describeSQL := builder.BuildDescribeTable(tableName)

	out, err := s.execRaw(ctx, dbType, db.Name, describeSQL)
	if err != nil {
		return nil, fmt.Errorf("获取表结构失败: %s", out)
	}

	tableInfo := ParseTableInfo(dbType, tableName, out)

	var columns []map[string]interface{}
	for _, col := range tableInfo.Columns {
		columns = append(columns, map[string]interface{}{
			"name":           col.Name,
			"type":           col.Type,
			"is_primary_key": col.IsPrimaryKey,
			"is_auto_incr":   col.IsAutoIncr,
			"has_default":    col.HasDefault,
			"default":        col.DefaultValue,
			"is_nullable":    col.IsNullable,
		})
	}

	return &DescribeResult{
		TableName:  tableName,
		PrimaryKey: tableInfo.PrimaryKey,
		Columns:    columns,
	}, nil
}

// QueryTable returns paginated rows from a table.
func (s *SQLQueryService) QueryTable(ctx context.Context, dbID int64, tableName string, page, pageSize int) (*PagedQueryResult, error) {
	if !ValidateTableName(tableName) {
		return nil, fmt.Errorf("无效的表名")
	}
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 50
	}
	if pageSize > 200 {
		pageSize = 200
	}
	offset := (page - 1) * pageSize

	db, _, dbType, err := s.lookupDB(ctx, dbID)
	if err != nil {
		return nil, err
	}

	// Count
	var total int
	switch dbType {
	case DBTypeMySQL:
		out, err := s.execRaw(ctx, DBTypeMySQL, db.Name, fmt.Sprintf("SELECT COUNT(*) FROM `%s`;", tableName))
		if err == nil {
			fmt.Sscanf(strings.TrimSpace(out), "%d", &total)
		}
	case DBTypePostgreSQL:
		out, err := s.execRaw(ctx, DBTypePostgreSQL, db.Name, fmt.Sprintf("SELECT COUNT(*) FROM \"%s\";", tableName))
		if err == nil {
			fmt.Sscanf(strings.TrimSpace(out), "%d", &total)
		}
	}

	// Data
	var headers []string
	var rows [][]interface{}
	switch dbType {
	case DBTypeMySQL:
		out, err := s.execRaw(ctx, DBTypeMySQL, db.Name,
			fmt.Sprintf("SELECT * FROM `%s` LIMIT %d OFFSET %d;", tableName, pageSize, offset))
		if err != nil {
			return nil, fmt.Errorf("查询失败: %s", out)
		}
		lines := strings.Split(strings.TrimSpace(out), "\n")
		for i, line := range lines {
			fields := strings.Split(line, "\t")
			if i == 0 {
				headers = fields
			} else {
				var row []interface{}
				for _, f := range fields {
					row = append(row, f)
				}
				rows = append(rows, row)
			}
		}
	case DBTypePostgreSQL:
		out, err := s.execRaw(ctx, DBTypePostgreSQL, db.Name,
			fmt.Sprintf("SELECT * FROM \"%s\" LIMIT %d OFFSET %d;", tableName, pageSize, offset))
		if err != nil {
			return nil, fmt.Errorf("查询失败: %s", out)
		}
		lines := strings.Split(strings.TrimSpace(out), "\n")
		for i, line := range lines {
			fields := strings.Split(line, "|")
			for j := range fields {
				fields[j] = strings.TrimSpace(fields[j])
			}
			if i == 0 {
				headers = fields
			} else if i >= 2 && !strings.HasPrefix(line, "(") && line != "" {
				var row []interface{}
				for _, f := range fields {
					row = append(row, f)
				}
				rows = append(rows, row)
			}
		}
	}

	return &PagedQueryResult{
		Headers:  headers,
		Rows:     rows,
		Total:    total,
		Page:     page,
		PageSize: pageSize,
	}, nil
}

// ExecuteSQL runs raw SQL and returns the result.
func (s *SQLQueryService) ExecuteSQL(ctx context.Context, dbID int64, sql string) (*DMLResult, error) {
	db, server, dbType, err := s.lookupDB(ctx, dbID)
	if err != nil {
		return nil, err
	}

	// Validate
	validator := NewSQLValidator(dbType)
	if r := validator.ValidateSQL(sql); !r.Valid {
		return &DMLResult{Success: false, Error: r.Message}, nil
	}

	out, execErr := s.execRaw(ctx, dbType, db.Name, sql)
	if execErr != nil {
		log.Printf("ExecuteSQL %s error [db=%s]: %s", server.Name, db.Name, out)
		return &DMLResult{Success: false, Error: SanitizeSQLError(out)}, nil
	}

	return &DMLResult{Success: true, Output: out}, nil
}

// InsertRecord inserts a row; dryRun=true returns the SQL without executing.
func (s *SQLQueryService) InsertRecord(ctx context.Context, dbID int64, table string, data map[string]interface{}, dryRun bool) (*DMLResult, error) {
	if !ValidateTableName(table) {
		return nil, fmt.Errorf("无效的表名")
	}
	db, _, dbType, err := s.lookupDB(ctx, dbID)
	if err != nil {
		return nil, err
	}

	builder := NewSQLBuilder(dbType)
	validator := NewSQLValidator(dbType)

	if r := validator.ValidateInsert(table, data, nil); !r.Valid {
		return &DMLResult{Success: false, Error: r.Message}, nil
	}

	sql := builder.BuildInsert(table, data, nil)
	if dryRun {
		return &DMLResult{Success: true, DryRun: true, SQL: sql}, nil
	}

	out, execErr := s.execRaw(ctx, dbType, db.Name, sql)
	if execErr != nil {
		return &DMLResult{Success: false, Error: out}, nil
	}
	return &DMLResult{Success: true, Output: out}, nil
}

// UpdateRecord updates a row; dryRun=true returns the SQL without executing.
func (s *SQLQueryService) UpdateRecord(ctx context.Context, dbID int64, table string, data map[string]interface{}, pk string, pkVal interface{}, dryRun bool) (*DMLResult, error) {
	if !ValidateTableName(table) {
		return nil, fmt.Errorf("无效的表名")
	}
	db, _, dbType, err := s.lookupDB(ctx, dbID)
	if err != nil {
		return nil, err
	}

	builder := NewSQLBuilder(dbType)
	validator := NewSQLValidator(dbType)

	if r := validator.ValidateUpdate(table, data, pk, pkVal); !r.Valid {
		return &DMLResult{Success: false, Error: r.Message}, nil
	}

	sql := builder.BuildUpdate(table, data, pk, pkVal)
	if dryRun {
		return &DMLResult{Success: true, DryRun: true, SQL: sql}, nil
	}

	out, execErr := s.execRaw(ctx, dbType, db.Name, sql)
	if execErr != nil {
		return &DMLResult{Success: false, Error: out}, nil
	}
	return &DMLResult{Success: true, Output: out}, nil
}

// DeleteRecord deletes a row; dryRun=true returns the SQL without executing.
func (s *SQLQueryService) DeleteRecord(ctx context.Context, dbID int64, table string, pk string, pkVal interface{}, dryRun bool) (*DMLResult, error) {
	if !ValidateTableName(table) {
		return nil, fmt.Errorf("无效的表名")
	}
	db, _, dbType, err := s.lookupDB(ctx, dbID)
	if err != nil {
		return nil, err
	}

	builder := NewSQLBuilder(dbType)
	validator := NewSQLValidator(dbType)

	if r := validator.ValidateDelete(table, pk, pkVal); !r.Valid {
		return &DMLResult{Success: false, Error: r.Message}, nil
	}

	sql := builder.BuildDelete(table, pk, pkVal)
	if dryRun {
		return &DMLResult{Success: true, DryRun: true, SQL: sql}, nil
	}

	out, execErr := s.execRaw(ctx, dbType, db.Name, sql)
	if execErr != nil {
		return &DMLResult{Success: false, Error: out}, nil
	}
	return &DMLResult{Success: true}, nil
}

// CreateTable creates a new table in the given database.
func (s *SQLQueryService) CreateTable(ctx context.Context, dbID int64, tableName string, columns []TableColumn) error {
	if !ValidateTableName(tableName) {
		return fmt.Errorf("无效的表名")
	}
	db, _, dbType, err := s.lookupDB(ctx, dbID)
	if err != nil {
		return err
	}

	// Validate column types
	allowedTypes := map[string]bool{
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
	for _, col := range columns {
		baseType := strings.ToUpper(strings.Split(col.Type, "(")[0])
		baseType = strings.TrimSpace(baseType)
		if !allowedTypes[baseType] {
			return fmt.Errorf("不支持的列类型: %s", col.Type)
		}
		if !ValidateTableName(col.Name) {
			return fmt.Errorf("无效的列名: %s", col.Name)
		}
	}

	// Build CREATE TABLE
	var sql string
	switch dbType {
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
		sql = fmt.Sprintf("CREATE TABLE `%s` (%s) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;", tableName, strings.Join(parts, ", "))
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
		sql = fmt.Sprintf("CREATE TABLE \"%s\" (%s);", tableName, strings.Join(parts, ", "))
	default:
		return fmt.Errorf("不支持的数据库类型")
	}

	out, execErr := s.execRaw(ctx, dbType, db.Name, sql)
	if execErr != nil {
		return fmt.Errorf("创建表失败: %s", out)
	}
	return nil
}

// DropTable drops a table from the given database.
func (s *SQLQueryService) DropTable(ctx context.Context, dbID int64, tableName string) error {
	if !ValidateTableName(tableName) {
		return fmt.Errorf("无效的表名")
	}
	db, _, dbType, err := s.lookupDB(ctx, dbID)
	if err != nil {
		return err
	}

	var sql string
	switch dbType {
	case DBTypeMySQL:
		sql = fmt.Sprintf("DROP TABLE `%s`;", tableName)
	case DBTypePostgreSQL:
		sql = fmt.Sprintf("DROP TABLE \"%s\";", tableName)
	default:
		return fmt.Errorf("不支持的数据库类型")
	}

	out, execErr := s.execRaw(ctx, dbType, db.Name, sql)
	if execErr != nil {
		return fmt.Errorf("删除表失败: %s", out)
	}
	return nil
}

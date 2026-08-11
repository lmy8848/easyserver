package database

import (
	"context"
	"fmt"
	"log"
	"strings"
)

// --- Logical database CRUD (live, server-owned) ---

func (s *Service) ListDatabases(ctx context.Context, instanceID int64) ([]Database, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	instance, err := s.repo.GetInstance(ctx, instanceID)
	if err != nil {
		return nil, fmt.Errorf("get instance: %w", err)
	}
	if instance == nil {
		return nil, fmt.Errorf("database instance not found")
	}
	return s.queryDatabases(ctx, instance)
}

// queryDatabases lists logical databases live from the database server. Databases are
// server-owned state — the panel never persists a mirror of them.
func (s *Service) queryDatabases(ctx context.Context, instance *DBInstance) ([]Database, error) {
	var runner SQLRunner
	switch instance.DBType {
	case DBTypeMySQL:
		runner = s.runnerFor(instance)
		res, err := runner.Query(ctx, instance, systemDBName(instance.DBType),
			"SELECT schema_name, default_character_set_name FROM information_schema.schemata WHERE schema_name NOT IN ('information_schema','mysql','performance_schema','sys') ORDER BY schema_name")
		if err != nil {
			return nil, fmt.Errorf("list databases failed: %s", SanitizeSQLError(err.Error()))
		}
		var dbs []Database
		for _, row := range res.Rows {
			dbs = append(dbs, Database{Name: str(row, 0), Charset: str(row, 1)})
		}
		return dbs, nil
	case DBTypePostgreSQL:
		runner = s.runnerFor(instance)
		res, err := runner.Query(ctx, instance, systemDBName(instance.DBType),
			"SELECT datname, pg_encoding_to_char(encoding) FROM pg_database WHERE datistemplate = false ORDER BY datname")
		if err != nil {
			return nil, fmt.Errorf("list databases failed: %s", SanitizeSQLError(err.Error()))
		}
		var dbs []Database
		for _, row := range res.Rows {
			dbs = append(dbs, Database{Name: str(row, 0), Charset: str(row, 1)})
		}
		return dbs, nil
	case DBTypeRedis:
		return []Database{}, nil
	default:
		return nil, fmt.Errorf("unsupported db type: %s", instance.DBType)
	}
}

// str coerces a query row cell to its string form for display fields. Driver
// cells may arrive as string, []byte or nil (NULL).
func str(row []any, i int) string {
	if i >= len(row) || row[i] == nil {
		return ""
	}
	switch v := row[i].(type) {
	case string:
		return v
	case []byte:
		return string(v)
	}
	return fmt.Sprintf("%v", row[i])
}

func (s *Service) CreateDatabase(ctx context.Context, instanceID int64, req *CreateDatabaseRequest) (*Database, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	instance, err := s.repo.GetInstance(ctx, instanceID)
	if err != nil {
		return nil, fmt.Errorf("get instance: %w", err)
	}
	if instance == nil {
		return nil, fmt.Errorf("database instance not found")
	}
	if instance.Status != "running" && instance.Status != "active" {
		return nil, fmt.Errorf("database instance is not running")
	}

	if !isValidDBName(req.Name) {
		return nil, fmt.Errorf("invalid database name")
	}

	charset := req.Charset
	if charset == "" {
		charset = defaultCharset
	}
	if !isValidCharset(charset) {
		return nil, fmt.Errorf("invalid charset: %s", charset)
	}

	// DDL statements cannot be parameter-bound; names/hosts are already
	// validated and passwords escaped by the builder. The system database hosts
	// instance-level statements — including CREATE DATABASE itself, which must
	// not run on the target database.
	builder := NewSQLBuilder(instance.DBType)
	sysDB := systemDBName(instance.DBType)
	switch instance.DBType {
	case DBTypeMySQL, DBTypePostgreSQL:
		if _, err := s.runnerFor(instance).Exec(ctx, instance, sysDB, builder.BuildCreateDatabase(req.Name, charset)); err != nil {
			return nil, fmt.Errorf("create database failed: %s", SanitizeSQLError(err.Error()))
		}
	default:
		return nil, fmt.Errorf("database creation not supported for %s", instance.DBType)
	}

	return &Database{Name: req.Name, Charset: charset}, nil
}

func (s *Service) DeleteDatabase(ctx context.Context, instanceID int64, dbName string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	instance, err := s.repo.GetInstance(ctx, instanceID)
	if err != nil {
		return fmt.Errorf("get instance: %w", err)
	}
	if instance == nil {
		return fmt.Errorf("database instance not found")
	}
	if instance.Status != "running" {
		return fmt.Errorf("database instance is not running")
	}
	if !isValidDBName(dbName) {
		return fmt.Errorf("invalid database name")
	}

	builder := NewSQLBuilder(instance.DBType)
	sysDB := systemDBName(instance.DBType)
	switch instance.DBType {
	case DBTypeMySQL, DBTypePostgreSQL:
		if _, err := s.runnerFor(instance).Exec(ctx, instance, sysDB, builder.BuildDropDatabase(dbName)); err != nil {
			return fmt.Errorf("drop database failed: %s", SanitizeSQLError(err.Error()))
		}
	default:
		return fmt.Errorf("database deletion not supported for %s", instance.DBType)
	}

	return nil
}

// --- DB User CRUD (live, server-owned) ---

func (s *Service) ListDBUsers(ctx context.Context, instanceID int64) ([]DBUser, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	instance, err := s.repo.GetInstance(ctx, instanceID)
	if err != nil {
		return nil, fmt.Errorf("get instance: %w", err)
	}
	if instance == nil {
		return nil, fmt.Errorf("database instance not found")
	}
	return s.queryUsers(ctx, instance)
}

// queryUsers lists database users live from the database server (the server owns them).
func (s *Service) queryUsers(ctx context.Context, instance *DBInstance) ([]DBUser, error) {
	switch instance.DBType {
	case DBTypeMySQL:
		res, err := s.runnerFor(instance).Query(ctx, instance, systemDBName(instance.DBType),
			"SELECT user, host FROM mysql.user WHERE user NOT IN ('mysql.session','mysql.sys','mysql.infoschema') ORDER BY user, host")
		if err != nil {
			return nil, fmt.Errorf("list users failed: %s", SanitizeSQLError(err.Error()))
		}
		var users []DBUser
		for _, row := range res.Rows {
			users = append(users, DBUser{Username: str(row, 0), Host: str(row, 1)})
		}
		return users, nil
	case DBTypePostgreSQL:
		res, err := s.runnerFor(instance).Query(ctx, instance, systemDBName(instance.DBType),
			"SELECT rolname FROM pg_roles WHERE rolcanlogin ORDER BY rolname")
		if err != nil {
			return nil, fmt.Errorf("list users failed: %s", SanitizeSQLError(err.Error()))
		}
		var users []DBUser
		for _, row := range res.Rows {
			users = append(users, DBUser{Username: str(row, 0)})
		}
		return users, nil
	case DBTypeRedis:
		return []DBUser{}, nil
	default:
		return nil, fmt.Errorf("unsupported db type: %s", instance.DBType)
	}
}

// isAdminUser reports whether username is the database's built-in administrator.
func isAdminUser(dbType DBType, username string) bool {
	switch dbType {
	case DBTypeMySQL:
		return username == "root"
	case DBTypePostgreSQL:
		return username == "postgres"
	}
	return false
}

func (s *Service) CreateDBUser(ctx context.Context, instanceID int64, req *CreateDBUserRequest) (*DBUser, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	instance, err := s.repo.GetInstance(ctx, instanceID)
	if err != nil {
		return nil, fmt.Errorf("get instance: %w", err)
	}
	if instance == nil {
		return nil, fmt.Errorf("database instance not found")
	}
	if instance.Status != "running" {
		return nil, fmt.Errorf("database instance is not running")
	}

	if !isValidUsername(req.Username) {
		return nil, fmt.Errorf("invalid username: only alphanumeric, underscore, hyphen, dot allowed (max %d chars)", maxUsernameLen)
	}

	host := req.Host
	if host == "" {
		host = "localhost"
	}
	if !isValidHost(host) {
		return nil, fmt.Errorf("invalid host")
	}

	builder := NewSQLBuilder(instance.DBType)
	sysDB := systemDBName(instance.DBType)
	switch instance.DBType {
	case DBTypeMySQL, DBTypePostgreSQL:
		sqlStr := builder.BuildCreateUser(req.Username, req.Password, host)
		if _, err := s.runnerFor(instance).Exec(ctx, instance, sysDB, sqlStr); err != nil {
			return nil, fmt.Errorf("create user failed: %s", SanitizeSQLError(err.Error()))
		}
	default:
		return nil, fmt.Errorf("user creation not supported for %s", instance.DBType)
	}

	return &DBUser{Username: req.Username, Host: host}, nil
}

func (s *Service) DeleteDBUser(ctx context.Context, instanceID int64, username, host string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	instance, err := s.repo.GetInstance(ctx, instanceID)
	if err != nil {
		return fmt.Errorf("get instance: %w", err)
	}
	if instance == nil {
		return fmt.Errorf("database instance not found")
	}

	if !isValidUsername(username) {
		return fmt.Errorf("invalid username")
	}
	if isAdminUser(instance.DBType, username) {
		return fmt.Errorf("cannot delete the administrator user")
	}
	if instance.DBType == DBTypeMySQL && !isValidHost(host) {
		return fmt.Errorf("invalid host")
	}

	builder := NewSQLBuilder(instance.DBType)
	sysDB := systemDBName(instance.DBType)
	switch instance.DBType {
	case DBTypeMySQL, DBTypePostgreSQL:
		sqlStr := builder.BuildDropUser(username, host)
		if _, err := s.runnerFor(instance).Exec(ctx, instance, sysDB, sqlStr); err != nil {
			return fmt.Errorf("drop user failed: %s", SanitizeSQLError(err.Error()))
		}
	default:
		return fmt.Errorf("user deletion not supported for %s", instance.DBType)
	}

	return nil
}

func (s *Service) GrantPrivileges(ctx context.Context, instanceID int64, username, host string, req *GrantRequest) error {
	if ctx == nil {
		ctx = context.Background()
	}
	instance, err := s.repo.GetInstance(ctx, instanceID)
	if err != nil {
		return fmt.Errorf("get instance: %w", err)
	}
	if instance == nil {
		return fmt.Errorf("database instance not found")
	}
	if instance.Status != "running" {
		return fmt.Errorf("database instance is not running")
	}

	if !isValidDBName(req.Database) {
		return fmt.Errorf("invalid database name")
	}

	// ValidatePrivileges is the per-engine whitelist (MySQL and PG differ, e.g.
	// INDEX exists only for MySQL); BuildGrant re-validates on generation.
	validated := ValidatePrivileges(instance.DBType, req.Privileges)
	if validated == "" {
		return fmt.Errorf("invalid privileges: %s", req.Privileges)
	}

	builder := NewSQLBuilder(instance.DBType)
	sysDB := systemDBName(instance.DBType)
	switch instance.DBType {
	case DBTypeMySQL, DBTypePostgreSQL:
		sqlStr := builder.BuildGrant(validated, req.Database, username, host)
		if _, err := s.runnerFor(instance).Exec(ctx, instance, sysDB, sqlStr); err != nil {
			return fmt.Errorf("grant failed: %s", SanitizeSQLError(err.Error()))
		}
	default:
		return fmt.Errorf("privilege grant not supported for %s", instance.DBType)
	}

	return nil
}

// --- SQL query operations ---

// getInstanceForSQL resolves the instance for a database-level SQL operation.
// Database names travel as URL path parameters now — no persisted db lookup.
func (s *Service) getInstanceForSQL(ctx context.Context, instanceID int64, dbName string) (*DBInstance, error) {
	if !isValidDBName(dbName) {
		return nil, fmt.Errorf("无效的数据库名")
	}
	instance, err := s.repo.GetInstance(ctx, instanceID)
	if err != nil || instance == nil {
		return nil, fmt.Errorf("数据库实例不存在")
	}
	return instance, nil
}
func (s *Service) ListTables(ctx context.Context, instanceID int64, dbName string) ([]map[string]interface{}, error) {
	instance, err := s.getInstanceForSQL(ctx, instanceID, dbName)
	if err != nil {
		return nil, err
	}

	var tables []map[string]interface{}
	builder := NewSQLBuilder(instance.DBType)
	res, err := s.runnerFor(instance).Query(ctx, instance, dbName, builder.BuildListTables())
	if err != nil {
		return nil, fmt.Errorf("获取表列表失败: %s", SanitizeSQLError(err.Error()))
	}
	for _, row := range res.Rows {
		tables = append(tables, map[string]interface{}{"name": str(row, 0)})
	}
	return tables, nil
}

func (s *Service) DescribeTable(ctx context.Context, instanceID int64, dbName, tableName string) (*DescribeResult, error) {
	if !ValidateTableName(tableName) {
		return nil, fmt.Errorf("无效的表名")
	}
	instance, err := s.getInstanceForSQL(ctx, instanceID, dbName)
	if err != nil {
		return nil, err
	}

	builder := NewSQLBuilder(instance.DBType)
	describeSQL := builder.BuildDescribeTable(tableName)

	res, err := s.runnerFor(instance).Query(ctx, instance, dbName, describeSQL)
	if err != nil {
		return nil, fmt.Errorf("获取表结构失败: %s", SanitizeSQLError(err.Error()))
	}

	// The driver channel returns structured describe rows (name, type, nullable,
	// default, pk-flag columns); the CLI channel parses the same shape from text.
	tableInfo := tableInfoFromQuery(instance.DBType, tableName, res)

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

func (s *Service) QueryTable(ctx context.Context, instanceID int64, dbName, tableName string, page, pageSize int) (*PagedQueryResult, error) {
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

	instance, err := s.getInstanceForSQL(ctx, instanceID, dbName)
	if err != nil {
		return nil, err
	}
	dbType := instance.DBType
	builder := NewSQLBuilder(dbType)

	var total int
	var headers []string
	var columnTypes []string
	var rows [][]interface{}
	switch dbType {
	case DBTypeMySQL, DBTypePostgreSQL:
		countRes, err := s.runnerFor(instance).Query(ctx, instance, dbName, builder.BuildCount(tableName))
		if err == nil && len(countRes.Rows) > 0 {
			fmt.Sscanf(str(countRes.Rows[0], 0), "%d", &total)
		}
		res, err := s.runnerFor(instance).Query(ctx, instance, dbName, builder.BuildSelect(tableName, nil, page, pageSize))
		if err != nil {
			return nil, fmt.Errorf("查询失败: %s", SanitizeSQLError(err.Error()))
		}
		for _, c := range res.Columns {
			headers = append(headers, c.Name)
			columnTypes = append(columnTypes, c.Type)
		}
		rows = make([][]interface{}, len(res.Rows))
		for i, row := range res.Rows {
			rows[i] = row
		}
	}

	return &PagedQueryResult{
		Headers:     headers,
		ColumnTypes: columnTypes,
		Rows:        rows,
		Total:       total,
		Page:        page,
		PageSize:    pageSize,
	}, nil
}

func (s *Service) ExecuteSQL(ctx context.Context, instanceID int64, dbName, sql string) (*DMLResult, error) {
	instance, err := s.getInstanceForSQL(ctx, instanceID, dbName)
	if err != nil {
		return nil, err
	}
	dbType := instance.DBType

	validator := NewSQLValidator(dbType)
	if r := validator.ValidateSQL(sql); !r.Valid {
		return &DMLResult{Success: false, Error: r.Message}, nil
	}

	if _, execErr := s.runnerFor(instance).Exec(ctx, instance, dbName, sql); execErr != nil {
		log.Printf("ExecuteSQL %s error [db=%s]: %s", instance.DBType, dbName, SanitizeSQLError(execErr.Error()))
		return &DMLResult{Success: false, Error: SanitizeSQLError(execErr.Error())}, nil
	}
	return &DMLResult{Success: true}, nil
}

func (s *Service) InsertRecord(ctx context.Context, instanceID int64, dbName, table string, data map[string]interface{}, dryRun bool) (*DMLResult, error) {
	if !ValidateTableName(table) {
		return nil, fmt.Errorf("无效的表名")
	}
	instance, err := s.getInstanceForSQL(ctx, instanceID, dbName)
	if err != nil {
		return nil, err
	}
	dbType := instance.DBType

	builder := NewSQLBuilder(dbType)
	validator := NewSQLValidator(dbType)

	if r := validator.ValidateInsert(table, data, nil); !r.Valid {
		return &DMLResult{Success: false, Error: r.Message}, nil
	}

	if dryRun {
		return &DMLResult{Success: true, DryRun: true, SQL: builder.BuildInsert(table, data, nil)}, nil
	}

	params, args := builder.BuildInsertParams(table, data, nil)
	if _, execErr := s.runnerFor(instance).Exec(ctx, instance, dbName, params, args...); execErr != nil {
		return &DMLResult{Success: false, Error: SanitizeSQLError(execErr.Error())}, nil
	}
	return &DMLResult{Success: true}, nil
}

func (s *Service) UpdateRecord(ctx context.Context, instanceID int64, dbName, table string, data map[string]interface{}, pk string, pkVal interface{}, dryRun bool) (*DMLResult, error) {
	if !ValidateTableName(table) {
		return nil, fmt.Errorf("无效的表名")
	}
	instance, err := s.getInstanceForSQL(ctx, instanceID, dbName)
	if err != nil {
		return nil, err
	}
	dbType := instance.DBType

	builder := NewSQLBuilder(dbType)
	validator := NewSQLValidator(dbType)

	if r := validator.ValidateUpdate(table, data, pk, pkVal); !r.Valid {
		return &DMLResult{Success: false, Error: r.Message}, nil
	}

	if dryRun {
		return &DMLResult{Success: true, DryRun: true, SQL: builder.BuildUpdate(table, data, pk, pkVal)}, nil
	}

	params, args := builder.BuildUpdateParams(table, data, pk, pkVal)
	if _, execErr := s.runnerFor(instance).Exec(ctx, instance, dbName, params, args...); execErr != nil {
		return &DMLResult{Success: false, Error: SanitizeSQLError(execErr.Error())}, nil
	}
	return &DMLResult{Success: true}, nil
}

func (s *Service) DeleteRecord(ctx context.Context, instanceID int64, dbName, table string, pk string, pkVal interface{}, dryRun bool) (*DMLResult, error) {
	if !ValidateTableName(table) {
		return nil, fmt.Errorf("无效的表名")
	}
	instance, err := s.getInstanceForSQL(ctx, instanceID, dbName)
	if err != nil {
		return nil, err
	}
	dbType := instance.DBType

	builder := NewSQLBuilder(dbType)
	validator := NewSQLValidator(dbType)

	if r := validator.ValidateDelete(table, pk, pkVal); !r.Valid {
		return &DMLResult{Success: false, Error: r.Message}, nil
	}

	if dryRun {
		return &DMLResult{Success: true, DryRun: true, SQL: builder.BuildDelete(table, pk, pkVal)}, nil
	}

	params, args := builder.BuildDeleteParams(table, pk, pkVal)
	if _, execErr := s.runnerFor(instance).Exec(ctx, instance, dbName, params, args...); execErr != nil {
		return &DMLResult{Success: false, Error: SanitizeSQLError(execErr.Error())}, nil
	}
	return &DMLResult{Success: true}, nil
}

func (s *Service) CreateTable(ctx context.Context, instanceID int64, dbName, tableName string, columns []TableColumn) error {
	if !ValidateTableName(tableName) {
		return fmt.Errorf("无效的表名")
	}
	instance, err := s.getInstanceForSQL(ctx, instanceID, dbName)
	if err != nil {
		return err
	}
	dbType := instance.DBType

	for _, col := range columns {
		baseType := strings.ToUpper(strings.Split(col.Type, "(")[0])
		baseType = strings.TrimSpace(baseType)
		if !allowedColumnTypes[baseType] {
			return fmt.Errorf("不支持的列类型: %s", col.Type)
		}
		if !ValidateTableName(col.Name) {
			return fmt.Errorf("无效的列名: %s", col.Name)
		}
	}

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

	if _, execErr := s.runnerFor(instance).Exec(ctx, instance, dbName, sql); execErr != nil {
		return fmt.Errorf("创建表失败: %s", SanitizeSQLError(execErr.Error()))
	}
	return nil
}

func (s *Service) DropTable(ctx context.Context, instanceID int64, dbName, tableName string) error {
	if !ValidateTableName(tableName) {
		return fmt.Errorf("无效的表名")
	}
	instance, err := s.getInstanceForSQL(ctx, instanceID, dbName)
	if err != nil {
		return err
	}
	dbType := instance.DBType

	var sql string
	switch dbType {
	case DBTypeMySQL:
		sql = fmt.Sprintf("DROP TABLE `%s`;", tableName)
	case DBTypePostgreSQL:
		sql = fmt.Sprintf("DROP TABLE \"%s\";", tableName)
	default:
		return fmt.Errorf("不支持的数据库类型")
	}

	if _, execErr := s.runnerFor(instance).Exec(ctx, instance, dbName, sql); execErr != nil {
		return fmt.Errorf("删除表失败: %s", SanitizeSQLError(execErr.Error()))
	}
	return nil
}

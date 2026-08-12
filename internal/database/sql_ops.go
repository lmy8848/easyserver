package database

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
)

// --- Logical database CRUD (live, server-owned) ---

func (s *Service) ListDatabases(ctx context.Context, instanceID int64) ([]Database, error) {
	instance, err := s.repo.GetInstance(ctx, instanceID)
	if err != nil {
		return nil, fmt.Errorf("get instance: %w", err)
	}
	if instance == nil {
		return nil, errors.New("database instance not found")
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
	instance, err := s.repo.GetInstance(ctx, instanceID)
	if err != nil {
		return nil, fmt.Errorf("get instance: %w", err)
	}
	if instance == nil {
		return nil, errors.New("database instance not found")
	}
	if instance.Status != "running" && instance.Status != "active" {
		return nil, errors.New("database instance is not running")
	}

	// DDL statements cannot be parameter-bound; names/hosts are validated and
	// passwords escaped by the builder. The system database hosts instance-level
	// statements — including CREATE DATABASE itself, which must not run on the
	// target database.
	charset := req.Charset
	if charset == "" {
		charset = defaultCharset
	}
	builder := NewSQLBuilder(instance.DBType)
	sqlStr, err := builder.BuildCreateDatabase(req.Name, charset)
	if err != nil {
		return nil, err
	}
	sysDB := systemDBName(instance.DBType)
	switch instance.DBType {
	case DBTypeMySQL, DBTypePostgreSQL:
		if _, err := s.runnerFor(instance).Exec(ctx, instance, sysDB, sqlStr); err != nil {
			return nil, fmt.Errorf("create database failed: %s", SanitizeSQLError(err.Error()))
		}
	default:
		return nil, fmt.Errorf("database creation not supported for %s", instance.DBType)
	}

	return &Database{Name: req.Name, Charset: req.Charset}, nil
}

func (s *Service) DeleteDatabase(ctx context.Context, instanceID int64, dbName string) error {
	instance, err := s.repo.GetInstance(ctx, instanceID)
	if err != nil {
		return fmt.Errorf("get instance: %w", err)
	}
	if instance == nil {
		return errors.New("database instance not found")
	}
	if instance.Status != "running" {
		return errors.New("database instance is not running")
	}

	builder := NewSQLBuilder(instance.DBType)
	sqlStr, err := builder.BuildDropDatabase(dbName)
	if err != nil {
		return err
	}
	sysDB := systemDBName(instance.DBType)
	switch instance.DBType {
	case DBTypeMySQL, DBTypePostgreSQL:
		if _, err := s.runnerFor(instance).Exec(ctx, instance, sysDB, sqlStr); err != nil {
			return fmt.Errorf("drop database failed: %s", SanitizeSQLError(err.Error()))
		}
	default:
		return fmt.Errorf("database deletion not supported for %s", instance.DBType)
	}

	return nil
}

// --- DB User CRUD (live, server-owned) ---

func (s *Service) ListDBUsers(ctx context.Context, instanceID int64) ([]DBUser, error) {
	instance, err := s.repo.GetInstance(ctx, instanceID)
	if err != nil {
		return nil, fmt.Errorf("get instance: %w", err)
	}
	if instance == nil {
		return nil, errors.New("database instance not found")
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
		// Redis 无用户体系（ACL 未接入），列表接口显式声明不支持，避免空列表误导。
		return nil, errors.New("暂不支持 Redis 用户管理")
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
	default:
		return false
	}
}

func (s *Service) CreateDBUser(ctx context.Context, instanceID int64, req *CreateDBUserRequest) (*DBUser, error) {
	instance, err := s.repo.GetInstance(ctx, instanceID)
	if err != nil {
		return nil, fmt.Errorf("get instance: %w", err)
	}
	if instance == nil {
		return nil, errors.New("database instance not found")
	}
	if instance.Status != "running" {
		return nil, errors.New("database instance is not running")
	}

	builder := NewSQLBuilder(instance.DBType)
	sqlStr, err := builder.BuildCreateUser(req.Username, req.Password, req.Host)
	if err != nil {
		return nil, err
	}
	sysDB := systemDBName(instance.DBType)
	switch instance.DBType {
	case DBTypeMySQL, DBTypePostgreSQL:
		if _, err := s.runnerFor(instance).Exec(ctx, instance, sysDB, sqlStr); err != nil {
			return nil, fmt.Errorf("create user failed: %s", SanitizeSQLError(err.Error()))
		}
	default:
		return nil, fmt.Errorf("user creation not supported for %s", instance.DBType)
	}

	host := req.Host
	if host == "" {
		host = "localhost"
	}
	return &DBUser{Username: req.Username, Host: host}, nil
}

func (s *Service) DeleteDBUser(ctx context.Context, instanceID int64, username, host string) error {
	instance, err := s.repo.GetInstance(ctx, instanceID)
	if err != nil {
		return fmt.Errorf("get instance: %w", err)
	}
	if instance == nil {
		return errors.New("database instance not found")
	}

	if isAdminUser(instance.DBType, username) {
		return errors.New("cannot delete the administrator user")
	}

	builder := NewSQLBuilder(instance.DBType)
	sqlStr, err := builder.BuildDropUser(username, host)
	if err != nil {
		return err
	}
	sysDB := systemDBName(instance.DBType)
	switch instance.DBType {
	case DBTypeMySQL, DBTypePostgreSQL:
		if _, err := s.runnerFor(instance).Exec(ctx, instance, sysDB, sqlStr); err != nil {
			return fmt.Errorf("drop user failed: %s", SanitizeSQLError(err.Error()))
		}
	default:
		return fmt.Errorf("user deletion not supported for %s", instance.DBType)
	}

	return nil
}

func (s *Service) GrantPrivileges(ctx context.Context, instanceID int64, username, host string, req *GrantRequest) error {
	instance, err := s.repo.GetInstance(ctx, instanceID)
	if err != nil {
		return fmt.Errorf("get instance: %w", err)
	}
	if instance == nil {
		return errors.New("database instance not found")
	}
	if instance.Status != "running" {
		return errors.New("database instance is not running")
	}

	builder := NewSQLBuilder(instance.DBType)
	sqlStr, err := builder.BuildGrant(req.Privileges, req.Database, username, host)
	if err != nil {
		return err
	}
	sysDB := systemDBName(instance.DBType)
	switch instance.DBType {
	case DBTypeMySQL, DBTypePostgreSQL:
		if _, err := s.runnerFor(instance).Exec(ctx, instance, sysDB, sqlStr); err != nil {
			return fmt.Errorf("grant failed: %s", SanitizeSQLError(err.Error()))
		}
	default:
		return fmt.Errorf("privilege grant not supported for %s", instance.DBType)
	}

	return nil
}

func (s *Service) ResetPassword(ctx context.Context, instanceID int64, username, host, newPassword string) error {
	instance, err := s.repo.GetInstance(ctx, instanceID)
	if err != nil {
		return fmt.Errorf("get instance: %w", err)
	}
	if instance == nil {
		return errors.New("database instance not found")
	}
	if instance.Status != "running" {
		return errors.New("database instance is not running")
	}

	builder := NewSQLBuilder(instance.DBType)
	sqlStr, err := builder.BuildResetPassword(username, newPassword, host)
	if err != nil {
		return err
	}
	sysDB := systemDBName(instance.DBType)
	if _, err := s.runnerFor(instance).Exec(ctx, instance, sysDB, sqlStr); err != nil {
		return fmt.Errorf("reset password failed: %s", SanitizeSQLError(err.Error()))
	}
	return nil
}

// --- SQL query operations ---

// getInstanceForSQL resolves the instance for a database-level SQL operation.
// Database names travel as URL path parameters now — no persisted db lookup.
func (s *Service) getInstanceForSQL(ctx context.Context, instanceID int64, dbName string) (*DBInstance, error) {
	if !isValidDBName(dbName) {
		return nil, errors.New("无效的数据库名")
	}
	instance, err := s.repo.GetInstance(ctx, instanceID)
	if err != nil || instance == nil {
		return nil, errors.New("数据库实例不存在")
	}
	return instance, nil
}
func (s *Service) ListTables(ctx context.Context, instanceID int64, dbName string) ([]map[string]any, error) {
	instance, err := s.getInstanceForSQL(ctx, instanceID, dbName)
	if err != nil {
		return nil, err
	}

	var tables []map[string]any
	builder := NewSQLBuilder(instance.DBType)
	res, err := s.runnerFor(instance).Query(ctx, instance, dbName, builder.BuildListTables())
	if err != nil {
		return nil, fmt.Errorf("获取表列表失败: %s", SanitizeSQLError(err.Error()))
	}
	for _, row := range res.Rows {
		tables = append(tables, map[string]any{"name": str(row, 0)})
	}
	return tables, nil
}

func (s *Service) DescribeTable(ctx context.Context, instanceID int64, dbName, tableName string) (*DescribeResult, error) {
	instance, err := s.getInstanceForSQL(ctx, instanceID, dbName)
	if err != nil {
		return nil, err
	}

	builder := NewSQLBuilder(instance.DBType)
	describeSQL, err := builder.BuildDescribeTable(tableName)
	if err != nil {
		return nil, err
	}

	// PG 的 describe SQL 用 $1 绑定表名，MySQL 的 DESCRIBE `table` 直接把名字内联，
	// 没有占位符——传多余参数会报 "expected 0 arguments"。只给 PG 传参。
	var queryArgs []any
	if instance.DBType == DBTypePostgreSQL {
		queryArgs = []any{tableName}
	}
	res, err := s.runnerFor(instance).Query(ctx, instance, dbName, describeSQL, queryArgs...)
	if err != nil {
		return nil, fmt.Errorf("获取表结构失败: %s", SanitizeSQLError(err.Error()))
	}

	// The driver channel returns structured describe rows (name, type, nullable,
	// default, pk-flag columns); the CLI channel parses the same shape from text.
	tableInfo := tableInfoFromQuery(instance.DBType, tableName, res)

	var columns []map[string]any
	for _, col := range tableInfo.Columns {
		columns = append(columns, map[string]any{
			"name":           col.Name,
			"type":           col.Type,
			"is_primary_key": col.IsPrimaryKey,
			"is_auto_incr":   col.IsAutoIncr,
			"has_default":    col.HasDefault,
			"default":        col.DefaultValue,
			"is_nullable":    col.IsNullable,
		})
	}

	result := &DescribeResult{
		TableName:  tableName,
		PrimaryKey: tableInfo.PrimaryKey,
		Columns:    columns,
	}
	// MySQL：取表级排序规则（COLLATE），前端据此显示字符集。PG 无表级字符集，跳过。
	if instance.DBType == DBTypeMySQL {
		collRes, qerr := s.runnerFor(instance).Query(ctx, instance, dbName,
			"SELECT TABLE_COLLATION FROM information_schema.TABLES WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ?", tableName)
		if qerr == nil && len(collRes.Rows) > 0 && len(collRes.Rows[0]) > 0 {
			result.Collation = str(collRes.Rows[0], 0)
		}
	}
	return result, nil
}

func (s *Service) QueryTable(ctx context.Context, instanceID int64, dbName, tableName string, page, pageSize int) (*PagedQueryResult, error) {
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
	countSQL, err := builder.BuildCount(tableName)
	if err != nil {
		return nil, err
	}
	selectSQL, err := builder.BuildSelect(tableName, nil, page, pageSize)
	if err != nil {
		return nil, err
	}

	var total int
	var headers []string
	var columnTypes []string
	var rows [][]any
	switch dbType {
	case DBTypeMySQL, DBTypePostgreSQL:
		countRes, err := s.runnerFor(instance).Query(ctx, instance, dbName, countSQL)
		if err == nil && len(countRes.Rows) > 0 {
			_, _ = fmt.Sscanf(str(countRes.Rows[0], 0), "%d", &total)
		}
		res, err := s.runnerFor(instance).Query(ctx, instance, dbName, selectSQL)
		if err != nil {
			return nil, fmt.Errorf("查询失败: %s", SanitizeSQLError(err.Error()))
		}
		for _, c := range res.Columns {
			headers = append(headers, c.Name)
			columnTypes = append(columnTypes, c.Type)
		}
		rows = make([][]any, len(res.Rows))
		copy(rows, res.Rows)
	default:
		// Redis 不走 SQL 分页查询
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

	if r := ValidateSQL(sql); !r.Valid {
		return &DMLResult{Success: false, Error: r.Message}, nil
	}

	trimmedSQL := strings.TrimSpace(sql)

	// 读语句走 Query 取结果集，写语句走 Exec 报受影响行数。
	if isReadStatement(trimmedSQL) {
		qres, queryErr := s.runnerFor(instance).Query(ctx, instance, dbName, trimmedSQL)
		if queryErr != nil {
			log.Printf("ExecuteSQL %s query error [db=%s]: %s", instance.DBType, dbName, SanitizeSQLError(queryErr.Error()))
			return &DMLResult{Success: false, Error: SanitizeSQLError(queryErr.Error())}, nil
		}
		headers := make([]string, len(qres.Columns))
		types := make([]string, len(qres.Columns))
		for i, col := range qres.Columns {
			headers[i] = col.Name
			types[i] = col.Type
		}
		rows := make([][]any, len(qres.Rows))
		copy(rows, qres.Rows)
		return &DMLResult{Success: true, Headers: headers, ColumnTypes: types, Rows: rows}, nil
	}

	execRes, execErr := s.runnerFor(instance).Exec(ctx, instance, dbName, trimmedSQL)
	if execErr != nil {
		log.Printf("ExecuteSQL %s exec error [db=%s]: %s", instance.DBType, dbName, SanitizeSQLError(execErr.Error()))
		return &DMLResult{Success: false, Error: SanitizeSQLError(execErr.Error())}, nil
	}

	output := "Query OK"
	if execRes != nil {
		if execRes.RowsAffected > 0 {
			output = fmt.Sprintf("Query OK, %d row(s) affected", execRes.RowsAffected)
			if execRes.LastInsertID > 0 {
				output += fmt.Sprintf(" (Last Insert ID: %d)", execRes.LastInsertID)
			}
		} else if execRes.LastInsertID > 0 {
			output = fmt.Sprintf("Query OK (Last Insert ID: %d)", execRes.LastInsertID)
		} else {
			output = "Query OK, 0 rows affected"
		}
	}
	return &DMLResult{Success: true, Output: output}, nil
}

// isReadStatement reports whether a statement is read-only, so ExecuteSQL
// routes it to Query (to surface result sets) instead of Exec. A data-modifying
// CTE (WITH ... INSERT/UPDATE/DELETE) counts as a write despite the WITH prefix.
func isReadStatement(stmt string) bool {
	upper := strings.ToUpper(strings.TrimSpace(stripLeadingComments(stmt)))
	fields := strings.Fields(upper)
	if len(fields) == 0 {
		return false
	}
	switch strings.TrimRight(fields[0], ";") {
	case "SELECT", "SHOW", "EXPLAIN", "DESCRIBE", "DESC", "PRAGMA":
		return true
	case "WITH":
		// WITH cte AS (...) INSERT/UPDATE/DELETE ... is a write disguised as a
		// SELECT-like prefix. The DML keywords are reserved, so a bare column
		// name can't fake them in a plain SELECT.
		for _, kw := range []string{"INSERT INTO ", "UPDATE ", "DELETE FROM "} {
			if strings.Contains(" "+upper+" ", kw) {
				return false
			}
		}
		return true
	}
	return false
}

func (s *Service) InsertRecord(ctx context.Context, instanceID int64, dbName, table string, data map[string]any, dryRun bool) (*DMLResult, error) {
	instance, err := s.getInstanceForSQL(ctx, instanceID, dbName)
	if err != nil {
		return nil, err
	}
	dbType := instance.DBType

	builder := NewSQLBuilder(dbType)

	if dryRun {
		sql, err := builder.BuildInsert(table, data, nil)
		if err != nil {
			return &DMLResult{Success: false, Error: err.Error()}, nil //nolint:nilerr // DML 错误打包进 DMLResult.Error
		}
		return &DMLResult{Success: true, DryRun: true, SQL: sql}, nil
	}

	params, args, err := builder.BuildInsertParams(table, data, nil)
	if err != nil {
		return &DMLResult{Success: false, Error: err.Error()}, nil //nolint:nilerr // DML 错误打包进 DMLResult.Error
	}
	if _, execErr := s.runnerFor(instance).Exec(ctx, instance, dbName, params, args...); execErr != nil {
		return &DMLResult{Success: false, Error: SanitizeSQLError(execErr.Error())}, nil
	}
	return &DMLResult{Success: true}, nil
}

func (s *Service) UpdateRecord(ctx context.Context, instanceID int64, dbName, table string, data map[string]any, pk string, pkVal any, dryRun bool) (*DMLResult, error) {
	instance, err := s.getInstanceForSQL(ctx, instanceID, dbName)
	if err != nil {
		return nil, err
	}
	dbType := instance.DBType

	builder := NewSQLBuilder(dbType)

	if dryRun {
		sql, err := builder.BuildUpdate(table, data, pk, pkVal)
		if err != nil {
			return &DMLResult{Success: false, Error: err.Error()}, nil //nolint:nilerr // DML 错误打包进 DMLResult.Error
		}
		return &DMLResult{Success: true, DryRun: true, SQL: sql}, nil
	}

	params, args, err := builder.BuildUpdateParams(table, data, pk, pkVal)
	if err != nil {
		return &DMLResult{Success: false, Error: err.Error()}, nil //nolint:nilerr // DML 错误打包进 DMLResult.Error
	}
	if _, execErr := s.runnerFor(instance).Exec(ctx, instance, dbName, params, args...); execErr != nil {
		return &DMLResult{Success: false, Error: SanitizeSQLError(execErr.Error())}, nil
	}
	return &DMLResult{Success: true}, nil
}

func (s *Service) DeleteRecord(ctx context.Context, instanceID int64, dbName, table string, pk string, pkVal any, dryRun bool) (*DMLResult, error) {
	instance, err := s.getInstanceForSQL(ctx, instanceID, dbName)
	if err != nil {
		return nil, err
	}
	dbType := instance.DBType

	builder := NewSQLBuilder(dbType)

	if dryRun {
		sql, err := builder.BuildDelete(table, pk, pkVal)
		if err != nil {
			return &DMLResult{Success: false, Error: err.Error()}, nil //nolint:nilerr // DML 错误打包进 DMLResult.Error
		}
		return &DMLResult{Success: true, DryRun: true, SQL: sql}, nil
	}

	params, args, err := builder.BuildDeleteParams(table, pk, pkVal)
	if err != nil {
		return &DMLResult{Success: false, Error: err.Error()}, nil //nolint:nilerr // DML 错误打包进 DMLResult.Error
	}
	if _, execErr := s.runnerFor(instance).Exec(ctx, instance, dbName, params, args...); execErr != nil {
		return &DMLResult{Success: false, Error: SanitizeSQLError(execErr.Error())}, nil
	}
	return &DMLResult{Success: true}, nil
}

func (s *Service) CreateTable(ctx context.Context, instanceID int64, dbName, tableName string, columns []TableColumn, charset, collation string) error {
	instance, err := s.getInstanceForSQL(ctx, instanceID, dbName)
	if err != nil {
		return err
	}
	dbType := instance.DBType

	builder := NewSQLBuilder(dbType)
	sql, err := builder.BuildCreateTable(tableName, columns, charset, collation)
	if err != nil {
		return err
	}

	if _, execErr := s.runnerFor(instance).Exec(ctx, instance, dbName, sql); execErr != nil {
		return fmt.Errorf("创建表失败: %s", SanitizeSQLError(execErr.Error()))
	}
	return nil
}

func (s *Service) DropTable(ctx context.Context, instanceID int64, dbName, tableName string) error {
	instance, err := s.getInstanceForSQL(ctx, instanceID, dbName)
	if err != nil {
		return err
	}
	dbType := instance.DBType

	builder := NewSQLBuilder(dbType)
	sql, err := builder.BuildDropTable(tableName)
	if err != nil {
		return err
	}

	if _, execErr := s.runnerFor(instance).Exec(ctx, instance, dbName, sql); execErr != nil {
		return fmt.Errorf("删除表失败: %s", SanitizeSQLError(execErr.Error()))
	}
	return nil
}

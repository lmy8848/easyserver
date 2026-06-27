package database_mgmt

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"easyserver/internal/dbserver"
	"easyserver/internal/infra/executor"

	"golang.org/x/crypto/bcrypt"
)

const (
	maxDBNameLen    = 64
	maxUsernameLen  = 32
	maxHostLen      = 255
	maxDescLen      = 500
	maxLogLines     = 5000
	defaultCharset  = "utf8mb4"
	defaultLogLines = 200

	DefaultBackupDir = "/var/backups/easyserver/db"
	DefaultRedisRDB  = "/var/lib/redis/dump.rdb"
	MaxBackupsPerDB  = 10
)

var validCharsets = map[string]bool{
	"utf8mb4": true, "utf8": true, "latin1": true,
	"ascii": true, "gbk": true, "big5": true,
}

var validPrivileges = map[string]bool{
	"ALL PRIVILEGES": true, "SELECT": true, "INSERT": true,
	"UPDATE": true, "DELETE": true, "CREATE": true, "DROP": true,
	"INDEX": true, "ALTER": true, "EXECUTE": true,
}

// Service manages databases, users, backups, and SQL queries.
type Service struct {
	repo      Repository
	executor  executor.CommandExecutor
	backupDir string
}

// NewService creates a new database management Service.
func NewService(repo Repository, exec executor.CommandExecutor) *Service {
	return &Service{
		repo:      repo,
		executor:  exec,
		backupDir: DefaultBackupDir,
	}
}

// --- Database CRUD ---

// ListDatabases returns all databases for a server.
func (s *Service) ListDatabases(ctx context.Context, dbServerID int64) ([]Database, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	server, err := s.repo.GetServer(ctx, dbServerID)
	if err != nil {
		return nil, fmt.Errorf("get server: %w", err)
	}
	if server == nil {
		return nil, fmt.Errorf("database server not found")
	}
	return s.repo.ListDatabases(ctx, dbServerID)
}

// GetDatabaseByID returns a database by its ID.
func (s *Service) GetDatabaseByID(ctx context.Context, id int64) (*Database, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	return s.repo.GetDatabaseByID(ctx, id)
}

// GetServerByID returns a server by its ID.
func (s *Service) GetServerByID(ctx context.Context, id int64) (*dbserver.DBServer, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	return s.repo.GetServer(ctx, id)
}

// CreateDatabase creates a new database.
func (s *Service) CreateDatabase(ctx context.Context, dbServerID int64, req *CreateDatabaseRequest) (*Database, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	version, err := s.repo.GetVersion(ctx, req.DBVersionID)
	if err != nil {
		return nil, fmt.Errorf("get version: %w", err)
	}
	if version == nil {
		return nil, fmt.Errorf("database version not found")
	}
	if version.Status != "running" && version.Status != "active" {
		return nil, fmt.Errorf("database version is not running")
	}

	server, err := s.repo.GetServer(ctx, dbServerID)
	if err != nil {
		return nil, fmt.Errorf("get server: %w", err)
	}
	if server == nil {
		return nil, fmt.Errorf("database server not found")
	}

	if !isValidDBName(req.Name) {
		return nil, fmt.Errorf("invalid database name")
	}
	if len(req.Description) > maxDescLen {
		return nil, fmt.Errorf("description too long (max %d chars)", maxDescLen)
	}

	charset := req.Charset
	if charset == "" {
		charset = defaultCharset
	}
	if !isValidCharset(charset) {
		return nil, fmt.Errorf("invalid charset: %s", charset)
	}

	switch server.Name {
	case "mysql":
		out, _, err := s.executor.RunCombined(ctx, "mysql", "-e", fmt.Sprintf("CREATE DATABASE `%s` CHARACTER SET %s;",
			strings.ReplaceAll(req.Name, "`", "``"), charset))
		if err != nil {
			return nil, fmt.Errorf("create database failed: %s", out)
		}
	case "postgresql":
		out, _, err := s.executor.RunCombined(ctx, "sudo", "-u", "postgres", "createdb", "-E", charset, req.Name)
		if err != nil {
			return nil, fmt.Errorf("create database failed: %s", out)
		}
	default:
		return nil, fmt.Errorf("database creation not supported for %s", server.Name)
	}

	id, err := s.repo.CreateDatabase(ctx, dbServerID, req.DBVersionID, req.Name, charset, req.Description)
	if err != nil {
		return nil, err
	}

	return &Database{
		ID:          id,
		DBServerID:  dbServerID,
		DBVersionID: req.DBVersionID,
		Name:        req.Name,
		Charset:     charset,
		Status:      "active",
		Version:     version.Version,
	}, nil
}

// DeleteDatabase deletes a database.
func (s *Service) DeleteDatabase(ctx context.Context, dbServerID, id int64) error {
	if ctx == nil {
		ctx = context.Background()
	}
	d, err := s.repo.GetDatabase(ctx, dbServerID, id)
	if err != nil {
		return fmt.Errorf("get database: %w", err)
	}
	if d == nil {
		return fmt.Errorf("database not found")
	}

	server, err := s.repo.GetServer(ctx, dbServerID)
	if err != nil {
		return fmt.Errorf("get server: %w", err)
	}
	if server == nil {
		return fmt.Errorf("database server not found")
	}

	version, err := s.repo.GetVersion(ctx, d.DBVersionID)
	if err != nil {
		return fmt.Errorf("get version: %w", err)
	}
	if version == nil || version.Status != "running" {
		return fmt.Errorf("database version is not running")
	}

	switch server.Name {
	case "mysql":
		out, _, err := s.executor.RunCombined(ctx, "mysql", "-e", fmt.Sprintf("DROP DATABASE `%s`;",
			strings.ReplaceAll(d.Name, "`", "``")))
		if err != nil {
			return fmt.Errorf("drop database failed: %s", out)
		}
	case "postgresql":
		out, _, err := s.executor.RunCombined(ctx, "sudo", "-u", "postgres", "dropdb", d.Name)
		if err != nil {
			return fmt.Errorf("drop database failed: %s", out)
		}
	default:
		return fmt.Errorf("database deletion not supported for %s", server.Name)
	}

	return s.repo.DeleteDatabase(ctx, dbServerID, id)
}

// --- DB User CRUD ---

// ListDBUsers returns all database users for a server.
func (s *Service) ListDBUsers(ctx context.Context, dbServerID int64) ([]DBUser, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	server, err := s.repo.GetServer(ctx, dbServerID)
	if err != nil {
		return nil, fmt.Errorf("get server: %w", err)
	}
	if server == nil {
		return nil, fmt.Errorf("database server not found")
	}
	return s.repo.ListDBUsers(ctx, dbServerID)
}

// CreateDBUser creates a new database user.
func (s *Service) CreateDBUser(ctx context.Context, dbServerID int64, req *CreateDBUserRequest) (*DBUser, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	server, err := s.repo.GetServer(ctx, dbServerID)
	if err != nil {
		return nil, fmt.Errorf("get server: %w", err)
	}
	if server == nil {
		return nil, fmt.Errorf("database server not found")
	}

	versions, err := s.repo.ListVersions(ctx, dbServerID)
	if err != nil {
		return nil, fmt.Errorf("list versions: %w", err)
	}
	anyRunning := false
	for _, v := range versions {
		if v.Status == "running" {
			anyRunning = true
			break
		}
	}
	if !anyRunning {
		return nil, fmt.Errorf("no running version available")
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

	switch server.Name {
	case "mysql":
		sqlStr := fmt.Sprintf("CREATE USER '%s'@'%s' IDENTIFIED BY '%s';", req.Username, host, escapeMySQLString(req.Password))
		out, _, err := s.executor.RunCombined(ctx, "mysql", "-e", sqlStr)
		if err != nil {
			return nil, fmt.Errorf("create user failed: %s", out)
		}
	case "postgresql":
		out, _, err := s.executor.RunCombined(ctx, "sudo", "-u", "postgres", "psql", "-c",
			fmt.Sprintf("CREATE USER \"%s\" WITH PASSWORD '%s';", req.Username, escapePGString(req.Password)))
		if err != nil {
			return nil, fmt.Errorf("create user failed: %s", out)
		}
	default:
		return nil, fmt.Errorf("user creation not supported for %s", server.Name)
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("failed to hash password: %w", err)
	}

	id, err := s.repo.CreateDBUser(ctx, dbServerID, req.Username, string(hashedPassword), host)
	if err != nil {
		return nil, err
	}
	return &DBUser{
		ID:         id,
		DBServerID: dbServerID,
		Username:   req.Username,
		Host:       host,
	}, nil
}

// DeleteDBUser deletes a database user.
func (s *Service) DeleteDBUser(ctx context.Context, dbServerID, id int64) error {
	if ctx == nil {
		ctx = context.Background()
	}
	u, err := s.repo.GetDBUser(ctx, dbServerID, id)
	if err != nil {
		return fmt.Errorf("get user: %w", err)
	}
	if u == nil {
		return fmt.Errorf("user not found")
	}

	server, err := s.repo.GetServer(ctx, dbServerID)
	if err != nil {
		return fmt.Errorf("get server: %w", err)
	}
	if server == nil {
		return fmt.Errorf("database server not found")
	}

	versions, err := s.repo.ListVersions(ctx, dbServerID)
	if err != nil {
		return fmt.Errorf("list versions: %w", err)
	}
	anyRunning := false
	for _, v := range versions {
		if v.Status == "running" {
			anyRunning = true
			break
		}
	}
	if !anyRunning {
		return fmt.Errorf("no running version available")
	}

	switch server.Name {
	case "mysql":
		sqlStr := fmt.Sprintf("DROP USER '%s'@'%s';", u.Username, u.Host)
		out, _, err := s.executor.RunCombined(ctx, "mysql", "-e", sqlStr)
		if err != nil {
			return fmt.Errorf("drop user failed: %s", out)
		}
	case "postgresql":
		out, _, err := s.executor.RunCombined(ctx, "sudo", "-u", "postgres", "psql", "-c",
			fmt.Sprintf("DROP USER \"%s\";", u.Username))
		if err != nil {
			return fmt.Errorf("drop user failed: %s", out)
		}
	default:
		return fmt.Errorf("user deletion not supported for %s", server.Name)
	}

	return s.repo.DeleteDBUser(ctx, dbServerID, id)
}

// GrantPrivileges grants privileges to a database user.
func (s *Service) GrantPrivileges(ctx context.Context, dbServerID, userID int64, req *GrantRequest) error {
	if ctx == nil {
		ctx = context.Background()
	}
	u, err := s.repo.GetDBUser(ctx, dbServerID, userID)
	if err != nil {
		return fmt.Errorf("get user: %w", err)
	}
	if u == nil {
		return fmt.Errorf("user not found")
	}

	server, err := s.repo.GetServer(ctx, dbServerID)
	if err != nil {
		return fmt.Errorf("get server: %w", err)
	}
	if server == nil {
		return fmt.Errorf("database server not found")
	}

	version, err := s.repo.GetVersion(ctx, req.DBVersionID)
	if err != nil {
		return fmt.Errorf("get version: %w", err)
	}
	if version == nil || version.Status != "running" {
		return fmt.Errorf("database version is not running")
	}

	if !isValidDBName(req.Database) {
		return fmt.Errorf("invalid database name")
	}

	for _, priv := range strings.Split(req.Privileges, ",") {
		priv = strings.TrimSpace(priv)
		if priv != "" && !isValidPrivilege(priv) {
			return fmt.Errorf("invalid privilege: %s", priv)
		}
	}

	switch server.Name {
	case "mysql":
		sqlStr := fmt.Sprintf("GRANT %s ON `%s`.* TO '%s'@'%s'; FLUSH PRIVILEGES;",
			req.Privileges, strings.ReplaceAll(req.Database, "`", "``"), u.Username, u.Host)
		out, _, err := s.executor.RunCombined(ctx, "mysql", "-e", sqlStr)
		if err != nil {
			return fmt.Errorf("grant failed: %s", out)
		}
	case "postgresql":
		sqlStr := fmt.Sprintf("GRANT %s ON DATABASE \"%s\" TO \"%s\";", req.Privileges, req.Database, u.Username)
		out, _, err := s.executor.RunCombined(ctx, "sudo", "-u", "postgres", "psql", "-c", sqlStr)
		if err != nil {
			return fmt.Errorf("grant failed: %s", out)
		}
	default:
		return fmt.Errorf("privilege grant not supported for %s", server.Name)
	}

	privStr := fmt.Sprintf("%s@%s", req.Privileges, req.Database)
	existing := u.Privileges
	if existing != "" {
		existing += ";"
	}
	if err := s.repo.UpdateDBUserPrivileges(ctx, userID, existing+privStr); err != nil {
		return fmt.Errorf("update privileges in db: %w", err)
	}

	return nil
}

// --- Backup operations ---

// SetBackupDir sets the backup directory.
func (s *Service) SetBackupDir(dir string) {
	s.backupDir = dir
}

// CreateBackup creates a backup of a database.
func (s *Service) CreateBackup(ctx context.Context, dbServerID, dbVersionID, databaseID int64, dbName, dbType string) (*DBBackup, error) {
	if err := os.MkdirAll(s.backupDir, 0755); err != nil {
		return nil, fmt.Errorf("create backup dir: %w", err)
	}

	timestamp := time.Now().Format("20060102150405")
	var fileName string
	switch dbType {
	case "mysql":
		fileName = fmt.Sprintf("%s_%s.sql", dbName, timestamp)
	case "postgresql":
		fileName = fmt.Sprintf("%s_%s.sql", dbName, timestamp)
	case "redis":
		fileName = fmt.Sprintf("dump_%s.rdb", timestamp)
	default:
		return nil, fmt.Errorf("unsupported db type: %s", dbType)
	}
	filePath := filepath.Join(s.backupDir, fileName)

	backup := &DBBackup{
		DBServerID:   dbServerID,
		DBVersionID:  dbVersionID,
		DatabaseID:   databaseID,
		DatabaseName: dbName,
		BackupType:   "manual",
		FilePath:     filePath,
		Status:       "pending",
	}

	id, err := s.repo.CreateBackup(ctx, backup)
	if err != nil {
		return nil, err
	}
	backup.ID = id

	backupCtx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	go func() {
		defer cancel()
		s.executeBackup(backupCtx, backup, dbType)
	}()

	return backup, nil
}

func (s *Service) executeBackup(ctx context.Context, backup *DBBackup, dbType string) {
	var err error

	switch dbType {
	case "mysql":
		err = s.backupMySQL(ctx, backup)
	case "postgresql":
		err = s.backupPostgreSQL(ctx, backup)
	case "redis":
		err = s.backupRedis(ctx, backup)
	}

	if err != nil {
		backup.Status = "failed"
		backup.ErrorMessage = err.Error()
		log.Printf("backup failed for %s: %v", backup.DatabaseName, err)
	} else {
		backup.Status = "completed"
		if info, err := os.Stat(backup.FilePath); err == nil {
			backup.FileSize = info.Size()
		}
	}

	if err := s.repo.UpdateBackupStatus(ctx, backup.ID, backup.Status, backup.FileSize, backup.ErrorMessage); err != nil {
		log.Printf("failed to update backup record %d: %v", backup.ID, err)
	}
}

func (s *Service) backupMySQL(ctx context.Context, backup *DBBackup) error {
	out, _, _, err := s.executor.Run(ctx, "mysqldump", "--single-transaction", "--routines", "--triggers", backup.DatabaseName)
	if err != nil {
		return fmt.Errorf("mysqldump failed: %w", err)
	}
	return os.WriteFile(backup.FilePath, []byte(out), 0644)
}

func (s *Service) backupPostgreSQL(ctx context.Context, backup *DBBackup) error {
	out, _, _, err := s.executor.Run(ctx, "sudo", "-u", "postgres", "pg_dump", "-Fc", backup.DatabaseName)
	if err != nil {
		return fmt.Errorf("pg_dump failed: %w", err)
	}
	return os.WriteFile(backup.FilePath, []byte(out), 0644)
}

func (s *Service) backupRedis(ctx context.Context, backup *DBBackup) error {
	_, _, err := s.executor.RunCombined(ctx, "redis-cli", "BGSAVE")
	if err != nil {
		return fmt.Errorf("redis BGSAVE failed: %w", err)
	}

	time.Sleep(2 * time.Second)

	data, err := os.ReadFile(DefaultRedisRDB)
	if err != nil {
		return fmt.Errorf("read RDB file: %w", err)
	}
	return os.WriteFile(backup.FilePath, data, 0644)
}

// ListBackups returns all backups for a database.
func (s *Service) ListBackups(ctx context.Context, databaseID int64) ([]DBBackup, error) {
	return s.repo.ListBackups(ctx, databaseID)
}

// GetBackup returns a backup by ID.
func (s *Service) GetBackup(ctx context.Context, id int64) (*DBBackup, error) {
	return s.repo.GetBackup(ctx, id)
}

// DeleteBackup deletes a backup file and record.
func (s *Service) DeleteBackup(ctx context.Context, id int64) error {
	backup, err := s.repo.GetBackup(ctx, id)
	if err != nil {
		return fmt.Errorf("backup not found: %w", err)
	}

	if err := os.Remove(backup.FilePath); err != nil && !os.IsNotExist(err) {
		log.Printf("failed to delete backup file %s: %v", backup.FilePath, err)
	}

	return s.repo.DeleteBackup(ctx, id)
}

// RestoreBackup restores a database from backup.
func (s *Service) RestoreBackup(ctx context.Context, id int64, dbType string) error {
	backup, err := s.repo.GetBackup(ctx, id)
	if err != nil {
		return fmt.Errorf("backup not found: %w", err)
	}

	if backup.Status != "completed" {
		return fmt.Errorf("backup is not in completed status")
	}

	if _, err := os.Stat(backup.FilePath); os.IsNotExist(err) {
		return fmt.Errorf("backup file not found")
	}

	switch dbType {
	case "mysql":
		return s.restoreMySQL(ctx, backup)
	case "postgresql":
		return s.restorePostgreSQL(ctx, backup)
	case "redis":
		return s.restoreRedis(ctx, backup)
	default:
		return fmt.Errorf("unsupported db type: %s", dbType)
	}
}

func (s *Service) restoreMySQL(ctx context.Context, backup *DBBackup) error {
	file, err := os.Open(backup.FilePath)
	if err != nil {
		return fmt.Errorf("open backup file: %w", err)
	}
	defer file.Close()

	cmd := exec.CommandContext(ctx, "mysql", backup.DatabaseName)
	cmd.Stdin = file
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("mysql restore failed: %s", string(out))
	}
	return nil
}

func (s *Service) restorePostgreSQL(ctx context.Context, backup *DBBackup) error {
	out, _, err := s.executor.RunCombined(ctx, "sudo", "-u", "postgres", "pg_restore", "-d", backup.DatabaseName, "-c", backup.FilePath)
	if err != nil {
		return fmt.Errorf("pg_restore failed: %s", out)
	}
	return nil
}

func (s *Service) restoreRedis(ctx context.Context, backup *DBBackup) error {
	s.executor.RunCombined(ctx, "redis-cli", "SHUTDOWN", "NOSAVE")
	time.Sleep(1 * time.Second)

	data, err := os.ReadFile(backup.FilePath)
	if err != nil {
		return fmt.Errorf("read backup file: %w", err)
	}
	if err := os.WriteFile(DefaultRedisRDB, data, 0644); err != nil {
		return fmt.Errorf("write RDB file: %w", err)
	}

	_, _, err = s.executor.RunCombined(ctx, "systemctl", "start", "redis-server")
	if err != nil {
		return fmt.Errorf("start Redis failed: %w", err)
	}
	return nil
}

// CleanOldBackups removes old backups beyond the limit.
func (s *Service) CleanOldBackups(ctx context.Context, databaseID int64, maxBackups int) error {
	if maxBackups <= 0 {
		maxBackups = MaxBackupsPerDB
	}

	backups, err := s.repo.ListBackups(ctx, databaseID)
	if err != nil {
		return err
	}

	if len(backups) > maxBackups {
		for _, b := range backups[maxBackups:] {
			os.Remove(b.FilePath)
			s.repo.DeleteBackup(ctx, b.ID)
		}
	}

	return nil
}

// --- SQL Query operations ---

// lookupDB resolves dbID → database + server + typed DBType.
func (s *Service) lookupDB(ctx context.Context, dbID int64) (*Database, *dbserver.DBServer, DBType, error) {
	db, err := s.repo.GetDatabaseByID(ctx, dbID)
	if err != nil || db == nil {
		return nil, nil, "", fmt.Errorf("数据库不存在")
	}
	server, err := s.repo.GetServer(ctx, db.DBServerID)
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

func (s *Service) execRaw(ctx context.Context, dbType DBType, dbName string, sql string) (string, error) {
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

// ListTables returns all table names in the given database.
func (s *Service) ListTables(ctx context.Context, dbID int64) ([]map[string]interface{}, error) {
	db, server, _, err := s.lookupDB(ctx, dbID)
	if err != nil {
		return nil, err
	}

	var tables []map[string]interface{}
	switch server.Name {
	case "mysql":
		out, err := s.execRaw(ctx, DBTypeMySQL, db.Name, "SHOW TABLES;")
		if err != nil {
			return nil, fmt.Errorf("获取表列表失败: %s", SanitizeSQLError(out))
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
			return nil, fmt.Errorf("获取表列表失败: %s", SanitizeSQLError(out))
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
func (s *Service) DescribeTable(ctx context.Context, dbID int64, tableName string) (*DescribeResult, error) {
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
		return nil, fmt.Errorf("获取表结构失败: %s", SanitizeSQLError(out))
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
func (s *Service) QueryTable(ctx context.Context, dbID int64, tableName string, page, pageSize int) (*PagedQueryResult, error) {
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

	var headers []string
	var rows [][]interface{}
	switch dbType {
	case DBTypeMySQL:
		out, err := s.execRaw(ctx, DBTypeMySQL, db.Name,
			fmt.Sprintf("SELECT * FROM `%s` LIMIT %d OFFSET %d;", tableName, pageSize, offset))
		if err != nil {
			return nil, fmt.Errorf("查询失败: %s", SanitizeSQLError(out))
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
			return nil, fmt.Errorf("查询失败: %s", SanitizeSQLError(out))
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
func (s *Service) ExecuteSQL(ctx context.Context, dbID int64, sql string) (*DMLResult, error) {
	db, server, dbType, err := s.lookupDB(ctx, dbID)
	if err != nil {
		return nil, err
	}

	validator := NewSQLValidator(dbType)
	if r := validator.ValidateSQL(sql); !r.Valid {
		return &DMLResult{Success: false, Error: r.Message}, nil
	}

	out, execErr := s.execRaw(ctx, dbType, db.Name, sql)
	if execErr != nil {
		log.Printf("ExecuteSQL %s error [db=%s]: %s", server.Name, db.Name, SanitizeSQLError(out))
		return &DMLResult{Success: false, Error: SanitizeSQLError(out)}, nil
	}
	return &DMLResult{Success: true, Output: out}, nil
}

// InsertRecord inserts a row; dryRun=true returns the SQL without executing.
func (s *Service) InsertRecord(ctx context.Context, dbID int64, table string, data map[string]interface{}, dryRun bool) (*DMLResult, error) {
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
		return &DMLResult{Success: false, Error: SanitizeSQLError(out)}, nil
	}
	return &DMLResult{Success: true, Output: out}, nil
}

// UpdateRecord updates a row; dryRun=true returns the SQL without executing.
func (s *Service) UpdateRecord(ctx context.Context, dbID int64, table string, data map[string]interface{}, pk string, pkVal interface{}, dryRun bool) (*DMLResult, error) {
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
		return &DMLResult{Success: false, Error: SanitizeSQLError(out)}, nil
	}
	return &DMLResult{Success: true, Output: out}, nil
}

// DeleteRecord deletes a row; dryRun=true returns the SQL without executing.
func (s *Service) DeleteRecord(ctx context.Context, dbID int64, table string, pk string, pkVal interface{}, dryRun bool) (*DMLResult, error) {
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
		return &DMLResult{Success: false, Error: SanitizeSQLError(out)}, nil
	}
	return &DMLResult{Success: true}, nil
}

// CreateTable creates a new table in the given database.
func (s *Service) CreateTable(ctx context.Context, dbID int64, tableName string, columns []TableColumn) error {
	if !ValidateTableName(tableName) {
		return fmt.Errorf("无效的表名")
	}
	db, _, dbType, err := s.lookupDB(ctx, dbID)
	if err != nil {
		return err
	}

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
		return fmt.Errorf("创建表失败: %s", SanitizeSQLError(out))
	}
	return nil
}

// DropTable drops a table from the given database.
func (s *Service) DropTable(ctx context.Context, dbID int64, tableName string) error {
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

func isValidPrivilege(priv string) bool {
	return validPrivileges[priv]
}

func escapeMySQLString(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `'`, `\'`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	s = strings.ReplaceAll(s, "\n", `\n`)
	s = strings.ReplaceAll(s, "\r", `\r`)
	s = strings.ReplaceAll(s, "\t", `\t`)
	s = strings.ReplaceAll(s, "\x00", `\0`)
	s = strings.ReplaceAll(s, "\x1a", `\Z`)
	return s
}

func escapePGString(s string) string {
	return strings.ReplaceAll(s, "'", "''")
}

// --- Config parsing helpers ---

// GetCommonParams returns metadata for common MySQL configuration parameters.
func GetCommonParams(section string) []ParamMeta {
	switch section {
	case "mysqld":
		return []ParamMeta{
			{Key: "port", Label: "监听端口", Description: "MySQL 服务监听的端口号", Type: "number", Default: "3306"},
			{Key: "datadir", Label: "数据目录", Description: "MySQL 数据文件存储路径", Type: "text", Default: "/var/lib/mysql"},
			{Key: "socket", Label: "Socket 文件", Description: "Unix Socket 文件路径", Type: "text", Default: "/var/run/mysqld/mysqld.sock"},
			{Key: "max_connections", Label: "最大连接数", Description: "允许的最大并发连接数", Type: "number", Default: "151"},
			{Key: "innodb_buffer_pool_size", Label: "InnoDB 缓冲池", Description: "InnoDB 缓冲池大小，建议设为内存的 70-80%", Type: "text", Unit: "MB/GB", Default: "128M"},
			{Key: "character-set-server", Label: "服务器字符集", Description: "默认字符集", Type: "select", Options: []string{"utf8mb4", "utf8", "latin1", "gbk"}, Default: "utf8mb4"},
			{Key: "collation-server", Label: "排序规则", Description: "默认排序规则", Type: "text", Default: "utf8mb4_general_ci"},
			{Key: "default-storage-engine", Label: "默认存储引擎", Description: "默认存储引擎", Type: "select", Options: []string{"InnoDB", "MyISAM", "MEMORY"}, Default: "InnoDB"},
			{Key: "max_allowed_packet", Label: "最大数据包", Description: "单个数据包最大大小", Type: "text", Unit: "MB", Default: "64M"},
			{Key: "tmp_table_size", Label: "临时表大小", Description: "内存临时表最大大小", Type: "text", Unit: "MB", Default: "64M"},
			{Key: "max_heap_table_size", Label: "堆表最大大小", Description: "用户创建的内存表最大大小", Type: "text", Unit: "MB", Default: "64M"},
			{Key: "sort_buffer_size", Label: "排序缓冲区", Description: "每个会话的排序缓冲区大小", Type: "text", Unit: "KB", Default: "256K"},
			{Key: "read_buffer_size", Label: "读缓冲区", Description: "顺序扫描的读缓冲区大小", Type: "text", Unit: "KB", Default: "256K"},
			{Key: "join_buffer_size", Label: "JOIN 缓冲区", Description: "JOIN 操作的缓冲区大小", Type: "text", Unit: "KB", Default: "256K"},
			{Key: "log_error", Label: "错误日志路径", Description: "错误日志文件路径", Type: "text", Default: "/var/log/mysql/error.log"},
			{Key: "slow_query_log", Label: "慢查询日志", Description: "是否启用慢查询日志", Type: "select", Options: []string{"ON", "OFF"}, Default: "OFF"},
			{Key: "slow_query_log_file", Label: "慢查询日志路径", Description: "慢查询日志文件路径", Type: "text", Default: "/var/log/mysql/slow.log"},
			{Key: "long_query_time", Label: "慢查询阈值", Description: "超过此时间的查询记录到慢查询日志", Type: "number", Unit: "秒", Default: "10"},
			{Key: "wait_timeout", Label: "空闲超时", Description: "非交互连接的空闲超时时间", Type: "number", Unit: "秒", Default: "28800"},
			{Key: "interactive_timeout", Label: "交互超时", Description: "交互连接的空闲超时时间", Type: "number", Unit: "秒", Default: "28800"},
		}
	case "client":
		return []ParamMeta{
			{Key: "default-character-set", Label: "默认字符集", Description: "客户端默认字符集", Type: "select", Options: []string{"utf8mb4", "utf8", "latin1", "gbk"}, Default: "utf8mb4"},
			{Key: "port", Label: "端口", Description: "连接端口", Type: "number", Default: "3306"},
			{Key: "socket", Label: "Socket", Description: "Unix Socket 文件路径", Type: "text", Default: "/var/run/mysqld/mysqld.sock"},
		}
	case "mysql":
		return []ParamMeta{
			{Key: "default-character-set", Label: "默认字符集", Description: "mysql 客户端默认字符集", Type: "select", Options: []string{"utf8mb4", "utf8", "latin1", "gbk"}, Default: "utf8mb4"},
		}
	case "mysqldump":
		return []ParamMeta{
			{Key: "max_allowed_packet", Label: "最大数据包", Description: "导出数据包最大大小", Type: "text", Unit: "MB", Default: "64M"},
			{Key: "default-character-set", Label: "默认字符集", Description: "导出默认字符集", Type: "select", Options: []string{"utf8mb4", "utf8", "latin1"}, Default: "utf8mb4"},
		}
	}
	return nil
}

// CommonConfigFilePaths returns common MySQL config file locations.
func CommonConfigFilePaths() []string {
	return []string{
		"/etc/mysql/my.cnf",
		"/etc/mysql/mysql.conf.d/mysqld.cnf",
		"/etc/my.cnf",
		"/usr/etc/my.cnf",
	}
}

// FindMySQLConfig finds the active MySQL config file.
func FindMySQLConfig() string {
	for _, path := range CommonConfigFilePaths() {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	return ""
}

// ParseMySQLConfig parses a my.cnf file into structured sections.
func ParseMySQLConfig(filePath string) (*DBConfig, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("cannot open config file: %w", err)
	}
	defer file.Close()

	config := &DBConfig{
		FilePath: filePath,
		Sections: []ConfigSection{},
	}

	var currentSection *ConfigSection
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}

		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			sectionName := line[1 : len(line)-1]
			config.Sections = append(config.Sections, ConfigSection{
				Name:   sectionName,
				Params: make(map[string]string),
			})
			currentSection = &config.Sections[len(config.Sections)-1]
			continue
		}

		if currentSection != nil {
			parts := strings.SplitN(line, "=", 2)
			if len(parts) == 2 {
				key := strings.TrimSpace(parts[0])
				value := strings.TrimSpace(parts[1])
				if idx := strings.Index(value, " #"); idx != -1 {
					value = strings.TrimSpace(value[:idx])
				}
				if idx := strings.Index(value, " //"); idx != -1 {
					value = strings.TrimSpace(value[:idx])
				}
				currentSection.Params[key] = value
			}
		}
	}

	return config, nil
}

func backupConfigFile(filePath string) error {
	backupPath := filePath + ".bak." + time.Now().Format("20060102150405")
	data, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read config for backup: %w", err)
	}
	if err := os.WriteFile(backupPath, data, 0644); err != nil {
		return fmt.Errorf("write backup: %w", err)
	}
	return nil
}

// SaveMySQLConfig saves the structured config back to file.
func SaveMySQLConfig(config *DBConfig) error {
	if err := backupConfigFile(config.FilePath); err != nil {
		return err
	}

	var sb strings.Builder
	sb.WriteString("# EasyServer generated MySQL configuration\n")
	sb.WriteString("# " + time.Now().Format("2006-01-02 15:04:05") + "\n\n")

	for _, section := range config.Sections {
		sb.WriteString(fmt.Sprintf("[%s]\n", section.Name))
		keys := make([]string, 0, len(section.Params))
		for key := range section.Params {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			sb.WriteString(fmt.Sprintf("%s = %s\n", key, section.Params[key]))
		}
		sb.WriteString("\n")
	}

	dir := filepath.Dir(config.FilePath)
	os.MkdirAll(dir, 0755)

	return os.WriteFile(config.FilePath, []byte(sb.String()), 0644)
}

// UpdateConfigParam updates a single parameter in a config section.
func UpdateConfigParam(config *DBConfig, section, key, value string) {
	for i, s := range config.Sections {
		if s.Name == section {
			config.Sections[i].Params[key] = value
			return
		}
	}
	config.Sections = append(config.Sections, ConfigSection{
		Name:   section,
		Params: map[string]string{key: value},
	})
}

// FindPostgreSQLConfig finds the active PostgreSQL config file.
func FindPostgreSQLConfig() string {
	paths := []string{
		"/etc/postgresql/16/main/postgresql.conf",
		"/etc/postgresql/15/main/postgresql.conf",
		"/etc/postgresql/14/main/postgresql.conf",
		"/etc/postgresql/13/main/postgresql.conf",
		"/var/lib/pgsql/data/postgresql.conf",
		"/var/lib/postgresql/16/main/postgresql.conf",
		"/var/lib/postgresql/15/main/postgresql.conf",
		"/var/lib/postgresql/14/main/postgresql.conf",
		"/var/lib/postgresql/13/main/postgresql.conf",
	}
	matches, _ := filepath.Glob("/etc/postgresql/*/main/postgresql.conf")
	paths = append(matches, paths...)
	matches, _ = filepath.Glob("/var/lib/postgresql/*/main/postgresql.conf")
	paths = append(matches, paths...)

	for _, path := range paths {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	return ""
}

// ParsePostgreSQLConfig parses a postgresql.conf file.
func ParsePostgreSQLConfig(filePath string) (*DBConfig, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("cannot open config file: %w", err)
	}
	defer file.Close()

	params := make(map[string]string)
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "include") || strings.HasPrefix(line, "include_if_exists") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			key := strings.TrimSpace(parts[0])
			value := strings.TrimSpace(parts[1])
			if idx := strings.Index(value, " #"); idx != -1 {
				value = strings.TrimSpace(value[:idx])
			}
			if len(value) >= 2 {
				if (value[0] == '\'' && value[len(value)-1] == '\'') ||
					(value[0] == '"' && value[len(value)-1] == '"') {
					value = value[1 : len(value)-1]
				}
			}
			params[key] = value
		}
	}

	return &DBConfig{
		FilePath: filePath,
		Sections: []ConfigSection{
			{Name: "main", Params: params},
		},
	}, nil
}

func pgNeedsQuote(value string) bool {
	if _, err := fmt.Sscanf(value, "%d", new(int)); err == nil {
		return false
	}
	switch strings.ToLower(value) {
	case "on", "off", "true", "false", "yes", "no":
		return false
	}
	if len(value) > 0 {
		lastChar := value[len(value)-1]
		if lastChar >= 'A' && lastChar <= 'Z' || lastChar >= 'a' && lastChar <= 'z' {
			numPart := value[:len(value)-1]
			if _, err := fmt.Sscanf(numPart, "%f", new(float64)); err == nil {
				return false
			}
		}
	}
	return true
}

// SavePostgreSQLConfig saves PostgreSQL config back to file.
func SavePostgreSQLConfig(config *DBConfig) error {
	if err := backupConfigFile(config.FilePath); err != nil {
		return err
	}

	var sb strings.Builder
	sb.WriteString("# EasyServer generated PostgreSQL configuration\n")
	sb.WriteString("# " + time.Now().Format("2006-01-02 15:04:05") + "\n\n")

	if len(config.Sections) > 0 {
		keys := make([]string, 0, len(config.Sections[0].Params))
		for key := range config.Sections[0].Params {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			value := config.Sections[0].Params[key]
			if pgNeedsQuote(value) {
				escaped := strings.ReplaceAll(value, "'", "''")
				sb.WriteString(fmt.Sprintf("%s = '%s'\n", key, escaped))
			} else {
				sb.WriteString(fmt.Sprintf("%s = %s\n", key, value))
			}
		}
	}

	dir := filepath.Dir(config.FilePath)
	os.MkdirAll(dir, 0755)
	return os.WriteFile(config.FilePath, []byte(sb.String()), 0644)
}

// GetPostgreSQLCommonParams returns metadata for common PostgreSQL parameters.
func GetPostgreSQLCommonParams() []ParamMeta {
	return []ParamMeta{
		{Key: "listen_addresses", Label: "监听地址", Description: "监听的 IP 地址，'*' 表示所有", Type: "text", Default: "localhost"},
		{Key: "port", Label: "端口", Description: "PostgreSQL 服务监听端口", Type: "number", Default: "5432"},
		{Key: "max_connections", Label: "最大连接数", Description: "允许的最大并发连接数", Type: "number", Default: "100"},
		{Key: "shared_buffers", Label: "共享缓冲区", Description: "共享缓冲区大小，建议设为内存的 25%", Type: "text", Unit: "MB/GB", Default: "128MB"},
		{Key: "work_mem", Label: "工作内存", Description: "每个排序/哈希操作的内存", Type: "text", Unit: "MB/KB", Default: "4MB"},
		{Key: "maintenance_work_mem", Label: "维护工作内存", Description: "VACUUM/CREATE INDEX 等维护操作的内存", Type: "text", Unit: "MB/GB", Default: "64MB"},
		{Key: "wal_level", Label: "WAL 级别", Description: "Write-Ahead 日志级别", Type: "select", Options: []string{"minimal", "replica", "logical"}, Default: "replica"},
		{Key: "max_wal_size", Label: "最大 WAL 大小", Description: "自动检查点之间的最大 WAL 大小", Type: "text", Unit: "MB/GB", Default: "1GB"},
		{Key: "min_wal_size", Label: "最小 WAL 大小", Description: "WAL 文件回收的最小大小", Type: "text", Unit: "MB/GB", Default: "80MB"},
		{Key: "log_destination", Label: "日志目标", Description: "日志输出目标", Type: "select", Options: []string{"stderr", "csvlog", "syslog"}, Default: "stderr"},
		{Key: "logging_collector", Label: "日志收集器", Description: "是否启用日志收集器", Type: "select", Options: []string{"on", "off"}, Default: "on"},
		{Key: "log_directory", Label: "日志目录", Description: "日志文件存储目录", Type: "text", Default: "pg_log"},
		{Key: "log_filename", Label: "日志文件名", Description: "日志文件名模式", Type: "text", Default: "postgresql-%Y-%m-%d_%H%M%S.log"},
		{Key: "password_encryption", Label: "密码加密", Description: "密码加密方式", Type: "select", Options: []string{"scram-sha-256", "md5"}, Default: "scram-sha-256"},
		{Key: "ssl", Label: "SSL", Description: "是否启用 SSL", Type: "select", Options: []string{"on", "off"}, Default: "off"},
	}
}

// FindRedisConfig finds the active Redis config file.
func FindRedisConfig() string {
	paths := []string{
		"/etc/redis/redis.conf",
		"/etc/redis.conf",
		"/opt/redis/redis.conf",
		"/usr/local/etc/redis.conf",
	}
	for _, path := range paths {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	return ""
}

// ParseRedisConfig parses a redis.conf file.
func ParseRedisConfig(filePath string) (*DBConfig, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("cannot open config file: %w", err)
	}
	defer file.Close()

	params := make(map[string]string)
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "include ") {
			continue
		}
		idx := strings.Index(line, " ")
		if idx == -1 {
			continue
		}
		key := line[:idx]
		value := strings.TrimSpace(line[idx+1:])
		if cidx := strings.Index(value, " #"); cidx != -1 {
			value = strings.TrimSpace(value[:cidx])
		}
		if key == "save" {
			if existing, ok := params["save"]; ok {
				params["save"] = existing + "\n" + value
			} else {
				params["save"] = value
			}
		} else {
			params[key] = value
		}
	}

	return &DBConfig{
		FilePath: filePath,
		Sections: []ConfigSection{
			{Name: "main", Params: params},
		},
	}, nil
}

// SaveRedisConfig saves Redis config back to file.
func SaveRedisConfig(config *DBConfig) error {
	if err := backupConfigFile(config.FilePath); err != nil {
		return err
	}

	var sb strings.Builder
	sb.WriteString("# EasyServer generated Redis configuration\n")
	sb.WriteString("# " + time.Now().Format("2006-01-02 15:04:05") + "\n\n")

	if len(config.Sections) > 0 {
		keys := make([]string, 0, len(config.Sections[0].Params))
		for key := range config.Sections[0].Params {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			value := config.Sections[0].Params[key]
			if key == "save" {
				lines := strings.Split(value, "\n")
				for _, l := range lines {
					l = strings.TrimSpace(l)
					if l != "" {
						sb.WriteString(fmt.Sprintf("save %s\n", l))
					}
				}
			} else {
				sb.WriteString(fmt.Sprintf("%s %s\n", key, value))
			}
		}
	}

	dir := filepath.Dir(config.FilePath)
	os.MkdirAll(dir, 0755)
	return os.WriteFile(config.FilePath, []byte(sb.String()), 0644)
}

// GetRedisCommonParams returns metadata for common Redis parameters.
func GetRedisCommonParams() []ParamMeta {
	return []ParamMeta{
		{Key: "bind", Label: "绑定地址", Description: "监听的 IP 地址", Type: "text", Default: "127.0.0.1"},
		{Key: "port", Label: "端口", Description: "Redis 服务监听端口", Type: "number", Default: "6379"},
		{Key: "protected-mode", Label: "保护模式", Description: "无密码时禁止外部访问", Type: "select", Options: []string{"yes", "no"}, Default: "yes"},
		{Key: "requirepass", Label: "访问密码", Description: "Redis 访问密码", Type: "text", Default: ""},
		{Key: "maxmemory", Label: "最大内存", Description: "Redis 最大内存使用量", Type: "text", Unit: "mb/gb", Default: "256mb"},
		{Key: "maxmemory-policy", Label: "内存淘汰策略", Description: "内存满时的 key 淘汰策略", Type: "select", Options: []string{"noeviction", "allkeys-lru", "volatile-lru", "allkeys-random", "volatile-random", "volatile-ttl"}, Default: "allkeys-lru"},
		{Key: "save", Label: "RDB 持久化", Description: "RDB 快照策略（秒 数据变更次数），多个策略用换行分隔", Type: "text", Default: "900 1"},
		{Key: "appendonly", Label: "AOF 持久化", Description: "是否启用 AOF 持久化", Type: "select", Options: []string{"yes", "no"}, Default: "no"},
		{Key: "appendfsync", Label: "AOF 同步策略", Description: "AOF 文件同步策略", Type: "select", Options: []string{"always", "everysec", "no"}, Default: "everysec"},
		{Key: "timeout", Label: "空闲超时", Description: "客户端空闲断开时间（秒），0 表示不断开", Type: "number", Unit: "秒", Default: "0"},
		{Key: "databases", Label: "数据库数量", Description: "Redis 数据库数量", Type: "number", Default: "16"},
		{Key: "tcp-backlog", Label: "TCP 积压", Description: "TCP 连接积压队列大小", Type: "number", Default: "511"},
		{Key: "tcp-keepalive", Label: "TCP 保活", Description: "TCP 保活探测间隔（秒）", Type: "number", Unit: "秒", Default: "300"},
		{Key: "loglevel", Label: "日志级别", Description: "Redis 日志级别", Type: "select", Options: []string{"debug", "verbose", "notice", "warning"}, Default: "notice"},
		{Key: "logfile", Label: "日志文件", Description: "日志文件路径，空表示标准输出", Type: "text", Default: ""},
	}
}

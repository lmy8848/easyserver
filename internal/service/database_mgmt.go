package service

import (
	"context"
	"fmt"
	"strings"

	"easyserver/internal/executor"
	"easyserver/internal/model"
	"easyserver/internal/repository"

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

type DatabaseMgmtService struct {
	repo     repository.DatabaseMgmtRepository
	executor executor.CommandExecutor
}

func NewDatabaseMgmtService(repo repository.DatabaseMgmtRepository, exec executor.CommandExecutor) *DatabaseMgmtService {
	return &DatabaseMgmtService{repo: repo, executor: exec}
}

// Database CRUD

func (s *DatabaseMgmtService) ListDatabases(ctx context.Context, dbServerID int64) ([]model.Database, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	return s.repo.ListDatabases(ctx, dbServerID)
}

func (s *DatabaseMgmtService) CreateDatabase(ctx context.Context, dbServerID int64, req *model.CreateDatabaseRequest) (*model.Database, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	// Get version info
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

	// Get server info
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

	// Create database via CLI
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

	return &model.Database{
		ID:          id,
		DBServerID:  dbServerID,
		DBVersionID: req.DBVersionID,
		Name:        req.Name,
		Charset:     charset,
		Status:      "active",
		Version:     version.Version,
	}, nil
}

func (s *DatabaseMgmtService) DeleteDatabase(ctx context.Context, dbServerID, id int64) error {
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

// DB User CRUD

func (s *DatabaseMgmtService) ListDBUsers(ctx context.Context, dbServerID int64) ([]model.DBUser, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	return s.repo.ListDBUsers(ctx, dbServerID)
}

func (s *DatabaseMgmtService) CreateDBUser(ctx context.Context, dbServerID int64, req *model.CreateDBUserRequest) (*model.DBUser, error) {
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

	// Check any version is running
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

	// Hash password before storing in SQLite
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("failed to hash password: %w", err)
	}

	id, err := s.repo.CreateDBUser(ctx, dbServerID, req.Username, string(hashedPassword), host)
	if err != nil {
		return nil, err
	}
	return &model.DBUser{
		ID:         id,
		DBServerID: dbServerID,
		Username:   req.Username,
		Host:       host,
	}, nil
}

func (s *DatabaseMgmtService) DeleteDBUser(ctx context.Context, dbServerID, id int64) error {
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

	// Check any version is running
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

func (s *DatabaseMgmtService) GrantPrivileges(ctx context.Context, dbServerID, userID int64, req *model.GrantRequest) error {
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

	// Validate privileges
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

// Validation functions

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

// escapeMySQLString escapes special characters for MySQL CLI strings
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

// escapePGString escapes single quotes for PostgreSQL strings (” is the PG escape)
func escapePGString(s string) string {
	return strings.ReplaceAll(s, "'", "''")
}

// GetDatabaseByID returns a database by its ID (exported for API handler)
func (s *DatabaseMgmtService) GetDatabaseByID(ctx context.Context, id int64) (*model.Database, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	return s.repo.GetDatabaseByID(ctx, id)
}

// GetServerByID returns a server by its ID (exported for API handler)
func (s *DatabaseMgmtService) GetServerByID(ctx context.Context, id int64) (*model.DBServer, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	return s.repo.GetServer(ctx, id)
}

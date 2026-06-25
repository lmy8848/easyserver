package service

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"strings"

	"easyserver/internal/executor"
	"easyserver/internal/model"

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
	db       *sql.DB
	executor executor.CommandExecutor
}

func NewDatabaseMgmtService(db *sql.DB, exec executor.CommandExecutor) *DatabaseMgmtService {
	return &DatabaseMgmtService{db: db, executor: exec}
}

// Database CRUD

func (s *DatabaseMgmtService) ListDatabases(ctx context.Context, dbServerID int64) ([]model.Database, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	rows, err := s.db.QueryContext(ctx, `SELECT d.id, d.db_server_id, d.db_version_id, d.name, d.charset, d.description,
		d.size_bytes, d.status, d.created_at, d.updated_at, COALESCE(v.version, '') as version
		FROM databases d
		LEFT JOIN db_versions v ON d.db_version_id = v.id
		WHERE d.db_server_id = ? ORDER BY d.id`, dbServerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var dbs []model.Database
	for rows.Next() {
		var d model.Database
		if err := rows.Scan(&d.ID, &d.DBServerID, &d.DBVersionID, &d.Name, &d.Charset, &d.Description,
			&d.SizeBytes, &d.Status, &d.CreatedAt, &d.UpdatedAt, &d.Version); err != nil {
			log.Printf("scan database row: %v", err)
			continue
		}
		dbs = append(dbs, d)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate databases: %w", err)
	}
	return dbs, nil
}

func (s *DatabaseMgmtService) CreateDatabase(ctx context.Context, dbServerID int64, req *model.CreateDatabaseRequest) (*model.Database, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	// Get version info
	version, err := s.getVersion(ctx, req.DBVersionID)
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
	server, err := s.getServer(ctx, dbServerID)
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

	result, err := s.db.ExecContext(ctx, `INSERT INTO databases (db_server_id, db_version_id, name, charset, description)
		VALUES (?, ?, ?, ?, ?)`, dbServerID, req.DBVersionID, req.Name, charset, req.Description)
	if err != nil {
		return nil, err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("get last insert id: %w", err)
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
	d, err := s.getDatabase(ctx, dbServerID, id)
	if err != nil {
		return fmt.Errorf("get database: %w", err)
	}
	if d == nil {
		return fmt.Errorf("database not found")
	}

	server, err := s.getServer(ctx, dbServerID)
	if err != nil {
		return fmt.Errorf("get server: %w", err)
	}
	if server == nil {
		return fmt.Errorf("database server not found")
	}

	version, err := s.getVersion(ctx, d.DBVersionID)
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

	_, err = s.db.ExecContext(ctx, "DELETE FROM databases WHERE id = ? AND db_server_id = ?", id, dbServerID)
	return err
}

// DB User CRUD

func (s *DatabaseMgmtService) ListDBUsers(ctx context.Context, dbServerID int64) ([]model.DBUser, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id, db_server_id, username, host, privileges, created_at
		FROM db_users WHERE db_server_id = ? ORDER BY id`, dbServerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []model.DBUser
	for rows.Next() {
		var u model.DBUser
		if err := rows.Scan(&u.ID, &u.DBServerID, &u.Username, &u.Host, &u.Privileges, &u.CreatedAt); err != nil {
			log.Printf("scan db user row: %v", err)
			continue
		}
		users = append(users, u)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate db users: %w", err)
	}
	return users, nil
}

func (s *DatabaseMgmtService) CreateDBUser(ctx context.Context, dbServerID int64, req *model.CreateDBUserRequest) (*model.DBUser, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	server, err := s.getServer(ctx, dbServerID)
	if err != nil {
		return nil, fmt.Errorf("get server: %w", err)
	}
	if server == nil {
		return nil, fmt.Errorf("database server not found")
	}

	// Check any version is running
	versions, err := s.listVersions(ctx, dbServerID)
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

	result, err := s.db.ExecContext(ctx, `INSERT INTO db_users (db_server_id, username, password, host) VALUES (?, ?, ?, ?)`,
		dbServerID, req.Username, string(hashedPassword), host)
	if err != nil {
		return nil, err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("get last insert id: %w", err)
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
	u, err := s.getDBUser(ctx, dbServerID, id)
	if err != nil {
		return fmt.Errorf("get user: %w", err)
	}
	if u == nil {
		return fmt.Errorf("user not found")
	}

	server, err := s.getServer(ctx, dbServerID)
	if err != nil {
		return fmt.Errorf("get server: %w", err)
	}
	if server == nil {
		return fmt.Errorf("database server not found")
	}

	// Check any version is running
	versions, err := s.listVersions(ctx, dbServerID)
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

	_, err = s.db.ExecContext(ctx, "DELETE FROM db_users WHERE id = ? AND db_server_id = ?", id, dbServerID)
	return err
}

func (s *DatabaseMgmtService) GrantPrivileges(ctx context.Context, dbServerID, userID int64, req *model.GrantRequest) error {
	if ctx == nil {
		ctx = context.Background()
	}
	u, err := s.getDBUser(ctx, dbServerID, userID)
	if err != nil {
		return fmt.Errorf("get user: %w", err)
	}
	if u == nil {
		return fmt.Errorf("user not found")
	}

	server, err := s.getServer(ctx, dbServerID)
	if err != nil {
		return fmt.Errorf("get server: %w", err)
	}
	if server == nil {
		return fmt.Errorf("database server not found")
	}

	version, err := s.getVersion(ctx, req.DBVersionID)
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
	if _, err := s.db.ExecContext(ctx, "UPDATE db_users SET privileges = ? WHERE id = ?", existing+privStr, userID); err != nil {
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

// Internal helpers

func (s *DatabaseMgmtService) getServer(ctx context.Context, id int64) (*model.DBServer, error) {
	ds := &model.DBServer{}
	err := s.db.QueryRowContext(ctx, `SELECT id, name, display_name, status FROM db_servers WHERE id = ?`, id).Scan(
		&ds.ID, &ds.Name, &ds.DisplayName, &ds.Status)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return ds, err
}

func (s *DatabaseMgmtService) getVersion(ctx context.Context, id int64) (*model.DBVersion, error) {
	v := &model.DBVersion{}
	err := s.db.QueryRowContext(ctx, `SELECT id, db_server_id, version, service_name, port, status FROM db_versions WHERE id = ?`, id).Scan(
		&v.ID, &v.DBServerID, &v.Version, &v.ServiceName, &v.Port, &v.Status)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return v, err
}

func (s *DatabaseMgmtService) listVersions(ctx context.Context, dbServerID int64) ([]model.DBVersion, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, db_server_id, version, service_name, port, status FROM db_versions WHERE db_server_id = ?`, dbServerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var versions []model.DBVersion
	for rows.Next() {
		var v model.DBVersion
		if err := rows.Scan(&v.ID, &v.DBServerID, &v.Version, &v.ServiceName, &v.Port, &v.Status); err != nil {
			log.Printf("scan version row: %v", err)
			continue
		}
		versions = append(versions, v)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate versions: %w", err)
	}
	return versions, nil
}

func (s *DatabaseMgmtService) getDatabase(ctx context.Context, dbServerID, id int64) (*model.Database, error) {
	d := &model.Database{}
	err := s.db.QueryRowContext(ctx, `SELECT id, db_server_id, db_version_id, name FROM databases WHERE id = ? AND db_server_id = ?`,
		id, dbServerID).Scan(&d.ID, &d.DBServerID, &d.DBVersionID, &d.Name)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return d, err
}

// GetDatabaseByID returns a database by its ID (exported for API handler)
func (s *DatabaseMgmtService) GetDatabaseByID(ctx context.Context, id int64) (*model.Database, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	d := &model.Database{}
	err := s.db.QueryRowContext(ctx, `SELECT id, db_server_id, db_version_id, name FROM databases WHERE id = ?`, id).Scan(
		&d.ID, &d.DBServerID, &d.DBVersionID, &d.Name)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return d, err
}

// GetServerByID returns a server by its ID (exported for API handler)
func (s *DatabaseMgmtService) GetServerByID(ctx context.Context, id int64) (*model.DBServer, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	return s.getServer(ctx, id)
}

func (s *DatabaseMgmtService) getDBUser(ctx context.Context, dbServerID, id int64) (*model.DBUser, error) {
	u := &model.DBUser{}
	err := s.db.QueryRowContext(ctx, `SELECT id, db_server_id, username, host FROM db_users WHERE id = ? AND db_server_id = ?`,
		id, dbServerID).Scan(&u.ID, &u.DBServerID, &u.Username, &u.Host)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return u, err
}

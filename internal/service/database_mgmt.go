package service

import (
	"database/sql"
	"fmt"
	"os/exec"
	"strings"

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
	db *sql.DB
}

func NewDatabaseMgmtService(db *sql.DB) *DatabaseMgmtService {
	return &DatabaseMgmtService{db: db}
}

// Database CRUD

func (s *DatabaseMgmtService) ListDatabases(dbServerID int64) ([]model.Database, error) {
	rows, err := s.db.Query(`SELECT d.id, d.db_server_id, d.db_version_id, d.name, d.charset, d.description,
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
		err := rows.Scan(&d.ID, &d.DBServerID, &d.DBVersionID, &d.Name, &d.Charset, &d.Description,
			&d.SizeBytes, &d.Status, &d.CreatedAt, &d.UpdatedAt, &d.Version)
		if err != nil {
			continue
		}
		dbs = append(dbs, d)
	}
	return dbs, nil
}

func (s *DatabaseMgmtService) CreateDatabase(dbServerID int64, req *model.CreateDatabaseRequest) (*model.Database, error) {
	// Get version info
	version, err := s.getVersion(req.DBVersionID)
	if err != nil || version == nil {
		return nil, fmt.Errorf("database version not found")
	}
	if version.Status != "running" && version.Status != "active" {
		return nil, fmt.Errorf("database version is not running")
	}

	// Get server info
	server, err := s.getServer(dbServerID)
	if err != nil || server == nil {
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
		out, err := exec.Command("mysql", "-e", fmt.Sprintf("CREATE DATABASE `%s` CHARACTER SET %s;", req.Name, charset)).CombinedOutput()
		if err != nil {
			return nil, fmt.Errorf("create database failed: %s", string(out))
		}
	case "postgresql":
		out, err := exec.Command("sudo", "-u", "postgres", "createdb", "-E", charset, req.Name).CombinedOutput()
		if err != nil {
			return nil, fmt.Errorf("create database failed: %s", string(out))
		}
	default:
		return nil, fmt.Errorf("database creation not supported for %s", server.Name)
	}

	result, err := s.db.Exec(`INSERT INTO databases (db_server_id, db_version_id, name, charset, description)
		VALUES (?, ?, ?, ?, ?)`, dbServerID, req.DBVersionID, req.Name, charset, req.Description)
	if err != nil {
		return nil, err
	}

	id, _ := result.LastInsertId()
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

func (s *DatabaseMgmtService) DeleteDatabase(dbServerID, id int64) error {
	d, err := s.getDatabase(dbServerID, id)
	if err != nil || d == nil {
		return fmt.Errorf("database not found")
	}

	server, _ := s.getServer(dbServerID)
	if server == nil {
		return fmt.Errorf("database server not found")
	}

	version, _ := s.getVersion(d.DBVersionID)
	if version == nil || version.Status != "running" {
		return fmt.Errorf("database version is not running")
	}

	switch server.Name {
	case "mysql":
		out, err := exec.Command("mysql", "-e", fmt.Sprintf("DROP DATABASE `%s`;", d.Name)).CombinedOutput()
		if err != nil {
			return fmt.Errorf("drop database failed: %s", string(out))
		}
	case "postgresql":
		out, err := exec.Command("sudo", "-u", "postgres", "dropdb", d.Name).CombinedOutput()
		if err != nil {
			return fmt.Errorf("drop database failed: %s", string(out))
		}
	default:
		return fmt.Errorf("database deletion not supported for %s", server.Name)
	}

	_, err = s.db.Exec("DELETE FROM databases WHERE id = ? AND db_server_id = ?", id, dbServerID)
	return err
}

// DB User CRUD

func (s *DatabaseMgmtService) ListDBUsers(dbServerID int64) ([]model.DBUser, error) {
	rows, err := s.db.Query(`SELECT id, db_server_id, username, host, privileges, created_at
		FROM db_users WHERE db_server_id = ? ORDER BY id`, dbServerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []model.DBUser
	for rows.Next() {
		var u model.DBUser
		err := rows.Scan(&u.ID, &u.DBServerID, &u.Username, &u.Host, &u.Privileges, &u.CreatedAt)
		if err != nil {
			continue
		}
		users = append(users, u)
	}
	return users, nil
}

func (s *DatabaseMgmtService) CreateDBUser(dbServerID int64, req *model.CreateDBUserRequest) (*model.DBUser, error) {
	server, err := s.getServer(dbServerID)
	if err != nil || server == nil {
		return nil, fmt.Errorf("database server not found")
	}

	// Check any version is running
	versions, _ := s.listVersions(dbServerID)
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
		sql := fmt.Sprintf("CREATE USER '%s'@'%s' IDENTIFIED BY '%s';", req.Username, host, escapeMySQLString(req.Password))
		out, err := exec.Command("mysql", "-e", sql).CombinedOutput()
		if err != nil {
			return nil, fmt.Errorf("create user failed: %s", string(out))
		}
	case "postgresql":
		out, err := exec.Command("sudo", "-u", "postgres", "psql", "-c",
			fmt.Sprintf("CREATE USER \"%s\" WITH PASSWORD '%s';", req.Username, escapePGString(req.Password))).CombinedOutput()
		if err != nil {
			return nil, fmt.Errorf("create user failed: %s", string(out))
		}
	default:
		return nil, fmt.Errorf("user creation not supported for %s", server.Name)
	}

	// Hash password before storing in SQLite
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("failed to hash password: %w", err)
	}

	result, err := s.db.Exec(`INSERT INTO db_users (db_server_id, username, password, host) VALUES (?, ?, ?, ?)`,
		dbServerID, req.Username, string(hashedPassword), host)
	if err != nil {
		return nil, err
	}

	id, _ := result.LastInsertId()
	return &model.DBUser{
		ID:         id,
		DBServerID: dbServerID,
		Username:   req.Username,
		Host:       host,
	}, nil
}

func (s *DatabaseMgmtService) DeleteDBUser(dbServerID, id int64) error {
	u, err := s.getDBUser(dbServerID, id)
	if err != nil || u == nil {
		return fmt.Errorf("user not found")
	}

	server, _ := s.getServer(dbServerID)
	if server == nil {
		return fmt.Errorf("database server not found")
	}

	// Check any version is running
	versions, _ := s.listVersions(dbServerID)
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
		sql := fmt.Sprintf("DROP USER '%s'@'%s';", u.Username, u.Host)
		out, err := exec.Command("mysql", "-e", sql).CombinedOutput()
		if err != nil {
			return fmt.Errorf("drop user failed: %s", string(out))
		}
	case "postgresql":
		out, err := exec.Command("sudo", "-u", "postgres", "psql", "-c",
			fmt.Sprintf("DROP USER \"%s\";", u.Username)).CombinedOutput()
		if err != nil {
			return fmt.Errorf("drop user failed: %s", string(out))
		}
	default:
		return fmt.Errorf("user deletion not supported for %s", server.Name)
	}

	_, err = s.db.Exec("DELETE FROM db_users WHERE id = ? AND db_server_id = ?", id, dbServerID)
	return err
}

func (s *DatabaseMgmtService) GrantPrivileges(dbServerID, userID int64, req *model.GrantRequest) error {
	u, err := s.getDBUser(dbServerID, userID)
	if err != nil || u == nil {
		return fmt.Errorf("user not found")
	}

	server, _ := s.getServer(dbServerID)
	if server == nil {
		return fmt.Errorf("database server not found")
	}

	version, _ := s.getVersion(req.DBVersionID)
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
		sql := fmt.Sprintf("GRANT %s ON `%s`.* TO '%s'@'%s'; FLUSH PRIVILEGES;", req.Privileges, req.Database, u.Username, u.Host)
		out, err := exec.Command("mysql", "-e", sql).CombinedOutput()
		if err != nil {
			return fmt.Errorf("grant failed: %s", string(out))
		}
	case "postgresql":
		sql := fmt.Sprintf("GRANT %s ON DATABASE \"%s\" TO \"%s\";", req.Privileges, req.Database, u.Username)
		out, err := exec.Command("sudo", "-u", "postgres", "psql", "-c", sql).CombinedOutput()
		if err != nil {
			return fmt.Errorf("grant failed: %s", string(out))
		}
	default:
		return fmt.Errorf("privilege grant not supported for %s", server.Name)
	}

	privStr := fmt.Sprintf("%s@%s", req.Privileges, req.Database)
	existing := u.Privileges
	if existing != "" {
		existing += ";"
	}
	s.db.Exec("UPDATE db_users SET privileges = ? WHERE id = ?", existing+privStr, userID)

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

// escapePGString escapes single quotes for PostgreSQL strings ('' is the PG escape)
func escapePGString(s string) string {
	return strings.ReplaceAll(s, "'", "''")
}

// Internal helpers

func (s *DatabaseMgmtService) getServer(id int64) (*model.DBServer, error) {
	ds := &model.DBServer{}
	err := s.db.QueryRow(`SELECT id, name, display_name, status FROM db_servers WHERE id = ?`, id).Scan(
		&ds.ID, &ds.Name, &ds.DisplayName, &ds.Status)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return ds, err
}

func (s *DatabaseMgmtService) getVersion(id int64) (*model.DBVersion, error) {
	v := &model.DBVersion{}
	err := s.db.QueryRow(`SELECT id, db_server_id, version, service_name, port, status FROM db_versions WHERE id = ?`, id).Scan(
		&v.ID, &v.DBServerID, &v.Version, &v.ServiceName, &v.Port, &v.Status)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return v, err
}

func (s *DatabaseMgmtService) listVersions(dbServerID int64) ([]model.DBVersion, error) {
	rows, err := s.db.Query(`SELECT id, db_server_id, version, service_name, port, status FROM db_versions WHERE db_server_id = ?`, dbServerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var versions []model.DBVersion
	for rows.Next() {
		var v model.DBVersion
		err := rows.Scan(&v.ID, &v.DBServerID, &v.Version, &v.ServiceName, &v.Port, &v.Status)
		if err != nil {
			continue
		}
		versions = append(versions, v)
	}
	return versions, nil
}

func (s *DatabaseMgmtService) getDatabase(dbServerID, id int64) (*model.Database, error) {
	d := &model.Database{}
	err := s.db.QueryRow(`SELECT id, db_server_id, db_version_id, name FROM databases WHERE id = ? AND db_server_id = ?`,
		id, dbServerID).Scan(&d.ID, &d.DBServerID, &d.DBVersionID, &d.Name)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return d, err
}

// GetDatabaseByID returns a database by its ID (exported for API handler)
func (s *DatabaseMgmtService) GetDatabaseByID(id int64) (*model.Database, error) {
	d := &model.Database{}
	err := s.db.QueryRow(`SELECT id, db_server_id, db_version_id, name FROM databases WHERE id = ?`, id).Scan(
		&d.ID, &d.DBServerID, &d.DBVersionID, &d.Name)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return d, err
}

// GetServerByID returns a server by its ID (exported for API handler)
func (s *DatabaseMgmtService) GetServerByID(id int64) (*model.DBServer, error) {
	return s.getServer(id)
}

func (s *DatabaseMgmtService) getDBUser(dbServerID, id int64) (*model.DBUser, error) {
	u := &model.DBUser{}
	err := s.db.QueryRow(`SELECT id, db_server_id, username, host FROM db_users WHERE id = ? AND db_server_id = ?`,
		id, dbServerID).Scan(&u.ID, &u.DBServerID, &u.Username, &u.Host)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return u, err
}

package service

import (
	"database/sql"
	"fmt"
	"log"
	"os/exec"
	"strconv"
	"strings"

	"easyserver/internal/model"
)

type DBServerService struct {
	db *sql.DB
}

func NewDBServerService(db *sql.DB) *DBServerService {
	return &DBServerService{db: db}
}

func (s *DBServerService) InitTables() error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS db_servers (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL UNIQUE,
			display_name TEXT NOT NULL,
			description TEXT DEFAULT '',
			default_port INTEGER DEFAULT 0,
			status TEXT DEFAULT 'not_installed',
			version TEXT DEFAULT '',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS db_versions (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			db_server_id INTEGER NOT NULL DEFAULT 0,
			version TEXT NOT NULL,
			service_name TEXT DEFAULT '',
			config_file TEXT DEFAULT '',
			data_dir TEXT DEFAULT '',
			port INTEGER DEFAULT 0,
			status TEXT DEFAULT 'stopped',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(db_server_id, version)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_db_versions_server ON db_versions(db_server_id)`,
		`CREATE TABLE IF NOT EXISTS databases (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			db_server_id INTEGER NOT NULL DEFAULT 0,
			db_version_id INTEGER NOT NULL DEFAULT 0,
			name TEXT NOT NULL,
			charset TEXT DEFAULT 'utf8mb4',
			description TEXT DEFAULT '',
			size_bytes INTEGER DEFAULT 0,
			status TEXT DEFAULT 'active',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_databases_server ON databases(db_server_id)`,
		`CREATE INDEX IF NOT EXISTS idx_databases_version ON databases(db_version_id)`,
		`CREATE TABLE IF NOT EXISTS db_users (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			db_server_id INTEGER NOT NULL DEFAULT 0,
			username TEXT NOT NULL,
			password TEXT DEFAULT '',
			host TEXT DEFAULT 'localhost',
			privileges TEXT DEFAULT '',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_db_users_server ON db_users(db_server_id)`,
	}
	for _, q := range queries {
		if _, err := s.db.Exec(q); err != nil {
			return err
		}
	}

	// Insert predefined entries
	for _, ds := range model.PredefinedDBServers() {
		var count int
		s.db.QueryRow("SELECT COUNT(*) FROM db_servers WHERE name = ?", ds.Name).Scan(&count)
		if count == 0 {
			s.db.Exec(`INSERT INTO db_servers (name, display_name, description, default_port)
				VALUES (?, ?, ?, ?)`,
				ds.Name, ds.DisplayName, ds.Description, ds.DefaultPort)
		}
	}

	// Migration: add columns to existing tables
	s.db.Exec("ALTER TABLE databases ADD COLUMN db_version_id INTEGER NOT NULL DEFAULT 0")

	return nil
}

// DB Server CRUD

func (s *DBServerService) List() ([]model.DBServer, error) {
	rows, err := s.db.Query(`SELECT id, name, display_name, description, default_port, status, version, created_at
		FROM db_servers ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var servers []model.DBServer
	for rows.Next() {
		var ds model.DBServer
		err := rows.Scan(&ds.ID, &ds.Name, &ds.DisplayName, &ds.Description,
			&ds.DefaultPort, &ds.Status, &ds.Version, &ds.CreatedAt)
		if err != nil {
			continue
		}
		servers = append(servers, ds)
	}
	return servers, nil
}

func (s *DBServerService) Get(id int64) (*model.DBServer, error) {
	ds := &model.DBServer{}
	err := s.db.QueryRow(`SELECT id, name, display_name, description, default_port, status, version, created_at
		FROM db_servers WHERE id = ?`, id).Scan(
		&ds.ID, &ds.Name, &ds.DisplayName, &ds.Description,
		&ds.DefaultPort, &ds.Status, &ds.Version, &ds.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return ds, nil
}

// Version management

func (s *DBServerService) ListVersions(dbServerID int64) ([]model.DBVersion, error) {
	rows, err := s.db.Query(`SELECT id, db_server_id, version, service_name, config_file, data_dir, port, status, created_at
		FROM db_versions WHERE db_server_id = ? ORDER BY id`, dbServerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var versions []model.DBVersion
	for rows.Next() {
		var v model.DBVersion
		err := rows.Scan(&v.ID, &v.DBServerID, &v.Version, &v.ServiceName,
			&v.ConfigFile, &v.DataDir, &v.Port, &v.Status, &v.CreatedAt)
		if err != nil {
			continue
		}
		versions = append(versions, v)
	}
	return versions, nil
}

func (s *DBServerService) GetVersion(id int64) (*model.DBVersion, error) {
	v := &model.DBVersion{}
	err := s.db.QueryRow(`SELECT id, db_server_id, version, service_name, config_file, data_dir, port, status, created_at
		FROM db_versions WHERE id = ?`, id).Scan(
		&v.ID, &v.DBServerID, &v.Version, &v.ServiceName,
		&v.ConfigFile, &v.DataDir, &v.Port, &v.Status, &v.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return v, nil
}

func (s *DBServerService) InstallVersion(dbServerID int64, req *model.CreateDBVersionRequest) (*model.DBVersion, error) {
	ds, err := s.Get(dbServerID)
	if err != nil || ds == nil {
		return nil, fmt.Errorf("database server not found")
	}

	// Check if version already installed
	var count int
	s.db.QueryRow("SELECT COUNT(*) FROM db_versions WHERE db_server_id = ? AND version = ?", dbServerID, req.Version).Scan(&count)
	if count > 0 {
		return nil, fmt.Errorf("version %s is already installed", req.Version)
	}

	// Find package name from templates
	packageName := ""
	templates := model.GetVersionTemplates(ds.Name)
	for _, t := range templates {
		if t.Version == req.Version {
			packageName = t.Package
			break
		}
	}
	if packageName == "" {
		packageName = fmt.Sprintf("%s-server", ds.Name)
	}

	// Install
	log.Printf("db: installing %s version %s (package: %s)", ds.Name, req.Version, packageName)
	exec.Command("apt-get", "update", "-y").Run()
	out, err := exec.Command("apt-get", "install", "-y", packageName).CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("install failed: %s", string(out))
	}

	// Detect service name
	serviceName := detectServiceName(ds.Name, req.Version)

	// Set port
	port := req.Port
	if port == 0 {
		port = ds.DefaultPort
	}

	// Enable and start
	exec.Command("systemctl", "enable", serviceName).Run()
	startOut, startErr := exec.Command("systemctl", "start", serviceName).CombinedOutput()
	status := "running"
	if startErr != nil {
		status = "stopped"
		log.Printf("db: failed to start %s: %s", serviceName, string(startOut))
	}

	// Save version record
	result, err := s.db.Exec(`INSERT INTO db_versions (db_server_id, version, service_name, port, status)
		VALUES (?, ?, ?, ?, ?)`, dbServerID, req.Version, serviceName, port, status)
	if err != nil {
		return nil, err
	}

	id, _ := result.LastInsertId()

	// Update server summary
	s.updateServerSummary(dbServerID)

	return &model.DBVersion{
		ID:          id,
		DBServerID:  dbServerID,
		Version:     req.Version,
		ServiceName: serviceName,
		Port:        port,
		Status:      "running",
	}, nil
}

func (s *DBServerService) UninstallVersion(versionID int64) error {
	v, err := s.GetVersion(versionID)
	if err != nil || v == nil {
		return fmt.Errorf("version not found")
	}

	// Check if databases exist for this version
	var dbCount int
	s.db.QueryRow("SELECT COUNT(*) FROM databases WHERE db_version_id = ?", versionID).Scan(&dbCount)
	if dbCount > 0 {
		return fmt.Errorf("cannot uninstall: %d databases still exist for this version", dbCount)
	}

	// Stop and remove
	exec.Command("systemctl", "stop", v.ServiceName).Run()
	exec.Command("systemctl", "disable", v.ServiceName).Run()

	ds, _ := s.Get(v.DBServerID)
	if ds != nil {
		templates := model.GetVersionTemplates(ds.Name)
		for _, t := range templates {
			if t.Version == v.Version {
				exec.Command("apt-get", "remove", "-y", t.Package).Run()
				break
			}
		}
	}

	s.db.Exec("DELETE FROM db_versions WHERE id = ?", versionID)
	s.updateServerSummary(v.DBServerID)
	return nil
}

func (s *DBServerService) StartVersion(versionID int64) error {
	v, err := s.GetVersion(versionID)
	if err != nil || v == nil {
		return fmt.Errorf("version not found")
	}
	out, err := exec.Command("systemctl", "start", v.ServiceName).CombinedOutput()
	if err != nil {
		return fmt.Errorf("start failed: %s", string(out))
	}
	s.db.Exec("UPDATE db_versions SET status = 'running' WHERE id = ?", versionID)
	s.updateServerSummary(v.DBServerID)
	return nil
}

func (s *DBServerService) StopVersion(versionID int64) error {
	v, err := s.GetVersion(versionID)
	if err != nil || v == nil {
		return fmt.Errorf("version not found")
	}
	out, err := exec.Command("systemctl", "stop", v.ServiceName).CombinedOutput()
	if err != nil {
		return fmt.Errorf("stop failed: %s", string(out))
	}
	s.db.Exec("UPDATE db_versions SET status = 'stopped' WHERE id = ?", versionID)
	s.updateServerSummary(v.DBServerID)
	return nil
}

func (s *DBServerService) RestartVersion(versionID int64) error {
	v, err := s.GetVersion(versionID)
	if err != nil || v == nil {
		return fmt.Errorf("version not found")
	}
	out, err := exec.Command("systemctl", "restart", v.ServiceName).CombinedOutput()
	if err != nil {
		return fmt.Errorf("restart failed: %s", string(out))
	}
	s.db.Exec("UPDATE db_versions SET status = 'running' WHERE id = ?", versionID)
	return nil
}

// UpdateVersionPort updates the port for a database version
func (s *DBServerService) UpdateVersionPort(versionID int64, newPort int) error {
	v, err := s.GetVersion(versionID)
	if err != nil || v == nil {
		return fmt.Errorf("version not found")
	}

	if v.Status == "running" {
		return fmt.Errorf("cannot change port while service is running. Stop it first")
	}

	s.db.Exec("UPDATE db_versions SET port = ? WHERE id = ?", newPort, versionID)
	return nil
}

func (s *DBServerService) GetVersionServiceLogs(versionID int64, lines int) (string, error) {
	v, err := s.GetVersion(versionID)
	if err != nil || v == nil {
		return "", fmt.Errorf("version not found")
	}
	if lines <= 0 {
		lines = 200
	}
	if lines > 5000 {
		lines = 5000
	}
	out, err := exec.Command("journalctl", "-u", v.ServiceName, "-n", strconv.Itoa(lines), "--no-pager").CombinedOutput()
	if err != nil {
		return string(out), nil
	}
	return string(out), nil
}

// RefreshStatus refreshes all versions for a server
func (s *DBServerService) RefreshStatus(dbServerID int64) {
	versions, _ := s.ListVersions(dbServerID)
	for _, v := range versions {
		out, _ := exec.Command("systemctl", "is-active", v.ServiceName).CombinedOutput()
		status := "stopped"
		if strings.TrimSpace(string(out)) == "active" {
			status = "running"
		}
		s.db.Exec("UPDATE db_versions SET status = ? WHERE id = ?", status, v.ID)
	}
	s.updateServerSummary(dbServerID)
}

func (s *DBServerService) RefreshAllStatus() {
	servers, _ := s.List()
	for _, ds := range servers {
		s.RefreshStatus(ds.ID)
	}
}

// updateServerSummary updates the server's status and version summary
func (s *DBServerService) updateServerSummary(dbServerID int64) {
	versions, _ := s.ListVersions(dbServerID)
	if len(versions) == 0 {
		s.db.Exec("UPDATE db_servers SET status = 'not_installed', version = '' WHERE id = ?", dbServerID)
		return
	}

	running := 0
	var versionParts []string
	for _, v := range versions {
		if v.Status == "running" || v.Status == "active" {
			running++
		}
		versionParts = append(versionParts, v.Version)
	}

	status := "stopped"
	if running == len(versions) {
		status = "running"
	} else if running > 0 {
		status = "partial"
	}

	summary := strings.Join(versionParts, ", ")
	s.db.Exec("UPDATE db_servers SET status = ?, version = ? WHERE id = ?", status, summary, dbServerID)
}

// detectServiceName detects the systemd service name for a database version
func detectServiceName(dbName, version string) string {
	switch dbName {
	case "mysql":
		return "mariadb"
	case "postgresql":
		// PostgreSQL uses version-specific service: postgresql@15-main or postgresql
		// Try version-specific first, fallback to generic
		versionService := fmt.Sprintf("postgresql@%s-main", version)
		if _, err := exec.LookPath("pg_ctlcluster"); err == nil {
			return versionService
		}
		return "postgresql"
	case "redis":
		return "redis-server"
	}
	return dbName
}

// GetMySQLCmd returns the mysql command for executing SQL
func (s *DBServerService) GetMySQLCmd() string {
	if _, err := exec.LookPath("mysql"); err == nil {
		return "mysql"
	}
	return ""
}

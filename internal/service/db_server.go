package service

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os/exec"
	"strconv"
	"strings"

	"easyserver/internal/executor"
	"easyserver/internal/model"
	"easyserver/internal/repository"
)

type DBServerService struct {
	db       *sql.DB
	executor executor.CommandExecutor
	repo     repository.DBServerRepository
}

func NewDBServerService(db *sql.DB, exec executor.CommandExecutor, repo repository.DBServerRepository) *DBServerService {
	return &DBServerService{db: db, executor: exec, repo: repo}
}

// Deprecated: InitTables is kept for backward compatibility only.
// Table creation is now handled by the migration system (migrations/ directory).
func (s *DBServerService) InitTables(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
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
		if _, err := s.db.ExecContext(ctx, q); err != nil {
			return err
		}
	}

	// Insert predefined entries
	for _, ds := range model.PredefinedDBServers() {
		var count int
		s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM db_servers WHERE name = ?", ds.Name).Scan(&count)
		if count == 0 {
			s.db.ExecContext(ctx, `INSERT INTO db_servers (name, display_name, description, default_port)
				VALUES (?, ?, ?, ?)`,
				ds.Name, ds.DisplayName, ds.Description, ds.DefaultPort)
		}
	}

	// Migration: add columns to existing tables (ignore error if column already exists)
	if _, err := s.db.ExecContext(ctx, "ALTER TABLE databases ADD COLUMN db_version_id INTEGER NOT NULL DEFAULT 0"); err != nil {
		// Column may already exist - this is expected on subsequent runs
		log.Printf("db migration: %v (may be expected if column exists)", err)
	}

	return nil
}

// SeedPredefinedServers inserts predefined database server entries if not exists.
// Called at startup to ensure default entries are present.
func (s *DBServerService) SeedPredefinedServers(ctx context.Context) {
	if ctx == nil {
		ctx = context.Background()
	}
	for _, ds := range model.PredefinedDBServers() {
		if err := s.repo.SeedServer(ctx, ds.Name, ds.DisplayName, ds.Description, ds.DefaultPort); err != nil {
			log.Printf("seed server %s: %v", ds.Name, err)
		}
	}
}

// DB Server CRUD

func (s *DBServerService) List(ctx context.Context) ([]model.DBServer, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	return s.repo.ListServers(ctx)
}

func (s *DBServerService) Get(ctx context.Context, id int64) (*model.DBServer, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	return s.repo.GetServer(ctx, id)
}

// Version management

func (s *DBServerService) ListVersions(ctx context.Context, dbServerID int64) ([]model.DBVersion, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	return s.repo.ListVersions(ctx, dbServerID)
}

func (s *DBServerService) GetVersion(ctx context.Context, id int64) (*model.DBVersion, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	return s.repo.GetVersion(ctx, id)
}

func (s *DBServerService) InstallVersion(ctx context.Context, dbServerID int64, req *model.CreateDBVersionRequest) (*model.DBVersion, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	ds, err := s.Get(ctx, dbServerID)
	if err != nil || ds == nil {
		return nil, fmt.Errorf("database server not found")
	}

	// Check if version already installed
	count, err := s.repo.CountVersionsByServerAndVersion(ctx, dbServerID, req.Version)
	if err != nil {
		return nil, err
	}
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
	s.executor.RunCombined(ctx, "apt-get", "update", "-y")
	out, _, err := s.executor.RunCombined(ctx, "apt-get", "install", "-y", packageName)
	if err != nil {
		return nil, fmt.Errorf("install failed: %s", out)
	}

	// Detect service name
	serviceName := detectServiceName(ds.Name, req.Version)

	// Set port
	port := req.Port
	if port == 0 {
		port = ds.DefaultPort
	}

	// Enable and start
	s.executor.RunCombined(ctx, "systemctl", "enable", serviceName)
	startOut, _, startErr := s.executor.RunCombined(ctx, "systemctl", "start", serviceName)
	status := "running"
	if startErr != nil {
		status = "stopped"
		log.Printf("db: failed to start %s: %s", serviceName, startOut)
	}

	// Save version record
	id, err := s.repo.CreateVersion(ctx, dbServerID, req.Version, serviceName, port, status)
	if err != nil {
		return nil, err
	}

	// Update server summary
	s.updateServerSummary(ctx, dbServerID)

	return &model.DBVersion{
		ID:          id,
		DBServerID:  dbServerID,
		Version:     req.Version,
		ServiceName: serviceName,
		Port:        port,
		Status:      status,
	}, nil
}

func (s *DBServerService) UninstallVersion(ctx context.Context, versionID int64) error {
	if ctx == nil {
		ctx = context.Background()
	}
	v, err := s.GetVersion(ctx, versionID)
	if err != nil || v == nil {
		return fmt.Errorf("version not found")
	}

	// Check if databases exist for this version
	dbCount, err := s.repo.CountDatabasesByVersion(ctx, versionID)
	if err != nil {
		return err
	}
	if dbCount > 0 {
		return fmt.Errorf("cannot uninstall: %d databases still exist for this version", dbCount)
	}

	// Stop and remove
	s.executor.RunCombined(ctx, "systemctl", "stop", v.ServiceName)
	s.executor.RunCombined(ctx, "systemctl", "disable", v.ServiceName)

	ds, _ := s.Get(ctx, v.DBServerID)
	if ds != nil {
		templates := model.GetVersionTemplates(ds.Name)
		for _, t := range templates {
			if t.Version == v.Version {
				s.executor.RunCombined(ctx, "apt-get", "remove", "-y", t.Package)
				break
			}
		}
	}

	s.repo.DeleteVersion(ctx, versionID)
	s.updateServerSummary(ctx, v.DBServerID)
	return nil
}

func (s *DBServerService) StartVersion(ctx context.Context, versionID int64) error {
	if ctx == nil {
		ctx = context.Background()
	}
	v, err := s.GetVersion(ctx, versionID)
	if err != nil || v == nil {
		return fmt.Errorf("version not found")
	}
	out, _, err := s.executor.RunCombined(ctx, "systemctl", "start", v.ServiceName)
	if err != nil {
		return fmt.Errorf("start failed: %s", out)
	}
	s.repo.UpdateVersionStatus(ctx, versionID, "running")
	s.updateServerSummary(ctx, v.DBServerID)
	return nil
}

func (s *DBServerService) StopVersion(ctx context.Context, versionID int64) error {
	if ctx == nil {
		ctx = context.Background()
	}
	v, err := s.GetVersion(ctx, versionID)
	if err != nil || v == nil {
		return fmt.Errorf("version not found")
	}
	out, _, err := s.executor.RunCombined(ctx, "systemctl", "stop", v.ServiceName)
	if err != nil {
		return fmt.Errorf("stop failed: %s", out)
	}
	s.repo.UpdateVersionStatus(ctx, versionID, "stopped")
	s.updateServerSummary(ctx, v.DBServerID)
	return nil
}

func (s *DBServerService) RestartVersion(ctx context.Context, versionID int64) error {
	if ctx == nil {
		ctx = context.Background()
	}
	v, err := s.GetVersion(ctx, versionID)
	if err != nil || v == nil {
		return fmt.Errorf("version not found")
	}
	out, _, err := s.executor.RunCombined(ctx, "systemctl", "restart", v.ServiceName)
	if err != nil {
		return fmt.Errorf("restart failed: %s", out)
	}
	s.repo.UpdateVersionStatus(ctx, versionID, "running")
	s.updateServerSummary(ctx, v.DBServerID)
	return nil
}

// UpdateVersionPort updates the port for a database version
func (s *DBServerService) UpdateVersionPort(ctx context.Context, versionID int64, newPort int) error {
	if ctx == nil {
		ctx = context.Background()
	}
	v, err := s.GetVersion(ctx, versionID)
	if err != nil || v == nil {
		return fmt.Errorf("version not found")
	}

	if v.Status == "running" {
		return fmt.Errorf("cannot change port while service is running. Stop it first")
	}

	return s.repo.UpdateVersionPort(ctx, versionID, newPort)
}

func (s *DBServerService) GetVersionServiceLogs(ctx context.Context, versionID int64, lines int) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	v, err := s.GetVersion(ctx, versionID)
	if err != nil || v == nil {
		return "", fmt.Errorf("version not found")
	}
	if lines <= 0 {
		lines = 200
	}
	if lines > 5000 {
		lines = 5000
	}
	out, _, err := s.executor.RunCombined(ctx, "journalctl", "-u", v.ServiceName, "-n", strconv.Itoa(lines), "--no-pager")
	if err != nil {
		return out, nil
	}
	return out, nil
}

// RefreshStatus refreshes all versions for a server
func (s *DBServerService) RefreshStatus(ctx context.Context, dbServerID int64) {
	if ctx == nil {
		ctx = context.Background()
	}
	versions, _ := s.ListVersions(ctx, dbServerID)
	for _, v := range versions {
		out, _, _ := s.executor.RunCombined(ctx, "systemctl", "is-active", v.ServiceName)
		status := "stopped"
		if strings.TrimSpace(out) == "active" {
			status = "running"
		}
		s.repo.UpdateVersionStatus(ctx, v.ID, status)
	}
	s.updateServerSummary(ctx, dbServerID)
}

func (s *DBServerService) RefreshAllStatus(ctx context.Context) {
	if ctx == nil {
		ctx = context.Background()
	}
	servers, _ := s.List(ctx)
	for _, ds := range servers {
		s.RefreshStatus(ctx, ds.ID)
	}
}

// updateServerSummary updates the server's status and version summary
func (s *DBServerService) updateServerSummary(ctx context.Context, dbServerID int64) {
	versions, _ := s.ListVersions(ctx, dbServerID)
	if len(versions) == 0 {
		s.repo.UpdateServerStatus(ctx, dbServerID, "not_installed", "")
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
	s.repo.UpdateServerStatus(ctx, dbServerID, status, summary)
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
func (s *DBServerService) GetMySQLCmd(ctx context.Context) string {
	if ctx == nil {
		ctx = context.Background()
	}
	if _, err := exec.LookPath("mariadb"); err == nil {
		return "mariadb"
	}
	if _, err := exec.LookPath("mysql"); err == nil {
		return "mysql"
	}
	return ""
}

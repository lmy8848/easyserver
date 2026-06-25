package dbserver

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"

	"easyserver/internal/executor"
	"easyserver/internal/model"
	"easyserver/internal/repository"
)

type Service struct {
	executor executor.CommandExecutor
	repo     repository.DBServerRepository
}

func NewService(exec executor.CommandExecutor, repo repository.DBServerRepository) *Service {
	return &Service{executor: exec, repo: repo}
}

// SeedPredefinedServers inserts predefined database server entries if not exists.
// Called at startup to ensure default entries are present.
func (s *Service) SeedPredefinedServers(ctx context.Context) {
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

func (s *Service) List(ctx context.Context) ([]model.DBServer, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	return s.repo.ListServers(ctx)
}

func (s *Service) Get(ctx context.Context, id int64) (*model.DBServer, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	return s.repo.GetServer(ctx, id)
}

// Version management

func (s *Service) ListVersions(ctx context.Context, dbServerID int64) ([]model.DBVersion, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	return s.repo.ListVersions(ctx, dbServerID)
}

func (s *Service) GetVersion(ctx context.Context, id int64) (*model.DBVersion, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	return s.repo.GetVersion(ctx, id)
}

func (s *Service) InstallVersion(ctx context.Context, dbServerID int64, req *model.CreateDBVersionRequest) (*model.DBVersion, error) {
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
	serviceName := s.detectServiceName(ds.Name, req.Version)

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

func (s *Service) UninstallVersion(ctx context.Context, versionID int64) error {
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

func (s *Service) StartVersion(ctx context.Context, versionID int64) error {
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

func (s *Service) StopVersion(ctx context.Context, versionID int64) error {
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

func (s *Service) RestartVersion(ctx context.Context, versionID int64) error {
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
func (s *Service) UpdateVersionPort(ctx context.Context, versionID int64, newPort int) error {
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

func (s *Service) GetVersionServiceLogs(ctx context.Context, versionID int64, lines int) (string, error) {
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
func (s *Service) RefreshStatus(ctx context.Context, dbServerID int64) {
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

func (s *Service) RefreshAllStatus(ctx context.Context) {
	if ctx == nil {
		ctx = context.Background()
	}
	servers, _ := s.List(ctx)
	for _, ds := range servers {
		s.RefreshStatus(ctx, ds.ID)
	}
}

// updateServerSummary updates the server's status and version summary
func (s *Service) updateServerSummary(ctx context.Context, dbServerID int64) {
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
func (s *Service) detectServiceName(dbName, version string) string {
	switch dbName {
	case "mysql":
		return "mariadb"
	case "postgresql":
		// PostgreSQL uses version-specific service: postgresql@15-main or postgresql
		// Try version-specific first, fallback to generic
		versionService := fmt.Sprintf("postgresql@%s-main", version)
		if _, err := s.executor.LookPath("pg_ctlcluster"); err == nil {
			return versionService
		}
		return "postgresql"
	case "redis":
		return "redis-server"
	}
	return dbName
}

// GetMySQLCmd returns the mysql command for executing SQL
func (s *Service) GetMySQLCmd(ctx context.Context) string {
	if ctx == nil {
		ctx = context.Background()
	}
	if _, err := s.executor.LookPath("mariadb"); err == nil {
		return "mariadb"
	}
	if _, err := s.executor.LookPath("mysql"); err == nil {
		return "mysql"
	}
	return ""
}

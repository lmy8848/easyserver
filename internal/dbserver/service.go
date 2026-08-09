package dbserver

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"easyserver/internal/deploy"
	"easyserver/internal/infra/executor"
)

type Service struct {
	executor      executor.CommandExecutor
	repo          Repository
	runtime       DatabaseRuntime
	encryptionKey []byte
}

func NewServiceWithEncryptionKey(exec executor.CommandExecutor, repo Repository, encryptionKey string) *Service {
	return &Service{
		executor:      exec,
		repo:          repo,
		runtime:       NewCLIContainerRuntime(exec),
		encryptionKey: []byte(encryptionKey),
	}
}

// NewServiceWithRuntime is the test seam for lifecycle behavior.
func NewServiceWithRuntime(repo Repository, runtime DatabaseRuntime, encryptionKey string) *Service {
	return &Service{repo: repo, runtime: runtime, encryptionKey: []byte(encryptionKey)}
}

// SeedPredefinedServers inserts predefined database server entries if not exists.
// Called at startup to ensure default entries are present.
func (s *Service) SeedPredefinedServers(ctx context.Context) {
	if ctx == nil {
		ctx = context.Background()
	}
	for _, ds := range PredefinedDBServers() {
		if err := s.repo.SeedServer(ctx, ds.Name, ds.DisplayName, ds.Description, ds.DefaultPort); err != nil {
			log.Printf("seed server %s: %v", ds.Name, err)
		}
	}
}

// DB Server CRUD

func (s *Service) List(ctx context.Context) ([]DBServer, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	return s.repo.ListServers(ctx)
}

func (s *Service) Get(ctx context.Context, id int64) (*DBServer, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	return s.repo.GetServer(ctx, id)
}

// Version management

func (s *Service) ListVersions(ctx context.Context, dbServerID int64) ([]DBInstance, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	ds, err := s.Get(ctx, dbServerID)
	if err != nil {
		return nil, fmt.Errorf("get server: %w", err)
	}
	if ds == nil {
		return nil, fmt.Errorf("database server not found")
	}
	return s.repo.ListVersions(ctx, dbServerID)
}

func (s *Service) GetVersion(ctx context.Context, id int64) (*DBInstance, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	return s.repo.GetVersion(ctx, id)
}

func (s *Service) InstallVersion(ctx context.Context, dbServerID int64, req *CreateDBInstanceRequest) (*DBInstance, error) {
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

	// Resolve an immutable container image from the engine's supported templates.
	image := ""
	templates := GetVersionTemplates(ds.Name)
	for _, t := range templates {
		if t.Version == req.Version {
			image = t.Image
			break
		}
	}
	if image == "" {
		return nil, fmt.Errorf("unsupported database image version %s", req.Version)
	}
	runtimeName := strings.ToLower(strings.TrimSpace(req.Runtime))
	if runtimeName == "" {
		runtimeName = "docker"
	}
	if runtimeName != "docker" && runtimeName != "podman" {
		return nil, fmt.Errorf("unsupported container runtime %q", runtimeName)
	}
	port := req.Port
	if port == 0 {
		port = ds.DefaultPort
	}
	if port < 1 || port > 65535 {
		return nil, fmt.Errorf("port must be between 1 and 65535")
	}
	bindAddress := strings.TrimSpace(req.BindAddress)
	if bindAddress == "" {
		bindAddress = "127.0.0.1"
	}
	containerName := fmt.Sprintf("easyserver-db-%s-%s", sanitizeName(ds.Name), sanitizeName(req.Version))
	volumeName := containerName + "-data"
	password, err := generateAdminPassword()
	if err != nil {
		return nil, err
	}
	adminUser := "root"
	if ds.Name == "postgresql" {
		adminUser = "postgres"
	}
	if len(s.encryptionKey) != 32 {
		return nil, fmt.Errorf("database encryption key must be configured")
	}
	encryptedPassword, err := deploy.Encrypt(password, s.encryptionKey)
	if err != nil {
		return nil, err
	}

	spec := containerSpec(ds.Name, runtimeName, req.Version, image, containerName, volumeName, bindAddress, port, adminUser, password)
	if err := s.runtime.Create(ctx, spec); err != nil {
		return nil, err
	}
	// Redis loads a config file on startup; seed the mounted config volume with
	// an initial file carrying the admin password before the first start.
	if ds.Name == "redis" {
		if err := seedRedisConfig(ctx, s.runtime, runtimeName, containerName, password); err != nil {
			_ = s.runtime.Remove(ctx, runtimeName, containerName)
			return nil, err
		}
	}
	if err := s.runtime.Start(ctx, runtimeName, containerName); err != nil {
		_ = s.runtime.Remove(ctx, runtimeName, containerName)
		return nil, fmt.Errorf("start database container: %w", err)
	}
	statusInfo, err := waitForHealthy(ctx, s.runtime, runtimeName, containerName, 2*time.Minute)
	if err != nil {
		_ = s.runtime.Remove(ctx, runtimeName, containerName)
		return nil, err
	}
	v := &DBInstance{DBServerID: dbServerID, Version: req.Version, ContainerName: containerName,
		Port: port, Status: "running", CreatedAt: "", Runtime: runtimeName, Image: image,
		ContainerID: containerName, VolumeName: volumeName, ConfigDir: spec.ConfigDir, BindAddress: bindAddress,
		AdminUser: adminUser, AdminPassword: encryptedPassword, AdminPasswordPlain: password, HealthStatus: statusInfo.Health}
	id, err := s.repo.CreateContainerVersion(ctx, v)
	if err != nil {
		_ = s.runtime.Remove(ctx, runtimeName, containerName)
		return nil, err
	}
	v.ID = id

	s.updateServerSummary(ctx, dbServerID)
	return v, nil
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

	if err := s.runtime.Remove(ctx, v.Runtime, v.ContainerName); err != nil {
		return fmt.Errorf("remove database container: %w", err)
	}
	s.repo.DeleteVersion(ctx, versionID)
	s.updateServerSummary(ctx, v.DBServerID)
	return nil
}

// DestroyVersion removes the managed container, its data volume and metadata.
func (s *Service) DestroyVersion(ctx context.Context, versionID int64) error {
	v, err := s.GetVersion(ctx, versionID)
	if err != nil || v == nil {
		return fmt.Errorf("version not found")
	}
	if count, err := s.repo.CountDatabasesByVersion(ctx, versionID); err != nil {
		return err
	} else if count > 0 {
		return fmt.Errorf("cannot destroy: %d databases still exist for this instance", count)
	}
	if err := s.runtime.Remove(ctx, v.Runtime, v.ContainerName); err != nil {
		return fmt.Errorf("remove database container: %w", err)
	}
	if err := s.runtime.RemoveVolume(ctx, v.Runtime, v.VolumeName); err != nil {
		return fmt.Errorf("remove database volume: %w", err)
	}
	if v.ConfigDir != "" {
		if err := s.runtime.RemoveVolume(ctx, v.Runtime, v.ContainerName+"-config"); err != nil {
			return fmt.Errorf("remove database config volume: %w", err)
		}
	}
	if err := s.repo.DeleteVersion(ctx, versionID); err != nil {
		return err
	}
	s.updateServerSummary(ctx, v.DBServerID)
	return nil
}

// ResetAdminPassword rotates the administrator password and returns it once.
func (s *Service) ResetAdminPassword(ctx context.Context, versionID int64) (string, error) {
	v, err := s.GetVersion(ctx, versionID)
	if err != nil || v == nil {
		return "", fmt.Errorf("version not found")
	}
	if v.Runtime == "" || v.ContainerName == "" || len(s.encryptionKey) != 32 {
		return "", fmt.Errorf("database instance encryption is not configured")
	}
	oldPassword, err := s.decryptAdminPassword(v)
	if err != nil {
		return "", err
	}
	password, err := generateAdminPassword()
	if err != nil {
		return "", err
	}
	switch {
	case strings.HasPrefix(strings.ToLower(v.Image), "mysql"):
		if _, err := s.runtime.Exec(ctx, v.Runtime, v.ContainerName, "mysql", "-uroot", "-p"+oldPassword, "-e", "ALTER USER 'root'@'localhost' IDENTIFIED BY '"+strings.ReplaceAll(password, "'", "''")+"';"); err != nil {
			return "", fmt.Errorf("reset MySQL password: %w", err)
		}
	case strings.HasPrefix(strings.ToLower(v.Image), "postgres"):
		if _, err := s.runtime.Exec(ctx, v.Runtime, v.ContainerName, "psql", "-U", "postgres", "-c", "ALTER USER postgres WITH PASSWORD '"+strings.ReplaceAll(password, "'", "''")+"';"); err != nil {
			return "", fmt.Errorf("reset PostgreSQL password: %w", err)
		}
	case strings.HasPrefix(strings.ToLower(v.Image), "redis"):
		if _, err := s.runtime.Exec(ctx, v.Runtime, v.ContainerName, "redis-cli", "-a", oldPassword, "CONFIG", "SET", "requirepass", password); err != nil {
			return "", fmt.Errorf("reset Redis password: %w", err)
		}
	default:
		return "", fmt.Errorf("password reset is not supported for this database image")
	}
	encrypted, err := deploy.Encrypt(password, s.encryptionKey)
	if err != nil {
		return "", err
	}
	if err := s.repo.UpdateVersionPassword(ctx, versionID, encrypted); err != nil {
		return "", err
	}
	return password, nil
}

func (s *Service) decryptAdminPassword(v *DBInstance) (string, error) {
	if len(s.encryptionKey) != 32 {
		return "", fmt.Errorf("database instance encryption is not configured")
	}
	password, err := deploy.Decrypt(v.AdminPassword, s.encryptionKey)
	if err != nil {
		return "", fmt.Errorf("decrypt administrator password: %w", err)
	}
	return password, nil
}

func (s *Service) StartVersion(ctx context.Context, versionID int64) error {
	if ctx == nil {
		ctx = context.Background()
	}
	v, err := s.GetVersion(ctx, versionID)
	if err != nil || v == nil {
		return fmt.Errorf("version not found")
	}
	if err := s.runtime.Start(ctx, v.Runtime, v.ContainerName); err != nil {
		return fmt.Errorf("start failed: %w", err)
	}
	if _, err := waitForHealthy(ctx, s.runtime, v.Runtime, v.ContainerName, 2*time.Minute); err != nil {
		return err
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
	if err := s.runtime.Stop(ctx, v.Runtime, v.ContainerName); err != nil {
		return fmt.Errorf("stop failed: %w", err)
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
	if err := s.runtime.Restart(ctx, v.Runtime, v.ContainerName); err != nil {
		return fmt.Errorf("restart failed: %w", err)
	}
	if _, err := waitForHealthy(ctx, s.runtime, v.Runtime, v.ContainerName, 2*time.Minute); err != nil {
		return err
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
	if newPort < 1 || newPort > 65535 {
		return fmt.Errorf("port must be between 1 and 65535")
	}
	ds, err := s.Get(ctx, v.DBServerID)
	if err != nil || ds == nil {
		return fmt.Errorf("database engine not found")
	}
	password, err := s.decryptAdminPassword(v)
	if err != nil {
		return err
	}
	if err := s.runtime.Remove(ctx, v.Runtime, v.ContainerName); err != nil {
		return fmt.Errorf("remove old database container: %w", err)
	}
	spec := containerSpec(ds.Name, v.Runtime, v.Version, v.Image, v.ContainerName, v.VolumeName, v.BindAddress, newPort, v.AdminUser, password)
	if err := s.runtime.Create(ctx, spec); err != nil {
		return fmt.Errorf("recreate database container: %w", err)
	}
	if err := s.runtime.Start(ctx, v.Runtime, v.ContainerName); err != nil {
		return fmt.Errorf("start database container: %w", err)
	}
	if _, err := waitForHealthy(ctx, s.runtime, v.Runtime, v.ContainerName, 2*time.Minute); err != nil {
		return err
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
	return s.runtime.Logs(ctx, v.Runtime, v.ContainerName, lines)
}

// GetVersionConfig reads the engine configuration from inside its container.
func (s *Service) GetVersionConfig(ctx context.Context, versionID int64) (string, string, error) {
	v, err := s.GetVersion(ctx, versionID)
	if err != nil || v == nil {
		return "", "", fmt.Errorf("version not found")
	}
	path := configPathForImage(v.Image)
	out, err := s.runtime.Exec(ctx, v.Runtime, v.ContainerName, "cat", path)
	return out, path, err
}

// SaveVersionConfig writes engine configuration inside the managed container.
func (s *Service) SaveVersionConfig(ctx context.Context, versionID int64, content string) error {
	if len(content) > 256*1024 {
		return fmt.Errorf("configuration is too large")
	}
	v, err := s.GetVersion(ctx, versionID)
	if err != nil || v == nil {
		return fmt.Errorf("version not found")
	}
	path := configPathForImage(v.Image)
	encoded := base64.StdEncoding.EncodeToString([]byte(content))
	if _, err := s.runtime.Exec(ctx, v.Runtime, v.ContainerName, "sh", "-c", "echo "+encoded+" | base64 -d > "+path); err != nil {
		return fmt.Errorf("save configuration: %w", err)
	}
	return nil
}

func configPathForImage(image string) string {
	image = strings.ToLower(image)
	switch {
	case strings.HasPrefix(image, "mysql"):
		return "/etc/mysql/conf.d/easyserver.cnf"
	case strings.HasPrefix(image, "postgres"):
		return "/var/lib/postgresql/data/postgresql.conf"
	default:
		return "/usr/local/etc/redis/redis.conf"
	}
}

// seedRedisConfig writes an initial redis.conf into the freshly-created config
// volume so `redis-server <config>` can load it on first start. The file is
// staged on the host, then copied into the container's mounted config dir.
func seedRedisConfig(ctx context.Context, runtime DatabaseRuntime, runtimeName, container, password string) error {
	content := "requirepass " + password + "\n"
	tmp, err := os.CreateTemp("", "easyserver-redis-*.conf")
	if err != nil {
		return fmt.Errorf("stage redis config: %w", err)
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.WriteString(content); err != nil {
		tmp.Close()
		return fmt.Errorf("write staged redis config: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close staged redis config: %w", err)
	}
	if err := runtime.CopyTo(ctx, runtimeName, container, tmp.Name(), "/usr/local/etc/redis/redis.conf"); err != nil {
		return fmt.Errorf("seed redis config: %w", err)
	}
	return nil
}

// RefreshStatus refreshes all versions for a server
func (s *Service) RefreshStatus(ctx context.Context, dbServerID int64) {
	if ctx == nil {
		ctx = context.Background()
	}
	versions, _ := s.ListVersions(ctx, dbServerID)
	for _, v := range versions {
		info, err := s.runtime.Status(ctx, v.Runtime, v.ContainerName)
		status := containerStatus(info, err)
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

func sanitizeName(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var b strings.Builder
	for _, ch := range value {
		if (ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9') || ch == '-' {
			b.WriteRune(ch)
		} else {
			b.WriteByte('-')
		}
	}
	return strings.Trim(b.String(), "-")
}

func generateAdminPassword() (string, error) {
	buf := make([]byte, 24)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate database password: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func containerSpec(engine, runtimeName, version, image, name, volume, bind string, port int, user, password string) ContainerSpec {
	dataDir, health := "/var/lib/mysql", "mysqladmin ping -h localhost -u"+user+" -p$MYSQL_ROOT_PASSWORD"
	env := map[string]string{"MYSQL_ROOT_PASSWORD": password}
	command := []string(nil)
	configVolume, configDir := "", ""
	switch engine {
	case "postgresql":
		// PostgreSQL config lives in its data volume (postgresql.conf), so no
		// separate config volume is mounted.
		dataDir = "/var/lib/postgresql/data"
		env = map[string]string{"POSTGRES_PASSWORD": password}
		health = "pg_isready -U postgres"
	case "redis":
		dataDir = "/data"
		env = map[string]string{"REDIS_PASSWORD": password}
		health = "redis-cli -a $REDIS_PASSWORD ping"
		configVolume, configDir = name+"-config", "/usr/local/etc/redis"
		// Load the mounted redis.conf so instance-level config persists and
		// takes effect on restart; the CLI requirepass still wins over the file.
		command = []string{"redis-server", "--requirepass", password, configDir + "/redis.conf"}
	default: // mysql
		configVolume, configDir = name+"-config", "/etc/mysql/conf.d"
	}
	return ContainerSpec{Runtime: runtimeName, Name: name, Image: image, Volume: volume, DataDir: dataDir,
		ConfigVolume: configVolume, ConfigDir: configDir,
		BindAddress: bind, HostPort: port, ContainerPort: port, Environment: env,
		Labels: map[string]string{"com.easyserver.engine": engine, "com.easyserver.version": version}, HealthCommand: health, Command: command}
}

func containerStatus(info ContainerStatus, err error) string {
	if err != nil {
		return "stopped"
	}
	if info.State == "running" && info.Health == "healthy" {
		return "running"
	}
	if info.State == "running" {
		return "unhealthy"
	}
	return "stopped"
}

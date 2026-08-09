package database

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"easyserver/internal/deploy"
	"easyserver/internal/infra"
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

// Service manages the whole database domain: container-backed instances
// (lifecycle) and the logical databases, users, backups and SQL inside them.
type Service struct {
	repo          Repository
	runtime       DatabaseRuntime
	encryptionKey []byte
	backupDir     string
}

// NewService creates a database Service over the given Repository, driving
// containers through the CLI Runtime seam.
func NewService(repo Repository, exec executor.CommandExecutor, encryptionKey string) *Service {
	return &Service{
		repo:          repo,
		runtime:       NewCLIContainerRuntime(exec),
		encryptionKey: []byte(encryptionKey),
		backupDir:     DefaultBackupDir,
	}
}

// NewServiceWithRuntime is the test seam for lifecycle behavior; it skips the
// CLI runtime construction.
func NewServiceWithRuntime(repo Repository, runtime DatabaseRuntime, encryptionKey string) *Service {
	return &Service{
		repo:          repo,
		runtime:       runtime,
		encryptionKey: []byte(encryptionKey),
		backupDir:     DefaultBackupDir,
	}
}

// SeedPredefinedServers inserts the predefined engine catalog entries if absent.
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

// --- Engine catalog ---

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

// GetServerByID returns a server by ID (alias used by the mgmt handlers).
func (s *Service) GetServerByID(ctx context.Context, id int64) (*DBServer, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	return s.repo.GetServer(ctx, id)
}

// --- Instance lifecycle ---

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

	count, err := s.repo.CountVersionsByServerAndVersion(ctx, dbServerID, req.Version)
	if err != nil {
		return nil, err
	}
	if count > 0 {
		return nil, fmt.Errorf("version %s is already installed", req.Version)
	}

	image := ""
	for _, t := range GetVersionTemplates(ds.Name) {
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

// DestroyVersion removes the managed container, its data/config volumes and metadata.
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

// UpdateVersionPort updates the port for an instance.
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
		lines = defaultLogLines
	}
	if lines > maxLogLines {
		lines = maxLogLines
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
// volume so `redis-server <config>` can load it on first start.
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

// --- Logical database CRUD ---

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

func (s *Service) GetDatabaseByID(ctx context.Context, id int64) (*Database, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	return s.repo.GetDatabaseByID(ctx, id)
}

func (s *Service) CreateDatabase(ctx context.Context, dbServerID int64, req *CreateDatabaseRequest) (*Database, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	version, err := s.repo.GetVersion(ctx, req.DBInstanceID)
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
		out, err := s.runInVersion(ctx, version, "mysql", "-e", fmt.Sprintf("CREATE DATABASE `%s` CHARACTER SET %s;",
			strings.ReplaceAll(req.Name, "`", "``"), charset))
		if err != nil {
			return nil, fmt.Errorf("create database failed: %s", out)
		}
	case "postgresql":
		out, err := s.runInVersion(ctx, version, "createdb", "-E", charset, req.Name)
		if err != nil {
			return nil, fmt.Errorf("create database failed: %s", out)
		}
	default:
		return nil, fmt.Errorf("database creation not supported for %s", server.Name)
	}

	id, err := s.repo.CreateDatabase(ctx, dbServerID, req.DBInstanceID, req.Name, charset, req.Description)
	if err != nil {
		return nil, err
	}

	return &Database{
		ID:           id,
		DBServerID:   dbServerID,
		DBInstanceID: req.DBInstanceID,
		Name:         req.Name,
		Charset:      charset,
		Status:       "active",
		Version:      version.Version,
	}, nil
}

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

	version, err := s.repo.GetVersion(ctx, d.DBInstanceID)
	if err != nil {
		return fmt.Errorf("get version: %w", err)
	}
	if version == nil || version.Status != "running" {
		return fmt.Errorf("database version is not running")
	}

	switch server.Name {
	case "mysql":
		out, err := s.runInVersion(ctx, version, "mysql", "-e", fmt.Sprintf("DROP DATABASE `%s`;",
			strings.ReplaceAll(d.Name, "`", "``")))
		if err != nil {
			return fmt.Errorf("drop database failed: %s", out)
		}
	case "postgresql":
		out, err := s.runInVersion(ctx, version, "dropdb", d.Name)
		if err != nil {
			return fmt.Errorf("drop database failed: %s", out)
		}
	default:
		return fmt.Errorf("database deletion not supported for %s", server.Name)
	}

	return s.repo.DeleteDatabase(ctx, dbServerID, id)
}

// --- DB User CRUD ---

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
	activeVersion := firstRunningVersion(versions)
	if activeVersion == nil {
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
		out, err := s.runInVersion(ctx, activeVersion, "mysql", "-e", sqlStr)
		if err != nil {
			return nil, fmt.Errorf("create user failed: %s", out)
		}
	case "postgresql":
		out, err := s.runInVersion(ctx, activeVersion, "psql", "-c",
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

	id, err := s.repo.CreateDBUser(ctx, activeVersion.ID, req.Username, string(hashedPassword), host)
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
	activeVersion := firstRunningVersion(versions)
	if activeVersion == nil {
		return fmt.Errorf("no running version available")
	}

	switch server.Name {
	case "mysql":
		sqlStr := fmt.Sprintf("DROP USER '%s'@'%s';", u.Username, u.Host)
		out, err := s.runInVersion(ctx, activeVersion, "mysql", "-e", sqlStr)
		if err != nil {
			return fmt.Errorf("drop user failed: %s", out)
		}
	case "postgresql":
		out, err := s.runInVersion(ctx, activeVersion, "psql", "-c",
			fmt.Sprintf("DROP USER \"%s\";", u.Username))
		if err != nil {
			return fmt.Errorf("drop user failed: %s", out)
		}
	default:
		return fmt.Errorf("user deletion not supported for %s", server.Name)
	}

	return s.repo.DeleteDBUser(ctx, dbServerID, id)
}

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

	version, err := s.repo.GetVersion(ctx, req.DBInstanceID)
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
		out, err := s.runInVersion(ctx, version, "mysql", "-e", sqlStr)
		if err != nil {
			return fmt.Errorf("grant failed: %s", out)
		}
	case "postgresql":
		sqlStr := fmt.Sprintf("GRANT %s ON DATABASE \"%s\" TO \"%s\";", req.Privileges, req.Database, u.Username)
		out, err := s.runInVersion(ctx, version, "psql", "-c", sqlStr)
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
		DBInstanceID: dbVersionID,
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
	infra.Go(func() {
		defer cancel()
		s.executeBackup(backupCtx, backup, dbType)
	})

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
	version, err := s.repo.GetVersion(ctx, backup.DBInstanceID)
	if err != nil || version == nil {
		return fmt.Errorf("database instance not found")
	}
	out, err := s.runInVersion(ctx, version, "mysqldump", "--single-transaction", "--routines", "--triggers", backup.DatabaseName)
	if err != nil {
		return fmt.Errorf("mysqldump failed: %w", err)
	}
	return os.WriteFile(backup.FilePath, []byte(out), 0644)
}

func (s *Service) backupPostgreSQL(ctx context.Context, backup *DBBackup) error {
	version, err := s.repo.GetVersion(ctx, backup.DBInstanceID)
	if err != nil || version == nil {
		return fmt.Errorf("database instance not found")
	}
	out, err := s.runInVersion(ctx, version, "pg_dump", "-Fc", backup.DatabaseName)
	if err != nil {
		return fmt.Errorf("pg_dump failed: %w", err)
	}
	return os.WriteFile(backup.FilePath, []byte(out), 0644)
}

func (s *Service) backupRedis(ctx context.Context, backup *DBBackup) error {
	version, err := s.repo.GetVersion(ctx, backup.DBInstanceID)
	if err != nil || version == nil {
		return fmt.Errorf("database instance not found")
	}
	_, err = s.runInVersion(ctx, version, "redis-cli", "BGSAVE")
	if err != nil {
		return fmt.Errorf("redis BGSAVE failed: %w", err)
	}

	time.Sleep(2 * time.Second)
	return s.runtime.CopyFrom(ctx, version.Runtime, version.ContainerName, "/data/dump.rdb", backup.FilePath)
}

func (s *Service) ListBackups(ctx context.Context, databaseID int64) ([]DBBackup, error) {
	return s.repo.ListBackups(ctx, databaseID)
}

func (s *Service) GetBackup(ctx context.Context, id int64) (*DBBackup, error) {
	return s.repo.GetBackup(ctx, id)
}

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
	version, err := s.repo.GetVersion(ctx, backup.DBInstanceID)
	if err != nil || version == nil {
		return fmt.Errorf("database instance not found")
	}
	target := "/tmp/easyserver-restore.sql"
	if err := s.runtime.CopyTo(ctx, version.Runtime, version.ContainerName, backup.FilePath, target); err != nil {
		return fmt.Errorf("copy backup into container: %w", err)
	}
	if _, err := s.runInVersion(ctx, version, "sh", "-c", "mysql "+shellQuote(backup.DatabaseName)+" < "+target); err != nil {
		return fmt.Errorf("mysql restore failed: %w", err)
	}
	return nil
}

func (s *Service) restorePostgreSQL(ctx context.Context, backup *DBBackup) error {
	version, err := s.repo.GetVersion(ctx, backup.DBInstanceID)
	if err != nil || version == nil {
		return fmt.Errorf("database instance not found")
	}
	target := "/tmp/easyserver-restore.dump"
	if err := s.runtime.CopyTo(ctx, version.Runtime, version.ContainerName, backup.FilePath, target); err != nil {
		return fmt.Errorf("copy backup into container: %w", err)
	}
	out, err := s.runInVersion(ctx, version, "pg_restore", "-d", backup.DatabaseName, "-c", target)
	if err != nil {
		return fmt.Errorf("pg_restore failed: %s", out)
	}
	return nil
}

func (s *Service) restoreRedis(ctx context.Context, backup *DBBackup) error {
	version, err := s.repo.GetVersion(ctx, backup.DBInstanceID)
	if err != nil || version == nil {
		return fmt.Errorf("database instance not found")
	}
	if err := s.runtime.Stop(ctx, version.Runtime, version.ContainerName); err != nil {
		return fmt.Errorf("stop Redis failed: %w", err)
	}
	if err := s.runtime.CopyTo(ctx, version.Runtime, version.ContainerName, backup.FilePath, "/data/dump.rdb"); err != nil {
		return fmt.Errorf("copy Redis backup: %w", err)
	}
	if err := s.runtime.Start(ctx, version.Runtime, version.ContainerName); err != nil {
		return fmt.Errorf("start Redis failed: %w", err)
	}
	return nil
}

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

// --- SQL query operations ---

func (s *Service) runInVersion(ctx context.Context, version *DBInstance, args ...string) (string, error) {
	if version == nil || version.Runtime == "" || version.ContainerName == "" {
		return "", fmt.Errorf("database instance is not container-managed")
	}
	args = s.withAdminCredentials(version, args)
	return s.runtime.Exec(ctx, version.Runtime, version.ContainerName, args...)
}

func (s *Service) withAdminCredentials(version *DBInstance, args []string) []string {
	if len(args) == 0 || len(s.encryptionKey) != 32 {
		return args
	}
	password, err := deploy.Decrypt(version.AdminPassword, s.encryptionKey)
	if err != nil || password == "" {
		return args
	}
	engine := strings.ToLower(version.Image)
	if strings.HasPrefix(engine, "mysql") {
		return append([]string{args[0], "-uroot", "-p" + password}, args[1:]...)
	}
	if strings.HasPrefix(engine, "postgres") {
		return append([]string{args[0], "-U", "postgres"}, args[1:]...)
	}
	if strings.HasPrefix(engine, "redis") && args[0] == "redis-cli" {
		return append([]string{args[0], "-a", password}, args[1:]...)
	}
	return args
}

func (s *Service) lookupDB(ctx context.Context, dbID int64) (*Database, *DBServer, *DBInstance, DBType, error) {
	db, err := s.repo.GetDatabaseByID(ctx, dbID)
	if err != nil || db == nil {
		return nil, nil, nil, "", fmt.Errorf("数据库不存在")
	}
	server, err := s.repo.GetServer(ctx, db.DBServerID)
	if err != nil || server == nil {
		return nil, nil, nil, "", fmt.Errorf("服务器不存在")
	}
	version, err := s.repo.GetVersion(ctx, db.DBInstanceID)
	if err != nil || version == nil {
		return nil, nil, nil, "", fmt.Errorf("数据库实例不存在")
	}
	dbType := getDBTypeFromName(server.Name)
	return db, server, version, dbType, nil
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

func firstRunningVersion(versions []DBInstance) *DBInstance {
	for i := range versions {
		if versions[i].Status == "running" {
			return &versions[i]
		}
	}
	return nil
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

func (s *Service) execRaw(ctx context.Context, version *DBInstance, dbType DBType, dbName string, sql string) (string, error) {
	switch dbType {
	case DBTypeMySQL:
		return s.runInVersion(ctx, version, "mysql", dbName, "-e", sql)
	case DBTypePostgreSQL:
		return s.runInVersion(ctx, version, "psql", "-d", dbName, "-c", sql)
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

func (s *Service) ListTables(ctx context.Context, dbID int64) ([]map[string]interface{}, error) {
	db, server, version, _, err := s.lookupDB(ctx, dbID)
	if err != nil {
		return nil, err
	}

	var tables []map[string]interface{}
	switch server.Name {
	case "mysql":
		out, err := s.execRaw(ctx, version, DBTypeMySQL, db.Name, "SHOW TABLES;")
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
		out, err := s.execRaw(ctx, version, DBTypePostgreSQL, db.Name,
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

func (s *Service) DescribeTable(ctx context.Context, dbID int64, tableName string) (*DescribeResult, error) {
	if !ValidateTableName(tableName) {
		return nil, fmt.Errorf("无效的表名")
	}
	db, _, version, dbType, err := s.lookupDB(ctx, dbID)
	if err != nil {
		return nil, err
	}

	builder := NewSQLBuilder(dbType)
	describeSQL := builder.BuildDescribeTable(tableName)

	out, err := s.execRaw(ctx, version, dbType, db.Name, describeSQL)
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

	db, _, version, dbType, err := s.lookupDB(ctx, dbID)
	if err != nil {
		return nil, err
	}

	var total int
	switch dbType {
	case DBTypeMySQL:
		out, err := s.execRaw(ctx, version, DBTypeMySQL, db.Name, fmt.Sprintf("SELECT COUNT(*) FROM `%s`;", tableName))
		if err == nil {
			fmt.Sscanf(strings.TrimSpace(out), "%d", &total)
		}
	case DBTypePostgreSQL:
		out, err := s.execRaw(ctx, version, DBTypePostgreSQL, db.Name, fmt.Sprintf("SELECT COUNT(*) FROM \"%s\";", tableName))
		if err == nil {
			fmt.Sscanf(strings.TrimSpace(out), "%d", &total)
		}
	}

	var headers []string
	var rows [][]interface{}
	switch dbType {
	case DBTypeMySQL:
		out, err := s.execRaw(ctx, version, DBTypeMySQL, db.Name,
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
		out, err := s.execRaw(ctx, version, DBTypePostgreSQL, db.Name,
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

func (s *Service) ExecuteSQL(ctx context.Context, dbID int64, sql string) (*DMLResult, error) {
	db, server, version, dbType, err := s.lookupDB(ctx, dbID)
	if err != nil {
		return nil, err
	}

	validator := NewSQLValidator(dbType)
	if r := validator.ValidateSQL(sql); !r.Valid {
		return &DMLResult{Success: false, Error: r.Message}, nil
	}

	out, execErr := s.execRaw(ctx, version, dbType, db.Name, sql)
	if execErr != nil {
		log.Printf("ExecuteSQL %s error [db=%s]: %s", server.Name, db.Name, SanitizeSQLError(out))
		return &DMLResult{Success: false, Error: SanitizeSQLError(out)}, nil
	}
	return &DMLResult{Success: true, Output: out}, nil
}

func (s *Service) InsertRecord(ctx context.Context, dbID int64, table string, data map[string]interface{}, dryRun bool) (*DMLResult, error) {
	if !ValidateTableName(table) {
		return nil, fmt.Errorf("无效的表名")
	}
	db, _, version, dbType, err := s.lookupDB(ctx, dbID)
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

	out, execErr := s.execRaw(ctx, version, dbType, db.Name, sql)
	if execErr != nil {
		return &DMLResult{Success: false, Error: SanitizeSQLError(out)}, nil
	}
	return &DMLResult{Success: true, Output: out}, nil
}

func (s *Service) UpdateRecord(ctx context.Context, dbID int64, table string, data map[string]interface{}, pk string, pkVal interface{}, dryRun bool) (*DMLResult, error) {
	if !ValidateTableName(table) {
		return nil, fmt.Errorf("无效的表名")
	}
	db, _, version, dbType, err := s.lookupDB(ctx, dbID)
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

	out, execErr := s.execRaw(ctx, version, dbType, db.Name, sql)
	if execErr != nil {
		return &DMLResult{Success: false, Error: SanitizeSQLError(out)}, nil
	}
	return &DMLResult{Success: true, Output: out}, nil
}

func (s *Service) DeleteRecord(ctx context.Context, dbID int64, table string, pk string, pkVal interface{}, dryRun bool) (*DMLResult, error) {
	if !ValidateTableName(table) {
		return nil, fmt.Errorf("无效的表名")
	}
	db, _, version, dbType, err := s.lookupDB(ctx, dbID)
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

	out, execErr := s.execRaw(ctx, version, dbType, db.Name, sql)
	if execErr != nil {
		return &DMLResult{Success: false, Error: SanitizeSQLError(out)}, nil
	}
	return &DMLResult{Success: true}, nil
}

func (s *Service) CreateTable(ctx context.Context, dbID int64, tableName string, columns []TableColumn) error {
	if !ValidateTableName(tableName) {
		return fmt.Errorf("无效的表名")
	}
	db, _, version, dbType, err := s.lookupDB(ctx, dbID)
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

	out, execErr := s.execRaw(ctx, version, dbType, db.Name, sql)
	if execErr != nil {
		return fmt.Errorf("创建表失败: %s", SanitizeSQLError(out))
	}
	return nil
}

func (s *Service) DropTable(ctx context.Context, dbID int64, tableName string) error {
	if !ValidateTableName(tableName) {
		return fmt.Errorf("无效的表名")
	}
	db, _, version, dbType, err := s.lookupDB(ctx, dbID)
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

	out, execErr := s.execRaw(ctx, version, dbType, db.Name, sql)
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

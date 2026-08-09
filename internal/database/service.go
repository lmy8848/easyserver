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
)

const (
	maxDBNameLen    = 64
	maxUsernameLen  = 32
	maxHostLen      = 255
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

// refreshInstanceStatus queries the container runtime (by container ID) and
// persists the derived instance status.
func (s *Service) refreshInstanceStatus(ctx context.Context, v *DBInstance) {
	info, err := s.runtime.Status(ctx, v.Runtime, v.ContainerID)
	status := containerStatus(info, err)
	v.Status = status
	s.repo.UpdateInstanceStatus(ctx, v.ID, status)
}

// --- Instance lifecycle ---

func (s *Service) ListInstances(ctx context.Context, dbType DBType) ([]DBInstance, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if !IsValidDBType(dbType) {
		return nil, fmt.Errorf("unsupported database type %q", dbType)
	}
	return s.repo.ListInstances(ctx, dbType)
}

func (s *Service) GetInstance(ctx context.Context, id int64) (*DBInstance, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	return s.repo.GetInstance(ctx, id)
}

func (s *Service) CreateInstance(ctx context.Context, dbType DBType, req *CreateDBInstanceRequest) (*DBInstance, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if !IsValidDBType(dbType) {
		return nil, fmt.Errorf("unsupported database type %q", dbType)
	}

	count, err := s.repo.CountInstancesByDBTypeAndVersion(ctx, dbType, req.Version)
	if err != nil {
		return nil, err
	}
	if count > 0 {
		return nil, fmt.Errorf("version %s is already installed for %s", req.Version, dbType)
	}

	// The client sends the image + version (the front-end owns the version/image
	// catalogue); the image is required — without it there is nothing to pull.
	if strings.TrimSpace(req.Image) == "" {
		return nil, fmt.Errorf("image is required")
	}
	runtimeName := strings.ToLower(strings.TrimSpace(req.Runtime))
	if runtimeName == "" {
		runtimeName = "docker"
	}
	if runtimeName != "docker" && runtimeName != "podman" {
		return nil, fmt.Errorf("unsupported container runtime %q", runtimeName)
	}
	// The client always sends the port (the front-end fills the engine default);
	// a missing/invalid value is rejected here.
	port := req.Port
	if port < 1 || port > 65535 {
		return nil, fmt.Errorf("port must be between 1 and 65535")
	}
	bindAddress := strings.TrimSpace(req.BindAddress)
	if bindAddress == "" {
		bindAddress = "127.0.0.1"
	}
	containerID := fmt.Sprintf("easyserver-db-%s-%s", sanitizeName(string(dbType)), sanitizeName(req.Version))
	volumeName := containerID + "-data"
	password, err := generateAdminPassword()
	if err != nil {
		return nil, err
	}
	if len(s.encryptionKey) != 32 {
		return nil, fmt.Errorf("database encryption key must be configured")
	}
	encryptedPassword, err := deploy.Encrypt(password, s.encryptionKey)
	if err != nil {
		return nil, err
	}

	spec := containerSpec(dbType, runtimeName, req.Version, req.Image, containerID, volumeName, bindAddress, port, password)
	if err := s.runtime.Create(ctx, spec); err != nil {
		return nil, err
	}
	if dbType == DBTypeRedis {
		if err := seedRedisConfig(ctx, s.runtime, runtimeName, containerID, password); err != nil {
			_ = s.runtime.Remove(ctx, runtimeName, containerID)
			return nil, err
		}
	}
	if err := s.runtime.Start(ctx, runtimeName, containerID); err != nil {
		_ = s.runtime.Remove(ctx, runtimeName, containerID)
		return nil, fmt.Errorf("start database container: %w", err)
	}
	if _, err := waitForHealthy(ctx, s.runtime, runtimeName, containerID, 2*time.Minute); err != nil {
		_ = s.runtime.Remove(ctx, runtimeName, containerID)
		return nil, err
	}
	v := &DBInstance{DBType: dbType, Version: req.Version, Port: port, Status: "running", Runtime: runtimeName, Image: req.Image,
		ContainerID: containerID, VolumeName: volumeName, ConfigDir: spec.ConfigDir, BindAddress: bindAddress,
		AdminPassword: encryptedPassword}
	id, err := s.repo.CreateInstance(ctx, v)
	if err != nil {
		_ = s.runtime.Remove(ctx, runtimeName, containerID)
		return nil, err
	}
	v.ID = id
	return v, nil
}

// ListDockerTags returns the published tags for an engine's official image,
// proxied from Docker Hub. It powers the front-end "更多版本" flow — users can
// pick any published tag, not just the curated presets.
func (s *Service) ListDockerTags(ctx context.Context, dbType DBType) ([]string, error) {
	if !IsValidDBType(dbType) {
		return nil, fmt.Errorf("unsupported database type %q", dbType)
	}
	return fetchDockerHubTags(dockerImageBase(dbType))
}

func (s *Service) UninstallInstance(ctx context.Context, instanceID int64) error {
	if ctx == nil {
		ctx = context.Background()
	}
	v, err := s.GetInstance(ctx, instanceID)
	if err != nil || v == nil {
		return fmt.Errorf("instance not found")
	}

	dbCount, err := s.countDatabases(ctx, v)
	if err != nil {
		return err
	}
	if dbCount > 0 {
		return fmt.Errorf("cannot uninstall: %d databases still exist for this instance", dbCount)
	}

	if err := s.runtime.Remove(ctx, v.Runtime, v.ContainerID); err != nil {
		return fmt.Errorf("remove database container: %w", err)
	}
	return s.repo.DeleteInstance(ctx, instanceID)
}

// DestroyInstance removes the managed container, its data/config volumes and metadata.
func (s *Service) DestroyInstance(ctx context.Context, instanceID int64) error {
	v, err := s.GetInstance(ctx, instanceID)
	if err != nil || v == nil {
		return fmt.Errorf("instance not found")
	}
	if count, err := s.countDatabases(ctx, v); err != nil {
		return err
	} else if count > 0 {
		return fmt.Errorf("cannot destroy: %d databases still exist for this instance", count)
	}
	if err := s.runtime.Remove(ctx, v.Runtime, v.ContainerID); err != nil {
		return fmt.Errorf("remove database container: %w", err)
	}
	if err := s.runtime.RemoveVolume(ctx, v.Runtime, v.VolumeName); err != nil {
		return fmt.Errorf("remove database volume: %w", err)
	}
	if v.ConfigDir != "" {
		if err := s.runtime.RemoveVolume(ctx, v.Runtime, strings.TrimSuffix(v.VolumeName, "-data")+"-config"); err != nil {
			return fmt.Errorf("remove database config volume: %w", err)
		}
	}
	return s.repo.DeleteInstance(ctx, instanceID)
}

// ResetAdminPassword rotates the administrator password and returns it once.
func (s *Service) ResetAdminPassword(ctx context.Context, instanceID int64) (string, error) {
	v, err := s.GetInstance(ctx, instanceID)
	if err != nil || v == nil {
		return "", fmt.Errorf("instance not found")
	}
	if v.Runtime == "" || v.ContainerID == "" || len(s.encryptionKey) != 32 {
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
	switch v.DBType {
	case DBTypeMySQL:
		if _, err := s.runtime.Exec(ctx, v.Runtime, v.ContainerID, "mysql", "-uroot", "-p"+oldPassword, "-e", "ALTER USER 'root'@'localhost' IDENTIFIED BY '"+strings.ReplaceAll(password, "'", "''")+"';"); err != nil {
			return "", fmt.Errorf("reset MySQL password: %w", err)
		}
	case DBTypePostgreSQL:
		if _, err := s.runtime.Exec(ctx, v.Runtime, v.ContainerID, "psql", "-U", "postgres", "-c", "ALTER USER postgres WITH PASSWORD '"+strings.ReplaceAll(password, "'", "''")+"';"); err != nil {
			return "", fmt.Errorf("reset PostgreSQL password: %w", err)
		}
	case DBTypeRedis:
		if _, err := s.runtime.Exec(ctx, v.Runtime, v.ContainerID, "redis-cli", "-a", oldPassword, "CONFIG", "SET", "requirepass", password); err != nil {
			return "", fmt.Errorf("reset Redis password: %w", err)
		}
	default:
		return "", fmt.Errorf("password reset is not supported for this database type")
	}
	encrypted, err := deploy.Encrypt(password, s.encryptionKey)
	if err != nil {
		return "", err
	}
	if err := s.repo.UpdateInstancePassword(ctx, instanceID, encrypted); err != nil {
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

func (s *Service) StartInstance(ctx context.Context, instanceID int64) error {
	if ctx == nil {
		ctx = context.Background()
	}
	v, err := s.GetInstance(ctx, instanceID)
	if err != nil || v == nil {
		return fmt.Errorf("instance not found")
	}
	if err := s.runtime.Start(ctx, v.Runtime, v.ContainerID); err != nil {
		return fmt.Errorf("start failed: %w", err)
	}
	if _, err := waitForHealthy(ctx, s.runtime, v.Runtime, v.ContainerID, 2*time.Minute); err != nil {
		return err
	}
	return s.repo.UpdateInstanceStatus(ctx, instanceID, "running")
}

func (s *Service) StopInstance(ctx context.Context, instanceID int64) error {
	if ctx == nil {
		ctx = context.Background()
	}
	v, err := s.GetInstance(ctx, instanceID)
	if err != nil || v == nil {
		return fmt.Errorf("instance not found")
	}
	if err := s.runtime.Stop(ctx, v.Runtime, v.ContainerID); err != nil {
		return fmt.Errorf("stop failed: %w", err)
	}
	return s.repo.UpdateInstanceStatus(ctx, instanceID, "stopped")
}

func (s *Service) RestartInstance(ctx context.Context, instanceID int64) error {
	if ctx == nil {
		ctx = context.Background()
	}
	v, err := s.GetInstance(ctx, instanceID)
	if err != nil || v == nil {
		return fmt.Errorf("instance not found")
	}
	if err := s.runtime.Restart(ctx, v.Runtime, v.ContainerID); err != nil {
		return fmt.Errorf("restart failed: %w", err)
	}
	if _, err := waitForHealthy(ctx, s.runtime, v.Runtime, v.ContainerID, 2*time.Minute); err != nil {
		return err
	}
	return s.repo.UpdateInstanceStatus(ctx, instanceID, "running")
}

// UpdateInstancePort updates the port for an instance.
func (s *Service) UpdateInstancePort(ctx context.Context, instanceID int64, newPort int) error {
	if ctx == nil {
		ctx = context.Background()
	}
	v, err := s.GetInstance(ctx, instanceID)
	if err != nil || v == nil {
		return fmt.Errorf("instance not found")
	}

	if v.Status == "running" {
		return fmt.Errorf("cannot change port while service is running. Stop it first")
	}
	if newPort < 1 || newPort > 65535 {
		return fmt.Errorf("port must be between 1 and 65535")
	}
	password, err := s.decryptAdminPassword(v)
	if err != nil {
		return err
	}
	if err := s.runtime.Remove(ctx, v.Runtime, v.ContainerID); err != nil {
		return fmt.Errorf("remove old database container: %w", err)
	}
	spec := containerSpec(v.DBType, v.Runtime, v.Version, v.Image, v.ContainerID, v.VolumeName, v.BindAddress, newPort, password)
	if err := s.runtime.Create(ctx, spec); err != nil {
		return fmt.Errorf("recreate database container: %w", err)
	}
	if err := s.runtime.Start(ctx, v.Runtime, v.ContainerID); err != nil {
		return fmt.Errorf("start database container: %w", err)
	}
	if _, err := waitForHealthy(ctx, s.runtime, v.Runtime, v.ContainerID, 2*time.Minute); err != nil {
		return err
	}
	return s.repo.UpdateInstancePort(ctx, instanceID, newPort)
}

func (s *Service) GetInstanceServiceLogs(ctx context.Context, instanceID int64, lines int) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	v, err := s.GetInstance(ctx, instanceID)
	if err != nil || v == nil {
		return "", fmt.Errorf("instance not found")
	}
	if lines <= 0 {
		lines = defaultLogLines
	}
	if lines > maxLogLines {
		lines = maxLogLines
	}
	return s.runtime.Logs(ctx, v.Runtime, v.ContainerID, lines)
}

// GetInstanceConfig reads the engine configuration from inside its container.
func (s *Service) GetInstanceConfig(ctx context.Context, instanceID int64) (string, string, error) {
	v, err := s.GetInstance(ctx, instanceID)
	if err != nil || v == nil {
		return "", "", fmt.Errorf("instance not found")
	}
	path := configPathForImage(v.Image)
	out, err := s.runtime.Exec(ctx, v.Runtime, v.ContainerID, "cat", path)
	return out, path, err
}

// SaveInstanceConfig writes engine configuration inside the managed container.
func (s *Service) SaveInstanceConfig(ctx context.Context, instanceID int64, content string) error {
	if len(content) > 256*1024 {
		return fmt.Errorf("configuration is too large")
	}
	v, err := s.GetInstance(ctx, instanceID)
	if err != nil || v == nil {
		return fmt.Errorf("instance not found")
	}
	path := configPathForImage(v.Image)
	encoded := base64.StdEncoding.EncodeToString([]byte(content))
	if _, err := s.runtime.Exec(ctx, v.Runtime, v.ContainerID, "sh", "-c", "echo "+encoded+" | base64 -d > "+path); err != nil {
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

// RefreshStatus refreshes instance statuses for an engine (dbType).
func (s *Service) RefreshStatus(ctx context.Context, dbType DBType) {
	if ctx == nil {
		ctx = context.Background()
	}
	instances, _ := s.repo.ListInstances(ctx, dbType)
	for _, v := range instances {
		s.refreshInstanceStatus(ctx, &v)
	}
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

func containerSpec(engine DBType, runtimeName, version, image, name, volume, bind string, port int, password string) ContainerSpec {
	dataDir, health := "/var/lib/mysql", "mysqladmin ping -h localhost -uroot -p$MYSQL_ROOT_PASSWORD"
	env := map[string]string{"MYSQL_ROOT_PASSWORD": password}
	command := []string(nil)
	configVolume, configDir := "", ""
	adminUser := "root"
	switch engine {
	case DBTypePostgreSQL:
		// PostgreSQL config lives in its data volume (postgresql.conf), so no
		// separate config volume is mounted.
		dataDir = "/var/lib/postgresql/data"
		env = map[string]string{"POSTGRES_PASSWORD": password}
		health = "pg_isready -U postgres"
		adminUser = "postgres"
	case DBTypeRedis:
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
		Labels: map[string]string{"com.easyserver.engine": string(engine), "com.easyserver.version": version, "com.easyserver.admin-user": adminUser}, HealthCommand: health, Command: command}
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

// --- Logical database CRUD (live, engine-owned) ---

func (s *Service) ListDatabases(ctx context.Context, instanceID int64) ([]Database, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	instance, err := s.repo.GetInstance(ctx, instanceID)
	if err != nil {
		return nil, fmt.Errorf("get instance: %w", err)
	}
	if instance == nil {
		return nil, fmt.Errorf("database instance not found")
	}
	return s.queryDatabases(ctx, instance)
}

// queryDatabases lists logical databases live from the engine. Databases are
// engine-owned state — the panel never persists a mirror of them.
func (s *Service) queryDatabases(ctx context.Context, instance *DBInstance) ([]Database, error) {
	var out string
	var err error
	switch instance.DBType {
	case DBTypeMySQL:
		out, err = s.runInVersion(ctx, instance, "mysql", "-N", "-B", "-e",
			"SELECT schema_name, default_character_set_name FROM information_schema.schemata WHERE schema_name NOT IN ('information_schema','mysql','performance_schema','sys') ORDER BY schema_name")
	case DBTypePostgreSQL:
		out, err = s.runInVersion(ctx, instance, "psql", "-t", "-A", "-c",
			"SELECT datname, pg_encoding_to_char(encoding) FROM pg_database WHERE datistemplate = false ORDER BY datname")
	case DBTypeRedis:
		return []Database{}, nil
	default:
		return nil, fmt.Errorf("unsupported db type: %s", instance.DBType)
	}
	if err != nil {
		return nil, fmt.Errorf("list databases failed: %s", SanitizeSQLError(out))
	}
	var dbs []Database
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.SplitN(line, "\t", 2)
		name := strings.TrimSpace(fields[0])
		if name == "" {
			continue
		}
		charset := ""
		if len(fields) > 1 {
			charset = strings.TrimSpace(fields[1])
		}
		dbs = append(dbs, Database{Name: name, Charset: charset})
	}
	return dbs, nil
}

// countDatabases returns the live logical-database count for an instance. A
// stopped instance cannot be queried, so the guard degrades to 0 — the data
// lives in the container volume and uninstall/destroy are explicit user
// confirmations anyway.
func (s *Service) countDatabases(ctx context.Context, v *DBInstance) (int, error) {
	if v.Status != "running" && v.Status != "active" {
		return 0, nil
	}
	dbs, err := s.queryDatabases(ctx, v)
	return len(dbs), err
}

func (s *Service) CreateDatabase(ctx context.Context, instanceID int64, req *CreateDatabaseRequest) (*Database, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	instance, err := s.repo.GetInstance(ctx, instanceID)
	if err != nil {
		return nil, fmt.Errorf("get instance: %w", err)
	}
	if instance == nil {
		return nil, fmt.Errorf("database instance not found")
	}
	if instance.Status != "running" && instance.Status != "active" {
		return nil, fmt.Errorf("database instance is not running")
	}

	if !isValidDBName(req.Name) {
		return nil, fmt.Errorf("invalid database name")
	}

	charset := req.Charset
	if charset == "" {
		charset = defaultCharset
	}
	if !isValidCharset(charset) {
		return nil, fmt.Errorf("invalid charset: %s", charset)
	}

	switch instance.DBType {
	case DBTypeMySQL:
		out, err := s.runInVersion(ctx, instance, "mysql", "-e", fmt.Sprintf("CREATE DATABASE `%s` CHARACTER SET %s;",
			strings.ReplaceAll(req.Name, "`", "``"), charset))
		if err != nil {
			return nil, fmt.Errorf("create database failed: %s", out)
		}
	case DBTypePostgreSQL:
		encoding := "UTF8"
		if charset == "latin1" {
			encoding = "LATIN1"
		}
		out, err := s.runInVersion(ctx, instance, "createdb", "-E", encoding, req.Name)
		if err != nil {
			return nil, fmt.Errorf("create database failed: %s", out)
		}
	default:
		return nil, fmt.Errorf("database creation not supported for %s", instance.DBType)
	}

	return &Database{Name: req.Name, Charset: charset}, nil
}

func (s *Service) DeleteDatabase(ctx context.Context, instanceID int64, dbName string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	instance, err := s.repo.GetInstance(ctx, instanceID)
	if err != nil {
		return fmt.Errorf("get instance: %w", err)
	}
	if instance == nil {
		return fmt.Errorf("database instance not found")
	}
	if instance.Status != "running" {
		return fmt.Errorf("database instance is not running")
	}

	switch instance.DBType {
	case DBTypeMySQL:
		out, err := s.runInVersion(ctx, instance, "mysql", "-e", fmt.Sprintf("DROP DATABASE `%s`;",
			strings.ReplaceAll(dbName, "`", "``")))
		if err != nil {
			return fmt.Errorf("drop database failed: %s", out)
		}
	case DBTypePostgreSQL:
		out, err := s.runInVersion(ctx, instance, "dropdb", dbName)
		if err != nil {
			return fmt.Errorf("drop database failed: %s", out)
		}
	default:
		return fmt.Errorf("database deletion not supported for %s", instance.DBType)
	}

	return nil
}

// --- DB User CRUD (live, engine-owned) ---

func (s *Service) ListDBUsers(ctx context.Context, instanceID int64) ([]DBUser, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	instance, err := s.repo.GetInstance(ctx, instanceID)
	if err != nil {
		return nil, fmt.Errorf("get instance: %w", err)
	}
	if instance == nil {
		return nil, fmt.Errorf("database instance not found")
	}
	return s.queryUsers(ctx, instance)
}

// queryUsers lists database users live from the engine (the engine owns them).
func (s *Service) queryUsers(ctx context.Context, instance *DBInstance) ([]DBUser, error) {
	var out string
	var err error
	switch instance.DBType {
	case DBTypeMySQL:
		out, err = s.runInVersion(ctx, instance, "mysql", "-N", "-B", "-e",
			"SELECT user, host FROM mysql.user WHERE user NOT IN ('mysql.session','mysql.sys','mysql.infoschema') ORDER BY user, host")
	case DBTypePostgreSQL:
		out, err = s.runInVersion(ctx, instance, "psql", "-t", "-A", "-c",
			"SELECT rolname FROM pg_roles WHERE rolcanlogin ORDER BY rolname")
	case DBTypeRedis:
		return []DBUser{}, nil
	default:
		return nil, fmt.Errorf("unsupported db type: %s", instance.DBType)
	}
	if err != nil {
		return nil, fmt.Errorf("list users failed: %s", SanitizeSQLError(out))
	}
	var users []DBUser
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.SplitN(line, "\t", 2)
		username := strings.TrimSpace(fields[0])
		if username == "" {
			continue
		}
		host := ""
		if len(fields) > 1 {
			host = strings.TrimSpace(fields[1])
		}
		users = append(users, DBUser{Username: username, Host: host})
	}
	return users, nil
}

// isAdminUser reports whether username is the engine's built-in administrator.
func isAdminUser(dbType DBType, username string) bool {
	switch dbType {
	case DBTypeMySQL:
		return username == "root"
	case DBTypePostgreSQL:
		return username == "postgres"
	}
	return false
}

func (s *Service) CreateDBUser(ctx context.Context, instanceID int64, req *CreateDBUserRequest) (*DBUser, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	instance, err := s.repo.GetInstance(ctx, instanceID)
	if err != nil {
		return nil, fmt.Errorf("get instance: %w", err)
	}
	if instance == nil {
		return nil, fmt.Errorf("database instance not found")
	}
	if instance.Status != "running" {
		return nil, fmt.Errorf("database instance is not running")
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

	switch instance.DBType {
	case DBTypeMySQL:
		sqlStr := fmt.Sprintf("CREATE USER '%s'@'%s' IDENTIFIED BY '%s';", req.Username, host, escapeMySQLString(req.Password))
		out, err := s.runInVersion(ctx, instance, "mysql", "-e", sqlStr)
		if err != nil {
			return nil, fmt.Errorf("create user failed: %s", out)
		}
	case DBTypePostgreSQL:
		out, err := s.runInVersion(ctx, instance, "psql", "-c",
			fmt.Sprintf("CREATE USER \"%s\" WITH PASSWORD '%s';", req.Username, escapePGString(req.Password)))
		if err != nil {
			return nil, fmt.Errorf("create user failed: %s", out)
		}
	default:
		return nil, fmt.Errorf("user creation not supported for %s", instance.DBType)
	}

	return &DBUser{Username: req.Username, Host: host}, nil
}

func (s *Service) DeleteDBUser(ctx context.Context, instanceID int64, username, host string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	instance, err := s.repo.GetInstance(ctx, instanceID)
	if err != nil {
		return fmt.Errorf("get instance: %w", err)
	}
	if instance == nil {
		return fmt.Errorf("database instance not found")
	}

	if !isValidUsername(username) {
		return fmt.Errorf("invalid username")
	}
	if isAdminUser(instance.DBType, username) {
		return fmt.Errorf("cannot delete the administrator user")
	}
	if instance.DBType == DBTypeMySQL && !isValidHost(host) {
		return fmt.Errorf("invalid host")
	}

	switch instance.DBType {
	case DBTypeMySQL:
		sqlStr := fmt.Sprintf("DROP USER '%s'@'%s';", username, host)
		out, err := s.runInVersion(ctx, instance, "mysql", "-e", sqlStr)
		if err != nil {
			return fmt.Errorf("drop user failed: %s", out)
		}
	case DBTypePostgreSQL:
		out, err := s.runInVersion(ctx, instance, "psql", "-c",
			fmt.Sprintf("DROP USER \"%s\";", username))
		if err != nil {
			return fmt.Errorf("drop user failed: %s", out)
		}
	default:
		return fmt.Errorf("user deletion not supported for %s", instance.DBType)
	}

	return nil
}

func (s *Service) GrantPrivileges(ctx context.Context, instanceID int64, username, host string, req *GrantRequest) error {
	if ctx == nil {
		ctx = context.Background()
	}
	instance, err := s.repo.GetInstance(ctx, instanceID)
	if err != nil {
		return fmt.Errorf("get instance: %w", err)
	}
	if instance == nil {
		return fmt.Errorf("database instance not found")
	}
	if instance.Status != "running" {
		return fmt.Errorf("database instance is not running")
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

	switch instance.DBType {
	case DBTypeMySQL:
		sqlStr := fmt.Sprintf("GRANT %s ON `%s`.* TO '%s'@'%s'; FLUSH PRIVILEGES;",
			req.Privileges, strings.ReplaceAll(req.Database, "`", "``"), username, host)
		out, err := s.runInVersion(ctx, instance, "mysql", "-e", sqlStr)
		if err != nil {
			return fmt.Errorf("grant failed: %s", out)
		}
	case DBTypePostgreSQL:
		sqlStr := fmt.Sprintf("GRANT %s ON DATABASE \"%s\" TO \"%s\";", req.Privileges, req.Database, username)
		out, err := s.runInVersion(ctx, instance, "psql", "-c", sqlStr)
		if err != nil {
			return fmt.Errorf("grant failed: %s", out)
		}
	default:
		return fmt.Errorf("privilege grant not supported for %s", instance.DBType)
	}

	return nil
}

// --- Backup operations ---

// SetBackupDir sets the backup directory.
func (s *Service) SetBackupDir(dir string) {
	s.backupDir = dir
}

func (s *Service) CreateBackup(ctx context.Context, instanceID int64, dbName string, dbType DBType) (*DBBackup, error) {
	if err := os.MkdirAll(s.backupDir, 0755); err != nil {
		return nil, fmt.Errorf("create backup dir: %w", err)
	}

	timestamp := time.Now().Format("20060102150405")
	var fileName string
	switch dbType {
	case DBTypeMySQL, DBTypePostgreSQL:
		fileName = fmt.Sprintf("%s_%s.sql", dbName, timestamp)
	case DBTypeRedis:
		fileName = fmt.Sprintf("dump_%s.rdb", timestamp)
	default:
		return nil, fmt.Errorf("unsupported db type: %s", dbType)
	}
	filePath := filepath.Join(s.backupDir, fileName)

	backup := &DBBackup{
		DBInstanceID: instanceID,
		DBType:       dbType,
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

func (s *Service) executeBackup(ctx context.Context, backup *DBBackup, dbType DBType) {
	var err error

	switch dbType {
	case DBTypeMySQL:
		err = s.backupMySQL(ctx, backup)
	case DBTypePostgreSQL:
		err = s.backupPostgreSQL(ctx, backup)
	case DBTypeRedis:
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
	instance, err := s.repo.GetInstance(ctx, backup.DBInstanceID)
	if err != nil || instance == nil {
		return fmt.Errorf("database instance not found")
	}
	out, err := s.runInVersion(ctx, instance, "mysqldump", "--single-transaction", "--routines", "--triggers", backup.DatabaseName)
	if err != nil {
		return fmt.Errorf("mysqldump failed: %w", err)
	}
	return os.WriteFile(backup.FilePath, []byte(out), 0644)
}

func (s *Service) backupPostgreSQL(ctx context.Context, backup *DBBackup) error {
	instance, err := s.repo.GetInstance(ctx, backup.DBInstanceID)
	if err != nil || instance == nil {
		return fmt.Errorf("database instance not found")
	}
	out, err := s.runInVersion(ctx, instance, "pg_dump", "-Fc", backup.DatabaseName)
	if err != nil {
		return fmt.Errorf("pg_dump failed: %w", err)
	}
	return os.WriteFile(backup.FilePath, []byte(out), 0644)
}

func (s *Service) backupRedis(ctx context.Context, backup *DBBackup) error {
	instance, err := s.repo.GetInstance(ctx, backup.DBInstanceID)
	if err != nil || instance == nil {
		return fmt.Errorf("database instance not found")
	}
	_, err = s.runInVersion(ctx, instance, "redis-cli", "BGSAVE")
	if err != nil {
		return fmt.Errorf("redis BGSAVE failed: %w", err)
	}

	time.Sleep(2 * time.Second)
	return s.runtime.CopyFrom(ctx, instance.Runtime, instance.ContainerID, "/data/dump.rdb", backup.FilePath)
}

func (s *Service) ListBackups(ctx context.Context, instanceID int64, dbName string) ([]DBBackup, error) {
	return s.repo.ListBackups(ctx, instanceID, dbName)
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

func (s *Service) RestoreBackup(ctx context.Context, id int64, dbType DBType) error {
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
	case DBTypeMySQL:
		return s.restoreMySQL(ctx, backup)
	case DBTypePostgreSQL:
		return s.restorePostgreSQL(ctx, backup)
	case DBTypeRedis:
		return s.restoreRedis(ctx, backup)
	default:
		return fmt.Errorf("unsupported db type: %s", dbType)
	}
}

func (s *Service) restoreMySQL(ctx context.Context, backup *DBBackup) error {
	instance, err := s.repo.GetInstance(ctx, backup.DBInstanceID)
	if err != nil || instance == nil {
		return fmt.Errorf("database instance not found")
	}
	target := "/tmp/easyserver-restore.sql"
	if err := s.runtime.CopyTo(ctx, instance.Runtime, instance.ContainerID, backup.FilePath, target); err != nil {
		return fmt.Errorf("copy backup into container: %w", err)
	}
	if _, err := s.runInVersion(ctx, instance, "sh", "-c", "mysql "+shellQuote(backup.DatabaseName)+" < "+target); err != nil {
		return fmt.Errorf("mysql restore failed: %w", err)
	}
	return nil
}

func (s *Service) restorePostgreSQL(ctx context.Context, backup *DBBackup) error {
	instance, err := s.repo.GetInstance(ctx, backup.DBInstanceID)
	if err != nil || instance == nil {
		return fmt.Errorf("database instance not found")
	}
	target := "/tmp/easyserver-restore.dump"
	if err := s.runtime.CopyTo(ctx, instance.Runtime, instance.ContainerID, backup.FilePath, target); err != nil {
		return fmt.Errorf("copy backup into container: %w", err)
	}
	out, err := s.runInVersion(ctx, instance, "pg_restore", "-d", backup.DatabaseName, "-c", target)
	if err != nil {
		return fmt.Errorf("pg_restore failed: %s", out)
	}
	return nil
}

func (s *Service) restoreRedis(ctx context.Context, backup *DBBackup) error {
	instance, err := s.repo.GetInstance(ctx, backup.DBInstanceID)
	if err != nil || instance == nil {
		return fmt.Errorf("database instance not found")
	}
	if err := s.runtime.Stop(ctx, instance.Runtime, instance.ContainerID); err != nil {
		return fmt.Errorf("stop Redis failed: %w", err)
	}
	if err := s.runtime.CopyTo(ctx, instance.Runtime, instance.ContainerID, backup.FilePath, "/data/dump.rdb"); err != nil {
		return fmt.Errorf("copy Redis backup: %w", err)
	}
	if err := s.runtime.Start(ctx, instance.Runtime, instance.ContainerID); err != nil {
		return fmt.Errorf("start Redis failed: %w", err)
	}
	return nil
}

func (s *Service) CleanOldBackups(ctx context.Context, instanceID int64, dbName string, maxBackups int) error {
	if maxBackups <= 0 {
		maxBackups = MaxBackupsPerDB
	}

	backups, err := s.repo.ListBackups(ctx, instanceID, dbName)
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

func (s *Service) runInVersion(ctx context.Context, instance *DBInstance, args ...string) (string, error) {
	if instance == nil || instance.Runtime == "" || instance.ContainerID == "" {
		return "", fmt.Errorf("database instance is not container-managed")
	}
	args = s.withAdminCredentials(instance, args)
	return s.runtime.Exec(ctx, instance.Runtime, instance.ContainerID, args...)
}

func (s *Service) withAdminCredentials(instance *DBInstance, args []string) []string {
	if len(args) == 0 || len(s.encryptionKey) != 32 {
		return args
	}
	password, err := deploy.Decrypt(instance.AdminPassword, s.encryptionKey)
	if err != nil || password == "" {
		return args
	}
	switch instance.DBType {
	case DBTypeMySQL:
		return append([]string{args[0], "-uroot", "-p" + password}, args[1:]...)
	case DBTypePostgreSQL:
		return append([]string{args[0], "-U", "postgres"}, args[1:]...)
	case DBTypeRedis:
		if args[0] == "redis-cli" {
			return append([]string{args[0], "-a", password}, args[1:]...)
		}
	}
	return args
}

// getInstanceForSQL resolves the instance for a database-level SQL operation.
// Database names travel as URL path parameters now — no persisted db lookup.
func (s *Service) getInstanceForSQL(ctx context.Context, instanceID int64, dbName string) (*DBInstance, error) {
	if !isValidDBName(dbName) {
		return nil, fmt.Errorf("无效的数据库名")
	}
	instance, err := s.repo.GetInstance(ctx, instanceID)
	if err != nil || instance == nil {
		return nil, fmt.Errorf("数据库实例不存在")
	}
	return instance, nil
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

func (s *Service) ListTables(ctx context.Context, instanceID int64, dbName string) ([]map[string]interface{}, error) {
	instance, err := s.getInstanceForSQL(ctx, instanceID, dbName)
	if err != nil {
		return nil, err
	}

	var tables []map[string]interface{}
	switch instance.DBType {
	case DBTypeMySQL:
		out, err := s.execRaw(ctx, instance, DBTypeMySQL, dbName, "SHOW TABLES;")
		if err != nil {
			return nil, fmt.Errorf("获取表列表失败: %s", SanitizeSQLError(out))
		}
		lines := strings.Split(strings.TrimSpace(out), "\n")
		for i, line := range lines {
			line = strings.TrimSpace(line)
			if i == 0 || line == "" {
				continue // first line is the "Tables_in_<db>" header
			}
			tables = append(tables, map[string]interface{}{"name": line})
		}
	case DBTypePostgreSQL:
		out, err := s.execRaw(ctx, instance, DBTypePostgreSQL, dbName,
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

func (s *Service) DescribeTable(ctx context.Context, instanceID int64, dbName, tableName string) (*DescribeResult, error) {
	if !ValidateTableName(tableName) {
		return nil, fmt.Errorf("无效的表名")
	}
	instance, err := s.getInstanceForSQL(ctx, instanceID, dbName)
	if err != nil {
		return nil, err
	}

	builder := NewSQLBuilder(instance.DBType)
	describeSQL := builder.BuildDescribeTable(tableName)

	out, err := s.execRaw(ctx, instance, instance.DBType, dbName, describeSQL)
	if err != nil {
		return nil, fmt.Errorf("获取表结构失败: %s", SanitizeSQLError(out))
	}

	tableInfo := ParseTableInfo(instance.DBType, tableName, out)

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

func (s *Service) QueryTable(ctx context.Context, instanceID int64, dbName, tableName string, page, pageSize int) (*PagedQueryResult, error) {
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

	instance, err := s.getInstanceForSQL(ctx, instanceID, dbName)
	if err != nil {
		return nil, err
	}
	dbType := instance.DBType

	var total int
	switch dbType {
	case DBTypeMySQL:
		out, err := s.execRaw(ctx, instance, DBTypeMySQL, dbName, fmt.Sprintf("SELECT COUNT(*) FROM `%s`;", tableName))
		if err == nil {
			fmt.Sscanf(strings.TrimSpace(out), "%d", &total)
		}
	case DBTypePostgreSQL:
		out, err := s.execRaw(ctx, instance, DBTypePostgreSQL, dbName, fmt.Sprintf("SELECT COUNT(*) FROM \"%s\";", tableName))
		if err == nil {
			fmt.Sscanf(strings.TrimSpace(out), "%d", &total)
		}
	}

	var headers []string
	var rows [][]interface{}
	switch dbType {
	case DBTypeMySQL:
		out, err := s.execRaw(ctx, instance, DBTypeMySQL, dbName,
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
		out, err := s.execRaw(ctx, instance, DBTypePostgreSQL, dbName,
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

func (s *Service) ExecuteSQL(ctx context.Context, instanceID int64, dbName, sql string) (*DMLResult, error) {
	instance, err := s.getInstanceForSQL(ctx, instanceID, dbName)
	if err != nil {
		return nil, err
	}
	dbType := instance.DBType

	validator := NewSQLValidator(dbType)
	if r := validator.ValidateSQL(sql); !r.Valid {
		return &DMLResult{Success: false, Error: r.Message}, nil
	}

	out, execErr := s.execRaw(ctx, instance, dbType, dbName, sql)
	if execErr != nil {
		log.Printf("ExecuteSQL %s error [db=%s]: %s", instance.DBType, dbName, SanitizeSQLError(out))
		return &DMLResult{Success: false, Error: SanitizeSQLError(out)}, nil
	}
	return &DMLResult{Success: true, Output: out}, nil
}

func (s *Service) InsertRecord(ctx context.Context, instanceID int64, dbName, table string, data map[string]interface{}, dryRun bool) (*DMLResult, error) {
	if !ValidateTableName(table) {
		return nil, fmt.Errorf("无效的表名")
	}
	instance, err := s.getInstanceForSQL(ctx, instanceID, dbName)
	if err != nil {
		return nil, err
	}
	dbType := instance.DBType

	builder := NewSQLBuilder(dbType)
	validator := NewSQLValidator(dbType)

	if r := validator.ValidateInsert(table, data, nil); !r.Valid {
		return &DMLResult{Success: false, Error: r.Message}, nil
	}

	sql := builder.BuildInsert(table, data, nil)
	if dryRun {
		return &DMLResult{Success: true, DryRun: true, SQL: sql}, nil
	}

	out, execErr := s.execRaw(ctx, instance, dbType, dbName, sql)
	if execErr != nil {
		return &DMLResult{Success: false, Error: SanitizeSQLError(out)}, nil
	}
	return &DMLResult{Success: true, Output: out}, nil
}

func (s *Service) UpdateRecord(ctx context.Context, instanceID int64, dbName, table string, data map[string]interface{}, pk string, pkVal interface{}, dryRun bool) (*DMLResult, error) {
	if !ValidateTableName(table) {
		return nil, fmt.Errorf("无效的表名")
	}
	instance, err := s.getInstanceForSQL(ctx, instanceID, dbName)
	if err != nil {
		return nil, err
	}
	dbType := instance.DBType

	builder := NewSQLBuilder(dbType)
	validator := NewSQLValidator(dbType)

	if r := validator.ValidateUpdate(table, data, pk, pkVal); !r.Valid {
		return &DMLResult{Success: false, Error: r.Message}, nil
	}

	sql := builder.BuildUpdate(table, data, pk, pkVal)
	if dryRun {
		return &DMLResult{Success: true, DryRun: true, SQL: sql}, nil
	}

	out, execErr := s.execRaw(ctx, instance, dbType, dbName, sql)
	if execErr != nil {
		return &DMLResult{Success: false, Error: SanitizeSQLError(out)}, nil
	}
	return &DMLResult{Success: true, Output: out}, nil
}

func (s *Service) DeleteRecord(ctx context.Context, instanceID int64, dbName, table string, pk string, pkVal interface{}, dryRun bool) (*DMLResult, error) {
	if !ValidateTableName(table) {
		return nil, fmt.Errorf("无效的表名")
	}
	instance, err := s.getInstanceForSQL(ctx, instanceID, dbName)
	if err != nil {
		return nil, err
	}
	dbType := instance.DBType

	builder := NewSQLBuilder(dbType)
	validator := NewSQLValidator(dbType)

	if r := validator.ValidateDelete(table, pk, pkVal); !r.Valid {
		return &DMLResult{Success: false, Error: r.Message}, nil
	}

	sql := builder.BuildDelete(table, pk, pkVal)
	if dryRun {
		return &DMLResult{Success: true, DryRun: true, SQL: sql}, nil
	}

	out, execErr := s.execRaw(ctx, instance, dbType, dbName, sql)
	if execErr != nil {
		return &DMLResult{Success: false, Error: SanitizeSQLError(out)}, nil
	}
	return &DMLResult{Success: true}, nil
}

func (s *Service) CreateTable(ctx context.Context, instanceID int64, dbName, tableName string, columns []TableColumn) error {
	if !ValidateTableName(tableName) {
		return fmt.Errorf("无效的表名")
	}
	instance, err := s.getInstanceForSQL(ctx, instanceID, dbName)
	if err != nil {
		return err
	}
	dbType := instance.DBType

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

	out, execErr := s.execRaw(ctx, instance, dbType, dbName, sql)
	if execErr != nil {
		return fmt.Errorf("创建表失败: %s", SanitizeSQLError(out))
	}
	return nil
}

func (s *Service) DropTable(ctx context.Context, instanceID int64, dbName, tableName string) error {
	if !ValidateTableName(tableName) {
		return fmt.Errorf("无效的表名")
	}
	instance, err := s.getInstanceForSQL(ctx, instanceID, dbName)
	if err != nil {
		return err
	}
	dbType := instance.DBType

	var sql string
	switch dbType {
	case DBTypeMySQL:
		sql = fmt.Sprintf("DROP TABLE `%s`;", tableName)
	case DBTypePostgreSQL:
		sql = fmt.Sprintf("DROP TABLE \"%s\";", tableName)
	default:
		return fmt.Errorf("不支持的数据库类型")
	}

	out, execErr := s.execRaw(ctx, instance, dbType, dbName, sql)
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

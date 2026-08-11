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
	"strconv"
	"strings"
	"sync"
	"time"

	"easyserver/internal/infra"
	"easyserver/internal/infra/executor"
	"easyserver/internal/infra/task"
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

// Service manages the whole database domain: container-backed instances
// (lifecycle) and the logical databases, users, backups and SQL inside them.
type Service struct {
	repo           Repository
	runtime        DatabaseRuntime // shared runtime for normal (non-install) ops
	backupDir      string
	taskMgr        *task.Manager          // background install executor (key=DBType 去重)
	runtimeFactory func() DatabaseRuntime // builds the runtime for background installs
	driver         SQLQueryRunner         // direct driver channel (MySQL/PostgreSQL)
}

// NewService creates a database Service over the given Repository, driving
// containers through the CLI Runtime seam and SQL through the direct driver
// channel.
func NewService(repo Repository, exec executor.CommandExecutor) *Service {
	rt := NewCLIContainerRuntime(exec)
	return &Service{
		repo:      repo,
		runtime:   rt,
		backupDir: DefaultBackupDir,
		taskMgr:   task.NewManager(8),
		runtimeFactory: func() DatabaseRuntime {
			return NewCLIContainerRuntime(exec)
		},
		driver: newDriverQueryRunner(),
	}
}

// NewServiceWithRuntime is the test seam for lifecycle behavior; it skips the
// CLI runtime construction.
func NewServiceWithRuntime(repo Repository, runtime DatabaseRuntime) *Service {
	return &Service{
		repo:      repo,
		runtime:   runtime,
		backupDir: DefaultBackupDir,
		taskMgr:   task.NewManager(8),
		runtimeFactory: func() DatabaseRuntime {
			return runtime
		},
		driver: newDriverQueryRunner(),
	}
}

// runnerFor returns the SQL channel for an instance. All SQL runs over the
// direct driver connection; broken port mappings (container_port = 0) and Redis
// surface as clear errors from the driver channel instead of falling back.
func (s *Service) runnerFor(inst *DBInstance) SQLQueryRunner {
	return s.driver
}

// refreshInstanceStatus queries the container runtime (by container ID) and
// persists the derived instance status.
func (s *Service) refreshInstanceStatus(ctx context.Context, v *DBInstance) {
	// installing/provisioning rows are mid-install records: the container is created but the
	// install isn't done, so its live state (created/restarting/…) is not the
	// instance's status — overwriting it here would turn "正在安装" into "stopped".
	// failed rows keep their status too: the container was deliberately rolled
	// back, so its live state (stopped) must not mask the failure.
	if v.Status == "installing" || v.Status == "provisioning" || v.Status == "failed" {
		return
	}
	info, err := s.runtime.Status(ctx, v.ContainerEngine, v.ContainerName)
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

// CreateInstanceResult is what CreateInstance returns. The instance row exists
// from the start (status "installing"), so the caller gets both the install id
// (to stream the log) and the instance id.
type CreateInstanceResult struct {
	InstallID  string `json:"install_id"`
	InstanceID int64  `json:"instance_id"`
	Version    string `json:"version"`
	Image      string `json:"image"`
	Port       int    `json:"port"`
	Status     string `json:"status"` // always "installing" (informational)
}

func (s *Service) CreateInstance(ctx context.Context, dbType DBType, req *CreateDBInstanceRequest) (*CreateInstanceResult, error) {
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
	engineName := strings.ToLower(strings.TrimSpace(req.ContainerEngine))
	if engineName == "" {
		engineName = "docker"
	}
	if engineName != "docker" && engineName != "podman" {
		return nil, fmt.Errorf("unsupported container runtime %q", engineName)
	}
	// The client always sends the port (the front-end fills the type default);
	// a missing/invalid value is rejected here.
	port := req.Port
	if port < 1 || port > 65535 {
		return nil, fmt.Errorf("port must be between 1 and 65535")
	}
	bindAddress := strings.TrimSpace(req.BindAddress)
	if bindAddress == "" {
		bindAddress = "127.0.0.1"
	}
	if bindAddress != "127.0.0.1" && bindAddress != "0.0.0.0" {
		return nil, fmt.Errorf("unsupported bind address %q (only 127.0.0.1 or 0.0.0.0)", bindAddress)
	}
	// 自定义容器名（可选）：非空时校验格式；空则用默认名。预检必须发生在写
	// row 之前，否则留下幽灵 installing 行。
	if name := strings.TrimSpace(req.ContainerName); name != "" {
		if err := validateContainerName(name); err != nil {
			return nil, err
		}
	}
	containerName := defaultContainerName(dbType, req.Version, req.ContainerName)
	volumeName := containerName + "-data"
	// Admin password is stored plainly: SQLite file and container environment
	// share the host, so a static key encrypts nothing an attacker can't read
	// next door — encryption would only add a missing-key failure mode.
	password, err := generateAdminPassword()
	if err != nil {
		return nil, err
	}

	// 安装前预检：同名容器已存在（残留或手动创建）则拒绝，避免 docker create
	// 抛出难懂的 "name already in use"。
	exists, err := s.runtime.Exists(ctx, engineName, containerName)
	if err != nil {
		return nil, fmt.Errorf("检查容器名占用失败: %w", err)
	}
	if exists {
		return nil, fmt.Errorf("容器名 %q 已被占用", containerName)
	}

	spec := containerSpec(dbType, engineName, req.Version, req.Image, containerName, volumeName, bindAddress, port, password)

	// Write the instance row up front (status "installing") — the install never
	// has a row-less phase, and the front-end renders the "正在安装" state straight
	// from the instance list. The install goroutine flips it to "running" on
	// success / "failed" on error (or removes the row on cancel). Dedup against
	// an in-flight install of the same DB type happens synchronously in the task
	// executor; if that rejects the request, undo the just-written row.
	row := &DBInstance{
		DBType: dbType, Version: req.Version, Port: port, ContainerPort: containerPortForType(dbType), Status: "installing",
		ContainerEngine: engineName, Image: req.Image, ContainerName: containerName,
		VolumeName: volumeName, ConfigDir: spec.ConfigDir, BindAddress: bindAddress, AdminPassword: password,
	}
	id, err := s.repo.CreateInstance(ctx, row)
	if err != nil {
		return nil, fmt.Errorf("write instance record: %w", err)
	}
	if _, err := s.taskMgr.StartWithLog(containerName, task.Options{}, func(ctx context.Context, log *task.TaskLog) error {
		rt := s.runtimeFactory()
		if cli, ok := rt.(*CLIContainerRuntime); ok {
			cli.SetOutputHook(func(line string) { log.Append(line) })
		}
		// Detach from the request context: the install outlives the HTTP request
		// (which is canceled once CreateInstance responds), so it must not inherit
		// its cancellation — the per-task context from the task executor drives it,
		// and CancelInstall cancels that.
		return s.installInstance(ctx, id, dbType, req.Version, req.Image, engineName, containerName, volumeName, password, spec, rt, log)
	}); err != nil {
		// Duplicate install (same container already installing) or concurrency
		// limit: the row was written above but no task started — remove it so the
		// panel doesn't show a phantom "installing" entry.
		_ = s.repo.DeleteInstance(ctx, id)
		return nil, err
	}
	return &CreateInstanceResult{InstallID: containerName, InstanceID: id, Version: req.Version, Image: req.Image, Port: port, Status: "installing"}, nil
}

// dockerTagsCache holds the full (filtered) tag list for one database type, with a TTL
// so the front-end paginates against one snapshot instead of re-hitting the
// rate-limited Docker Hub API on every page flip.
type dockerTagsCache struct {
	tags    []string
	fetched time.Time
}

var dockerTagsCacheStore = struct {
	sync.Mutex
	m map[DBType]dockerTagsCache
}{m: make(map[DBType]dockerTagsCache)}

const dockerTagsCacheTTL = 10 * time.Minute

// ListDockerTags returns one page of published tags for a database type's official
// image, proxied from Docker Hub (filtered to version-like tags, cached). It
// powers the front-end "更多版本" flow — users can pick any published tag, not
// just the curated presets.
func (s *Service) ListDockerTags(ctx context.Context, dbType DBType, page, pageSize int) ([]string, int, error) {
	if !IsValidDBType(dbType) {
		return nil, 0, fmt.Errorf("unsupported database type %q", dbType)
	}
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 10
	}
	tags, err := s.dockerTags(dbType)
	if err != nil {
		return nil, 0, err
	}
	start := (page - 1) * pageSize
	if start >= len(tags) {
		return []string{}, len(tags), nil
	}
	end := start + pageSize
	if end > len(tags) {
		end = len(tags)
	}
	return tags[start:end], len(tags), nil
}

func (s *Service) dockerTags(dbType DBType) ([]string, error) {
	dockerTagsCacheStore.Lock()
	defer dockerTagsCacheStore.Unlock()
	if c, ok := dockerTagsCacheStore.m[dbType]; ok && time.Since(c.fetched) < dockerTagsCacheTTL {
		return c.tags, nil
	}
	tags, err := fetchDockerHubTags(dockerImageBase(dbType))
	if err != nil {
		return nil, err
	}
	dockerTagsCacheStore.m[dbType] = dockerTagsCache{tags: tags, fetched: time.Now()}
	return tags, nil
}

// InstallTask exposes an in-flight install's log and completion to the SSE
// handler. Returns false when no task exists for the install id (successful
// installs are cleaned up on completion, so this is only live/failed/canceled).
func (s *Service) InstallTask(installID string) (*task.Task, bool) {
	return s.taskMgr.Get(installID)
}

// WaitForInstall blocks until the install finishes and returns its error. Used
// by tests (and a handy end-of-stream sync for future callers).
func (s *Service) WaitForInstall(installID string) error {
	t, ok := s.taskMgr.Get(installID)
	if !ok {
		return nil
	}
	<-t.Done()
	return t.Err()
}

// UninstallInstance removes the managed container. The data volume is retained
// by default so the instance can be re-installed onto it; purge deletes it too.
func (s *Service) UninstallInstance(ctx context.Context, instanceID int64, purge bool) error {
	if ctx == nil {
		ctx = context.Background()
	}
	v, err := s.GetInstance(ctx, instanceID)
	if err != nil || v == nil {
		return fmt.Errorf("instance not found")
	}

	if err := s.runtime.Remove(ctx, v.ContainerEngine, v.ContainerName); err != nil {
		return fmt.Errorf("remove database container: %w", err)
	}
	if purge {
		if err := s.runtime.RemoveVolume(ctx, v.ContainerEngine, v.VolumeName); err != nil {
			return fmt.Errorf("remove database volume: %w", err)
		}
		if v.ConfigDir != "" {
			if err := s.runtime.RemoveVolume(ctx, v.ContainerEngine, strings.TrimSuffix(v.VolumeName, "-data")+"-config"); err != nil {
				return fmt.Errorf("remove database config volume: %w", err)
			}
		}
	}
	return s.repo.DeleteInstance(ctx, instanceID)
}

// DestroyInstance removes the managed container, its data/config volumes and metadata.
func (s *Service) DestroyInstance(ctx context.Context, instanceID int64) error {
	v, err := s.GetInstance(ctx, instanceID)
	if err != nil || v == nil {
		return fmt.Errorf("instance not found")
	}
	if err := s.runtime.Remove(ctx, v.ContainerEngine, v.ContainerName); err != nil {
		return fmt.Errorf("remove database container: %w", err)
	}
	if err := s.runtime.RemoveVolume(ctx, v.ContainerEngine, v.VolumeName); err != nil {
		return fmt.Errorf("remove database volume: %w", err)
	}
	if v.ConfigDir != "" {
		if err := s.runtime.RemoveVolume(ctx, v.ContainerEngine, strings.TrimSuffix(v.VolumeName, "-data")+"-config"); err != nil {
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
	if v.ContainerEngine == "" || v.ContainerName == "" {
		return "", fmt.Errorf("database instance is not container-managed")
	}
	oldPassword := v.AdminPassword
	password, err := generateAdminPassword()
	if err != nil {
		return "", err
	}
	switch v.DBType {
	case DBTypeMySQL:
		// Same MYSQL_PWD trick as withAdminCredentials — no `-p` on the command
		// line, so no stderr warning leaks into output.
		if _, err := s.runtime.Exec(ctx, v.ContainerEngine, v.ContainerName, "-e", "MYSQL_PWD="+oldPassword, "mysql", "-uroot", "-e", "ALTER USER 'root'@'localhost' IDENTIFIED BY '"+strings.ReplaceAll(password, "'", "''")+"';"); err != nil {
			return "", fmt.Errorf("reset MySQL password: %w", err)
		}
	case DBTypePostgreSQL:
		if _, err := s.runtime.Exec(ctx, v.ContainerEngine, v.ContainerName, "psql", "-U", "postgres", "-c", "ALTER USER postgres WITH PASSWORD '"+strings.ReplaceAll(password, "'", "''")+"';"); err != nil {
			return "", fmt.Errorf("reset PostgreSQL password: %w", err)
		}
	case DBTypeRedis:
		if _, err := s.runtime.Exec(ctx, v.ContainerEngine, v.ContainerName, "redis-cli", "-a", oldPassword, "CONFIG", "SET", "requirepass", password); err != nil {
			return "", fmt.Errorf("reset Redis password: %w", err)
		}
	default:
		return "", fmt.Errorf("password reset is not supported for this database type")
	}
	if err := s.repo.UpdateInstancePassword(ctx, instanceID, password); err != nil {
		return "", err
	}
	return password, nil
}

// ResetAdminPassword rotates the administrator password and returns it once.
// The password is stored plainly — see the admin-password comment in
// CreateInstance (same-host SQLite and container env, encryption adds nothing).

func (s *Service) StartInstance(ctx context.Context, instanceID int64) error {
	if ctx == nil {
		ctx = context.Background()
	}
	v, err := s.GetInstance(ctx, instanceID)
	if err != nil || v == nil {
		return fmt.Errorf("instance not found")
	}
	if err := s.runtime.Start(ctx, v.ContainerEngine, v.ContainerName); err != nil {
		return fmt.Errorf("start failed: %w", err)
	}
	if _, err := waitForHealthy(ctx, s.runtime, v.ContainerEngine, v.ContainerName, 2*time.Minute); err != nil {
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
	if err := s.runtime.Stop(ctx, v.ContainerEngine, v.ContainerName); err != nil {
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
	if err := s.runtime.Restart(ctx, v.ContainerEngine, v.ContainerName); err != nil {
		return fmt.Errorf("restart failed: %w", err)
	}
	if _, err := waitForHealthy(ctx, s.runtime, v.ContainerEngine, v.ContainerName, 2*time.Minute); err != nil {
		return err
	}
	return s.repo.UpdateInstanceStatus(ctx, instanceID, "running")
}

// 修改端口是结构化配置的一部分（保存配置时端口变化触发 recreateInstanceContainer，
// 见 SaveInstanceConfig）—— 没有独立的修改端口入口。

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
	return s.runtime.Logs(ctx, v.ContainerEngine, v.ContainerName, lines)
}

// GetInstanceConfig 返回实例的结构化配置：参数值（覆盖项或编译默认值）+ 编辑元数据。
// 覆盖项存于容器配置卷（面板生成的配置文件），面板是唯一写入方；参数元数据与编译
// 默认值定义在代码（config.go）。port 是实例级状态（DB 的 port 列），始终显示当前值。
func (s *Service) GetInstanceConfig(ctx context.Context, instanceID int64) (*InstanceConfigView, error) {
	v, err := s.GetInstance(ctx, instanceID)
	if err != nil || v == nil {
		return nil, fmt.Errorf("instance not found")
	}
	content, err := s.runtime.ReadVolumeFile(ctx, v.ContainerEngine, v.Image, configVolumeFor(v), configFileDestPath(v))
	if err != nil {
		return nil, err
	}
	params := effectiveParams(v.DBType, parseConfigFile(v.DBType, content))
	params["port"] = strconv.Itoa(v.Port)
	view := &InstanceConfigView{FilePath: configPathForImage(v.Image)}
	view.Sections = append(view.Sections, ConfigSectionView{
		Name:   configSectionName(v.DBType),
		Params: params,
		Meta:   configParams(v.DBType),
	})
	return view, nil
}

// SaveInstanceConfig 保存结构化配置并立即生效：
//  1. 读容器配置卷现有覆盖项 → 合并本次修改 → 按参数生成配置文件写回卷；
//  2. port 参数变化 → 重建容器更新端口映射（配置卷已是新文件，启动即加载）；
//     其余参数变化且实例运行中 → 重启容器使配置生效。面板是配置唯一写入方。
func (s *Service) SaveInstanceConfig(ctx context.Context, instanceID int64, sections []ConfigSectionView) error {
	v, err := s.GetInstance(ctx, instanceID)
	if err != nil || v == nil {
		return fmt.Errorf("instance not found")
	}
	content, err := s.runtime.ReadVolumeFile(ctx, v.ContainerEngine, v.Image, configVolumeFor(v), configFileDestPath(v))
	if err != nil {
		return err
	}
	stored := parseConfigFile(v.DBType, content)
	for _, section := range sections {
		for key, value := range section.Params {
			if strings.TrimSpace(value) == "" {
				delete(stored, key)
				continue
			}
			stored[key] = strings.TrimSpace(value)
		}
	}
	if err := s.runtime.SeedVolumeFile(ctx, v.ContainerEngine, v.Image, configVolumeFor(v), configFileDestPath(v), generateConfigFile(v.DBType, stored)); err != nil {
		return err
	}

	// 端口：结构化参数里的值，非法/缺失回退当前实例端口。
	newPort := v.Port
	if raw, ok := stored["port"]; ok {
		if p, err := strconv.Atoi(raw); err == nil && p >= 1 && p <= 65535 {
			newPort = p
		}
	}
	if newPort != v.Port {
		return s.recreateInstanceContainer(ctx, v, newPort)
	}
	if v.Status == "running" {
		return s.RestartInstance(ctx, instanceID)
	}
	return nil
}

// recreateInstanceContainer 用新端口重建容器（移除旧映射、按新端口 create/start），
// 数据卷保留，配置卷在调用前已写入新内容，启动即加载。保存配置里 port 参数时调用。
func (s *Service) recreateInstanceContainer(ctx context.Context, v *DBInstance, newPort int) error {
	spec := containerSpec(v.DBType, v.ContainerEngine, v.Version, v.Image, v.ContainerName, v.VolumeName, v.BindAddress, newPort, v.AdminPassword)
	if err := s.runtime.Remove(ctx, v.ContainerEngine, v.ContainerName); err != nil {
		return fmt.Errorf("remove old database container: %w", err)
	}
	if err := s.runtime.Create(ctx, spec); err != nil {
		return fmt.Errorf("recreate database container: %w", err)
	}
	if err := s.runtime.Start(ctx, v.ContainerEngine, v.ContainerName); err != nil {
		return fmt.Errorf("start database container: %w", err)
	}
	if _, err := waitForHealthy(ctx, s.runtime, v.ContainerEngine, v.ContainerName, 2*time.Minute); err != nil {
		return err
	}
	return s.repo.UpdateInstancePort(ctx, v.ID, newPort)
}

// configVolumeFor 返回配置持久化卷：MySQL/Redis 用独立配置卷，PostgreSQL 的
// 覆盖项写在数据卷的 postgresql.auto.conf（服务器启动时自动读取，无需独立卷）。
func configVolumeFor(v *DBInstance) string {
	if v.DBType == DBTypePostgreSQL {
		return v.VolumeName
	}
	return strings.TrimSuffix(v.VolumeName, "-data") + "-config"
}

// configFileDestPath 是配置文件相对其所在卷挂载点的路径（SeedVolumeFile 把卷挂在
// /easyserver-init，dest 是卷内相对路径）。
func configFileDestPath(v *DBInstance) string {
	switch v.DBType {
	case DBTypeMySQL:
		return "easyserver.cnf"
	case DBTypeRedis:
		return "redis.conf"
	default: // postgresql — 覆盖项写进数据卷的 postgresql.auto.conf（PG 服务器启动时在
		// postgresql.conf 之后自动读取的标准覆盖文件），不碰镜像生成的全量配置。
		return pgConfigDestRelative(v.Image)
	}
}

// pgConfigDestRelative 是 postgresql.auto.conf 相对数据卷挂载点的路径：PG 18+ 的
// PGDATA 移到版本子目录，旧版在数据卷根。
func pgConfigDestRelative(image string) string {
	if major := postgresMajor(image); major >= 18 {
		return fmt.Sprintf("%d/docker/postgresql.auto.conf", major)
	}
	return "postgresql.auto.conf"
}

// MySQL/Redis 的默认配置由 generateConfigFile 根据结构化参数（编译默认值）生成，
// 安装时通过 SeedVolumeFile 预置进持久配置卷，见 config.go。

func configPathForImage(image string) string {
	image = strings.ToLower(image)
	// Images are stored fully qualified (docker.io/mysql:8.0) — match the repo
	// basename, not the registry prefix (docker.io/… has no "mysql" prefix).
	if i := strings.LastIndex(image, "/"); i >= 0 {
		image = image[i+1:]
	}
	switch {
	case strings.HasPrefix(image, "mysql"):
		// 面板管理的自定义片段，位于镜像 my.cnf 自动 !includedir 的 conf.d；
		// 安装时由 generateConfigFile 预置（镜像里无默认配置可复制）。
		return "/etc/mysql/conf.d/easyserver.cnf"
	case strings.HasPrefix(image, "postgres"):
		return pgConfigPath(image)
	default:
		return "/usr/local/etc/redis/redis.conf"
	}
}

// postgresMajor extracts the PostgreSQL major version from an image reference
// (e.g. "docker.io/postgres:18-alpine" → 18). Returns 0 when the tag carries no
// leading version number.
func postgresMajor(image string) int {
	i := strings.LastIndex(image, ":")
	if i < 0 {
		return 0
	}
	var major int
	if n, err := fmt.Sscanf(image[i+1:], "%d", &major); err != nil || n != 1 {
		return 0
	}
	return major
}

// pgDataDir is the container path the data volume is mounted at. postgres:18+
// moved the default PGDATA into a major-version subdirectory
// (/var/lib/postgresql/<major>/docker, docker-library/postgres#1259): mounting
// a volume at the old /var/lib/postgresql/data makes the entrypoint refuse to
// start ("unused mount/volume"), so 18+ must mount the parent instead.
func pgDataDir(image string) string {
	if postgresMajor(image) >= 18 {
		return "/var/lib/postgresql"
	}
	return "/var/lib/postgresql/data"
}

// pgConfigPath is where postgresql.conf lives inside the container — under the
// same PGDATA the image uses, which moved to a version subdir in 18+.
func pgConfigPath(image string) string {
	if major := postgresMajor(image); major >= 18 {
		return fmt.Sprintf("/var/lib/postgresql/%d/docker/postgresql.auto.conf", major)
	}
	return "/var/lib/postgresql/data/postgresql.auto.conf"
}

// RefreshStatus refreshes instance statuses for a database type (dbType).
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

// containerNameRe 是容器名的允许字符集（docker 与 podman 的规则交集）：
// 首字符字母/数字，其余字母/数字/下划线/点/连字符。Docker 的 daemon 会拒绝
// 不属于 [a-zA-Z0-9][a-zA-Z0-9_.-]* 的名字，Podman 更宽松但向下兼容该集合，
// 因此按交集校验两个引擎都必然接受。
var containerNameRe = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]*$`)

const maxContainerNameLen = 128

func validateContainerName(name string) error {
	if len(name) > maxContainerNameLen {
		return fmt.Errorf("容器名过长（最多 %d 个字符）", maxContainerNameLen)
	}
	if !containerNameRe.MatchString(name) {
		return fmt.Errorf("容器名只能包含字母、数字以及 _ . -，且必须以字母或数字开头")
	}
	return nil
}

// defaultContainerName 计算受管数据库容器的名字。未提供自定义名时用确定性默认
// "easyserver-db-<type>-<version>"；提供时用用户自定义名（已 by 调用方校验）。
func defaultContainerName(dbType DBType, version, custom string) string {
	if name := strings.TrimSpace(custom); name != "" {
		return name
	}
	return fmt.Sprintf("easyserver-db-%s-%s", sanitizeName(string(dbType)), sanitizeName(version))
}

func generateAdminPassword() (string, error) {
	buf := make([]byte, 24)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate database password: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

// containerPortForType is the port the database engine listens on INSIDE the
// container — always the engine default (MySQL 3306 / PostgreSQL 5432 / Redis
// 6379). The user-selected port is only the host mapping (HostPort). The old
// code mapped the user port 1:1 onto the container port (`--publish X:X`),
// which pointed at a container port where nothing listens — the mapping was
// useless to any external client and unusable by the direct-connection channel.
func containerPortForType(dbType DBType) int {
	switch dbType {
	case DBTypePostgreSQL:
		return 5432
	case DBTypeRedis:
		return 6379
	}
	return 3306
}

func containerSpec(dbType DBType, engineName, version, image, name, volume, bind string, port int, password string) ContainerSpec {
	dataDir, health := "/var/lib/mysql", "mysqladmin ping -h localhost -uroot -p$MYSQL_ROOT_PASSWORD"
	env := map[string]string{"MYSQL_ROOT_PASSWORD": password}
	command := []string(nil)
	configVolume, configDir := "", ""
	adminUser := "root"
	switch dbType {
	case DBTypePostgreSQL:
		// PostgreSQL config lives in its data volume (postgresql.conf), so no
		// separate config volume is mounted. 18+ images expect the volume at
		// /var/lib/postgresql (PGDATA moved into a version subdir) — see pgDataDir.
		dataDir = pgDataDir(image)
		env = map[string]string{"POSTGRES_PASSWORD": password}
		health = "pg_isready -U postgres"
		adminUser = "postgres"
	case DBTypeRedis:
		dataDir = "/data"
		env = map[string]string{"REDIS_PASSWORD": password}
		health = "redis-cli -a $REDIS_PASSWORD ping"
		configVolume, configDir = name+"-config", "/usr/local/etc/redis"
		command = []string{"redis-server", "--requirepass", password, configDir + "/redis.conf"}
	default: // mysql — 挂配置卷，安装时从镜像预置默认配置（见 SeedVolume 注释）
		configVolume, configDir = name+"-config", "/etc/mysql/conf.d"
	}
	return ContainerSpec{ContainerEngine: engineName, Name: name, Image: image, Volume: volume, DataDir: dataDir,
		ConfigVolume: configVolume, ConfigDir: configDir,
		BindAddress: bind, HostPort: port, ContainerPort: containerPortForType(dbType), Environment: env,
		Labels: map[string]string{"com.easyserver.dbtype": string(dbType), "com.easyserver.version": version, "com.easyserver.admin-user": adminUser}, HealthCommand: health, Command: command}
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

// --- Logical database CRUD (live, server-owned) ---

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

// queryDatabases lists logical databases live from the database server. Databases are
// server-owned state — the panel never persists a mirror of them.
func (s *Service) queryDatabases(ctx context.Context, instance *DBInstance) ([]Database, error) {
	var runner SQLQueryRunner
	switch instance.DBType {
	case DBTypeMySQL:
		runner = s.runnerFor(instance)
		res, err := runner.Query(ctx, instance, systemDBName(instance.DBType),
			"SELECT schema_name, default_character_set_name FROM information_schema.schemata WHERE schema_name NOT IN ('information_schema','mysql','performance_schema','sys') ORDER BY schema_name")
		if err != nil {
			return nil, fmt.Errorf("list databases failed: %s", SanitizeSQLError(err.Error()))
		}
		var dbs []Database
		for _, row := range res.Rows {
			dbs = append(dbs, Database{Name: str(row, 0), Charset: str(row, 1)})
		}
		return dbs, nil
	case DBTypePostgreSQL:
		runner = s.runnerFor(instance)
		res, err := runner.Query(ctx, instance, systemDBName(instance.DBType),
			"SELECT datname, pg_encoding_to_char(encoding) FROM pg_database WHERE datistemplate = false ORDER BY datname")
		if err != nil {
			return nil, fmt.Errorf("list databases failed: %s", SanitizeSQLError(err.Error()))
		}
		var dbs []Database
		for _, row := range res.Rows {
			dbs = append(dbs, Database{Name: str(row, 0), Charset: str(row, 1)})
		}
		return dbs, nil
	case DBTypeRedis:
		return []Database{}, nil
	default:
		return nil, fmt.Errorf("unsupported db type: %s", instance.DBType)
	}
}

// str coerces a query row cell to its string form for display fields. Driver
// cells may arrive as string, []byte or nil (NULL).
func str(row []any, i int) string {
	if i >= len(row) || row[i] == nil {
		return ""
	}
	switch v := row[i].(type) {
	case string:
		return v
	case []byte:
		return string(v)
	}
	return fmt.Sprintf("%v", row[i])
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

	// DDL statements cannot be parameter-bound; names/hosts are already
	// validated and passwords escaped by the builder. The system database hosts
	// instance-level statements — including CREATE DATABASE itself, which must
	// not run on the target database.
	builder := NewSQLBuilder(instance.DBType)
	sysDB := systemDBName(instance.DBType)
	switch instance.DBType {
	case DBTypeMySQL, DBTypePostgreSQL:
		if _, err := s.runnerFor(instance).Exec(ctx, instance, sysDB, builder.BuildCreateDatabase(req.Name, charset)); err != nil {
			return nil, fmt.Errorf("create database failed: %s", SanitizeSQLError(err.Error()))
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
	if !isValidDBName(dbName) {
		return fmt.Errorf("invalid database name")
	}

	builder := NewSQLBuilder(instance.DBType)
	sysDB := systemDBName(instance.DBType)
	switch instance.DBType {
	case DBTypeMySQL, DBTypePostgreSQL:
		if _, err := s.runnerFor(instance).Exec(ctx, instance, sysDB, builder.BuildDropDatabase(dbName)); err != nil {
			return fmt.Errorf("drop database failed: %s", SanitizeSQLError(err.Error()))
		}
	default:
		return fmt.Errorf("database deletion not supported for %s", instance.DBType)
	}

	return nil
}

// --- DB User CRUD (live, server-owned) ---

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

// queryUsers lists database users live from the database server (the server owns them).
func (s *Service) queryUsers(ctx context.Context, instance *DBInstance) ([]DBUser, error) {
	switch instance.DBType {
	case DBTypeMySQL:
		res, err := s.runnerFor(instance).Query(ctx, instance, systemDBName(instance.DBType),
			"SELECT user, host FROM mysql.user WHERE user NOT IN ('mysql.session','mysql.sys','mysql.infoschema') ORDER BY user, host")
		if err != nil {
			return nil, fmt.Errorf("list users failed: %s", SanitizeSQLError(err.Error()))
		}
		var users []DBUser
		for _, row := range res.Rows {
			users = append(users, DBUser{Username: str(row, 0), Host: str(row, 1)})
		}
		return users, nil
	case DBTypePostgreSQL:
		res, err := s.runnerFor(instance).Query(ctx, instance, systemDBName(instance.DBType),
			"SELECT rolname FROM pg_roles WHERE rolcanlogin ORDER BY rolname")
		if err != nil {
			return nil, fmt.Errorf("list users failed: %s", SanitizeSQLError(err.Error()))
		}
		var users []DBUser
		for _, row := range res.Rows {
			users = append(users, DBUser{Username: str(row, 0)})
		}
		return users, nil
	case DBTypeRedis:
		return []DBUser{}, nil
	default:
		return nil, fmt.Errorf("unsupported db type: %s", instance.DBType)
	}
}

// isAdminUser reports whether username is the database's built-in administrator.
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

	builder := NewSQLBuilder(instance.DBType)
	sysDB := systemDBName(instance.DBType)
	switch instance.DBType {
	case DBTypeMySQL, DBTypePostgreSQL:
		sqlStr := builder.BuildCreateUser(req.Username, req.Password, host)
		if _, err := s.runnerFor(instance).Exec(ctx, instance, sysDB, sqlStr); err != nil {
			return nil, fmt.Errorf("create user failed: %s", SanitizeSQLError(err.Error()))
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

	builder := NewSQLBuilder(instance.DBType)
	sysDB := systemDBName(instance.DBType)
	switch instance.DBType {
	case DBTypeMySQL, DBTypePostgreSQL:
		sqlStr := builder.BuildDropUser(username, host)
		if _, err := s.runnerFor(instance).Exec(ctx, instance, sysDB, sqlStr); err != nil {
			return fmt.Errorf("drop user failed: %s", SanitizeSQLError(err.Error()))
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

	// ValidatePrivileges is the per-engine whitelist (MySQL and PG differ, e.g.
	// INDEX exists only for MySQL); BuildGrant re-validates on generation.
	validated := ValidatePrivileges(instance.DBType, req.Privileges)
	if validated == "" {
		return fmt.Errorf("invalid privileges: %s", req.Privileges)
	}

	builder := NewSQLBuilder(instance.DBType)
	sysDB := systemDBName(instance.DBType)
	switch instance.DBType {
	case DBTypeMySQL, DBTypePostgreSQL:
		sqlStr := builder.BuildGrant(validated, req.Database, username, host)
		if _, err := s.runnerFor(instance).Exec(ctx, instance, sysDB, sqlStr); err != nil {
			return fmt.Errorf("grant failed: %s", SanitizeSQLError(err.Error()))
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
	return s.runtime.CopyFrom(ctx, instance.ContainerEngine, instance.ContainerName, "/data/dump.rdb", backup.FilePath)
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
	if err := s.runtime.CopyTo(ctx, instance.ContainerEngine, instance.ContainerName, backup.FilePath, target); err != nil {
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
	if err := s.runtime.CopyTo(ctx, instance.ContainerEngine, instance.ContainerName, backup.FilePath, target); err != nil {
		return fmt.Errorf("copy backup into container: %w", err)
	}
	if _, err := s.runInVersion(ctx, instance, "pg_restore", "-d", backup.DatabaseName, "-c", target); err != nil {
		return fmt.Errorf("pg_restore failed: %s", SanitizeSQLError(err.Error()))
	}
	return nil
}

func (s *Service) restoreRedis(ctx context.Context, backup *DBBackup) error {
	instance, err := s.repo.GetInstance(ctx, backup.DBInstanceID)
	if err != nil || instance == nil {
		return fmt.Errorf("database instance not found")
	}
	if err := s.runtime.Stop(ctx, instance.ContainerEngine, instance.ContainerName); err != nil {
		return fmt.Errorf("stop Redis failed: %w", err)
	}
	if err := s.runtime.CopyTo(ctx, instance.ContainerEngine, instance.ContainerName, backup.FilePath, "/data/dump.rdb"); err != nil {
		return fmt.Errorf("copy Redis backup: %w", err)
	}
	if err := s.runtime.Start(ctx, instance.ContainerEngine, instance.ContainerName); err != nil {
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

// runInVersion runs a command inside the instance's container via the CLI
// runtime, with admin credentials injected. Used for engine-side tooling that
// has no driver equivalent (mysqldump / pg_dump / redis-cli) and config
// reloads — never for SQL data operations, which use the driver channel.
func (s *Service) runInVersion(ctx context.Context, instance *DBInstance, args ...string) (string, error) {
	if instance == nil || instance.ContainerEngine == "" || instance.ContainerName == "" {
		return "", fmt.Errorf("database instance is not container-managed")
	}
	args = s.withAdminCredentials(instance, args)
	return s.runtime.Exec(ctx, instance.ContainerEngine, instance.ContainerName, args...)
}

func (s *Service) withAdminCredentials(instance *DBInstance, args []string) []string {
	if len(args) == 0 || instance.AdminPassword == "" {
		return args
	}
	password := instance.AdminPassword
	switch instance.DBType {
	case DBTypeMySQL:
		// Password via MYSQL_PWD env instead of `-p` on the command line: mysql
		// prints "Using a password on the command line interface can be insecure."
		// to stderr on every `-p` invocation, and stderr is merged into the parsed
		// output (RunCombined), so the warning would surface as a bogus first row
		// in tabular listings. `exec -e` injects the env before the command.
		return append([]string{"-e", "MYSQL_PWD=" + password, args[0], "-uroot"}, args[1:]...)
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
	builder := NewSQLBuilder(instance.DBType)
	res, err := s.runnerFor(instance).Query(ctx, instance, dbName, builder.BuildListTables())
	if err != nil {
		return nil, fmt.Errorf("获取表列表失败: %s", SanitizeSQLError(err.Error()))
	}
	for _, row := range res.Rows {
		tables = append(tables, map[string]interface{}{"name": str(row, 0)})
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

	res, err := s.runnerFor(instance).Query(ctx, instance, dbName, describeSQL)
	if err != nil {
		return nil, fmt.Errorf("获取表结构失败: %s", SanitizeSQLError(err.Error()))
	}

	// The driver channel returns structured describe rows (name, type, nullable,
	// default, pk-flag columns); the CLI channel parses the same shape from text.
	tableInfo := tableInfoFromQuery(instance.DBType, tableName, res)

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

	instance, err := s.getInstanceForSQL(ctx, instanceID, dbName)
	if err != nil {
		return nil, err
	}
	dbType := instance.DBType
	builder := NewSQLBuilder(dbType)

	var total int
	var headers []string
	var columnTypes []string
	var rows [][]interface{}
	switch dbType {
	case DBTypeMySQL, DBTypePostgreSQL:
		countRes, err := s.runnerFor(instance).Query(ctx, instance, dbName, builder.BuildCount(tableName))
		if err == nil && len(countRes.Rows) > 0 {
			fmt.Sscanf(str(countRes.Rows[0], 0), "%d", &total)
		}
		res, err := s.runnerFor(instance).Query(ctx, instance, dbName, builder.BuildSelect(tableName, nil, page, pageSize))
		if err != nil {
			return nil, fmt.Errorf("查询失败: %s", SanitizeSQLError(err.Error()))
		}
		for _, c := range res.Columns {
			headers = append(headers, c.Name)
			columnTypes = append(columnTypes, c.Type)
		}
		rows = make([][]interface{}, len(res.Rows))
		for i, row := range res.Rows {
			rows[i] = row
		}
	}

	return &PagedQueryResult{
		Headers:     headers,
		ColumnTypes: columnTypes,
		Rows:        rows,
		Total:       total,
		Page:        page,
		PageSize:    pageSize,
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

	if _, execErr := s.runnerFor(instance).Exec(ctx, instance, dbName, sql); execErr != nil {
		log.Printf("ExecuteSQL %s error [db=%s]: %s", instance.DBType, dbName, SanitizeSQLError(execErr.Error()))
		return &DMLResult{Success: false, Error: SanitizeSQLError(execErr.Error())}, nil
	}
	return &DMLResult{Success: true}, nil
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

	if dryRun {
		return &DMLResult{Success: true, DryRun: true, SQL: builder.BuildInsert(table, data, nil)}, nil
	}

	params, args := builder.BuildInsertParams(table, data, nil)
	if _, execErr := s.runnerFor(instance).Exec(ctx, instance, dbName, params, args...); execErr != nil {
		return &DMLResult{Success: false, Error: SanitizeSQLError(execErr.Error())}, nil
	}
	return &DMLResult{Success: true}, nil
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

	if dryRun {
		return &DMLResult{Success: true, DryRun: true, SQL: builder.BuildUpdate(table, data, pk, pkVal)}, nil
	}

	params, args := builder.BuildUpdateParams(table, data, pk, pkVal)
	if _, execErr := s.runnerFor(instance).Exec(ctx, instance, dbName, params, args...); execErr != nil {
		return &DMLResult{Success: false, Error: SanitizeSQLError(execErr.Error())}, nil
	}
	return &DMLResult{Success: true}, nil
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

	if dryRun {
		return &DMLResult{Success: true, DryRun: true, SQL: builder.BuildDelete(table, pk, pkVal)}, nil
	}

	params, args := builder.BuildDeleteParams(table, pk, pkVal)
	if _, execErr := s.runnerFor(instance).Exec(ctx, instance, dbName, params, args...); execErr != nil {
		return &DMLResult{Success: false, Error: SanitizeSQLError(execErr.Error())}, nil
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

	if _, execErr := s.runnerFor(instance).Exec(ctx, instance, dbName, sql); execErr != nil {
		return fmt.Errorf("创建表失败: %s", SanitizeSQLError(execErr.Error()))
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

	if _, execErr := s.runnerFor(instance).Exec(ctx, instance, dbName, sql); execErr != nil {
		return fmt.Errorf("删除表失败: %s", SanitizeSQLError(execErr.Error()))
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

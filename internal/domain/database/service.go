package database

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"easyserver/internal/domain/notification"
	"easyserver/internal/infra/config"
	"easyserver/internal/infra/errx"
	"easyserver/internal/infra/task"
	"easyserver/internal/util"
)

const (
	maxLogLines     = 5000
	defaultLogLines = 200
)

// Service manages the whole database domain: container-backed instances
// (lifecycle) and the logical databases, users, backups and SQL inside them.
type Service struct {
	repo           Repository
	runtime        DatabaseRuntime        // shared runtime for normal (non-install) ops
	taskMgr        *task.Manager          // background install executor (key=DBType 去重)
	runtimeFactory func() DatabaseRuntime // builds the runtime for background installs
	driver         SQLRunner              // direct driver channel (MySQL/PostgreSQL)
	redisOps       *redisRunner           // direct Redis channel (key browser ops)
	notifSink      notification.Sink      // optional: failure notifications (nil = disabled)

	restoreMu   sync.Mutex
	restoreTask map[int64]*RestoreStatus // backupID → 恢复任务内存态（恢复不写备份行）
}

// NewService creates a database Service over the given Repository and container
// runtime. Production passes NewSocketContainerRuntime(); tests pass a fake
// DatabaseRuntime. Sweeps orphaned backup rows (running → failed) from a
// previous crashed process.
func NewService(repo Repository, runtime DatabaseRuntime) *Service {
	return NewServiceWithSink(repo, runtime, nil)
}

// NewServiceWithSink 在 NewService 基础上附加通知 sink。sink 为 nil 时安装失败
// 不发送站内通知（测试与未接线场景）。
func NewServiceWithSink(repo Repository, runtime DatabaseRuntime, sink notification.Sink) *Service {
	s := &Service{
		repo:    repo,
		runtime: runtime,
		taskMgr: task.NewManager(8),
		runtimeFactory: func() DatabaseRuntime {
			return runtime
		},
		driver:      newDriverSQLRunner(),
		redisOps:    newRedisRunner(),
		restoreTask: make(map[int64]*RestoreStatus),
		notifSink:   sink,
	}
	s.SweepOrphanBackups(context.Background())
	return s
}

// runnerFor returns the SQL channel for an instance. All SQL runs over the
// direct driver connection; Redis surfaces as clear errors from the channel
// instead of falling back.
func (s *Service) runnerFor(inst *DBInstance) SQLRunner {
	return s.driver
}

// redisFor returns the direct Redis channel for an instance (key browser ops).
func (s *Service) redisFor() *redisRunner {
	return s.redisOps
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
	_ = s.repo.UpdateInstanceStatus(ctx, v.ID, status)
}

// --- Instance lifecycle ---

func (s *Service) ListInstances(ctx context.Context, dbType DBType) ([]DBInstance, error) {
	if !IsValidDBType(dbType) {
		return nil, fmt.Errorf("unsupported database type %q", dbType)
	}
	return s.repo.ListInstances(ctx, dbType)
}

func (s *Service) GetInstance(ctx context.Context, id int64) (*DBInstance, error) {
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
	if !IsValidDBType(dbType) {
		return nil, fmt.Errorf("unsupported database type %q", dbType)
	}

	// 单实例约束按"数据目录归属"判定：目录 key 是 sanitize 后的 version（8.0. 与
	// 8.0 指向同一目录），逐个比对已存在实例的 dir key，避免原始版本写法不同
	// 绕过约束而共享一个数据目录。
	instances, err := s.repo.ListInstances(ctx, dbType)
	if err != nil {
		return nil, err
	}
	dirKey := instanceDirKey(dbType, req.Version)
	for _, inst := range instances {
		if instanceDirKey(inst.DBType, inst.Version) == dirKey {
			return nil, errx.Conflict("version %s is already installed for %s", req.Version, dbType)
		}
	}

	// The client sends the image + version (the front-end owns the version/image
	// catalogue); the image is required — without it there is nothing to pull.
	if strings.TrimSpace(req.Image) == "" {
		return nil, errors.New("image is required")
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
		return nil, errors.New("port must be between 1 and 65535")
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
	// 数据目录是宿主绝对路径（/opt/easyserver/db/<type>-<version>/data），不再有命名卷。
	// 同 <type>-<version> 只允许一个实例（上面已查），目录归属唯一。
	volumeName := hostDataDir(dbType, req.Version)
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
		DBType: dbType, Version: req.Version, Port: port, Status: "installing",
		ContainerEngine: engineName, Image: req.Image, ContainerName: containerName,
		VolumeName: volumeName, ConfigDir: spec.ConfigDir, BindAddress: bindAddress, AdminPassword: password,
	}
	id, err := s.repo.CreateInstance(ctx, row)
	if err != nil {
		return nil, fmt.Errorf("write instance record: %w", err)
	}
	if _, err := s.taskMgr.StartWithLog(ctx, containerName, task.Options{}, func(ctx context.Context, log *task.TaskLog) error {
		rt := s.runtimeFactory()
		if sockRt, ok := rt.(*SocketContainerRuntime); ok {
			dedicatedRt := NewSocketContainerRuntime(sockRt.client)
			dedicatedRt.SetOutputHook(func(line string) { log.Append(line) })
			rt = dedicatedRt
		}
		// Detach from the request context: the install outlives the HTTP request
		// (which is canceled once CreateInstance responds), so it must not inherit
		// its cancellation — the per-task context from the task executor drives it,
		// and CancelInstall cancels that.
		installErr := s.installInstance(ctx, id, dbType, req.Version, req.Image, engineName, containerName, password, spec, rt, log)
		if installErr != nil && ctx.Err() == nil && !errors.Is(installErr, context.Canceled) {
			// 非取消的真实失败才发站内通知；用户取消（ctx 已取消或错误本身是
			// Canceled）是安静操作。业务上下文（版本/类型）在此闭包内，正是
			// 调用方按需发送的理由。
			title := fmt.Sprintf("%s %s 安装失败", dbType, req.Version)
			s.notifyInstallFailed(title, fmt.Sprintf("安装 %s (%s) 失败: %v", req.Image, containerName, installErr))
		}
		return installErr
	}); err != nil {
		// Duplicate install (same container already installing) or concurrency
		// limit: the row was written above but no task started — remove it so the
		// panel doesn't show a phantom "installing" entry.
		_ = s.repo.DeleteInstance(ctx, id)
		return nil, err
	}
	return &CreateInstanceResult{InstallID: containerName, InstanceID: id, Version: req.Version, Image: req.Image, Port: port, Status: "installing"}, nil
}

// notifyInstallFailed 向站内通知投递安装失败。sink 未接线（nil）时静默跳过；
// 发送失败只记日志，不阻断调用方（通知是旁路，失败不影响安装结果）。
func (s *Service) notifyInstallFailed(title, message string) {
	if s.notifSink == nil {
		return
	}
	if _, err := s.notifSink.CreateIfNotExists(notification.CreateNotificationRequest{
		Type:    "deploy",
		Title:   title,
		Message: message,
		Level:   "error",
	}); err != nil {
		log.Printf("database: notify install failed %q: %v", title, err)
	}
}

// installInstance runs the container creation pipeline and reports progress
// into log. rt is an install-scoped runtime whose command output is hooked into
// log. The instance row already exists (status "installing", written up front by
// CreateInstance); this flips it to "running" on success / "failed" on error, or
// removes it entirely when the user cancels. ctx is the per-task cancel context
// from the task executor.
func (s *Service) installInstance(ctx context.Context, id int64, dbType DBType, version, image, engineName, containerName, password string, spec ContainerSpec, rt DatabaseRuntime, log *task.TaskLog) error {
	canceled := func() bool { return ctx.Err() != nil }
	// removeInstance is the cancel cleanup: drop the container and the instance
	// row — the user aborted, so nothing lingers (a failed install keeps its row
	// for inspection; a canceled one does not).
	removeInstance := func() {
		_ = rt.Remove(ctx, engineName, containerName)
		_ = s.repo.DeleteInstance(ctx, id)
	}
	fail := func(msg string, err error) error {
		if canceled() {
			removeInstance()
			log.Append("❌ 安装已取消")
			return errors.New("安装已取消")
		}
		// 失败时保留容器，便于排查失败现场（容器日志还在）。重新安装走
		// "卸载+安装"两步，卸载会先删掉这个残留容器，所以不会被占用卡住。
		_ = s.repo.UpdateInstanceStatus(ctx, id, "failed")
		log.Append("❌ " + msg + ": " + err.Error())
		return err
	}

	log.Append("开始安装 " + image + " ...")
	// 数据/配置目录是宿主挂载：先建好并 chown 给容器进程（uid 999），否则容器内
	// mysqld/redis 无法写数据目录（Redis 的 CONFIG REWRITE 也是这个原因才从命名卷
	// 迁到宿主路径）。Redis 同时预置空 redis.conf（配置文件必须存在，见函数注释）。
	if err := prepareHostDirs(spec); err != nil {
		return fail("准备宿主数据目录失败", err)
	}
	if err := rt.Create(ctx, spec); err != nil {
		if canceled() {
			removeInstance()
			log.Append("❌ 安装已取消")
			return errors.New("安装已取消")
		}
		// No container was created — still flip the row to "failed" so the
		// instance doesn't sit at "installing" forever (the log panel surfaces
		// the error and offers reinstall).
		_ = s.repo.UpdateInstanceStatus(ctx, id, "failed")
		log.Append("❌ 创建容器失败: " + err.Error())
		return err
	}
	log.Append("容器已创建，启动服务...")

	if err := rt.Start(ctx, engineName, containerName); err != nil {
		return fail("启动容器失败", err)
	}
	// 等待就绪不设超时：数据库初始化（尤其首次拉镜像后）没有固定时长，卡住时
	// 由容器退出（exited 快失败）或用户取消来终止，而不是倒计时误杀。
	if _, err := waitForHealthy(ctx, rt, engineName, containerName, 0); err != nil {
		return fail("数据库未就绪", err)
	}
	log.Append("✅ 安装完成，数据库已就绪")
	if err := s.repo.UpdateInstanceStatus(ctx, id, "running"); err != nil {
		return err
	}
	return nil
}

// CancelInstall aborts an in-flight install. The goroutine observes the cancel
// at its next command boundary (image pull, create, start, health poll) and
// removes the container and the instance row before finishing — a canceled
// install leaves no row behind, unlike a failed one.
func (s *Service) CancelInstall(installID string) error {
	if !s.taskMgr.Cancel(installID) {
		return errx.NotFound("安装已结束或不存在")
	}
	return nil
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
func (s *Service) ListDockerTags(ctx context.Context, dbType DBType) ([]string, error) {
	if !IsValidDBType(dbType) {
		return nil, fmt.Errorf("unsupported database type %q", dbType)
	}
	return s.dockerTags(ctx, dbType)
}

func (s *Service) dockerTags(ctx context.Context, dbType DBType) ([]string, error) {
	dockerTagsCacheStore.Lock()
	defer dockerTagsCacheStore.Unlock()
	if c, ok := dockerTagsCacheStore.m[dbType]; ok && time.Since(c.fetched) < dockerTagsCacheTTL {
		return c.tags, nil
	}
	tags, err := fetchDockerHubTags(ctx, dockerImageBase(dbType))
	if err != nil {
		return nil, err
	}
	dockerTagsCacheStore.m[dbType] = dockerTagsCache{tags: tags, fetched: time.Now()}
	return tags, nil
}

// dockerHubTagPage mirrors one paginated tag page from hub.docker.com. Only
// the fields the picker needs are kept.
type dockerHubTagPage struct {
	Next    string `json:"next"`
	Results []struct {
		Name string `json:"name"`
	} `json:"results"`
}

// fetchDockerHubTags lists every published version-like tag for an official
// Docker Hub image (library/xxx). It walks the API's pagination (page_size=100)
// until exhausted; tags that don't look like a version (e.g. latest,
// oraclelinux9) are dropped. The front-end "更多版本" flow calls this so users
// can install any published tag, not just the curated presets.
func fetchDockerHubTags(ctx context.Context, image string) ([]string, error) {
	client := &http.Client{Timeout: 15 * time.Second}
	seen := make(map[string]bool)
	var tags []string
	url := fmt.Sprintf("https://hub.docker.com/v2/repositories/library/%s/tags?page_size=100", image)
	for url != "" {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("User-Agent", "easyserver")
		resp, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("查询 Docker Hub 失败: %w", err)
		}
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			return nil, fmt.Errorf("docker hub 返回 %s", resp.Status)
		}
		var page dockerHubTagPage
		if err := json.NewDecoder(resp.Body).Decode(&page); err != nil {
			resp.Body.Close()
			return nil, fmt.Errorf("解析 Docker Hub 响应失败: %w", err)
		}
		resp.Body.Close()
		for _, r := range page.Results {
			if r.Name == "" || seen[r.Name] || !versionLike(r.Name) {
				continue
			}
			seen[r.Name] = true
			tags = append(tags, r.Name)
		}
		url = page.Next
	}
	return tags, nil
}

// versionLike reports whether a tag looks like a version number the picker
// should offer. Plain version tags (8.4, 8.4.11) and alpine variants
// (7-alpine, used by Redis) are kept; platform-specific tags such as
// "8.4-oraclelinux9" or "8.4-bullseye" are noise and dropped.
func versionLike(tag string) bool {
	if tag == "" {
		return false
	}
	if !unicode.IsDigit([]rune(tag)[0]) {
		return false
	}
	if found := strings.Contains(tag, "-"); found {
		return strings.HasSuffix(tag, "-alpine")
	}
	return true
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

// UninstallInstance removes the managed container. The host data directory is
// retained by default so the instance can be re-installed onto it; purge
// deletes the whole instance directory (data + config + backups) too.
func (s *Service) UninstallInstance(ctx context.Context, instanceID int64, purge bool) error {
	v, err := s.GetInstance(ctx, instanceID)
	if err != nil || v == nil {
		return errx.NotFound("instance not found")
	}

	if err := s.runtime.Remove(ctx, v.ContainerEngine, v.ContainerName); err != nil {
		return fmt.Errorf("remove database container: %w", err)
	}
	// Drop cached direct-connection pools so a reinstall reconnects fresh.
	s.driver.Close(instanceID)
	s.redisOps.Close(instanceID)
	if purge {
		// 数据目录是宿主路径，整目录（data + config + es_backups/ 备份）直接 RemoveAll，
		// 不再走引擎卷删除。
		if err := os.RemoveAll(filepath.Dir(v.VolumeName)); err != nil {
			return fmt.Errorf("remove database data directory: %w", err)
		}
	}
	return s.repo.DeleteInstance(ctx, instanceID)
}

// ResetAdminPassword rotates the administrator password and returns it once.
func (s *Service) ResetAdminPassword(ctx context.Context, instanceID int64) (string, error) {
	v, err := s.GetInstance(ctx, instanceID)
	if err != nil || v == nil {
		return "", errx.NotFound("instance not found")
	}
	if v.ContainerEngine == "" || v.ContainerName == "" {
		return "", errors.New("database instance is not container-managed")
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
		return "", errors.New("password reset is not supported for this database type")
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
	v, err := s.GetInstance(ctx, instanceID)
	if err != nil || v == nil {
		return errx.NotFound("instance not found")
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
	v, err := s.GetInstance(ctx, instanceID)
	if err != nil || v == nil {
		return errx.NotFound("instance not found")
	}
	if err := s.runtime.Stop(ctx, v.ContainerEngine, v.ContainerName); err != nil {
		return fmt.Errorf("stop failed: %w", err)
	}
	return s.repo.UpdateInstanceStatus(ctx, instanceID, "stopped")
}

func (s *Service) RestartInstance(ctx context.Context, instanceID int64) error {
	v, err := s.GetInstance(ctx, instanceID)
	if err != nil || v == nil {
		return errx.NotFound("instance not found")
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
	v, err := s.GetInstance(ctx, instanceID)
	if err != nil || v == nil {
		return "", errx.NotFound("instance not found")
	}
	if lines <= 0 {
		lines = defaultLogLines
	}
	if lines > maxLogLines {
		lines = maxLogLines
	}
	return s.runtime.Logs(ctx, v.ContainerEngine, v.ContainerName, lines)
}

// GetInstanceConfig 返回实例的结构化配置：驱动读面板参数的运行时值（读到啥返回啥）
// + 编辑元数据。port 是实例级状态（DB 的 port 列，容器映射管理），始终显示当前值。
// 配置读写都需要实例运行（驱动连接），停止的实例返回明确错误。
func (s *Service) GetInstanceConfig(ctx context.Context, instanceID int64) (*InstanceConfigView, error) {
	v, err := s.GetInstance(ctx, instanceID)
	if err != nil || v == nil {
		return nil, errx.NotFound("instance not found")
	}
	if err := s.ensureInstanceRunning(ctx, v, "读取配置"); err != nil {
		return nil, err
	}
	params, err := s.readConfigValues(ctx, v)
	if err != nil {
		return nil, err
	}
	params["port"] = strconv.Itoa(v.Port)
	return &InstanceConfigView{Params: params, Meta: configParams(v.DBType)}, nil
}

// SaveInstanceConfig 保存结构化配置并立即生效：
//  1. 只应用传入的非空覆盖值（空值 = 无操作，前端已过滤，后端兜底跳过）；
//  2. 实例运行中（驱动持久化需要连接）——覆盖值直接写入数据库自身持久化机制
//     （SET PERSIST / ALTER SYSTEM / CONFIG SET+REWRITE），在线生效；
//  3. port 参数变化 → 重建容器更新端口映射（port 是容器映射，驱动改不了）；
//  4. PG 的 postmaster 级参数 reload 不生效 → 重启容器；
//     其余参数驱动已在线生效，不重启。
func (s *Service) SaveInstanceConfig(ctx context.Context, instanceID int64, params map[string]string) error {
	v, err := s.GetInstance(ctx, instanceID)
	if err != nil || v == nil {
		return errx.NotFound("instance not found")
	}

	// 组装本次覆盖值，空值跳过（不清覆盖、不重置 —— 保存只应用改过的字段）。
	filtered := make(map[string]string)
	for key, value := range params {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			filtered[key] = trimmed
		}
	}
	newPort := v.Port
	if raw, ok := filtered["port"]; ok {
		if p, err := strconv.Atoi(raw); err == nil && p >= 1 && p <= 65535 {
			newPort = p
		}
		delete(filtered, "port")
	}

	if err := s.ensureInstanceRunning(ctx, v, "保存配置"); err != nil {
		return err
	}
	restart, err := s.applyConfigValues(ctx, v, filtered)
	if err != nil {
		return err
	}
	if newPort != v.Port {
		return s.recreateInstanceContainer(ctx, v, newPort)
	}
	if restart {
		return s.RestartInstance(ctx, instanceID)
	}
	return nil
}

// ensureInstanceRunning 检查实例容器在运行 —— 配置读写走驱动，连接需要端口映射
// 可用。返回的错误含"未运行"，经 WrapError 映射为 409。
func (s *Service) ensureInstanceRunning(ctx context.Context, v *DBInstance, action string) error {
	info, err := s.runtime.Status(ctx, v.ContainerEngine, v.ContainerName)
	if err != nil || info.State != "running" {
		return errx.Conflict("实例未运行，无法%s（请先启动实例）", action)
	}
	return nil
}

// recreateInstanceContainer 用新端口重建容器（移除旧映射、按新端口 create/start），
// 数据卷保留；端口是容器映射，驱动改不了，只能重建。保存配置里 port 参数时调用。
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
	// 端口变了，缓存池（driver/redis 按 (instance, db) 缓存，DSN 固化旧端口）必须
	// 清掉，下次查询才按新端口重连。
	s.driver.Close(v.ID)
	s.redisOps.Close(v.ID)
	return s.repo.UpdateInstancePort(ctx, v.ID, newPort)
}

// postgresMajor extracts the PostgreSQL major version from an image reference
// (e.g. "docker.io/postgres:18-alpine" → 18, "postgres:18@sha256:..." → 18).
// Returns 0 when the tag carries no leading version number.
func postgresMajor(image string) int {
	if at := strings.Index(image, "@"); at >= 0 {
		image = image[:at]
	}
	slash := strings.LastIndex(image, "/")
	colon := strings.LastIndex(image, ":")
	if colon < 0 || colon < slash {
		return 0
	}
	var major int
	if n, err := fmt.Sscanf(image[colon+1:], "%d", &major); err != nil || n != 1 {
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

// RefreshStatus refreshes instance statuses for a database type (dbType).
func (s *Service) RefreshStatus(ctx context.Context, dbType DBType) {
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

// hostDBBaseDir 是数据库实例数据/配置目录的宿主根。每个实例占
// <base>/<dbtype>-<version>/{data,config}，目录与实例一一对应（同类型+版本只允许
// 一个实例）。变量而非常量：测试改指向临时目录，避免单元测试真实操作 /opt。
var hostDBBaseDir = config.DataRoot + "/db"

// containerUID 是官方 mysql/postgres/redis 镜像内数据进程的 uid（均为 999）。
// 宿主目录必须 chown 给它，容器内进程才能写数据目录与 CONFIG REWRITE 写回配置。
const containerUID = 999

// instanceDirKey 是某 (类型,版本) 实例在宿主上的目录段，如 mysql-8.0。
func instanceDirKey(dbType DBType, version string) string {
	return sanitizeName(string(dbType)) + "-" + sanitizeVersion(version)
}

// sanitizeVersion 保留版本里的点（8.0 / 18-alpine 是合法的路径段），其余
// 非法路径字符一律压成 '-'。
func sanitizeVersion(version string) string {
	version = strings.ToLower(strings.TrimSpace(version))
	var b strings.Builder
	for _, ch := range version {
		if (ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9') || ch == '-' || ch == '.' {
			b.WriteRune(ch)
		} else {
			b.WriteByte('-')
		}
	}
	return strings.Trim(b.String(), "-.")
}

func hostDataDir(dbType DBType, version string) string {
	return filepath.Join(hostDBBaseDir, instanceDirKey(dbType, version), "data")
}

func hostConfigDir(dbType DBType, version string) string {
	return filepath.Join(hostDBBaseDir, instanceDirKey(dbType, version), "config")
}

// prepareHostDirs 创建实例的宿主数据/配置目录并 chown 给容器进程（uid 999）。
// Redis 还预置空的 redis.conf —— redis-server 以配置文件路径启动时文件不存在是
// 致命错误（不是忽略），空文件 + 命令行参数即等价于"默认配置 + --requirepass"。
func prepareHostDirs(spec ContainerSpec) error {
	for _, dir := range []string{spec.Volume, spec.ConfigVolume} {
		if dir == "" {
			continue
		}
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("create data directory %s: %w", dir, err)
		}
		// chown 999:999 只对 root 可行（rootful Podman 是 ADR-0006 的支持模式）。
		// 非 root（rootless / 单元测试）跳过：容器进程映射的 uid 不由面板决定，
		// chown 也会因 EPERM 失败。
		if os.Geteuid() == 0 {
			if err := os.Chown(dir, containerUID, containerUID); err != nil {
				return fmt.Errorf("chown data directory %s: %w", dir, err)
			}
		}
	}
	if spec.ConfigVolume != "" {
		conf := filepath.Join(spec.ConfigVolume, "redis.conf")
		if err := os.WriteFile(conf, nil, 0644); err != nil {
			return fmt.Errorf("create redis.conf: %w", err)
		}
		if os.Geteuid() == 0 {
			if err := os.Chown(conf, containerUID, containerUID); err != nil {
				return fmt.Errorf("chown redis.conf: %w", err)
			}
		}
	}
	return nil
}

// containerDataDir 返回某实例数据目录在容器内的路径（宿主挂载的落点）。备份
// 从宿主数据目录推导 es_backups/ 后，映射回容器路径供容器内 dump 直接写入。
func containerDataDir(instance *DBInstance) string {
	switch instance.DBType {
	case DBTypePostgreSQL:
		return pgDataDir(instance.Image)
	case DBTypeRedis:
		return "/data"
	default:
		return "/var/lib/mysql"
	}
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
		return errors.New("容器名只能包含字母、数字以及 _ . -，且必须以字母或数字开头")
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
	return util.RandomBase64(24)
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
	default:
		return 3306
	}
}

// containerSpec builds the structured create contract. volume is the host data
// directory (DBInstance.VolumeName, /opt/easyserver/db/<type>-<version>/data);
// Redis additionally mounts the host config directory and starts redis-server on
// the empty redis.conf + --requirepass (see prepareHostDirs).
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
		// 配置卷是宿主 config/ 目录（不是命名卷）：CONFIG REWRITE 直接写回宿主可见
		// 的 redis.conf，不再有一次性 seed 容器和 root 属主写权限问题。
		configVolume, configDir = hostConfigDir(dbType, version), "/usr/local/etc/redis"
		// 配置文件必须是第一个位置参数：Redis 8 会把 --requirepass 之后所有
		// token 当作其值，选项放在配置路径后面则路径被吞成第二个参数报错。
		command = []string{"redis-server", configDir + "/redis.conf", "--requirepass", password}
	default: // mysql — 配置由驱动持久化（SET PERSIST），不挂配置卷
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

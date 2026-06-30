package runtimeenv

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"easyserver/internal/envconfig"
	"easyserver/internal/infra/executor"
)

type Service struct {
	repo         Repository
	executor     executor.CommandExecutor
	installLocks sync.Map
}

// envKeyPattern is the POSIX-ish env-var name whitelist used by CreateMirror
// to keep user-supplied keys from injecting newlines or '[' into the generated
// /etc/mise/config.toml. Matches: leading letter/underscore, then any letter,
// digit, or underscore. See applyDefault / buildMiseConfigContent.
var envKeyPattern = regexp.MustCompile(`^[A-Z_][A-Z0-9_]*$`)

func NewService(repo Repository, exec executor.CommandExecutor) *Service {
	return &Service{
		repo:     repo,
		executor: exec,
	}
}

// InitMirrors initializes the default mirrors if the table is empty
func (s *Service) InitMirrors(ctx context.Context) error {
	// Boot-time state healing (AC3)
	if err := s.repo.HealState(ctx); err != nil {
		log.Printf("runtime: failed to heal state: %v", err)
	}

	count, err := s.repo.CountMirrors(ctx)
	if err != nil {
		return err
	}
	if count == 0 {
		mirrors := []RuntimeMirror{
			{Lang: "node", EnvKey: "MISE_NODE_MIRROR_URL", EnvValue: "https://npmmirror.com/mirrors/node", Enabled: 1, Source: "seed"},
			{Lang: "node", EnvKey: "MISE_NODE_MIRROR_URL", EnvValue: "https://nodejs.org/dist", Enabled: 0, Source: "seed"},
			{Lang: "node", EnvKey: "MISE_NODE_MIRROR_URL", EnvValue: "https://mirrors.tuna.tsinghua.edu.cn/nodejs-release", Enabled: 0, Source: "seed"},
			{Lang: "go", EnvKey: "MISE_GO_DOWNLOAD_MIRROR", EnvValue: "https://mirrors.aliyun.com/golang", Enabled: 1, Source: "seed"},
			{Lang: "go", EnvKey: "MISE_GO_DOWNLOAD_MIRROR", EnvValue: "https://go.dev/dl", Enabled: 0, Source: "seed"},
			{Lang: "go", EnvKey: "MISE_GO_DOWNLOAD_MIRROR", EnvValue: "https://mirrors.ustc.edu.cn/golang", Enabled: 0, Source: "seed"},
		}
		if err := s.repo.SeedMirrors(ctx, mirrors); err != nil {
			return err
		}
	}

	// Unconditional regeneration: covers servers upgraded from pre-Issue-07
	// where global_default rows already exist but /etc/mise/config.toml has
	// no [tools] section. Idempotent on subsequent boots.
	if err := s.GenerateMiseConfig(ctx); err != nil {
		log.Printf("runtime: failed to generate mise config on boot: %v", err)
		return err
	}

	return nil
}

// ListAll returns all installed runtime environments
func (s *Service) ListAll(ctx context.Context) ([]RuntimeEnvironment, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	envs, err := s.repo.ListAll(ctx)
	if err != nil {
		return nil, err
	}
	for i := range envs {
		envs[i].Path = miseInstallPath(envs[i].Name, envs[i].Version)
	}
	return envs, nil
}

// ListByName returns all versions of a specific runtime environment
func (s *Service) ListByName(ctx context.Context, name string) ([]RuntimeEnvironment, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	envs, err := s.repo.ListByName(ctx, name)
	if err != nil {
		return nil, err
	}
	for i := range envs {
		envs[i].Path = miseInstallPath(envs[i].Name, envs[i].Version)
	}
	return envs, nil
}

// GetDefault returns the default version of a runtime environment
func (s *Service) GetDefault(ctx context.Context, name string) (*RuntimeEnvironment, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	env, err := s.repo.GetDefault(ctx, name)
	if err != nil || env == nil {
		return env, err
	}
	env.Path = miseInstallPath(env.Name, env.Version)
	return env, nil
}

// GetByID returns a runtime environment by ID
func (s *Service) GetByID(ctx context.Context, id int64) (*RuntimeEnvironment, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	env, err := s.repo.GetByID(ctx, id)
	if err != nil || env == nil {
		return env, err
	}
	env.Path = miseInstallPath(env.Name, env.Version)
	return env, nil
}

// Install installs a runtime environment
func (s *Service) Install(ctx context.Context, name, version string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if !isValidVersion(version) {
		return fmt.Errorf("invalid version format: %s", version)
	}

	var miseTool string
	for _, r := range GetCatalog() {
		if r.Lang == name {
			miseTool = r.MiseTool
			break
		}
	}
	if miseTool == "" {
		return fmt.Errorf("unsupported runtime: %s", name)
	}

	// `mise latest <tool>@<prefix>` 把"前缀版本"解析成精确版本（如 Node 选
	// 主版本 20 → 20.11.0）。但对 vfox 插件（PHP / Java）该命令对完整版本
	// 经常返回空 stdout（vfox 自己实现的 latest 行为不统一），导致这里把
	// 用户已选好的完整版本当成解析失败。
	//
	// 兜底：mise latest 失败或输出为空时，直接采用前端传入的 version。如果
	// 它确实不是合法 mise 版本，后续 `mise install` 会有明确报错。
	cmd := s.executor.Command(ctx, executor.StartOptions{}, "/usr/local/bin/mise", "latest", fmt.Sprintf("%s@%s", miseTool, version))
	cmd.Env = append(os.Environ(), "MISE_DATA_DIR=/var/lib/easyserver/mise", "MISE_YES=1")
	out, _ := cmd.Output()
	var exactVersion string
	if outLines := strings.Split(strings.TrimSpace(string(out)), "\n"); len(outLines) > 0 {
		exactVersion = strings.TrimSpace(outLines[len(outLines)-1])
	}
	if exactVersion == "" {
		exactVersion = version
	}

	lockKey := name + "@" + exactVersion
	if _, loaded := s.installLocks.LoadOrStore(lockKey, true); loaded {
		return fmt.Errorf("installation of %s is already in progress", lockKey)
	}

	var id int64
	defer func() {
		// Only remove lock if we fail BEFORE starting background routine.
		// Background routine handles its own cleanup.
		if id == 0 {
			s.installLocks.Delete(lockKey)
		}
	}()

	exists, err := s.repo.ExistsByNameAndVersion(ctx, name, exactVersion)
	if err != nil {
		return err
	}
	if exists {
		return fmt.Errorf("%s %s is already installed", name, exactVersion)
	}

	id, err = s.repo.Create(ctx, name, version, exactVersion, "installing")
	if err != nil {
		return err
	}

	go s.installRuntime(context.Background(), id, name, version, exactVersion, miseTool)
	return nil
}

// installRuntime performs the actual installation
func (s *Service) installRuntime(ctx context.Context, id int64, name, version, exactVersion, miseTool string) {
	defer s.installLocks.Delete(name + "@" + exactVersion)

	// PHP / Python 是源码编译，必须先把 autoconf / libxml2-dev 等系统级
	// 编译依赖装上，否则 mise install 会卡在 buildconf 阶段并报错。
	// node / go / java 走预编译二进制，ensureBuildDeps 对它们是 no-op。
	if err := s.ensureBuildDeps(ctx, id, name); err != nil {
		log.Printf("runtime: failed to ensure build deps for %s: %v", name, err)
		s.appendProgress(ctx, id, 25, "deps-failed", fmt.Sprintf("✗ 安装编译依赖失败：%v", err))
		s.repo.UpdateStatusToFailed(ctx, id, "编译依赖安装失败，详见日志")
		return
	}

	target := fmt.Sprintf("%s@%s", miseTool, exactVersion)
	_, exitCode, err := s.runStreaming(ctx, id, 30, "installing", fmt.Sprintf("正在安装 %s...", target), "/usr/local/bin/mise", "install", "-y", target)

	if err != nil || exitCode != 0 {
		log.Printf("runtime: failed to install %s %s: %v", name, exactVersion, err)
		s.repo.UpdateStatusToFailed(ctx, id, "安装失败，详见日志")
		return
	}

	s.appendProgress(ctx, id, 100, "done", "安装完成")
	s.repo.UpdateStatusToInstalled(ctx, id, "")

	hasDefault, _ := s.repo.HasDefault(ctx, name)
	if !hasDefault {
		// First version installed for this lang → auto-promote to default. Must
		// go through applyDefault so /etc/mise/config.toml is regenerated; see
		// Issue 07.
		if err := s.applyDefault(ctx, name, exactVersion); err != nil {
			log.Printf("runtime: auto-default after install of %s@%s failed: %v", name, exactVersion, err)
		}
	}
	log.Printf("runtime: installed %s %s", name, exactVersion)
}

// runStreaming runs a command and streams its output to the database.
// Prior logs in the row are captured up-front and prepended to every write, so
// multi-stage installers (e.g. apt → yum fallback, nvm install → node install)
// don't lose earlier command output. Assumes a single writer per id (one install
// goroutine), which the Install entry point guarantees.
func (s *Service) runStreaming(ctx context.Context, id int64, progress int, step, initialMsg, name string, args ...string) (string, int, error) {
	var prefix string
	if _, _, cur, _, err := s.repo.GetProgress(ctx, id); err != nil {
		log.Printf("runtime: runStreaming failed to read prior logs for id=%d: %v", id, err)
	} else if cur != "" {
		// Bound prefix growth: keep the tail (UTF-8 aware) when it gets too long,
		// matching outputBuf's truncation policy so total DB logs stay roughly ≤ 1MB.
		if len(cur) > 500000 {
			targetStart := len(cur) - 400000
			idx := strings.IndexByte(cur[targetStart:], '\n')
			if idx >= 0 {
				idx++ // skip past the newline so the tail doesn't start with a blank line
			} else {
				// no newline found — advance to the next valid UTF-8 boundary
				idx = 0
				for idx < len(cur)-targetStart && !utf8.RuneStart(cur[targetStart+idx]) {
					idx++
				}
			}
			cur = "..." + cur[targetStart+idx:]
		}
		prefix = cur + "\n"
	}

	s.updateProgress(ctx, id, progress, step, prefix+initialMsg)

	cmd := s.executor.Command(ctx, executor.StartOptions{}, name, args...)
	// DEBIAN_FRONTEND=noninteractive 防止 ensureBuildDeps 里的 apt-get install
	// 撞上 tzdata 这类会忽略 -y 的交互式 postinst 直接挂住。对 mise/npm 无副作用。
	cmd.Env = append(os.Environ(),
		"MISE_DATA_DIR=/var/lib/easyserver/mise",
		"MISE_YES=1",
		"DEBIAN_FRONTEND=noninteractive",
	)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", -1, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return "", -1, err
	}

	if err := cmd.Start(); err != nil {
		return "", -1, err
	}

	var outputBuf bytes.Buffer
	var mu sync.Mutex
	var wg sync.WaitGroup
	var changed bool

	writeFn := func(r io.Reader) {
		defer wg.Done()
		buf := make([]byte, 32*1024)
		for {
			n, err := r.Read(buf)
			if n > 0 {
				mu.Lock()
				outputBuf.Write(buf[:n])
				changed = true
				// truncate buffer to avoid OOM, leave roughly 100KB headroom
				if outputBuf.Len() > 500000 {
					b := outputBuf.Bytes()
					targetStart := len(b) - 400000
					// Find the first newline after targetStart to avoid breaking UTF-8 chars
					idx := bytes.IndexByte(b[targetStart:], '\n')
					if idx == -1 {
						idx = 0 // if no newline, find first valid UTF-8 boundary
						for idx < len(b)-targetStart && !utf8.RuneStart(b[targetStart+idx]) {
							idx++
						}
					}

					prefix := []byte("...")
					remain := b[targetStart+idx:]
					remainLen := len(remain)

					// Use copy to avoid allocation
					copy(b[len(prefix):], remain)
					copy(b[:len(prefix)], prefix)
					outputBuf.Truncate(len(prefix) + remainLen)
				}
				mu.Unlock()
			}
			if err != nil {
				break
			}
		}
	}

	wg.Add(2)
	go writeFn(stdout)
	go writeFn(stderr)

	errChan := make(chan error, 1)
	go func() {
		wg.Wait()
		errChan <- cmd.Wait()
	}()

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			mu.Lock()
			hasChanged := changed
			changed = false
			var currentLog string
			if hasChanged {
				currentLog = outputBuf.String()
			}
			mu.Unlock()
			if hasChanged && currentLog != "" {
				s.updateProgress(ctx, id, progress, step, prefix+initialMsg+"\n"+currentLog)
			}
		case err := <-errChan:
			mu.Lock()
			finalOutput := outputBuf.String()
			mu.Unlock()
			if finalOutput != "" {
				s.updateProgress(ctx, id, progress, step, prefix+initialMsg+"\n"+finalOutput)
			}
			exitCode := 0
			if err != nil {
				if exitErr, ok := err.(*exec.ExitError); ok {
					exitCode = exitErr.ExitCode()
				} else {
					exitCode = -1
				}
			}
			return finalOutput, exitCode, err
		}
	}
}

// updateProgress updates the installation progress
func (s *Service) updateProgress(ctx context.Context, id int64, progress int, step, logs string) {
	// Sanitize logs to remove sensitive information
	sanitizedLogs := sanitizeLogs(logs)
	s.repo.UpdateProgress(ctx, id, progress, step, sanitizedLogs)
}

// appendProgress updates progress/step and appends a line to the existing logs
// instead of overwriting them, so the install command's full output is preserved.
// Caller must guarantee no concurrent writer on the same id (runStreaming has returned).
func (s *Service) appendProgress(ctx context.Context, id int64, progress int, step, line string) {
	_, _, cur, _, err := s.repo.GetProgress(ctx, id)
	if err != nil {
		// Reading logs failed — don't blow away whatever is there. The status update
		// that follows will still mark the runtime as installed; the user just won't
		// see the final "安装完成" line.
		log.Printf("runtime: appendProgress failed to read current logs for id=%d: %v", id, err)
		return
	}
	if cur == "" {
		s.updateProgress(ctx, id, progress, step, line)
		return
	}
	s.updateProgress(ctx, id, progress, step, cur+"\n"+line)
}

// sensitivePatterns matches lines that look like actual secrets, not just any word match
var sensitivePatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)password\s*[:=]\s*\S`),
	regexp.MustCompile(`(?i)secret\s*[:=]\s*\S`),
	regexp.MustCompile(`(?i)api[_-]?key\s*[:=]\s*\S`),
	regexp.MustCompile(`(?i)access[_-]?token\s*[:=]\s*\S`),
	regexp.MustCompile(`(?i)credential\s*[:=]\s*\S`),
}

// sanitizeLogs removes sensitive information from logs
func sanitizeLogs(logs string) string {
	lines := strings.Split(logs, "\n")
	var sanitized []string
	for _, line := range lines {
		isSensitive := false
		for _, pat := range sensitivePatterns {
			if pat.MatchString(line) {
				isSensitive = true
				break
			}
		}
		if !isSensitive {
			sanitized = append(sanitized, line)
		}
	}
	return strings.Join(sanitized, "\n")
}

// isValidVersion validates version string to prevent command injection
// Only allows numbers, letters, dots, hyphens, plus, and underscores (e.g., 17.0.19, 20.10.0, 1.21.5-beta, 21.0.1+12-LTS, temurin-21.0.1)
func isValidVersion(version string) bool {
	if len(version) == 0 || len(version) > 50 {
		return false
	}
	for _, c := range version {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c == '.' || c == '-' || c == '+' || c == '_') {
			return false
		}
	}
	// Must start with a digit or a letter
	first := version[0]
	if !((first >= '0' && first <= '9') || (first >= 'a' && first <= 'z') || (first >= 'A' && first <= 'Z')) {
		return false
	}
	return true
}

// GetProgress returns the installation progress for a runtime environment
func (s *Service) GetProgress(ctx context.Context, id int64) (int, string, string, string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	return s.repo.GetProgress(ctx, id)
}

// Uninstall uninstalls a runtime environment
func (s *Service) Uninstall(ctx context.Context, name, version string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	env, err := s.repo.GetByNameAndVersion(ctx, name, version)
	if err != nil {
		return err
	}
	if env == nil {
		return fmt.Errorf("%s %s not found", name, version)
	}

	if env.Status == "installing" || env.Status == "uninstalling" {
		return fmt.Errorf("operation in progress: currently %s", env.Status)
	}

	// 默认版本也允许卸载：cleanupRelatedData 会先清掉 global_default 行
	// （否则 Delete 会触发 FK 冲突），随后 GenerateMiseConfig 会把 [tools]
	// 里对应条目移除。该 lang 卸载后变为"无默认"，由用户重新指定。

	conflicts, err := s.repo.GetConflictingReferences(ctx, env.ID)
	if err != nil {
		return fmt.Errorf("failed to check conflicts: %w", err)
	}

	if len(conflicts) > 0 {
		return fmt.Errorf("conflict: %s", strings.Join(conflicts, ", "))
	}

	if env.Status == "failed" {
		s.cleanupRelatedData(ctx, env.ID)
		if err := s.repo.Delete(ctx, env.ID); err != nil {
			return err
		}
		// Removing a failed default (possible — line 502 lets failed slip past
		// the IsDefault block) leaves a stale [tools] entry; regenerate.
		if err := s.GenerateMiseConfig(ctx); err != nil {
			log.Printf("runtime: failed to regen mise config after uninstall of failed %s@%s: %v", name, version, err)
		}
		return nil
	}

	if err := s.repo.UpdateStatus(ctx, env.ID, "uninstalling"); err != nil {
		return fmt.Errorf("failed to update status: %w", err)
	}

	s.cleanupRelatedData(ctx, env.ID)

	go func() {
		bgCtx := context.Background()
		uninstallErr := s.uninstallRuntime(bgCtx, env)
		if uninstallErr != nil {
			log.Printf("runtime: failed to uninstall %s %s: %v", env.Name, env.Version, uninstallErr)
			s.repo.UpdateStatusToUninstallFailed(bgCtx, env.ID, uninstallErr.Error())
			// cleanupRelatedData (called synchronously before this goroutine)
			// already dropped any global_default row pinning this version. The
			// runtime_version row sticks around in uninstall_failed status, but
			// the mise config must reflect the new DB state so SSH users no
			// longer resolve to a binary mise is about to remove.
			if err := s.GenerateMiseConfig(bgCtx); err != nil {
				log.Printf("runtime: failed to regen mise config after failed uninstall of %s@%s: %v", env.Name, env.Version, err)
			}
		} else {
			s.repo.Delete(bgCtx, env.ID)
			// Mirror the mise config to the new DB state — the just-removed
			// runtime may have been the global default for its lang.
			if err := s.GenerateMiseConfig(bgCtx); err != nil {
				log.Printf("runtime: failed to regen mise config after uninstall of %s@%s: %v", env.Name, env.Version, err)
			}
		}
	}()

	return nil
}

// cleanupRelatedData cleans up env vars, PATH entries, AND any global_default
// row pinning this runtime_version. The global_default cleanup is required so
// Delete(runtime_version) won't trip the FK constraint; the caller is expected
// to GenerateMiseConfig afterwards so the on-disk [tools] section reflects the
// removal (see Uninstall / uninstallRuntime).
func (s *Service) cleanupRelatedData(ctx context.Context, runtimeID int64) {
	// Delete environment variables
	rows, err := s.repo.CleanupEnvConfigs(ctx, runtimeID)
	if err != nil {
		log.Printf("runtime: failed to cleanup env configs: %v", err)
	} else if rows > 0 {
		log.Printf("runtime: cleaned up %d environment variables", rows)
	}

	// Delete PATH entries
	rows, err = s.repo.CleanupPathEntries(ctx, runtimeID)
	if err != nil {
		log.Printf("runtime: failed to cleanup path entries: %v", err)
	} else if rows > 0 {
		log.Printf("runtime: cleaned up %d PATH entries", rows)
	}

	// Drop any global_default row pinning this runtime so the upcoming Delete
	// won't violate the FK and the next GenerateMiseConfig drops the stale
	// [tools] entry.
	rows, err = s.repo.CleanupGlobalDefaultsByRuntimeID(ctx, runtimeID)
	if err != nil {
		log.Printf("runtime: failed to cleanup global_default: %v", err)
	} else if rows > 0 {
		log.Printf("runtime: cleared %d global_default row(s) for runtime %d", rows, runtimeID)
	}
}

// uninstallRuntime performs the actual uninstallation
func (s *Service) uninstallRuntime(ctx context.Context, env *RuntimeEnvironment) error {
	s.updateProgress(ctx, env.ID, 0, "uninstalling", "")

	var miseTool string
	for _, r := range GetCatalog() {
		if r.Lang == env.Name {
			miseTool = r.MiseTool
			break
		}
	}
	if miseTool == "" {
		return fmt.Errorf("unsupported runtime: %s", env.Name)
	}

	target := fmt.Sprintf("%s@%s", miseTool, env.Version)
	_, exitCode, err := s.runStreaming(ctx, env.ID, 30, "uninstalling", fmt.Sprintf("正在卸载 %s...", target), "/usr/local/bin/mise", "uninstall", "-y", target)

	if err != nil || exitCode != 0 {
		return fmt.Errorf("卸载失败 (exit %d)，详见日志", exitCode)
	}

	log.Printf("runtime: uninstalled %s %s", env.Name, env.Version)
	return nil
}

// GetRemoteVersions dynamically fetches available versions using mise ls-remote
func (s *Service) GetRemoteVersions(ctx context.Context, lang string) ([]string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	var miseTool string
	for _, r := range GetCatalog() {
		if r.Lang == lang {
			miseTool = r.MiseTool
			break
		}
	}
	if miseTool == "" {
		return nil, fmt.Errorf("unsupported runtime: %s", lang)
	}

	cmd := s.executor.Command(ctx, executor.StartOptions{}, "/usr/local/bin/mise", "ls-remote", miseTool)
	cmd.Env = append(os.Environ(), "MISE_DATA_DIR=/var/lib/easyserver/mise", "MISE_YES=1")
	out, err := cmd.Output()
	if err != nil {
		var stderr string
		if exitErr, ok := err.(*exec.ExitError); ok {
			stderr = string(exitErr.Stderr)
		}
		return nil, fmt.Errorf("failed to fetch remote versions: %v, stderr: %s", err, stderr)
	}

	lines := strings.Split(string(out), "\n")
	var versions []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" && isValidVersion(line) {
			versions = append(versions, line)
		}
	}

	// Reverse to put newest first
	for i, j := 0, len(versions)-1; i < j; i, j = i+1, j-1 {
		versions[i], versions[j] = versions[j], versions[i]
	}

	return versions, nil
}

// GetEnvConfigsByRuntimeID returns environment configs for a runtime
func (s *Service) GetEnvConfigsByRuntimeID(ctx context.Context, runtimeID int64) ([]envconfig.EnvConfig, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	return s.repo.ListEnvConfigsByRuntimeID(ctx, runtimeID)
}

// GetPathEntriesByRuntimeID returns PATH entries for a runtime
func (s *Service) GetPathEntriesByRuntimeID(ctx context.Context, runtimeID int64) ([]envconfig.PathEntry, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	return s.repo.ListPathEntriesByRuntimeID(ctx, runtimeID)
}

// isValidUninstallPath checks if the path is safe for deletion
// Only allows paths under /home, /opt, or /usr/local that are not system-critical
func isValidUninstallPath(path string) bool {
	// Reject empty or root path
	if path == "" || path == "/" {
		return false
	}

	// Reject system-critical paths
	systemPaths := []string{
		"/bin", "/sbin", "/usr", "/etc", "/var", "/tmp", "/dev", "/proc", "/sys",
	}
	for _, sp := range systemPaths {
		if path == sp || strings.HasPrefix(path, sp+"/") {
			return false
		}
	}

	// Only allow paths under /home or /opt
	allowedPrefixes := []string{"/home/", "/opt/"}
	for _, prefix := range allowedPrefixes {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}

	return false
}

// SetDefault sets a version as the default for a runtime environment
func (s *Service) SetDefault(ctx context.Context, name, version string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	// Validate version to prevent command injection
	if !isValidVersion(version) {
		return fmt.Errorf("invalid version format: %s", version)
	}

	// Check if the version exists
	env, err := s.repo.GetByNameAndVersion(ctx, name, version)
	if err != nil {
		return err
	}
	if env == nil {
		return fmt.Errorf("%s %s not found", name, version)
	}
	// Refuse to promote a not-ready version to default: writing such a row to
	// /etc/mise/config.toml [tools] would point SSH users at a binary that
	// isn't on disk, making the whole runtime unusable for them.
	if env.Status != "installed" {
		return fmt.Errorf("cannot set %s %s as default: status is %q (must be installed)", name, version, env.Status)
	}

	return s.applyDefault(ctx, name, version)
}

// applyDefault marks (name, version) as the global default for that lang AND
// regenerates /etc/mise/config.toml so SSH users / `mise current` immediately
// see the change. Both effects are required for the API to be truthful — see
// Issue 07. Three call sites (SetDefault, installRuntime, ImportDetected) all
// route through here; do NOT bypass to the repo helpers directly.
func (s *Service) applyDefault(ctx context.Context, name, version string) error {
	if err := s.repo.ResetDefaults(ctx, name); err != nil {
		return err
	}
	if err := s.repo.SetDefaultByNameAndVersion(ctx, name, version); err != nil {
		return err
	}
	if err := s.GenerateMiseConfig(ctx); err != nil {
		log.Printf("runtime: failed to regenerate mise config after default %s=%s: %v", name, version, err)
		return fmt.Errorf("default set in DB but mise config regeneration failed: %w", err)
	}
	return nil
}

// shellEscape escapes a string for safe use in shell commands.
func shellEscape(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

// ListMirrors returns all mirrors
func (s *Service) ListMirrors(ctx context.Context) ([]RuntimeMirror, error) {
	return s.repo.ListMirrors(ctx)
}

// UpdateMirror updates a mirror
func (s *Service) UpdateMirror(ctx context.Context, req *RuntimeMirrorUpdateRequest, id int64) error {
	m, err := s.repo.GetMirror(ctx, id)
	if err != nil {
		return err
	}
	if m == nil {
		return fmt.Errorf("mirror not found")
	}

	newEnvValue := m.EnvValue
	if req.EnvValue != nil {
		val := strings.TrimRight(*req.EnvValue, "/")
		if m.Source == "seed" && val != strings.TrimRight(m.EnvValue, "/") {
			return fmt.Errorf("cannot modify seed mirror URL")
		}
		newEnvValue = val
	}
	newEnabled := m.Enabled
	if req.Enabled != nil {
		newEnabled = *req.Enabled
	}

	// If enabling, disable others with same EnvKey
	if newEnabled == 1 {
		err := s.repo.DisableOtherMirrors(ctx, m.EnvKey, id)
		if err != nil {
			return err
		}
	}

	err = s.repo.UpdateMirror(ctx, id, newEnvValue, newEnabled)
	if err != nil {
		return err
	}

	return s.GenerateMiseConfig(ctx)
}

// CreateMirror creates a user mirror
func (s *Service) CreateMirror(ctx context.Context, req *RuntimeMirrorCreateRequest) (int64, error) {
	// env_key flows verbatim into /etc/mise/config.toml. A value containing a
	// newline or '[' would let an admin inject a fake [tools] section and
	// thereby pin SSH users to an attacker-chosen version. POSIX env-var
	// naming rules are stricter than what TOML needs, but they're a clear
	// and well-known whitelist.
	if !envKeyPattern.MatchString(req.EnvKey) {
		return 0, fmt.Errorf("invalid env_key %q: must match %s", req.EnvKey, envKeyPattern.String())
	}

	enabled := 1
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	m := &RuntimeMirror{
		Lang:     req.Lang,
		EnvKey:   req.EnvKey,
		EnvValue: strings.TrimRight(req.EnvValue, "/"),
		Enabled:  enabled,
		Source:   "user",
	}
	id, err := s.repo.CreateMirror(ctx, m)
	if err != nil {
		return 0, err
	}

	// If enabling, disable others
	if enabled == 1 {
		err := s.repo.DisableOtherMirrors(ctx, m.EnvKey, id)
		if err != nil {
			return 0, err
		}
	}

	s.GenerateMiseConfig(ctx)
	return id, nil
}

// DeleteMirror deletes a mirror
func (s *Service) DeleteMirror(ctx context.Context, id int64) error {
	m, err := s.repo.GetMirror(ctx, id)
	if err != nil {
		return err
	}
	if m == nil {
		return fmt.Errorf("mirror not found")
	}
	if m.Source == "seed" {
		return fmt.Errorf("cannot delete seed mirror")
	}

	err = s.repo.DeleteMirror(ctx, id)
	if err != nil {
		return err
	}

	return s.GenerateMiseConfig(ctx)
}

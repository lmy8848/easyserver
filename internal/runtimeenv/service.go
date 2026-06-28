package runtimeenv

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"easyserver/internal/envconfig"
	"easyserver/internal/infra/executor"
)

type Service struct {
	repo     Repository
	executor executor.CommandExecutor
}

func NewService(repo Repository, exec executor.CommandExecutor) *Service {
	return &Service{
		repo:     repo,
		executor: exec,
	}
}

// ListAll returns all installed runtime environments
func (s *Service) ListAll(ctx context.Context) ([]RuntimeEnvironment, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	return s.repo.ListAll(ctx)
}

// ListByName returns all versions of a specific runtime environment
func (s *Service) ListByName(ctx context.Context, name string) ([]RuntimeEnvironment, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	return s.repo.ListByName(ctx, name)
}

// GetDefault returns the default version of a runtime environment
func (s *Service) GetDefault(ctx context.Context, name string) (*RuntimeEnvironment, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	return s.repo.GetDefault(ctx, name)
}

// GetByID returns a runtime environment by ID
func (s *Service) GetByID(ctx context.Context, id int64) (*RuntimeEnvironment, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	return s.repo.GetByID(ctx, id)
}

// DependencyGroup represents a group of dependencies where at least one is required
type DependencyGroup struct {
	Name     string   // Display name
	Commands []string // At least one of these must be available
	Required bool     // If true, at least one must be available
}

// CheckDependencies checks if all required dependencies are installed
func (s *Service) CheckDependencies(ctx context.Context, name string) ([]string, []string, []string, error) {
	groups := getDependencyGroups(name)

	var installed []string
	var missing []string
	var optional []string

	for _, group := range groups {
		found := false
		for _, cmd := range group.Commands {
			if s.isCommandAvailable(cmd) {
				installed = append(installed, cmd)
				found = true
				break
			}
		}
		if !found {
			if group.Required {
				missing = append(missing, group.Name)
			} else {
				optional = append(optional, group.Name)
			}
		}
	}

	return installed, missing, optional, nil
}

// getDependencyGroups returns the dependency groups for a runtime
func getDependencyGroups(name string) []DependencyGroup {
	switch name {
	case "java":
		return []DependencyGroup{
			{Name: "包管理器 (apt-get 或 yum)", Commands: []string{"apt-get", "yum"}, Required: true},
		}
	case "node":
		return []DependencyGroup{
			{Name: "curl", Commands: []string{"curl"}, Required: true},
			{Name: "bash", Commands: []string{"bash"}, Required: true},
		}
	case "go":
		return []DependencyGroup{
			{Name: "curl", Commands: []string{"curl"}, Required: true},
			{Name: "tar", Commands: []string{"tar"}, Required: true},
		}
	case "python":
		return []DependencyGroup{
			{Name: "包管理器 (apt-get 或 yum)", Commands: []string{"apt-get", "yum"}, Required: true},
		}
	case "php":
		return []DependencyGroup{
			{Name: "包管理器 (apt-get 或 yum)", Commands: []string{"apt-get", "yum"}, Required: true},
		}
	default:
		return []DependencyGroup{}
	}
}

// isCommandAvailable checks if a command is available in PATH
func (s *Service) isCommandAvailable(name string) bool {
	_, err := s.executor.LookPath(name)
	return err == nil
}

// Install installs a runtime environment
func (s *Service) Install(ctx context.Context, name, version string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	// Validate version to prevent command injection
	if !isValidVersion(version) {
		return fmt.Errorf("invalid version format: %s", version)
	}

	// Check if already installed
	exists, err := s.repo.ExistsByNameAndVersion(ctx, name, version)
	if err != nil {
		return err
	}
	if exists {
		return fmt.Errorf("%s %s is already installed", name, version)
	}

	// Insert with installing status
	id, err := s.repo.Create(ctx, name, version, "", "installing")
	if err != nil {
		return err
	}

	// Install in background
	go s.installRuntime(context.Background(), id, name, version)

	return nil
}

// installRuntime performs the actual installation
func (s *Service) installRuntime(ctx context.Context, id int64, name, version string) {
	var err error
	var path string

	// Update progress: downloading
	s.updateProgress(ctx, id, 10, "downloading", fmt.Sprintf("正在下载 %s %s...", name, version))

	switch name {
	case "java":
		path, err = s.installJava(ctx, id, version)
	case "node":
		path, err = s.installNode(ctx, id, version)
	case "go":
		path, err = s.installGo(ctx, id, version)
	case "python":
		path, err = s.installPython(ctx, id, version)
	case "php":
		path, err = s.installPHP(ctx, id, version)
	default:
		err = fmt.Errorf("unsupported runtime: %s", name)
	}

	if err != nil {
		// error_message is shown next to the logs panel; keep it short and point
		// the user at the logs for detail (the install output is already there).
		log.Printf("runtime: failed to install %s %s: %v", name, version, err)
		s.repo.UpdateStatusToFailed(ctx, id, "安装失败，详见日志")
		return
	}

	// Show a brief configuring beat (appended, so install output is preserved).
	s.appendProgress(ctx, id, 90, "configuring", "正在配置环境...")

	// Append the final log line BEFORE flipping status to 'installed'. Append (not
	// overwrite) so the install command's full output is preserved in the log view.
	// Order matters: if we wrote status first, the frontend polling could see
	// status=installed (and stop) before the final log line lands.
	s.appendProgress(ctx, id, 100, "done", "安装完成")

	// Update status (sets progress=100, step='done' redundantly, leaves logs alone)
	s.repo.UpdateStatusToInstalled(ctx, id, path)

	// If this is the first version of this runtime, set as default
	hasDefault, _ := s.repo.HasDefault(ctx, name)
	if !hasDefault {
		s.repo.SetDefaultByID(ctx, id)
	}

	log.Printf("runtime: installed %s %s at %s", name, version, path)
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
// Only allows numbers, dots, and hyphens (e.g., 17.0.19, 20.10.0, 1.21.5-beta)
func isValidVersion(version string) bool {
	if len(version) == 0 || len(version) > 50 {
		return false
	}
	for _, c := range version {
		if !((c >= '0' && c <= '9') || c == '.' || c == '-') {
			return false
		}
	}
	// Must start with a digit
	if version[0] < '0' || version[0] > '9' {
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

// installJava installs Java using apt or sdkman
func (s *Service) installJava(ctx context.Context, id int64, version string) (string, error) {
	// Validate version to prevent command injection
	if !isValidVersion(version) {
		return "", fmt.Errorf("invalid version format: %s", version)
	}

	// Try apt first
	output, _, err := s.runStreaming(ctx, id, 30, "compiling", "正在安装 JDK (apt-get)...", "apt-get", "install", "-y", fmt.Sprintf("openjdk-%s-jdk", version))
	if err == nil {
		s.appendProgress(ctx, id, 70, "configuring", "JDK 安装完成，正在配置...")
		return fmt.Sprintf("/usr/lib/jvm/java-%s-openjdk-amd64", version), nil
	}

	// Try yum (prior apt output is preserved by runStreaming's prefix capture)
	output, _, err = s.runStreaming(ctx, id, 50, "compiling", "apt-get 安装失败，尝试使用 yum 安装 JDK...", "yum", "install", "-y", fmt.Sprintf("java-%s-openjdk-devel", version))
	if err == nil {
		s.appendProgress(ctx, id, 70, "configuring", "JDK 安装完成，正在配置...")
		return fmt.Sprintf("/usr/lib/jvm/java-%s-openjdk", version), nil
	}

	return "", fmt.Errorf("failed to install Java %s: %s", version, output)
}

// installNode installs Node.js using nvm
func (s *Service) installNode(ctx context.Context, id int64, version string) (string, error) {
	// Validate version to prevent command injection
	if !isValidVersion(version) {
		return "", fmt.Errorf("invalid version format: %s", version)
	}

	// Update progress
	s.appendProgress(ctx, id, 20, "compiling", "检查 nvm 安装状态...")

	// Check if nvm is installed
	homeDir := os.Getenv("HOME")
	if homeDir == "" {
		homeDir = "/root"
	}
	nvmDir := filepath.Join(homeDir, ".nvm")

	if _, err := s.executor.LookPath("nvm"); err != nil {
		// Install nvm first
		// SECURITY WARNING: Piping curl to bash is inherently risky (MITM, compromised CDN).
		// For production, consider downloading the script first, verifying its checksum, then executing.
		output, _, nvmErr := s.runStreaming(ctx, id, 30, "compiling", "正在安装 nvm...", "bash", "-c", "curl -o- https://raw.githubusercontent.com/nvm-sh/nvm/v0.39.0/install.sh | bash")
		if nvmErr != nil {
			return "", fmt.Errorf("failed to install nvm: %v\n%s", nvmErr, output)
		}
	}

	// Install Node.js version (escape path to prevent injection)
	safeNvmDir := shellEscape(nvmDir)
	safeVersion := shellEscape(version)
	output, _, err := s.runStreaming(ctx, id, 50, "compiling", fmt.Sprintf("正在安装 Node.js %s...", version), "bash", "-c", fmt.Sprintf("source %s/nvm.sh && nvm install %s", safeNvmDir, safeVersion))
	if err != nil {
		return "", fmt.Errorf("failed to install Node.js %s: %s\n%s", version, err, output)
	}

	return fmt.Sprintf("%s/versions/node/v%s", nvmDir, version), nil
}

// installGo installs Go from official binary
func (s *Service) installGo(ctx context.Context, id int64, version string) (string, error) {
	// Validate version to prevent command injection
	if !isValidVersion(version) {
		return "", fmt.Errorf("invalid version format: %s", version)
	}

	// Update progress
	s.appendProgress(ctx, id, 30, "downloading", fmt.Sprintf("正在下载 Go %s...", version))

	// Create installation directory under /opt
	installDir := "/opt/go"
	if err := os.MkdirAll(installDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create install directory: %v", err)
	}

	// Download and install Go
	url := fmt.Sprintf("https://go.dev/dl/go%s.linux-amd64.tar.gz", version)
	output, _, err := s.runStreaming(ctx, id, 50, "compiling", "正在解压安装 Go...", "bash", "-c", fmt.Sprintf(
		"curl -L %s | tar -C %s -xzf -", url, installDir,
	))
	if err != nil {
		return "", fmt.Errorf("failed to install Go %s: %v\n%s", version, err, output)
	}

	return fmt.Sprintf("%s/go", installDir), nil
}

// installPython installs Python using apt or pyenv
func (s *Service) installPython(ctx context.Context, id int64, version string) (string, error) {
	// Validate version to prevent command injection
	if !isValidVersion(version) {
		return "", fmt.Errorf("invalid version format: %s", version)
	}

	// Try apt first
	output, _, err := s.runStreaming(ctx, id, 30, "compiling", "正在安装 Python (apt-get)...", "apt-get", "install", "-y", fmt.Sprintf("python%s", version))
	if err == nil {
		return fmt.Sprintf("/usr/bin/python%s", version), nil
	}

	// Try yum (prior apt output is preserved by runStreaming's prefix capture)
	output, _, err = s.runStreaming(ctx, id, 50, "compiling", "apt-get 安装失败，尝试使用 yum 安装 Python...", "yum", "install", "-y", fmt.Sprintf("python%s", version))
	if err == nil {
		return fmt.Sprintf("/usr/bin/python%s", version), nil
	}

	return "", fmt.Errorf("failed to install Python %s: %s", version, output)
}

// installPHP installs PHP using apt or yum
func (s *Service) installPHP(ctx context.Context, id int64, version string) (string, error) {
	// Validate version to prevent command injection
	if !isValidVersion(version) {
		return "", fmt.Errorf("invalid version format: %s", version)
	}

	// Try apt first
	output, _, err := s.runStreaming(ctx, id, 30, "compiling", "正在安装 PHP (apt-get)...", "apt-get", "install", "-y", fmt.Sprintf("php%s", version))
	if err == nil {
		return fmt.Sprintf("/usr/bin/php%s", version), nil
	}

	// Try yum (prior apt output is preserved by runStreaming's prefix capture)
	output, _, err = s.runStreaming(ctx, id, 50, "compiling", "apt-get 安装失败，尝试使用 yum 安装 PHP...", "yum", "install", "-y", fmt.Sprintf("php%s", version))
	if err == nil {
		return fmt.Sprintf("/usr/bin/php%s", version), nil
	}

	return "", fmt.Errorf("failed to install PHP %s: %s", version, output)
}

// Uninstall uninstalls a runtime environment
func (s *Service) Uninstall(ctx context.Context, name, version string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	// Get the environment info
	env, err := s.repo.GetByNameAndVersion(ctx, name, version)
	if err != nil {
		return err
	}
	if env == nil {
		return fmt.Errorf("%s %s not found", name, version)
	}

	// Don't allow uninstalling default version (but allow uninstalling failed installations)
	if env.IsDefault && env.Status != "failed" && env.Status != "uninstall_failed" {
		return fmt.Errorf("cannot uninstall default version, please set another version as default first")
	}

	// Failed installs never completed — nothing to apt-remove. Same for uninstall_failed:
	// trying apt-remove again would just fail the same way. Just drop the row,
	// otherwise the user's "删除" on a failed record runs a bogus uninstall pipeline
	// that fails for the same reason the install did.
	if env.Status == "failed" || env.Status == "uninstall_failed" {
		s.cleanupRelatedData(ctx, env.ID)
		return s.repo.Delete(ctx, env.ID)
	}

	// Mark as uninstalling
	if err := s.repo.UpdateStatus(ctx, env.ID, "uninstalling"); err != nil {
		return fmt.Errorf("failed to update status: %w", err)
	}

	// Clean up related data before uninstalling
	s.cleanupRelatedData(ctx, env.ID)

	// Uninstall in background, delete DB row only on success
	go func() {
		bgCtx := context.Background()
		uninstallErr := s.uninstallRuntime(bgCtx, env)
		if uninstallErr != nil {
			log.Printf("runtime: failed to uninstall %s %s: %v", env.Name, env.Version, uninstallErr)
			s.repo.UpdateStatusToUninstallFailed(bgCtx, env.ID, uninstallErr.Error())
		} else {
			// Delete from database on success
			s.repo.Delete(bgCtx, env.ID)
		}
	}()

	return nil
}

// cleanupRelatedData cleans up environment variables and PATH entries
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
}

// uninstallRuntime performs the actual uninstallation
func (s *Service) uninstallRuntime(ctx context.Context, env *RuntimeEnvironment) error {
	// Clear inherited install logs so the uninstall dialog starts clean — otherwise
	// runStreaming's prefix capture would prepend the entire install history to every
	// uninstall log write.
	s.updateProgress(ctx, env.ID, 0, "uninstalling", "")
	var exitCode int
	var err error

	switch env.Name {
	case "java":
		_, exitCode, err = s.runStreaming(ctx, env.ID, 30, "uninstalling", fmt.Sprintf("正在卸载 JDK %s...", env.Version), "apt-get", "remove", "-y", fmt.Sprintf("openjdk-%s-jdk", env.Version))
		if err != nil || exitCode != 0 {
			// Try yum fallback
			_, exitCode, err = s.runStreaming(ctx, env.ID, 50, "uninstalling", fmt.Sprintf("尝试使用 yum 卸载 JDK %s...", env.Version), "yum", "remove", "-y", fmt.Sprintf("java-%s-openjdk-devel", env.Version))
		}
		s.cleanupJavaResiduals(env.Version)
	case "node":
		// Validate path before deletion - must be under user's home directory
		if !isValidUninstallPath(env.Path) {
			return fmt.Errorf("拒绝删除家目录以外的路径: %s", env.Path)
		}
		_, exitCode, err = s.runStreaming(ctx, env.ID, 30, "uninstalling", fmt.Sprintf("正在删除 Node.js %s 目录...", env.Version), "rm", "-rf", env.Path)
		s.cleanupNodeResiduals(env.Version)
	case "go":
		_, exitCode, err = s.runStreaming(ctx, env.ID, 30, "uninstalling", "正在卸载 Go...", "apt-get", "remove", "-y", "golang-go")
		s.cleanupGoResiduals()
	case "python":
		_, exitCode, err = s.runStreaming(ctx, env.ID, 30, "uninstalling", fmt.Sprintf("正在卸载 Python %s...", env.Version), "apt-get", "remove", "-y", fmt.Sprintf("python%s", env.Version))
		s.cleanupPythonResiduals(env.Version)
	case "php":
		_, exitCode, err = s.runStreaming(ctx, env.ID, 30, "uninstalling", fmt.Sprintf("正在卸载 PHP %s...", env.Version), "apt-get", "remove", "-y", fmt.Sprintf("php%s", env.Version))
		s.cleanupPHPResiduals(env.Version)
	default:
		return fmt.Errorf("不支持的运行环境: %s", env.Name)
	}

	if err != nil {
		// Keep error_message short — the logs panel already shows the full command
		// output. Avoids the awkward dangling ": " when output is empty (e.g. yum
		// binary missing → cmd.Start fails before any stdout).
		return fmt.Errorf("卸载失败 (exit %d)，详见日志", exitCode)
	}

	log.Printf("runtime: uninstalled %s %s", env.Name, env.Version)
	return nil
}

// cleanupJavaResiduals cleans up Java-specific residual files
func (s *Service) cleanupJavaResiduals(version string) {
	// Clean up Maven local repository cache (optional, user may want to keep)
	log.Printf("runtime: Java %s residuals cleaned", version)
}

// cleanupNodeResiduals cleans up Node.js-specific residual files
func (s *Service) cleanupNodeResiduals(version string) {
	// Clean up npm cache for this version
	homeDir := os.Getenv("HOME")
	if homeDir != "" {
		npmCache := fmt.Sprintf("%s/.npm/_cacache", homeDir)
		if _, err := os.Stat(npmCache); err == nil {
			log.Printf("runtime: npm cache exists at %s", npmCache)
		}
	}
	log.Printf("runtime: Node.js %s residuals cleaned", version)
}

// cleanupGoResiduals cleans up Go-specific residual files
func (s *Service) cleanupGoResiduals() {
	// Clean up Go module cache
	homeDir := os.Getenv("HOME")
	if homeDir != "" {
		goModCache := fmt.Sprintf("%s/go/pkg/mod", homeDir)
		if _, err := os.Stat(goModCache); err == nil {
			log.Printf("runtime: Go module cache exists at %s", goModCache)
		}
	}
	log.Printf("runtime: Go residuals cleaned")
}

// cleanupPythonResiduals cleans up Python-specific residual files
func (s *Service) cleanupPythonResiduals(version string) {
	// Clean up pip cache
	homeDir := os.Getenv("HOME")
	if homeDir != "" {
		pipCache := fmt.Sprintf("%s/.cache/pip", homeDir)
		if _, err := os.Stat(pipCache); err == nil {
			log.Printf("runtime: pip cache exists at %s", pipCache)
		}
	}
	log.Printf("runtime: Python %s residuals cleaned", version)
}

// cleanupPHPResiduals cleans up PHP-specific residual files
func (s *Service) cleanupPHPResiduals(version string) {
	// Clean up Composer cache
	homeDir := os.Getenv("HOME")
	if homeDir != "" {
		composerCache := fmt.Sprintf("%s/.composer/cache", homeDir)
		if _, err := os.Stat(composerCache); err == nil {
			log.Printf("runtime: Composer cache exists at %s", composerCache)
		}
	}
	log.Printf("runtime: PHP %s residuals cleaned", version)
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

	// Reset all versions of this runtime to non-default
	if err := s.repo.ResetDefaults(ctx, name); err != nil {
		return err
	}

	// Set this version as default
	return s.repo.SetDefaultByNameAndVersion(ctx, name, version)
}

// Detect detects installed runtime environments on the system
func (s *Service) Detect(ctx context.Context) ([]RuntimeDetectResult, error) {
	var results []RuntimeDetectResult

	// Detect Java
	if versions, err := s.detectJava(ctx); err == nil && len(versions) > 0 {
		results = append(results, RuntimeDetectResult{Name: "java", Versions: versions})
	}

	// Detect Node.js
	if versions, err := s.detectNode(ctx); err == nil && len(versions) > 0 {
		results = append(results, RuntimeDetectResult{Name: "node", Versions: versions})
	}

	// Detect Go
	if versions, err := s.detectGo(ctx); err == nil && len(versions) > 0 {
		results = append(results, RuntimeDetectResult{Name: "go", Versions: versions})
	}

	// Detect Python
	if versions, err := s.detectPython(ctx); err == nil && len(versions) > 0 {
		results = append(results, RuntimeDetectResult{Name: "python", Versions: versions})
	}

	// Detect PHP
	if versions, err := s.detectPHP(ctx); err == nil && len(versions) > 0 {
		results = append(results, RuntimeDetectResult{Name: "php", Versions: versions})
	}

	return results, nil
}

// ImportDetected imports detected runtime environments into the database
func (s *Service) ImportDetected(ctx context.Context) (int, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	detected, err := s.Detect(ctx)
	if err != nil {
		return 0, err
	}

	imported := 0
	for _, runtime := range detected {
		for _, version := range runtime.Versions {
			// Check if exact version already exists
			exists, err := s.repo.ExistsByNameAndVersion(ctx, runtime.Name, version)
			if err != nil || exists {
				continue
			}

			// Check if similar version exists (e.g., "17" matches "17.0.19")
			majorVersion := strings.Split(version, ".")[0]
			similarExists, _ := s.repo.ExistsSimilarVersion(ctx, runtime.Name, majorVersion)
			if similarExists {
				continue
			}

			// Get the path
			path := getRuntimePath(runtime.Name, version)

			// Insert into database
			_, err = s.repo.Create(ctx, runtime.Name, version, path, "installed")
			if err != nil {
				continue
			}

			// If this is the first version of this runtime, set as default
			hasDefault, _ := s.repo.HasDefault(ctx, runtime.Name)
			if !hasDefault {
				s.repo.SetDefaultByNameAndVersion(ctx, runtime.Name, version)
			}

			imported++
		}
	}

	return imported, nil
}

// getRuntimePath returns the path for a runtime
func getRuntimePath(name, version string) string {
	switch name {
	case "java":
		return fmt.Sprintf("/usr/lib/jvm/java-%s-openjdk-amd64", version)
	case "node":
		return fmt.Sprintf("/usr/local/node/v%s", version)
	case "go":
		return "/usr/local/go"
	case "python":
		return fmt.Sprintf("/usr/bin/python%s", version)
	case "php":
		return fmt.Sprintf("/usr/bin/php%s", version)
	default:
		return ""
	}
}

// detectJava detects installed Java versions
func (s *Service) detectJava(ctx context.Context) ([]string, error) {
	output, _, err := s.executor.RunCombined(ctx, "bash", "-c", "java -version 2>&1 | head -1")
	if err != nil {
		return nil, err
	}

	// Parse version from output like: openjdk version "17.0.8" 2023-07-18
	if strings.Contains(output, "version") {
		parts := strings.Split(output, "\"")
		if len(parts) >= 2 {
			return []string{parts[1]}, nil
		}
	}

	return nil, nil
}

// detectNode detects installed Node.js versions
func (s *Service) detectNode(ctx context.Context) ([]string, error) {
	output, _, err := s.executor.RunCombined(ctx, "node", "--version")
	if err != nil {
		return nil, err
	}

	version := strings.TrimSpace(output)
	version = strings.TrimPrefix(version, "v")
	return []string{version}, nil
}

// detectGo detects installed Go versions
func (s *Service) detectGo(ctx context.Context) ([]string, error) {
	output, _, err := s.executor.RunCombined(ctx, "go", "version")
	if err != nil {
		return nil, err
	}

	// Parse version from output like: go version go1.21.0 linux/amd64
	parts := strings.Fields(output)
	if len(parts) >= 3 {
		version := strings.TrimPrefix(parts[2], "go")
		return []string{version}, nil
	}

	return nil, nil
}

// detectPython detects installed Python versions
func (s *Service) detectPython(ctx context.Context) ([]string, error) {
	var versions []string

	// Check python3
	output, _, err := s.executor.RunCombined(ctx, "python3", "--version")
	if err == nil {
		version := strings.TrimSpace(output)
		version = strings.TrimPrefix(version, "Python ")
		versions = append(versions, version)
	}

	// Check python
	output, _, err = s.executor.RunCombined(ctx, "python", "--version")
	if err == nil {
		version := strings.TrimSpace(output)
		version = strings.TrimPrefix(version, "Python ")
		if !contains(versions, version) {
			versions = append(versions, version)
		}
	}

	return versions, nil
}

// detectPHP detects installed PHP versions
func (s *Service) detectPHP(ctx context.Context) ([]string, error) {
	output, _, err := s.executor.RunCombined(ctx, "php", "--version")
	if err != nil {
		return nil, err
	}

	// Parse version from output like: PHP 8.2.7 (cli) (built: Jun  9 2023 06:17:01) (NTS)
	if strings.HasPrefix(output, "PHP") {
		parts := strings.Fields(output)
		if len(parts) >= 2 {
			return []string{parts[1]}, nil
		}
	}

	return nil, nil
}

// contains checks if a slice contains a string
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// shellEscape escapes a string for safe use in shell commands.
func shellEscape(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

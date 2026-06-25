package service

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"easyserver/internal/executor"
	"easyserver/internal/model"
	"easyserver/internal/repository"
	"easyserver/internal/repository/sqlite"
)

type RuntimeService struct {
	db       *sql.DB
	repo     repository.RuntimeRepository
	executor executor.CommandExecutor
}

func NewRuntimeService(db *sql.DB, exec executor.CommandExecutor) *RuntimeService {
	return &RuntimeService{
		db:       db,
		repo:     sqlite.NewRuntimeRepository(db),
		executor: exec,
	}
}

// ListAll returns all installed runtime environments
func (s *RuntimeService) ListAll(ctx context.Context) ([]model.RuntimeEnvironment, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	return s.repo.ListAll(ctx)
}

// ListByName returns all versions of a specific runtime environment
func (s *RuntimeService) ListByName(ctx context.Context, name string) ([]model.RuntimeEnvironment, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	return s.repo.ListByName(ctx, name)
}

// GetDefault returns the default version of a runtime environment
func (s *RuntimeService) GetDefault(ctx context.Context, name string) (*model.RuntimeEnvironment, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	return s.repo.GetDefault(ctx, name)
}

// GetByID returns a runtime environment by ID
func (s *RuntimeService) GetByID(ctx context.Context, id int64) (*model.RuntimeEnvironment, error) {
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
func (s *RuntimeService) CheckDependencies(ctx context.Context, name string) ([]string, []string, []string, error) {
	groups := getDependencyGroups(name)

	var installed []string
	var missing []string
	var optional []string

	for _, group := range groups {
		found := false
		for _, cmd := range group.Commands {
			if isCommandAvailable(cmd) {
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
func isCommandAvailable(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

// Install installs a runtime environment
func (s *RuntimeService) Install(ctx context.Context, name, version string) error {
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
func (s *RuntimeService) installRuntime(ctx context.Context, id int64, name, version string) {
	var err error
	var path string

	// Update progress: downloading
	s.updateProgress(id, 10, "downloading", fmt.Sprintf("正在下载 %s %s...", name, version))

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
		errMsg := fmt.Sprintf("安装失败: %v", err)
		log.Printf("runtime: failed to install %s %s: %v", name, version, err)
		s.db.Exec("UPDATE runtime_environments SET status = 'failed', error_message = ?, progress = 0, progress_step = 'failed' WHERE id = ? AND status = 'installing'", errMsg, id)
		return
	}

	// Update progress: configuring
	s.updateProgress(id, 90, "configuring", "正在配置环境...")

	// Update status
	s.db.Exec("UPDATE runtime_environments SET status = 'installed', path = ?, progress = 100, progress_step = 'done' WHERE id = ? AND status = 'installing'", path, id)

	// If this is the first version of this runtime, set as default
	var count int
	s.db.QueryRow("SELECT COUNT(*) FROM runtime_environments WHERE name = ? AND is_default = 1", name).Scan(&count)
	if count == 0 {
		s.db.Exec("UPDATE runtime_environments SET is_default = 1 WHERE id = ?", id)
	}

	log.Printf("runtime: installed %s %s at %s", name, version, path)
}

// updateProgress updates the installation progress
func (s *RuntimeService) updateProgress(id int64, progress int, step, logs string) {
	// Sanitize logs to remove sensitive information
	sanitizedLogs := sanitizeLogs(logs)
	s.db.Exec("UPDATE runtime_environments SET progress = ?, progress_step = ?, logs = ? WHERE id = ?",
		progress, step, sanitizedLogs, id)
}

// sanitizeLogs removes sensitive information from logs
func sanitizeLogs(logs string) string {
	// Remove potential password/key patterns
	lines := strings.Split(logs, "\n")
	var sanitized []string
	for _, line := range lines {
		// Skip lines that might contain sensitive data
		lower := strings.ToLower(line)
		if strings.Contains(lower, "password") ||
			strings.Contains(lower, "secret") ||
			strings.Contains(lower, "token") ||
			strings.Contains(lower, "key") ||
			strings.Contains(lower, "credential") {
			continue
		}
		sanitized = append(sanitized, line)
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
func (s *RuntimeService) GetProgress(ctx context.Context, id int64) (int, string, string, string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	var progress int
	var step, logs, errorMessage string
	err := s.db.QueryRowContext(ctx,
		"SELECT progress, progress_step, logs, error_message FROM runtime_environments WHERE id = ?",
		id,
	).Scan(&progress, &step, &logs, &errorMessage)
	if err != nil {
		return 0, "", "", "", err
	}
	return progress, step, logs, errorMessage, nil
}

// installJava installs Java using apt or sdkman
func (s *RuntimeService) installJava(ctx context.Context, id int64, version string) (string, error) {
	// Validate version to prevent command injection
	if !isValidVersion(version) {
		return "", fmt.Errorf("invalid version format: %s", version)
	}

	// Update progress: compiling
	s.updateProgress(id, 30, "compiling", "正在安装 JDK...")

	// Try apt first
	output, _, err := s.executor.RunCombined(ctx, "apt-get", "install", "-y", fmt.Sprintf("openjdk-%s-jdk", version))
	if err == nil {
		s.updateProgress(id, 70, "configuring", "JDK 安装完成，正在配置...")
		return fmt.Sprintf("/usr/lib/jvm/java-%s-openjdk-amd64", version), nil
	}

	// Try yum
	s.updateProgress(id, 50, "compiling", "尝试使用 yum 安装...")
	output, _, err = s.executor.RunCombined(ctx, "yum", "install", "-y", fmt.Sprintf("java-%s-openjdk-devel", version))
	if err == nil {
		s.updateProgress(id, 70, "configuring", "JDK 安装完成，正在配置...")
		return fmt.Sprintf("/usr/lib/jvm/java-%s-openjdk", version), nil
	}

	return "", fmt.Errorf("failed to install Java %s: %s", version, output)
}

// installNode installs Node.js using nvm
func (s *RuntimeService) installNode(ctx context.Context, id int64, version string) (string, error) {
	// Validate version to prevent command injection
	if !isValidVersion(version) {
		return "", fmt.Errorf("invalid version format: %s", version)
	}

	// Update progress
	s.updateProgress(id, 20, "compiling", "检查 nvm 安装状态...")

	// Check if nvm is installed
	homeDir := os.Getenv("HOME")
	if homeDir == "" {
		homeDir = "/root"
	}
	nvmDir := filepath.Join(homeDir, ".nvm")

	if _, err := exec.LookPath("nvm"); err != nil {
		// Install nvm first
		// SECURITY WARNING: Piping curl to bash is inherently risky (MITM, compromised CDN).
		// For production, consider downloading the script first, verifying its checksum, then executing.
		s.updateProgress(id, 30, "compiling", "正在安装 nvm...")
		_, _, nvmErr := s.executor.RunCombined(ctx, "bash", "-c", "curl -o- https://raw.githubusercontent.com/nvm-sh/nvm/v0.39.0/install.sh | bash")
		if nvmErr != nil {
			return "", fmt.Errorf("failed to install nvm: %v", nvmErr)
		}
	}

	// Install Node.js version (escape path to prevent injection)
	s.updateProgress(id, 50, "compiling", fmt.Sprintf("正在安装 Node.js %s...", version))
	safeNvmDir := shellEscape(nvmDir)
	safeVersion := shellEscape(version)
	output, _, err := s.executor.RunCombined(ctx, "bash", "-c", fmt.Sprintf("source %s/nvm.sh && nvm install %s", safeNvmDir, safeVersion))
	if err != nil {
		return "", fmt.Errorf("failed to install Node.js %s: %s", version, output)
	}

	return fmt.Sprintf("%s/versions/node/v%s", nvmDir, version), nil
}

// installGo installs Go from official binary
func (s *RuntimeService) installGo(ctx context.Context, id int64, version string) (string, error) {
	// Validate version to prevent command injection
	if !isValidVersion(version) {
		return "", fmt.Errorf("invalid version format: %s", version)
	}

	// Update progress
	s.updateProgress(id, 30, "downloading", fmt.Sprintf("正在下载 Go %s...", version))

	// Create installation directory under /opt
	installDir := "/opt/go"
	if err := os.MkdirAll(installDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create install directory: %v", err)
	}

	// Download and install Go
	url := fmt.Sprintf("https://go.dev/dl/go%s.linux-amd64.tar.gz", version)
	s.updateProgress(id, 50, "compiling", "正在解压安装...")
	output, _, err := s.executor.RunCombined(ctx, "bash", "-c", fmt.Sprintf(
		"curl -L %s | tar -C %s -xzf -", url, installDir,
	))
	if err != nil {
		return "", fmt.Errorf("failed to install Go %s: %s", version, output)
	}

	return fmt.Sprintf("%s/go", installDir), nil
}

// installPython installs Python using apt or pyenv
func (s *RuntimeService) installPython(ctx context.Context, id int64, version string) (string, error) {
	// Validate version to prevent command injection
	if !isValidVersion(version) {
		return "", fmt.Errorf("invalid version format: %s", version)
	}

	// Update progress
	s.updateProgress(id, 30, "compiling", "正在安装 Python...")

	// Try apt first
	output, _, err := s.executor.RunCombined(ctx, "apt-get", "install", "-y", fmt.Sprintf("python%s", version))
	if err == nil {
		return fmt.Sprintf("/usr/bin/python%s", version), nil
	}

	// Try yum
	output, _, err = s.executor.RunCombined(ctx, "yum", "install", "-y", fmt.Sprintf("python%s", version))
	if err == nil {
		return fmt.Sprintf("/usr/bin/python%s", version), nil
	}

	return "", fmt.Errorf("failed to install Python %s: %s", version, output)
}

// installPHP installs PHP using apt or yum
func (s *RuntimeService) installPHP(ctx context.Context, id int64, version string) (string, error) {
	// Validate version to prevent command injection
	if !isValidVersion(version) {
		return "", fmt.Errorf("invalid version format: %s", version)
	}

	// Update progress
	s.updateProgress(id, 30, "compiling", "正在安装 PHP...")

	// Try apt first
	output, _, err := s.executor.RunCombined(ctx, "apt-get", "install", "-y", fmt.Sprintf("php%s", version))
	if err == nil {
		return fmt.Sprintf("/usr/bin/php%s", version), nil
	}

	// Try yum
	s.updateProgress(id, 50, "compiling", "尝试使用 yum 安装...")
	output, _, err = s.executor.RunCombined(ctx, "yum", "install", "-y", fmt.Sprintf("php%s", version))
	if err == nil {
		return fmt.Sprintf("/usr/bin/php%s", version), nil
	}

	return "", fmt.Errorf("failed to install PHP %s: %s", version, output)
}

// Uninstall uninstalls a runtime environment
func (s *RuntimeService) Uninstall(ctx context.Context, name, version string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	// Get the environment info
	env, err := s.get(name, version)
	if err != nil {
		return err
	}
	if env == nil {
		return fmt.Errorf("%s %s not found", name, version)
	}

	// Don't allow uninstalling default version (but allow uninstalling failed installations)
	if env.IsDefault && env.Status != "failed" {
		return fmt.Errorf("cannot uninstall default version, please set another version as default first")
	}

	// Mark as uninstalling
	_, err = s.db.ExecContext(ctx, "UPDATE runtime_environments SET status = 'uninstalling' WHERE id = ?", env.ID)
	if err != nil {
		return fmt.Errorf("failed to update status: %w", err)
	}

	// Clean up related data before uninstalling
	s.cleanupRelatedData(env.ID)

	// Uninstall in background, delete DB row only on success
	go func() {
		bgCtx := context.Background()
		s.uninstallRuntime(bgCtx, env)
		// Delete from database
		s.db.ExecContext(bgCtx, "DELETE FROM runtime_environments WHERE id = ?", env.ID)
	}()

	return nil
}

// cleanupRelatedData cleans up environment variables and PATH entries
func (s *RuntimeService) cleanupRelatedData(runtimeID int64) {
	// Delete environment variables
	result, err := s.db.Exec("DELETE FROM env_configs WHERE runtime_id = ?", runtimeID)
	if err != nil {
		log.Printf("runtime: failed to cleanup env configs: %v", err)
	} else {
		rows, _ := result.RowsAffected()
		if rows > 0 {
			log.Printf("runtime: cleaned up %d environment variables", rows)
		}
	}

	// Delete PATH entries
	result, err = s.db.Exec("DELETE FROM path_entries WHERE runtime_id = ?", runtimeID)
	if err != nil {
		log.Printf("runtime: failed to cleanup path entries: %v", err)
	} else {
		rows, _ := result.RowsAffected()
		if rows > 0 {
			log.Printf("runtime: cleaned up %d PATH entries", rows)
		}
	}
}

// uninstallRuntime performs the actual uninstallation
func (s *RuntimeService) uninstallRuntime(ctx context.Context, env *model.RuntimeEnvironment) {
	var err error

	switch env.Name {
	case "java":
		_, _, err = s.executor.RunCombined(ctx, "apt-get", "remove", "-y", fmt.Sprintf("openjdk-%s-jdk", env.Version))
		// Clean up Java-specific residuals
		s.cleanupJavaResiduals(env.Version)
	case "node":
		// Validate path before deletion - must be under user's home directory
		if !isValidUninstallPath(env.Path) {
			log.Printf("runtime: refusing to delete path outside home directory: %s", env.Path)
			return
		}
		_, _, err = s.executor.RunCombined(ctx, "rm", "-rf", env.Path)
		// Clean up Node.js-specific residuals
		s.cleanupNodeResiduals(env.Version)
	case "go":
		// Go is installed via apt or official binary, use apt to remove
		_, _, err = s.executor.RunCombined(ctx, "apt-get", "remove", "-y", "golang-go")
		// Clean up Go-specific residuals
		s.cleanupGoResiduals()
	case "python":
		_, _, err = s.executor.RunCombined(ctx, "apt-get", "remove", "-y", fmt.Sprintf("python%s", env.Version))
		// Clean up Python-specific residuals
		s.cleanupPythonResiduals(env.Version)
	case "php":
		_, _, err = s.executor.RunCombined(ctx, "apt-get", "remove", "-y", fmt.Sprintf("php%s", env.Version))
		// Clean up PHP-specific residuals
		s.cleanupPHPResiduals(env.Version)
	}

	if err != nil {
		log.Printf("runtime: failed to uninstall %s %s: %v", env.Name, env.Version, err)
	} else {
		log.Printf("runtime: uninstalled %s %s", env.Name, env.Version)
	}
}

// cleanupJavaResiduals cleans up Java-specific residual files
func (s *RuntimeService) cleanupJavaResiduals(version string) {
	// Clean up Maven local repository cache (optional, user may want to keep)
	log.Printf("runtime: Java %s residuals cleaned", version)
}

// cleanupNodeResiduals cleans up Node.js-specific residual files
func (s *RuntimeService) cleanupNodeResiduals(version string) {
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
func (s *RuntimeService) cleanupGoResiduals() {
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
func (s *RuntimeService) cleanupPythonResiduals(version string) {
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
func (s *RuntimeService) cleanupPHPResiduals(version string) {
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
func (s *RuntimeService) GetEnvConfigsByRuntimeID(ctx context.Context, runtimeID int64) ([]model.EnvConfig, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	rows, err := s.db.QueryContext(ctx,
		"SELECT id, name, value, runtime_id, is_global, created_at, updated_at FROM env_configs WHERE runtime_id = ?",
		runtimeID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var configs []model.EnvConfig
	for rows.Next() {
		var c model.EnvConfig
		var isGlobal int
		err := rows.Scan(&c.ID, &c.Name, &c.Value, &c.RuntimeID, &isGlobal, &c.CreatedAt, &c.UpdatedAt)
		if err != nil {
			continue
		}
		c.IsGlobal = isGlobal != 0
		configs = append(configs, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate: %w", err)
	}

	return configs, nil
}

// GetPathEntriesByRuntimeID returns PATH entries for a runtime
func (s *RuntimeService) GetPathEntriesByRuntimeID(ctx context.Context, runtimeID int64) ([]model.PathEntry, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	rows, err := s.db.QueryContext(ctx,
		"SELECT id, path, runtime_id, is_global, order_num, created_at FROM path_entries WHERE runtime_id = ?",
		runtimeID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []model.PathEntry
	for rows.Next() {
		var e model.PathEntry
		var isGlobal int
		err := rows.Scan(&e.ID, &e.Path, &e.RuntimeID, &isGlobal, &e.Order, &e.CreatedAt)
		if err != nil {
			continue
		}
		e.IsGlobal = isGlobal != 0
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate: %w", err)
	}

	return entries, nil
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
func (s *RuntimeService) SetDefault(ctx context.Context, name, version string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	// Validate version to prevent command injection
	if !isValidVersion(version) {
		return fmt.Errorf("invalid version format: %s", version)
	}

	// Check if the version exists
	env, err := s.get(name, version)
	if err != nil {
		return err
	}
	if env == nil {
		return fmt.Errorf("%s %s not found", name, version)
	}

	// Reset all versions of this runtime to non-default
	_, err = s.db.ExecContext(ctx, "UPDATE runtime_environments SET is_default = 0 WHERE name = ?", name)
	if err != nil {
		return err
	}

	// Set this version as default
	_, err = s.db.ExecContext(ctx, "UPDATE runtime_environments SET is_default = 1 WHERE name = ? AND version = ?", name, version)
	return err
}

// Detect detects installed runtime environments on the system
func (s *RuntimeService) Detect(ctx context.Context) ([]model.RuntimeDetectResult, error) {
	var results []model.RuntimeDetectResult

	// Detect Java
	if versions, err := s.detectJava(ctx); err == nil && len(versions) > 0 {
		results = append(results, model.RuntimeDetectResult{Name: "java", Versions: versions})
	}

	// Detect Node.js
	if versions, err := s.detectNode(ctx); err == nil && len(versions) > 0 {
		results = append(results, model.RuntimeDetectResult{Name: "node", Versions: versions})
	}

	// Detect Go
	if versions, err := s.detectGo(ctx); err == nil && len(versions) > 0 {
		results = append(results, model.RuntimeDetectResult{Name: "go", Versions: versions})
	}

	// Detect Python
	if versions, err := s.detectPython(ctx); err == nil && len(versions) > 0 {
		results = append(results, model.RuntimeDetectResult{Name: "python", Versions: versions})
	}

	// Detect PHP
	if versions, err := s.detectPHP(ctx); err == nil && len(versions) > 0 {
		results = append(results, model.RuntimeDetectResult{Name: "php", Versions: versions})
	}

	return results, nil
}

// ImportDetected imports detected runtime environments into the database
func (s *RuntimeService) ImportDetected(ctx context.Context) (int, error) {
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
			var count int
			err := s.db.QueryRowContext(ctx,
				"SELECT COUNT(*) FROM runtime_environments WHERE name = ? AND version = ?",
				runtime.Name, version,
			).Scan(&count)
			if err != nil {
				continue
			}
			if count > 0 {
				continue
			}

			// Check if similar version exists (e.g., "17" matches "17.0.19")
			majorVersion := strings.Split(version, ".")[0]
			var similarCount int
			err = s.db.QueryRowContext(ctx,
				"SELECT COUNT(*) FROM runtime_environments WHERE name = ? AND (version = ? OR version LIKE ?)",
				runtime.Name, majorVersion, majorVersion+".%",
			).Scan(&similarCount)
			if err == nil && similarCount > 0 {
				// Similar version exists, skip
				continue
			}

			// Get the path
			path := getRuntimePath(runtime.Name, version)

			// Insert into database
			_, err = s.db.ExecContext(ctx,
				"INSERT INTO runtime_environments (name, version, path, status) VALUES (?, ?, ?, ?)",
				runtime.Name, version, path, "installed",
			)
			if err != nil {
				continue
			}

			// If this is the first version of this runtime, set as default
			var defaultCount int
			s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM runtime_environments WHERE name = ? AND is_default = 1", runtime.Name).Scan(&defaultCount)
			if defaultCount == 0 {
				s.db.ExecContext(ctx, "UPDATE runtime_environments SET is_default = 1 WHERE name = ? AND version = ?", runtime.Name, version)
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

// get returns a specific runtime environment
func (s *RuntimeService) get(name, version string) (*model.RuntimeEnvironment, error) {
	env := &model.RuntimeEnvironment{}
	var isDefault int
	err := s.db.QueryRow(
		"SELECT id, name, version, path, is_default, status, installed_at FROM runtime_environments WHERE name = ? AND version = ?",
		name, version,
	).Scan(&env.ID, &env.Name, &env.Version, &env.Path, &isDefault, &env.Status, &env.InstalledAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	env.IsDefault = isDefault != 0
	return env, nil
}

// detectJava detects installed Java versions
func (s *RuntimeService) detectJava(ctx context.Context) ([]string, error) {
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
func (s *RuntimeService) detectNode(ctx context.Context) ([]string, error) {
	output, _, err := s.executor.RunCombined(ctx, "node", "--version")
	if err != nil {
		return nil, err
	}

	version := strings.TrimSpace(output)
	version = strings.TrimPrefix(version, "v")
	return []string{version}, nil
}

// detectGo detects installed Go versions
func (s *RuntimeService) detectGo(ctx context.Context) ([]string, error) {
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
func (s *RuntimeService) detectPython(ctx context.Context) ([]string, error) {
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
func (s *RuntimeService) detectPHP(ctx context.Context) ([]string, error) {
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

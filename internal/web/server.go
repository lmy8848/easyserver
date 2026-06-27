package web

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"easyserver/internal/infra/apperror"
	"easyserver/internal/infra/executor"
)

// sanitizePackageName allows only alphanumeric characters, hyphens, dots, and plus signs
// in package names to prevent shell injection.
func sanitizePackageName(name string) string {
	var b strings.Builder
	for _, c := range name {
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-' || c == '.' || c == '+' || c == '_' {
			b.WriteRune(c)
		}
	}
	return b.String()
}

// Service manages web server lifecycle (install, start/stop, config).
type Service struct {
	repo     ServerRepository
	executor executor.CommandExecutor
}

func NewService(repo ServerRepository, exec executor.CommandExecutor) *Service {
	return &Service{repo: repo, executor: exec}
}

// SeedPredefinedWebServers inserts predefined web server entries if not exists.
// Called at startup to ensure default entries are present.
func (s *Service) SeedPredefinedWebServers(ctx context.Context) {
	if ctx == nil {
		ctx = context.Background()
	}
	existing, _ := s.repo.List(ctx)
	existingByName := make(map[string]bool, len(existing))
	for _, ws := range existing {
		existingByName[ws.Name] = true
	}

	for _, ws := range PredefinedWebServers() {
		if !existingByName[ws.Name] {
			s.repo.Create(ctx, &ws)
		}
	}
}

func (s *Service) List(ctx context.Context) ([]WebServer, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	return s.repo.List(ctx)
}

func (s *Service) Get(ctx context.Context, id int64) (*WebServer, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	return s.repo.Get(ctx, id)
}

func (s *Service) Create(ctx context.Context, ws *WebServer) error {
	if ctx == nil {
		ctx = context.Background()
	}
	return s.repo.Create(ctx, ws)
}

func (s *Service) Delete(ctx context.Context, id int64) error {
	if ctx == nil {
		ctx = context.Background()
	}
	ws, err := s.repo.Get(ctx, id)
	if err != nil {
		return err
	}
	if ws == nil {
		return apperror.ErrNotFound.WithMessage("Web 服务器不存在")
	}

	count, _ := s.repo.CountWebsitesByServerID(ctx, id)
	if count > 0 {
		return apperror.ErrConflict.WithMessage(fmt.Sprintf("无法删除：%d 个网站正在使用此服务器", count))
	}

	return s.repo.Delete(ctx, id)
}

// Install installs the web server software.
// The install command is always looked up from the predefined template to prevent
// execution of arbitrary commands stored in the database (e.g. via SQL injection).
func (s *Service) Install(ctx context.Context, id int64) error {
	if ctx == nil {
		ctx = context.Background()
	}
	ws, err := s.repo.Get(ctx, id)
	if err != nil {
		return err
	}
	if ws == nil {
		return apperror.ErrNotFound.WithMessage("Web 服务器不存在")
	}

	// Always use the predefined install command, never trust the database value
	predef := FindPredefinedWebServer(ws.Name)
	if predef == nil {
		return apperror.ErrBadRequest.WithMessage(fmt.Sprintf("未知的服务器类型 '%s'", ws.Name))
	}
	installCmd := predef.InstallCmd
	if installCmd == "" {
		return apperror.ErrBadRequest.WithMessage(fmt.Sprintf("服务器类型 '%s' 未配置安装命令", ws.Name))
	}

	s.executor.RunCombined(ctx, "apt-get", "update", "-y")

	parts := strings.Fields(installCmd)
	out, _, err := s.executor.RunCombined(ctx, parts[0], parts[1:]...)
	if err != nil {
		return fmt.Errorf("install failed: %s", out)
	}

	if ws.ServiceName != "" {
		s.executor.RunCombined(ctx, "systemctl", "enable", ws.ServiceName)
		s.executor.RunCombined(ctx, "systemctl", "start", ws.ServiceName)
	}

	s.RefreshStatus(ctx, id)
	return nil
}

// Uninstall removes the web server software
func (s *Service) Uninstall(ctx context.Context, id int64) error {
	if ctx == nil {
		ctx = context.Background()
	}
	ws, err := s.repo.Get(ctx, id)
	if err != nil {
		return err
	}
	if ws == nil {
		return apperror.ErrNotFound.WithMessage("Web 服务器不存在")
	}

	count, _ := s.repo.CountWebsitesByServerID(ctx, id)
	if count > 0 {
		return apperror.ErrConflict.WithMessage(fmt.Sprintf("无法卸载：%d 个网站正在使用此服务器", count))
	}

	if ws.ServiceName != "" {
		s.executor.RunCombined(ctx, "systemctl", "stop", ws.ServiceName)
		s.executor.RunCombined(ctx, "systemctl", "disable", ws.ServiceName)
	}

	// Always use the predefined uninstall command, never trust the database value
	predef := FindPredefinedWebServer(ws.Name)
	var uninstallCmd string
	if predef != nil && predef.UninstallCmd != "" {
		uninstallCmd = predef.UninstallCmd
	} else {
		// Sanitize ws.Name to prevent shell injection - only allow alphanumeric, hyphens, dots
		safeName := sanitizePackageName(ws.Name)
		if safeName == "" {
			return apperror.ErrBadRequest.WithMessage(fmt.Sprintf("无效的服务器名称：%s", ws.Name))
		}
		uninstallCmd = fmt.Sprintf("apt-get remove -y %s", safeName)
	}
	parts := strings.Fields(uninstallCmd)
	out, _, err := s.executor.RunCombined(ctx, parts[0], parts[1:]...)
	if err != nil {
		return fmt.Errorf("uninstall failed: %s", out)
	}

	s.repo.UpdateStatus(ctx, id, "not_installed")
	return nil
}

// Start starts the web server service
func (s *Service) Start(ctx context.Context, id int64) error {
	if ctx == nil {
		ctx = context.Background()
	}
	ws, err := s.repo.Get(ctx, id)
	if err != nil {
		return err
	}
	if ws == nil {
		return apperror.ErrNotFound.WithMessage("Web 服务器不存在")
	}
	if ws.ServiceName == "" {
		return apperror.ErrBadRequest.WithMessage("未配置服务名称")
	}

	out, _, err := s.executor.RunCombined(ctx, "systemctl", "start", ws.ServiceName)
	if err != nil {
		return fmt.Errorf("start failed: %s", out)
	}

	return s.repo.UpdateStatus(ctx, id, "running")
}

// Stop stops the web server service
func (s *Service) Stop(ctx context.Context, id int64) error {
	if ctx == nil {
		ctx = context.Background()
	}
	ws, err := s.repo.Get(ctx, id)
	if err != nil {
		return err
	}
	if ws == nil {
		return apperror.ErrNotFound.WithMessage("Web 服务器不存在")
	}
	if ws.ServiceName == "" {
		return apperror.ErrBadRequest.WithMessage("未配置服务名称")
	}

	out, _, err := s.executor.RunCombined(ctx, "systemctl", "stop", ws.ServiceName)
	if err != nil {
		return fmt.Errorf("stop failed: %s", out)
	}

	return s.repo.UpdateStatus(ctx, id, "stopped")
}

// Restart restarts the web server service
func (s *Service) Restart(ctx context.Context, id int64) error {
	if ctx == nil {
		ctx = context.Background()
	}
	ws, err := s.repo.Get(ctx, id)
	if err != nil {
		return err
	}
	if ws == nil {
		return apperror.ErrNotFound.WithMessage("Web 服务器不存在")
	}
	if ws.ServiceName == "" {
		return apperror.ErrBadRequest.WithMessage("未配置服务名称")
	}

	out, _, err := s.executor.RunCombined(ctx, "systemctl", "restart", ws.ServiceName)
	if err != nil {
		return fmt.Errorf("restart failed: %s", out)
	}

	return s.repo.UpdateStatus(ctx, id, "running")
}

// Reload reloads the web server config without restarting
func (s *Service) Reload(ctx context.Context, id int64) error {
	if ctx == nil {
		ctx = context.Background()
	}
	ws, err := s.repo.Get(ctx, id)
	if err != nil {
		return err
	}
	if ws == nil {
		return apperror.ErrNotFound.WithMessage("Web 服务器不存在")
	}

	// Test config first
	if ok, _ := s.TestConfig(ctx, id); !ok {
		return apperror.ErrBadRequest.WithMessage("配置测试失败，已中止重载")
	}

	if ws.ServiceName != "" {
		out, _, err := s.executor.RunCombined(ctx, "systemctl", "reload", ws.ServiceName)
		if err != nil {
			return fmt.Errorf("reload failed: %s", out)
		}
	}
	return nil
}

// TestConfig tests the web server configuration
func (s *Service) TestConfig(ctx context.Context, id int64) (bool, string) {
	if ctx == nil {
		ctx = context.Background()
	}
	ws, err := s.repo.Get(ctx, id)
	if err != nil || ws == nil {
		return false, "web server not found"
	}

	switch ws.Name {
	case "nginx":
		out, _, err := s.executor.RunCombined(ctx, "nginx", "-t")
		msg := strings.TrimSpace(out)
		if err != nil {
			return false, msg
		}
		return true, msg
	case "apache":
		out, _, err := s.executor.RunCombined(ctx, "apache2ctl", "configtest")
		msg := strings.TrimSpace(out)
		if err != nil {
			return false, msg
		}
		return true, msg
	case "caddy":
		out, _, err := s.executor.RunCombined(ctx, "caddy", "validate", "--config", ws.ConfigFile)
		msg := strings.TrimSpace(out)
		if err != nil {
			return false, msg
		}
		return true, msg
	default:
		return true, "no config test available"
	}
}

// validateConfigPath checks that a config file path is safe and within allowed directories.
// This prevents path traversal or manipulation of ConfigFile stored in the database.
func validateConfigPath(path string) error {
	if path == "" {
		return apperror.ErrBadRequest.WithMessage("配置文件路径为空")
	}
	if strings.Contains(path, "..") {
		return apperror.ErrBadRequest.WithMessage("配置路径不能包含 '..'")
	}
	// Clean the path to resolve any . or extra slashes
	cleaned := filepath.Clean(path)
	// Allowlist of config directory prefixes
	allowedPrefixes := []string{
		"/etc/nginx/",
		"/etc/apache2/",
		"/etc/tomcat9/",
		"/etc/caddy/",
		"/etc/httpd/",
		"/etc/lighttpd/",
	}
	for _, prefix := range allowedPrefixes {
		if strings.HasPrefix(cleaned, prefix) || cleaned == strings.TrimSuffix(prefix, "/") {
			return nil
		}
	}
	return apperror.ErrPathViolation.WithMessage(fmt.Sprintf("配置路径 %q 不在允许的目录中", path))
}

// GetConfig reads the main config file content
func (s *Service) GetConfig(ctx context.Context, id int64) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	ws, err := s.repo.Get(ctx, id)
	if err != nil {
		return "", err
	}
	if ws == nil {
		return "", apperror.ErrNotFound.WithMessage("Web 服务器不存在")
	}
	if ws.ConfigFile == "" {
		return "", apperror.ErrBadRequest.WithMessage("未配置配置文件路径")
	}
	if err := validateConfigPath(ws.ConfigFile); err != nil {
		return "", err
	}

	data, err := os.ReadFile(ws.ConfigFile)
	if err != nil {
		return "", fmt.Errorf("failed to read config: %w", err)
	}
	return string(data), nil
}

// SaveConfig writes content to the main config file
func (s *Service) SaveConfig(ctx context.Context, id int64, content string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	ws, err := s.repo.Get(ctx, id)
	if err != nil {
		return err
	}
	if ws == nil {
		return apperror.ErrNotFound.WithMessage("Web 服务器不存在")
	}
	if ws.ConfigFile == "" {
		return apperror.ErrBadRequest.WithMessage("未配置配置文件路径")
	}
	if err := validateConfigPath(ws.ConfigFile); err != nil {
		return err
	}

	// Backup current config
	backupPath := ws.ConfigFile + ".bak." + time.Now().Format("20060102150405")
	if data, err := os.ReadFile(ws.ConfigFile); err == nil {
		os.WriteFile(backupPath, data, 0644)
	}

	return os.WriteFile(ws.ConfigFile, []byte(content), 0644)
}

// GetServiceLogs returns recent service logs via journalctl
func (s *Service) GetServiceLogs(ctx context.Context, id int64, lines int) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	ws, err := s.repo.Get(ctx, id)
	if err != nil {
		return "", err
	}
	if ws == nil {
		return "", apperror.ErrNotFound.WithMessage("Web 服务器不存在")
	}
	if ws.ServiceName == "" {
		return "", apperror.ErrBadRequest.WithMessage("未配置服务名称")
	}
	if lines <= 0 {
		lines = 100
	}

	out, _, err := s.executor.RunCombined(ctx, "journalctl", "-u", ws.ServiceName, "-n", strconv.Itoa(lines), "--no-pager")
	if err != nil {
		return out, nil
	}
	return out, nil
}

// SetAutoStart enables/disables auto-start on boot
func (s *Service) SetAutoStart(ctx context.Context, id int64, enabled bool) error {
	if ctx == nil {
		ctx = context.Background()
	}
	ws, err := s.repo.Get(ctx, id)
	if err != nil {
		return err
	}
	if ws == nil {
		return apperror.ErrNotFound.WithMessage("Web 服务器不存在")
	}
	if ws.ServiceName == "" {
		return apperror.ErrBadRequest.WithMessage("未配置服务名称")
	}

	action := "disable"
	if enabled {
		action = "enable"
	}

	out, _, err := s.executor.RunCombined(ctx, "systemctl", action, ws.ServiceName)
	if err != nil {
		return fmt.Errorf("systemctl %s failed: %s", action, out)
	}
	return nil
}

// RefreshStatus detects full runtime state
func (s *Service) RefreshStatus(ctx context.Context, id int64) error {
	if ctx == nil {
		ctx = context.Background()
	}
	ws, err := s.repo.Get(ctx, id)
	if err != nil {
		return err
	}
	if ws == nil {
		return apperror.ErrNotFound.WithMessage("Web 服务器不存在")
	}

	installed := false
	version := ""

	switch ws.Name {
	case "nginx":
		if _, err := s.executor.LookPath("nginx"); err == nil {
			installed = true
			out, _, _ := s.executor.RunCombined(ctx, "nginx", "-v")
			version = strings.TrimSpace(out)
		}
	case "apache":
		if _, err := s.executor.LookPath("apache2"); err == nil {
			installed = true
			out, _, _ := s.executor.RunCombined(ctx, "apache2", "-v")
			lines := strings.Split(strings.TrimSpace(out), "\n")
			if len(lines) > 0 {
				version = strings.TrimSpace(lines[0])
			}
		}
	case "tomcat":
		if _, err := os.Stat("/etc/tomcat9"); err == nil {
			installed = true
			version = "tomcat9"
		}
	case "caddy":
		if _, err := s.executor.LookPath("caddy"); err == nil {
			installed = true
			out, _, _ := s.executor.RunCombined(ctx, "caddy", "version")
			version = strings.TrimSpace(out)
		}
	default:
		if ws.BinaryPath != "" {
			if _, err := os.Stat(ws.BinaryPath); err == nil {
				installed = true
				version = "installed"
			}
		}
	}

	status := "not_installed"
	if installed {
		status = "stopped"
		if ws.ServiceName != "" {
			out, _, _ := s.executor.RunCombined(ctx, "systemctl", "is-active", ws.ServiceName)
			if strings.TrimSpace(out) == "active" {
				status = "running"
			}
		}
	}

	return s.repo.UpdateStatusAndVersion(ctx, id, status, version)
}

// RefreshAllStatus refreshes status for all web servers
func (s *Service) RefreshAllStatus(ctx context.Context) {
	if ctx == nil {
		ctx = context.Background()
	}
	servers, _ := s.repo.List(ctx)
	for _, ws := range servers {
		s.RefreshStatus(ctx, ws.ID)
	}
}

// GetConnections returns active connection count (for Nginx)
func (s *Service) GetConnections(ctx context.Context, id int64) (int, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	ws, err := s.repo.Get(ctx, id)
	if err != nil {
		return 0, err
	}
	if ws == nil {
		return 0, apperror.ErrNotFound.WithMessage("Web 服务器不存在")
	}

	// Count connections from ss
	out, _, _ := s.executor.RunCombined(ctx, "ss", "-tlnp")
	count := 0
	for _, line := range strings.Split(out, "\n") {
		if ws.ServiceName != "" && strings.Contains(line, ws.ServiceName) {
			count++
		}
	}
	return count, nil
}

// GetProcessInfo returns PID, memory, uptime for the service
func (s *Service) GetProcessInfo(ctx context.Context, id int64) (pid int, memBytes int64, uptime string, err error) {
	if ctx == nil {
		ctx = context.Background()
	}
	ws, e := s.repo.Get(ctx, id)
	if e != nil {
		return 0, 0, "", e
	}
	if ws == nil {
		return 0, 0, "", apperror.ErrNotFound.WithMessage("Web 服务器不存在")
	}
	if ws.ServiceName == "" {
		return 0, 0, "", nil
	}

	// Get main PID via systemctl
	out, _, e := s.executor.RunCombined(ctx, "systemctl", "show", ws.ServiceName, "--property=MainPID,ActiveEnterTimestamp")
	if e != nil {
		return 0, 0, "", nil
	}

	lines := strings.Split(strings.TrimSpace(out), "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "MainPID=") {
			pidStr := strings.TrimPrefix(line, "MainPID=")
			pid, _ = strconv.Atoi(pidStr)
		}
		if strings.HasPrefix(line, "ActiveEnterTimestamp=") {
			ts := strings.TrimPrefix(line, "ActiveEnterTimestamp=")
			if t, e := time.Parse("Mon 2006-01-02 15:04:05 MST", ts); e == nil {
				d := time.Since(t)
				if d < time.Minute {
					uptime = fmt.Sprintf("%ds", int(d.Seconds()))
				} else if d < time.Hour {
					uptime = fmt.Sprintf("%dm", int(d.Minutes()))
				} else if d < 24*time.Hour {
					uptime = fmt.Sprintf("%dh%dm", int(d.Hours()), int(d.Minutes())%60)
				} else {
					uptime = fmt.Sprintf("%dd%dh", int(d.Hours()/24), int(d.Hours())%24)
				}
			}
		}
	}

	// Get memory from /proc
	if pid > 0 {
		statusPath := fmt.Sprintf("/proc/%d/status", pid)
		if data, e := os.ReadFile(statusPath); e == nil {
			for _, line := range strings.Split(string(data), "\n") {
				if strings.HasPrefix(line, "VmRSS:") {
					fields := strings.Fields(line)
					if len(fields) >= 2 {
						kb, _ := strconv.ParseInt(fields[1], 10, 64)
						memBytes = kb * 1024
					}
				}
			}
		}
	}

	return pid, memBytes, uptime, nil
}

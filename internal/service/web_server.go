package service

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"easyserver/internal/executor"
	"easyserver/internal/model"
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

type WebServerService struct {
	db       *sql.DB
	executor executor.CommandExecutor
}

func NewWebServerService(db *sql.DB, exec executor.CommandExecutor) *WebServerService {
	return &WebServerService{db: db, executor: exec}
}

// Deprecated: InitTables is kept for backward compatibility only.
// Table creation is now handled by the migration system (migrations/ directory).
func (s *WebServerService) InitTables(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	queries := []string{
		`CREATE TABLE IF NOT EXISTS web_servers (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL UNIQUE,
			display_name TEXT NOT NULL,
			description TEXT DEFAULT '',
			install_cmd TEXT DEFAULT '',
			uninstall_cmd TEXT DEFAULT '',
			config_path TEXT DEFAULT '',
			config_file TEXT DEFAULT '',
			sites_available TEXT DEFAULT '',
			sites_enabled TEXT DEFAULT '',
			service_name TEXT DEFAULT '',
			binary_path TEXT DEFAULT '',
			default_port INTEGER DEFAULT 80,
			log_dir TEXT DEFAULT '',
			status TEXT DEFAULT 'not_installed',
			version TEXT DEFAULT '',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_web_servers_name ON web_servers(name)`,
	}
	for _, q := range queries {
		if _, err := s.db.ExecContext(ctx, q); err != nil {
			return err
		}
	}

	// Migration: add new columns if missing
	cols := []struct{ name, typ string }{
		{"uninstall_cmd", "TEXT DEFAULT ''"},
		{"config_file", "TEXT DEFAULT ''"},
		{"binary_path", "TEXT DEFAULT ''"},
		{"default_port", "INTEGER DEFAULT 80"},
		{"log_dir", "TEXT DEFAULT ''"},
	}
	for _, c := range cols {
		s.db.ExecContext(ctx, fmt.Sprintf("ALTER TABLE web_servers ADD COLUMN %s %s", c.name, c.typ))
	}

	// Insert predefined web servers if not exists
	for _, ws := range model.PredefinedWebServers() {
		var count int
		s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM web_servers WHERE name = ?", ws.Name).Scan(&count)
		if count == 0 {
			s.db.ExecContext(ctx, `INSERT INTO web_servers (name, display_name, description, install_cmd, uninstall_cmd, config_path, config_file, sites_available, sites_enabled, service_name, binary_path, default_port, log_dir)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				ws.Name, ws.DisplayName, ws.Description, ws.InstallCmd, ws.UninstallCmd,
				ws.ConfigPath, ws.ConfigFile, ws.SitesAvailable, ws.SitesEnabled,
				ws.ServiceName, ws.BinaryPath, ws.DefaultPort, ws.LogDir)
		} else {
			// Update existing with new fields
			s.db.ExecContext(ctx, `UPDATE web_servers SET uninstall_cmd=?, config_file=?, binary_path=?, default_port=?, log_dir=?, description=? WHERE name=? AND (uninstall_cmd='' OR config_file='')`,
				ws.UninstallCmd, ws.ConfigFile, ws.BinaryPath, ws.DefaultPort, ws.LogDir, ws.Description, ws.Name)
		}
	}

	return nil
}

// SeedPredefinedWebServers inserts predefined web server entries if not exists.
// Called at startup to ensure default entries are present.
func (s *WebServerService) SeedPredefinedWebServers(ctx context.Context) {
	if ctx == nil {
		ctx = context.Background()
	}
	for _, ws := range model.PredefinedWebServers() {
		var count int
		s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM web_servers WHERE name = ?", ws.Name).Scan(&count)
		if count == 0 {
			s.db.ExecContext(ctx, `INSERT INTO web_servers (name, display_name, description, install_cmd, uninstall_cmd, config_path, config_file, sites_available, sites_enabled, service_name, binary_path, default_port, log_dir)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				ws.Name, ws.DisplayName, ws.Description, ws.InstallCmd, ws.UninstallCmd,
				ws.ConfigPath, ws.ConfigFile, ws.SitesAvailable, ws.SitesEnabled,
				ws.ServiceName, ws.BinaryPath, ws.DefaultPort, ws.LogDir)
		} else {
			// Update existing with new fields
			s.db.ExecContext(ctx, `UPDATE web_servers SET uninstall_cmd=?, config_file=?, binary_path=?, default_port=?, log_dir=?, description=? WHERE name=? AND (uninstall_cmd='' OR config_file='')`,
				ws.UninstallCmd, ws.ConfigFile, ws.BinaryPath, ws.DefaultPort, ws.LogDir, ws.Description, ws.Name)
		}
	}
}

func (s *WebServerService) List(ctx context.Context) ([]model.WebServer, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id, name, display_name, description, install_cmd, uninstall_cmd,
		config_path, config_file, sites_available, sites_enabled, service_name, binary_path,
		default_port, log_dir, status, version, created_at
		FROM web_servers ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var servers []model.WebServer
	for rows.Next() {
		var ws model.WebServer
		err := rows.Scan(&ws.ID, &ws.Name, &ws.DisplayName, &ws.Description,
			&ws.InstallCmd, &ws.UninstallCmd, &ws.ConfigPath, &ws.ConfigFile,
			&ws.SitesAvailable, &ws.SitesEnabled, &ws.ServiceName, &ws.BinaryPath,
			&ws.DefaultPort, &ws.LogDir, &ws.Status, &ws.Version, &ws.CreatedAt)
		if err != nil {
			continue
		}
		servers = append(servers, ws)
	}
	return servers, nil
}

func (s *WebServerService) Get(ctx context.Context, id int64) (*model.WebServer, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	ws := &model.WebServer{}
	err := s.db.QueryRowContext(ctx, `SELECT id, name, display_name, description, install_cmd, uninstall_cmd,
		config_path, config_file, sites_available, sites_enabled, service_name, binary_path,
		default_port, log_dir, status, version, created_at
		FROM web_servers WHERE id = ?`, id).Scan(
		&ws.ID, &ws.Name, &ws.DisplayName, &ws.Description,
		&ws.InstallCmd, &ws.UninstallCmd, &ws.ConfigPath, &ws.ConfigFile,
		&ws.SitesAvailable, &ws.SitesEnabled, &ws.ServiceName, &ws.BinaryPath,
		&ws.DefaultPort, &ws.LogDir, &ws.Status, &ws.Version, &ws.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return ws, nil
}

func (s *WebServerService) Create(ctx context.Context, ws *model.WebServer) error {
	if ctx == nil {
		ctx = context.Background()
	}
	result, err := s.db.ExecContext(ctx, `INSERT INTO web_servers (name, display_name, description, install_cmd, uninstall_cmd, config_path, config_file, sites_available, sites_enabled, service_name, binary_path, default_port, log_dir)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		ws.Name, ws.DisplayName, ws.Description, ws.InstallCmd, ws.UninstallCmd,
		ws.ConfigPath, ws.ConfigFile, ws.SitesAvailable, ws.SitesEnabled,
		ws.ServiceName, ws.BinaryPath, ws.DefaultPort, ws.LogDir)
	if err != nil {
		return err
	}
	ws.ID, _ = result.LastInsertId()
	return nil
}

func (s *WebServerService) Delete(ctx context.Context, id int64) error {
	if ctx == nil {
		ctx = context.Background()
	}
	ws, err := s.Get(ctx, id)
	if err != nil {
		return err
	}
	if ws == nil {
		return fmt.Errorf("web server not found")
	}

	var count int
	s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM websites WHERE web_server_id = ?", id).Scan(&count)
	if count > 0 {
		return fmt.Errorf("cannot delete: %d websites are using this server", count)
	}

	_, err = s.db.ExecContext(ctx, "DELETE FROM web_servers WHERE id = ?", id)
	return err
}

// Install installs the web server software.
// The install command is always looked up from the predefined template to prevent
// execution of arbitrary commands stored in the database (e.g. via SQL injection).
func (s *WebServerService) Install(ctx context.Context, id int64) error {
	if ctx == nil {
		ctx = context.Background()
	}
	ws, err := s.Get(ctx, id)
	if err != nil {
		return err
	}
	if ws == nil {
		return fmt.Errorf("web server not found")
	}

	// Always use the predefined install command, never trust the database value
	predef := model.FindPredefinedWebServer(ws.Name)
	if predef == nil {
		return fmt.Errorf("unknown server type '%s'", ws.Name)
	}
	installCmd := predef.InstallCmd
	if installCmd == "" {
		return fmt.Errorf("no install command configured for server type '%s'", ws.Name)
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
func (s *WebServerService) Uninstall(ctx context.Context, id int64) error {
	if ctx == nil {
		ctx = context.Background()
	}
	ws, err := s.Get(ctx, id)
	if err != nil {
		return err
	}
	if ws == nil {
		return fmt.Errorf("web server not found")
	}

	var count int
	s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM websites WHERE web_server_id = ?", id).Scan(&count)
	if count > 0 {
		return fmt.Errorf("cannot uninstall: %d websites are using this server", count)
	}

	if ws.ServiceName != "" {
		s.executor.RunCombined(ctx, "systemctl", "stop", ws.ServiceName)
		s.executor.RunCombined(ctx, "systemctl", "disable", ws.ServiceName)
	}

	// Always use the predefined uninstall command, never trust the database value
	predef := model.FindPredefinedWebServer(ws.Name)
	var uninstallCmd string
	if predef != nil && predef.UninstallCmd != "" {
		uninstallCmd = predef.UninstallCmd
	} else {
		// Sanitize ws.Name to prevent shell injection - only allow alphanumeric, hyphens, dots
		safeName := sanitizePackageName(ws.Name)
		if safeName == "" {
			return fmt.Errorf("invalid server name: %s", ws.Name)
		}
		uninstallCmd = fmt.Sprintf("apt-get remove -y %s", safeName)
	}
	parts := strings.Fields(uninstallCmd)
	out, _, err := s.executor.RunCombined(ctx, parts[0], parts[1:]...)
	if err != nil {
		return fmt.Errorf("uninstall failed: %s", out)
	}

	s.db.ExecContext(ctx, "UPDATE web_servers SET status = 'not_installed', version = '' WHERE id = ?", id)
	return nil
}

// Start starts the web server service
func (s *WebServerService) Start(ctx context.Context, id int64) error {
	if ctx == nil {
		ctx = context.Background()
	}
	ws, err := s.Get(ctx, id)
	if err != nil {
		return err
	}
	if ws == nil {
		return fmt.Errorf("web server not found")
	}
	if ws.ServiceName == "" {
		return fmt.Errorf("no service name configured")
	}

	out, _, err := s.executor.RunCombined(ctx, "systemctl", "start", ws.ServiceName)
	if err != nil {
		return fmt.Errorf("start failed: %s", out)
	}

	s.db.ExecContext(ctx, "UPDATE web_servers SET status = 'running' WHERE id = ?", id)
	return nil
}

// Stop stops the web server service
func (s *WebServerService) Stop(ctx context.Context, id int64) error {
	if ctx == nil {
		ctx = context.Background()
	}
	ws, err := s.Get(ctx, id)
	if err != nil {
		return err
	}
	if ws == nil {
		return fmt.Errorf("web server not found")
	}
	if ws.ServiceName == "" {
		return fmt.Errorf("no service name configured")
	}

	out, _, err := s.executor.RunCombined(ctx, "systemctl", "stop", ws.ServiceName)
	if err != nil {
		return fmt.Errorf("stop failed: %s", out)
	}

	s.db.ExecContext(ctx, "UPDATE web_servers SET status = 'stopped' WHERE id = ?", id)
	return nil
}

// Restart restarts the web server service
func (s *WebServerService) Restart(ctx context.Context, id int64) error {
	if ctx == nil {
		ctx = context.Background()
	}
	ws, err := s.Get(ctx, id)
	if err != nil {
		return err
	}
	if ws == nil {
		return fmt.Errorf("web server not found")
	}
	if ws.ServiceName == "" {
		return fmt.Errorf("no service name configured")
	}

	out, _, err := s.executor.RunCombined(ctx, "systemctl", "restart", ws.ServiceName)
	if err != nil {
		return fmt.Errorf("restart failed: %s", out)
	}

	s.db.ExecContext(ctx, "UPDATE web_servers SET status = 'running' WHERE id = ?", id)
	return nil
}

// Reload reloads the web server config without restarting
func (s *WebServerService) Reload(ctx context.Context, id int64) error {
	if ctx == nil {
		ctx = context.Background()
	}
	ws, err := s.Get(ctx, id)
	if err != nil {
		return err
	}
	if ws == nil {
		return fmt.Errorf("web server not found")
	}

	// Test config first
	if ok, _ := s.TestConfig(ctx, id); !ok {
		return fmt.Errorf("config test failed, reload aborted")
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
func (s *WebServerService) TestConfig(ctx context.Context, id int64) (bool, string) {
	if ctx == nil {
		ctx = context.Background()
	}
	ws, err := s.Get(ctx, id)
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

// GetConfig reads the main config file content
func (s *WebServerService) GetConfig(ctx context.Context, id int64) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	ws, err := s.Get(ctx, id)
	if err != nil {
		return "", err
	}
	if ws == nil {
		return "", fmt.Errorf("web server not found")
	}
	if ws.ConfigFile == "" {
		return "", fmt.Errorf("no config file path configured")
	}

	data, err := os.ReadFile(ws.ConfigFile)
	if err != nil {
		return "", fmt.Errorf("failed to read config: %w", err)
	}
	return string(data), nil
}

// SaveConfig writes content to the main config file
func (s *WebServerService) SaveConfig(ctx context.Context, id int64, content string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	ws, err := s.Get(ctx, id)
	if err != nil {
		return err
	}
	if ws == nil {
		return fmt.Errorf("web server not found")
	}
	if ws.ConfigFile == "" {
		return fmt.Errorf("no config file path configured")
	}

	// Backup current config
	backupPath := ws.ConfigFile + ".bak." + time.Now().Format("20060102150405")
	if data, err := os.ReadFile(ws.ConfigFile); err == nil {
		os.WriteFile(backupPath, data, 0644)
	}

	return os.WriteFile(ws.ConfigFile, []byte(content), 0644)
}

// GetServiceLogs returns recent service logs via journalctl
func (s *WebServerService) GetServiceLogs(ctx context.Context, id int64, lines int) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	ws, err := s.Get(ctx, id)
	if err != nil {
		return "", err
	}
	if ws == nil {
		return "", fmt.Errorf("web server not found")
	}
	if ws.ServiceName == "" {
		return "", fmt.Errorf("no service name configured")
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
func (s *WebServerService) SetAutoStart(ctx context.Context, id int64, enabled bool) error {
	if ctx == nil {
		ctx = context.Background()
	}
	ws, err := s.Get(ctx, id)
	if err != nil {
		return err
	}
	if ws == nil {
		return fmt.Errorf("web server not found")
	}
	if ws.ServiceName == "" {
		return fmt.Errorf("no service name configured")
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
func (s *WebServerService) RefreshStatus(ctx context.Context, id int64) error {
	if ctx == nil {
		ctx = context.Background()
	}
	ws, err := s.Get(ctx, id)
	if err != nil {
		return err
	}
	if ws == nil {
		return fmt.Errorf("web server not found")
	}

	installed := false
	version := ""

	switch ws.Name {
	case "nginx":
		if _, err := exec.LookPath("nginx"); err == nil {
			installed = true
			out, _, _ := s.executor.RunCombined(ctx, "nginx", "-v")
			version = strings.TrimSpace(out)
		}
	case "apache":
		if _, err := exec.LookPath("apache2"); err == nil {
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
		if _, err := exec.LookPath("caddy"); err == nil {
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

	s.db.ExecContext(ctx, "UPDATE web_servers SET status = ?, version = ? WHERE id = ?", status, version, id)
	return nil
}

// RefreshAllStatus refreshes status for all web servers
func (s *WebServerService) RefreshAllStatus(ctx context.Context) {
	if ctx == nil {
		ctx = context.Background()
	}
	servers, _ := s.List(ctx)
	for _, ws := range servers {
		s.RefreshStatus(ctx, ws.ID)
	}
}

// GetConnections returns active connection count (for Nginx)
func (s *WebServerService) GetConnections(ctx context.Context, id int64) (int, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	ws, err := s.Get(ctx, id)
	if err != nil {
		return 0, err
	}
	if ws == nil {
		return 0, fmt.Errorf("web server not found")
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
func (s *WebServerService) GetProcessInfo(ctx context.Context, id int64) (pid int, memBytes int64, uptime string, err error) {
	if ctx == nil {
		ctx = context.Background()
	}
	ws, e := s.Get(ctx, id)
	if e != nil {
		return 0, 0, "", e
	}
	if ws == nil {
		return 0, 0, "", fmt.Errorf("web server not found")
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

package service

import (
	"database/sql"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"easyserver/internal/model"
)

type WebServerService struct {
	db *sql.DB
}

func NewWebServerService(db *sql.DB) *WebServerService {
	return &WebServerService{db: db}
}

func (s *WebServerService) InitTables() error {
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
		if _, err := s.db.Exec(q); err != nil {
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
		s.db.Exec(fmt.Sprintf("ALTER TABLE web_servers ADD COLUMN %s %s", c.name, c.typ))
	}

	// Insert predefined web servers if not exists
	for _, ws := range model.PredefinedWebServers() {
		var count int
		s.db.QueryRow("SELECT COUNT(*) FROM web_servers WHERE name = ?", ws.Name).Scan(&count)
		if count == 0 {
			s.db.Exec(`INSERT INTO web_servers (name, display_name, description, install_cmd, uninstall_cmd, config_path, config_file, sites_available, sites_enabled, service_name, binary_path, default_port, log_dir)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				ws.Name, ws.DisplayName, ws.Description, ws.InstallCmd, ws.UninstallCmd,
				ws.ConfigPath, ws.ConfigFile, ws.SitesAvailable, ws.SitesEnabled,
				ws.ServiceName, ws.BinaryPath, ws.DefaultPort, ws.LogDir)
		} else {
			// Update existing with new fields
			s.db.Exec(`UPDATE web_servers SET uninstall_cmd=?, config_file=?, binary_path=?, default_port=?, log_dir=?, description=? WHERE name=? AND (uninstall_cmd='' OR config_file='')`,
				ws.UninstallCmd, ws.ConfigFile, ws.BinaryPath, ws.DefaultPort, ws.LogDir, ws.Description, ws.Name)
		}
	}

	return nil
}

func (s *WebServerService) List() ([]model.WebServer, error) {
	rows, err := s.db.Query(`SELECT id, name, display_name, description, install_cmd, uninstall_cmd,
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

func (s *WebServerService) Get(id int64) (*model.WebServer, error) {
	ws := &model.WebServer{}
	err := s.db.QueryRow(`SELECT id, name, display_name, description, install_cmd, uninstall_cmd,
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

func (s *WebServerService) Create(ws *model.WebServer) error {
	result, err := s.db.Exec(`INSERT INTO web_servers (name, display_name, description, install_cmd, uninstall_cmd, config_path, config_file, sites_available, sites_enabled, service_name, binary_path, default_port, log_dir)
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

func (s *WebServerService) Delete(id int64) error {
	ws, err := s.Get(id)
	if err != nil {
		return err
	}
	if ws == nil {
		return fmt.Errorf("web server not found")
	}

	var count int
	s.db.QueryRow("SELECT COUNT(*) FROM websites WHERE web_server_id = ?", id).Scan(&count)
	if count > 0 {
		return fmt.Errorf("cannot delete: %d websites are using this server", count)
	}

	_, err = s.db.Exec("DELETE FROM web_servers WHERE id = ?", id)
	return err
}

// Install installs the web server software
func (s *WebServerService) Install(id int64) error {
	ws, err := s.Get(id)
	if err != nil {
		return err
	}
	if ws == nil {
		return fmt.Errorf("web server not found")
	}
	if ws.InstallCmd == "" {
		return fmt.Errorf("no install command configured")
	}

	exec.Command("apt-get", "update", "-y").Run()

	parts := strings.Fields(ws.InstallCmd)
	cmd := exec.Command(parts[0], parts[1:]...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("install failed: %s", string(out))
	}

	if ws.ServiceName != "" {
		exec.Command("systemctl", "enable", ws.ServiceName).Run()
		exec.Command("systemctl", "start", ws.ServiceName).Run()
	}

	s.RefreshStatus(id)
	return nil
}

// Uninstall removes the web server software
func (s *WebServerService) Uninstall(id int64) error {
	ws, err := s.Get(id)
	if err != nil {
		return err
	}
	if ws == nil {
		return fmt.Errorf("web server not found")
	}

	var count int
	s.db.QueryRow("SELECT COUNT(*) FROM websites WHERE web_server_id = ?", id).Scan(&count)
	if count > 0 {
		return fmt.Errorf("cannot uninstall: %d websites are using this server", count)
	}

	if ws.ServiceName != "" {
		exec.Command("systemctl", "stop", ws.ServiceName).Run()
		exec.Command("systemctl", "disable", ws.ServiceName).Run()
	}

	uninstallCmd := ws.UninstallCmd
	if uninstallCmd == "" {
		uninstallCmd = fmt.Sprintf("apt-get remove -y %s", ws.Name)
	}
	parts := strings.Fields(uninstallCmd)
	out, err := exec.Command(parts[0], parts[1:]...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("uninstall failed: %s", string(out))
	}

	s.db.Exec("UPDATE web_servers SET status = 'not_installed', version = '' WHERE id = ?", id)
	return nil
}

// Start starts the web server service
func (s *WebServerService) Start(id int64) error {
	ws, err := s.Get(id)
	if err != nil {
		return err
	}
	if ws == nil {
		return fmt.Errorf("web server not found")
	}
	if ws.ServiceName == "" {
		return fmt.Errorf("no service name configured")
	}

	out, err := exec.Command("systemctl", "start", ws.ServiceName).CombinedOutput()
	if err != nil {
		return fmt.Errorf("start failed: %s", string(out))
	}

	s.db.Exec("UPDATE web_servers SET status = 'running' WHERE id = ?", id)
	return nil
}

// Stop stops the web server service
func (s *WebServerService) Stop(id int64) error {
	ws, err := s.Get(id)
	if err != nil {
		return err
	}
	if ws == nil {
		return fmt.Errorf("web server not found")
	}
	if ws.ServiceName == "" {
		return fmt.Errorf("no service name configured")
	}

	out, err := exec.Command("systemctl", "stop", ws.ServiceName).CombinedOutput()
	if err != nil {
		return fmt.Errorf("stop failed: %s", string(out))
	}

	s.db.Exec("UPDATE web_servers SET status = 'stopped' WHERE id = ?", id)
	return nil
}

// Restart restarts the web server service
func (s *WebServerService) Restart(id int64) error {
	ws, err := s.Get(id)
	if err != nil {
		return err
	}
	if ws == nil {
		return fmt.Errorf("web server not found")
	}
	if ws.ServiceName == "" {
		return fmt.Errorf("no service name configured")
	}

	out, err := exec.Command("systemctl", "restart", ws.ServiceName).CombinedOutput()
	if err != nil {
		return fmt.Errorf("restart failed: %s", string(out))
	}

	s.db.Exec("UPDATE web_servers SET status = 'running' WHERE id = ?", id)
	return nil
}

// Reload reloads the web server config without restarting
func (s *WebServerService) Reload(id int64) error {
	ws, err := s.Get(id)
	if err != nil {
		return err
	}
	if ws == nil {
		return fmt.Errorf("web server not found")
	}

	// Test config first
	if ok, _ := s.TestConfig(id); !ok {
		return fmt.Errorf("config test failed, reload aborted")
	}

	if ws.ServiceName != "" {
		out, err := exec.Command("systemctl", "reload", ws.ServiceName).CombinedOutput()
		if err != nil {
			return fmt.Errorf("reload failed: %s", string(out))
		}
	}
	return nil
}

// TestConfig tests the web server configuration
func (s *WebServerService) TestConfig(id int64) (bool, string) {
	ws, err := s.Get(id)
	if err != nil || ws == nil {
		return false, "web server not found"
	}

	switch ws.Name {
	case "nginx":
		out, err := exec.Command("nginx", "-t").CombinedOutput()
		msg := strings.TrimSpace(string(out))
		if err != nil {
			return false, msg
		}
		return true, msg
	case "apache":
		out, err := exec.Command("apache2ctl", "configtest").CombinedOutput()
		msg := strings.TrimSpace(string(out))
		if err != nil {
			return false, msg
		}
		return true, msg
	case "caddy":
		out, err := exec.Command("caddy", "validate", "--config", ws.ConfigFile).CombinedOutput()
		msg := strings.TrimSpace(string(out))
		if err != nil {
			return false, msg
		}
		return true, msg
	default:
		return true, "no config test available"
	}
}

// GetConfig reads the main config file content
func (s *WebServerService) GetConfig(id int64) (string, error) {
	ws, err := s.Get(id)
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
func (s *WebServerService) SaveConfig(id int64, content string) error {
	ws, err := s.Get(id)
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
func (s *WebServerService) GetServiceLogs(id int64, lines int) (string, error) {
	ws, err := s.Get(id)
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

	out, err := exec.Command("journalctl", "-u", ws.ServiceName, "-n", strconv.Itoa(lines), "--no-pager").CombinedOutput()
	if err != nil {
		return string(out), nil
	}
	return string(out), nil
}

// SetAutoStart enables/disables auto-start on boot
func (s *WebServerService) SetAutoStart(id int64, enabled bool) error {
	ws, err := s.Get(id)
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

	out, err := exec.Command("systemctl", action, ws.ServiceName).CombinedOutput()
	if err != nil {
		return fmt.Errorf("systemctl %s failed: %s", action, string(out))
	}
	return nil
}

// RefreshStatus detects full runtime state
func (s *WebServerService) RefreshStatus(id int64) error {
	ws, err := s.Get(id)
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
			out, _ := exec.Command("nginx", "-v").CombinedOutput()
			version = strings.TrimSpace(string(out))
		}
	case "apache":
		if _, err := exec.LookPath("apache2"); err == nil {
			installed = true
			out, _ := exec.Command("apache2", "-v").CombinedOutput()
			lines := strings.Split(strings.TrimSpace(string(out)), "\n")
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
			out, _ := exec.Command("caddy", "version").CombinedOutput()
			version = strings.TrimSpace(string(out))
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
			out, _ := exec.Command("systemctl", "is-active", ws.ServiceName).CombinedOutput()
			if strings.TrimSpace(string(out)) == "active" {
				status = "running"
			}
		}
	}

	s.db.Exec("UPDATE web_servers SET status = ?, version = ? WHERE id = ?", status, version, id)
	return nil
}

// RefreshAllStatus refreshes status for all web servers
func (s *WebServerService) RefreshAllStatus() {
	servers, _ := s.List()
	for _, ws := range servers {
		s.RefreshStatus(ws.ID)
	}
}

// GetConnections returns active connection count (for Nginx)
func (s *WebServerService) GetConnections(id int64) (int, error) {
	ws, err := s.Get(id)
	if err != nil {
		return 0, err
	}
	if ws == nil {
		return 0, fmt.Errorf("web server not found")
	}

	// Count connections from ss
	out, _ := exec.Command("ss", "-tlnp").CombinedOutput()
	count := 0
	for _, line := range strings.Split(string(out), "\n") {
		if ws.ServiceName != "" && strings.Contains(line, ws.ServiceName) {
			count++
		}
	}
	return count, nil
}

// GetProcessInfo returns PID, memory, uptime for the service
func (s *WebServerService) GetProcessInfo(id int64) (pid int, memBytes int64, uptime string, err error) {
	ws, e := s.Get(id)
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
	out, e := exec.Command("systemctl", "show", ws.ServiceName, "--property=MainPID,ActiveEnterTimestamp").CombinedOutput()
	if e != nil {
		return 0, 0, "", nil
	}

	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
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

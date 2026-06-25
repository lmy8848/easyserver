package api

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

type SystemHandler struct{}

func NewSystemHandler() *SystemHandler {
	return &SystemHandler{}
}

// SSHLogin represents an SSH login record
type SSHLogin struct {
	Username string `json:"username"`
	IP       string `json:"ip"`
	Time     string `json:"time"`
	Type     string `json:"type"` // login, logout, failed
	Terminal string `json:"terminal"`
}

// GetSSHLogins returns recent SSH login history
func (h *SystemHandler) GetSSHLogins(c *gin.Context) {
	limit := 100
	if l, err := strconv.Atoi(c.Query("limit")); err == nil && l > 0 && l <= 500 {
		limit = l
	}

	// Try to use `last` command first (more reliable)
	logins, err := getLastLogins(limit)
	if err != nil || len(logins) == 0 {
		// Fallback to parsing /var/log/auth.log
		logins, err = getAuthLogins(limit)
		if err != nil {
			InternalError(c, err.Error())
			return
		}
	}

	Success(c, logins)
}

// getLastLogins uses the `last` command to get login history
func getLastLogins(limit int) ([]SSHLogin, error) {
	cmd := exec.Command("last", "-n", fmt.Sprintf("%d", limit), "-F", "-i")
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	var logins []SSHLogin
	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" || strings.HasPrefix(line, "wtmp") || strings.HasPrefix(line, "reboot") {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < 10 {
			continue
		}

		username := fields[0]
		terminal := fields[1]
		ip := fields[2]
		loginTime := strings.Join(fields[3:8], " ")

		// Determine login type
		loginType := "login"
		if strings.Contains(line, "still logged in") {
			loginType = "active"
		} else if terminal == ":0" || terminal == "console" {
			loginType = "console"
		}

		// Filter out non-SSH logins (optional)
		if terminal != "pts/" && !strings.HasPrefix(terminal, "pts/") {
			if terminal != "ssh" && !strings.Contains(ip, ".") {
				continue
			}
		}

		logins = append(logins, SSHLogin{
			Username: username,
			IP:       ip,
			Time:     loginTime,
			Type:     loginType,
			Terminal: terminal,
		})
	}

	return logins, nil
}

// getAuthLogins parses /var/log/auth.log for SSH logins using a bounded ring buffer.
func getAuthLogins(limit int) ([]SSHLogin, error) {
	authLog := "/var/log/auth.log"
	if _, err := os.Stat(authLog); os.IsNotExist(err) {
		// Try alternatives
		authLog = "/var/log/secure"
		if _, err := os.Stat(authLog); os.IsNotExist(err) {
			return nil, fmt.Errorf("no auth log found")
		}
	}

	file, err := os.Open(authLog)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	// Use a ring buffer to keep only the last `maxLines` lines in memory
	const maxLines = 1000
	lines := make([]string, 0, maxLines)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
		if len(lines) > maxLines {
			lines = lines[1:]
		}
	}

	var logins []SSHLogin
	for _, line := range lines {
		if !strings.Contains(line, "sshd") {
			continue
		}

		login := parseAuthLogLine(line)
		if login != nil {
			logins = append(logins, *login)
		}
	}

	// Return last N entries
	if len(logins) > limit {
		logins = logins[len(logins)-limit:]
	}

	return logins, nil
}

// parseAuthLogLine parses a single auth.log line for SSH events
func parseAuthLogLine(line string) *SSHLogin {
	// Example lines:
	// Jun 15 10:30:45 server sshd[12345]: Accepted password for user from 192.168.1.1 port 12345 ssh2
	// Jun 15 10:30:45 server sshd[12345]: Failed password for user from 192.168.1.1 port 12345 ssh2
	// Jun 15 10:30:45 server sshd[12345]: Accepted publickey for user from 192.168.1.1 port 12345 ssh2

	if !strings.Contains(line, "Accepted") && !strings.Contains(line, "Failed") {
		return nil
	}

	login := &SSHLogin{}

	// Parse time (first 15 chars: "Jun 15 10:30:45")
	if len(line) > 15 {
		login.Time = line[:15]
		// Add current year
		login.Time = login.Time + " " + time.Now().Format("2006")
	}

	// Parse type
	if strings.Contains(line, "Accepted") {
		login.Type = "login"
	} else if strings.Contains(line, "Failed") {
		login.Type = "failed"
	}

	// Parse username
	if idx := strings.Index(line, "for "); idx != -1 {
		rest := line[idx+4:]
		if endIdx := strings.Index(rest, " from "); endIdx != -1 {
			login.Username = rest[:endIdx]
		}
	}

	// Parse IP
	if idx := strings.Index(line, "from "); idx != -1 {
		rest := line[idx+5:]
		if endIdx := strings.Index(rest, " "); endIdx != -1 {
			login.IP = rest[:endIdx]
		}
	}

	login.Terminal = "ssh"

	return login
}

// GetSystemSSHConfig returns SSH server configuration
func (h *SystemHandler) GetSystemSSHConfig(c *gin.Context) {
	port := 22
	permitRootLogin := "yes"
	passwordAuth := "yes"
	status := "unknown"

	// Read SSH config
	if data, err := os.ReadFile("/etc/ssh/sshd_config"); err == nil {
		scanner := bufio.NewScanner(strings.NewReader(string(data)))
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if strings.HasPrefix(line, "Port ") {
				fmt.Sscanf(line, "Port %d", &port)
			}
			if strings.HasPrefix(line, "PermitRootLogin ") {
				permitRootLogin = strings.TrimPrefix(line, "PermitRootLogin ")
			}
			if strings.HasPrefix(line, "PasswordAuthentication ") {
				passwordAuth = strings.TrimPrefix(line, "PasswordAuthentication ")
			}
		}
	}

	// Check if SSH service is running
	cmd := exec.Command("systemctl", "is-active", "ssh")
	output, err := cmd.Output()
	if err == nil {
		status = strings.TrimSpace(string(output))
	}

	Success(c, gin.H{
		"port":                    port,
		"permit_root_login":       permitRootLogin,
		"password_authentication": passwordAuth,
		"status":                  status,
	})
}

// CheckPort checks if a port is available
func (h *SystemHandler) CheckPort(c *gin.Context) {
	portStr := c.Query("port")
	if portStr == "" {
		BadRequest(c, "端口不能为空")
		return
	}

	port, err := strconv.Atoi(portStr)
	if err != nil || port < 1 || port > 65535 {
		BadRequest(c, "无效的端口号 (1-65535)")
		return
	}

	// Try to listen on the port
	addr := fmt.Sprintf(":%d", port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		// Port is in use - try to find what's using it
		processInfo := getPortProcess(port)
		Success(c, gin.H{
			"available": false,
			"port":      port,
			"process":   processInfo,
			"message":   fmt.Sprintf("端口 %d 已被占用", port),
		})
		return
	}
	listener.Close()

	Success(c, gin.H{
		"available": true,
		"port":      port,
		"message":   fmt.Sprintf("端口 %d 可用", port),
	})
}

// getPortProcess finds the process using a given port
func getPortProcess(port int) string {
	// Try ss first
	out, err := exec.Command("ss", "-tlnp", fmt.Sprintf("sport = :%d", port)).CombinedOutput()
	if err == nil {
		lines := strings.Split(strings.TrimSpace(string(out)), "\n")
		for _, line := range lines[1:] { // skip header
			if strings.Contains(line, fmt.Sprintf(":%d", port)) {
				// Extract process info
				if idx := strings.Index(line, "users:((\""); idx != -1 {
					end := strings.Index(line[idx:], "\"")
					if end > 0 {
						return line[idx+9 : idx+end]
					}
				}
				return strings.TrimSpace(line)
			}
		}
	}

	// Fallback to netstat
	out, err = exec.Command("netstat", "-tlnp").CombinedOutput()
	if err == nil {
		for _, line := range strings.Split(string(out), "\n") {
			if strings.Contains(line, fmt.Sprintf(":%d ", port)) || strings.Contains(line, fmt.Sprintf(":%d\t", port)) {
				return strings.TrimSpace(line)
			}
		}
	}

	return "unknown"
}

// CheckPorts checks multiple ports at once
func (h *SystemHandler) CheckPorts(c *gin.Context) {
	portsStr := c.Query("ports") // comma-separated: "80,443,3306"
	if portsStr == "" {
		BadRequest(c, "端口列表不能为空")
		return
	}

	var results []map[string]interface{}
	for _, p := range strings.Split(portsStr, ",") {
		p = strings.TrimSpace(p)
		port, err := strconv.Atoi(p)
		if err != nil || port < 1 || port > 65535 {
			results = append(results, map[string]interface{}{
				"port": p, "available": false, "message": "无效端口",
			})
			continue
		}

		addr := fmt.Sprintf(":%d", port)
		listener, err := net.Listen("tcp", addr)
		if err != nil {
			results = append(results, map[string]interface{}{
				"port": port, "available": false, "message": fmt.Sprintf("端口 %d 已被占用", port),
			})
			continue
		}
		listener.Close()
		results = append(results, map[string]interface{}{
			"port": port, "available": true, "message": fmt.Sprintf("端口 %d 可用", port),
		})
	}

	Success(c, results)
}

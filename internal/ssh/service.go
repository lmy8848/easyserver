package ssh

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"

	"easyserver/internal/infra/executor"
)

const sshdConfigPath = "/etc/ssh/sshd_config"

// Service manages SSH server configuration.
type Service struct {
	configPath string
	executor   executor.CommandExecutor
}

// NewService creates a new SSH service.
func NewService(exec executor.CommandExecutor) *Service {
	return &Service{
		configPath: sshdConfigPath,
		executor:   exec,
	}
}

// GetConfig parses and returns the current SSH configuration.
func (s *Service) GetConfig() (*Config, error) {
	config := &Config{
		Port:                   22,
		PermitRootLogin:        "yes",
		PasswordAuthentication: "yes",
		PubkeyAuthentication:   "yes",
		MaxAuthTries:           6,
		LoginGraceTime:         120,
		ClientAliveInterval:    0,
		ClientAliveCountMax:    3,
	}

	file, err := os.Open(s.configPath)
	if err != nil {
		return config, nil // Return defaults if file doesn't exist
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.SplitN(line, " ", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		// Case-insensitive key matching (sshd_config is case-insensitive)
		switch strings.ToLower(key) {
		case "port":
			if port, err := strconv.Atoi(value); err == nil {
				config.Port = port
			}
		case "permitrootlogin":
			config.PermitRootLogin = value
		case "passwordauthentication":
			config.PasswordAuthentication = value
		case "pubkeyauthentication":
			config.PubkeyAuthentication = value
		case "maxauthtries":
			if n, err := strconv.Atoi(value); err == nil {
				config.MaxAuthTries = n
			}
		case "logingracetime":
			if n, err := strconv.Atoi(value); err == nil {
				config.LoginGraceTime = n
			}
		case "clientaliveinterval":
			if n, err := strconv.Atoi(value); err == nil {
				config.ClientAliveInterval = n
			}
		case "clientalivecountmax":
			if n, err := strconv.Atoi(value); err == nil {
				config.ClientAliveCountMax = n
			}
		case "allowusers":
			config.AllowUsers = value
		case "denyusers":
			config.DenyUsers = value
		}
	}

	return config, nil
}

// SaveConfig saves the SSH configuration.
func (s *Service) SaveConfig(config *Config) error {
	// Backup original file
	backupPath := s.configPath + ".bak"
	if err := copyFile(s.configPath, backupPath); err != nil {
		log.Printf("ssh: backup failed: %v", err)
	}

	// Read original file to preserve comments and structure
	originalLines := readLines(s.configPath)

	// Build new config lines
	var newLines []string
	updated := make(map[string]bool)

	for _, line := range originalLines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			newLines = append(newLines, line)
			continue
		}

		parts := strings.SplitN(trimmed, " ", 2)
		if len(parts) != 2 {
			newLines = append(newLines, line)
			continue
		}

		key := strings.TrimSpace(parts[0])
		switch key {
		case "Port":
			newLines = append(newLines, fmt.Sprintf("Port %d", config.Port))
			updated["Port"] = true
		case "PermitRootLogin":
			newLines = append(newLines, fmt.Sprintf("PermitRootLogin %s", config.PermitRootLogin))
			updated["PermitRootLogin"] = true
		case "PasswordAuthentication":
			newLines = append(newLines, fmt.Sprintf("PasswordAuthentication %s", config.PasswordAuthentication))
			updated["PasswordAuthentication"] = true
		case "PubkeyAuthentication":
			newLines = append(newLines, fmt.Sprintf("PubkeyAuthentication %s", config.PubkeyAuthentication))
			updated["PubkeyAuthentication"] = true
		case "MaxAuthTries":
			newLines = append(newLines, fmt.Sprintf("MaxAuthTries %d", config.MaxAuthTries))
			updated["MaxAuthTries"] = true
		case "LoginGraceTime":
			newLines = append(newLines, fmt.Sprintf("LoginGraceTime %d", config.LoginGraceTime))
			updated["LoginGraceTime"] = true
		case "ClientAliveInterval":
			newLines = append(newLines, fmt.Sprintf("ClientAliveInterval %d", config.ClientAliveInterval))
			updated["ClientAliveInterval"] = true
		case "ClientAliveCountMax":
			newLines = append(newLines, fmt.Sprintf("ClientAliveCountMax %d", config.ClientAliveCountMax))
			updated["ClientAliveCountMax"] = true
		case "AllowUsers":
			if config.AllowUsers != "" {
				newLines = append(newLines, fmt.Sprintf("AllowUsers %s", config.AllowUsers))
			}
			updated["AllowUsers"] = true
		case "DenyUsers":
			if config.DenyUsers != "" {
				newLines = append(newLines, fmt.Sprintf("DenyUsers %s", config.DenyUsers))
			}
			updated["DenyUsers"] = true
		default:
			newLines = append(newLines, line)
		}
	}

	// Append new settings that weren't in the original file
	if !updated["Port"] {
		newLines = append(newLines, fmt.Sprintf("Port %d", config.Port))
	}
	if !updated["PermitRootLogin"] {
		newLines = append(newLines, fmt.Sprintf("PermitRootLogin %s", config.PermitRootLogin))
	}
	if !updated["PasswordAuthentication"] {
		newLines = append(newLines, fmt.Sprintf("PasswordAuthentication %s", config.PasswordAuthentication))
	}
	if !updated["PubkeyAuthentication"] {
		newLines = append(newLines, fmt.Sprintf("PubkeyAuthentication %s", config.PubkeyAuthentication))
	}

	// Write to temp file first
	tmpPath := s.configPath + ".tmp"
	if err := writeLines(tmpPath, newLines); err != nil {
		return fmt.Errorf("write config: %w", err)
	}

	// Replace original file
	if err := os.Rename(tmpPath, s.configPath); err != nil {
		return fmt.Errorf("replace config: %w", err)
	}

	log.Printf("ssh: config saved to %s", s.configPath)
	return nil
}

// TestConfig tests the SSH configuration.
func (s *Service) TestConfig(ctx context.Context) (string, error) {
	output, exitCode, err := s.executor.RunCombined(ctx, "sshd", "-t")
	if err != nil {
		return output, fmt.Errorf("config test failed: %w", err)
	}
	if exitCode != 0 {
		return output, fmt.Errorf("config test failed (exit code %d)", exitCode)
	}
	return "Configuration is valid", nil
}

// ReloadSSH reloads the SSH service.
func (s *Service) ReloadSSH(ctx context.Context) error {
	output, exitCode, err := s.executor.RunCombined(ctx, "systemctl", "reload", "sshd")
	if err != nil || exitCode != 0 {
		// Try ssh service name
		output2, exitCode2, err2 := s.executor.RunCombined(ctx, "systemctl", "reload", "ssh")
		if err2 != nil || exitCode2 != 0 {
			msg := output
			if msg == "" {
				msg = output2
			}
			return fmt.Errorf("reload failed: %s: %v", msg, coalesceErr(err, err2))
		}
	}
	log.Printf("ssh: service reloaded")
	return nil
}

// GetSessions returns active SSH sessions.
func (s *Service) GetSessions(ctx context.Context) ([]Session, error) {
	var sessions []Session
	seenPIDs := make(map[int]bool)

	// Method 1: Use `who -u` for interactive sessions (with TTY)
	whoOut, _, whoExit, whoErr := s.executor.Run(ctx, "who", "-u")
	if whoErr == nil && whoExit == 0 {
		scanner := bufio.NewScanner(strings.NewReader(whoOut))
		for scanner.Scan() {
			line := scanner.Text()
			session := parseWhoLine(line)
			if session != nil {
				sessions = append(sessions, *session)
				seenPIDs[session.PID] = true
			}
		}
	}

	// Method 2: Use `ss` to detect all SSH connections (including non-interactive)
	sshPort := 22
	if cfg, err := s.GetConfig(); err == nil && cfg.Port > 0 {
		sshPort = cfg.Port
	}
	sshPortStr := fmt.Sprintf(":%d ", sshPort)
	sshPortTab := fmt.Sprintf(":%d\t", sshPort)

	ssOut, _, ssExit, ssErr := s.executor.Run(ctx, "ss", "-tnp")
	if ssErr == nil && ssExit == 0 {
		scanner := bufio.NewScanner(strings.NewReader(ssOut))
		for scanner.Scan() {
			line := scanner.Text()
			if !strings.Contains(line, sshPortStr) && !strings.Contains(line, sshPortTab) {
				continue
			}
			if strings.HasPrefix(line, "State") || strings.HasPrefix(line, "Netid") {
				continue
			}

			session := parseSSLine(line)
			if session != nil && !seenPIDs[session.PID] {
				sessions = append(sessions, *session)
				seenPIDs[session.PID] = true
			}
		}
	}

	// Method 3: Use `ps` to find sshd processes with user sessions
	psOut, _, psExit, psErr := s.executor.Run(ctx, "ps", "aux")
	if psErr == nil && psExit == 0 {
		scanner := bufio.NewScanner(strings.NewReader(psOut))
		for scanner.Scan() {
			line := scanner.Text()
			if !strings.Contains(line, "sshd:") || strings.Contains(line, "grep") {
				continue
			}
			if strings.Contains(line, "/usr/sbin/sshd") || strings.Contains(line, "-D") {
				continue
			}

			session := parsePSLine(line)
			if session != nil && !seenPIDs[session.PID] {
				sessions = append(sessions, *session)
				seenPIDs[session.PID] = true
			}
		}
	}

	return sessions, nil
}

// KillSession kills an SSH session by PID.
func (s *Service) KillSession(ctx context.Context, pid int) error {
	output, exitCode, err := s.executor.RunCombined(ctx, "kill", strconv.Itoa(pid))
	if err != nil {
		return fmt.Errorf("kill failed: %s: %w", output, err)
	}
	if exitCode != 0 {
		return fmt.Errorf("kill failed: %s (exit code %d)", output, exitCode)
	}
	log.Printf("ssh: killed session %d", pid)
	return nil
}

// GetLoginHistory returns recent SSH login attempts.
func (s *Service) GetLoginHistory(ctx context.Context, limit int) ([]LoginRecord, error) {
	// Try journalctl first
	stdout, _, exitCode, err := s.executor.Run(ctx, "journalctl", "-u", "sshd", "-u", "ssh", "--no-pager", "-n", strconv.Itoa(limit), "--output=short-iso")
	if err != nil || exitCode != 0 {
		// Fallback to /var/log/auth.log
		return s.getLoginHistoryFromAuthLog(limit)
	}

	var records []LoginRecord
	scanner := bufio.NewScanner(strings.NewReader(stdout))
	for scanner.Scan() {
		line := scanner.Text()
		record := parseSSHLogLine(line)
		if record != nil {
			records = append(records, *record)
		}
	}

	return records, nil
}

func (s *Service) getLoginHistoryFromAuthLog(limit int) ([]LoginRecord, error) {
	logPaths := []string{"/var/log/auth.log", "/var/log/secure"}
	var file *os.File
	for _, path := range logPaths {
		var err error
		file, err = os.Open(path)
		if err == nil {
			break
		}
	}
	if file == nil {
		return nil, fmt.Errorf("no SSH log file found (tried auth.log and secure)")
	}
	defer file.Close()

	var records []LoginRecord
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.Contains(line, "sshd") && (strings.Contains(line, "Accepted") || strings.Contains(line, "Failed")) {
			record := parseSSHLogLine(line)
			if record != nil {
				records = append(records, *record)
			}
		}
	}

	if len(records) > limit {
		records = records[len(records)-limit:]
	}

	return records, nil
}

// --- Parsing helpers ---

func parseSSLine(line string) *Session {
	fields := strings.Fields(line)
	if len(fields) < 6 {
		return nil
	}

	remoteAddr := fields[4]
	remoteIP := remoteAddr
	if idx := strings.LastIndex(remoteAddr, ":"); idx > 0 {
		remoteIP = remoteAddr[:idx]
	}

	pid := 0
	pidIdx := strings.Index(line, "pid=")
	if pidIdx > 0 {
		pidStr := line[pidIdx+3:]
		if endIdx := strings.Index(pidStr, ","); endIdx > 0 {
			pidStr = pidStr[:endIdx]
		}
		if p, err := strconv.Atoi(pidStr); err == nil {
			pid = p
		}
	}

	if pid == 0 {
		return nil
	}

	sessionType := "ssh"
	tty := "notty"
	if strings.Contains(line, "@pts/") {
		tty = "pts/?"
		sessionType = "interactive"
	}

	return &Session{
		User: "root",
		TTY:  tty,
		PID:  pid,
		From: remoteIP,
		Type: sessionType,
	}
}

func parsePSLine(line string) *Session {
	fields := strings.Fields(line)
	if len(fields) < 11 {
		return nil
	}

	user := fields[0]
	pidStr := fields[1]

	sshdPart := ""
	for i, f := range fields {
		if f == "sshd:" && i+1 < len(fields) {
			sshdPart = fields[i+1]
			break
		}
	}

	if sshdPart == "" {
		return nil
	}

	tty := "notty"
	sessionUser := user
	if atIdx := strings.Index(sshdPart, "@"); atIdx > 0 {
		sessionUser = sshdPart[:atIdx]
		tty = sshdPart[atIdx+1:]
	}

	pid := 0
	if p, err := strconv.Atoi(pidStr); err == nil {
		pid = p
	}

	if pid == 0 {
		return nil
	}

	loginTime := fields[8]

	sessionType := "ssh"
	if tty == "notty" {
		sessionType = "non-interactive"
	} else {
		sessionType = "interactive"
	}

	return &Session{
		User:      sessionUser,
		TTY:       tty,
		PID:       pid,
		LoginTime: loginTime,
		Type:      sessionType,
	}
}

func parseWhoLine(line string) *Session {
	fields := strings.Fields(line)
	if len(fields) < 3 {
		return nil
	}

	fromIdx := -1
	for i, f := range fields {
		if strings.HasPrefix(f, "(") && strings.HasSuffix(f, ")") {
			fromIdx = i
			break
		}
	}

	if fromIdx < 0 {
		return nil
	}

	pidIdx := fromIdx - 1
	if pidIdx < 2 {
		return nil
	}

	pid := 0
	if p, err := strconv.Atoi(fields[pidIdx]); err == nil {
		pid = p
	}

	from := strings.TrimPrefix(fields[fromIdx], "(")
	from = strings.TrimSuffix(from, ")")

	session := &Session{
		User: fields[0],
		TTY:  fields[1],
		PID:  pid,
		From: from,
	}

	if pidIdx >= 3 {
		session.LoginTime = strings.Join(fields[2:pidIdx-1], " ")
	}

	return session
}

func parseSSHLogLine(line string) *LoginRecord {
	record := &LoginRecord{}

	if strings.Contains(line, "Accepted") {
		record.Status = "success"
	} else if strings.Contains(line, "Failed") || strings.Contains(line, "Invalid") {
		record.Status = "failed"
	} else {
		return nil
	}

	if idx := strings.Index(line, "from "); idx >= 0 {
		rest := line[idx+5:]
		if endIdx := strings.Index(rest, " "); endIdx >= 0 {
			record.IP = rest[:endIdx]
		}
	}

	if idx := strings.Index(line, "port "); idx >= 0 {
		rest := line[idx+5:]
		if endIdx := strings.Index(rest, " "); endIdx >= 0 {
			if port, err := strconv.Atoi(rest[:endIdx]); err == nil {
				record.Port = port
			}
		}
	}

	if idx := strings.Index(line, "for "); idx >= 0 {
		rest := line[idx+4:]
		if endIdx := strings.Index(rest, " "); endIdx >= 0 {
			record.User = rest[:endIdx]
		}
	}

	if strings.Contains(line, "password") {
		record.Method = "password"
	} else if strings.Contains(line, "publickey") {
		record.Method = "publickey"
	}

	// Extract full ISO timestamp (first field before space)
	if idx := strings.Index(line, " "); idx >= 0 {
		record.Time = line[:idx]
	}

	return record
}

// --- File helpers ---

func coalesceErr(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}

func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0644)
}

func readLines(path string) []string {
	file, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer file.Close()

	var lines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	return lines
}

func writeLines(path string, lines []string) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	writer := bufio.NewWriter(file)
	for _, line := range lines {
		fmt.Fprintln(writer, line)
	}
	return writer.Flush()
}

package ssh

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"

	infrasystemd "easyserver/internal/infra/systemd"
	"easyserver/internal/util"
)

const (
	sshdConfigPath = "/etc/ssh/sshd_config"
	// sshdDropInPath 是 EasyServer 托管的 drop-in 配置文件。文件名必须以 .conf
	// 结尾才会被主配置中的 `Include /etc/ssh/sshd_config.d/*.conf` 加载
	// （Debian/Ubuntu 默认有这一行），这样保存配置不再改写主配置文件。
	// `99-` 前缀保证按字典序最后处理（sshd 关键字"先到先得"），覆盖其他
	// drop-in（如 50-cloud-init.conf）的同名指令。
	sshdDropInDir  = "/etc/ssh/sshd_config.d"
	sshdDropInPath = sshdDropInDir + "/99-easyserver.conf"
)

// Service manages SSH server configuration.
type Service struct{}

// NewService creates a new SSH service.
func NewService() *Service {
	return &Service{}
}

// sshdBinary returns the sshd path, falling back to the standard location when
// it's not on PATH (普通用户/受限环境的 PATH 通常不含 /usr/sbin)。
func sshdBinary() string {
	if p, err := exec.LookPath("sshd"); err == nil {
		return p
	}
	return "/usr/sbin/sshd"
}

// CheckStatus reports whether the SSH server is usable (sshd binary present),
// and whether the sshd service is currently running.
// 前端据此决定是否渲染 SSH 管理操作，避免不可用时每个操作逐个报错。
func (s *Service) CheckStatus(ctx context.Context) (map[string]any, error) {
	if _, err := os.Stat(sshdBinary()); err != nil {
		//nolint:nilerr // 不可用是正常业务结果（前端展示占位），不是错误。
		return map[string]any{
			"available": false,
			"reason":    "未检测到 sshd（OpenSSH Server）：可能未安装，或系统使用其他 SSH 实现（如 dropbear）",
		}, nil
	}
	// 服务状态：先试 sshd 再试 ssh（Debian 的服务名是 ssh）。
	running := false
	for _, unit := range []string{"sshd", "ssh"} {
		if util.SystemdUnitActive(ctx, unit) {
			running = true
			break
		}
	}
	return map[string]any{
		"available": true,
		"reason":    "",
		"running":   running,
	}, nil
}

// GetConfig returns the effective SSH configuration parsed from `sshd -T`
// (Include 展开、"先到先得"合并后的最终生效配置)。失败即报错
func (s *Service) GetConfig(ctx context.Context) (*Config, error) {
	out, err := exec.CommandContext(ctx, sshdBinary(), "-T").CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("sshd -T 失败: %w: %s", err, strings.TrimSpace(string(out)))
	}
	// sshd -T 输出完整生效配置（含默认值），解析即最终值，无需预置默认。
	config := &Config{}
	s.applyLines(config, strings.Split(string(out), "\n"))
	return config, nil
}

// applyLines parses sshd config directives (`sshd -T` output, keys are matched
// case-insensitively) into config.
func (s *Service) applyLines(config *Config, lines []string) {
	for _, text := range lines {
		line := strings.TrimSpace(text)
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
}

// hasSSHDIncludeDir reports whether the main config includes the sshd_config.d
// directory (Debian/Ubuntu 默认第一行 `Include /etc/ssh/sshd_config.d/*.conf`)。
// 未 Include 时 drop-in 不会生效——保存直接报错，由用户手动补上 Include，
// 面板不猜测、不静默写主配置。
func hasSSHDIncludeDir() bool {
	for _, line := range readLines(sshdConfigPath) {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if after, ok := strings.CutPrefix(trimmed, "Include"); ok {
			if strings.Contains(after, "sshd_config.d") && strings.Contains(after, ".conf") {
				return true
			}
		}
	}
	return false
}

// SaveConfig saves the SSH configuration to the EasyServer drop-in
// （/etc/ssh/sshd_config.d/99-easyserver.conf），不动主配置文件。
func (s *Service) SaveConfig(config *Config) error {
	if !hasSSHDIncludeDir() {
		return fmt.Errorf("主配置未 Include %s，drop-in 不会生效；请在 /etc/ssh/sshd_config 中加入 `Include %s/*.conf` 后重试", sshdDropInDir, sshdDropInDir)
	}
	if err := os.MkdirAll(sshdDropInDir, 0755); err != nil {
		return fmt.Errorf("create drop-in dir: %w", err)
	}
	return s.saveDropIn(sshdDropInPath, config)
}

// saveDropIn writes the managed settings as a standalone drop-in file
// (atomic tmp+rename). 整文件重写——文件完全由 EasyServer 生成，无需保留结构。
func (s *Service) saveDropIn(path string, config *Config) error {
	// Backup previous drop-in so Harden 测试失败时可回滚。
	if err := copyFile(path, path+".bak"); err != nil {
		log.Printf("ssh: backup failed: %v", err)
	}

	lines := []string{
		"# EasyServer 托管的 SSH 配置。请勿手动编辑——此文件会被整文件覆盖。",
		"",
		fmt.Sprintf("Port %d", config.Port),
		"PermitRootLogin " + config.PermitRootLogin,
		"PasswordAuthentication " + config.PasswordAuthentication,
		"PubkeyAuthentication " + config.PubkeyAuthentication,
		fmt.Sprintf("MaxAuthTries %d", config.MaxAuthTries),
		fmt.Sprintf("LoginGraceTime %d", config.LoginGraceTime),
		fmt.Sprintf("ClientAliveInterval %d", config.ClientAliveInterval),
		fmt.Sprintf("ClientAliveCountMax %d", config.ClientAliveCountMax),
	}
	// ponytail: sshd 无"清除指令"语法，AllowUsers/DenyUsers 置空时只能不写行；
	// 若主配置或其他 drop-in 已有该指令，清空后仍需手动改主配置。
	if config.AllowUsers != "" {
		lines = append(lines, "AllowUsers "+config.AllowUsers)
	}
	if config.DenyUsers != "" {
		lines = append(lines, "DenyUsers "+config.DenyUsers)
	}

	tmpPath := path + ".tmp"
	if err := writeLines(tmpPath, lines); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("replace config: %w", err)
	}
	log.Printf("ssh: config saved to %s", path)
	return nil
}

// TestConfig tests the SSH configuration.
func (s *Service) TestConfig(ctx context.Context) (string, error) {
	output, err := exec.CommandContext(ctx, sshdBinary(), "-t").CombinedOutput()
	if err != nil {
		return string(output), fmt.Errorf("config test failed: %w", err)
	}
	return "Configuration is valid", nil
}

// ReloadSSH reloads the SSH service.
func (s *Service) ReloadSSH(ctx context.Context) error {
	client := infrasystemd.DefaultClient()
	_, sshdErr := client.ReloadUnitContext(ctx, "sshd.service", "replace")
	if sshdErr == nil {
		return nil
	}
	_, sshErr := client.ReloadUnitContext(ctx, "ssh.service", "replace")
	if sshErr == nil {
		return nil
	}
	// Both attempts failed, return wrapped error preserving underlying causes
	return fmt.Errorf("reload SSH failed: sshd.service error: %w; ssh.service error: %w", sshdErr, sshErr)
}

// GetSessions returns active SSH sessions.
func (s *Service) GetSessions(ctx context.Context) ([]Session, error) {
	var sessions []Session
	seenPIDs := make(map[int]bool)

	// Method 1: Use `who -u` for interactive sessions (with TTY)
	whoOut, whoErr := exec.CommandContext(ctx, "who", "-u").Output()
	if whoErr == nil {
		scanner := bufio.NewScanner(strings.NewReader(string(whoOut)))
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
	if cfg, err := s.GetConfig(ctx); err == nil && cfg.Port > 0 {
		sshPort = cfg.Port
	}
	sshPortStr := fmt.Sprintf(":%d ", sshPort)
	sshPortTab := fmt.Sprintf(":%d\t", sshPort)

	ssOut, ssErr := exec.CommandContext(ctx, "ss", "-tnp").Output()
	if ssErr == nil {
		scanner := bufio.NewScanner(strings.NewReader(string(ssOut)))
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
	psOut, psErr := exec.CommandContext(ctx, "ps", "aux").Output()
	if psErr == nil {
		scanner := bufio.NewScanner(strings.NewReader(string(psOut)))
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
func (s *Service) KillSession(_ context.Context, pid int) error {
	if pid <= 0 {
		return errors.New("invalid PID")
	}
	if err := syscall.Kill(pid, syscall.SIGTERM); err != nil {
		return fmt.Errorf("kill session %d failed: %w", pid, err)
	}
	log.Printf("ssh: killed session %d", pid)
	return nil
}

// GetLoginHistory returns recent SSH login attempts.
func (s *Service) GetLoginHistory(ctx context.Context, limit int) ([]LoginRecord, error) {
	// Try journalctl first
	stdout, err := exec.CommandContext(ctx, "journalctl", "-u", "sshd", "-u", "ssh", "--no-pager", "-n", strconv.Itoa(limit), "--output=short-iso").Output()
	if err != nil {
		// Fallback to /var/log/auth.log
		return s.getLoginHistoryFromAuthLog(limit)
	}

	var records []LoginRecord
	scanner := bufio.NewScanner(strings.NewReader(string(stdout)))
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
		return nil, errors.New("no SSH log file found (tried auth.log and secure)")
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

	sessionType := "interactive"
	if tty == "notty" {
		sessionType = "non-interactive"
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

	if _, after, ok := strings.Cut(line, "from "); ok {
		rest := after
		if before, _, ok := strings.Cut(rest, " "); ok {
			record.IP = before
		}
	}

	if _, after, ok := strings.Cut(line, "port "); ok {
		rest := after
		if before, _, ok := strings.Cut(rest, " "); ok {
			if port, err := strconv.Atoi(before); err == nil {
				record.Port = port
			}
		}
	}

	if _, after, ok := strings.Cut(line, "for "); ok {
		rest := after
		if before, _, ok := strings.Cut(rest, " "); ok {
			record.User = before
		}
	}

	if strings.Contains(line, "password") {
		record.Method = "password"
	} else if strings.Contains(line, "publickey") {
		record.Method = "publickey"
	}

	// Extract full ISO timestamp (first field before space)
	if before, _, ok := strings.Cut(line, " "); ok {
		record.Time = before
	}

	return record
}

// --- File helpers ---

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

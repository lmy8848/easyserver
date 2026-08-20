package ssh

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"encoding/pem"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"easyserver/internal/infra/errx"
	infrasystemd "easyserver/internal/infra/systemd"
	"easyserver/internal/util"

	"golang.org/x/crypto/ssh"
)

// HardenOptions controls a one-shot SSH hardening pass.
type HardenOptions struct {
	Port                int    `json:"port"`
	DisableRootLogin    bool   `json:"disable_root_login"`
	DisablePasswordAuth bool   `json:"disable_password_auth"`
	MaxAuthTries        int    `json:"max_auth_tries"`
	AllowUsers          string `json:"allow_users"`
}

// Harden applies SSH hardening: backs up config (SaveConfig does), applies the
// requested options, tests with `sshd -t`, and reloads. On test failure the
// backup is restored and sshd reloaded so the server stays reachable.
//
// Guards:
//   - DisablePasswordAuth requires at least one authorized key (avoid lockout).
//   - changing Port requires the new port to be free (avoid lockout).
func (s *Service) Harden(ctx context.Context, opts HardenOptions) (*Config, error) {
	cfg, err := s.GetConfig(ctx)
	if err != nil {
		return nil, errx.Internal("读取 SSH 配置失败: %w", err)
	}

	if opts.DisablePasswordAuth {
		keys, _ := s.ListAuthorizedKeys()
		if len(keys) == 0 {
			return nil, errx.BadRequest("禁用密码登录前需先配置至少一个 SSH 公钥，否则将无法登录")
		}
	}
	if opts.Port != 0 && opts.Port != cfg.Port {
		if !portAvailable(ctx, opts.Port) {
			return nil, errx.BadRequest("端口 %d 未空闲或不可用，请更换", opts.Port)
		}
	}

	if opts.Port != 0 {
		cfg.Port = opts.Port
	}
	if opts.DisableRootLogin {
		cfg.PermitRootLogin = "no"
	}
	if opts.DisablePasswordAuth {
		cfg.PasswordAuthentication = "no"
		cfg.PubkeyAuthentication = "yes"
	}
	if opts.MaxAuthTries > 0 {
		cfg.MaxAuthTries = opts.MaxAuthTries
	}
	if opts.AllowUsers != "" {
		cfg.AllowUsers = opts.AllowUsers
	}

	// SaveConfig backs up to .bak automatically.
	if err := s.SaveConfig(cfg); err != nil {
		return nil, errx.Internal("保存配置失败: %w", err)
	}

	// Validate before reload; restore on failure.
	if out, err := s.TestConfig(ctx); err != nil {
		// Rollback on test failure.
		_ = s.restoreBackup()
		_ = s.ReloadSSH(ctx)
		return nil, errx.BadRequest("加固配置校验未通过，已自动回滚：%s", out)
	}
	if err := s.ReloadSSH(ctx); err != nil {
		return nil, errx.Internal("重载 SSH 服务失败: %w", err)
	}
	return cfg, nil
}

// restoreBackup copies the .bak backup back over the drop-in file.
func (s *Service) restoreBackup() error {
	return copyFile(sshdDropInPath+".bak", sshdDropInPath)
}

// portAvailable reports whether a TCP port is free to listen on.
func portAvailable(ctx context.Context, port int) bool {
	lc := net.ListenConfig{}
	ln, err := lc.Listen(ctx, "tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return false
	}
	_ = ln.Close()
	return true
}

// --- SSH key management (authorized_keys) ---

// AuthorizedKey represents one entry in ~/.ssh/authorized_keys.
type AuthorizedKey struct {
	Comment     string `json:"comment"`
	Type        string `json:"type"`        // ssh-rsa, ssh-ed25519, ecdsa-sha2-nistp521, etc.
	Key         string `json:"key"`         // base64, truncated for display
	Fingerprint string `json:"fingerprint"` // SHA256:...
	Line        string `json:"-"`           // full original line
}

func authorizedKeysPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".ssh", "authorized_keys"), nil
}

// ListAuthorizedKeys parses ~/.ssh/authorized_keys.
func (s *Service) ListAuthorizedKeys() ([]AuthorizedKey, error) {
	p, err := authorizedKeysPath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return []AuthorizedKey{}, nil
		}
		return nil, err
	}
	var keys []AuthorizedKey
	for line := range strings.SplitSeq(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		pubKey, comment, _, _, err := ssh.ParseAuthorizedKey([]byte(line))
		if err == nil {
			k := AuthorizedKey{
				Type:        pubKey.Type(),
				Key:         string(ssh.MarshalAuthorizedKey(pubKey)),
				Fingerprint: ssh.FingerprintSHA256(pubKey),
				Comment:     comment,
				Line:        line,
			}
			parts := strings.Fields(k.Key)
			if len(parts) >= 2 {
				k.Key = parts[1]
			}
			if len(k.Key) > 20 {
				k.Key = k.Key[:20] + "..."
			}
			keys = append(keys, k)
			continue
		}

		// Fallback for custom or legacy lines
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}
		k := AuthorizedKey{Type: parts[0], Key: parts[1], Line: line}
		if len(parts) > 2 {
			k.Comment = strings.Join(parts[2:], " ")
		}
		if len(k.Key) > 20 {
			k.Key = k.Key[:20] + "..."
		}
		keys = append(keys, k)
	}
	return keys, nil
}

// AddAuthorizedKey appends a public key to ~/.ssh/authorized_keys.
func (s *Service) AddAuthorizedKey(pub string) error {
	pub = strings.TrimSpace(pub)
	if pub == "" {
		return errx.BadRequest("公钥不能为空")
	}
	if _, _, _, _, err := ssh.ParseAuthorizedKey([]byte(pub)); err != nil {
		return errx.BadRequest("公钥格式无效: %v", err)
	}
	p, err := authorizedKeysPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
		return err
	}
	f, err := os.OpenFile(p, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := fmt.Fprintf(f, "%s\n", pub); err != nil {
		return err
	}
	return nil
}

// RemoveAuthorizedKey removes entries whose comment or full line matches.
func (s *Service) RemoveAuthorizedKey(comment string) error {
	p, err := authorizedKeysPath()
	if err != nil {
		return err
	}
	data, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	var kept []string
	removed := false
	for line := range strings.SplitSeq(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		parts := strings.Fields(trimmed)
		matchComment := len(parts) > 2 && strings.Join(parts[2:], " ") == comment
		matchLine := trimmed == comment
		if matchComment || matchLine {
			removed = true
			continue
		}
		kept = append(kept, line)
	}
	if !removed {
		return errx.NotFound("未找到匹配的公钥")
	}
	return os.WriteFile(p, []byte(strings.Join(kept, "\n")+"\n"), 0600)
}

// GenerateKeyPair creates a new key pair in-memory using Go native cryptography APIs.
// The private key is returned to the caller as an OpenSSH PEM block (not stored server-side);
// the public key is added to authorized_keys automatically.
func (s *Service) GenerateKeyPair(ctx context.Context, name, keyType string) (string, error) {
	if name == "" {
		name = "easyserver-key"
	}
	if keyType == "" {
		keyType = "ed25519"
	}
	keyType = strings.ToLower(keyType)
	cleanName := filepath.Clean(name)
	if cleanName == "" || strings.Contains(cleanName, "\x00") || strings.Contains(cleanName, "\n") || strings.Contains(cleanName, "\r") {
		return "", errx.BadRequest("invalid key comment name")
	}
	comment := cleanName + "@easyserver"

	var (
		privBlock *pem.Block
		sshPub    ssh.PublicKey
		err       error
	)

	switch keyType {
	case "ed25519":
		pub, priv, genErr := ed25519.GenerateKey(rand.Reader)
		if genErr != nil {
			return "", fmt.Errorf("generate ed25519 key: %w", genErr)
		}
		privBlock, err = ssh.MarshalPrivateKey(priv, comment)
		if err != nil {
			return "", fmt.Errorf("marshal ed25519 private key: %w", err)
		}
		sshPub, err = ssh.NewPublicKey(pub)
		if err != nil {
			return "", fmt.Errorf("convert ed25519 public key: %w", err)
		}
	case "rsa":
		priv, genErr := rsa.GenerateKey(rand.Reader, 4096)
		if genErr != nil {
			return "", fmt.Errorf("generate rsa key: %w", genErr)
		}
		privBlock, err = ssh.MarshalPrivateKey(priv, comment)
		if err != nil {
			return "", fmt.Errorf("marshal rsa private key: %w", err)
		}
		sshPub, err = ssh.NewPublicKey(&priv.PublicKey)
		if err != nil {
			return "", fmt.Errorf("convert rsa public key: %w", err)
		}
	default:
		return "", errx.BadRequest("不支持的密钥类型: %s", keyType)
	}

	pubLine := fmt.Sprintf("%s %s", strings.TrimSpace(string(ssh.MarshalAuthorizedKey(sshPub))), comment)
	if err := s.AddAuthorizedKey(pubLine); err != nil {
		return "", err
	}

	privPEM := pem.EncodeToMemory(privBlock)
	return string(privPEM), nil
}

// --- fail2ban ---

// Fail2banStatus reports fail2ban install/active state and jails.
type Fail2banStatus struct {
	Installed bool   `json:"installed"`
	Active    bool   `json:"active"`
	Enabled   bool   `json:"enabled"`
	Jails     []Jail `json:"jails"`
}

// Jail is a fail2ban jail summary.
type Jail struct {
	Name   string `json:"name"`
	Failed int    `json:"failed"`
	Banned int    `json:"banned"`
}

// Fail2banStatus returns fail2ban state. Never errors on "not installed".
func (s *Service) Fail2banStatus(ctx context.Context) *Fail2banStatus {
	st := &Fail2banStatus{}
	if _, err := exec.LookPath("fail2ban-client"); err != nil {
		return st // fail2ban 未安装时返回未安装状态
	}
	st.Installed = true
	st.Active = util.SystemdUnitActive(ctx, "fail2ban")
	st.Enabled = util.SystemdUnitEnabled(ctx, "fail2ban")
	// List jails.
	out, err := exec.CommandContext(ctx, "fail2ban-client", "status").CombinedOutput()
	if err == nil {
		for line := range strings.SplitSeq(string(out), "\n") {
			line = strings.TrimSpace(line)
			if after, ok := strings.CutPrefix(line, "Jail list:"); ok {
				jails := strings.SplitSeq(after, ",")
				for j := range jails {
					j = strings.TrimSpace(j)
					if j == "" {
						continue
					}
					st.Jails = append(st.Jails, Jail{Name: j})
				}
			}
		}
		// Per-jail failed/banned counts.
		for i := range st.Jails {
			out, _ := exec.CommandContext(ctx, "fail2ban-client", "status", st.Jails[i].Name).CombinedOutput()
			for line := range strings.SplitSeq(string(out), "\n") {
				line = strings.TrimSpace(line)
				if strings.HasPrefix(line, "Currently failed:") {
					_, _ = fmt.Sscanf(line, "Currently failed:%d", &st.Jails[i].Failed)
				}
				if strings.HasPrefix(line, "Currently banned:") {
					_, _ = fmt.Sscanf(line, "Currently banned:%d", &st.Jails[i].Banned)
				}
			}
		}
	}
	return st
}

// InstallFail2ban installs fail2ban and enables an sshd jail.
func (s *Service) InstallFail2ban(ctx context.Context) error {
	if _, err := exec.LookPath("fail2ban-client"); err == nil {
		return errx.Conflict("fail2ban 已安装")
	}
	if _, err := exec.CommandContext(ctx, "apt-get", "install", "-y", "fail2ban").CombinedOutput(); err != nil {
		return fmt.Errorf("安装 fail2ban 失败: %w", err)
	}
	// Minimal sshd jail config.
	jail := `[sshd]
enabled = true
port = ssh
filter = sshd
logpath = %(sshd_log)s
maxretry = 5
bantime = 1h
`
	if err := os.WriteFile("/etc/fail2ban/jail.d/sshd.local", []byte(jail), 0644); err != nil {
		return err
	}
	client := infrasystemd.DefaultClient()
	if _, _, err := client.EnableUnitFilesContext(ctx, []string{"fail2ban.service"}, false, false); err != nil {
		return fmt.Errorf("启用 fail2ban 失败: %w", err)
	}
	if _, err := client.StartUnitContext(ctx, "fail2ban.service", "replace"); err != nil {
		return fmt.Errorf("启动 fail2ban 失败: %w", err)
	}
	return nil
}

// ReloadFail2ban reloads fail2ban config.
func (s *Service) ReloadFail2ban(ctx context.Context) error {
	if _, err := exec.LookPath("fail2ban-client"); err != nil {
		return errx.BadRequest("fail2ban 未安装")
	}
	if _, err := infrasystemd.DefaultClient().ReloadUnitContext(ctx, "fail2ban.service", "replace"); err != nil {
		return fmt.Errorf("重载 fail2ban 失败: %w", err)
	}
	return nil
}

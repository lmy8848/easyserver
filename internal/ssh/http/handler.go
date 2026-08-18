package http

import (
	"strconv"
	"strings"

	"easyserver/internal/httpx"
	"easyserver/internal/httpx/middleware"
	"easyserver/internal/infra/errx"
	"easyserver/internal/ssh"

	"github.com/gin-gonic/gin"
)

// SSHHandler handles SSH management requests
type SSHHandler struct {
	sshService *ssh.Service
}

// NewSSHHandler creates a new SSH handler
func NewSSHHandler(sshService *ssh.Service) *SSHHandler {
	return &SSHHandler{sshService: sshService}
}

// GetConfig returns the current SSH configuration
func (h *SSHHandler) GetConfig(c *gin.Context) (any, error) {
	config, err := h.sshService.GetConfig(c.Request.Context())
	if err != nil {
		return nil, errx.Internal("读取 SSH 配置失败: %w", err)
	}
	return config, nil
}

// CheckStatus reports whether the SSH server is usable.
func (h *SSHHandler) CheckStatus(c *gin.Context) (any, error) {
	return h.sshService.CheckStatus(c.Request.Context())
}

// SaveConfig saves the SSH configuration
func (h *SSHHandler) SaveConfig(c *gin.Context) (any, error) {
	var config struct {
		Port                   int    `json:"port"`
		PermitRootLogin        string `json:"permit_root_login"`
		PasswordAuthentication string `json:"password_auth"`
		PubkeyAuthentication   string `json:"pubkey_auth"`
		MaxAuthTries           int    `json:"max_auth_tries"`
		LoginGraceTime         int    `json:"login_grace_time"`
		ClientAliveInterval    int    `json:"client_alive_interval"`
		ClientAliveCountMax    int    `json:"client_alive_count_max"`
		AllowUsers             string `json:"allow_users"`
		DenyUsers              string `json:"deny_users"`
	}

	if err := c.ShouldBindJSON(&config); err != nil {
		return nil, errx.BadRequest("无效的请求: %w", err)
	}

	middleware.AuditSummary(c, "保存 SSH 配置")

	// Validate port
	if config.Port < 1 || config.Port > 65535 {
		return nil, errx.BadRequest("端口必须在 1 到 65535 之间")
	}

	// Validate PermitRootLogin
	validPermitRootLogin := map[string]bool{"yes": true, "no": true, "prohibit-password": true}
	if config.PermitRootLogin != "" && !validPermitRootLogin[config.PermitRootLogin] {
		return nil, errx.BadRequest("permit_root_login 必须是 yes、no 或 prohibit-password")
	}

	// Validate PasswordAuthentication
	validYesNo := map[string]bool{"yes": true, "no": true}
	if config.PasswordAuthentication != "" && !validYesNo[config.PasswordAuthentication] {
		return nil, errx.BadRequest("password_auth 必须是 yes 或 no")
	}

	// Validate PubkeyAuthentication
	if config.PubkeyAuthentication != "" && !validYesNo[config.PubkeyAuthentication] {
		return nil, errx.BadRequest("pubkey_auth 必须是 yes 或 no")
	}

	// Validate numeric bounds
	if config.MaxAuthTries < 0 || config.MaxAuthTries > 100 {
		return nil, errx.BadRequest("max_auth_tries 必须在 0 到 100 之间")
	}
	if config.LoginGraceTime < 0 || config.LoginGraceTime > 3600 {
		return nil, errx.BadRequest("login_grace_time 必须在 0 到 3600 之间")
	}
	if config.ClientAliveInterval < 0 || config.ClientAliveInterval > 86400 {
		return nil, errx.BadRequest("client_alive_interval 必须在 0 到 86400 之间")
	}
	if config.ClientAliveCountMax < 0 || config.ClientAliveCountMax > 100 {
		return nil, errx.BadRequest("client_alive_count_max 必须在 0 到 100 之间")
	}

	// Sanitize AllowUsers/DenyUsers (strip newlines to prevent CRLF injection)
	config.AllowUsers = strings.NewReplacer("\n", "", "\r", "").Replace(config.AllowUsers)
	config.DenyUsers = strings.NewReplacer("\n", "", "\r", "").Replace(config.DenyUsers)

	// Save config
	sshConfig := &ssh.Config{
		Port:                   config.Port,
		PermitRootLogin:        config.PermitRootLogin,
		PasswordAuthentication: config.PasswordAuthentication,
		PubkeyAuthentication:   config.PubkeyAuthentication,
		MaxAuthTries:           config.MaxAuthTries,
		LoginGraceTime:         config.LoginGraceTime,
		ClientAliveInterval:    config.ClientAliveInterval,
		ClientAliveCountMax:    config.ClientAliveCountMax,
		AllowUsers:             config.AllowUsers,
		DenyUsers:              config.DenyUsers,
	}

	if err := h.sshService.SaveConfig(sshConfig); err != nil {
		return nil, errx.Internal("保存配置失败: %w", err)
	}

	return gin.H{"message": "SSH 配置已保存"}, nil
}

// TestConfig tests the SSH configuration
func (h *SSHHandler) TestConfig(c *gin.Context) (any, error) {
	middleware.AuditSummary(c, "测试 SSH 配置")
	output, err := h.sshService.TestConfig(c.Request.Context())
	if err != nil {
		return nil, errx.BadRequest("%s", output)
	}
	return gin.H{"message": output}, nil
}

// ReloadSSH reloads the SSH service
func (h *SSHHandler) ReloadSSH(c *gin.Context) (any, error) {
	middleware.AuditSummary(c, "重载 SSH 服务")
	if err := h.sshService.ReloadSSH(c.Request.Context()); err != nil {
		return nil, errx.Internal("重载 SSH 失败: %w", err)
	}
	return gin.H{"message": "SSH 服务已重载"}, nil
}

// GetSessions returns active SSH sessions
func (h *SSHHandler) GetSessions(c *gin.Context) (any, error) {
	sessions, err := h.sshService.GetSessions(c.Request.Context())
	if err != nil {
		return nil, err
	}
	return gin.H{"sessions": sessions}, nil
}

// KillSession kills an SSH session
func (h *SSHHandler) KillSession(c *gin.Context) (any, error) {
	pidStr := c.Param("pid")
	pid, err := strconv.Atoi(pidStr)
	if err != nil {
		return nil, errx.BadRequest("无效的 PID")
	}

	middleware.AuditSummary(c, "终止 SSH 会话 "+strconv.Itoa(pid))

	// Validate PID bounds
	if pid <= 1 {
		return nil, errx.BadRequest("无效的 PID: 必须大于 1")
	}
	if pid > 4194304 { // Max PID on Linux
		return nil, errx.BadRequest("无效的 PID: 数值过大")
	}

	if err := h.sshService.KillSession(c.Request.Context(), pid); err != nil {
		return nil, errx.Internal("终止会话失败: %w", err)
	}
	return gin.H{"message": "会话已终止"}, nil
}

// GetLoginHistory returns SSH login history
func (h *SSHHandler) GetLoginHistory(c *gin.Context) (any, error) {
	limitStr := c.DefaultQuery("limit", "50")
	limit, err := strconv.Atoi(limitStr)
	if err != nil || limit < 1 {
		limit = 50
	}

	records, err := h.sshService.GetLoginHistory(c.Request.Context(), limit)
	if err != nil {
		return nil, err
	}
	return gin.H{"records": records}, nil
}

// Harden applies SSH hardening options (backup + test + reload, rollback on failure).
func (h *SSHHandler) Harden(c *gin.Context) (any, error) {
	var req ssh.HardenOptions
	if err := c.ShouldBindJSON(&req); err != nil {
		return nil, err
	}
	cfg, err := h.sshService.Harden(c.Request.Context(), req)
	if err != nil {
		return nil, err
	}
	return cfg, nil
}

// ListAuthorizedKeys returns ~/.ssh/authorized_keys entries.
func (h *SSHHandler) ListAuthorizedKeys(c *gin.Context) (any, error) {
	keys, err := h.sshService.ListAuthorizedKeys()
	if err != nil {
		return nil, err
	}
	return gin.H{"keys": keys}, nil
}

// AddAuthorizedKey appends a public key.
func (h *SSHHandler) AddAuthorizedKey(c *gin.Context) (any, error) {
	var req struct {
		Key string `json:"key" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		return nil, err
	}
	if err := h.sshService.AddAuthorizedKey(req.Key); err != nil {
		return nil, err
	}
	return gin.H{"message": "公钥已添加"}, nil
}

// RemoveAuthorizedKey removes a key by comment (query ?comment=).
func (h *SSHHandler) RemoveAuthorizedKey(c *gin.Context) (any, error) {
	comment := c.Query("comment")
	if comment == "" {
		return nil, errx.BadRequest("缺少 comment 参数")
	}
	if err := h.sshService.RemoveAuthorizedKey(comment); err != nil {
		return nil, err
	}
	return gin.H{"message": "公钥已删除"}, nil
}

// GenerateKeyPair generates a new key pair, returns private key, auto-authorizes public key.
func (h *SSHHandler) GenerateKeyPair(c *gin.Context) (any, error) {
	var req struct {
		Name    string `json:"name"`
		KeyType string `json:"key_type"`
	}
	_ = c.ShouldBindJSON(&req)
	priv, err := h.sshService.GenerateKeyPair(c.Request.Context(), req.Name, req.KeyType)
	if err != nil {
		return nil, err
	}
	return gin.H{"private_key": priv}, nil
}

// Fail2banStatus returns fail2ban install/active state and jails.
func (h *SSHHandler) Fail2banStatus(c *gin.Context) (any, error) {
	st := h.sshService.Fail2banStatus(c.Request.Context())
	return st, nil
}

// InstallFail2ban installs and enables fail2ban with an sshd jail.
func (h *SSHHandler) InstallFail2ban(c *gin.Context) (any, error) {
	if err := h.sshService.InstallFail2ban(c.Request.Context()); err != nil {
		return nil, err
	}
	return gin.H{"message": "fail2ban 已安装并启用"}, nil
}

// ReloadFail2ban reloads fail2ban config.
func (h *SSHHandler) ReloadFail2ban(c *gin.Context) (any, error) {
	if err := h.sshService.ReloadFail2ban(c.Request.Context()); err != nil {
		return nil, err
	}
	return gin.H{"message": "fail2ban 已重载"}, nil
}

func RegisterRoutes(protected *gin.RouterGroup, sshService *ssh.Service) {
	handler := NewSSHHandler(sshService)

	// SSH Config
	protected.GET("/ssh/status", httpx.H(handler.CheckStatus))
	protected.GET("/ssh/config", httpx.H(handler.GetConfig))
	protected.PUT("/ssh/config", httpx.H(handler.SaveConfig))
	protected.POST("/ssh/config/test", httpx.H(handler.TestConfig))
	protected.POST("/ssh/config/reload", httpx.H(handler.ReloadSSH))

	// SSH Sessions
	protected.GET("/ssh/sessions", httpx.H(handler.GetSessions))
	protected.POST("/ssh/sessions/:pid/kill", httpx.H(handler.KillSession))

	// SSH Login History
	protected.GET("/ssh/logins", httpx.H(handler.GetLoginHistory))

	// SSH Hardening
	protected.POST("/ssh/harden", httpx.H(handler.Harden))
	protected.GET("/ssh/authorized-keys", httpx.H(handler.ListAuthorizedKeys))
	protected.POST("/ssh/authorized-keys", httpx.H(handler.AddAuthorizedKey))
	protected.DELETE("/ssh/authorized-keys", httpx.H(handler.RemoveAuthorizedKey))
	protected.POST("/ssh/keys/generate", httpx.H(handler.GenerateKeyPair))

	// fail2ban
	protected.GET("/ssh/fail2ban", httpx.H(handler.Fail2banStatus))
	protected.POST("/ssh/fail2ban/install", httpx.H(handler.InstallFail2ban))
	protected.POST("/ssh/fail2ban/reload", httpx.H(handler.ReloadFail2ban))
}

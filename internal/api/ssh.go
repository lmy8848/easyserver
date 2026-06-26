package api

import (
	"strconv"
	"strings"

	"easyserver/internal/model"
	
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
func (h *SSHHandler) GetConfig(c *gin.Context) {
	config, err := h.sshService.GetConfig()
	if err != nil {
		c.Error(WrapError(err))
		return
	}
	Success(c, config)
}

// SaveConfig saves the SSH configuration
func (h *SSHHandler) SaveConfig(c *gin.Context) {
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
		c.Error(ErrBadRequest.WithMessage("无效的请求: "+err.Error()))
		return
	}

	// Validate port
	if config.Port < 1 || config.Port > 65535 {
		c.Error(ErrBadRequest.WithMessage("端口必须在 1 到 65535 之间"))
		return
	}

	// Validate PermitRootLogin
	validPermitRootLogin := map[string]bool{"yes": true, "no": true, "prohibit-password": true}
	if config.PermitRootLogin != "" && !validPermitRootLogin[config.PermitRootLogin] {
		c.Error(ErrBadRequest.WithMessage("permit_root_login 必须是 yes、no 或 prohibit-password"))
		return
	}

	// Validate PasswordAuthentication
	validYesNo := map[string]bool{"yes": true, "no": true}
	if config.PasswordAuthentication != "" && !validYesNo[config.PasswordAuthentication] {
		c.Error(ErrBadRequest.WithMessage("password_auth 必须是 yes 或 no"))
		return
	}

	// Validate PubkeyAuthentication
	if config.PubkeyAuthentication != "" && !validYesNo[config.PubkeyAuthentication] {
		c.Error(ErrBadRequest.WithMessage("pubkey_auth 必须是 yes 或 no"))
		return
	}

	// Validate numeric bounds
	if config.MaxAuthTries < 0 || config.MaxAuthTries > 100 {
		c.Error(ErrBadRequest.WithMessage("max_auth_tries 必须在 0 到 100 之间"))
		return
	}
	if config.LoginGraceTime < 0 || config.LoginGraceTime > 3600 {
		c.Error(ErrBadRequest.WithMessage("login_grace_time 必须在 0 到 3600 之间"))
		return
	}
	if config.ClientAliveInterval < 0 || config.ClientAliveInterval > 86400 {
		c.Error(ErrBadRequest.WithMessage("client_alive_interval 必须在 0 到 86400 之间"))
		return
	}
	if config.ClientAliveCountMax < 0 || config.ClientAliveCountMax > 100 {
		c.Error(ErrBadRequest.WithMessage("client_alive_count_max 必须在 0 到 100 之间"))
		return
	}

	// Sanitize AllowUsers/DenyUsers (strip newlines to prevent CRLF injection)
	config.AllowUsers = strings.NewReplacer("\n", "", "\r", "").Replace(config.AllowUsers)
	config.DenyUsers = strings.NewReplacer("\n", "", "\r", "").Replace(config.DenyUsers)

	// Save config
	sshConfig := &model.SSHConfig{
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
		c.Error(ErrInternal.WithMessage("保存配置失败: "+err.Error()))
		return
	}

	Success(c, gin.H{"message": "SSH 配置已保存"})
}

// TestConfig tests the SSH configuration
func (h *SSHHandler) TestConfig(c *gin.Context) {
	output, err := h.sshService.TestConfig(c.Request.Context())
	if err != nil {
		c.Error(ErrBadRequest.WithMessage(output))
		return
	}
	Success(c, gin.H{"message": output})
}

// ReloadSSH reloads the SSH service
func (h *SSHHandler) ReloadSSH(c *gin.Context) {
	if err := h.sshService.ReloadSSH(c.Request.Context()); err != nil {
		c.Error(ErrInternal.WithMessage("重载 SSH 失败: "+err.Error()))
		return
	}
	Success(c, gin.H{"message": "SSH 服务已重载"})
}

// GetSessions returns active SSH sessions
func (h *SSHHandler) GetSessions(c *gin.Context) {
	sessions, err := h.sshService.GetSessions(c.Request.Context())
	if err != nil {
		c.Error(WrapError(err))
		return
	}
	Success(c, gin.H{"sessions": sessions})
}

// KillSession kills an SSH session
func (h *SSHHandler) KillSession(c *gin.Context) {
	pidStr := c.Param("pid")
	pid, err := strconv.Atoi(pidStr)
	if err != nil {
		c.Error(ErrBadRequest.WithMessage("无效的 PID"))
		return
	}

	// Validate PID bounds
	if pid <= 1 {
		c.Error(ErrBadRequest.WithMessage("无效的 PID: 必须大于 1"))
		return
	}
	if pid > 4194304 { // Max PID on Linux
		c.Error(ErrBadRequest.WithMessage("无效的 PID: 数值过大"))
		return
	}

	if err := h.sshService.KillSession(c.Request.Context(), pid); err != nil {
		c.Error(ErrInternal.WithMessage("终止会话失败: "+err.Error()))
		return
	}
	Success(c, gin.H{"message": "会话已终止"})
}

// GetLoginHistory returns SSH login history
func (h *SSHHandler) GetLoginHistory(c *gin.Context) {
	limitStr := c.DefaultQuery("limit", "50")
	limit, err := strconv.Atoi(limitStr)
	if err != nil || limit < 1 {
		limit = 50
	}

	records, err := h.sshService.GetLoginHistory(c.Request.Context(), limit)
	if err != nil {
		c.Error(WrapError(err))
		return
	}
	Success(c, gin.H{"records": records})
}

func registerSSHRoutes(protected *gin.RouterGroup, sshService *ssh.Service) {
	handler := NewSSHHandler(sshService)

	// SSH Config
	protected.GET("/ssh/config", handler.GetConfig)
	protected.PUT("/ssh/config", handler.SaveConfig)
	protected.POST("/ssh/config/test", handler.TestConfig)
	protected.POST("/ssh/config/reload", handler.ReloadSSH)

	// SSH Sessions
	protected.GET("/ssh/sessions", handler.GetSessions)
	protected.POST("/ssh/sessions/:pid/kill", handler.KillSession)

	// SSH Login History
	protected.GET("/ssh/logins", handler.GetLoginHistory)
}

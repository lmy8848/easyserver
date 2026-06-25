package api

import (
	"strconv"
	"strings"

	"easyserver/internal/model"
	"easyserver/internal/service"

	"github.com/gin-gonic/gin"
)

// SSHHandler handles SSH management requests
type SSHHandler struct {
	sshService *service.SSHConfigService
}

// NewSSHHandler creates a new SSH handler
func NewSSHHandler(sshService *service.SSHConfigService) *SSHHandler {
	return &SSHHandler{sshService: sshService}
}

// GetConfig returns the current SSH configuration
func (h *SSHHandler) GetConfig(c *gin.Context) {
	config, err := h.sshService.GetConfig()
	if err != nil {
		InternalError(c, err.Error())
		return
	}
	Success(c, config)
}

// SaveConfig saves the SSH configuration
func (h *SSHHandler) SaveConfig(c *gin.Context) {
	var config struct {
		Port                int    `json:"port"`
		PermitRootLogin     string `json:"permit_root_login"`
		PasswordAuthentication string `json:"password_auth"`
		PubkeyAuthentication   string `json:"pubkey_auth"`
		MaxAuthTries        int    `json:"max_auth_tries"`
		LoginGraceTime      int    `json:"login_grace_time"`
		ClientAliveInterval int    `json:"client_alive_interval"`
		ClientAliveCountMax int    `json:"client_alive_count_max"`
		AllowUsers          string `json:"allow_users"`
		DenyUsers           string `json:"deny_users"`
	}

	if err := c.ShouldBindJSON(&config); err != nil {
		BadRequest(c, "无效的请求: "+err.Error())
		return
	}

	// Validate port
	if config.Port < 1 || config.Port > 65535 {
		BadRequest(c, "端口必须在 1 到 65535 之间")
		return
	}

	// Validate PermitRootLogin
	validPermitRootLogin := map[string]bool{"yes": true, "no": true, "prohibit-password": true}
	if config.PermitRootLogin != "" && !validPermitRootLogin[config.PermitRootLogin] {
		BadRequest(c, "permit_root_login 必须是 yes、no 或 prohibit-password")
		return
	}

	// Validate PasswordAuthentication
	validYesNo := map[string]bool{"yes": true, "no": true}
	if config.PasswordAuthentication != "" && !validYesNo[config.PasswordAuthentication] {
		BadRequest(c, "password_auth 必须是 yes 或 no")
		return
	}

	// Validate PubkeyAuthentication
	if config.PubkeyAuthentication != "" && !validYesNo[config.PubkeyAuthentication] {
		BadRequest(c, "pubkey_auth 必须是 yes 或 no")
		return
	}

	// Validate numeric bounds
	if config.MaxAuthTries < 0 || config.MaxAuthTries > 100 {
		BadRequest(c, "max_auth_tries 必须在 0 到 100 之间")
		return
	}
	if config.LoginGraceTime < 0 || config.LoginGraceTime > 3600 {
		BadRequest(c, "login_grace_time 必须在 0 到 3600 之间")
		return
	}
	if config.ClientAliveInterval < 0 || config.ClientAliveInterval > 86400 {
		BadRequest(c, "client_alive_interval 必须在 0 到 86400 之间")
		return
	}
	if config.ClientAliveCountMax < 0 || config.ClientAliveCountMax > 100 {
		BadRequest(c, "client_alive_count_max 必须在 0 到 100 之间")
		return
	}

	// Sanitize AllowUsers/DenyUsers (strip newlines to prevent CRLF injection)
	config.AllowUsers = strings.NewReplacer("\n", "", "\r", "").Replace(config.AllowUsers)
	config.DenyUsers = strings.NewReplacer("\n", "", "\r", "").Replace(config.DenyUsers)

	// Save config
	sshConfig := &model.SSHConfig{
		Port:                config.Port,
		PermitRootLogin:     config.PermitRootLogin,
		PasswordAuthentication: config.PasswordAuthentication,
		PubkeyAuthentication:   config.PubkeyAuthentication,
		MaxAuthTries:        config.MaxAuthTries,
		LoginGraceTime:      config.LoginGraceTime,
		ClientAliveInterval: config.ClientAliveInterval,
		ClientAliveCountMax: config.ClientAliveCountMax,
		AllowUsers:          config.AllowUsers,
		DenyUsers:           config.DenyUsers,
	}

	if err := h.sshService.SaveConfig(sshConfig); err != nil {
		InternalError(c, "保存配置失败: "+err.Error())
		return
	}

	Success(c, gin.H{"message": "SSH 配置已保存"})
}

// TestConfig tests the SSH configuration
func (h *SSHHandler) TestConfig(c *gin.Context) {
	output, err := h.sshService.TestConfig(c.Request.Context())
	if err != nil {
		BadRequest(c, output)
		return
	}
	Success(c, gin.H{"message": output})
}

// ReloadSSH reloads the SSH service
func (h *SSHHandler) ReloadSSH(c *gin.Context) {
	if err := h.sshService.ReloadSSH(c.Request.Context()); err != nil {
		InternalError(c, "重载 SSH 失败: "+err.Error())
		return
	}
	Success(c, gin.H{"message": "SSH 服务已重载"})
}

// GetSessions returns active SSH sessions
func (h *SSHHandler) GetSessions(c *gin.Context) {
	sessions, err := h.sshService.GetSessions(c.Request.Context())
	if err != nil {
		InternalError(c, err.Error())
		return
	}
	Success(c, gin.H{"sessions": sessions})
}

// KillSession kills an SSH session
func (h *SSHHandler) KillSession(c *gin.Context) {
	pidStr := c.Param("pid")
	pid, err := strconv.Atoi(pidStr)
	if err != nil {
		BadRequest(c, "无效的 PID")
		return
	}

	// Validate PID bounds
	if pid <= 1 {
		BadRequest(c, "无效的 PID: 必须大于 1")
		return
	}
	if pid > 4194304 { // Max PID on Linux
		BadRequest(c, "无效的 PID: 数值过大")
		return
	}

	if err := h.sshService.KillSession(c.Request.Context(), pid); err != nil {
		InternalError(c, "终止会话失败: "+err.Error())
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
		InternalError(c, err.Error())
		return
	}
	Success(c, gin.H{"records": records})
}

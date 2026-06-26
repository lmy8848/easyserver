package api

import (
	"log"
	"time"

	"easyserver/internal/middleware"
	"easyserver/internal/model"
	"easyserver/internal/audit"
	"easyserver/internal/auth"

	"github.com/gin-gonic/gin"
)

type AuthHandler struct {
	authService    *auth.AuthService
	auditService   *audit.Service
	sessionService *auth.SessionService
	jwtSecret      string
	sessionTimeout time.Duration
}

func NewAuthHandler(authService *auth.AuthService, jwtSecret string, auditService *audit.Service, sessionService *auth.SessionService, sessionTimeout time.Duration) *AuthHandler {
	return &AuthHandler{
		authService:    authService,
		auditService:   auditService,
		sessionService: sessionService,
		jwtSecret:      jwtSecret,
		sessionTimeout: sessionTimeout,
	}
}

type LoginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

type ChangePasswordRequest struct {
	OldPassword string `json:"old_password" binding:"required"`
	NewPassword string `json:"new_password" binding:"required,min=8"`
}

func (h *AuthHandler) Login(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(ErrBadRequest.Wrap(err))
		return
	}

	ip := c.ClientIP()
	userAgent := c.Request.UserAgent()

	// Use LoginWithInfo to log activity
	user, err := h.authService.LoginWithInfo(c.Request.Context(), req.Username, req.Password, ip, userAgent)
	if err != nil {
		// Also log to audit log
		if h.auditService != nil {
			h.auditService.LogSecurityEvent(c.Request.Context(), req.Username, "LOGIN_FAILED",
				err.Error(), ip, userAgent)
		}
		c.Error(ErrUnauthorized.Wrap(err))
		return
	}

	// Check if TOTP is enabled for this user
	totpEnabled, err := h.authService.IsTOTPEnabled(c.Request.Context(), user.ID)
	if err != nil {
		c.Error(ErrInternal.WithMessage("检查 TOTP 状态失败"))
		return
	}

	// If TOTP is enabled, return a temporary token for TOTP verification
	if totpEnabled {
		tempToken, err := middleware.GenerateTOTPTempToken(h.jwtSecret, user.ID)
		if err != nil {
			c.Error(ErrInternal.WithMessage("生成临时令牌失败"))
			return
		}

		Success(c, gin.H{
			"requires_totp": true,
			"temp_token":    tempToken,
		})
		return
	}

	// If TOTP is not enabled, proceed with normal login
	token, err := middleware.GenerateToken(h.jwtSecret, user.ID, user.Username, string(user.Role), h.sessionTimeout)
	if err != nil {
		c.Error(ErrInternal.WithMessage("生成令牌失败"))
		return
	}

	// Create session (single session per user - remove old sessions first)
	if h.sessionService != nil {
		// Remove all existing sessions for this user
		if err := h.sessionService.RemoveUserSessions(c.Request.Context(), user.ID); err != nil { log.Printf("warning: remove old sessions: %v", err) }
		// Create new session
		expiresAt := time.Now().Add(h.sessionTimeout)
		h.sessionService.CreateSession(c.Request.Context(), token, user.ID, user.Username, string(user.Role), ip, userAgent, expiresAt)
	}

	// Also log to audit log
	if h.auditService != nil {
		h.auditService.LogSecurityEvent(c.Request.Context(), req.Username, "LOGIN_SUCCESS",
			"User logged in successfully", ip, userAgent)
	}

	Success(c, gin.H{
		"token":            token,
		"user":             user,
		"must_change_pass": user.MustChangePass,
	})
}

func (h *AuthHandler) Logout(c *gin.Context) {
	token := c.GetHeader("Authorization")
	if len(token) > 7 && token[:7] == "Bearer " {
		tokenStr := token[7:]

		// Blacklist the token
		if h.authService != nil {
			userID := c.GetInt64("user_id")
			if userID > 0 {
				// Blacklist until token would naturally expire
				h.authService.AddTokenToBlacklist(c.Request.Context(), userID, tokenStr, time.Now().Add(h.sessionTimeout))
			}
		}

		// Remove session
		if h.sessionService != nil {
			h.sessionService.RemoveSession(c.Request.Context(), tokenStr)
		}
	}

	// Log logout
	if h.auditService != nil {
		if uname, ok := c.Get("username"); ok {
			if unameStr, ok := uname.(string); ok {
				h.auditService.LogSecurityEvent(c.Request.Context(), unameStr, "LOGOUT",
					"User logged out", c.ClientIP(), c.Request.UserAgent())
			}
		}
	}
	Success(c, nil)
}

func (h *AuthHandler) GetProfile(c *gin.Context) {
	userID := c.GetInt64("user_id")
	if userID == 0 {
		c.Error(ErrUnauthorized.WithMessage("未登录"))
		return
	}

	// Get user from database
	user, err := h.authService.GetUserByID(c.Request.Context(), userID)
	if err != nil {
		c.Error(ErrInternal.WithMessage("获取用户信息失败"))
		return
	}

	Success(c, user)
}

func (h *AuthHandler) ChangePassword(c *gin.Context) {
	userID := c.GetInt64("user_id")
	username, _ := c.Get("username")

	var req ChangePasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(ErrBadRequest.Wrap(err))
		return
	}

	if err := h.authService.ChangePassword(c.Request.Context(), userID, req.OldPassword, req.NewPassword); err != nil {
		// Log failed password change
		if h.auditService != nil {
			if unameStr, ok := username.(string); ok {
				h.auditService.LogSecurityEvent(c.Request.Context(), unameStr, "PASSWORD_CHANGE_FAILED",
					err.Error(), c.ClientIP(), c.Request.UserAgent())
			}
		}
		c.Error(ErrBadRequest.Wrap(err))
		return
	}

	// Invalidate all existing tokens for this user after password change
	// This forces re-authentication on all devices
	if err := h.authService.InvalidateAllUserTokens(c.Request.Context(), userID); err != nil {
		log.Printf("auth: failed to invalidate tokens after password change for user %d: %v", userID, err)
	}

	// Log successful password change
	if h.auditService != nil {
		if unameStr, ok := username.(string); ok {
			h.auditService.LogSecurityEvent(c.Request.Context(), unameStr, "PASSWORD_CHANGED",
				"Password changed successfully", c.ClientIP(), c.Request.UserAgent())
		}
	}

	Success(c, nil)
}

// TOTP verification request
type TOTPVerifyRequest struct {
	TempToken string `json:"temp_token" binding:"required"`
	Code      string `json:"code" binding:"required"`
}

// Backup code verification request
type BackupCodeVerifyRequest struct {
	TempToken  string `json:"temp_token" binding:"required"`
	BackupCode string `json:"backup_code" binding:"required"`
}

// TOTP enable request
type TOTPEnableRequest struct {
	Code string `json:"code" binding:"required"`
}

// TOTP disable request
type TOTPDisableRequest struct {
	Password string `json:"password" binding:"required"`
}

// VerifyTOTP handles TOTP verification during login (step 2)
func (h *AuthHandler) VerifyTOTP(c *gin.Context) {
	var req TOTPVerifyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(ErrBadRequest.Wrap(err))
		return
	}

	// Validate temp token
	userID, err := middleware.ValidateTOTPTempToken(h.jwtSecret, req.TempToken)
	if err != nil {
		c.Error(ErrUnauthorized.WithMessage("临时令牌无效或已过期"))
		return
	}

	// Get user's TOTP secret
	secret, err := h.authService.GetTOTPSecret(c.Request.Context(), userID)
	if err != nil {
		c.Error(ErrUnauthorized.WithMessage("该用户未启用 TOTP"))
		return
	}

	// Verify TOTP code
	if !h.authService.VerifyTOTP(secret, req.Code) {
		// Log failed TOTP verification
		if h.auditService != nil {
			user, _ := h.authService.GetUserByID(c.Request.Context(), userID)
			if user != nil {
				h.auditService.LogSecurityEvent(c.Request.Context(), user.Username, "TOTP_VERIFICATION_FAILED",
					"Invalid TOTP code", c.ClientIP(), c.Request.UserAgent())
			}
		}
		c.Error(ErrUnauthorized.WithMessage("TOTP 验证码无效"))
		return
	}

	// Get user info
	user, err := h.authService.GetUserByID(c.Request.Context(), userID)
	if err != nil {
		c.Error(ErrInternal.WithMessage("获取用户信息失败"))
		return
	}

	// Generate full token
	token, err := middleware.GenerateToken(h.jwtSecret, user.ID, user.Username, string(user.Role), h.sessionTimeout)
	if err != nil {
		c.Error(ErrInternal.WithMessage("生成令牌失败"))
		return
	}

	// Create session
	ip := c.ClientIP()
	userAgent := c.Request.UserAgent()
	if h.sessionService != nil {
		if err := h.sessionService.RemoveUserSessions(c.Request.Context(), user.ID); err != nil { log.Printf("warning: remove old sessions: %v", err) }
		expiresAt := time.Now().Add(h.sessionTimeout)
		h.sessionService.CreateSession(c.Request.Context(), token, user.ID, user.Username, string(user.Role), ip, userAgent, expiresAt)
	}

	// Log successful login
	if h.auditService != nil {
		h.auditService.LogSecurityEvent(c.Request.Context(), user.Username, "LOGIN_SUCCESS_2FA",
			"User logged in with 2FA", ip, userAgent)
	}

	Success(c, gin.H{
		"token":            token,
		"user":             user,
		"must_change_pass": user.MustChangePass,
	})
}

// VerifyBackupCode handles backup code verification during login (step 2)
func (h *AuthHandler) VerifyBackupCode(c *gin.Context) {
	var req BackupCodeVerifyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(ErrBadRequest.Wrap(err))
		return
	}

	// Validate temp token
	userID, err := middleware.ValidateTOTPTempToken(h.jwtSecret, req.TempToken)
	if err != nil {
		c.Error(ErrUnauthorized.WithMessage("临时令牌无效或已过期"))
		return
	}

	// Verify backup code
	valid, err := h.authService.VerifyBackupCode(c.Request.Context(), userID, req.BackupCode)
	if err != nil {
		c.Error(ErrInternal.WithMessage("验证备用码失败"))
		return
	}
	if !valid {
		// Log failed backup code verification
		if h.auditService != nil {
			user, _ := h.authService.GetUserByID(c.Request.Context(), userID)
			if user != nil {
				h.auditService.LogSecurityEvent(c.Request.Context(), user.Username, "BACKUP_CODE_VERIFICATION_FAILED",
					"Invalid backup code", c.ClientIP(), c.Request.UserAgent())
			}
		}
		c.Error(ErrUnauthorized.WithMessage("备用码无效"))
		return
	}

	// Get user info
	user, err := h.authService.GetUserByID(c.Request.Context(), userID)
	if err != nil {
		c.Error(ErrInternal.WithMessage("获取用户信息失败"))
		return
	}

	// Generate full token
	token, err := middleware.GenerateToken(h.jwtSecret, user.ID, user.Username, string(user.Role), h.sessionTimeout)
	if err != nil {
		c.Error(ErrInternal.WithMessage("生成令牌失败"))
		return
	}

	// Create session
	ip := c.ClientIP()
	userAgent := c.Request.UserAgent()
	if h.sessionService != nil {
		if err := h.sessionService.RemoveUserSessions(c.Request.Context(), user.ID); err != nil { log.Printf("warning: remove old sessions: %v", err) }
		expiresAt := time.Now().Add(h.sessionTimeout)
		h.sessionService.CreateSession(c.Request.Context(), token, user.ID, user.Username, string(user.Role), ip, userAgent, expiresAt)
	}

	// Log successful login with backup code
	if h.auditService != nil {
		h.auditService.LogSecurityEvent(c.Request.Context(), user.Username, "LOGIN_SUCCESS_BACKUP_CODE",
			"User logged in with backup code", ip, userAgent)
	}

	Success(c, gin.H{
		"token":            token,
		"user":             user,
		"must_change_pass": user.MustChangePass,
	})
}

// SetupTOTP generates TOTP setup information (QR code, secret)
func (h *AuthHandler) SetupTOTP(c *gin.Context) {
	userID := c.GetInt64("user_id")
	username, _ := c.Get("username")

	// Check if TOTP is already enabled
	enabled, err := h.authService.IsTOTPEnabled(c.Request.Context(), userID)
	if err != nil {
		c.Error(ErrInternal.WithMessage("检查 TOTP 状态失败"))
		return
	}
	if enabled {
		c.Error(ErrBadRequest.WithMessage("TOTP 已启用"))
		return
	}

	// Generate TOTP setup
	unameStr, _ := username.(string)
	result, err := h.authService.GenerateTOTP(userID, unameStr)
	if err != nil {
		c.Error(ErrInternal.WithMessage("生成 TOTP 设置失败"))
		return
	}

	// Store the secret temporarily (totp_enabled = 0, secret stored for verification)
	err = h.authService.StorePendingSecret(c.Request.Context(), userID, result.Secret)
	if err != nil {
		c.Error(ErrInternal.WithMessage("存储 TOTP 密钥失败"))
		return
	}

	Success(c, result)
}

// EnableTOTP enables 2FA for the user
func (h *AuthHandler) EnableTOTP(c *gin.Context) {
	userID := c.GetInt64("user_id")
	username, _ := c.Get("username")

	var req TOTPEnableRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(ErrBadRequest.Wrap(err))
		return
	}

	// Get the pending secret from setup step
	secret, err := h.authService.GetPendingSecret(c.Request.Context(), userID)
	if err != nil {
		// If no pending secret, need to setup first
		c.Error(ErrBadRequest.WithMessage("请先设置 TOTP"))
		return
	}

	// Enable TOTP
	backupCodes, err := h.authService.EnableTOTP(c.Request.Context(), userID, secret, req.Code)
	if err != nil {
		// Log failed enable attempt
		if h.auditService != nil {
			if unameStr, ok := username.(string); ok {
				h.auditService.LogSecurityEvent(c.Request.Context(), unameStr, "TOTP_ENABLE_FAILED",
					err.Error(), c.ClientIP(), c.Request.UserAgent())
			}
		}
		c.Error(ErrBadRequest.Wrap(err))
		return
	}

	// Log successful enable
	if h.auditService != nil {
		if unameStr, ok := username.(string); ok {
			h.auditService.LogSecurityEvent(c.Request.Context(), unameStr, "TOTP_ENABLED",
				"2FA enabled successfully", c.ClientIP(), c.Request.UserAgent())
		}
	}

	Success(c, gin.H{
		"backup_codes": backupCodes,
	})
}

// DisableTOTP disables 2FA for the user
func (h *AuthHandler) DisableTOTP(c *gin.Context) {
	userID := c.GetInt64("user_id")
	username, _ := c.Get("username")

	var req TOTPDisableRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(ErrBadRequest.Wrap(err))
		return
	}

	// Disable TOTP
	if err := h.authService.DisableTOTP(c.Request.Context(), userID, req.Password); err != nil {
		// Log failed disable attempt
		if h.auditService != nil {
			if unameStr, ok := username.(string); ok {
				h.auditService.LogSecurityEvent(c.Request.Context(), unameStr, "TOTP_DISABLE_FAILED",
					err.Error(), c.ClientIP(), c.Request.UserAgent())
			}
		}
		c.Error(ErrBadRequest.Wrap(err))
		return
	}

	// Log successful disable
	if h.auditService != nil {
		if unameStr, ok := username.(string); ok {
			h.auditService.LogSecurityEvent(c.Request.Context(), unameStr, "TOTP_DISABLED",
				"2FA disabled successfully", c.ClientIP(), c.Request.UserAgent())
		}
	}

	Success(c, nil)
}

// GetTOTPStatus returns the TOTP status for the user
func (h *AuthHandler) GetTOTPStatus(c *gin.Context) {
	userID := c.GetInt64("user_id")

	enabled, err := h.authService.IsTOTPEnabled(c.Request.Context(), userID)
	if err != nil {
		c.Error(ErrInternal.WithMessage("检查 TOTP 状态失败"))
		return
	}

	Success(c, gin.H{
		"enabled": enabled,
	})
}

// GetSessions returns all active sessions for the current user
func (h *AuthHandler) GetSessions(c *gin.Context) {
	userID := c.GetInt64("user_id")

	sessions, err := h.sessionService.GetUserSessions(c.Request.Context(), userID)
	if err != nil {
		c.Error(ErrInternal.WithMessage("获取会话列表失败"))
		return
	}

	if sessions == nil {
		sessions = []model.Session{}
	}

	Success(c, sessions)
}

// KickSession removes a specific session (kick other device)
func (h *AuthHandler) KickSession(c *gin.Context) {
	userID := c.GetInt64("user_id")
	currentToken := c.GetHeader("Authorization")
	if len(currentToken) > 7 && currentToken[:7] == "Bearer " {
		currentToken = currentToken[7:]
	}

	var req struct {
		Token string `json:"token" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(ErrBadRequest.Wrap(err))
		return
	}

	// Prevent kicking yourself
	if req.Token == currentToken {
		c.Error(ErrBadRequest.WithMessage("不能踢出自己的会话，请使用登出功能"))
		return
	}

	// Verify the session belongs to the current user
	sessions, err := h.sessionService.GetUserSessions(c.Request.Context(), userID)
	if err != nil {
		c.Error(ErrInternal.WithMessage("获取会话列表失败"))
		return
	}

	found := false
	for _, sess := range sessions {
		if sess.Token == req.Token {
			found = true
			break
		}
	}
	if !found {
		c.Error(ErrNotFound.WithMessage("会话不存在"))
		return
	}

	// Remove the session
	if err := h.sessionService.RemoveSessionByToken(c.Request.Context(), req.Token); err != nil {
		c.Error(ErrInternal.WithMessage("删除会话失败"))
		return
	}

	// Log the action
	if h.auditService != nil {
		if uname, ok := c.Get("username"); ok {
			if unameStr, ok := uname.(string); ok {
				h.auditService.LogSecurityEvent(c.Request.Context(), unameStr, "SESSION_KICKED",
					"Kicked session", c.ClientIP(), c.Request.UserAgent())
			}
		}
	}

	Success(c, gin.H{"message": "会话已踢出"})
}

// KickAllOtherSessions removes all sessions except the current one
func (h *AuthHandler) KickAllOtherSessions(c *gin.Context) {
	userID := c.GetInt64("user_id")
	currentToken := c.GetHeader("Authorization")
	if len(currentToken) > 7 && currentToken[:7] == "Bearer " {
		currentToken = currentToken[7:]
	}

	// Remove all other sessions
	if err := h.sessionService.RemoveOtherSessions(c.Request.Context(), userID, currentToken); err != nil {
		c.Error(ErrInternal.WithMessage("删除会话失败"))
		return
	}

	// Log the action
	if h.auditService != nil {
		if uname, ok := c.Get("username"); ok {
			if unameStr, ok := uname.(string); ok {
				h.auditService.LogSecurityEvent(c.Request.Context(), unameStr, "ALL_OTHER_SESSIONS_KICKED",
					"Kicked all other sessions", c.ClientIP(), c.Request.UserAgent())
			}
		}
	}

	Success(c, gin.H{"message": "已踢出所有其他会话"})
}

func registerAuthRoutes(
	api *gin.RouterGroup,
	authService *auth.AuthService,
	auditService *audit.Service,
	sessionService *auth.SessionService,
	jwtSecret string,
	sessionValidator func(string) (bool, error),
	tokenValidator func(int64, string, time.Time) (bool, error),
	sessionTimeout time.Duration,
) {
	// Public auth routes
	auth := api.Group("/auth")
	authHandler := NewAuthHandler(authService, jwtSecret, auditService, sessionService, sessionTimeout)
	{
		auth.POST("/login", authHandler.Login)
		auth.POST("/verify-totp", authHandler.VerifyTOTP)
		auth.POST("/verify-backup", authHandler.VerifyBackupCode)
	}

	// Protected auth routes
	authProtected := api.Group("/auth")
	authProtected.Use(middleware.JWTMiddleware(jwtSecret, sessionValidator, tokenValidator))
	{
		authProtected.POST("/logout", authHandler.Logout)
		authProtected.GET("/me", authHandler.GetProfile)
		authProtected.POST("/change-password", authHandler.ChangePassword)
		authProtected.POST("/totp/setup", authHandler.SetupTOTP)
		authProtected.POST("/totp/enable", authHandler.EnableTOTP)
		authProtected.POST("/totp/disable", authHandler.DisableTOTP)
		authProtected.GET("/totp/status", authHandler.GetTOTPStatus)
		// Session management
		authProtected.GET("/sessions", authHandler.GetSessions)
		authProtected.POST("/sessions/kick", authHandler.KickSession)
		authProtected.POST("/sessions/kick-all", authHandler.KickAllOtherSessions)
	}
}

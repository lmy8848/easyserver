package api

import (
	"log"
	"time"

	"easyserver/internal/api/middleware"
	"easyserver/internal/audit"
	"easyserver/internal/auth"
	"easyserver/internal/infra/config"
	"easyserver/internal/qrlogin"

	"github.com/gin-gonic/gin"
)

type AuthHandler struct {
	authService    *auth.AuthService
	auditService   *audit.Service
	sessionService *auth.SessionService
	jwtSecret      string
	sessionTimeout time.Duration
	cfg            *config.Config
}

func NewAuthHandler(authService *auth.AuthService, jwtSecret string, auditService *audit.Service, sessionService *auth.SessionService, sessionTimeout time.Duration, cfg *config.Config) *AuthHandler {
	return &AuthHandler{
		authService:    authService,
		auditService:   auditService,
		sessionService: sessionService,
		jwtSecret:      jwtSecret,
		sessionTimeout: sessionTimeout,
		cfg:            cfg,
	}
}

// auditSecurity logs a security event for the authenticated user. Auth routes
// are mounted outside the AuditMiddleware group, so sensitive operations
// (logout/totp/password) record themselves. summary is a human-readable Chinese description.
func (h *AuthHandler) auditSecurity(c *gin.Context, summary string) {
	if h.auditService == nil {
		return
	}
	uname, _ := c.Get("username")
	unameStr, _ := uname.(string)
	h.auditService.LogSecurityEvent(c.Request.Context(), unameStr, summary)
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
	var req struct {
		Username       string `json:"username" binding:"required"`
		Password       string `json:"password" binding:"required"`
		TurnstileToken string `json:"turnstile_token"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(ErrBadRequest.Wrap(err))
		return
	}

	// Verify Cloudflare Turnstile challenge if enabled for login.
	if h.cfg.Turnstile.EnableLogin && !verifier.Verify(c.Request.Context(), h.cfg.Turnstile.SecretKey, req.TurnstileToken, c.ClientIP()) {
		c.Error(ErrForbidden.WithMessage("人机验证失败,请重试"))
		return
	}

	ip := c.ClientIP()
	userAgent := c.Request.UserAgent()

	// Use LoginWithInfo to log activity
	user, err := h.authService.LoginWithInfo(c.Request.Context(), req.Username, req.Password, ip, userAgent)
	if err != nil {
		c.Error(ErrUnauthorized.WithMessage(err.Error()))
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
		tempToken, err := auth.GenerateTOTPTempToken(h.jwtSecret, user.ID)
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
	token, err := auth.GenerateToken(h.jwtSecret, user.ID, user.Username, string(user.Role), h.sessionTimeout)
	if err != nil {
		c.Error(ErrInternal.WithMessage("生成令牌失败"))
		return
	}

	// Create session (single session per user - remove old sessions first)
	if h.sessionService != nil {
		// Remove all existing sessions for this user
		if err := h.sessionService.RemoveUserSessions(c.Request.Context(), user.ID); err != nil {
			log.Printf("warning: remove old sessions: %v", err)
		}
		// Create new session
		expiresAt := time.Now().Add(h.sessionTimeout)
		h.sessionService.CreateSession(c.Request.Context(), token, user.ID, user.Username, string(user.Role), ip, userAgent, expiresAt)
	}

	// Also log to audit log
	if h.auditService != nil {
		h.auditService.LogSecurityEvent(c.Request.Context(), req.Username, "登录成功 (IP: "+ip+")")
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

	h.auditSecurity(c, "退出登录")
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

	var req ChangePasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(ErrBadRequest.Wrap(err))
		return
	}

	if err := h.authService.ChangePassword(c.Request.Context(), userID, req.OldPassword, req.NewPassword); err != nil {
		c.Error(ErrBadRequest.WithMessage(err.Error()))
		return
	}

	// Invalidate all existing tokens for this user after password change
	// This forces re-authentication on all devices
	if err := h.authService.InvalidateAllUserTokens(c.Request.Context(), userID); err != nil {
		log.Printf("auth: failed to invalidate tokens after password change for user %d: %v", userID, err)
	}

	// Also remove all sessions so sessionValidator rejects old tokens immediately
	// (token invalidation is fail-closed but sessions are the primary check).
	if h.sessionService != nil {
		if err := h.sessionService.RemoveUserSessions(c.Request.Context(), userID); err != nil {
			log.Printf("auth: failed to remove sessions after password change for user %d: %v", userID, err)
		}
	}

	h.auditSecurity(c, "修改密码")
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
	var req struct {
		TOTPVerifyRequest
		TurnstileToken string `json:"turnstile_token"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(ErrBadRequest.Wrap(err))
		return
	}

	if h.cfg.Turnstile.EnableLogin && !verifier.Verify(c.Request.Context(), h.cfg.Turnstile.SecretKey, req.TurnstileToken, c.ClientIP()) {
		c.Error(ErrForbidden.WithMessage("人机验证失败,请重试"))
		return
	}

	// Validate temp token
	userID, err := auth.ValidateTOTPTempToken(h.jwtSecret, req.TempToken)
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
	token, err := auth.GenerateToken(h.jwtSecret, user.ID, user.Username, string(user.Role), h.sessionTimeout)
	if err != nil {
		c.Error(ErrInternal.WithMessage("生成令牌失败"))
		return
	}

	// Create session
	ip := c.ClientIP()
	userAgent := c.Request.UserAgent()
	if h.sessionService != nil {
		if err := h.sessionService.RemoveUserSessions(c.Request.Context(), user.ID); err != nil {
			log.Printf("warning: remove old sessions: %v", err)
		}
		expiresAt := time.Now().Add(h.sessionTimeout)
		h.sessionService.CreateSession(c.Request.Context(), token, user.ID, user.Username, string(user.Role), ip, userAgent, expiresAt)
	}

	// Log successful login
	if h.auditService != nil {
		h.auditService.LogSecurityEvent(c.Request.Context(), user.Username, "两步验证登录成功 (IP: "+c.ClientIP()+")")
	}

	Success(c, gin.H{
		"token":            token,
		"user":             user,
		"must_change_pass": user.MustChangePass,
	})
}

// VerifyBackupCode handles backup code verification during login (step 2)
func (h *AuthHandler) VerifyBackupCode(c *gin.Context) {
	var req struct {
		BackupCodeVerifyRequest
		TurnstileToken string `json:"turnstile_token"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(ErrBadRequest.Wrap(err))
		return
	}

	if h.cfg.Turnstile.EnableLogin && !verifier.Verify(c.Request.Context(), h.cfg.Turnstile.SecretKey, req.TurnstileToken, c.ClientIP()) {
		c.Error(ErrForbidden.WithMessage("人机验证失败,请重试"))
		return
	}

	// Validate temp token
	userID, err := auth.ValidateTOTPTempToken(h.jwtSecret, req.TempToken)
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
	token, err := auth.GenerateToken(h.jwtSecret, user.ID, user.Username, string(user.Role), h.sessionTimeout)
	if err != nil {
		c.Error(ErrInternal.WithMessage("生成令牌失败"))
		return
	}

	// Create session
	ip := c.ClientIP()
	userAgent := c.Request.UserAgent()
	if h.sessionService != nil {
		if err := h.sessionService.RemoveUserSessions(c.Request.Context(), user.ID); err != nil {
			log.Printf("warning: remove old sessions: %v", err)
		}
		expiresAt := time.Now().Add(h.sessionTimeout)
		h.sessionService.CreateSession(c.Request.Context(), token, user.ID, user.Username, string(user.Role), ip, userAgent, expiresAt)
	}

	// Log successful login with backup code
	if h.auditService != nil {
		h.auditService.LogSecurityEvent(c.Request.Context(), user.Username, "备用码登录成功 (IP: "+c.ClientIP()+")")
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
	unameStr, _ := c.Get("username")
	result, err := h.authService.GenerateTOTP(userID, unameStr.(string))
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
		c.Error(ErrBadRequest.Wrap(err))
		return
	}

	h.auditSecurity(c, "开启二次验证")
	Success(c, gin.H{
		"backup_codes": backupCodes,
	})
}

// DisableTOTP disables 2FA for the user
func (h *AuthHandler) DisableTOTP(c *gin.Context) {
	userID := c.GetInt64("user_id")

	var req TOTPDisableRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(ErrBadRequest.Wrap(err))
		return
	}

	// Disable TOTP
	if err := h.authService.DisableTOTP(c.Request.Context(), userID, req.Password); err != nil {
		c.Error(ErrBadRequest.Wrap(err))
		return
	}

	h.auditSecurity(c, "关闭二次验证")
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
		sessions = []auth.Session{}
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
				h.auditService.LogSecurityEvent(c.Request.Context(), unameStr, "踢出会话")
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
				h.auditService.LogSecurityEvent(c.Request.Context(), unameStr, "下线其他设备")
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
	qrService *qrlogin.Service,
	jwtSecret string,
	sessionValidator func(string) (bool, error),
	tokenValidator func(int64, string, time.Time) (bool, error),
	sessionTimeout time.Duration,
	loginRateLimit int,
	loginRateInterval time.Duration,
	cfg *config.Config,
) {
	// Public auth routes (no JWT required) — Tier 3: strict login limiter
	auth := api.Group("/auth")
	auth.Use(middleware.RateLimitMiddleware("login", loginRateLimit, loginRateInterval))
	authHandler := NewAuthHandler(authService, jwtSecret, auditService, sessionService, sessionTimeout, cfg)
	{
		auth.POST("/login", authHandler.Login)
		auth.POST("/verify-totp", authHandler.VerifyTOTP)
		auth.POST("/verify-backup", authHandler.VerifyBackupCode)
		// Public Turnstile config (no secret key): site key + enabled flows.
		auth.GET("/turnstile/config", func(c *gin.Context) {
			Success(c, gin.H{
				"site_key":            cfg.Turnstile.SiteKey,
				"enable_login":        cfg.Turnstile.EnableLogin,
				"enable_qr_login":     cfg.Turnstile.EnableQRLogin,
				"enable_public_share": cfg.Turnstile.EnablePublicShare,
			})
		})
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

	// Scan-to-login: public (rate-limited) session/status/cancel + protected confirm.
	registerQRLoginRoutes(auth, authProtected, qrService, authService, auditService)
}

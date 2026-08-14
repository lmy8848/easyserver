package http

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"time"

	"easyserver/internal/auth"
	"easyserver/internal/httpx"
	"easyserver/internal/httpx/middleware"
	"easyserver/internal/infra"
	"easyserver/internal/infra/config"
	"easyserver/internal/infra/errx"
	"easyserver/internal/qrlogin"

	"github.com/gin-gonic/gin"
)

// SecurityEventLogger logs security-relevant events (login, TOTP verification).
// Used by public auth routes that run outside the AuditMiddleware group.
type SecurityEventLogger interface {
	LogSecurityEvent(ctx context.Context, username, summary string)
}

// AuthAuditLogger combines security event logging (for public auth routes)
// with request/operation logging (for AuditMiddleware on protected routes).
// *audit.Service satisfies this interface implicitly.
type AuthAuditLogger interface {
	SecurityEventLogger
	middleware.RequestLogger
}

type AuthHandler struct {
	authService    *auth.AuthService
	auditLog       AuthAuditLogger
	sessionService *auth.SessionService
	jwtSecret      string
	sessionTimeout time.Duration
	cfg            *config.Config
}

func NewAuthHandler(authService *auth.AuthService, jwtSecret string, auditLog AuthAuditLogger, sessionService *auth.SessionService, sessionTimeout time.Duration, cfg *config.Config) *AuthHandler {
	return &AuthHandler{
		authService:    authService,
		auditLog:       auditLog,
		sessionService: sessionService,
		jwtSecret:      jwtSecret,
		sessionTimeout: sessionTimeout,
		cfg:            cfg,
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

// setAuthCookie 把登录态 JWT 写入 HttpOnly cookie。无条件对所有登录设置：
// 移动端原生 App 忽略 cookie（用响应体 token），无副作用。
func setAuthCookie(c *gin.Context, token string, maxAge time.Duration) {
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     middleware.AuthCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		Secure:   c.Request.TLS != nil, // 按协议动态：http 部署也能用，https 自动加
		MaxAge:   int(maxAge / time.Second),
	})
}

// clearAuthCookie 登出时清除登录态 cookie。
func clearAuthCookie(c *gin.Context) {
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     middleware.AuthCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   -1,
	})
}

// isWebClient 判断登录来源：空/mobile 之外的 web 客户端。仅用于决定响应体是否含
// token（web 不含进 cookie，mobile 含供 App 存 header）。
func isWebClient(clientType string) bool {
	return clientType == "" || clientType == "web"
}

func (h *AuthHandler) Login(c *gin.Context) (any, error) {
	var req struct {
		Username       string `json:"username" binding:"required"`
		Password       string `json:"password" binding:"required"`
		TurnstileToken string `json:"turnstile_token"`
		ClientType     string `json:"client_type"`
		DeviceID       string `json:"device_id"`
		DeviceInfo     string `json:"device_info"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		return nil, errx.BadRequest("invalid request: %w", err)
	}

	// Verify Cloudflare Turnstile challenge if enabled for login.
	if h.cfg.Server.Turnstile.EnableLogin && !infra.Default.Verify(c.Request.Context(), h.cfg.Server.Turnstile.SecretKey, req.TurnstileToken, c.ClientIP()) {
		return nil, errx.Forbidden("人机验证失败,请重试")
	}

	ip := c.ClientIP()
	userAgent := c.Request.UserAgent()

	// Use LoginWithInfo to log activity
	user, err := h.authService.LoginWithInfo(c.Request.Context(), req.Username, req.Password, ip, userAgent)
	if err != nil {
		return nil, errx.Unauthorized("%s", err.Error())
	}

	// Check if TOTP is enabled for this user
	totpEnabled, err := h.authService.IsTOTPEnabled(c.Request.Context(), user.ID)
	if err != nil {
		return nil, errx.Internal("检查 TOTP 状态失败")
	}

	// If TOTP is enabled, return a temporary token for TOTP verification
	if totpEnabled {
		tempToken, err := auth.GenerateTOTPTempToken(h.jwtSecret, user.ID)
		if err != nil {
			return nil, errx.Internal("生成临时令牌失败")
		}

		return gin.H{
			"requires_totp": true,
			"temp_token":    tempToken,
		}, nil
	}

	// If TOTP is not enabled, proceed with normal login
	token, err := auth.GenerateToken(h.jwtSecret, user.ID, user.Username, string(user.Role), h.sessionTimeout)
	if err != nil {
		return nil, errx.Internal("生成令牌失败")
	}

	// Create session. In single-session mode (default) a new login evicts the
	// user's other sessions; when AllowMultiSession is on they coexist so mobile
	// and web can stay logged in at the same time.
	if h.sessionService != nil {
		if !h.cfg.Auth.AllowMultiSession {
			if err := h.sessionService.RemoveUserSessions(c.Request.Context(), user.ID); err != nil {
				log.Printf("warning: remove old sessions: %v", err)
			}
		}
		clientType := req.ClientType
		if clientType == "" {
			clientType = "web"
		}
		expiresAt := time.Now().Add(h.sessionTimeout)
		sess := &auth.Session{
			UserID:     user.ID,
			Username:   user.Username,
			Role:       string(user.Role),
			IP:         ip,
			UserAgent:  userAgent,
			ClientType: clientType,
			DeviceID:   req.DeviceID,
			DeviceInfo: req.DeviceInfo,
			ExpiresAt:  expiresAt,
			Token:      token,
		}
		if err := h.createSessionWithBinding(c, sess); err != nil {
			return nil, err
		}
	}

	// Also log to audit log
	if h.auditLog != nil {
		h.auditLog.LogSecurityEvent(c.Request.Context(), req.Username, "登录成功 (IP: "+ip+")")
	}

	// 无条件 Set-Cookie（移动端原生 App 忽略 cookie，用响应体 token）。
	setAuthCookie(c, token, h.sessionTimeout)

	// 响应体是否含 token 按客户端区分：web 不含（HttpOnly 防已登录 token 进 JS，
	// 纵深防御为主——伪造 client_type 拿到的也只是登录者自己凭据的 token）；
	// mobile 含（App 需 header token）。
	if isWebClient(req.ClientType) {
		return gin.H{
			"user":             user,
			"must_change_pass": user.MustChangePass,
		}, nil
	}
	return gin.H{
		"token":            token,
		"user":             user,
		"must_change_pass": user.MustChangePass,
	}, nil
}

func (h *AuthHandler) Logout(c *gin.Context) (any, error) {
	// 中间件已把 token 放进 context（header 或 cookie 来源）。
	// 登录态吊销由 session 层完成（RemoveSession），单 token 黑名单已移除（死代码）。
	tokenStr := c.GetString("token")
	if tokenStr != "" && h.sessionService != nil {
		// Remove session → sessionValidator 立即拒绝该 token
		if err := h.sessionService.RemoveSession(c.Request.Context(), tokenStr); err != nil {
			log.Printf("auth: failed to remove session on logout: %v", err)
		}
	}

	// 清除登录态 cookie（幂等，web 登出必走）
	clearAuthCookie(c)

	middleware.AuditSummary(c, "退出登录")
	return nil, nil
}

func (h *AuthHandler) GetProfile(c *gin.Context) (any, error) {
	userID := c.GetInt64("user_id")
	if userID == 0 {
		return nil, errx.Unauthorized("未登录")
	}

	// Get user from database
	user, err := h.authService.GetUserByID(c.Request.Context(), userID)
	if err != nil {
		return nil, errx.Internal("获取用户信息失败")
	}

	return user, nil
}

func (h *AuthHandler) ChangePassword(c *gin.Context) (any, error) {
	userID := c.GetInt64("user_id")

	var req ChangePasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		return nil, errx.BadRequest("invalid request: %w", err)
	}

	if err := h.authService.ChangePassword(c.Request.Context(), userID, req.OldPassword, req.NewPassword); err != nil {
		return nil, errx.BadRequest("%s", err.Error())
	}

	// 改密码后删除该用户所有 session，旧 token 立即失效（吊销只靠 session 表）。
	if h.sessionService != nil {
		if err := h.sessionService.RemoveUserSessions(c.Request.Context(), userID); err != nil {
			log.Printf("auth: failed to remove sessions after password change for user %d: %v", userID, err)
		}
	}

	middleware.AuditSummary(c, "修改密码")
	return nil, nil
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
func (h *AuthHandler) VerifyTOTP(c *gin.Context) (any, error) {
	var req struct {
		TOTPVerifyRequest
		TurnstileToken string `json:"turnstile_token"`
		ClientType     string `json:"client_type"`
		DeviceID       string `json:"device_id"`
		DeviceInfo     string `json:"device_info"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		return nil, errx.BadRequest("invalid request: %w", err)
	}

	if h.cfg.Server.Turnstile.EnableLogin && !infra.Default.Verify(c.Request.Context(), h.cfg.Server.Turnstile.SecretKey, req.TurnstileToken, c.ClientIP()) {
		return nil, errx.Forbidden("人机验证失败,请重试")
	}

	// Validate temp token
	userID, err := auth.ValidateTOTPTempToken(h.jwtSecret, req.TempToken)
	if err != nil {
		return nil, errx.Unauthorized("临时令牌无效或已过期")
	}

	// Get user's TOTP secret
	secret, err := h.authService.GetTOTPSecret(c.Request.Context(), userID)
	if err != nil {
		return nil, errx.Unauthorized("该用户未启用 TOTP")
	}

	// Verify TOTP code
	if !h.authService.VerifyTOTP(secret, req.Code) {
		return nil, errx.Unauthorized("TOTP 验证码无效")
	}

	// Get user info
	user, err := h.authService.GetUserByID(c.Request.Context(), userID)
	if err != nil {
		return nil, errx.Internal("获取用户信息失败")
	}

	// Generate full token
	token, err := auth.GenerateToken(h.jwtSecret, user.ID, user.Username, string(user.Role), h.sessionTimeout)
	if err != nil {
		return nil, errx.Internal("生成令牌失败")
	}

	// Create session
	ip := c.ClientIP()
	userAgent := c.Request.UserAgent()
	if h.sessionService != nil {
		if !h.cfg.Auth.AllowMultiSession {
			if err := h.sessionService.RemoveUserSessions(c.Request.Context(), user.ID); err != nil {
				log.Printf("warning: remove old sessions: %v", err)
			}
		}
		clientType := req.ClientType
		if clientType == "" {
			clientType = "web"
		}
		expiresAt := time.Now().Add(h.sessionTimeout)
		sess := &auth.Session{
			UserID:     user.ID,
			Username:   user.Username,
			Role:       string(user.Role),
			IP:         ip,
			UserAgent:  userAgent,
			ClientType: clientType,
			DeviceID:   req.DeviceID,
			DeviceInfo: req.DeviceInfo,
			ExpiresAt:  expiresAt,
			Token:      token,
		}
		if err := h.createSessionWithBinding(c, sess); err != nil {
			return nil, err
		}
	}

	// Log successful login
	if h.auditLog != nil {
		h.auditLog.LogSecurityEvent(c.Request.Context(), user.Username, "两步验证登录成功 (IP: "+c.ClientIP()+")")
	}

	setAuthCookie(c, token, h.sessionTimeout)
	if isWebClient(req.ClientType) {
		return gin.H{
			"user":             user,
			"must_change_pass": user.MustChangePass,
		}, nil
	}
	return gin.H{
		"token":            token,
		"user":             user,
		"must_change_pass": user.MustChangePass,
	}, nil
}

// VerifyBackupCode handles backup code verification during login (step 2)
func (h *AuthHandler) VerifyBackupCode(c *gin.Context) (any, error) {
	var req struct {
		BackupCodeVerifyRequest
		TurnstileToken string `json:"turnstile_token"`
		ClientType     string `json:"client_type"`
		DeviceID       string `json:"device_id"`
		DeviceInfo     string `json:"device_info"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		return nil, errx.BadRequest("invalid request: %w", err)
	}

	if h.cfg.Server.Turnstile.EnableLogin && !infra.Default.Verify(c.Request.Context(), h.cfg.Server.Turnstile.SecretKey, req.TurnstileToken, c.ClientIP()) {
		return nil, errx.Forbidden("人机验证失败,请重试")
	}

	// Validate temp token
	userID, err := auth.ValidateTOTPTempToken(h.jwtSecret, req.TempToken)
	if err != nil {
		return nil, errx.Unauthorized("临时令牌无效或已过期")
	}

	// Verify backup code
	valid, err := h.authService.VerifyBackupCode(c.Request.Context(), userID, req.BackupCode)
	if err != nil {
		return nil, errx.Internal("验证备用码失败")
	}
	if !valid {
		// Log failed backup code verification
		return nil, errx.Unauthorized("备用码无效")
	}

	// Get user info
	user, err := h.authService.GetUserByID(c.Request.Context(), userID)
	if err != nil {
		return nil, errx.Internal("获取用户信息失败")
	}

	// Generate full token
	token, err := auth.GenerateToken(h.jwtSecret, user.ID, user.Username, string(user.Role), h.sessionTimeout)
	if err != nil {
		return nil, errx.Internal("生成令牌失败")
	}

	// Create session
	ip := c.ClientIP()
	userAgent := c.Request.UserAgent()
	if h.sessionService != nil {
		if !h.cfg.Auth.AllowMultiSession {
			if err := h.sessionService.RemoveUserSessions(c.Request.Context(), user.ID); err != nil {
				log.Printf("warning: remove old sessions: %v", err)
			}
		}
		clientType := req.ClientType
		if clientType == "" {
			clientType = "web"
		}
		expiresAt := time.Now().Add(h.sessionTimeout)
		sess := &auth.Session{
			UserID:     user.ID,
			Username:   user.Username,
			Role:       string(user.Role),
			IP:         ip,
			UserAgent:  userAgent,
			ClientType: clientType,
			DeviceID:   req.DeviceID,
			DeviceInfo: req.DeviceInfo,
			ExpiresAt:  expiresAt,
			Token:      token,
		}
		if err := h.createSessionWithBinding(c, sess); err != nil {
			return nil, err
		}
	}

	// Log successful login with backup code
	if h.auditLog != nil {
		h.auditLog.LogSecurityEvent(c.Request.Context(), user.Username, "备用码登录成功 (IP: "+c.ClientIP()+")")
	}

	setAuthCookie(c, token, h.sessionTimeout)
	if isWebClient(req.ClientType) {
		return gin.H{
			"user":             user,
			"must_change_pass": user.MustChangePass,
		}, nil
	}
	return gin.H{
		"token":            token,
		"user":             user,
		"must_change_pass": user.MustChangePass,
	}, nil
}

// SetupTOTP generates TOTP setup information (QR code, secret)
func (h *AuthHandler) SetupTOTP(c *gin.Context) (any, error) {
	userID := c.GetInt64("user_id")

	// Check if TOTP is already enabled
	enabled, err := h.authService.IsTOTPEnabled(c.Request.Context(), userID)
	if err != nil {
		return nil, errx.Internal("检查 TOTP 状态失败")
	}
	if enabled {
		return nil, errx.BadRequest("TOTP 已启用")
	}

	// Generate TOTP setup
	unameStr, _ := c.Get("username")
	result, err := h.authService.GenerateTOTP(userID, unameStr.(string))
	if err != nil {
		return nil, errx.Internal("生成 TOTP 设置失败")
	}

	// Store the secret temporarily (totp_enabled = 0, secret stored for verification)
	err = h.authService.StorePendingSecret(c.Request.Context(), userID, result.Secret)
	if err != nil {
		return nil, errx.Internal("存储 TOTP 密钥失败")
	}

	return result, nil
}

// EnableTOTP enables 2FA for the user
func (h *AuthHandler) EnableTOTP(c *gin.Context) (any, error) {
	userID := c.GetInt64("user_id")

	var req TOTPEnableRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		return nil, errx.BadRequest("invalid request: %w", err)
	}

	// Get the pending secret from setup step
	secret, err := h.authService.GetPendingSecret(c.Request.Context(), userID)
	if err != nil {
		// If no pending secret, need to setup first
		return nil, errx.BadRequest("请先设置 TOTP")
	}

	// Enable TOTP
	backupCodes, err := h.authService.EnableTOTP(c.Request.Context(), userID, secret, req.Code)
	if err != nil {
		return nil, errx.BadRequest("%w", err)
	}

	middleware.AuditSummary(c, "开启二次验证")
	return gin.H{
		"backup_codes": backupCodes,
	}, nil
}

// DisableTOTP disables 2FA for the user
func (h *AuthHandler) DisableTOTP(c *gin.Context) (any, error) {
	userID := c.GetInt64("user_id")

	var req TOTPDisableRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		return nil, errx.BadRequest("invalid request: %w", err)
	}

	// Disable TOTP
	if err := h.authService.DisableTOTP(c.Request.Context(), userID, req.Password); err != nil {
		return nil, errx.BadRequest("%w", err)
	}

	middleware.AuditSummary(c, "关闭二次验证")
	return nil, nil
}

// GetTOTPStatus returns the TOTP status for the user
func (h *AuthHandler) GetTOTPStatus(c *gin.Context) (any, error) {
	userID := c.GetInt64("user_id")

	enabled, err := h.authService.IsTOTPEnabled(c.Request.Context(), userID)
	if err != nil {
		return nil, errx.Internal("检查 TOTP 状态失败")
	}

	return gin.H{
		"enabled": enabled,
	}, nil
}

// createSessionWithBinding creates a session, enforcing the mobile single-device
// binding when enabled. For mobile logins with binding on: requires device_id
// (M2), delegates to CreateMobileSessionBound (atomic check+replace, H1/M3); on
// rejection it audits + emits a login-blocked notification (M1) and returns 403.
// Other logins go through CreateSession with the error checked (H2). device_id
// is a client-reported soft identifier, not a security boundary (M5).
func (h *AuthHandler) createSessionWithBinding(c *gin.Context, sess *auth.Session) error {
	if h.sessionService == nil {
		return nil
	}
	if sess.ClientType == "mobile" && h.cfg.Auth.MobileDeviceBinding {
		if sess.DeviceID == "" {
			return errx.BadRequest("移动端登录缺少设备标识 device_id")
		}
		if err := h.sessionService.CreateMobileSessionBound(c.Request.Context(), sess); err != nil {
			if errors.Is(err, auth.ErrMobileDeviceBound) {
				if h.auditLog != nil {
					h.auditLog.LogSecurityEvent(c.Request.Context(), sess.Username, "移动端登录被拒(已有其他设备登录)")
				}
				if h.authService != nil {
					h.authService.NotifyLogin(auth.LoginEvent{
						Username:  sess.Username,
						IP:        sess.IP,
						UserAgent: sess.UserAgent,
						Time:      time.Now().Format(time.RFC3339),
						Success:   false,
						Reason:    "移动端绑定:已有其他设备登录",
					})
				}
				return errx.Forbidden("已有其他移动设备登录，请先在面板「会话管理」中解绑该设备后再试")
			}
			log.Printf("auth: create mobile session for user %d: %v", sess.UserID, err)
			return errx.Internal("创建会话失败")
		}
		return nil
	}
	if err := h.sessionService.CreateSession(c.Request.Context(), sess.Token, sess.UserID, sess.Username, sess.Role, sess.IP, sess.UserAgent, sess.ClientType, sess.DeviceID, sess.DeviceInfo, sess.ExpiresAt); err != nil {
		log.Printf("auth: create session for user %d: %v", sess.UserID, err)
		return errx.Internal("创建会话失败")
	}
	return nil
}

// GetSessions returns all active sessions for the current user. Each session is
// annotated with is_current (the calling token's hash matches) so the UI can
// hide the self-kick action.
func (h *AuthHandler) GetSessions(c *gin.Context) (any, error) {
	userID := c.GetInt64("user_id")

	sessions, err := h.sessionService.GetUserSessions(c.Request.Context(), userID)
	if err != nil {
		return nil, errx.Internal("获取会话列表失败")
	}

	currentToken := ""
	if raw, ok := c.Get("token"); ok {
		if tokenStr, ok := raw.(string); ok && tokenStr != "" {
			currentToken = tokenStr
		}
	}

	type sessionView struct {
		auth.Session
		IsCurrent bool `json:"is_current"`
	}
	views := make([]sessionView, 0, len(sessions))
	for _, s := range sessions {
		views = append(views, sessionView{Session: s, IsCurrent: currentToken != "" && s.Token == currentToken})
	}
	return views, nil
}

// KickSession removes a specific session (kick other device)
func (h *AuthHandler) KickSession(c *gin.Context) (any, error) {
	userID := c.GetInt64("user_id")
	var currentToken string
	if raw, ok := c.Get("token"); ok {
		if tokenStr, ok := raw.(string); ok {
			currentToken = tokenStr
		}
	}

	var req struct {
		Token string `json:"token" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		return nil, errx.BadRequest("invalid request: %w", err)
	}

	// Prevent kicking yourself (req.Token is the raw token from GetSessions).
	if currentToken != "" && req.Token == currentToken {
		return nil, errx.BadRequest("不能踢出自己的会话，请使用登出功能")
	}

	// Verify the session belongs to the current user
	sessions, err := h.sessionService.GetUserSessions(c.Request.Context(), userID)
	if err != nil {
		return nil, errx.Internal("获取会话列表失败")
	}

	found := false
	for _, sess := range sessions {
		if sess.Token == req.Token {
			found = true
			break
		}
	}
	if !found {
		return nil, errx.NotFound("会话不存在")
	}

	// Remove the session. req.Token is the stored hash (from GetSessions), so
	// delete by it as-is - do NOT re-hash (that would never match).
	if err := h.sessionService.RemoveSessionByStoredToken(c.Request.Context(), req.Token); err != nil {
		return nil, errx.Internal("删除会话失败")
	}

	middleware.AuditSummary(c, "踢出会话")

	return gin.H{"message": "会话已踢出"}, nil
}

// KickAllOtherSessions removes all sessions except the current one
func (h *AuthHandler) KickAllOtherSessions(c *gin.Context) (any, error) {
	userID := c.GetInt64("user_id")
	currentToken := c.GetString("token")

	// Remove all other sessions
	if err := h.sessionService.RemoveOtherSessions(c.Request.Context(), userID, currentToken); err != nil {
		return nil, errx.Internal("删除会话失败")
	}

	middleware.AuditSummary(c, "下线其他设备")

	return gin.H{"message": "已踢出所有其他会话"}, nil
}

type QRLoginHandler struct {
	qrService   *qrlogin.Service
	authService *auth.AuthService
	cfg         *config.Config
}

func NewQRLoginHandler(qrService *qrlogin.Service, authService *auth.AuthService, cfg *config.Config) *QRLoginHandler {
	return &QRLoginHandler{qrService: qrService, authService: authService, cfg: cfg}
}

// CreateQRSession starts a new pending scan-to-login session and returns the QR
// token + base64 PNG for the web to render.
func (h *QRLoginHandler) CreateQRSession(c *gin.Context) (any, error) {
	res, err := h.qrService.CreateSession(c.Request.Context())
	if err != nil {
		return nil, errx.Internal("%w", err)
	}
	return res, nil
}

// GetQRStatus is polled by the web client. On confirmed it returns the web
// login token + user payload (one-time; the session is consumed).
// Uses POST + body so the qr_token (a secret that redeems a web JWT) never
// lands in URL/access logs/Referer the way a query string would.
func (h *QRLoginHandler) GetQRStatus(c *gin.Context) (any, error) {
	var req qrlogin.ConfirmRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		return nil, errx.BadRequest("invalid request: %w", err)
	}
	if req.QRToken == "" {
		return nil, errx.BadRequest("缺少 qr_token")
	}
	c.Header("Cache-Control", "no-store")
	res, err := h.qrService.GetStatus(c.Request.Context(), req.QRToken)
	if err != nil {
		return nil, errx.Internal("%w", err)
	}
	// 确认后：web token 改走 HttpOnly cookie，不再回传 JS（防 XSS 窃取）。
	if res.Status == qrlogin.StatusConfirmed && res.Token != "" {
		setAuthCookie(c, res.Token, h.cfg.Auth.SessionTimeout)
		res.Token = ""
	}
	return res, nil
}

// ConfirmQRLogin is called by the authenticated mobile app after scanning. The
// mobile's JWT (validated by middleware) authorizes issuing a web session for
// the same admin user. Creates a coexisting session (mobile stays logged in).
func (h *QRLoginHandler) ConfirmQRLogin(c *gin.Context) (any, error) {
	var req struct {
		qrlogin.ConfirmRequest
		TurnstileToken string `json:"turnstile_token"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		return nil, errx.BadRequest("invalid request: %w", err)
	}

	if h.cfg.Server.Turnstile.EnableQRLogin && !infra.Default.Verify(c.Request.Context(), h.cfg.Server.Turnstile.SecretKey, req.TurnstileToken, c.ClientIP()) {
		return nil, errx.Forbidden("人机验证失败,请重试")
	}

	userID := c.GetInt64("user_id")
	if userID == 0 {
		return nil, errx.Unauthorized("未登录")
	}

	// Fetch the user to build the web login payload (mirrors password login).
	user, err := h.authService.GetUserByID(c.Request.Context(), userID)
	if err != nil {
		return nil, errx.Internal("获取用户信息失败")
	}

	// {user, must_change_pass} payload handed to the web client on pickup.
	payload, _ := json.Marshal(struct {
		User           any  `json:"user"`
		MustChangePass bool `json:"must_change_pass"`
	}{User: user, MustChangePass: user.MustChangePass})

	ip := c.ClientIP()
	userAgent := c.Request.UserAgent()
	if err := h.qrService.Confirm(c.Request.Context(), req.QRToken, user.ID, user.Username, string(user.Role), ip, userAgent, string(payload)); err != nil {
		switch {
		case errors.Is(err, qrlogin.ErrNotPending), errors.Is(err, qrlogin.ErrExpired):
			return nil, errx.BadRequest("%s", err.Error())
		default:
			return nil, errx.Internal("%w", err)
		}
	}

	middleware.AuditSummary(c, "扫码登录授权 (IP: "+ip+")")
	return gin.H{"ok": true}, nil
}

// CancelQRLogin dismisses a pending QR session.
func (h *QRLoginHandler) CancelQRLogin(c *gin.Context) (any, error) {
	var req qrlogin.ConfirmRequest
	_ = c.ShouldBindJSON(&req)
	if req.QRToken == "" {
		return nil, errx.BadRequest("缺少 qr_token")
	}
	_ = h.qrService.Cancel(c.Request.Context(), req.QRToken)
	return nil, nil
}

// registerQRLoginRoutes wires the scan-to-login endpoints onto the auth groups.
// publicAuth is the rate-limited public group; authProtected requires a JWT.
func registerQRLoginRoutes(publicAuth, authProtected *gin.RouterGroup, qrService *qrlogin.Service, authService *auth.AuthService, cfg *config.Config) {
	h := NewQRLoginHandler(qrService, authService, cfg)
	publicAuth.POST("/qr/session", httpx.H(h.CreateQRSession))
	publicAuth.POST("/qr/status", httpx.H(h.GetQRStatus))
	publicAuth.POST("/qr/cancel", httpx.H(h.CancelQRLogin))
	authProtected.POST("/qr/confirm", httpx.H(h.ConfirmQRLogin))
}

// RegisterRoutes wires auth routes onto the gin router group.
func RegisterRoutes(
	api *gin.RouterGroup,
	authService *auth.AuthService,
	auditLog AuthAuditLogger,
	sessionService *auth.SessionService,
	qrService *qrlogin.Service,
	jwtSecret string,
	sessionValidator func(string) (bool, error),
	sessionTimeout time.Duration,
	loginRateLimit int,
	loginRateInterval time.Duration,
	cfg *config.Config,
) {
	// Public auth routes (no JWT required) — Tier 3: strict login limiter
	authGroup := api.Group("/auth")
	authGroup.Use(middleware.RateLimitMiddleware("login", loginRateLimit, loginRateInterval))
	authHandler := NewAuthHandler(authService, jwtSecret, auditLog, sessionService, sessionTimeout, cfg)
	{
		authGroup.POST("/login", httpx.H(authHandler.Login))
		authGroup.POST("/verify-totp", httpx.H(authHandler.VerifyTOTP))
		authGroup.POST("/verify-backup", httpx.H(authHandler.VerifyBackupCode))
		// Public Turnstile config (no secret key): site key + enabled flows.
		authGroup.GET("/turnstile/config", httpx.H(func(c *gin.Context) (any, error) {
			return gin.H{
				"site_key":            cfg.Server.Turnstile.SiteKey,
				"enable_login":        cfg.Server.Turnstile.EnableLogin,
				"enable_qr_login":     cfg.Server.Turnstile.EnableQRLogin,
				"enable_public_share": cfg.Server.Turnstile.EnablePublicShare,
			}, nil
		}))
	}

	// Protected auth routes
	authProtected := api.Group("/auth")
	authProtected.Use(
		middleware.JWTMiddleware(jwtSecret, sessionValidator),
		middleware.AuditMiddleware(auditLog),
	)
	{
		authProtected.POST("/logout", httpx.H(authHandler.Logout))
		authProtected.GET("/me", httpx.H(authHandler.GetProfile))
		authProtected.POST("/change-password", httpx.H(authHandler.ChangePassword))
		authProtected.POST("/totp/setup", httpx.H(authHandler.SetupTOTP))
		authProtected.POST("/totp/enable", httpx.H(authHandler.EnableTOTP))
		authProtected.POST("/totp/disable", httpx.H(authHandler.DisableTOTP))
		authProtected.GET("/totp/status", httpx.H(authHandler.GetTOTPStatus))
		// Session management
		authProtected.GET("/sessions", httpx.H(authHandler.GetSessions))
		authProtected.POST("/sessions/kick", httpx.H(authHandler.KickSession))
		authProtected.POST("/sessions/kick-all", httpx.H(authHandler.KickAllOtherSessions))
	}

	// Scan-to-login: public (rate-limited) session/status/cancel + protected confirm.
	registerQRLoginRoutes(authGroup, authProtected, qrService, authService, cfg)
}

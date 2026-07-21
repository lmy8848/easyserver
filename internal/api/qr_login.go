package api

import (
	"encoding/json"

	"easyserver/internal/audit"
	"easyserver/internal/auth"
	"easyserver/internal/infra/config"
	"easyserver/internal/infra/turnstile"
	"easyserver/internal/qrlogin"

	"github.com/gin-gonic/gin"
)

// QRLoginHandler exposes the scan-to-login endpoints. Public routes (session
// create / status poll / cancel) are rate-limited alongside login; the confirm
// route requires the mobile app's JWT.
type QRLoginHandler struct {
	qrService    *qrlogin.Service
	auditService *audit.Service
	authService  *auth.AuthService
	cfg          *config.Config
}

func NewQRLoginHandler(qrService *qrlogin.Service, authService *auth.AuthService, auditService *audit.Service, cfg *config.Config) *QRLoginHandler {
	return &QRLoginHandler{qrService: qrService, authService: authService, auditService: auditService, cfg: cfg}
}

// CreateQRSession starts a new pending scan-to-login session and returns the QR
// token + base64 PNG for the web to render.
func (h *QRLoginHandler) CreateQRSession(c *gin.Context) {
	res, err := h.qrService.CreateSession(c.Request.Context())
	if err != nil {
		c.Error(ErrInternal.Wrap(err))
		return
	}
	Success(c, res)
}

// GetQRStatus is polled by the web client. On confirmed it returns the web
// login token + user payload (one-time; the session is consumed).
// Uses POST + body so the qr_token (a secret that redeems a web JWT) never
// lands in URL/access logs/Referer the way a query string would.
func (h *QRLoginHandler) GetQRStatus(c *gin.Context) {
	var req qrlogin.ConfirmRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(ErrBadRequest.Wrap(err))
		return
	}
	if req.QRToken == "" {
		c.Error(ErrBadRequest.WithMessage("缺少 qr_token"))
		return
	}
	c.Header("Cache-Control", "no-store")
	res, err := h.qrService.GetStatus(c.Request.Context(), req.QRToken)
	if err != nil {
		c.Error(ErrInternal.Wrap(err))
		return
	}
	Success(c, res)
}

// ConfirmQRLogin is called by the authenticated mobile app after scanning. The
// mobile's JWT (validated by middleware) authorizes issuing a web session for
// the same admin user. Creates a coexisting session (mobile stays logged in).
func (h *QRLoginHandler) ConfirmQRLogin(c *gin.Context) {
	var req struct {
		qrlogin.ConfirmRequest
		TurnstileToken string `json:"turnstile_token"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(ErrBadRequest.Wrap(err))
		return
	}

	if h.cfg.Server.Turnstile.EnableQRLogin && !turnstile.Default.Verify(c.Request.Context(), h.cfg.Server.Turnstile.SecretKey, req.TurnstileToken, c.ClientIP()) {
		c.Error(ErrForbidden.WithMessage("人机验证失败,请重试"))
		return
	}

	userID := c.GetInt64("user_id")
	if userID == 0 {
		c.Error(ErrUnauthorized.WithMessage("未登录"))
		return
	}

	// Fetch the user to build the web login payload (mirrors password login).
	user, err := h.authService.GetUserByID(c.Request.Context(), userID)
	if err != nil {
		c.Error(ErrInternal.WithMessage("获取用户信息失败"))
		return
	}

	// {user, must_change_pass} payload handed to the web client on pickup.
	payload, _ := json.Marshal(struct {
		User           any  `json:"user"`
		MustChangePass bool `json:"must_change_pass"`
	}{User: user, MustChangePass: user.MustChangePass})

	ip := c.ClientIP()
	userAgent := c.Request.UserAgent()
	if err := h.qrService.Confirm(c.Request.Context(), req.QRToken, user.ID, user.Username, string(user.Role), ip, userAgent, string(payload)); err != nil {
		switch err {
		case qrlogin.ErrNotPending, qrlogin.ErrExpired:
			c.Error(ErrBadRequest.WithMessage(err.Error()))
		default:
			c.Error(ErrInternal.Wrap(err))
		}
		return
	}

	if h.auditService != nil {
		h.auditService.LogSecurityEvent(c.Request.Context(), user.Username, "扫码登录授权 (IP: "+ip+")")
	}
	Success(c, gin.H{"ok": true})
}

// CancelQRLogin dismisses a pending QR session.
func (h *QRLoginHandler) CancelQRLogin(c *gin.Context) {
	var req qrlogin.ConfirmRequest
	_ = c.ShouldBindJSON(&req)
	if req.QRToken == "" {
		c.Error(ErrBadRequest.WithMessage("缺少 qr_token"))
		return
	}
	_ = h.qrService.Cancel(c.Request.Context(), req.QRToken)
	Success(c, nil)
}

// registerQRLoginRoutes wires the scan-to-login endpoints onto the auth groups.
// publicAuth is the rate-limited public group; authProtected requires a JWT.
func registerQRLoginRoutes(publicAuth, authProtected *gin.RouterGroup, qrService *qrlogin.Service, authService *auth.AuthService, auditService *audit.Service, cfg *config.Config) {
	h := NewQRLoginHandler(qrService, authService, auditService, cfg)
	publicAuth.POST("/qr/session", h.CreateQRSession)
	publicAuth.POST("/qr/status", h.GetQRStatus)
	publicAuth.POST("/qr/cancel", h.CancelQRLogin)
	authProtected.POST("/qr/confirm", h.ConfirmQRLogin)
}

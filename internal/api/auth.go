package api

import (
	"time"

	"easyserver/internal/middleware"
	"easyserver/internal/service"

	"github.com/gin-gonic/gin"
)

type AuthHandler struct {
	authService    *service.AuthService
	auditService   *service.AuditService
	sessionService *service.SessionService
	jwtSecret      string
}

func NewAuthHandler(authService *service.AuthService, jwtSecret string, auditService *service.AuditService, sessionService *service.SessionService) *AuthHandler {
	return &AuthHandler{
		authService:    authService,
		auditService:   auditService,
		sessionService: sessionService,
		jwtSecret:      jwtSecret,
	}
}

type LoginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

type ChangePasswordRequest struct {
	OldPassword string `json:"old_password" binding:"required"`
	NewPassword string `json:"new_password" binding:"required,min=6"`
}

func (h *AuthHandler) Login(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequest(c, err.Error())
		return
	}

	ip := c.ClientIP()
	userAgent := c.Request.UserAgent()

	// Use LoginWithInfo to log activity
	user, err := h.authService.LoginWithInfo(req.Username, req.Password, ip, userAgent)
	if err != nil {
		// Also log to audit log
		if h.auditService != nil {
			h.auditService.LogSecurityEvent(req.Username, "LOGIN_FAILED",
				err.Error(), ip, userAgent)
		}
		Unauthorized(c, err.Error())
		return
	}

	token, err := middleware.GenerateToken(h.jwtSecret, user.ID, user.Username, string(user.Role))
	if err != nil {
		InternalError(c, "failed to generate token")
		return
	}

	// Create session (single session per user - remove old sessions first)
	if h.sessionService != nil {
		// Remove all existing sessions for this user
		h.sessionService.RemoveUserSessions(user.ID)
		// Create new session
		expiresAt := time.Now().Add(24 * time.Hour)
		h.sessionService.CreateSession(token, user.ID, user.Username, string(user.Role), ip, userAgent, expiresAt)
	}

	// Also log to audit log
	if h.auditService != nil {
		h.auditService.LogSecurityEvent(req.Username, "LOGIN_SUCCESS",
			"User logged in successfully", ip, userAgent)
	}

	Success(c, gin.H{
		"token":            token,
		"user":             user,
		"must_change_pass": user.MustChangePass,
	})
}

func (h *AuthHandler) Logout(c *gin.Context) {
	// Remove session
	if h.sessionService != nil {
		token := c.GetHeader("Authorization")
		if len(token) > 7 && token[:7] == "Bearer " {
			h.sessionService.RemoveSession(token[7:])
		}
	}

	// Log logout
	if h.auditService != nil {
		username, _ := c.Get("username")
		if username != nil {
			h.auditService.LogSecurityEvent(username.(string), "LOGOUT",
				"User logged out", c.ClientIP(), c.Request.UserAgent())
		}
	}
	Success(c, nil)
}

func (h *AuthHandler) GetProfile(c *gin.Context) {
	userID := c.GetInt64("user_id")
	if userID == 0 {
		Unauthorized(c, "not authenticated")
		return
	}

	// Get user from database
	user, err := h.authService.GetUserByID(userID)
	if err != nil {
		InternalError(c, "failed to get user")
		return
	}

	Success(c, user)
}

func (h *AuthHandler) ChangePassword(c *gin.Context) {
	userID := c.GetInt64("user_id")
	username, _ := c.Get("username")

	var req ChangePasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequest(c, err.Error())
		return
	}

	if err := h.authService.ChangePassword(userID, req.OldPassword, req.NewPassword); err != nil {
		// Log failed password change
		if h.auditService != nil && username != nil {
			h.auditService.LogSecurityEvent(username.(string), "PASSWORD_CHANGE_FAILED",
				err.Error(), c.ClientIP(), c.Request.UserAgent())
		}
		BadRequest(c, err.Error())
		return
	}

	// Log successful password change
	if h.auditService != nil && username != nil {
		h.auditService.LogSecurityEvent(username.(string), "PASSWORD_CHANGED",
			"Password changed successfully", c.ClientIP(), c.Request.UserAgent())
	}

	Success(c, nil)
}

package api

import (
	"time"

	"easyserver/internal/middleware"
	"easyserver/internal/service"

	"github.com/gin-gonic/gin"
)

// registerAuthRoutes registers auth-related routes (public + protected)
func registerAuthRoutes(
	api *gin.RouterGroup,
	authService *service.AuthService,
	auditService *service.AuditService,
	sessionService *service.SessionService,
	totpService *service.TOTPService,
	jwtSecret string,
	sessionValidator func(string) (bool, error),
	tokenValidator func(int64, string, time.Time) (bool, error),
	sessionTimeout time.Duration,
) {
	// Public auth routes
	auth := api.Group("/auth")
	authHandler := NewAuthHandler(authService, jwtSecret, auditService, sessionService, totpService, sessionTimeout)
	{
		auth.POST("/login", authHandler.Login)
		auth.POST("/logout", authHandler.Logout)
		auth.POST("/verify-totp", authHandler.VerifyTOTP)
		auth.POST("/verify-backup", authHandler.VerifyBackupCode)
	}

	// Protected auth routes
	authProtected := api.Group("/auth")
	authProtected.Use(middleware.JWTMiddleware(jwtSecret, sessionValidator, tokenValidator))
	{
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

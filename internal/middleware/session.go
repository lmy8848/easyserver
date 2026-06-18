package middleware

import (
	"log"
	"strings"
	"time"

	"easyserver/internal/service"

	"github.com/gin-gonic/gin"
)

// SessionHeartbeatMiddleware updates the session's last_active on every request
// If session doesn't exist, creates one (for tokens obtained before session management)
func SessionHeartbeatMiddleware(sessionService *service.SessionService) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Get the token from Authorization header
		authHeader := c.GetHeader("Authorization")
		if authHeader != "" {
			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) == 2 && parts[0] == "Bearer" {
				token := parts[1]

				// Get user info from context (set by JWT middleware)
				// Use comma-ok assertion to avoid panic
				userID, _ := c.Get("user_id")
				username, _ := c.Get("username")
				role, _ := c.Get("role")

				if userID != nil && username != nil {
					// Type assert with comma-ok pattern
					uid, ok := userID.(int64)
					if !ok {
						log.Printf("session: invalid user_id type in context")
						c.Next()
						return
					}

					uname, ok := username.(string)
					if !ok {
						log.Printf("session: invalid username type in context")
						c.Next()
						return
					}

					// Role might not be set, use default if missing
					roleStr := "viewer"
					if role != nil {
						if r, ok := role.(string); ok {
							roleStr = r
						}
					}

					// Try to update existing session
					err := sessionService.UpdateActivity(token)
					if err != nil {
						// Session doesn't exist, create one
						go func() {
							ip := c.ClientIP()
							userAgent := c.Request.UserAgent()
							expiresAt := time.Now().Add(24 * time.Hour)
							sessionService.CreateSession(
								token,
								uid,
								uname,
								roleStr,
								ip,
								userAgent,
								expiresAt,
							)
						}()
					}
				}
			}
		}

		c.Next()
	}
}

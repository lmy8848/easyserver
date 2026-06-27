package middleware

import (
	"strings"

	"github.com/gin-gonic/gin"
)

// CORSMiddleware creates a CORS middleware with configurable allowed origins
func CORSMiddleware(allowedOrigins []string, devMode bool) gin.HandlerFunc {
	// Check if wildcard origin is configured
	hasWildcard := false
	for _, o := range allowedOrigins {
		if o == "*" {
			hasWildcard = true
			break
		}
	}

	return func(c *gin.Context) {
		origin := c.Request.Header.Get("Origin")

		// Check if origin is allowed
		allowed := false
		if devMode {
			// In dev mode, allow localhost (exact host check to prevent spoofing)
			if strings.HasPrefix(origin, "http://localhost:") || strings.HasPrefix(origin, "http://127.0.0.1:") {
				allowed = true
			}
		}

		if !allowed {
			for _, o := range allowedOrigins {
				if o == "*" || o == origin {
					allowed = true
					break
				}
			}
		}

		if allowed && origin != "" {
			c.Header("Access-Control-Allow-Origin", origin)
		}

		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization")

		// SECURITY: Do NOT set Allow-Credentials when wildcard origin is used.
		// Browsers reject responses with Allow-Origin=* + Allow-Credentials=true,
		// but reflecting the request origin with credentials enabled allows any
		// site to make credentialed cross-origin requests (CSRF risk).
		if !hasWildcard {
			c.Header("Access-Control-Allow-Credentials", "true")
		}
		c.Header("Access-Control-Max-Age", "86400")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	}
}

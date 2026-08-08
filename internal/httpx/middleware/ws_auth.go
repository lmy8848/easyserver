package middleware

import (
	"fmt"
	"strings"

	"easyserver/internal/infra/apperror"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

// WSAuthMiddleware creates a middleware for WebSocket authentication
// It supports token from:
// 1. Sec-WebSocket-Protocol header (preferred)
// 2. URL query parameter "token" (fallback, deprecated)
// 3. HttpOnly cookie (web, same-origin handshake carries it automatically)
func WSAuthMiddleware(secret string, sessionValidator SessionValidator) gin.HandlerFunc {
	return func(c *gin.Context) {
		var tokenString string

		// Extract token from Sec-WebSocket-Protocol header
		protocols := c.GetHeader("Sec-WebSocket-Protocol")
		if protocols != "" {
			// Protocol format: "token, <jwt-token>" or just "<jwt-token>"
			parts := strings.Split(protocols, ",")
			for _, part := range parts {
				part = strings.TrimSpace(part)
				if part != "" && part != "token" {
					tokenString = part
					break
				}
			}
		}

		// Fallback: HttpOnly cookie (web same-origin WS handshake)
		if tokenString == "" {
			if cookie, err := c.Cookie(AuthCookieName); err == nil && cookie != "" {
				tokenString = cookie
			}
		}

		if tokenString == "" {
			c.Error(apperror.ErrUnauthorized.WithMessage("missing token"))
			c.Abort()
			return
		}

		// Parse and validate JWT
		claims := &JWTClaims{}
		token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
			}
			return []byte(secret), nil
		})

		if err != nil || !token.Valid {
			c.Error(apperror.ErrUnauthorized.WithMessage("invalid or expired token"))
			c.Abort()
			return
		}

		// Check session validator (single session per user)
		if sessionValidator != nil {
			valid, err := sessionValidator(tokenString)
			if err != nil {
				c.Error(apperror.ErrInternal.WithMessage("session validation error"))
				c.Abort()
				return
			}
			if !valid {
				c.Error(apperror.ErrUnauthorized.WithMessage("session expired, please login again"))
				c.Abort()
				return
			}
		}

		// Set user info in context
		c.Set("user_id", claims.UserID)
		c.Set("username", claims.Username)
		c.Set("role", claims.Role)
		c.Next()
	}
}

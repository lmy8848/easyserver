package middleware

import (
	"fmt"
		"strings"

	"easyserver/internal/apperror"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

// WSAuthMiddleware creates a middleware for WebSocket authentication
// It supports token from:
// 1. Sec-WebSocket-Protocol header (preferred)
// 2. URL query parameter "token" (fallback, deprecated)
func WSAuthMiddleware(secret string, sessionValidator SessionValidator, validators ...TokenValidator) gin.HandlerFunc {
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

		// Check token validators (e.g., blacklist)
		for _, validator := range validators {
			if validator != nil {
				invalidated, err := validator(claims.UserID, tokenString, claims.IssuedAt.Time)
				if err != nil {
					c.Error(apperror.ErrInternal.WithMessage("token validation error"))
					c.Abort()
					return
				}
				if invalidated {
					c.Error(apperror.ErrUnauthorized.WithMessage("token has been revoked"))
					c.Abort()
					return
				}
			}
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

// RequireWSRole creates a middleware that checks if the WebSocket connection
// has one of the required roles
func RequireWSRole(roles ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		role, exists := c.Get("role")
		if !exists {
			c.Error(apperror.ErrUnauthorized.WithMessage("role not found"))
			c.Abort()
			return
		}

		roleStr, ok := role.(string)
		if !ok {
			c.Error(apperror.ErrUnauthorized.WithMessage("invalid role format"))
			c.Abort()
			return
		}

		for _, allowedRole := range roles {
			if roleStr == allowedRole {
				c.Next()
				return
			}
		}

		c.Error(apperror.ErrUnauthorized.WithMessage("insufficient permissions"))
		c.Abort()
	}
}

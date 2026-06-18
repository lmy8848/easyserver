package middleware

import (
	"fmt"
	"net/http"
	"strings"

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

		// Try Sec-WebSocket-Protocol header first (preferred method)
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

		// Fallback to query parameter (deprecated)
		if tokenString == "" {
			tokenString = c.Query("token")
			if tokenString != "" {
				// Log deprecation warning
				// TODO: Add proper logging/metrics for deprecated usage
			}
		}

		if tokenString == "" {
			c.JSON(http.StatusUnauthorized, gin.H{
				"code":    40100,
				"message": "missing token",
				"data":    nil,
			})
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
			c.JSON(http.StatusUnauthorized, gin.H{
				"code":    40101,
				"message": "invalid or expired token",
				"data":    nil,
			})
			c.Abort()
			return
		}

		// Check token validators (e.g., blacklist)
		for _, validator := range validators {
			if validator != nil {
				invalidated, err := validator(claims.UserID, tokenString, claims.IssuedAt.Time)
				if err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{
						"code":    50000,
						"message": "token validation error",
						"data":    nil,
					})
					c.Abort()
					return
				}
				if invalidated {
					c.JSON(http.StatusUnauthorized, gin.H{
						"code":    40102,
						"message": "token has been revoked",
						"data":    nil,
					})
					c.Abort()
					return
				}
			}
		}

		// Check session validator (single session per user)
		if sessionValidator != nil {
			valid, err := sessionValidator(tokenString)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{
					"code":    50000,
					"message": "session validation error",
					"data":    nil,
				})
				c.Abort()
				return
			}
			if !valid {
				c.JSON(http.StatusUnauthorized, gin.H{
					"code":    40103,
					"message": "session expired, please login again",
					"data":    nil,
				})
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
			c.JSON(http.StatusForbidden, gin.H{
				"code":    40300,
				"message": "role not found",
				"data":    nil,
			})
			c.Abort()
			return
		}

		roleStr, ok := role.(string)
		if !ok {
			c.JSON(http.StatusForbidden, gin.H{
				"code":    40300,
				"message": "invalid role format",
				"data":    nil,
			})
			c.Abort()
			return
		}

		for _, allowedRole := range roles {
			if roleStr == allowedRole {
				c.Next()
				return
			}
		}

		c.JSON(http.StatusForbidden, gin.H{
			"code":    40301,
			"message": "insufficient permissions",
			"data":    nil,
		})
		c.Abort()
	}
}

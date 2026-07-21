package middleware

import (
	"fmt"
	"strings"

	"easyserver/internal/auth"
	"easyserver/internal/infra/apperror"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

func JWTMiddleware(secret string, sessionValidator auth.SessionValidator, validators ...auth.TokenValidator) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.Error(apperror.ErrUnauthorized.WithMessage("missing authorization header"))
			c.Abort()
			return
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || parts[0] != "Bearer" {
			c.Error(apperror.ErrUnauthorized.WithMessage("invalid authorization format"))
			c.Abort()
			return
		}

		tokenString := parts[1]
		claims := &auth.JWTClaims{}
		token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
			}
			return []byte(secret), nil
		})

		if err != nil || !token.Valid {
			c.Error(apperror.ErrTokenExpired.WithMessage("invalid or expired token"))
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

		c.Set("user_id", claims.UserID)
		c.Set("username", claims.Username)
		c.Set("role", claims.Role)
		c.Set("token", tokenString)
		c.Next()
	}
}

package middleware

import (
	"easyserver/internal/infra/apperror"
	"github.com/gin-gonic/gin"
)

// RequireRole creates a middleware that checks if the user has one of the required roles
func RequireRole(roles ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		role, exists := c.Get("role")
		if !exists {
			c.Error(apperror.ErrForbidden.WithMessage("role not found"))
			c.Abort()
			return
		}

		roleStr, ok := role.(string)
		if !ok {
			c.Error(apperror.ErrForbidden.WithMessage("invalid role format"))
			c.Abort()
			return
		}

		for _, allowedRole := range roles {
			if roleStr == allowedRole {
				c.Next()
				return
			}
		}

		c.Error(apperror.ErrForbidden.WithMessage("insufficient permissions"))
		c.Abort()
	}
}

package middleware

import (
	"easyserver/internal/infra/apperror"
	"github.com/gin-gonic/gin"
)

// RequireSingleAdmin ensures only admin role can access (single-admin mode fallback)
func RequireSingleAdmin() gin.HandlerFunc {
	return func(c *gin.Context) {
		role, exists := c.Get("role")
		if !exists {
			c.Error(apperror.ErrForbidden.WithMessage("single-admin mode, role missing"))
			c.Abort()
			return
		}
		roleStr, ok := role.(string)
		if !ok || roleStr != "admin" {
			c.Error(apperror.ErrForbidden.WithMessage("single-admin mode, admin only"))
			c.Abort()
			return
		}
		c.Next()
	}
}

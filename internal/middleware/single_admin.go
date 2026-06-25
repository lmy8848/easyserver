package middleware

import "github.com/gin-gonic/gin"

// RequireSingleAdmin ensures only admin role can access (single-admin mode fallback)
func RequireSingleAdmin() gin.HandlerFunc {
	return func(c *gin.Context) {
		role, exists := c.Get("role")
		if !exists {
			c.JSON(403, gin.H{"code": 40300, "message": "single-admin mode, role missing"})
			c.Abort()
			return
		}
		roleStr, ok := role.(string)
		if !ok || roleStr != "admin" {
			c.JSON(403, gin.H{"code": 40300, "message": "single-admin mode, admin only"})
			c.Abort()
			return
		}
		c.Next()
	}
}

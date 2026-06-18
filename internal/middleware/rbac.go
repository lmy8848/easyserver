package middleware

import (
	"github.com/gin-gonic/gin"
)

func RequireRole(roles ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		role, exists := c.Get("role")
		if !exists {
			c.JSON(403, gin.H{
				"code":    40300,
				"message": "forbidden",
				"data":    nil,
			})
			c.Abort()
			return
		}

		roleStr, ok := role.(string)
		if !ok {
			c.JSON(403, gin.H{
				"code":    40300,
				"message": "forbidden",
				"data":    nil,
			})
			c.Abort()
			return
		}

		for _, r := range roles {
			if roleStr == r {
				c.Next()
				return
			}
		}

		c.JSON(403, gin.H{
			"code":    40300,
			"message": "insufficient permissions",
			"data":    nil,
		})
		c.Abort()
	}
}
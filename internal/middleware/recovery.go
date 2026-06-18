package middleware

import (
	"log"
	"net/http"
	"runtime/debug"

	"github.com/gin-gonic/gin"
)

// RecoveryMiddleware recovers from panics and returns 500 error
func RecoveryMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if err := recover(); err != nil {
				log.Printf("Panic recovered: %v\n%s", err, debug.Stack())

				c.JSON(http.StatusInternalServerError, gin.H{
					"code":    50000,
					"message": "internal server error",
				})
				c.Abort()
			}
		}()

		c.Next()
	}
}

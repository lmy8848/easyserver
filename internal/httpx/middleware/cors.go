package middleware

import (
	"net/http"
	"slices"

	"github.com/gin-gonic/gin"
)

// CORSMiddleware creates a CORS middleware with configurable allowed origins
func CORSMiddleware(allowedOrigins []string, devMode bool) gin.HandlerFunc {
	// Check if wildcard origin is configured
	hasWildcard := slices.Contains(allowedOrigins, "*")

	return func(c *gin.Context) {
		origin := c.Request.Header.Get("Origin")

		// Check if origin is allowed
		var allowed bool
		if devMode || hasWildcard {
			// 在开发模式下，或者配置了通配符，允许任意域名的跨域请求
			allowed = true
		} else {
			// 与 CSRF 共用 trustedOrigin 规则：本机回环源默认信任，其余源需显式配置。
			allowed = trustedOrigin(origin, "", allowedOrigins)
		}

		if allowed && origin != "" {
			c.Header("Access-Control-Allow-Origin", origin)
			// Credentials 仅在放行该源时声明，避免对非白名单源虚设。
			// （Allow-Origin 缺失时浏览器本就拒绝发送，credentials 虚设无实际危害，
			//  但语义上只对放行的源声明更精确。）
			c.Header("Access-Control-Allow-Credentials", "true")
		}

		c.Header("Access-Control-Allow-Methods", "GET, HEAD, POST, PUT, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Requested-With")
		c.Header("Access-Control-Max-Age", "86400")

		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	}
}

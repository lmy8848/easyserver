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
			// 通配 + credentials 组合在浏览器规范下不成立（带凭据时 * 无效）。
			// 此前对任意 Origin 反射 + credentials，等于把面板数据开放给
			// 受害者浏览器访问的任意网站；生产模式回字面 *（浏览器自动拒绝
			// 凭据请求），仅 devMode 保留反射便于本地联调。
			if hasWildcard && !devMode {
				c.Header("Access-Control-Allow-Origin", "*")
			} else {
				c.Header("Access-Control-Allow-Origin", origin)
				// Credentials 仅在放行该源时声明，避免对非白名单源虚设。
				c.Header("Access-Control-Allow-Credentials", "true")
			}
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

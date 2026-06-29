package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// CSRFMiddleware rejects state-changing requests that lack the X-Requested-With
// header set by the browser for same-origin XMLHttpRequest/fetch calls. This
// prevents cross-site form submissions and naive CSRF attacks without needing
// a synchronizer token.
func CSRFMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Method == "GET" || c.Request.Method == "HEAD" || c.Request.Method == "OPTIONS" {
			c.Next()
			return
		}
		if c.GetHeader("X-Requested-With") != "XMLHttpRequest" {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"code":    40300,
				"message": "CSRF validation failed: missing X-Requested-With header",
			})
			return
		}
		c.Next()
	}
}

// SecurityMiddleware adds security headers with a pre-generated CSP nonce.
// The nonce must be injected into <script> tags at build/startup time.
func SecurityMiddleware(nonce string) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Prevent clickjacking
		c.Header("X-Frame-Options", "DENY")

		// Prevent MIME sniffing
		c.Header("X-Content-Type-Options", "nosniff")

		// Referrer policy
		c.Header("Referrer-Policy", "strict-origin-when-cross-origin")

		// Content Security Policy
		// - script-src uses nonce to allow only our scripts (production mode only)
		// - style-src uses unsafe-inline because Ant Design CSS-in-JS requires inline styles
		scriptSrc := "'self'"
		if nonce != "" {
			scriptSrc += " 'nonce-" + nonce + "'"
		}
		c.Header("Content-Security-Policy",
			"default-src 'self'; "+
				"script-src "+scriptSrc+"; "+
				"style-src 'self' 'unsafe-inline'; "+
				"img-src 'self' data: blob:; "+
				"font-src 'self' data:; "+
				"connect-src 'self' ws: wss:; "+
				"worker-src 'self' blob:; "+
				"frame-ancestors 'none'; "+
				"base-uri 'self'; "+
				"form-action 'self';",
		)

		// Strict Transport Security (only for HTTPS)
		if c.Request.TLS != nil {
			c.Header("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}

		c.Next()
	}
}

const (
	// DefaultMaxBodySize is the maximum request body size for JSON/POST requests.
	DefaultMaxBodySize = 10 << 20 // 10 MB
)

// MaxBodySizeMiddleware limits request body size to prevent memory exhaustion
// from oversized JSON payloads.
func MaxBodySizeMiddleware(maxSize int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Body != nil {
			c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxSize)
		}
		c.Next()
	}
}

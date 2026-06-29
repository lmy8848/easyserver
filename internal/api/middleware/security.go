package middleware

import (
	"github.com/gin-gonic/gin"
)

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

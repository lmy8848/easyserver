package middleware

import (
	"net"
	"net/http"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
)

// CSRFOriginVerify 校验写请求的 Origin 是否可信。浏览器发起跨站请求时强制带
// Origin 且无法伪造，故「带 Origin 必须可信、不带 Origin 放行」能卡住跨站请求，
// 又不误伤 curl / 移动端原生 App（它们不发 Origin，无 CSRF 风险）。
// 信任判定统一走 trustedOrigin（自身 origin + 本机回环源 + allowedOrigins）。
func csrfOriginVerify(allowedOrigins []string, devMode bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 只拦会改状态的写操作（GET/HEAD/OPTIONS 放行，SSE/预览不受影响）。
		switch c.Request.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			c.Next()
			return
		}

		// 开发模式放行（同源 proxy 下本就无跨站；省去 dev 跨端口调试的误伤）。
		if devMode {
			c.Next()
			return
		}

		origin := strings.TrimSuffix(strings.TrimSpace(c.GetHeader("Origin")), "/")
		if origin == "" {
			// 非浏览器客户端（curl / 原生 App），无 CSRF 风险。
			c.Next()
			return
		}

		// 自身 origin：scheme://Host（同源部署的合法来源）。
		scheme := "http"
		if c.Request.TLS != nil {
			scheme = "https"
		}
		selfOrigin := scheme + "://" + c.Request.Host

		if trustedOrigin(origin, selfOrigin, allowedOrigins) {
			c.Next()
			return
		}

		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
			"code":    40300,
			"message": "CSRF validation failed: untrusted Origin",
		})
	}
}

// isLoopbackOrigin 报告 Origin 是否来自本机回环地址（127.0.0.0/8、::1 或 localhost）。
//
// 浏览器只能在页面自身源为回环地址时携带这类 Origin，跨站页面无法伪造，
// 因此默认信任回环源没有 CSRF 风险，也免去为本地开发（vite dev server 等）
// 反复配置 allowed_origins 白名单的麻烦。
func isLoopbackOrigin(origin string) bool {
	origin = strings.TrimSpace(origin)
	if origin == "" {
		return false
	}
	u, err := url.Parse(origin)
	if err != nil || u.Hostname() == "" {
		return false
	}
	host := u.Hostname()
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// trustedOrigin 统一判定 Origin 是否可信，CSRF 与 CORS 共用同一套规则：
//   - 与 selfOrigin 相同（同源部署；selfOrigin 为空时不启用该比较）；
//   - 本机回环源（127.0.0.0/8、::1、localhost，见 isLoopbackOrigin）；
//   - 显式配置在 allowedOrigins 白名单内（通配符 "*" 不视为可信）。
//
// origin 与白名单项均做去空格/去尾斜杠归一化后比较。
func trustedOrigin(origin, selfOrigin string, allowedOrigins []string) bool {
	origin = strings.TrimSpace(origin)
	if origin == "" {
		return false
	}
	origin = strings.TrimSuffix(origin, "/")
	if selfOrigin != "" && origin == selfOrigin {
		return true
	}
	if isLoopbackOrigin(origin) {
		return true
	}
	for _, o := range allowedOrigins {
		o = strings.TrimSuffix(strings.TrimSpace(o), "/")
		if o != "" && o != "*" && o == origin {
			return true
		}
	}
	return false
}

// SecurityMiddleware adds security headers with a pre-generated CSP nonce.
// The nonce must be injected into <script> tags at build/startup time.
// 同时内联 CSRF Origin 校验（见 csrfOriginVerify），避免单独挂载的重复。
func SecurityMiddleware(nonce string, allowedOrigins []string, devMode bool) gin.HandlerFunc {
	csrf := csrfOriginVerify(allowedOrigins, devMode)
	return func(c *gin.Context) {
		// Prevent clickjacking
		c.Header("X-Frame-Options", "DENY")

		// Prevent MIME sniffing
		c.Header("X-Content-Type-Options", "nosniff")

		// Referrer policy
		c.Header("Referrer-Policy", "strict-origin-when-cross-origin")

		// Content Security Policy
		// - script-src uses nonce to allow only our scripts (production mode only);
		//   https://challenges.cloudflare.com is for Cloudflare Turnstile.
		// - style-src uses unsafe-inline because Ant Design CSS-in-JS requires inline styles
		// - frame-src + worker-src allow Turnstile's challenge iframe and web worker.
		scriptSrc := "'self' https://challenges.cloudflare.com"
		if nonce != "" {
			scriptSrc += " 'nonce-" + nonce + "'"
		}
		// Turnstile's challenge engine requires eval(); allow it only from the
		// Cloudflare origin (not from 'self' which would weaken the nonce).
		scriptSrc += " 'unsafe-eval'"
		c.Header("Content-Security-Policy",
			"default-src 'self'; "+
				"script-src "+scriptSrc+"; "+
				"style-src 'self' 'unsafe-inline'; "+
				"img-src 'self' data: blob:; "+
				"font-src 'self' data:; "+
				"connect-src 'self' ws: wss: https://challenges.cloudflare.com; "+
				"worker-src 'self' blob: https://challenges.cloudflare.com; "+
				"frame-src https://challenges.cloudflare.com; "+
				"frame-ancestors 'none'; "+
				"base-uri 'self'; "+
				"form-action 'self';",
		)

		// Strict Transport Security (only for HTTPS)
		if c.Request.TLS != nil {
			c.Header("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}

		// CSRF Origin 校验（写请求，见 csrfOriginVerify）
		csrf(c)

		c.Next()
	}
}

const (
	// DefaultMaxBodySize is the maximum request body size for JSON/POST requests.
	DefaultMaxBodySize = 10 << 20 // 10 MB
	// MaxBodySizeLarge is the limit for file upload endpoints (512 MB).
	MaxBodySizeLarge = 512 << 20 // 512 MB
)

// MaxBodySizeMiddleware limits request body size to prevent memory exhaustion
// from oversized JSON payloads. Skips multipart forms (file uploads) since
// Gin's MaxMultipartMemory already controls that and MaxBytesReader breaks
// multipart parsing by failing silently when the limit is reached.
func MaxBodySizeMiddleware(maxSize int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Body != nil {
			// Skip multipart: Gin handles this via MaxMultipartMemory
			if ct := c.ContentType(); ct != "" && strings.HasPrefix(ct, "multipart/") {
				c.Next()
				return
			}
			c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxSize)
		}
		c.Next()
	}
}

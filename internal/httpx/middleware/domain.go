package middleware

import (
	"net"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// DomainRedirectMiddleware redirects requests to the canonical domain when forceDomain is true.
func DomainRedirectMiddleware(domain string, forceDomain bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !forceDomain || domain == "" {
			c.Next()
			return
		}

		// Skip WebSocket upgrades
		if c.GetHeader("Upgrade") == "websocket" {
			c.Next()
			return
		}

		// Skip health check
		if c.Request.URL.Path == "/health" {
			c.Next()
			return
		}

		host := c.Request.Host
		if h, _, err := net.SplitHostPort(host); err == nil {
			host = h
		}

		if !strings.EqualFold(host, domain) {
			target := "https://" + domain + c.Request.URL.RequestURI()
			c.Redirect(http.StatusMovedPermanently, target)
			c.Abort()
			return
		}

		c.Next()
	}
}

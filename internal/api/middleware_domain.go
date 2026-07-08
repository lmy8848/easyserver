package api

import (
	"net"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// DomainRedirectMiddleware enforces canonical domain redirect rules based on
// RedirectMode and WwwHandling settings.
//
// RedirectMode:
//   - "off": no redirect
//   - "ip_only": redirect only when Host is an IP address
//   - "non_matching": redirect when Host doesn't match Domain (includes IP)
//
// WwwHandling:
//   - "off": no normalization
//   - "force_www": redirect non-www to www.domain
//   - "remove_www": redirect www.domain to domain
func DomainRedirectMiddleware(domain, redirectMode, wwwHandling string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if redirectMode == "" || redirectMode == "off" {
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
		hostLower := strings.ToLower(host)
		domainLower := strings.ToLower(domain)

		shouldRedirect := false

		switch redirectMode {
		case "ip_only":
			if net.ParseIP(host) != nil {
				shouldRedirect = true
			}
		case "non_matching":
			if domain != "" && !strings.EqualFold(host, domain) {
				shouldRedirect = true
			}
		}

		// www normalization takes precedence (applied after domain redirect target)
		wwwTarget := ""
		if wwwHandling == "force_www" && domain != "" {
			wwwDomain := "www." + domainLower
			if hostLower != wwwDomain && hostLower != domainLower {
				// Host is neither www nor bare domain → redirect to www first
				shouldRedirect = true
				wwwTarget = "https://" + wwwDomain + c.Request.URL.RequestURI()
			} else if hostLower == domainLower {
				// bare domain → force www
				shouldRedirect = true
				wwwTarget = "https://" + wwwDomain + c.Request.URL.RequestURI()
			}
		} else if wwwHandling == "remove_www" && domain != "" {
			if strings.HasPrefix(hostLower, "www.") {
				shouldRedirect = true
				wwwTarget = "https://" + domain + c.Request.URL.RequestURI()
			}
		}

		if shouldRedirect {
			target := wwwTarget
			if target == "" {
				// Default: redirect to canonical domain (or https if domain set)
				if domain != "" {
					target = "https://" + domain + c.Request.URL.RequestURI()
				} else {
					target = "https://" + host + c.Request.URL.RequestURI()
				}
			}
			c.Redirect(http.StatusMovedPermanently, target)
			c.Abort()
			return
		}

		c.Next()
	}
}

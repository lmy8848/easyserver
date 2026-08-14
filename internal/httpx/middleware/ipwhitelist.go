package middleware

import (
	"net"
	"strings"
	"sync"

	"easyserver/internal/infra/errx"

	"github.com/gin-gonic/gin"
)

type IPWhitelist struct {
	mu      sync.RWMutex
	allowed []*net.IPNet
	enabled bool
}

func NewIPWhitelist(allowedIPs []string) *IPWhitelist {
	wl := &IPWhitelist{
		enabled: len(allowedIPs) > 0,
	}
	wl.Update(allowedIPs)
	return wl
}

func (wl *IPWhitelist) IsAllowed(ip string) bool {
	if !wl.enabled {
		return true
	}

	clientIP := net.ParseIP(ip)
	if clientIP == nil {
		return false
	}

	wl.mu.RLock()
	defer wl.mu.RUnlock()

	for _, ipNet := range wl.allowed {
		if ipNet.Contains(clientIP) {
			return true
		}
	}

	return false
}

func (wl *IPWhitelist) Update(allowed []string) {
	wl.mu.Lock()
	defer wl.mu.Unlock()

	wl.allowed = nil
	wl.enabled = len(allowed) > 0

	for _, cidr := range allowed {
		_, ipNet, err := net.ParseCIDR(cidr)
		if err != nil {
			ip := net.ParseIP(cidr)
			if ip != nil {
				// Single IP, convert to /32 or /128
				bits := 32
				if ip.To4() == nil {
					bits = 128
				}
				mask := net.CIDRMask(bits, bits)
				ipNet = &net.IPNet{IP: ip, Mask: mask}
			}
		}
		if ipNet != nil {
			wl.allowed = append(wl.allowed, ipNet)
		}
	}
}

// IPWhitelistMiddleware checks client IP against whitelist
func IPWhitelistMiddleware(whitelist *IPWhitelist) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !whitelist.IsAllowed(c.ClientIP()) {
			c.Error(errx.Forbidden("IP not allowed"))
			c.Abort()
			return
		}

		c.Next()
	}
}

// UserIPWhitelistFunc is a callback that returns the user's IP whitelist string by user ID.
// Returns empty string if user has no whitelist (allow all).
type UserIPWhitelistFunc func(userID int64) (string, error)

// UserIPWhitelistMiddleware checks client IP against the per-user IP whitelist.
// Must run after JWTMiddleware (needs user_id in context).
func UserIPWhitelistMiddleware(getWhitelist UserIPWhitelistFunc) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, exists := c.Get("user_id")
		if !exists {
			c.Next()
			return
		}

		uid, ok := userID.(int64)
		if !ok {
			c.Next()
			return
		}
		whitelistStr, err := getWhitelist(uid)
		if err != nil {
			// On error, don't block — let request through
			c.Next()
			return
		}

		// Empty whitelist means allow all
		if whitelistStr == "" {
			c.Next()
			return
		}

		clientIP := c.ClientIP()
		allowed := false
		for _, ip := range splitAndTrim(whitelistStr) {
			if matchIP(clientIP, ip) {
				allowed = true
				break
			}
		}

		if !allowed {
			c.Error(errx.Forbidden("your IP is not in the user whitelist"))
			c.Abort()
			return
		}

		c.Next()
	}
}

// splitAndTrim splits a comma-separated string and trims whitespace
func splitAndTrim(s string) []string {
	parts := []string{}
	for part := range strings.SplitSeq(s, ",") {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			parts = append(parts, trimmed)
		}
	}
	return parts
}

// matchIP checks if clientIP matches an allowed IP pattern (exact IP or CIDR)
func matchIP(clientIP, pattern string) bool {
	// Try CIDR match
	if strings.Contains(pattern, "/") {
		_, ipNet, err := net.ParseCIDR(pattern)
		if err == nil {
			ip := net.ParseIP(clientIP)
			return ip != nil && ipNet.Contains(ip)
		}
		return false
	}

	// Exact IP match
	return clientIP == pattern
}

package middleware

import (
	"net"
	"strings"
	"sync"

	"easyserver/internal/config"

	"easyserver/internal/apperror"
	"github.com/gin-gonic/gin"
)

type IPWhitelist struct {
	mu      sync.RWMutex
	allowed []*net.IPNet
	enabled bool
}

func NewIPWhitelist(cfg *config.AuthConfig) *IPWhitelist {
	wl := &IPWhitelist{
		enabled: len(cfg.IPWhitelist) > 0,
	}

	for _, cidr := range cfg.IPWhitelist {
		_, ipNet, err := net.ParseCIDR(cidr)
		if err != nil {
			// Try as single IP
			ip := net.ParseIP(cidr)
			if ip != nil {
				// Create /32 or /128 mask
				if ip.To4() != nil {
					_, ipNet, _ = net.ParseCIDR(cidr + "/32")
				} else {
					_, ipNet, _ = net.ParseCIDR(cidr + "/128")
				}
			}
		}
		if ipNet != nil {
			wl.allowed = append(wl.allowed, ipNet)
		}
	}

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
				if ip.To4() != nil {
					_, ipNet, _ = net.ParseCIDR(cidr + "/32")
				} else {
					_, ipNet, _ = net.ParseCIDR(cidr + "/128")
				}
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
			c.Error(apperror.ErrForbidden.WithMessage("IP not allowed"))
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
			c.Error(apperror.ErrForbidden.WithMessage("your IP is not in the user whitelist"))
			c.Abort()
			return
		}

		c.Next()
	}
}

// splitAndTrim splits a comma-separated string and trims whitespace
func splitAndTrim(s string) []string {
	parts := []string{}
	for _, p := range strings.Split(s, ",") {
		p = strings.TrimSpace(p)
		if p != "" {
			parts = append(parts, p)
		}
	}
	return parts
}

// matchIP checks if clientIP matches a whitelist entry (supports CIDR and exact IP)
func matchIP(clientIP, entry string) bool {
	// Try CIDR match
	if strings.Contains(entry, "/") {
		_, ipNet, err := net.ParseCIDR(entry)
		if err != nil {
			return false
		}
		ip := net.ParseIP(clientIP)
		return ip != nil && ipNet.Contains(ip)
	}
	// Exact IP match
	return clientIP == entry
}

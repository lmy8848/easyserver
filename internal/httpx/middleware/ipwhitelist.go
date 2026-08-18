package middleware

import (
	"net"
	"sync"
	"sync/atomic"

	"easyserver/internal/infra/errx"

	"github.com/gin-gonic/gin"
)

type IPWhitelist struct {
	mu      sync.RWMutex
	allowed []*net.IPNet
	enabled bool
}

// 全局 IP 白名单实例（与 ratelimit 的注册表同款模式），供运行中热更新。
// router 构建时注册，settings handler 更新时读取。
var globalIPWhitelist atomic.Pointer[IPWhitelist]

// GetIPWhitelist 返回全局白名单实例，未注册时返回 nil。
func GetIPWhitelist() *IPWhitelist {
	return globalIPWhitelist.Load()
}

// RegisterIPWhitelist 注册全局白名单实例（进程内只调用一次）。
func RegisterIPWhitelist(wl *IPWhitelist) {
	globalIPWhitelist.Store(wl)
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

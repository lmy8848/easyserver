package middleware

import (
	"sync"
	"time"

	"easyserver/internal/infra/errx"

	"github.com/gin-gonic/gin"
)

// RateLimiter implements an in-memory token bucket / sliding window rate limiter
type RateLimiter struct {
	mu       sync.Mutex
	visitors map[string]*visitor
	rate     int           // requests allowed
	interval time.Duration // per time interval
}

type visitor struct {
	lastSeen time.Time
	tokens   int
}

// Global registry of named rate limiters (so same named limiter is reused across requests)
var (
	rateLimiters sync.Map
	cleanupOnce  sync.Once
)

// NewRateLimiter creates a new rate limiter
func NewRateLimiter(rate int, interval time.Duration) *RateLimiter {
	rl := &RateLimiter{
		visitors: make(map[string]*visitor),
		rate:     rate,
		interval: interval,
	}

	// Start background cleanup goroutine once
	cleanupOnce.Do(func() {
		go func() {
			ticker := time.NewTicker(time.Minute)
			for range ticker.C {
				rateLimiters.Range(func(key, value any) bool {
					if limiter, ok := value.(*RateLimiter); ok {
						limiter.cleanup(time.Minute * 5)
					}
					return true
				})
			}
		}()
	})

	return rl
}

// GetRateLimiter returns a named rate limiter from the registry, or nil if not registered
func GetRateLimiter(name string) *RateLimiter {
	if val, ok := rateLimiters.Load(name); ok {
		return val.(*RateLimiter)
	}
	return nil
}

// StopRateLimiter stops and clears all registered rate limiters
func StopRateLimiter() {
	rateLimiters.Range(func(k, v any) bool {
		rateLimiters.Delete(k)
		return true
	})
}

// RegisterRateLimiter registers a named rate limiter
func RegisterRateLimiter(name string, limiter *RateLimiter) {
	rateLimiters.Store(name, limiter)
}

// isAllowed checks if a request from the given IP is allowed
func (rl *RateLimiter) isAllowed(ip string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	v, exists := rl.visitors[ip]

	if !exists {
		rl.visitors[ip] = &visitor{
			lastSeen: now,
			tokens:   rl.rate - 1,
		}
		return true
	}

	// Refill tokens based on elapsed time
	elapsed := now.Sub(v.lastSeen)
	if elapsed >= rl.interval {
		v.tokens = rl.rate - 1
		v.lastSeen = now
		return true
	}

	// Calculate tokens to add proportionally
	tokensToAdd := int(float64(elapsed) / float64(rl.interval) * float64(rl.rate))
	if tokensToAdd > 0 {
		v.tokens = min(v.tokens+tokensToAdd, rl.rate)
		v.lastSeen = now
	}

	if v.tokens > 0 {
		v.tokens--
		return true
	}

	return false
}

// Reset clears the rate limiter history for an IP (e.g., after successful login)
func (rl *RateLimiter) Reset(ip string) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	delete(rl.visitors, ip)
}

// ResetAll clears all rate limiter history
func (rl *RateLimiter) ResetAll() {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	rl.visitors = make(map[string]*visitor)
}

// UpdateRate dynamically updates rate and interval for the limiter
func (rl *RateLimiter) UpdateRate(rate int, interval time.Duration) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	rl.rate = rate
	rl.interval = interval
}

// cleanup removes stale entries
func (rl *RateLimiter) cleanup(maxAge time.Duration) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	for ip, v := range rl.visitors {
		if now.Sub(v.lastSeen) > maxAge {
			delete(rl.visitors, ip)
		}
	}
}

// RateLimitMiddleware creates a named rate limiter and returns a Gin handler.
// Use different names for different tiers (e.g. "api", "login", "assets") so
// each tier gets its own independent limiter.
func RateLimitMiddleware(name string, rate int, interval time.Duration) gin.HandlerFunc {
	// Reuse an existing limiter for the same name instead of replacing it (which
	// would leak the old limiter's cleanup goroutine).
	limiter := GetRateLimiter(name)
	if limiter == nil {
		limiter = NewRateLimiter(rate, interval)
		rateLimiters.Store(name, limiter)
	}

	return func(c *gin.Context) {
		ip := c.ClientIP()

		if !limiter.isAllowed(ip) {
			c.Error(errx.RateLimit("Too many requests"))
			c.Abort()
			return
		}

		c.Next()
	}
}

package middleware

import (
	"sync"
	"time"

	"easyserver/internal/infra/apperror"
	"github.com/gin-gonic/gin"
)

type visitor struct {
	count    int
	lastSeen time.Time
}

const maxVisitors = 10000

type RateLimiter struct {
	visitors map[string]*visitor
	mu       sync.RWMutex
	rate     int
	interval time.Duration
	done     chan struct{}
}

func NewRateLimiter(rate int, interval time.Duration) *RateLimiter {
	rl := &RateLimiter{
		visitors: make(map[string]*visitor),
		rate:     rate,
		interval: interval,
		done:     make(chan struct{}),
	}

	// Cleanup old visitors every minute
	go rl.cleanup()

	return rl
}

func (rl *RateLimiter) Stop() {
	close(rl.done)
}

func (rl *RateLimiter) cleanup() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			rl.mu.Lock()
			for ip, v := range rl.visitors {
				if time.Since(v.lastSeen) > rl.interval {
					delete(rl.visitors, ip)
				}
			}
			rl.mu.Unlock()
		case <-rl.done:
			return
		}
	}
}

func (rl *RateLimiter) isAllowed(ip string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	v, exists := rl.visitors[ip]
	if !exists {
		// Enforce max visitors limit to prevent memory exhaustion
		if len(rl.visitors) >= maxVisitors {
			// Evict oldest visitors
			rl.evictOldest()
		}
		rl.visitors[ip] = &visitor{count: 1, lastSeen: time.Now()}
		return true
	}

	if time.Since(v.lastSeen) > rl.interval {
		v.count = 1
		v.lastSeen = time.Now()
		return true
	}

	if v.count >= rl.rate {
		return false
	}

	v.count++
	v.lastSeen = time.Now()
	return true
}

// evictOldest removes the oldest 10% of visitors to free memory
func (rl *RateLimiter) evictOldest() {
	type entry struct {
		ip   string
		time time.Time
	}
	var entries []entry
	for ip, v := range rl.visitors {
		entries = append(entries, entry{ip, v.lastSeen})
	}
	// Sort by lastSeen ascending
	for i := 0; i < len(entries); i++ {
		for j := i + 1; j < len(entries); j++ {
			if entries[j].time.Before(entries[i].time) {
				entries[i], entries[j] = entries[j], entries[i]
			}
		}
	}
	// Remove oldest 10%
	toRemove := len(entries) / 10
	if toRemove < 1 {
		toRemove = 1
	}
	for i := 0; i < toRemove; i++ {
		delete(rl.visitors, entries[i].ip)
	}
}

// globalRateLimiter is the package-level rate limiter for Stop() access
var globalRateLimiter *RateLimiter
var globalRateLimiterOnce sync.Once

// StopRateLimiter stops the background cleanup goroutine.
// Should be called during server shutdown.
func StopRateLimiter() {
	if globalRateLimiter != nil {
		globalRateLimiter.Stop()
	}
}

// RateLimitMiddleware limits request rate per IP
func RateLimitMiddleware(rate int, interval time.Duration) gin.HandlerFunc {
	limiter := NewRateLimiter(rate, interval)
	globalRateLimiterOnce.Do(func() {
		globalRateLimiter = limiter
	})

	return func(c *gin.Context) {
		ip := c.ClientIP()

		if !limiter.isAllowed(ip) {
			c.Error(apperror.ErrRateLimit.WithMessage("rate limit exceeded"))
			c.Abort()
			return
		}

		c.Next()
	}
}

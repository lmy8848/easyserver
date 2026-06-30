package middleware

import (
	"sync"
	"sync/atomic"
	"time"

	"easyserver/internal/infra/apperror"
	"github.com/gin-gonic/gin"
)

	type visitor struct {
		count       int
		windowStart time.Time
	}

	const maxVisitors = 10000

	type RateLimiter struct {
		visitors map[string]*visitor
		mu       sync.RWMutex
		rate     atomic.Int64
		interval atomic.Int64
		done     chan struct{}
	}

	func NewRateLimiter(rate int, interval time.Duration) *RateLimiter {
		rl := &RateLimiter{
			visitors: make(map[string]*visitor),
			done:     make(chan struct{}),
		}
		rl.rate.Store(int64(rate))
		rl.interval.Store(int64(interval))

		go rl.cleanup()

		return rl
	}

	func (rl *RateLimiter) UpdateRate(rate int, interval time.Duration) {
		rl.rate.Store(int64(rate))
		rl.interval.Store(int64(interval))
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
				interval := time.Duration(rl.interval.Load())
				now := time.Now()
				for ip, v := range rl.visitors {
					if now.Sub(v.windowStart) > interval*2 {
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

		rate := int(rl.rate.Load())
		interval := time.Duration(rl.interval.Load())
		now := time.Now()

		v, exists := rl.visitors[ip]
		if !exists {
			if len(rl.visitors) >= maxVisitors {
				rl.evictOldest()
			}
			rl.visitors[ip] = &visitor{count: 1, windowStart: now}
			return true
		}

		if now.Sub(v.windowStart) > interval {
			v.count = 1
			v.windowStart = now
			return true
		}

		if v.count >= rate {
			return false
		}

		v.count++
		return true
	}

	func (rl *RateLimiter) evictOldest() {
		type entry struct {
			ip   string
			time time.Time
		}
		var entries []entry
		for ip, v := range rl.visitors {
			entries = append(entries, entry{ip, v.windowStart})
		}
		for i := 0; i < len(entries); i++ {
			for j := i + 1; j < len(entries); j++ {
				if entries[j].time.Before(entries[i].time) {
					entries[i], entries[j] = entries[j], entries[i]
				}
			}
		}
		toRemove := len(entries) / 10
		if toRemove < 1 {
			toRemove = 1
		}
		for i := 0; i < toRemove; i++ {
			delete(rl.visitors, entries[i].ip)
		}
	}

var rateLimiters sync.Map

func StopRateLimiter() {
	rateLimiters.Range(func(_, v interface{}) bool {
		v.(*RateLimiter).Stop()
		return true
	})
}

// GetRateLimiter returns a named rate limiter for runtime updates.
func GetRateLimiter(name string) *RateLimiter {
	if v, ok := rateLimiters.Load(name); ok {
		return v.(*RateLimiter)
	}
	return nil
}

// RateLimitMiddleware creates a named rate limiter and returns a Gin handler.
// Use different names for different tiers (e.g. "api", "login", "assets") so
// each tier gets its own independent limiter.
func RateLimitMiddleware(name string, rate int, interval time.Duration) gin.HandlerFunc {
	limiter := NewRateLimiter(rate, interval)
	rateLimiters.Store(name, limiter)

	return func(c *gin.Context) {
		ip := c.ClientIP()

		if !limiter.isAllowed(ip) {
			c.Error(apperror.ErrRateLimit)
			c.Abort()
			return
		}

		c.Next()
	}
}

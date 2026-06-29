package middleware

import (
	"sync"
	"sync/atomic"
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

// UpdateRate updates the rate limit and interval at runtime (no restart needed).
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
			for ip, v := range rl.visitors {
				if time.Since(v.lastSeen) > interval {
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

	v, exists := rl.visitors[ip]
	if !exists {
		if len(rl.visitors) >= maxVisitors {
			rl.evictOldest()
		}
		rl.visitors[ip] = &visitor{count: 1, lastSeen: time.Now()}
		return true
	}

	if time.Since(v.lastSeen) > interval {
		v.count = 1
		v.lastSeen = time.Now()
		return true
	}

	if v.count >= rate {
		return false
	}

	v.count++
	v.lastSeen = time.Now()
	return true
}

func (rl *RateLimiter) evictOldest() {
	type entry struct {
		ip   string
		time time.Time
	}
	var entries []entry
	for ip, v := range rl.visitors {
		entries = append(entries, entry{ip, v.lastSeen})
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

var globalRateLimiter atomic.Value

func StopRateLimiter() {
	if v := globalRateLimiter.Load(); v != nil {
		v.(*RateLimiter).Stop()
	}
}

// GetRateLimiter returns the global rate limiter for runtime updates.
func GetRateLimiter() *RateLimiter {
	if v := globalRateLimiter.Load(); v != nil {
		return v.(*RateLimiter)
	}
	return nil
}

func RateLimitMiddleware(rate int, interval time.Duration) gin.HandlerFunc {
	limiter := NewRateLimiter(rate, interval)
	globalRateLimiter.Store(limiter)

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

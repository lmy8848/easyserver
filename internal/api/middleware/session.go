package middleware

import (
	"strings"
	"sync"
	"time"

	"easyserver/internal/auth"
	"easyserver/internal/infra"

	"github.com/gin-gonic/gin"
)

// globalSessionLimiter is the package-level heartbeat limiter for Stop() access
var globalSessionLimiter *sessionHeartbeatLimiter

// sessionHeartbeatLimiter rate-limits session heartbeat updates to once per interval per token
type sessionHeartbeatLimiter struct {
	mu       sync.Mutex
	lastBeat map[string]time.Time
	interval time.Duration
	done     chan struct{}
}

func newSessionHeartbeatLimiter(beatInterval, cleanupInterval time.Duration) *sessionHeartbeatLimiter {
	l := &sessionHeartbeatLimiter{
		lastBeat: make(map[string]time.Time),
		interval: beatInterval,
		done:     make(chan struct{}),
	}
	// Clean up expired tokens periodically
	infra.Go(func() {
		ticker := time.NewTicker(cleanupInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				l.cleanup()
			case <-l.done:
				return
			}
		}
	})
	return l
}

func (l *sessionHeartbeatLimiter) shouldUpdate(token string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if last, ok := l.lastBeat[token]; ok && time.Since(last) < l.interval {
		return false
	}
	l.lastBeat[token] = time.Now()
	return true
}

func (l *sessionHeartbeatLimiter) cleanup() {
	l.mu.Lock()
	defer l.mu.Unlock()
	for token, last := range l.lastBeat {
		if time.Since(last) > 5*time.Minute {
			delete(l.lastBeat, token)
		}
	}
}

// StopSessionHeartbeatLimiter stops the background cleanup goroutine.
// Should be called during server shutdown.
func StopSessionHeartbeatLimiter() {
	if globalSessionLimiter != nil {
		close(globalSessionLimiter.done)
	}
}

// SessionHeartbeatMiddleware updates the session's last_active on every request.
// Rate-limited to one update per 30 seconds per token to reduce DB writes.
// A missing session row means the token was revoked; the JWT middleware's
// sessionValidator already rejects it, so we do NOT recreate it here (that would
// revive revoked sessions and read a recycled gin.Context - use-after-recycle).
func SessionHeartbeatMiddleware(sessionService *auth.SessionService, sessionTimeout time.Duration) gin.HandlerFunc {
	limiter := newSessionHeartbeatLimiter(30*time.Second, 5*time.Minute)
	globalSessionLimiter = limiter
	return func(c *gin.Context) {
		// Only heartbeat for authenticated requests (JWT middleware set user_id).
		if _, ok := c.Get("user_id"); !ok {
			c.Next()
			return
		}
		authHeader := c.GetHeader("Authorization")
		if authHeader != "" {
			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) == 2 && parts[0] == "Bearer" {
				token := parts[1]
				if limiter.shouldUpdate(token) {
					_ = sessionService.UpdateActivity(c.Request.Context(), token)
				}
			}
		}

		c.Next()
	}
}

package middleware

import (
	"context"
	"log"
	"strings"
	"sync"
	"time"

	"easyserver/internal/auth"

	"github.com/gin-gonic/gin"
)

// globalSessionLimiter is the package-level heartbeat limiter for Stop() access
var globalSessionLimiter *sessionHeartbeatLimiter

// sessionHeartbeatLimiter rate-limits session heartbeat updates to once per interval per token
type sessionHeartbeatLimiter struct {
	mu          sync.Mutex
	lastBeat    map[string]time.Time
	createdOnce map[string]bool // tokens that already triggered a CreateSession
	interval    time.Duration
	done        chan struct{}
}

func newSessionHeartbeatLimiter(beatInterval, cleanupInterval time.Duration) *sessionHeartbeatLimiter {
	l := &sessionHeartbeatLimiter{
		lastBeat:    make(map[string]time.Time),
		createdOnce: make(map[string]bool),
		interval:    beatInterval,
		done:        make(chan struct{}),
	}
	// Clean up expired tokens periodically
	go func() {
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
	}()
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

// shouldCreate returns true only the first time a token needs session creation,
// preventing goroutine pile-up when CreateSession persistently fails.
func (l *sessionHeartbeatLimiter) shouldCreate(token string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.createdOnce[token] {
		return false
	}
	l.createdOnce[token] = true
	return true
}

func (l *sessionHeartbeatLimiter) cleanup() {
	l.mu.Lock()
	defer l.mu.Unlock()
	for token, last := range l.lastBeat {
		if time.Since(last) > 5*time.Minute {
			delete(l.lastBeat, token)
			delete(l.createdOnce, token)
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

// SessionHeartbeatMiddleware updates the session's last_active on every request
// If session doesn't exist, creates one (for tokens obtained before session management)
// Rate-limited to one update per 30 seconds per token to reduce DB writes.
func SessionHeartbeatMiddleware(sessionService *auth.SessionService, sessionTimeout time.Duration) gin.HandlerFunc {
	limiter := newSessionHeartbeatLimiter(30*time.Second, 5*time.Minute)
	globalSessionLimiter = limiter
	return func(c *gin.Context) {
		// Get the token from Authorization header
		authHeader := c.GetHeader("Authorization")
		if authHeader != "" {
			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) == 2 && parts[0] == "Bearer" {
				token := parts[1]

				// Get user info from context (set by JWT middleware)
				// Use comma-ok assertion to avoid panic
				userID, _ := c.Get("user_id")
				username, _ := c.Get("username")
				role, _ := c.Get("role")

				if userID != nil && username != nil {
					// Type assert with comma-ok pattern
					uid, ok := userID.(int64)
					if !ok {
						log.Printf("session: invalid user_id type in context")
						c.Next()
						return
					}

					uname, ok := username.(string)
					if !ok {
						log.Printf("session: invalid username type in context")
						c.Next()
						return
					}

					// Role might not be set, use default if missing
					roleStr := "viewer"
					if role != nil {
						if r, ok := role.(string); ok {
							roleStr = r
						}
					}

					// Try to update existing session (rate-limited to once per 30s)
					err := error(nil)
					if limiter.shouldUpdate(token) {
						err = sessionService.UpdateActivity(c.Request.Context(), token)
					}
					if err != nil {
						// Session doesn't exist, create one with a bounded timeout
						// to prevent goroutine leak on persistent DB failures.
						if limiter.shouldCreate(token) {
							go func() {
								ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
								defer cancel()
								ip := c.ClientIP()
								userAgent := c.Request.UserAgent()
								expiresAt := time.Now().Add(sessionTimeout)
								sessionService.CreateSession(
									ctx,
									token,
									uid,
									uname,
									roleStr,
									ip,
									userAgent,
									expiresAt,
								)
							}()
						}
					}
				}
			}
		}

		c.Next()
	}
}

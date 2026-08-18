package auth

import (
	"context"
	"log"
	"sort"
	"sync"
	"time"

	"easyserver/internal/infra/config"
	"easyserver/internal/infra/errx"
)

// SessionService manages login sessions in memory. Sessions do not survive
// process restarts — users must re-login after a restart, which is acceptable
// (and arguably safer) for a server admin panel.
type SessionService struct {
	mu       sync.RWMutex
	sessions map[string]*Session // raw JWT token → session
	store    *config.Store
}

// NewSessionService 构造会话服务。idle_timeout 每次清理实时读 store
// （settings 修改后立即生效）；清理循环周期在构造时取初值（ticker 周期
// 无法中途变更）。
func NewSessionService(ctx context.Context, wg *sync.WaitGroup, store *config.Store) *SessionService {
	s := &SessionService{
		sessions: make(map[string]*Session),
		store:    store,
	}
	wg.Go(func() {
		s.cleanupLoop(ctx, store.Get().Auth.SessionCleanupInterval.Duration())
	})
	return s
}

func (s *SessionService) cleanupLoop(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			if err := s.CleanupExpiredSessions(ctx); err != nil {
				log.Printf("session cleanup error: %v", err)
			}
		case <-ctx.Done():
			return
		}
	}
}

func (s *SessionService) CreateSession(ctx context.Context, token string, userID int64, username, role, ip, userAgent, clientType, deviceID, deviceInfo string, expiresAt time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[token] = &Session{
		UserID:     userID,
		Username:   username,
		Role:       role,
		IP:         ip,
		UserAgent:  userAgent,
		ClientType: clientType,
		DeviceID:   deviceID,
		DeviceInfo: deviceInfo,
		LoginAt:    time.Now(),
		ExpiresAt:  expiresAt,
		Token:      token,
	}
	return nil
}

func (s *SessionService) UpdateActivity(ctx context.Context, token string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, ok := s.sessions[token]
	if !ok {
		return errx.NotFound("session not found")
	}
	sess.LoginAt = time.Now()
	return nil
}

func (s *SessionService) RemoveSession(ctx context.Context, token string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, token)
	return nil
}

func (s *SessionService) RemoveUserSessions(ctx context.Context, userID int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for token, sess := range s.sessions {
		if sess.UserID == userID {
			delete(s.sessions, token)
		}
	}
	return nil
}

// RemoveSessionByStoredToken deletes a session by its token. In the in-memory
// store the token IS the key, so this is identical to RemoveSession. Kept for
// API compatibility with the handler's kick path.
func (s *SessionService) RemoveSessionByStoredToken(ctx context.Context, token string) error {
	return s.RemoveSession(ctx, token)
}

// RemoveMobileSessions deletes all mobile sessions for a user. Used by the
// same-device refresh path to replace the bound mobile session.
func (s *SessionService) RemoveMobileSessions(ctx context.Context, userID int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for token, sess := range s.sessions {
		if sess.UserID == userID && sess.ClientType == "mobile" {
			delete(s.sessions, token)
		}
	}
	return nil
}

func (s *SessionService) GetActiveSessions(ctx context.Context) ([]Session, error) {
	return s.getActiveSessions(), nil
}

func (s *SessionService) GetUserSessions(ctx context.Context, userID int64) ([]Session, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	now := time.Now()
	var result []Session
	for _, sess := range s.sessions {
		if sess.UserID == userID && sess.ExpiresAt.After(now) {
			result = append(result, *sess)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].LoginAt.After(result[j].LoginAt)
	})
	return result, nil
}

func (s *SessionService) CleanupExpiredSessions(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	inactiveCutoff := now.Add(-1 * time.Hour)
	for token, sess := range s.sessions {
		if sess.ExpiresAt.Before(now) {
			delete(s.sessions, token)
			continue
		}
		if sess.LoginAt.Before(inactiveCutoff) {
			delete(s.sessions, token)
		}
	}
	return nil
}

func (s *SessionService) GetSessionCount(ctx context.Context) (int, error) {
	return len(s.getActiveSessions()), nil
}

func (s *SessionService) IsSessionValid(ctx context.Context, token string) (bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	sess, ok := s.sessions[token]
	if !ok {
		return false, nil
	}
	now := time.Now()
	if sess.ExpiresAt.Before(now) {
		return false, nil
	}
	if idle := s.store.Get().Auth.IdleTimeout.Duration(); idle > 0 && sess.LoginAt.Before(now.Add(-idle)) {
		return false, nil
	}
	return true, nil
}

func (s *SessionService) GetActiveSessionsWithToken(ctx context.Context) ([]Session, error) {
	return s.getActiveSessions(), nil
}

func (s *SessionService) RemoveOtherSessions(ctx context.Context, userID int64, currentToken string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for token, sess := range s.sessions {
		if sess.UserID == userID && token != currentToken {
			delete(s.sessions, token)
		}
	}
	return nil
}

func (s *SessionService) IsSessionValidByToken(ctx context.Context, token string) (*Session, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	sess, ok := s.sessions[token]
	if !ok {
		return nil, errx.NotFound("session not found")
	}
	cp := *sess
	return &cp, nil
}

// getActiveSessions returns all non-expired, recently-active sessions.
func (s *SessionService) getActiveSessions() []Session {
	s.mu.RLock()
	defer s.mu.RUnlock()
	now := time.Now()
	var result []Session
	for _, sess := range s.sessions {
		if sess.ExpiresAt.After(now) && sess.LoginAt.After(now.Add(-5*time.Minute)) {
			result = append(result, *sess)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].LoginAt.After(result[j].LoginAt)
	})
	return result
}

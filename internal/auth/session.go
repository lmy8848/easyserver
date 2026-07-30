package auth

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sort"
	"sync"
	"time"
)

// SessionService manages login sessions in memory. Sessions do not survive
// process restarts — users must re-login after a restart, which is acceptable
// (and arguably safer) for a server admin panel.
type SessionService struct {
	mu          sync.RWMutex
	sessions    map[string]*Session // raw JWT token → session
	idleTimeout time.Duration
	// mobileMu serializes mobile-session create/replace per process so the
	// single-device binding check + create is atomic (prevents two mobile
	// logins racing past an empty check). Single-admin panel -> one mutex.
	mobileMu sync.Mutex
}

func NewSessionService(ctx context.Context, wg *sync.WaitGroup, idleTimeout, cleanupInterval time.Duration) *SessionService {
	s := &SessionService{
		sessions:    make(map[string]*Session),
		idleTimeout: idleTimeout,
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		s.cleanupLoop(ctx, cleanupInterval)
	}()
	return s
}

func (s *SessionService) cleanupLoop(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			if err := s.CleanupExpiredSessions(context.Background()); err != nil {
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

// ErrMobileDeviceBound is returned by CreateMobileSessionBound when an active
// mobile session exists on a different device_id; the caller rejects the login.
var ErrMobileDeviceBound = errors.New("mobile device bound by another session")

// CreateMobileSessionBound creates a mobile session enforcing single-device
// binding, atomically (process-locked). If an active mobile session exists on
// the same device_id, the new session is created FIRST and the old one removed
// after (so a create failure does NOT lock the user out). On a different
// device_id it returns ErrMobileDeviceBound and creates nothing.
//
// device_id is a client-reported soft identifier, not a security boundary; the
// real control is "one active mobile session + panel-revocable"
func (s *SessionService) CreateMobileSessionBound(ctx context.Context, session *Session) error {
	if ctx == nil {
		ctx = context.Background()
	}
	s.mobileMu.Lock()
	defer s.mobileMu.Unlock()

	existing := s.activeMobileSessionLocked(session.UserID)
	if existing != nil {
		if session.DeviceID != "" && existing.DeviceID == session.DeviceID {
			// Same device: create new first, then remove the old.
			s.mu.Lock()
			s.sessions[session.Token] = session
			delete(s.sessions, existing.Token)
			s.mu.Unlock()
			return nil
		}
		return ErrMobileDeviceBound
	}
	s.mu.Lock()
	s.sessions[session.Token] = session
	s.mu.Unlock()
	return nil
}

// activeMobileSessionLocked returns the user's active mobile session, or nil.
// Caller must hold mobileMu.
func (s *SessionService) activeMobileSessionLocked(userID int64) *Session {
	s.mu.RLock()
	defer s.mu.RUnlock()
	now := time.Now()
	for _, sess := range s.sessions {
		if sess.UserID == userID && sess.ClientType == "mobile" && sess.ExpiresAt.After(now) {
			return sess
		}
	}
	return nil
}

func (s *SessionService) UpdateActivity(ctx context.Context, token string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, ok := s.sessions[token]
	if !ok {
		return fmt.Errorf("session not found")
	}
	sess.LoginAt = time.Now()
	return nil
}

func (s *SessionService) RemoveSession(ctx context.Context, token string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, token)
	return nil
}

func (s *SessionService) RemoveUserSessions(ctx context.Context, userID int64) error {
	if ctx == nil {
		ctx = context.Background()
	}
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
	if ctx == nil {
		ctx = context.Background()
	}
	return s.RemoveSession(ctx, token)
}

// RemoveMobileSessions deletes all mobile sessions for a user. Used by the
// same-device refresh path to replace the bound mobile session.
func (s *SessionService) RemoveMobileSessions(ctx context.Context, userID int64) error {
	if ctx == nil {
		ctx = context.Background()
	}
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
	if ctx == nil {
		ctx = context.Background()
	}
	return s.getActiveSessions(), nil
}

func (s *SessionService) GetUserSessions(ctx context.Context, userID int64) ([]Session, error) {
	if ctx == nil {
		ctx = context.Background()
	}
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
	if ctx == nil {
		ctx = context.Background()
	}
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
	if ctx == nil {
		ctx = context.Background()
	}
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
	if s.idleTimeout > 0 && sess.LoginAt.Before(now.Add(-s.idleTimeout)) {
		return false, nil
	}
	return true, nil
}

func (s *SessionService) GetActiveSessionsWithToken(ctx context.Context) ([]Session, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	return s.getActiveSessions(), nil
}

func (s *SessionService) RemoveOtherSessions(ctx context.Context, userID int64, currentToken string) error {
	if ctx == nil {
		ctx = context.Background()
	}
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
	if ctx == nil {
		ctx = context.Background()
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	sess, ok := s.sessions[token]
	if !ok {
		return nil, fmt.Errorf("session not found")
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

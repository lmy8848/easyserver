package auth

import (
	"context"
	"log"
	"sort"
	"sync"
	"time"

	"easyserver/internal/infra/config"
	"easyserver/internal/infra/errx"
	"easyserver/internal/util"
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

// ClientTypeMobileDirect 标识直连通道铸造的临时凭据会话（非登录会话）。
const ClientTypeMobileDirect = "mobile-direct"

func (s *SessionService) CreateSession(ctx context.Context, token string, userID int64, ip, userAgent, clientType string, expiresAt time.Time) error {
	sid, err := util.RandomHex(8)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[token] = &Session{
		ID:         sid,
		UserID:     userID,
		IP:         ip,
		UserAgent:  userAgent,
		ClientType: clientType,
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

// RemoveSessionByID 按 ID 删除当前用户的会话（会话列表脱 JWT 后的踢出路径）。
func (s *SessionService) RemoveSessionByID(ctx context.Context, userID int64, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for token, sess := range s.sessions {
		if sess.ID == id && sess.UserID == userID {
			delete(s.sessions, token)
			return nil
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
			// 直连通道的临时凭据会话（15 分钟滚动）不是登录会话，不进列表；
			// 设备级的启停走「移动端直连」设置的吊销
			if sess.ClientType == ClientTypeMobileDirect {
				continue
			}
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
	// 移动端会话由 refresh 链控制生命周期（90 天可吊销），不套用 Web 的空闲超时，
	// 否则手机放半小时不用就被踢；Web 仍按 idle_timeout 生效。
	// 直连临时会话同理，其生命周期由 15 分钟绝对过期控制。
	if sess.ClientType != "mobile" && sess.ClientType != ClientTypeMobileDirect {
		if idle := s.store.Get().Auth.IdleTimeout.Duration(); idle > 0 && sess.LoginAt.Before(now.Add(-idle)) {
			return false, nil
		}
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

// getActiveSessions 返回所有仍然有效的会话，供"登录会话"页面展示与踢出。
// 有效性口径与 IsSessionValid 一致：未过绝对过期时间，且未超过空闲超时。
// LoginAt 在每次请求时刷新，语义上是最近活动时间而非首次登录时间。
// 此前用固定 5 分钟活跃窗口过滤，会让静置几分钟的设备从列表里消失、又无法踢出。
func (s *SessionService) getActiveSessions() []Session {
	s.mu.RLock()
	defer s.mu.RUnlock()
	now := time.Now()
	idle := s.store.Get().Auth.IdleTimeout.Duration()
	var result []Session
	for _, sess := range s.sessions {
		if !sess.ExpiresAt.After(now) {
			continue
		}
		if idle > 0 && sess.LoginAt.Before(now.Add(-idle)) {
			continue
		}
		result = append(result, *sess)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].LoginAt.After(result[j].LoginAt)
	})
	return result
}

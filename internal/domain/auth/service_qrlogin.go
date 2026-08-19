package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	"easyserver/internal/infra/config"
)

// qrTokenBytes is the entropy for a QR session token (32 bytes => 64 hex chars).
const qrTokenBytes = 32

// pendingTTL is how long a pending QR session stays valid (web must poll, mobile
// must scan+confirm within this window).
const pendingTTL = 2 * time.Minute

// QRLoginService implements the scan-to-login state machine. Sessions live in
// process memory — a restart discards pending scans, which is correct (the user
// just re-scans, same as TOTP setup). It issues the web JWT and creates a
// coexisting session (no RemoveUserSessions) so the authorizing mobile stays
// logged in. JWT 密钥与会话时长实时读 store。
type QRLoginService struct {
	mu             sync.Mutex
	sessions       map[string]*QRLoginSession // qrToken -> session
	store          *config.Store
	sessionService *SessionService
}

func NewQRLoginService(store *config.Store, sessionService *SessionService) *QRLoginService {
	return &QRLoginService{
		sessions:       make(map[string]*QRLoginSession),
		store:          store,
		sessionService: sessionService,
	}
}

// CreateSession generates a new pending QR session and returns the QR token
// for the web to render as a QR code.
func (s *QRLoginService) CreateSession(ctx context.Context) (*CreateResult, error) {
	s.cleanupExpiredLocked()

	b := make([]byte, qrTokenBytes)
	if _, err := rand.Read(b); err != nil {
		return nil, fmt.Errorf("generate qr token: %w", err)
	}
	qrToken := hex.EncodeToString(b)

	now := time.Now()
	sess := &QRLoginSession{
		QRToken:   qrToken,
		Status:    QRStatusPending,
		ExpiresAt: now.Add(pendingTTL),
	}

	s.mu.Lock()
	s.sessions[qrToken] = sess
	s.mu.Unlock()

	return &CreateResult{
		QRToken:   qrToken,
		ExpiresAt: sess.ExpiresAt,
	}, nil
}

// GetStatus returns the current state for the polling web client. A confirmed
// session is claimed atomically (under the mutex) so the issued token can only
// be picked up by one poll; later polls see "expired". Invalid/expired tokens
// report "expired" without leaking existence.
func (s *QRLoginService) GetStatus(ctx context.Context, qrToken string) (*StatusResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	sess, ok := s.sessions[qrToken]
	if ok && sess.Status == QRStatusConfirmed {
		// 原子领取：仅一个 poll 能抢到 confirmed 会话（置为 consumed）。
		sess.Status = QRStatusConsumed
		res := &StatusResult{
			Status:    "confirmed",
			ExpiresAt: sess.ExpiresAt,
			Token:     sess.WebToken,
			User:      sess.User,
		}
		return res, nil
	}

	// 未抢到：据当前状态区分 expired / cancelled / pending。
	if !ok || sess.Status == QRStatusConsumed {
		// 已被消费或已清理。
		return &StatusResult{Status: "expired"}, nil
	}
	switch sess.Status {
	case QRStatusCancelled:
		return &StatusResult{Status: "cancelled", ExpiresAt: sess.ExpiresAt}, nil
	default: // pending
		if time.Now().After(sess.ExpiresAt) {
			return &StatusResult{Status: "expired"}, nil
		}
		return &StatusResult{Status: "pending", ExpiresAt: sess.ExpiresAt}, nil
	}
}

// Confirm is called by the authenticated mobile app after scanning. It validates
// the QR session is pending+unexpired, issues a web JWT, creates a coexisting
// session, and stores the token + user snapshot for the web to pick up.
func (s *QRLoginService) Confirm(ctx context.Context, qrToken string, user *User, ip, userAgent string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	sess, ok := s.sessions[qrToken]
	if !ok || sess.Status != QRStatusPending {
		return ErrQRNotPending
	}
	if time.Now().After(sess.ExpiresAt) {
		delete(s.sessions, qrToken)
		return ErrQRExpired
	}

	cur := s.store.Get()
	webToken, err := GenerateToken(cur.Auth.JWTSecret, user.ID, user.Username, cur.Auth.SessionTimeout.Duration())
	if err != nil {
		return fmt.Errorf("generate web token: %w", err)
	}

	// Coexist: create the web session WITHOUT removing the mobile's session.
	if s.sessionService != nil {
		expiresAt := time.Now().Add(cur.Auth.SessionTimeout.Duration())
		if err := s.sessionService.CreateSession(ctx, webToken, user.ID, ip, userAgent, "web", expiresAt); err != nil {
			return fmt.Errorf("create web session: %w", err)
		}
	}

	sess.Status = QRStatusConfirmed
	sess.WebToken = webToken
	sess.User = user
	return nil
}

// Cancel removes a pending session (user dismissed the QR).
func (s *QRLoginService) Cancel(ctx context.Context, qrToken string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, qrToken)
	return nil
}

// cleanupExpiredLocked removes expired, cancelled and consumed sessions. Caller
// must hold s.mu. Called on each CreateSession (opportunistic; no background loop).
func (s *QRLoginService) cleanupExpiredLocked() {
	now := time.Now()
	for token, sess := range s.sessions {
		if sess.ExpiresAt.Before(now) || sess.Status == QRStatusCancelled || sess.Status == QRStatusConsumed {
			delete(s.sessions, token)
		}
	}
}

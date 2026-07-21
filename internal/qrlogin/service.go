package qrlogin

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"easyserver/internal/auth"

	"github.com/skip2/go-qrcode"
)

// qrTokenBytes is the entropy for a QR session token (32 bytes => 64 hex chars).
const qrTokenBytes = 32

// pendingTTL is how long a pending QR session stays valid (web must poll, mobile
// must scan+confirm within this window).
const pendingTTL = 2 * time.Minute

// Service implements the scan-to-login state machine. It depends on the auth
// package for JWT issuance and session creation; QR login deliberately creates
// a coexisting session (no RemoveUserSessions) so the authorizing mobile stays
// logged in.
type Service struct {
	repo           Repository
	jwtSecret      string
	sessionTimeout time.Duration
	sessionService *auth.SessionService
}

func NewService(repo Repository, jwtSecret string, sessionTimeout time.Duration, sessionService *auth.SessionService) *Service {
	return &Service{repo: repo, jwtSecret: jwtSecret, sessionTimeout: sessionTimeout, sessionService: sessionService}
}

// CreateSession generates a new pending QR session and returns the QR token +
// base64-encoded PNG (content "esqr:<token>") for the web to render.
func (s *Service) CreateSession(ctx context.Context) (*CreateResult, error) {
	// Opportunistic cleanup of stale pending/cancelled rows.
	if _, err := s.repo.DeleteExpired(ctx); err != nil {
		// non-fatal; a failed cleanup shouldn't block login
		_ = err
	}

	b := make([]byte, qrTokenBytes)
	if _, err := rand.Read(b); err != nil {
		return nil, fmt.Errorf("generate qr token: %w", err)
	}
	qrToken := hex.EncodeToString(b)

	now := time.Now()
	sess := &QRLoginSession{
		QRToken:   qrToken,
		Status:    StatusPending,
		CreatedAt: now,
		ExpiresAt: now.Add(pendingTTL),
	}
	if _, err := s.repo.Create(ctx, sess); err != nil {
		return nil, err
	}

	png, err := qrcode.Encode("esqr:"+qrToken, qrcode.Medium, 256)
	if err != nil {
		return nil, fmt.Errorf("encode qr: %w", err)
	}

	return &CreateResult{
		QRToken:      qrToken,
		QRCodeBase64: "data:image/png;base64," + base64.StdEncoding.EncodeToString(png),
		ExpiresAt:    sess.ExpiresAt,
	}, nil
}

// GetStatus returns the current state for the polling web client. A confirmed
// session is consumed (deleted) on first read so the issued token can only be
// picked up once. Invalid/expired tokens report "expired" without leaking
// existence.
func (s *Service) GetStatus(ctx context.Context, qrToken string) (*StatusResult, error) {
	sess, err := s.repo.GetByToken(ctx, qrToken)
	if err != nil {
		return nil, err
	}
	if sess == nil {
		return &StatusResult{Status: "expired"}, nil
	}

	switch sess.Status {
	case StatusConfirmed:
		// One-time pickup: hand over the token + user payload, then delete.
		res := &StatusResult{
			Status:    "confirmed",
			ExpiresAt: sess.ExpiresAt,
			Token:     sess.WebToken,
		}
		if sess.UserJSON != "" {
			var lp LoginPayload
			if json.Unmarshal([]byte(sess.UserJSON), &lp) == nil {
				res.User = lp.User
				res.MustChangePass = lp.MustChangePass
			}
		}
		_ = s.repo.Delete(ctx, qrToken)
		return res, nil
	case StatusCancelled:
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
// session, and stores the token+user payload for the web to pick up. userJSON
// is the pre-serialized {user, must_change_pass} payload.
func (s *Service) Confirm(ctx context.Context, qrToken string, userID int64, username, role, ip, userAgent, userJSON string) error {
	sess, err := s.repo.GetByToken(ctx, qrToken)
	if err != nil {
		return err
	}
	if sess == nil || sess.Status != StatusPending {
		return ErrNotPending
	}
	if time.Now().After(sess.ExpiresAt) {
		_ = s.repo.Delete(ctx, qrToken)
		return ErrExpired
	}

	webToken, err := auth.GenerateToken(s.jwtSecret, userID, username, role, s.sessionTimeout)
	if err != nil {
		return fmt.Errorf("generate web token: %w", err)
	}

	// Coexist: create the web session WITHOUT removing the mobile's session.
	if s.sessionService != nil {
		expiresAt := time.Now().Add(s.sessionTimeout)
		if err := s.sessionService.CreateSession(ctx, webToken, userID, username, role, ip, userAgent, "web", "", "", expiresAt); err != nil {
			return fmt.Errorf("create web session: %w", err)
		}
	}

	if err := s.repo.MarkConfirmed(ctx, qrToken, userID, webToken, userJSON); err != nil {
		return err
	}
	return nil
}

// Cancel removes a pending session (user dismissed the QR).
func (s *Service) Cancel(ctx context.Context, qrToken string) error {
	return s.repo.Delete(ctx, qrToken)
}

// CleanupExpired removes stale rows; called periodically by the caller.
func (s *Service) CleanupExpired(ctx context.Context) (int64, error) {
	return s.repo.DeleteExpired(ctx)
}

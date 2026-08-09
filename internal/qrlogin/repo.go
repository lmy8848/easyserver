package qrlogin

import "context"

// Repository defines data access for QR login sessions.
type Repository interface {
	Create(ctx context.Context, s *QRLoginSession) (int64, error)
	GetByToken(ctx context.Context, qrToken string) (*QRLoginSession, error)
	// MarkConfirmed stores the issued web token + user payload and transitions to confirmed.
	MarkConfirmed(ctx context.Context, qrToken string, userID int64, webToken string, userJSON string) error
	// Consume atomically claims a confirmed session via a conditional UPDATE
	// (confirmed -> consumed). It returns the session only if THIS caller won the
	// race; ok is false when another poll already claimed it, it was cancelled,
	// or it is still pending.
	Consume(ctx context.Context, qrToken string) (*QRLoginSession, bool, error)
	Delete(ctx context.Context, qrToken string) error
	DeleteExpired(ctx context.Context) (int64, error)
}

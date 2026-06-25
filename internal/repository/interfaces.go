package repository

import (
	"context"
	"time"

	"easyserver/internal/model"
)

// UserRepository defines the interface for user data access
type UserRepository interface {
	GetByID(ctx context.Context, id int64) (*model.User, error)
	GetByUsername(ctx context.Context, username string) (*model.User, error)
	Create(ctx context.Context, user *model.User) error
	Update(ctx context.Context, user *model.User) error
	Delete(ctx context.Context, id int64) error
	List(ctx context.Context, offset, limit int) ([]model.User, int64, error)
	UpdateLoginAttempts(ctx context.Context, id int64, attempts int, lockedUntil *time.Time) error
	UpdatePassword(ctx context.Context, id int64, passwordHash string) error
	SetMustChangePass(ctx context.Context, id int64, mustChange bool) error
}

// SessionRepository defines the interface for session data access
type SessionRepository interface {
	Create(ctx context.Context, session *model.Session) error
	GetByToken(ctx context.Context, token string) (*model.Session, error)
	DeleteByToken(ctx context.Context, token string) error
	DeleteByUserID(ctx context.Context, userID int64) error
	DeleteExpired(ctx context.Context) error
	GetActiveByUserID(ctx context.Context, userID int64) ([]model.Session, error)
	GetActive(ctx context.Context) ([]model.Session, error)
	UpdateActivity(ctx context.Context, token string) error
	Count(ctx context.Context) (int, error)
}

// TokenBlacklistRepository defines the interface for token blacklist data access
type TokenBlacklistRepository interface {
	Add(ctx context.Context, userID int64, token string, expiresAt time.Time) error
	IsBlacklisted(ctx context.Context, token string) (bool, error)
	AddUserInvalidation(ctx context.Context, userID int64) error
	IsUserInvalidated(ctx context.Context, userID int64, issuedAt time.Time) (bool, error)
	Clean(ctx context.Context) error
}

// AuditRepository defines the interface for audit log data access
type AuditRepository interface {
	Log(ctx context.Context, entry *model.AuditLog) error
	Query(ctx context.Context, filter AuditFilter) (int64, []model.AuditLog, error)
	GetActions(ctx context.Context) ([]string, error)
	Clean(ctx context.Context, before time.Time) (int64, error)
}

// AuditFilter defines the filter criteria for audit log queries
type AuditFilter struct {
	Username  string
	Action    string
	Resource  string
	IP        string
	StartDate string
	EndDate string
	Offset    int
	Limit     int
}

// MonitorRepository defines the interface for monitor data access
type MonitorRepository interface {
	Save(ctx context.Context, point *model.MonitorPoint) error
	SaveBatch(ctx context.Context, points []*model.MonitorPoint) error
	GetLatest(ctx context.Context) (*model.MonitorPoint, error)
	GetHistory(ctx context.Context, start, end time.Time) ([]model.MonitorPoint, error)
	Clean(ctx context.Context, before time.Time) (int64, error)
}

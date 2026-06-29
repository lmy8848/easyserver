package audit

import (
	"context"
	"time"
)

// AuditLog is a single audit log entry persisted by the writer.
type AuditLog struct {
	ID        int64
	UserID    int64
	Username  string
	Action    string
	Resource  string
	Detail    string
	IP        string
	UserAgent string
	Type      string
	CreatedAt time.Time
}

// Repository defines the interface for audit log data access.
type Repository interface {
	Log(ctx context.Context, entry *AuditLog) error
	Query(ctx context.Context, filter AuditFilter) (int64, []AuditLog, error)
	GetActions(ctx context.Context, logType string) ([]string, error)
	Clean(ctx context.Context, before time.Time) (int64, error)

	AppendBatch(ctx context.Context, entries []AuditLog) error
}

// AuditFilter defines the filter criteria for audit log queries.
type AuditFilter struct {
	Username  string
	Action    string
	Resource  string
	IP        string
	Type      string
	StartDate string
	EndDate   string
	Offset    int
	Limit     int
}

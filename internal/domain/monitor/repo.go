package monitor

import (
	"context"
	"time"
)

// Repository defines the interface for monitor data access
type Repository interface {
	EnsureIndexes(ctx context.Context) error
	Save(ctx context.Context, point *MonitorPoint) error
	SaveBatch(ctx context.Context, points []*MonitorPoint) error
	GetLatest(ctx context.Context) (*MonitorPoint, error)
	GetHistory(ctx context.Context, start, end time.Time) ([]MonitorPoint, error)
	Clean(ctx context.Context, before time.Time) (int64, error)
}

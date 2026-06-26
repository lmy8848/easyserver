package notification

import "context"

// Repository defines the interface for notification data access
type Repository interface {
	List(ctx context.Context, unreadOnly bool, limit int) ([]Notification, error)
	CountUnread(ctx context.Context) (int, error)
	Create(ctx context.Context, req CreateNotificationRequest) error
	CreateIfNotExists(ctx context.Context, req CreateNotificationRequest) error
	MarkAsRead(ctx context.Context, id int64) error
	MarkAllAsRead(ctx context.Context) error
	Delete(ctx context.Context, id int64) error
	CleanOld(ctx context.Context, days int) (int64, error)
}

package notification

import "context"

// Sink 是通知发送方的窄接口：产生事件的外部模块（database / cron / alert 等）
// 只依赖这个口子向站内通知投递，不依赖整个 Service。*Service 天然实现之。
type Sink interface {
	CreateIfNotExists(req CreateNotificationRequest) (*Notification, error)
}

// Repository defines the interface for notification data access
type Repository interface {
	List(ctx context.Context, unreadOnly bool, limit int) ([]Notification, error)
	CountUnread(ctx context.Context) (int, error)
	Create(ctx context.Context, req CreateNotificationRequest) (*Notification, error)
	CreateIfNotExists(ctx context.Context, req CreateNotificationRequest) (*Notification, error)
	MarkAsRead(ctx context.Context, id int64) error
	MarkAllAsRead(ctx context.Context) error
	Delete(ctx context.Context, id int64) error
	CleanOld(ctx context.Context, days int) (int64, error)
}

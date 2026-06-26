package service

import (
	"easyserver/internal/notification"
)

// NotificationService is now defined in easyserver/internal/notification.Service.
// Kept as alias for backward compatibility.
type NotificationService = notification.Service

// NewNotificationService creates a new NotificationService
func NewNotificationService(repo notification.Repository) *NotificationService {
	return notification.NewService(repo)
}

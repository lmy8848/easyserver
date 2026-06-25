package service

import (
	"context"
	"fmt"
	"time"

	"easyserver/internal/model"
	"easyserver/internal/repository"
)

// --- Constants ---
const (
	defaultNotificationLimit = 50
	maxNotificationLimit     = 200
	defaultRetentionDays     = 30
	cpuAlertThreshold        = 90.0 // percent
	memoryAlertThreshold     = 85.0 // percent
)

// NotificationService handles notification CRUD and alert detection
type NotificationService struct {
	repo repository.NotificationRepository
}

// NewNotificationService creates a new NotificationService
func NewNotificationService(repo repository.NotificationRepository) *NotificationService {
	return &NotificationService{repo: repo}
}

// List returns notifications with optional filters
func (s *NotificationService) List(notOnly bool, limit int) ([]model.Notification, error) {
	if limit <= 0 || limit > maxNotificationLimit {
		limit = defaultNotificationLimit
	}
	return s.repo.List(context.Background(), notOnly, limit)
}

// CountUnread returns the count of unread notifications
func (s *NotificationService) CountUnread() (int, error) {
	return s.repo.CountUnread(context.Background())
}

// Create adds a new notification
func (s *NotificationService) Create(req model.CreateNotificationRequest) error {
	return s.repo.Create(context.Background(), req)
}

// CreateIfNotExists adds a notification only if a similar one doesn't exist in the last hour
func (s *NotificationService) CreateIfNotExists(req model.CreateNotificationRequest) error {
	return s.repo.CreateIfNotExists(context.Background(), req)
}

// MarkAsRead marks a notification as read
func (s *NotificationService) MarkAsRead(id int64) error {
	return s.repo.MarkAsRead(context.Background(), id)
}

// MarkAllAsRead marks all notifications as read
func (s *NotificationService) MarkAllAsRead() error {
	return s.repo.MarkAllAsRead(context.Background())
}

// Delete removes a notification
func (s *NotificationService) Delete(id int64) error {
	return s.repo.Delete(context.Background(), id)
}

// CleanOld removes notifications older than given days
func (s *NotificationService) CleanOld(days int) (int64, error) {
	if days <= 0 {
		days = defaultRetentionDays
	}
	return s.repo.CleanOld(context.Background(), days)
}

// --- Alert Detection ---

// CheckSystemAlerts monitors system metrics and creates notifications for anomalies
func (s *NotificationService) CheckSystemAlerts(overview *model.SystemOverview) {
	if overview == nil {
		return
	}

	// CPU > threshold
	if overview.CPUUsage > cpuAlertThreshold {
		s.CreateIfNotExists(model.CreateNotificationRequest{
			Type:    "alert",
			Title:   "CPU 使用率过高",
			Message: fmt.Sprintf("当前 CPU 使用率 %.1f%%, 超过 %.0f%% 阈值", overview.CPUUsage, cpuAlertThreshold),
			Level:   "error",
		})
	}

	// Memory > threshold
	if overview.MemoryUsage > memoryAlertThreshold {
		s.CreateIfNotExists(model.CreateNotificationRequest{
			Type:    "alert",
			Title:   "内存使用率过高",
			Message: fmt.Sprintf("当前内存使用 %.1f%% (%d/%d MB), 超过 %.0f%% 阈值", overview.MemoryUsage, overview.MemoryUsed, overview.MemoryTotal, memoryAlertThreshold),
			Level:   "warning",
		})
	}
}

// StartPeriodicCleanup runs periodic cleanup of old notifications
func (s *NotificationService) StartPeriodicCleanup(stopCh <-chan struct{}) {
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			s.CleanOld(30)
		case <-stopCh:
			return
		}
	}
}

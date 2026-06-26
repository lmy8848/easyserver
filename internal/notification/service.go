package notification

import (
	"context"
	"fmt"
	"time"
)

// --- Constants ---
const (
	defaultLimit         = 50
	maxLimit             = 200
	defaultRetentionDays = 30
	cpuAlertThreshold    = 90.0 // percent
	memoryAlertThreshold = 85.0 // percent
)

// SystemOverview contains the subset of system metrics needed for alert detection.
// Defined locally to avoid importing the model package (cycle prevention).
type SystemOverview struct {
	CPUUsage    float64
	MemoryUsage float64
	MemoryUsed  int64
	MemoryTotal int64
}

// Service handles notification CRUD and alert detection
type Service struct {
	repo Repository
}

// NewService creates a new notification Service
func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

// List returns notifications with optional filters
func (s *Service) List(notOnly bool, limit int) ([]Notification, error) {
	if limit <= 0 || limit > maxLimit {
		limit = defaultLimit
	}
	return s.repo.List(context.Background(), notOnly, limit)
}

// CountUnread returns the count of unread notifications
func (s *Service) CountUnread() (int, error) {
	return s.repo.CountUnread(context.Background())
}

// Create adds a new notification
func (s *Service) Create(req CreateNotificationRequest) error {
	return s.repo.Create(context.Background(), req)
}

// CreateIfNotExists adds a notification only if a similar one doesn't exist in the last hour
func (s *Service) CreateIfNotExists(req CreateNotificationRequest) error {
	return s.repo.CreateIfNotExists(context.Background(), req)
}

// MarkAsRead marks a notification as read
func (s *Service) MarkAsRead(id int64) error {
	return s.repo.MarkAsRead(context.Background(), id)
}

// MarkAllAsRead marks all notifications as read
func (s *Service) MarkAllAsRead() error {
	return s.repo.MarkAllAsRead(context.Background())
}

// Delete removes a notification
func (s *Service) Delete(id int64) error {
	return s.repo.Delete(context.Background(), id)
}

// CleanOld removes notifications older than given days
func (s *Service) CleanOld(days int) (int64, error) {
	if days <= 0 {
		days = defaultRetentionDays
	}
	return s.repo.CleanOld(context.Background(), days)
}

// --- Alert Detection ---

// CheckSystemAlerts monitors system metrics and creates notifications for anomalies
func (s *Service) CheckSystemAlerts(overview *SystemOverview) {
	if overview == nil {
		return
	}

	// CPU > threshold
	if overview.CPUUsage > cpuAlertThreshold {
		s.CreateIfNotExists(CreateNotificationRequest{
			Type:    "alert",
			Title:   "CPU 使用率过高",
			Message: fmt.Sprintf("当前 CPU 使用率 %.1f%%, 超过 %.0f%% 阈值", overview.CPUUsage, cpuAlertThreshold),
			Level:   "error",
		})
	}

	// Memory > threshold
	if overview.MemoryUsage > memoryAlertThreshold {
		s.CreateIfNotExists(CreateNotificationRequest{
			Type:    "alert",
			Title:   "内存使用率过高",
			Message: fmt.Sprintf("当前内存使用 %.1f%% (%d/%d MB), 超过 %.0f%% 阈值", overview.MemoryUsage, overview.MemoryUsed, overview.MemoryTotal, memoryAlertThreshold),
			Level:   "warning",
		})
	}
}

// StartPeriodicCleanup runs periodic cleanup of old notifications
func (s *Service) StartPeriodicCleanup(stopCh <-chan struct{}) {
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

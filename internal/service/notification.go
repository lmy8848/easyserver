package service

import (
	"database/sql"
	"fmt"
	"time"

	"easyserver/internal/model"
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
	db *sql.DB
}

// NewNotificationService creates a new NotificationService
func NewNotificationService(db *sql.DB) *NotificationService {
	return &NotificationService{db: db}
}

// List returns notifications with optional filters
func (s *NotificationService) List(notOnly bool, limit int) ([]model.Notification, error) {
	if limit <= 0 || limit > maxNotificationLimit {
		limit = defaultNotificationLimit
	}

	query := `SELECT id, type, title, message, level, is_read, COALESCE(metadata,''), created_at
	          FROM notifications`
	if notOnly {
		query += ` WHERE is_read = 0`
	}
	query += ` ORDER BY created_at DESC LIMIT ?`

	rows, err := s.db.Query(query, limit)
	if err != nil {
		return nil, fmt.Errorf("list notifications: %w", err)
	}
	defer rows.Close()

	var result []model.Notification
	for rows.Next() {
		var n model.Notification
		var isRead int
		if err := rows.Scan(&n.ID, &n.Type, &n.Title, &n.Message, &n.Level, &isRead, &n.Metadata, &n.CreatedAt); err != nil {
			continue
		}
		n.IsRead = isRead != 0
		result = append(result, n)
	}
	return result, nil
}

// CountUnread returns the count of unread notifications
func (s *NotificationService) CountUnread() (int, error) {
	var count int
	err := s.db.QueryRow("SELECT COUNT(*) FROM notifications WHERE is_read = 0").Scan(&count)
	return count, err
}

// Create adds a new notification
func (s *NotificationService) Create(req model.CreateNotificationRequest) error {
	level := req.Level
	if level == "" {
		level = "info"
	}
	_, err := s.db.Exec(
		"INSERT INTO notifications (type, title, message, level, metadata) VALUES (?, ?, ?, ?, ?)",
		req.Type, req.Title, req.Message, level, req.Metadata,
	)
	return err
}

// CreateIfNotExists adds a notification only if a similar one doesn't exist in the last hour
// Used for alert deduplication
func (s *NotificationService) CreateIfNotExists(req model.CreateNotificationRequest) error {
	level := req.Level
	if level == "" {
		level = "info"
	}

	// Check for duplicate in last hour
	var exists int
	s.db.QueryRow(
		"SELECT COUNT(*) FROM notifications WHERE type = ? AND title = ? AND created_at > datetime('now', '-1 hour')",
		req.Type, req.Title,
	).Scan(&exists)

	if exists > 0 {
		return nil // deduplicated
	}

	_, err := s.db.Exec(
		"INSERT INTO notifications (type, title, message, level, metadata) VALUES (?, ?, ?, ?, ?)",
		req.Type, req.Title, req.Message, level, req.Metadata,
	)
	return err
}

// MarkAsRead marks a notification as read
func (s *NotificationService) MarkAsRead(id int64) error {
	_, err := s.db.Exec("UPDATE notifications SET is_read = 1 WHERE id = ?", id)
	return err
}

// MarkAllAsRead marks all notifications as read
func (s *NotificationService) MarkAllAsRead() error {
	_, err := s.db.Exec("UPDATE notifications SET is_read = 1 WHERE is_read = 0")
	return err
}

// Delete removes a notification
func (s *NotificationService) Delete(id int64) error {
	_, err := s.db.Exec("DELETE FROM notifications WHERE id = ?", id)
	return err
}

// CleanOld removes notifications older than given days
func (s *NotificationService) CleanOld(days int) (int64, error) {
	if days <= 0 {
		days = defaultRetentionDays
	}
	result, err := s.db.Exec(
		"DELETE FROM notifications WHERE created_at < datetime('now', ?)",
		fmt.Sprintf("-%d days", days),
	)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
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

	// High load average (per core)
	if overview.CPUUsage > 0 {
		// Approximate core count from CPU usage calculation
		// Simple heuristic: if load > 4, it's high for most servers
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

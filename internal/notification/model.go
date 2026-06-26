package notification

// Notification represents a system notification
type Notification struct {
	ID        int64  `json:"id"`
	Type      string `json:"type"` // alert/security/deploy/cron/update/system
	Title     string `json:"title"`
	Message   string `json:"message"`
	Level     string `json:"level"` // info/warning/error
	IsRead    bool   `json:"is_read"`
	Metadata  string `json:"metadata"` // JSON
	CreatedAt string `json:"created_at"`
}

// CreateNotificationRequest is the request body for creating a notification
type CreateNotificationRequest struct {
	Type     string `json:"type" binding:"required"`
	Title    string `json:"title" binding:"required"`
	Message  string `json:"message" binding:"required"`
	Level    string `json:"level"`
	Metadata string `json:"metadata"`
}

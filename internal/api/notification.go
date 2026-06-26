package api

import (
	"strconv"

	"easyserver/internal/model"
	"easyserver/internal/notification"

	"github.com/gin-gonic/gin"
)

// NotificationHandler handles notification API requests
type NotificationHandler struct {
	ns *notification.Service
}

// NewNotificationHandler creates a new NotificationHandler
func NewNotificationHandler(ns *notification.Service) *NotificationHandler {
	return &NotificationHandler{ns: ns}
}

// List returns notifications
func (h *NotificationHandler) List(c *gin.Context) {
	unreadOnly := c.Query("unread") == "true"
	limitStr := c.DefaultQuery("limit", "50")
	limit, _ := strconv.Atoi(limitStr)

	notifications, err := h.ns.List(unreadOnly, limit)
	if err != nil {
		c.Error(WrapError(err))
		return
	}
	Success(c, notifications)
}

// CountUnread returns unread count
func (h *NotificationHandler) CountUnread(c *gin.Context) {
	count, err := h.ns.CountUnread()
	if err != nil {
		c.Error(WrapError(err))
		return
	}
	Success(c, gin.H{"count": count})
}

// Create adds a new notification (admin only)
func (h *NotificationHandler) Create(c *gin.Context) {
	var req model.CreateNotificationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(ErrBadRequest.WithMessage("无效的请求参数"))
		return
	}
	if err := h.ns.Create(req); err != nil {
		c.Error(WrapError(err))
		return
	}
	Success(c, gin.H{"message": "通知已创建"})
}

// MarkAsRead marks a notification as read
func (h *NotificationHandler) MarkAsRead(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.Error(ErrBadRequest.WithMessage("无效的通知 ID"))
		return
	}
	if err := h.ns.MarkAsRead(id); err != nil {
		c.Error(WrapError(err))
		return
	}
	Success(c, gin.H{"message": "已标记为已读"})
}

// MarkAllAsRead marks all as read
func (h *NotificationHandler) MarkAllAsRead(c *gin.Context) {
	if err := h.ns.MarkAllAsRead(); err != nil {
		c.Error(WrapError(err))
		return
	}
	Success(c, gin.H{"message": "全部已标记为已读"})
}

// Delete removes a notification
func (h *NotificationHandler) Delete(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.Error(ErrBadRequest.WithMessage("无效的通知 ID"))
		return
	}
	if err := h.ns.Delete(id); err != nil {
		c.Error(WrapError(err))
		return
	}
	Success(c, gin.H{"message": "已删除"})
}

func registerNotificationRoutes(protected *gin.RouterGroup, ns *notification.Service) {
	handler := NewNotificationHandler(ns)

	notifGroup := protected.Group("/notifications")
	{
		notifGroup.GET("", handler.List)
		notifGroup.GET("/unread-count", handler.CountUnread)
		notifGroup.POST("", handler.Create)
		notifGroup.PUT("/:id/read", handler.MarkAsRead)
		notifGroup.PUT("/read-all", handler.MarkAllAsRead)
		notifGroup.DELETE("/:id", handler.Delete)
	}
}

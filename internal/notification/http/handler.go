package http

import (
	"fmt"
	"net/http"
	"strconv"

	"easyserver/internal/httpx"
	"easyserver/internal/httpx/middleware"
	"easyserver/internal/infra/errx"
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
func (h *NotificationHandler) List(c *gin.Context) (any, error) {
	unreadOnly := c.Query("unread") == "true"
	limitStr := c.DefaultQuery("limit", "50")
	limit, _ := strconv.Atoi(limitStr)

	notifications, err := h.ns.List(unreadOnly, limit)
	if err != nil {
		return nil, err
	}
	return notifications, nil
}

// CountUnread returns unread count
func (h *NotificationHandler) CountUnread(c *gin.Context) (any, error) {
	count, err := h.ns.CountUnread()
	if err != nil {
		return nil, err
	}
	return gin.H{"count": count}, nil
}

// Create adds a new notification (admin only)
func (h *NotificationHandler) Create(c *gin.Context) (any, error) {
	var req notification.CreateNotificationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		return nil, errx.BadRequest("无效的请求参数")
	}
	middleware.AuditSummary(c, "创建通知")
	if _, err := h.ns.Create(req); err != nil {
		return nil, err
	}
	return gin.H{"message": "通知已创建"}, nil
}

// HandleSSE handles notification SSE stream requests
func (h *NotificationHandler) HandleSSE(c *gin.Context) (any, error) {
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		return nil, errx.Internal("当前连接不支持流式输出")
	}
	// 连接建立即 flush 一次，确保客户端 onopen 立即触发
	fmt.Fprint(c.Writer, ": connected\n\n")
	flusher.Flush()

	client := &notification.NotificationClient{
		Send: make(chan []byte, 16),
	}
	h.ns.Hub().Register(client)
	defer h.ns.Hub().Unregister(client)

	ctx := c.Request.Context()
	for {
		select {
		case <-ctx.Done():
			return nil, nil
		case msg, ok := <-client.Send:
			if !ok {
				return nil, nil
			}
			if _, err := c.Writer.Write(msg); err != nil {
				return nil, nil
			}
			flusher.Flush()
		}
	}
}

// MarkAsRead marks a notification as read
func (h *NotificationHandler) MarkAsRead(c *gin.Context) (any, error) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		return nil, errx.BadRequest("无效的通知 ID")
	}
	middleware.AuditSummary(c, "标记通知已读 "+strconv.FormatInt(id, 10))
	if err := h.ns.MarkAsRead(id); err != nil {
		return nil, err
	}
	return gin.H{"message": "已标记为已读"}, nil
}

// MarkAllAsRead marks all as read
func (h *NotificationHandler) MarkAllAsRead(c *gin.Context) (any, error) {
	middleware.AuditSummary(c, "全部标记已读")
	if err := h.ns.MarkAllAsRead(); err != nil {
		return nil, err
	}
	return gin.H{"message": "全部已标记为已读"}, nil
}

// Delete removes a notification
func (h *NotificationHandler) Delete(c *gin.Context) (any, error) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		return nil, errx.BadRequest("无效的通知 ID")
	}
	middleware.AuditSummary(c, "删除通知 "+strconv.FormatInt(id, 10))
	if err := h.ns.Delete(id); err != nil {
		return nil, err
	}
	return gin.H{"message": "已删除"}, nil
}

func RegisterRoutes(protected *gin.RouterGroup, ns *notification.Service) {
	handler := NewNotificationHandler(ns)

	notifGroup := protected.Group("/notifications")
	{
		notifGroup.GET("", httpx.H(handler.List))
		notifGroup.GET("/unread-count", httpx.H(handler.CountUnread))
		notifGroup.GET("/stream", httpx.H(handler.HandleSSE))
		notifGroup.POST("", httpx.H(handler.Create))
		notifGroup.PUT("/:id/read", httpx.H(handler.MarkAsRead))
		notifGroup.PUT("/read-all", httpx.H(handler.MarkAllAsRead))
		notifGroup.DELETE("/:id", httpx.H(handler.Delete))
	}
}

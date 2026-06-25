package api

import (
	"easyserver/internal/service"

	"github.com/gin-gonic/gin"
)

// registerNotificationRoutes registers notification routes
func registerNotificationRoutes(protected *gin.RouterGroup, ns *service.NotificationService) {
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

package notification

import (
	"fmt"
	"sync"
)

// NotificationClient represents a subscriber to notification SSE stream
type NotificationClient struct {
	Send chan []byte
}

// NotificationHub manages connected notification SSE clients
type NotificationHub struct {
	clients sync.Map // map[*NotificationClient]struct{}
}

// NewNotificationHub creates a new NotificationHub
func NewNotificationHub() *NotificationHub {
	return &NotificationHub{}
}

// Register adds a client to the hub
func (h *NotificationHub) Register(c *NotificationClient) {
	h.clients.Store(c, struct{}{})
}

// Unregister removes a client from the hub and closes its channel
func (h *NotificationHub) Unregister(c *NotificationClient) {
	if _, ok := h.clients.Load(c); ok {
		h.clients.Delete(c)
		close(c.Send)
	}
}

// Broadcast sends a formatted SSE event to all connected clients
func (h *NotificationHub) Broadcast(event string, payload []byte) {
	msg := fmt.Appendf(nil, "event: %s\ndata: %s\n\n", event, payload)
	h.clients.Range(func(key, value any) bool {
		c := key.(*NotificationClient)
		select {
		case c.Send <- msg:
		default:
			// Skip slow clients
		}
		return true
	})
}

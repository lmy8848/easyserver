package httpx

import (
	"github.com/coder/websocket"
)

// AcceptWebSocketOptions returns standard AcceptOptions for WebSocket connections
// relying entirely on Token-based authentication (Sec-WebSocket-Protocol) to prevent CSWSH.
// Shared by WebSocket handlers.
func AcceptWebSocketOptions() *websocket.AcceptOptions {
	return &websocket.AcceptOptions{
		Subprotocols:       []string{"token"},
		InsecureSkipVerify: true,
	}
}

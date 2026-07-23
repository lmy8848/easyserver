package httpx

import (
	"net/http"

	gorillaWs "github.com/gorilla/websocket"
)

// CreateUpgrader creates a WebSocket upgrader without origin checking,
// relying entirely on Token-based authentication (Sec-WebSocket-Protocol) to prevent CSWSH.
// Shared by the monitor, service (systemd) and terminal WebSocket handlers.
func CreateUpgrader() gorillaWs.Upgrader {
	return gorillaWs.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		Subprotocols:    []string{"token"},
		CheckOrigin: func(r *http.Request) bool {
			// Skip origin check as we rely on JWT token via Sec-WebSocket-Protocol
			return true
		},
	}
}

package httpx

import (
	"net/http"
	"strings"

	gorillaWs "github.com/gorilla/websocket"
)

// CreateUpgrader creates a WebSocket upgrader with origin checking.
// Shared by the monitor, service (systemd) and terminal WebSocket handlers.
func CreateUpgrader(allowedOrigins []string, devMode bool) gorillaWs.Upgrader {
	return gorillaWs.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		Subprotocols:    []string{"token"},
		CheckOrigin: func(r *http.Request) bool {
			origin := r.Header.Get("Origin")

			// In dev mode, allow localhost and any same-host origin (e.g. Vite dev server)
			if devMode {
				if strings.HasPrefix(origin, "http://localhost:") || strings.HasPrefix(origin, "http://127.0.0.1:") {
					return true
				}
				// Allow origins from the same host (e.g. http://124.221.35.180:5173)
				if host := r.Host; host != "" {
					hostPart := host
					if idx := strings.Index(host, ":"); idx >= 0 {
						hostPart = host[:idx]
					}
					if strings.Contains(origin, "://"+hostPart) {
						return true
					}
				}
			}

			// If no origins configured, deny all (except dev mode localhost)
			if len(allowedOrigins) == 0 {
				return false
			}

			// Check against allowed origins
			for _, allowed := range allowedOrigins {
				if origin == allowed {
					return true
				}
			}

			return false
		},
	}
}

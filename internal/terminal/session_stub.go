//go:build linux && !cgo
// +build linux,!cgo

package terminal

import (
	"fmt"
	"time"

	"easyserver/internal/executor"
)

// CreateSession creates a new terminal session (stub: CGO required for PTY).
func (m *Manager) CreateSession(id string) (*Session, error) {
	return nil, fmt.Errorf("terminal requires CGO; rebuild with CGO_ENABLED=1")
}

// HandleInput handles input from WebSocket.
func (s *Session) HandleInput(msg []byte) error {
	return fmt.Errorf("terminal requires CGO; rebuild with CGO_ENABLED=1")
}

// StartIdleTimeout is a no-op without CGO.
func (m *Manager) StartIdleTimeout(timeout time.Duration) {
	// No-op without CGO
}

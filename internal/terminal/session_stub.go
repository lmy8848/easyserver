//go:build linux && !cgo
// +build linux,!cgo

package terminal

import "fmt"

// CreateSession creates a new terminal session (stub: CGO required for PTY).
func (m *Manager) CreateSession(id string) (*Session, error) {
	return nil, fmt.Errorf("terminal requires CGO; rebuild with CGO_ENABLED=1")
}

// HandleInput handles input from WebSocket.
func (s *Session) HandleInput(msg []byte) error {
	return fmt.Errorf("terminal requires CGO; rebuild with CGO_ENABLED=1")
}

// Close closes the terminal session (stub: no-op without CGO).
func (s *Session) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
}

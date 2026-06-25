//go:build linux && !cgo
// +build linux,!cgo

package service

import (
	"fmt"
	"sync"
	"time"

	"easyserver/internal/executor"
)

type TerminalSession struct {
	ID     string
	Send   chan []byte
	mu     sync.Mutex
	closed bool
}

type TerminalManager struct {
	mu       sync.RWMutex
	sessions map[string]*TerminalSession
	executor executor.CommandExecutor
}

func NewTerminalManager(exec executor.CommandExecutor) *TerminalManager {
	return &TerminalManager{
		sessions: make(map[string]*TerminalSession),
		executor: exec,
	}
}

// CreateSession creates a new terminal session (stub: CGO required for PTY)
func (m *TerminalManager) CreateSession(id string) (*TerminalSession, error) {
	return nil, fmt.Errorf("terminal requires CGO; rebuild with CGO_ENABLED=1")
}

// GetSession returns a terminal session
func (m *TerminalManager) GetSession(id string) (*TerminalSession, error) {
	return nil, fmt.Errorf("terminal requires CGO; rebuild with CGO_ENABLED=1")
}

// CloseSession closes a terminal session
func (m *TerminalManager) CloseSession(id string) error {
	return fmt.Errorf("terminal requires CGO; rebuild with CGO_ENABLED=1")
}

// HandleInput handles input from WebSocket
func (s *TerminalSession) HandleInput(msg []byte) error {
	return fmt.Errorf("terminal requires CGO; rebuild with CGO_ENABLED=1")
}

// StartIdleTimeout starts idle timeout checker
func (m *TerminalManager) StartIdleTimeout(timeout time.Duration) {
	// No-op without CGO
}

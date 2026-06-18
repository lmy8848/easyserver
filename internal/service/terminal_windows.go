//go:build windows
// +build windows

package service

import (
	"fmt"
	"sync"
	"time"
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
}

func NewTerminalManager() *TerminalManager {
	return &TerminalManager{
		sessions: make(map[string]*TerminalSession),
	}
}

// CreateSession creates a new terminal session (Windows stub)
func (m *TerminalManager) CreateSession(id string) (*TerminalSession, error) {
	return nil, fmt.Errorf("terminal is not supported on Windows")
}

// GetSession returns a terminal session
func (m *TerminalManager) GetSession(id string) (*TerminalSession, error) {
	return nil, fmt.Errorf("terminal is not supported on Windows")
}

// CloseSession closes a terminal session
func (m *TerminalManager) CloseSession(id string) error {
	return fmt.Errorf("terminal is not supported on Windows")
}

// HandleInput handles input from WebSocket
func (s *TerminalSession) HandleInput(msg []byte) error {
	return fmt.Errorf("terminal is not supported on Windows")
}

// StartIdleTimeout starts idle timeout checker
func (m *TerminalManager) StartIdleTimeout(timeout time.Duration) {
	// No-op on Windows
}

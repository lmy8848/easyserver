//go:build linux && cgo
// +build linux,cgo

package service

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"sync"
	"time"

	"easyserver/internal/executor"

	"github.com/creack/pty"
)

// Terminal session constants
const (
	// TermPTYReadBufSize is the buffer size for reading from PTY
	TermPTYReadBufSize = 4096
	// TermSendChanSize is the buffer size for the terminal send channel
	TermSendChanSize = 256
)

type TerminalSession struct {
	ID     string
	PTY    *os.File
	Cmd    *exec.Cmd
	Send   chan []byte
	mu     sync.Mutex
	closed bool
	// LastActivity tracks the last time input was received
	LastActivity time.Time
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

// CreateSession creates a new terminal session
func (m *TerminalManager) CreateSession(id string) (*TerminalSession, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Check if session already exists
	if _, exists := m.sessions[id]; exists {
		return nil, fmt.Errorf("session %s already exists", id)
	}

	// Get shell
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/bash"
	}

	// Create command through executor so process creation still goes through the shared seam.
	cmd := m.executor.Command(nil, executor.StartOptions{Env: []string{"TERM=xterm-256color"}}, shell)
	if cmd == nil {
		return nil, fmt.Errorf("executor returned nil command")
	}

	// Start PTY
	ptmx, err := pty.Start(cmd)
	if err != nil {
		return nil, fmt.Errorf("failed to start PTY: %w", err)
	}

	session := &TerminalSession{
		ID:           id,
		PTY:          ptmx,
		Cmd:          cmd,
		Send:         make(chan []byte, TermSendChanSize),
		LastActivity: time.Now(),
	}

	m.sessions[id] = session

	// Start reading from PTY
	go session.readLoop()

	log.Printf("terminal: created session %s", id)
	return session, nil
}

// GetSession returns a terminal session
func (m *TerminalManager) GetSession(id string) (*TerminalSession, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	session, exists := m.sessions[id]
	if !exists {
		return nil, fmt.Errorf("session %s not found", id)
	}

	return session, nil
}

// CloseSession closes a terminal session
func (m *TerminalManager) CloseSession(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	session, exists := m.sessions[id]
	if !exists {
		return fmt.Errorf("session %s not found", id)
	}

	session.Close()
	delete(m.sessions, id)
	log.Printf("terminal: closed session %s", id)
	return nil
}

// Resize resizes the PTY
func (s *TerminalSession) Resize(cols, rows uint16) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return fmt.Errorf("session is closed")
	}

	return pty.Setsize(s.PTY, &pty.Winsize{
		Cols: cols,
		Rows: rows,
	})
}

// Write writes data to the PTY
func (s *TerminalSession) Write(data []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return fmt.Errorf("session is closed")
	}

	_, err := s.PTY.Write(data)
	return err
}

// UpdateActivity records the last activity time
func (s *TerminalSession) UpdateActivity() {
	s.mu.Lock()
	s.LastActivity = time.Now()
	s.mu.Unlock()
}

// IsClosed returns whether the session is closed
func (s *TerminalSession) IsClosed() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closed
}

// Close closes the terminal session
func (s *TerminalSession) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return
	}

	s.closed = true

	// Kill process before closing PTY to avoid race condition:
	// closing PTY while Read is blocked can cause FD reuse issues.
	if s.Cmd.Process != nil {
		s.Cmd.Process.Kill()
		s.Cmd.Wait()
	}

	s.PTY.Close()
	close(s.Send)
}

// readLoop reads from PTY and sends to WebSocket
func (s *TerminalSession) readLoop() {
	buf := make([]byte, TermPTYReadBufSize)

	for {
		// Check closed flag before attempting read
		if s.IsClosed() {
			return
		}

		n, err := s.PTY.Read(buf)
		if err != nil {
			if !s.IsClosed() {
				// Send exit message
				exitMsg, _ := json.Marshal(map[string]interface{}{
					"type": "exit",
					"code": 0,
				})
				select {
				case s.Send <- exitMsg:
				default:
				}
			}
			return
		}

		if n > 0 {
			// Send output message
			outputMsg, _ := json.Marshal(map[string]interface{}{
				"type": "output",
				"data": string(buf[:n]),
			})
			select {
			case s.Send <- outputMsg:
			default:
				// Drop message if buffer is full
			}
		}
	}
}

// HandleInput handles input from WebSocket
func (s *TerminalSession) HandleInput(msg []byte) error {
	var input struct {
		Type string `json:"type"`
		Data string `json:"data"`
		Cols uint16 `json:"cols"`
		Rows uint16 `json:"rows"`
	}

	if err := json.Unmarshal(msg, &input); err != nil {
		return err
	}

	switch input.Type {
	case "input":
		s.UpdateActivity()
		return s.Write([]byte(input.Data))
	case "resize":
		return s.Resize(input.Cols, input.Rows)
	default:
		return fmt.Errorf("unknown message type: %s", input.Type)
	}
}

// StartIdleTimeout starts idle timeout checker
func (m *TerminalManager) StartIdleTimeout(timeout time.Duration) {
	go func() {
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()

		for range ticker.C {
			now := time.Now()

			// Use RLock first to check sessions
			m.mu.RLock()
			var toClose []string
			for id, session := range m.sessions {
				if !session.IsClosed() {
					session.mu.Lock()
					idle := now.Sub(session.LastActivity)
					session.mu.Unlock()
					if idle >= timeout {
						toClose = append(toClose, id)
					}
				}
			}
			m.mu.RUnlock()

			// Close timed out sessions with write lock
			if len(toClose) > 0 {
				m.mu.Lock()
				for _, id := range toClose {
					if session, exists := m.sessions[id]; exists {
						session.Close()
						delete(m.sessions, id)
						log.Printf("terminal: session %s idle timeout", id)
					}
				}
				m.mu.Unlock()
			}
		}
	}()
}

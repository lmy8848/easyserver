package terminal

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"time"

	"easyserver/internal/executor"

	"github.com/creack/pty"
)

const (
	// PTYReadBufSize is the buffer size for reading from PTY
	PTYReadBufSize = 4096
	// SendChanSize is the buffer size for the terminal send channel
	SendChanSize = 256
)

// CreateSession creates a new terminal session.
func (m *Manager) CreateSession(id string) (*Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.sessions[id]; exists {
		return nil, fmt.Errorf("session %s already exists", id)
	}

	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/bash"
	}

	cmd := m.executor.Command(nil, executor.StartOptions{Env: []string{"TERM=xterm-256color"}}, shell)
	if cmd == nil {
		return nil, fmt.Errorf("executor returned nil command")
	}

	ptmx, err := pty.Start(cmd)
	if err != nil {
		return nil, fmt.Errorf("failed to start PTY: %w", err)
	}

	session := &Session{
		ID:           id,
		PTY:          ptmx,
		Cmd:          cmd,
		Send:         make(chan []byte, SendChanSize),
		LastActivity: time.Now(),
	}

	m.sessions[id] = session
	go session.readLoop()

	log.Printf("terminal: created session %s", id)
	return session, nil
}

// Resize resizes the PTY.
func (s *Session) Resize(cols, rows uint16) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return fmt.Errorf("session is closed")
	}

	ptmx, ok := s.PTY.(*os.File)
	if !ok {
		return fmt.Errorf("invalid PTY type")
	}

	return pty.Setsize(ptmx, &pty.Winsize{
		Cols: cols,
		Rows: rows,
	})
}

// Write writes data to the PTY.
func (s *Session) Write(data []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return fmt.Errorf("session is closed")
	}

	ptmx, ok := s.PTY.(*os.File)
	if !ok {
		return fmt.Errorf("invalid PTY type")
	}

	_, err := ptmx.Write(data)
	return err
}

// Close closes the terminal session.
func (s *Session) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return
	}

	s.closed = true

	// Close PTY first to unblock any pending Read
	if ptmx, ok := s.PTY.(*os.File); ok {
		ptmx.Close()
	}

	// Then kill the process
	if cmd, ok := s.Cmd.(*exec.Cmd); ok && cmd.Process != nil {
		cmd.Process.Kill()
		cmd.Wait()
	}

	close(s.Send)
}

// readLoop reads from PTY and sends to WebSocket.
func (s *Session) readLoop() {
	buf := make([]byte, PTYReadBufSize)

	for {
		if s.IsClosed() {
			return
		}

		ptmx, ok := s.PTY.(*os.File)
		if !ok {
			return
		}

		n, err := ptmx.Read(buf)
		if err != nil {
			if !s.IsClosed() {
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
			outputMsg, _ := json.Marshal(map[string]interface{}{
				"type": "output",
				"data": string(buf[:n]),
			})
			select {
			case s.Send <- outputMsg:
			default:
			}
		}
	}
}

// HandleInput handles input from WebSocket.
func (s *Session) HandleInput(msg []byte) error {
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

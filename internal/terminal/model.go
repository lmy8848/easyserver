package terminal

import (
	"fmt"
	"sync"
	"time"

	"easyserver/internal/infra/executor"
)

// Session represents a terminal session.
type Session struct {
	ID           string
	PTY          interface{} // *os.File on linux+cgo, nil on stub
	Cmd          interface{} // *exec.Cmd on linux+cgo, nil on stub
	Send         chan []byte
	mu           sync.Mutex
	closed       bool
	LastActivity time.Time
}

// IsClosed returns whether the session is closed.
func (s *Session) IsClosed() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closed
}

// UpdateActivity records the last activity time.
func (s *Session) UpdateActivity() {
	s.mu.Lock()
	s.LastActivity = time.Now()
	s.mu.Unlock()
}

// Manager manages terminal sessions.
const MaxTerminalSessions = 10

type Manager struct {
	mu       sync.RWMutex
	sessions map[string]*Session
	executor executor.CommandExecutor
	done     chan struct{} // Channel to signal shutdown
}

// NewManager creates a new terminal Manager.
func NewManager(exec executor.CommandExecutor) *Manager {
	return &Manager{
		sessions: make(map[string]*Session),
		executor: exec,
		done:     make(chan struct{}),
	}
}

// GetSession returns a terminal session.
func (m *Manager) GetSession(id string) (*Session, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	session, exists := m.sessions[id]
	if !exists {
		return nil, fmt.Errorf("session %s not found", id)
	}

	return session, nil
}

// CloseSession closes a terminal session.
func (m *Manager) CloseSession(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	session, exists := m.sessions[id]
	if !exists {
		return fmt.Errorf("session %s not found", id)
	}

	session.Close()
	delete(m.sessions, id)
	return nil
}

// StartIdleTimeout starts idle timeout checker.
func (m *Manager) StartIdleTimeout(timeout time.Duration) {
	go func() {
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()

		for {
			select {
			case <-m.done:
				return
			case now := <-ticker.C:
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

				if len(toClose) > 0 {
					m.mu.Lock()
					for _, id := range toClose {
						if session, exists := m.sessions[id]; exists {
							session.Close()
							delete(m.sessions, id)
						}
					}
					m.mu.Unlock()
				}
			}
		}
	}()
}

// StopIdleTimeout stops the idle timeout checker.
func (m *Manager) StopIdleTimeout() {
	close(m.done)
}

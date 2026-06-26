package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"regexp"
	"strings"
	"sync"
	"time"

	"easyserver/internal/audit"
	"easyserver/internal/terminal"

	"github.com/gin-gonic/gin"
	gorillaWs "github.com/gorilla/websocket"
)

// Terminal WebSocket constants
const (
	// TermWSWriteDeadline is the deadline for writing a message to the WebSocket
	TermWSWriteDeadline = 10 * time.Second
	// TermWSReadDeadline is the deadline for reading a message from the WebSocket
	TermWSReadDeadline = 60 * time.Second
	// TermWSPingInterval is the interval for sending ping messages
	TermWSPingInterval = 30 * time.Second
	// TermWSReadLimit is the maximum message size for WebSocket reads
	TermWSReadLimit = 4096
	// TermCmdBufLimit is the maximum size of the command buffer for audit logging
	TermCmdBufLimit = 8192
)

// sessionIDRegex validates terminal session IDs to prevent injection
var sessionIDRegex = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,64}$`)

// formatDuration formats a duration into a human-readable string
func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%d秒", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%d分%d秒", int(d.Minutes()), int(d.Seconds())%60)
	}
	return fmt.Sprintf("%d小时%d分", int(d.Hours()), int(d.Minutes())%60)
}

type TerminalHandler struct {
	terminalManager *terminal.Manager
	auditService    *audit.Service
	jwtSecret       string
	upgrader        gorillaWs.Upgrader
}

func NewTerminalHandler(terminalManager *terminal.Manager, jwtSecret string, auditService *audit.Service, allowedOrigins []string, devMode bool) *TerminalHandler {
	return &TerminalHandler{
		terminalManager: terminalManager,
		auditService:    auditService,
		jwtSecret:       jwtSecret,
		upgrader:        createUpgrader(allowedOrigins, devMode),
	}
}

// wsMessage represents a WebSocket message from the client
type wsMessage struct {
	Type string `json:"type"`
	Data string `json:"data"`
}

// HandleWebSocket handles terminal WebSocket connections
func (h *TerminalHandler) HandleWebSocket(c *gin.Context) {
	// User info already set by WSAuthMiddleware
	userIDIface, ok := c.Get("user_id")
	if !ok {
		c.Error(ErrUnauthorized.WithMessage("用户ID未找到"))
		return
	}
	userID, ok := userIDIface.(int64)
	if !ok {
		c.Error(ErrInternal.WithMessage("用户ID类型无效"))
		return
	}
	usernameIface, ok := c.Get("username")
	if !ok {
		c.Error(ErrUnauthorized.WithMessage("用户名未找到"))
		return
	}
	username, ok := usernameIface.(string)
	if !ok {
		c.Error(ErrInternal.WithMessage("用户名类型无效"))
		return
	}

	// Get and validate session ID from URL
	sessionID := c.Param("id")
	if sessionID == "" {
		c.Error(ErrBadRequest.WithMessage("会话ID不能为空"))
		return
	}
	if !sessionIDRegex.MatchString(sessionID) {
		c.Error(ErrBadRequest.WithMessage("会话ID格式无效"))
		return
	}

	// Create or get session
	session, err := h.terminalManager.CreateSession(sessionID)
	if err != nil {
		// Try to get existing session
		session, err = h.terminalManager.GetSession(sessionID)
		if err != nil {
			c.Error(WrapError(err))
			return
		}
	}

	// Log terminal session start
	sessionStartTime := time.Now()
	if h.auditService != nil {
		h.auditService.LogOperation(c.Request.Context(), userID, username, "TERMINAL_OPEN",
			"/terminal/"+sessionID, "Terminal session opened", c.ClientIP(), c.Request.UserAgent())
	}

	// Upgrade to WebSocket
	conn, err := h.upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("terminal: websocket upgrade error: %v", err)
		h.terminalManager.CloseSession(sessionID)
		return
	}

	// wsWrite is the channel for serialized WebSocket writes.
	// writePump reads from it; the forwarding goroutine and readPump write to it.
	wsWrite := make(chan []byte, 256)

	// Start writePump goroutine to serialize all WebSocket writes
	var writePumpWg sync.WaitGroup
	writePumpWg.Add(1)
	go func() {
		defer writePumpWg.Done()
		h.writePump(conn, wsWrite)
	}()

	// Start forwarding goroutine: session.Send (PTY output) -> wsWrite
	var fwdWg sync.WaitGroup
	fwdWg.Add(1)
	go func() {
		defer fwdWg.Done()
		for msg := range session.Send {
			select {
			case wsWrite <- msg:
			default:
			}
		}
	}()

	// readPump: reads from WebSocket, writes input to PTY
	h.readPump(c, conn, session, wsWrite, userID, username, sessionID)

	// Cleanup sequence:
	// 1. Close session -> closes session.Send -> forwarding goroutine exits
	// 2. Wait for forwarding goroutine
	// 3. Close wsWrite -> writePump exits
	// 4. Wait for writePump
	// 5. Close WebSocket
	h.terminalManager.CloseSession(sessionID)
	fwdWg.Wait()
	close(wsWrite)
	writePumpWg.Wait()
	conn.Close()

	// Log terminal session close with duration
	if h.auditService != nil {
		duration := time.Since(sessionStartTime)
		durationStr := formatDuration(duration)
		h.auditService.LogOperation(context.Background(), userID, username, "TERMINAL_CLOSE",
			"/terminal/"+sessionID,
			fmt.Sprintf("Terminal session closed, duration: %s", durationStr),
			c.ClientIP(), c.Request.UserAgent())
	}
}

// writePump handles all writes to the WebSocket connection.
// It reads from wsWrite (PTY output and pong responses) and sends periodic pings.
// All writes go through a mutex to comply with gorilla/websocket's concurrency requirements.
func (h *TerminalHandler) writePump(conn *gorillaWs.Conn, wsWrite <-chan []byte) {
	ticker := time.NewTicker(TermWSPingInterval)
	defer ticker.Stop()

	writeMu := &sync.Mutex{}

	writeMsg := func(msgType int, data []byte) bool {
		writeMu.Lock()
		defer writeMu.Unlock()
		conn.SetWriteDeadline(time.Now().Add(TermWSWriteDeadline))
		if err := conn.WriteMessage(msgType, data); err != nil {
			return false
		}
		return true
	}

	for {
		select {
		case msg, ok := <-wsWrite:
			if !ok {
				writeMsg(gorillaWs.CloseMessage, []byte{})
				return
			}
			if !writeMsg(gorillaWs.TextMessage, msg) {
				return
			}
		case <-ticker.C:
			if !writeMsg(gorillaWs.PingMessage, nil) {
				return
			}
		}
	}
}

// readPump reads messages from the WebSocket connection and handles them.
// It writes ping/pong responses through wsWrite to ensure serialized WebSocket writes.
func (h *TerminalHandler) readPump(c *gin.Context, conn *gorillaWs.Conn, session *terminal.Session, wsWrite chan<- []byte, userID int64, username string, sessionID string) {
	// Command buffer for logging
	var commandBuffer strings.Builder
	commandBuffer.Grow(256)

	// Set up read deadline and pong handler
	conn.SetReadLimit(TermWSReadLimit)
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(TermWSReadDeadline))
		return nil
	})

	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			if gorillaWs.IsUnexpectedCloseError(err, gorillaWs.CloseGoingAway, gorillaWs.CloseNormalClosure) {
				log.Printf("terminal: websocket read error: %v", err)
			}
			break
		}

		// Parse message type
		var msgType wsMessage
		if err := json.Unmarshal(msg, &msgType); err != nil {
			continue
		}

		// Handle ping/pong
		if msgType.Type == "ping" {
			// Route pong through wsWrite to ensure serialized writes
			select {
			case wsWrite <- []byte(`{"type":"pong"}`):
			default:
			}
			continue
		}

		// Track terminal input for command logging
		if msgType.Type == "input" {
			for _, ch := range msgType.Data {
				if ch == '\r' || ch == '\n' {
					// Enter pressed - log the command
					cmd := strings.TrimSpace(commandBuffer.String())
					if cmd != "" && h.auditService != nil {
						h.auditService.LogTerminalCommand(
							c.Request.Context(), userID, username, sessionID, cmd, c.ClientIP())
					}
					commandBuffer.Reset()
				} else if ch == 127 || ch == '\b' {
					// Backspace - remove last char
					str := commandBuffer.String()
					if len(str) > 0 {
						commandBuffer.Reset()
						commandBuffer.WriteString(str[:len(str)-1])
					}
				} else if ch >= 32 {
					// Prevent unbounded growth of command buffer
					if commandBuffer.Len() < TermCmdBufLimit {
						commandBuffer.WriteByte(byte(ch))
					}
				}
			}
		}

		// Handle terminal input
		if err := session.HandleInput(msg); err != nil {
			log.Printf("terminal: handle input error: %v", err)
		}
	}
}

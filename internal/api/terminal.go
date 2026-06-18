package api

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"easyserver/internal/service"

	"github.com/gin-gonic/gin"
	gorillaWs "github.com/gorilla/websocket"
)

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
	terminalManager *service.TerminalManager
	auditService    *service.AuditService
	jwtSecret       string
	upgrader        gorillaWs.Upgrader
}

func NewTerminalHandler(jwtSecret string, auditService *service.AuditService, allowedOrigins []string, devMode bool) *TerminalHandler {
	return &TerminalHandler{
		terminalManager: service.NewTerminalManager(),
		auditService:    auditService,
		jwtSecret:       jwtSecret,
		upgrader:        createUpgrader(allowedOrigins, devMode),
	}
}

// HandleWebSocket handles terminal WebSocket connections
func (h *TerminalHandler) HandleWebSocket(c *gin.Context) {
	// User info already set by WSAuthMiddleware
	userID, _ := c.Get("user_id")
	username, _ := c.Get("username")

	// Get session ID from URL
	sessionID := c.Param("id")
	if sessionID == "" {
		BadRequest(c, "session id is required")
		return
	}

	// Create or get session
	session, err := h.terminalManager.CreateSession(sessionID)
	if err != nil {
		// Try to get existing session
		session, err = h.terminalManager.GetSession(sessionID)
		if err != nil {
			InternalError(c, err.Error())
			return
		}
	}

	// Log terminal session start
	sessionStartTime := time.Now()
	if h.auditService != nil {
		h.auditService.LogOperation(userID.(int64), username.(string), "TERMINAL_OPEN",
			"/terminal/"+sessionID, "Terminal session opened", c.ClientIP(), c.Request.UserAgent())
	}

	// Upgrade to WebSocket
	conn, err := h.upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("terminal: websocket upgrade error: %v", err)
		h.terminalManager.CloseSession(sessionID)
		return
	}

	defer func() {
		conn.Close()
		h.terminalManager.CloseSession(sessionID)
		// Log terminal session close with duration
		if h.auditService != nil {
			duration := time.Since(sessionStartTime)
			durationStr := formatDuration(duration)
			h.auditService.LogOperation(userID.(int64), username.(string), "TERMINAL_CLOSE",
				"/terminal/"+sessionID,
				fmt.Sprintf("Terminal session closed, duration: %s", durationStr),
				c.ClientIP(), c.Request.UserAgent())
		}
	}()

	// Command buffer for logging
	var commandBuffer strings.Builder
	commandBuffer.Grow(256)

	// Write goroutine - send terminal output to WebSocket
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case msg, ok := <-session.Send:
				if !ok {
					conn.WriteMessage(gorillaWs.CloseMessage, []byte{})
					return
				}
				conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
				if err := conn.WriteMessage(gorillaWs.TextMessage, msg); err != nil {
					return
				}
			case <-ticker.C:
				conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
				if err := conn.WriteMessage(gorillaWs.PingMessage, nil); err != nil {
					return
				}
			}
		}
	}()

	// Read goroutine - receive input from WebSocket
	conn.SetReadLimit(4096)
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(60 * time.Second))
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
		var msgType struct {
			Type string `json:"type"`
			Data string `json:"data"`
		}
		if err := json.Unmarshal(msg, &msgType); err != nil {
			continue
		}

		// Handle ping/pong
		if msgType.Type == "ping" {
			conn.WriteMessage(gorillaWs.TextMessage, []byte(`{"type":"pong"}`))
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
							userID.(int64), username.(string), sessionID, cmd, c.ClientIP())
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
					// Printable character
					commandBuffer.WriteByte(byte(ch))
				}
			}
		}

		// Handle terminal input
		if err := session.HandleInput(msg); err != nil {
			log.Printf("terminal: handle input error: %v", err)
		}
	}
}

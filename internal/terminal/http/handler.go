package http

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"easyserver/internal/httpx"
	"easyserver/internal/httpx/middleware"
	"easyserver/internal/infra"
	"easyserver/internal/infra/errx"
	"easyserver/internal/terminal"

	"github.com/coder/websocket"
	"github.com/gin-gonic/gin"
)

// Terminal WebSocket constants
const (
	// TermWSWriteDeadline is the deadline for writing a message to the WebSocket
	TermWSWriteDeadline = 10 * time.Second
	// TermWSPingInterval is the interval for sending ping keepalives
	TermWSPingInterval = 30 * time.Second
	// TermWSReadLimit is the maximum message size for WebSocket reads
	TermWSReadLimit = 4096
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

// OperationLogger records terminal session operations (open/close) for audit.
// *audit.Service satisfies this interface implicitly.
type OperationLogger interface {
	LogOperation(ctx context.Context, userID int64, username, action, resource string, extra map[string]any, ip, userAgent string)
}

type TerminalHandler struct {
	terminalManager *terminal.Manager
	auditLog        OperationLogger
	jwtSecret       string
	acceptOpts      *websocket.AcceptOptions
}

func NewTerminalHandler(terminalManager *terminal.Manager, jwtSecret string, auditLog OperationLogger, allowedOrigins []string, devMode bool) *TerminalHandler {
	return &TerminalHandler{
		terminalManager: terminalManager,
		auditLog:        auditLog,
		jwtSecret:       jwtSecret,
		acceptOpts:      httpx.AcceptWebSocketOptions(),
	}
}

// HandleWebSocket handles terminal WebSocket connections
func (h *TerminalHandler) HandleWebSocket(c *gin.Context) (any, error) {
	// User info already set by WSAuthMiddleware
	userIDIface, ok := c.Get("user_id")
	if !ok {
		return nil, errx.Unauthorized("用户ID未找到")
	}
	userID, ok := userIDIface.(int64)
	if !ok {
		return nil, errx.Internal("用户ID类型无效")
	}
	usernameIface, ok := c.Get("username")
	if !ok {
		return nil, errx.Unauthorized("用户名未找到")
	}
	username, ok := usernameIface.(string)
	if !ok {
		return nil, errx.Internal("用户名类型无效")
	}

	// Generate unique session ID
	var randBytes [8]byte
	_, _ = rand.Read(randBytes[:])
	sessionID := fmt.Sprintf("term-%d-%s", time.Now().UnixMilli(), hex.EncodeToString(randBytes[:]))

	// Create session
	session, err := h.terminalManager.CreateSession(sessionID)
	if err != nil {
		return nil, err
	}

	// Log terminal session start
	sessionStartTime := time.Now()
	if h.auditLog != nil {
		h.auditLog.LogOperation(c.Request.Context(), userID, username, string(middleware.ActionExecute),
			string(middleware.ResourceTerminal), map[string]any{"summary": "终端会话已打开", "session_id": sessionID}, c.ClientIP(), c.Request.UserAgent())
	}

	// Accept WebSocket connection
	conn, err := websocket.Accept(c.Writer, c.Request, h.acceptOpts)
	if err != nil {
		log.Printf("terminal: websocket accept error: %v", err)
		_ = h.terminalManager.CloseSession(sessionID)
		return nil, nil
	}
	defer func() { _ = conn.CloseNow() }()

	ctx, cancel := context.WithCancel(c.Request.Context())
	defer cancel()

	// Forward PTY output to WebSocket
	var fwdWg sync.WaitGroup
	fwdWg.Add(1)
	infra.Go(func() {
		defer fwdWg.Done()
		defer cancel()
		for {
			select {
			case msg, ok := <-session.Send:
				if !ok {
					return
				}
				writeCtx, writeCancel := context.WithTimeout(ctx, TermWSWriteDeadline)
				err := conn.Write(writeCtx, websocket.MessageText, msg)
				writeCancel()
				if err != nil {
					return
				}
			case <-session.Done():
				return
			case <-ctx.Done():
				return
			}
		}
	})

	// Periodic ping keepalive
	var pingWg sync.WaitGroup
	pingWg.Add(1)
	infra.Go(func() {
		defer pingWg.Done()
		ticker := time.NewTicker(TermWSPingInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				pingCtx, pingCancel := context.WithTimeout(ctx, TermWSWriteDeadline)
				err := conn.Ping(pingCtx)
				pingCancel()
				if err != nil {
					cancel()
					return
				}
			case <-ctx.Done():
				return
			}
		}
	})

	// Read loop: reads from WebSocket, writes input to PTY
	conn.SetReadLimit(TermWSReadLimit)
	for {
		_, msg, err := conn.Read(ctx)
		if err != nil {
			if websocket.CloseStatus(err) != websocket.StatusNormalClosure &&
				websocket.CloseStatus(err) != websocket.StatusGoingAway &&
				!errors.Is(err, context.Canceled) {
				log.Printf("terminal: websocket read error: %v", err)
			}
			break
		}

		// Handle terminal input
		if err := session.HandleInput(msg); err != nil {
			log.Printf("terminal: handle input error: %v", err)
		}
	}

	// Cleanup sequence
	cancel()
	_ = h.terminalManager.CloseSession(sessionID)
	fwdWg.Wait()
	pingWg.Wait()
	_ = conn.Close(websocket.StatusNormalClosure, "terminal session closed")

	// Log terminal session close with duration
	if h.auditLog != nil {
		duration := time.Since(sessionStartTime)
		durationStr := formatDuration(duration)
		h.auditLog.LogOperation(context.Background(), userID, username, string(middleware.ActionExecute),
			string(middleware.ResourceTerminal),
			map[string]any{"summary": "终端会话已关闭", "duration": durationStr, "session_id": sessionID},
			c.ClientIP(), c.Request.UserAgent())
	}
	return nil, nil
}

// RegisterRoutes registers terminal WebSocket routes
func RegisterRoutes(wsGroup *gin.RouterGroup, terminalManager *terminal.Manager, jwtSecret string, auditLog OperationLogger, allowedOrigins []string, devMode bool) {
	handler := NewTerminalHandler(terminalManager, jwtSecret, auditLog, allowedOrigins, devMode)
	wsGroup.GET("/terminal", httpx.H(handler.HandleWebSocket))
}

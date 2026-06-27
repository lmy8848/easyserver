package api

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"easyserver/internal/audit"
	"easyserver/internal/infra/executor"
	"easyserver/internal/monitor"
	"easyserver/internal/systemd"
	"easyserver/internal/terminal"

	"github.com/gin-gonic/gin"
	gorillaWs "github.com/gorilla/websocket"
)

// Monitor WebSocket constants
const (
	// MonitorWSPingInterval is the interval for sending ping messages
	MonitorWSPingInterval = 30 * time.Second
	// MonitorWSWriteDeadline is the deadline for writing a message to the WebSocket
	MonitorWSWriteDeadline = 10 * time.Second
	// MonitorWSReadDeadline is the deadline for reading a message from the WebSocket
	MonitorWSReadDeadline = 60 * time.Second
	// MonitorWSReadLimit is the maximum message size for WebSocket reads
	MonitorWSReadLimit = 512
)

// createUpgrader creates a WebSocket upgrader with origin checking
func createUpgrader(allowedOrigins []string, devMode bool) gorillaWs.Upgrader {
	return gorillaWs.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		Subprotocols:    []string{"token"},
		CheckOrigin: func(r *http.Request) bool {
			// In dev mode, allow localhost
			if devMode {
				origin := r.Header.Get("Origin")
				if strings.HasPrefix(origin, "http://localhost:") || strings.HasPrefix(origin, "http://127.0.0.1:") {
					return true
				}
			}

			// If no origins configured, deny all (except dev mode localhost)
			if len(allowedOrigins) == 0 {
				return false
			}

			// Check against allowed origins
			origin := r.Header.Get("Origin")
			for _, allowed := range allowedOrigins {
				if origin == allowed {
					return true
				}
			}

			return false
		},
	}
}

type MonitorHandler struct {
	monitorService *monitor.MonitorService
	jwtSecret      string
	upgrader       gorillaWs.Upgrader
}

func NewMonitorHandler(monitorService *monitor.MonitorService, jwtSecret string, allowedOrigins []string, devMode bool) *MonitorHandler {
	return &MonitorHandler{
		monitorService: monitorService,
		jwtSecret:      jwtSecret,
		upgrader:       createUpgrader(allowedOrigins, devMode),
	}
}

func (h *MonitorHandler) HandleStats(c *gin.Context) {
	snapshot, err := h.monitorService.GetCurrentStats(c.Request.Context())
	if err != nil {
		Success(c, nil)
		return
	}

	Success(c, snapshot)
}

func (h *MonitorHandler) HandleHistory(c *gin.Context) {
	now := time.Now().UTC()

	startStr := c.DefaultQuery("start", now.Add(-1*time.Hour).Format(time.RFC3339))
	endStr := c.DefaultQuery("end", now.Format(time.RFC3339))

	start, err := time.Parse(time.RFC3339, startStr)
	if err != nil {
		c.Error(ErrBadRequest.WithMessage("无效的开始时间"))
		return
	}

	end, err := time.Parse(time.RFC3339, endStr)
	if err != nil {
		c.Error(ErrBadRequest.WithMessage("无效的结束时间"))
		return
	}

	// Validate range
	if start.After(end) {
		c.Error(ErrBadRequest.WithMessage("开始时间必须早于结束时间"))
		return
	}

	// Cap maximum range to 7 days
	maxRange := 7 * 24 * time.Hour
	if end.Sub(start) > maxRange {
		start = end.Add(-maxRange)
	}

	points, err := h.monitorService.GetHistory(c.Request.Context(), start, end)
	if err != nil {
		c.Error(ErrInternal.WithMessage("查询历史数据失败"))
		return
	}

	snapshots := make([]interface{}, len(points))
	for i, p := range points {
		snapshots[i] = p.ToSnapshot()
	}

	Success(c, gin.H{
		"points": snapshots,
	})
}

func (h *MonitorHandler) HandleWebSocket(c *gin.Context) {
	// User info already set by WSAuthMiddleware
	conn, err := h.upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("monitor ws upgrade error: %v", err)
		return
	}

	client := &monitor.MonitorClient{
		Send: make(chan []byte, 16),
	}

	h.monitorService.Hub().Register(client)

	defer func() {
		h.monitorService.Hub().Unregister(client)
		conn.Close()
	}()

	// Write mutex ensures only one goroutine writes to the connection at a time
	writeMu := &sync.Mutex{}

	go func() {
		ticker := time.NewTicker(MonitorWSPingInterval)
		defer ticker.Stop()

		for {
			select {
			case msg, ok := <-client.Send:
				if !ok {
					writeMu.Lock()
					conn.WriteMessage(gorillaWs.CloseMessage, []byte{})
					writeMu.Unlock()
					return
				}
				writeMu.Lock()
				conn.SetWriteDeadline(time.Now().Add(MonitorWSWriteDeadline))
				if err := conn.WriteMessage(gorillaWs.TextMessage, msg); err != nil {
					writeMu.Unlock()
					return
				}
				writeMu.Unlock()
			case <-ticker.C:
				writeMu.Lock()
				conn.SetWriteDeadline(time.Now().Add(MonitorWSWriteDeadline))
				if err := conn.WriteMessage(gorillaWs.PingMessage, nil); err != nil {
					writeMu.Unlock()
					return
				}
				writeMu.Unlock()
			}
		}
	}()

	conn.SetReadLimit(MonitorWSReadLimit)
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(MonitorWSReadDeadline))
		return nil
	})

	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			break
		}

		var req map[string]interface{}
		if err := json.Unmarshal(msg, &req); err != nil {
			continue
		}

		if req["type"] == "ping" {
			// Send pong through channel to avoid concurrent write
			select {
			case client.Send <- []byte(`{"type":"pong"}`):
			default:
			}
		}
	}
}

func registerMonitorRoutes(protected *gin.RouterGroup, wsGroup *gin.RouterGroup, monitorService *monitor.MonitorService, jwtSecret string, allowedOrigins []string, devMode bool) {
	handler := NewMonitorHandler(monitorService, jwtSecret, allowedOrigins, devMode)
	protected.GET("/monitor/stats", handler.HandleStats)
	protected.GET("/monitor/history", handler.HandleHistory)
	wsGroup.GET("/monitor", handler.HandleWebSocket)
}

// registerServiceRoutes registers service management routes
func registerServiceRoutes(protected *gin.RouterGroup, wsGroup *gin.RouterGroup, serviceManager *systemd.ServiceManager, exec executor.CommandExecutor, jwtSecret string, allowedOrigins []string, devMode bool) {
	handler := NewServiceHandler(serviceManager, exec, jwtSecret, allowedOrigins, devMode)
	protected.GET("/services", handler.List)
	protected.GET("/services/:name", handler.Get)
	protected.GET("/services/:name/logs", handler.GetLogs)
	protected.POST("/services/:name/start", handler.Start)
	protected.POST("/services/:name/stop", handler.Stop)
	protected.POST("/services/:name/restart", handler.Restart)
	protected.POST("/services/:name/enable", handler.Enable)
	protected.POST("/services/:name/disable", handler.Disable)
	wsGroup.GET("/services/:name/logs", handler.HandleLogsWebSocket)
}

// registerTerminalRoutes registers terminal routes
func registerTerminalRoutes(protected *gin.RouterGroup, wsGroup *gin.RouterGroup, terminalManager *terminal.Manager, jwtSecret string, auditService *audit.Service, allowedOrigins []string, devMode bool) {
	protected.GET("/terminal/:id", func(c *gin.Context) {
		Success(c, nil)
	})
	handler := NewTerminalHandler(terminalManager, jwtSecret, auditService, allowedOrigins, devMode)
	wsGroup.GET("/terminal/:id", handler.HandleWebSocket)
}

// registerSystemRoutes registers system routes
func registerSystemRoutes(protected *gin.RouterGroup, exec executor.CommandExecutor) {
	handler := NewSystemHandler(exec)
	protected.GET("/system/ssh-logins", handler.GetSSHLogins)
	protected.GET("/system/ssh-config", handler.GetSystemSSHConfig)
	protected.GET("/system/check-port", handler.CheckPort)
	protected.GET("/system/check-ports", handler.CheckPorts)
}

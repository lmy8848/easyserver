package api

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"

	"easyserver/internal/service"

	"github.com/gin-gonic/gin"
	gorillaWs "github.com/gorilla/websocket"
)

// createUpgrader creates a WebSocket upgrader with origin checking
func createUpgrader(allowedOrigins []string, devMode bool) gorillaWs.Upgrader {
	return gorillaWs.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
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
	monitorService *service.MonitorService
	jwtSecret      string
	upgrader       gorillaWs.Upgrader
}

func NewMonitorHandler(monitorService *service.MonitorService, jwtSecret string, allowedOrigins []string, devMode bool) *MonitorHandler {
	return &MonitorHandler{
		monitorService: monitorService,
		jwtSecret:      jwtSecret,
		upgrader:       createUpgrader(allowedOrigins, devMode),
	}
}

func (h *MonitorHandler) HandleStats(c *gin.Context) {
	snapshot, err := h.monitorService.GetCurrentStats()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"code":    0,
			"message": "ok",
			"data":    nil,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "ok",
		"data":    snapshot,
	})
}

func (h *MonitorHandler) HandleHistory(c *gin.Context) {
	now := time.Now().UTC()

	startStr := c.DefaultQuery("start", now.Add(-1*time.Hour).Format(time.RFC3339))
	endStr := c.DefaultQuery("end", now.Format(time.RFC3339))

	start, err := time.Parse(time.RFC3339, startStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40000,
			"message": "invalid start time",
		})
		return
	}

	end, err := time.Parse(time.RFC3339, endStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40000,
			"message": "invalid end time",
		})
		return
	}

	points, err := h.monitorService.GetHistory(start, end)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    50000,
			"message": "failed to query history",
		})
		return
	}

	snapshots := make([]interface{}, len(points))
	for i, p := range points {
		snapshots[i] = p.ToSnapshot()
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "ok",
		"data": gin.H{
			"points": snapshots,
		},
	})
}

func (h *MonitorHandler) HandleWebSocket(c *gin.Context) {
	// User info already set by WSAuthMiddleware
	conn, err := h.upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("monitor ws upgrade error: %v", err)
		return
	}

	client := &service.MonitorClient{
		Send: make(chan []byte, 16),
	}

	h.monitorService.Hub().Register(client)

	defer func() {
		h.monitorService.Hub().Unregister(client)
		conn.Close()
	}()

	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case msg, ok := <-client.Send:
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

	conn.SetReadLimit(512)
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(60 * time.Second))
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
			conn.WriteMessage(gorillaWs.TextMessage, []byte(`{"type":"pong"}`))
		}
	}
}

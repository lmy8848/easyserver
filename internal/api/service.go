package api

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os/exec"
	"regexp"
	"strconv"
	"time"

	"easyserver/internal/service"

	"github.com/gin-gonic/gin"
	gorillaWs "github.com/gorilla/websocket"
)

var serviceNameRegex = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

func validateServiceName(name string) bool {
	return serviceNameRegex.MatchString(name)
}

// logsUpgrader is now replaced by h.upgrader in ServiceHandler

type ServiceHandler struct {
	serviceManager *service.ServiceManager
	jwtSecret      string
	upgrader       gorillaWs.Upgrader
}

func NewServiceHandler(jwtSecret string, allowedOrigins []string, devMode bool) *ServiceHandler {
	return &ServiceHandler{
		serviceManager: service.NewServiceManager(),
		jwtSecret:      jwtSecret,
		upgrader:       createUpgrader(allowedOrigins, devMode),
	}
}

// List returns all services
func (h *ServiceHandler) List(c *gin.Context) {
	services, err := h.serviceManager.List()
	if err != nil {
		InternalError(c, err.Error())
		return
	}

	Success(c, services)
}

// Get returns a specific service
func (h *ServiceHandler) Get(c *gin.Context) {
	name := c.Param("name")
	if name == "" || !validateServiceName(name) {
		BadRequest(c, "invalid service name")
		return
	}

	svc, err := h.serviceManager.Get(name)
	if err != nil {
		NotFound(c, err.Error())
		return
	}

	Success(c, svc)
}

// Start starts a service
func (h *ServiceHandler) Start(c *gin.Context) {
	name := c.Param("name")
	if name == "" || !validateServiceName(name) {
		BadRequest(c, "invalid service name")
		return
	}

	if err := h.serviceManager.Start(name); err != nil {
		InternalError(c, err.Error())
		return
	}

	Success(c, gin.H{"name": name, "state": "active"})
}

// Stop stops a service
func (h *ServiceHandler) Stop(c *gin.Context) {
	name := c.Param("name")
	if name == "" || !validateServiceName(name) {
		BadRequest(c, "invalid service name")
		return
	}

	if err := h.serviceManager.Stop(name); err != nil {
		InternalError(c, err.Error())
		return
	}

	Success(c, gin.H{"name": name, "state": "inactive"})
}

// Restart restarts a service
func (h *ServiceHandler) Restart(c *gin.Context) {
	name := c.Param("name")
	if name == "" || !validateServiceName(name) {
		BadRequest(c, "invalid service name")
		return
	}

	if err := h.serviceManager.Restart(name); err != nil {
		InternalError(c, err.Error())
		return
	}

	Success(c, gin.H{"name": name, "state": "active"})
}

// Enable enables a service
func (h *ServiceHandler) Enable(c *gin.Context) {
	name := c.Param("name")
	if name == "" || !validateServiceName(name) {
		BadRequest(c, "invalid service name")
		return
	}

	if err := h.serviceManager.Enable(name); err != nil {
		InternalError(c, err.Error())
		return
	}

	Success(c, gin.H{"name": name, "enabled": true})
}

// Disable disables a service
func (h *ServiceHandler) Disable(c *gin.Context) {
	name := c.Param("name")
	if name == "" || !validateServiceName(name) {
		BadRequest(c, "invalid service name")
		return
	}

	if err := h.serviceManager.Disable(name); err != nil {
		InternalError(c, err.Error())
		return
	}

	Success(c, gin.H{"name": name, "enabled": false})
}

// GetLogs returns service logs
func (h *ServiceHandler) GetLogs(c *gin.Context) {
	name := c.Param("name")
	if name == "" || !validateServiceName(name) {
		BadRequest(c, "invalid service name")
		return
	}

	tailStr := c.DefaultQuery("tail", "100")
	tail, err := strconv.Atoi(tailStr)
	if err != nil || tail <= 0 {
		tail = 100
	}

	since := c.Query("since")

	logs, err := h.serviceManager.GetLogs(name, tail, since)
	if err != nil {
		InternalError(c, err.Error())
		return
	}

	Success(c, gin.H{"lines": logs})
}

// HandleLogsWebSocket streams service logs via WebSocket
func (h *ServiceHandler) HandleLogsWebSocket(c *gin.Context) {
	// User info already set by WSAuthMiddleware
	name := c.Param("name")
	if name == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    40000,
			"message": "service name is required",
			"data":    nil,
		})
		return
	}

	conn, err := h.upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("service logs ws upgrade error: %v", err)
		return
	}
	defer conn.Close()

	// Start journalctl -f to follow logs
	cmd := exec.Command("journalctl", "-u", name+".service", "-f", "--no-pager", "--output=json")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		conn.WriteMessage(gorillaWs.TextMessage, []byte(`{"type":"error","message":"failed to get log stream"}`))
		return
	}

	if err := cmd.Start(); err != nil {
		conn.WriteMessage(gorillaWs.TextMessage, []byte(`{"type":"error","message":"failed to start log stream"}`))
		return
	}
	defer cmd.Process.Kill()

	scanner := bufio.NewScanner(stdout)
	msgCh := make(chan []byte, 64)
	errCh := make(chan error, 1)

	go func() {
		for scanner.Scan() {
			line := scanner.Text()

			// Parse journalctl JSON output
			var entry struct {
				Message           string `json:"MESSAGE"`
				RealtimeTimestamp string `json:"__REALTIME_TIMESTAMP"`
				Priority          string `json:"PRIORITY"`
			}

			logData := map[string]interface{}{
				"time":     time.Now().Format("2006-01-02 15:04:05"),
				"message":  line,
				"priority": "info",
			}

			if err := json.Unmarshal([]byte(line), &entry); err == nil && entry.Message != "" {
				logTime := time.Now().Format("2006-01-02 15:04:05")
				if entry.RealtimeTimestamp != "" {
					var usec int64
					if _, err := fmt.Sscanf(entry.RealtimeTimestamp, "%d", &usec); err == nil {
						logTime = time.Unix(usec/1000000, (usec%1000000)*1000).Format("2006-01-02 15:04:05")
					}
				}

				priority := "info"
				switch entry.Priority {
				case "0":
					priority = "emerg"
				case "1":
					priority = "alert"
				case "2":
					priority = "crit"
				case "3":
					priority = "err"
				case "4":
					priority = "warn"
				case "5":
					priority = "notice"
				case "6":
					priority = "info"
				case "7":
					priority = "debug"
				}

				logData["time"] = logTime
				logData["message"] = entry.Message
				logData["priority"] = priority
			}

			msg, _ := json.Marshal(map[string]interface{}{
				"type": "log",
				"data": logData,
			})
			select {
			case msgCh <- msg:
			default:
			}
		}
		errCh <- scanner.Err()
	}()

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	done := make(chan struct{})
	defer close(done)

	// Start read goroutine to detect client disconnect
	go func() {
		conn.SetReadLimit(512)
		conn.SetPongHandler(func(string) error {
			conn.SetReadDeadline(time.Now().Add(60 * time.Second))
			return nil
		})
		for {
			_, _, err := conn.ReadMessage()
			if err != nil {
				return
			}
		}
	}()

	for {
		select {
		case msg := <-msgCh:
			conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := conn.WriteMessage(gorillaWs.TextMessage, msg); err != nil {
				return
			}
		case <-errCh:
			return
		case <-ticker.C:
			conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := conn.WriteMessage(gorillaWs.PingMessage, nil); err != nil {
				return
			}
		case <-done:
			return
		}
	}
}

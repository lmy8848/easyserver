package api

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"easyserver/internal/audit"
	"easyserver/internal/infra/executor"
	"easyserver/internal/systemd"

	"github.com/gin-gonic/gin"
	gorillaWs "github.com/gorilla/websocket"
)

var serviceNameRegex = regexp.MustCompile(`^[a-zA-Z0-9_@.:%\\-]+$`)

func validateServiceName(name string) bool {
	return serviceNameRegex.MatchString(name)
}

// logsUpgrader is now replaced by h.upgrader in ServiceHandler

type ServiceHandler struct {
	serviceManager    *systemd.ServiceManager
	executor          executor.CommandExecutor
	jwtSecret         string
	upgrader          gorillaWs.Upgrader
	auditService      *audit.Service
	protectedServices []string // Services that cannot be stopped/disabled
}

func NewServiceHandler(serviceManager *systemd.ServiceManager, exec executor.CommandExecutor, jwtSecret string, auditService *audit.Service, allowedOrigins []string, devMode bool) *ServiceHandler {
	return &ServiceHandler{
		serviceManager:    serviceManager,
		executor:          exec,
		jwtSecret:         jwtSecret,
		auditService:      auditService,
		upgrader:          createUpgrader(allowedOrigins, devMode),
		protectedServices: []string{"easyserver"}, // Panel's own service
	}
}

// isProtectedService checks if a service is protected
func (h *ServiceHandler) isProtectedService(name string) bool {
	for _, svc := range h.protectedServices {
		if svc == name {
			return true
		}
	}
	return false
}

// List returns all services
func (h *ServiceHandler) List(c *gin.Context) {
	services, err := h.serviceManager.List(c.Request.Context())
	if err != nil {
		c.Error(WrapError(err))
		return
	}

	Success(c, services)
}

// GetDetails returns detailed info (PID, memory, enabled) for specific services
func (h *ServiceHandler) GetDetails(c *gin.Context) {
	var req struct {
		Names []string `json:"names"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || len(req.Names) == 0 {
		c.Error(ErrBadRequest.WithMessage("缺少服务名称参数"))
		return
	}

	for _, name := range req.Names {
		name = strings.TrimSpace(name)
		if name == "" || !validateServiceName(name) {
			c.Error(ErrBadRequest.WithMessage("无效的服务名称"))
			return
		}
	}

	details, err := h.serviceManager.GetDetails(c.Request.Context(), req.Names)
	if err != nil {
		c.Error(WrapError(err))
		return
	}

	Success(c, details)
}

// Get returns a specific service
func (h *ServiceHandler) Get(c *gin.Context) {
	name := c.Param("name")
	if name == "" || !validateServiceName(name) {
		c.Error(ErrBadRequest.WithMessage("无效的服务名称"))
		return
	}

	svc, err := h.serviceManager.Get(c.Request.Context(), name)
	if err != nil {
		c.Error(ErrNotFound.Wrap(err))
		return
	}

	Success(c, svc)
}

// Start starts a service
func (h *ServiceHandler) Start(c *gin.Context) {
	name := c.Param("name")
	if name == "" || !validateServiceName(name) {
		c.Error(ErrBadRequest.WithMessage("无效的服务名称"))
		return
	}

	if err := h.serviceManager.Start(c.Request.Context(), name); err != nil {
		c.Error(WrapError(err))
		return
	}

	Success(c, gin.H{"name": name, "state": "active"})
}

// Stop stops a service
func (h *ServiceHandler) Stop(c *gin.Context) {
	name := c.Param("name")
	if name == "" || !validateServiceName(name) {
		c.Error(ErrBadRequest.WithMessage("无效的服务名称"))
		return
	}

	// Check if service is protected
	if h.isProtectedService(name) {
		c.Error(ErrBadRequest.WithMessage("无法停止面板自身的服务"))
		return
	}

	if err := h.serviceManager.Stop(c.Request.Context(), name); err != nil {
		c.Error(WrapError(err))
		return
	}

	Success(c, gin.H{"name": name, "state": "inactive"})
}

// Restart restarts a service
func (h *ServiceHandler) Restart(c *gin.Context) {
	name := c.Param("name")
	if name == "" || !validateServiceName(name) {
		c.Error(ErrBadRequest.WithMessage("无效的服务名称"))
		return
	}

	// Check if service is protected
	if h.isProtectedService(name) {
		c.Error(ErrBadRequest.WithMessage("无法从此处重启面板自身服务，请使用 /api/settings/restart"))
		return
	}

	if err := h.serviceManager.Restart(c.Request.Context(), name); err != nil {
		c.Error(WrapError(err))
		return
	}
	Success(c, gin.H{"name": name, "state": "active"})
}

// Enable enables a service
func (h *ServiceHandler) Enable(c *gin.Context) {
	name := c.Param("name")
	if name == "" || !validateServiceName(name) {
		c.Error(ErrBadRequest.WithMessage("无效的服务名称"))
		return
	}

	if err := h.serviceManager.Enable(c.Request.Context(), name); err != nil {
		c.Error(WrapError(err))
		return
	}
	Success(c, gin.H{"name": name, "enabled": true})
}

// Disable disables a service
func (h *ServiceHandler) Disable(c *gin.Context) {
	name := c.Param("name")
	if name == "" || !validateServiceName(name) {
		c.Error(ErrBadRequest.WithMessage("无效的服务名称"))
		return
	}

	// Check if service is protected
	if h.isProtectedService(name) {
		c.Error(ErrBadRequest.WithMessage("无法禁用面板自身的服务"))
		return
	}

	if err := h.serviceManager.Disable(c.Request.Context(), name); err != nil {
		c.Error(WrapError(err))
		return
	}
	Success(c, gin.H{"name": name, "enabled": false})
}

// GetLogs returns service logs
func (h *ServiceHandler) GetLogs(c *gin.Context) {
	name := c.Param("name")
	if name == "" || !validateServiceName(name) {
		c.Error(ErrBadRequest.WithMessage("无效的服务名称"))
		return
	}

	tailStr := c.DefaultQuery("tail", "100")
	tail, err := strconv.Atoi(tailStr)
	if err != nil || tail <= 0 {
		tail = 100
	}

	since := c.Query("since")

	logs, err := h.serviceManager.GetLogs(c.Request.Context(), name, tail, since)
	if err != nil {
		c.Error(WrapError(err))
		return
	}

	Success(c, gin.H{"lines": logs})
}

// HandleLogsWebSocket streams service logs via WebSocket
func (h *ServiceHandler) HandleLogsWebSocket(c *gin.Context) {
	// User info already set by WSAuthMiddleware
	name := c.Param("name")
	if name == "" || !validateServiceName(name) {
		c.Error(ErrBadRequest.WithMessage("无效的服务名称"))
		return
	}

	conn, err := h.upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("service logs ws upgrade error: %v", err)
		return
	}
	defer conn.Close()

	// Write mutex ensures only one goroutine writes to the connection at a time
	writeMu := &sync.Mutex{}

	// Start journalctl -f to follow logs
	proc, err := h.executor.Start(context.Background(), executor.StartOptions{}, "journalctl", "-u", name+".service", "-f", "--no-pager", "--output=json")
	if err != nil {
		writeMu.Lock()
		conn.WriteMessage(gorillaWs.TextMessage, []byte(`{"type":"error","message":"启动日志流失败"}`))
		writeMu.Unlock()
		return
	}
	stdout, err := proc.StdoutPipe()
	if err != nil {
		proc.Kill()
		writeMu.Lock()
		conn.WriteMessage(gorillaWs.TextMessage, []byte(`{"type":"error","message":"获取日志流失败"}`))
		writeMu.Unlock()
		return
	}
	defer proc.Kill()

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
			writeMu.Lock()
			conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := conn.WriteMessage(gorillaWs.TextMessage, msg); err != nil {
				writeMu.Unlock()
				return
			}
			writeMu.Unlock()
		case <-errCh:
			return
		case <-ticker.C:
			writeMu.Lock()
			conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := conn.WriteMessage(gorillaWs.PingMessage, nil); err != nil {
				writeMu.Unlock()
				return
			}
			writeMu.Unlock()
		case <-done:
			return
		}
	}
}

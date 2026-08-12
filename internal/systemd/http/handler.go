package http

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"easyserver/internal/httpx"
	"easyserver/internal/httpx/middleware"
	"easyserver/internal/infra"
	"easyserver/internal/infra/apperror"
	"easyserver/internal/infra/executor"
	"easyserver/internal/systemd"

	"github.com/gin-gonic/gin"
)

var serviceNameRegex = regexp.MustCompile(`^[a-zA-Z0-9_@.:%\\-]+$`)

func validateServiceName(name string) bool {
	return serviceNameRegex.MatchString(name)
}

type ServiceHandler struct {
	serviceManager    *systemd.ServiceManager
	executor          executor.CommandExecutor
	jwtSecret         string
	protectedServices []string // Services that cannot be stopped/disabled
}

func NewServiceHandler(serviceManager *systemd.ServiceManager, exec executor.CommandExecutor, jwtSecret string, allowedOrigins []string, devMode bool) *ServiceHandler {
	return &ServiceHandler{
		serviceManager:    serviceManager,
		executor:          exec,
		jwtSecret:         jwtSecret,
		protectedServices: []string{"easyserver"}, // Panel's own service
	}
}

// isProtectedService checks if a service is protected
func (h *ServiceHandler) isProtectedService(name string) bool {
	baseName := strings.TrimSuffix(name, ".service")
	for _, svc := range h.protectedServices {
		if svc == name || svc == baseName {
			return true
		}
	}
	return false
}

// List returns all services (or managed services only if ?managed=true)
func (h *ServiceHandler) List(c *gin.Context) {
	var services []systemd.ServiceInfo
	var err error
	if c.Query("managed") == "true" {
		services, err = h.serviceManager.ListManaged(c.Request.Context())
	} else {
		services, err = h.serviceManager.List(c.Request.Context())
	}
	if err != nil {
		c.Error(apperror.WrapError(err))
		return
	}

	httpx.Success(c, services)
}

// GetDetails returns detailed info (PID, memory, enabled) for specific services
func (h *ServiceHandler) GetDetails(c *gin.Context) {
	var req struct {
		Names []string `json:"names"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || len(req.Names) == 0 {
		c.Error(apperror.ErrBadRequest.WithMessage("缺少服务名称参数"))
		return
	}

	for _, name := range req.Names {
		name = strings.TrimSpace(name)
		if name == "" || !validateServiceName(name) {
			c.Error(apperror.ErrBadRequest.WithMessage("无效的服务名称"))
			return
		}
	}

	details, err := h.serviceManager.GetDetails(c.Request.Context(), req.Names)
	if err != nil {
		c.Error(apperror.WrapError(err))
		return
	}

	httpx.Success(c, details)
}

// Get returns a specific service
func (h *ServiceHandler) Get(c *gin.Context) {
	name := c.Param("name")
	if name == "" || !validateServiceName(name) {
		c.Error(apperror.ErrBadRequest.WithMessage("无效的服务名称"))
		return
	}

	svc, err := h.serviceManager.Get(c.Request.Context(), name)
	if err != nil {
		c.Error(apperror.ErrNotFound.Wrap(err))
		return
	}

	httpx.Success(c, svc)
}

// Start starts a service
func (h *ServiceHandler) Start(c *gin.Context) {
	name := c.Param("name")
	if name == "" || !validateServiceName(name) {
		c.Error(apperror.ErrBadRequest.WithMessage("无效的服务名称"))
		return
	}

	middleware.AuditSummary(c, "启动系统服务 "+name)
	if err := h.serviceManager.Start(c.Request.Context(), name); err != nil {
		c.Error(apperror.WrapError(err))
		return
	}

	httpx.Success(c, gin.H{"name": name, "state": "active"})
}

// Stop stops a service
func (h *ServiceHandler) Stop(c *gin.Context) {
	name := c.Param("name")
	if name == "" || !validateServiceName(name) {
		c.Error(apperror.ErrBadRequest.WithMessage("无效的服务名称"))
		return
	}

	middleware.AuditSummary(c, "停止系统服务 "+name)

	// Check if service is protected
	if h.isProtectedService(name) {
		c.Error(apperror.ErrBadRequest.WithMessage("无法停止面板自身的服务"))
		return
	}

	if err := h.serviceManager.Stop(c.Request.Context(), name); err != nil {
		c.Error(apperror.WrapError(err))
		return
	}

	httpx.Success(c, gin.H{"name": name, "state": "inactive"})
}

// Restart restarts a service
func (h *ServiceHandler) Restart(c *gin.Context) {
	name := c.Param("name")
	if name == "" || !validateServiceName(name) {
		c.Error(apperror.ErrBadRequest.WithMessage("无效的服务名称"))
		return
	}

	middleware.AuditSummary(c, "重启系统服务 "+name)

	// Check if service is protected
	if h.isProtectedService(name) {
		c.Error(apperror.ErrBadRequest.WithMessage("无法从此处重启面板自身服务，请使用 /api/settings/restart"))
		return
	}

	if err := h.serviceManager.Restart(c.Request.Context(), name); err != nil {
		c.Error(apperror.WrapError(err))
		return
	}
	httpx.Success(c, gin.H{"name": name, "state": "active"})
}

// Enable enables a service
func (h *ServiceHandler) Enable(c *gin.Context) {
	name := c.Param("name")
	if name == "" || !validateServiceName(name) {
		c.Error(apperror.ErrBadRequest.WithMessage("无效的服务名称"))
		return
	}

	middleware.AuditSummary(c, "启用系统服务 "+name)
	if err := h.serviceManager.Enable(c.Request.Context(), name); err != nil {
		c.Error(apperror.WrapError(err))
		return
	}
	httpx.Success(c, gin.H{"name": name, "enabled": true})
}

// Disable disables a service
func (h *ServiceHandler) Disable(c *gin.Context) {
	name := c.Param("name")
	if name == "" || !validateServiceName(name) {
		c.Error(apperror.ErrBadRequest.WithMessage("无效的服务名称"))
		return
	}

	middleware.AuditSummary(c, "禁用系统服务 "+name)

	// Check if service is protected
	if h.isProtectedService(name) {
		c.Error(apperror.ErrBadRequest.WithMessage("无法禁用面板自身的服务"))
		return
	}

	if err := h.serviceManager.Disable(c.Request.Context(), name); err != nil {
		c.Error(apperror.WrapError(err))
		return
	}
	httpx.Success(c, gin.H{"name": name, "enabled": false})
}

// ServiceLogs 是 /services/:name/logs 的总入口：带 ?stream=1 走实时 SSE，否则返回历史日志 REST。
func (h *ServiceHandler) ServiceLogs(c *gin.Context) {
	if c.Query("stream") == "1" {
		h.HandleLogsSSE(c)
		return
	}
	h.GetLogs(c)
}

// GetLogs returns service logs
func (h *ServiceHandler) GetLogs(c *gin.Context) {
	name := c.Param("name")
	if name == "" || !validateServiceName(name) {
		c.Error(apperror.ErrBadRequest.WithMessage("无效的服务名称"))
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
		c.Error(apperror.WrapError(err))
		return
	}

	httpx.Success(c, gin.H{"lines": logs})
}

// HandleLogsSSE 通过 Server-Sent Events 单向推送服务日志（journalctl -f 跟随）。
func (h *ServiceHandler) HandleLogsSSE(c *gin.Context) {
	name := c.Param("name")
	if name == "" || !validateServiceName(name) {
		c.Error(apperror.ErrBadRequest.WithMessage("无效的服务名称"))
		return
	}

	// journalctl -f 跟随；进程独立于本请求（Background），避免客户端断开时被 request ctx 杀。
	// 注意：须先 StdoutPipe 再 Start，否则报 "StdoutPipe after process started"。
	cmd := h.executor.Command(context.Background(), executor.StartOptions{}, "journalctl", "-u", name+".service", "-f", "--no-pager", "--output=json")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		c.Error(apperror.ErrInternal.WithMessage("获取日志流失败"))
		return
	}
	if err := cmd.Start(); err != nil {
		c.Error(apperror.ErrInternal.WithMessage("启动日志流失败"))
		return
	}
	defer func() { _ = cmd.Process.Kill() }()

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		c.Error(apperror.ErrInternal.WithMessage("当前连接不支持流式输出"))
		return
	}
	// 连接建立即 flush 一次，确保客户端立即收到响应头（否则无日志时 header 延迟到心跳）。
	fmt.Fprint(c.Writer, ": connected\n\n")
	flusher.Flush()

	scanner := bufio.NewScanner(stdout)
	msgCh := make(chan []byte, 64)

	infra.Go(func() {
		for scanner.Scan() {
			line := scanner.Text()

			// Parse journalctl JSON output
			var entry struct {
				Message           string `json:"MESSAGE"`
				RealtimeTimestamp string `json:"__REALTIME_TIMESTAMP"`
				Priority          string `json:"PRIORITY"`
			}

			logData := map[string]any{
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

			msg, _ := json.Marshal(map[string]any{
				"type": "log",
				"data": logData,
			})
			select {
			case msgCh <- msg:
			default:
			}
		}
	})

	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case msg := <-msgCh:
			if _, err := fmt.Fprintf(c.Writer, "data: %s\n\n", msg); err != nil {
				return
			}
			flusher.Flush()
		case <-ticker.C:
			// 心跳注释行，避免连接被中间层空闲断开。
			if _, err := fmt.Fprint(c.Writer, ": ping\n\n"); err != nil {
				return
			}
			flusher.Flush()
		case <-c.Request.Context().Done():
			// 客户端断开（fetch abort）时取消 request context。
			return
		}
	}
}

// ============================================================
// 托管服务 CRUD（生成/更新/删除 unit 文件，只对 easyserver-svc- 前缀有效）
// ============================================================

// Create 创建托管服务（生成 unit + 按需 enable/start）。
// 只生成 easyserver-svc-<name>.service，不支持创建系统服务的 unit。
func (h *ServiceHandler) Create(c *gin.Context) {
	var spec systemd.ManagedUnitSpec
	if err := c.ShouldBindJSON(&spec); err != nil {
		c.Error(apperror.ErrBadRequest.WithMessage("参数错误: " + err.Error()))
		return
	}
	spec.Name = strings.TrimPrefix(strings.TrimSpace(spec.Name), "easyserver-svc-")
	middleware.AuditSummary(c, "创建托管服务 "+spec.Name)
	if err := h.serviceManager.CreateManaged(c.Request.Context(), &spec); err != nil {
		c.Error(apperror.WrapError(err))
		return
	}
	httpx.Success(c, gin.H{"message": "创建成功"})
}

// Update 更新托管服务（重写 unit + 运行中则重启）。
// 容易: 须为完整 unit 名（easyserver-svc-<name>），非托管前缀返回错误。
func (h *ServiceHandler) Update(c *gin.Context) {
	name := c.Param("name")
	shortName, err := requireManagedName(name)
	if err != nil {
		c.Error(err)
		return
	}
	var spec systemd.ManagedUnitSpec
	if err := c.ShouldBindJSON(&spec); err != nil {
		c.Error(apperror.ErrBadRequest.WithMessage("参数错误: " + err.Error()))
		return
	}
	spec.Name = shortName
	middleware.AuditSummary(c, "更新托管服务 "+shortName)
	if err := h.serviceManager.UpdateManaged(c.Request.Context(), &spec); err != nil {
		c.Error(apperror.WrapError(err))
		return
	}
	httpx.Success(c, gin.H{"message": "更新成功"})
}

// Delete 删除托管服务（stop + disable + rm unit）。
// 容易: 须为完整 unit 名（easyserver-svc-<name>），非托管前缀返回错误。
func (h *ServiceHandler) Delete(c *gin.Context) {
	name := c.Param("name")
	shortName, err := requireManagedName(name)
	if err != nil {
		c.Error(err)
		return
	}
	middleware.AuditSummary(c, "删除托管服务 "+shortName)
	if err := h.serviceManager.DeleteManaged(c.Request.Context(), shortName); err != nil {
		c.Error(apperror.WrapError(err))
		return
	}
	httpx.Success(c, gin.H{"message": "删除成功"})
}

// requireManagedName 校验 :name 是 easyserver-svc- 前缀的托管服务，
// 返回去掉前缀的短名。非托管前缀返回错误（系统服务不允许 CRUD）。
func requireManagedName(fullName string) (string, error) {
	if fullName == "" {
		return "", apperror.ErrBadRequest.WithMessage("缺少服务名称")
	}
	shortName := systemd.UnitName(fullName + ".service")
	if shortName == "" {
		return "", apperror.ErrBadRequest.WithMessage("只有 easyserver-svc- 托管服务支持此操作")
	}
	return shortName, nil
}

// RegisterRoutes registers service management routes
func RegisterRoutes(protected *gin.RouterGroup, wsGroup *gin.RouterGroup, serviceManager *systemd.ServiceManager, exec executor.CommandExecutor, jwtSecret string, allowedOrigins []string, devMode bool) {
	handler := NewServiceHandler(serviceManager, exec, jwtSecret, allowedOrigins, devMode)
	protected.GET("/services", handler.List)
	protected.POST("/services", handler.Create)
	protected.POST("/services/details", handler.GetDetails)
	protected.GET("/services/:name", handler.Get)
	protected.PUT("/services/:name", handler.Update)
	protected.DELETE("/services/:name", handler.Delete)
	protected.GET("/services/:name/logs", handler.ServiceLogs)
	protected.POST("/services/:name/start", handler.Start)
	protected.POST("/services/:name/stop", handler.Stop)
	protected.POST("/services/:name/restart", handler.Restart)
	protected.POST("/services/:name/enable", handler.Enable)
	protected.POST("/services/:name/disable", handler.Disable)
}

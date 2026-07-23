package http

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"

	"easyserver/internal/httpx"
	"easyserver/internal/infra"
	"easyserver/internal/infra/apperror"
	"easyserver/internal/infra/executor"
	"easyserver/internal/monitor"

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

type MonitorHandler struct {
	monitorService *monitor.MonitorService
	executor       executor.CommandExecutor
	jwtSecret      string
	upgrader       gorillaWs.Upgrader
}

func NewMonitorHandler(monitorService *monitor.MonitorService, exec executor.CommandExecutor, jwtSecret string, allowedOrigins []string, devMode bool) *MonitorHandler {
	return &MonitorHandler{
		monitorService: monitorService,
		executor:       exec,
		jwtSecret:      jwtSecret,
		upgrader:       httpx.CreateUpgrader(),
	}
}

func (h *MonitorHandler) HandleStats(c *gin.Context) {
	snapshot, err := h.monitorService.GetCurrentStats(c.Request.Context())
	if err != nil {
		httpx.Success(c, nil)
		return
	}

	httpx.Success(c, snapshot)
}

func (h *MonitorHandler) HandleHistory(c *gin.Context) {
	now := time.Now().UTC()

	startStr := c.DefaultQuery("start", now.Add(-1*time.Hour).Format(time.RFC3339))
	endStr := c.DefaultQuery("end", now.Format(time.RFC3339))

	start, err := time.Parse(time.RFC3339, startStr)
	if err != nil {
		c.Error(apperror.ErrBadRequest.WithMessage("无效的开始时间"))
		return
	}

	end, err := time.Parse(time.RFC3339, endStr)
	if err != nil {
		c.Error(apperror.ErrBadRequest.WithMessage("无效的结束时间"))
		return
	}

	// Validate range
	if start.After(end) {
		c.Error(apperror.ErrBadRequest.WithMessage("开始时间必须早于结束时间"))
		return
	}

	// Cap maximum range to 7 days
	maxRange := 7 * 24 * time.Hour
	if end.Sub(start) > maxRange {
		start = end.Add(-maxRange)
	}

	points, err := h.monitorService.GetHistory(c.Request.Context(), start, end)
	if err != nil {
		c.Error(apperror.ErrInternal.WithMessage("查询历史数据失败"))
		return
	}

	snapshots := make([]interface{}, len(points))
	for i, p := range points {
		snapshots[i] = p.ToSnapshot()
	}

	httpx.Success(c, gin.H{
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

	infra.Go(func() {
		ticker := time.NewTicker(MonitorWSPingInterval)
		defer ticker.Stop()

		for {
			select {
			case msg, ok := <-client.Send:
				if !ok {
					conn.WriteMessage(gorillaWs.CloseMessage, []byte{})
					return
				}
				conn.SetWriteDeadline(time.Now().Add(MonitorWSWriteDeadline))
				if err := conn.WriteMessage(gorillaWs.TextMessage, msg); err != nil {
					return
				}
			case <-ticker.C:
				conn.SetWriteDeadline(time.Now().Add(MonitorWSWriteDeadline))
				if err := conn.WriteMessage(gorillaWs.PingMessage, nil); err != nil {
					return
				}
			}
		}
	})

	conn.SetReadLimit(MonitorWSReadLimit)
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(MonitorWSReadDeadline))
		return nil
	})

	for {
		_, _, err := conn.ReadMessage()
		if err != nil {
			break
		}
	}
}

// RegisterRoutes registers monitor related routes
func RegisterRoutes(protected *gin.RouterGroup, wsGroup *gin.RouterGroup, monitorService *monitor.MonitorService, exec executor.CommandExecutor, jwtSecret string, allowedOrigins []string, devMode bool) {
	handler := NewMonitorHandler(monitorService, exec, jwtSecret, allowedOrigins, devMode)
	protected.GET("/monitor/stats", handler.HandleStats)
	protected.GET("/monitor/history", handler.HandleHistory)
	wsGroup.GET("/monitor", handler.HandleWebSocket)

	// Processes
	protected.GET("/monitor/processes", handler.ListSystemProcesses)
	protected.GET("/monitor/processes/:pid", handler.GetSystemProcess)

	// Ports (listening ports list + availability check)
	protected.GET("/monitor/ports", handler.GetListeningPorts)
	protected.GET("/monitor/check-port", handler.CheckPort)
}

// PortInfo represents a single listening port entry.
type PortInfo struct {
	Protocol    string `json:"protocol"`     // tcp/tcp6/udp/udp6
	Port        int    `json:"port"`         // local port
	LocalAddr   string `json:"local_addr"`   // full local address
	State       string `json:"state"`        // LISTEN, ESTABLISHED, etc.
	PID         int    `json:"pid"`          // process ID (0 if unknown)
	ProcessName string `json:"process_name"` // process name (if available)
	User        string `json:"user"`         // owning user
}

// GetListeningPorts returns all TCP/UDP listening ports.
func (h *MonitorHandler) GetListeningPorts(c *gin.Context) {
	ports := getListeningPorts()
	httpx.Success(c, gin.H{"ports": ports, "total": len(ports)})
}

// parseProcNet parses /proc/net/tcp, /proc/net/tcp6, /proc/net/udp, /proc/net/udp6.
func parseProcNet(path, proto string) []PortInfo {
	file, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer file.Close()

	var ports []PortInfo
	scanner := bufio.NewScanner(file)
	first := true
	for scanner.Scan() {
		line := scanner.Text()
		if first {
			first = false // skip header
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}

		// local_address is in format "local_address:port" (hex)
		localAddr := fields[1]
		state := fields[3]

		// Only show LISTEN state (0A for TCP)
		if proto == "tcp" || proto == "tcp6" {
			if state != "0A" {
				continue
			}
		}

		hostPort := strings.Split(localAddr, ":")
		if len(hostPort) != 2 {
			continue
		}
		portHex := hostPort[1]
		portVal, err := strconv.ParseInt(portHex, 16, 32)
		if err != nil {
			continue
		}

		// Get inode (field index 9)
		inode := ""
		if len(fields) > 9 {
			inode = fields[9]
		}

		pi := PortInfo{
			Protocol:  proto,
			Port:      int(portVal),
			LocalAddr: formatHostAddr(hostPort[0], int(portVal)),
			State:     "LISTEN",
		}

		if inode != "" && inode != "0" {
			pi.PID = findPIDByInode(inode)
			if pi.PID > 0 {
				pi.ProcessName = getProcessName(pi.PID)
				pi.User = getProcessUser(pi.PID)
			}
		}

		ports = append(ports, pi)
	}
	return ports
}

// formatHostAddr converts hex IP:port to human-readable format.
func formatHostAddr(hexIP string, port int) string {
	if len(hexIP) == 8 {
		// IPv4
		a, _ := strconv.ParseUint(hexIP[6:8], 16, 8)
		b, _ := strconv.ParseUint(hexIP[4:6], 16, 8)
		c, _ := strconv.ParseUint(hexIP[2:4], 16, 8)
		d, _ := strconv.ParseUint(hexIP[0:2], 16, 8)
		return fmt.Sprintf("%d.%d.%d.%d:%d", a, b, c, d, port)
	}
	return fmt.Sprintf("[%s]:%d", hexIP, port)
}

// getListeningPorts aggregates all proc net files.
func getListeningPorts() []PortInfo {
	if runtime.GOOS != "linux" {
		return nil
	}
	var all []PortInfo
	protos := []struct {
		path  string
		proto string
	}{
		{"/proc/net/tcp", "tcp"},
		{"/proc/net/tcp6", "tcp6"},
		{"/proc/net/udp", "udp"},
		{"/proc/net/udp6", "udp6"},
	}
	for _, p := range protos {
		ports := parseProcNet(p.path, p.proto)
		all = append(all, ports...)
	}
	return all
}

// findPIDByInode searches /proc/*/fd/ for a socket matching the inode.
func findPIDByInode(inode string) int {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return 0
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		pid, err := strconv.Atoi(entry.Name())
		if err != nil {
			continue
		}
		fdDir := fmt.Sprintf("/proc/%d/fd", pid)
		fds, err := os.ReadDir(fdDir)
		if err != nil {
			continue
		}
		for _, fd := range fds {
			link, err := os.Readlink(fmt.Sprintf("%s/%s", fdDir, fd.Name()))
			if err != nil {
				continue
			}
			if strings.Contains(link, "socket:") && strings.Contains(link, inode) {
				return pid
			}
		}
		// Limit search for performance
		if pid > 500 {
			break
		}
	}
	return 0
}

func getProcessName(pid int) string {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/comm", pid))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func getProcessUser(pid int) string {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/status", pid))
	if err != nil {
		return ""
	}
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "Uid:") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				return uidToName(fields[1])
			}
		}
	}
	return ""
}

func uidToName(uid string) string {
	data, err := os.ReadFile("/etc/passwd")
	if err != nil {
		return uid
	}
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		parts := strings.Split(scanner.Text(), ":")
		if len(parts) >= 3 && parts[2] == uid {
			return parts[0]
		}
	}
	return uid
}

// ============================================================
// System process monitoring - MonitorHandler methods
// ============================================================

func (h *MonitorHandler) ListSystemProcesses(c *gin.Context) {
	sortBy := c.DefaultQuery("sort_by", "memory")
	order := c.DefaultQuery("order", "desc")
	search := c.Query("search")
	limitStr := c.DefaultQuery("limit", "100")
	limit, _ := strconv.Atoi(limitStr)

	processes, err := h.monitorService.ListSystemProcesses(sortBy, order, search, limit)
	if err != nil {
		c.Error(apperror.WrapError(err))
		return
	}
	httpx.Success(c, processes)
}

func (h *MonitorHandler) GetSystemProcess(c *gin.Context) {
	pidStr := c.Param("pid")
	pid, err := strconv.Atoi(pidStr)
	if err != nil {
		c.Error(apperror.ErrBadRequest.WithMessage("无效的PID"))
		return
	}

	proc, err := h.monitorService.GetSystemProcess(pid)
	if err != nil {
		c.Error(apperror.ErrNotFound.WithMessage(fmt.Sprintf("进程 %d 不存在", pid)))
		return
	}
	httpx.Success(c, proc)
}

// ============================================================
// Port availability check
// ============================================================

// CheckPort checks if a port is available (try to listen) and reports the
// occupying process when it is in use.
func (h *MonitorHandler) CheckPort(c *gin.Context) {
	portStr := c.Query("port")
	if portStr == "" {
		c.Error(apperror.ErrBadRequest.WithMessage("端口不能为空"))
		return
	}

	port, err := strconv.Atoi(portStr)
	if err != nil || port < 1 || port > 65535 {
		c.Error(apperror.ErrBadRequest.WithMessage("无效的端口号 (1-65535)"))
		return
	}

	addr := fmt.Sprintf(":%d", port)
	listener, listenErr := safeListen(addr)
	if listenErr != nil {
		processInfo := h.getPortProcess(c.Request.Context(), port)
		httpx.Success(c, gin.H{
			"available": false,
			"port":      port,
			"process":   processInfo,
			"message":   fmt.Sprintf("端口 %d 已被占用", port),
		})
		return
	}
	if listener != nil {
		listener.Close()
	}

	httpx.Success(c, gin.H{
		"available": true,
		"port":      port,
		"message":   fmt.Sprintf("端口 %d 可用", port),
	})
}

// safeListen wraps net.Listen with panic recovery.
func safeListen(addr string) (net.Listener, error) {
	var listener net.Listener
	var err error
	func() {
		defer func() {
			if r := recover(); r != nil {
				err = fmt.Errorf("listen panic: %v", r)
			}
		}()
		listener, err = net.Listen("tcp", addr)
	}()
	return listener, err
}

// getPortProcess finds the process using a given port via ss/netstat.
func (h *MonitorHandler) getPortProcess(ctx context.Context, port int) string {
	defer func() {
		if r := recover(); r != nil {
			// Silently recover from any parsing panics
		}
	}()

	out, _, err := h.executor.RunCombined(ctx, "ss", "-tlnp", fmt.Sprintf("sport = :%d", port))
	if err == nil && strings.TrimSpace(out) != "" {
		lines := strings.Split(strings.TrimSpace(out), "\n")
		if len(lines) > 1 {
			for _, line := range lines[1:] {
				if strings.Contains(line, fmt.Sprintf(":%d", port)) {
					return strings.TrimSpace(line)
				}
			}
		}
	}

	out, _, err = h.executor.RunCombined(ctx, "netstat", "-tlnp")
	if err == nil {
		for _, line := range strings.Split(out, "\n") {
			if strings.Contains(line, fmt.Sprintf(":%d ", port)) || strings.Contains(line, fmt.Sprintf(":%d\t", port)) {
				return strings.TrimSpace(line)
			}
		}
	}

	return "unknown"
}

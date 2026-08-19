package http

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"

	"easyserver/internal/httpx"
	"easyserver/internal/infra/errx"
	"easyserver/internal/monitor"

	"github.com/gin-gonic/gin"
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
	jwtSecret      string
}

func NewMonitorHandler(monitorService *monitor.MonitorService, jwtSecret string, allowedOrigins []string, devMode bool) *MonitorHandler {
	return &MonitorHandler{
		monitorService: monitorService,
		jwtSecret:      jwtSecret,
	}
}

func (h *MonitorHandler) HandleStats(c *gin.Context) (any, error) {
	snapshot, err := h.monitorService.GetCurrentStats(c.Request.Context())
	if err != nil {
		// 采集失败如实上报，不伪装成功（前端据此显示降级提示）。
		// 底层错误可能含系统路径（/proc 等），前端只见友好文案。
		return nil, errx.Internal("监控数据采集失败: %w", err)
	}

	return snapshot, nil
}

func (h *MonitorHandler) HandleHistory(c *gin.Context) (any, error) {
	now := time.Now().UTC()

	startStr := c.DefaultQuery("start", now.Add(-1*time.Hour).Format(time.RFC3339))
	endStr := c.DefaultQuery("end", now.Format(time.RFC3339))

	start, err := time.Parse(time.RFC3339, startStr)
	if err != nil {
		return nil, errx.BadRequest("无效的开始时间")
	}

	end, err := time.Parse(time.RFC3339, endStr)
	if err != nil {
		return nil, errx.BadRequest("无效的结束时间")
	}

	// Validate range
	if start.After(end) {
		return nil, errx.BadRequest("开始时间必须早于结束时间")
	}

	// Cap maximum range to 7 days
	maxRange := 7 * 24 * time.Hour
	if end.Sub(start) > maxRange {
		start = end.Add(-maxRange)
	}

	points, err := h.monitorService.GetHistory(c.Request.Context(), start, end)
	if err != nil {
		return nil, errx.Internal("查询历史数据失败: %w", err)
	}

	snapshots := make([]any, len(points))
	for i, p := range points {
		snapshots[i] = p.ToSnapshot()
	}

	return gin.H{
		"points": snapshots,
	}, nil
}

// MonitorSSEPingInterval 是 SSE 心跳间隔（注释行保持连接活跃）。
const MonitorSSEPingInterval = 15 * time.Second

// HandleSSE 通过 Server-Sent Events 单向推送系统监控实时数据。
// 鉴权复用 protected 组（Bearer），前端用 fetch 流式读取（可带 Authorization header）。
func (h *MonitorHandler) HandleSSE(c *gin.Context) (any, error) {
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		return nil, errx.Internal("当前连接不支持流式输出")
	}
	// 连接建立即 flush 一次，确保客户端立即收到响应头。
	fmt.Fprint(c.Writer, ": connected\n\n")
	flusher.Flush()

	client := &monitor.MonitorClient{
		Send: make(chan []byte, 16),
	}

	h.monitorService.Hub().Register(client)
	defer h.monitorService.Hub().Unregister(client)

	ticker := time.NewTicker(MonitorSSEPingInterval)
	defer ticker.Stop()

	for {
		select {
		case msg, ok := <-client.Send:
			if !ok {
				return nil, nil
			}
			if _, err := fmt.Fprintf(c.Writer, "data: %s\n\n", msg); err != nil {
				return nil, nil
			}
			flusher.Flush()
		case <-ticker.C:
			// 心跳注释行：保持连接活跃，事件端忽略之。
			if _, err := fmt.Fprint(c.Writer, ": ping\n\n"); err != nil {
				return nil, nil
			}
			flusher.Flush()
		case <-c.Request.Context().Done():
			// 客户端断开（fetch abort）时取消 request context。
			return nil, nil
		}
	}
}

// RegisterRoutes registers monitor related routes
func RegisterRoutes(protected *gin.RouterGroup, wsGroup *gin.RouterGroup, monitorService *monitor.MonitorService, jwtSecret string, allowedOrigins []string, devMode bool) {
	handler := NewMonitorHandler(monitorService, jwtSecret, allowedOrigins, devMode)
	protected.GET("/monitor/stats", httpx.H(handler.HandleStats))
	protected.GET("/monitor/history", httpx.H(handler.HandleHistory))
	protected.GET("/monitor", httpx.H(handler.HandleSSE))

	// Processes
	protected.GET("/monitor/processes", httpx.H(handler.ListSystemProcesses))
	protected.GET("/monitor/processes/:pid", httpx.H(handler.GetSystemProcess))

	// Ports (listening ports list + availability check)
	protected.GET("/monitor/ports", httpx.H(handler.GetListeningPorts))
	protected.GET("/monitor/check-port", httpx.H(handler.CheckPort))
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
func (h *MonitorHandler) GetListeningPorts(c *gin.Context) (any, error) {
	ports := getListeningPorts()
	return gin.H{"ports": ports, "total": len(ports)}, nil
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

// formatHostAddr converts hex IP and port to readable format.
func formatHostAddr(hexIP string, port int) string {
	if len(hexIP) == 8 {
		// IPv4
		var ip = make(net.IP, 4)
		for i := range 4 {
			if b, err := strconv.ParseUint(hexIP[6-i*2:8-i*2], 16, 8); err == nil {
				ip[i] = byte(b)
			}
		}
		return fmt.Sprintf("%s:%d", ip.String(), port)
	} else if len(hexIP) == 32 {
		// IPv6
		var ip = make(net.IP, 16)
		for i := 0; i < 16; i += 4 {
			for j := range 4 {
				if b, err := strconv.ParseUint(hexIP[i*2+(3-j)*2:i*2+(4-j)*2], 16, 8); err == nil {
					ip[i+j] = byte(b)
				}
			}
		}
		return fmt.Sprintf("[%s]:%d", ip.String(), port)
	}
	return fmt.Sprintf("%s:%d", hexIP, port)
}

// findPIDByInode searches /proc/*/fd/* for socket with given inode.
func findPIDByInode(targetInode string) int {
	if runtime.GOOS != "linux" {
		return 0
	}

	target := fmt.Sprintf("socket:[%s]", targetInode)
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
			if link == target {
				return pid
			}
		}
	}

	return 0
}

// getListeningPorts returns all listening ports across TCP/UDP.
func getListeningPorts() []PortInfo {
	var allPorts []PortInfo

	if runtime.GOOS == "linux" {
		allPorts = append(allPorts, parseProcNet("/proc/net/tcp", "tcp")...)
		allPorts = append(allPorts, parseProcNet("/proc/net/tcp6", "tcp6")...)
		allPorts = append(allPorts, parseProcNet("/proc/net/udp", "udp")...)
		allPorts = append(allPorts, parseProcNet("/proc/net/udp6", "udp6")...)
	}

	// Fallback to netstat/ss if /proc/net didn't return anything or not on Linux
	if len(allPorts) == 0 {
		allPorts = parseSSPorts()
	}

	return allPorts
}

// parseSSPorts uses `ss -tulpn` command as fallback.
func parseSSPorts() []PortInfo {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "ss", "-tulpn").Output()
	if err != nil {
		return nil
	}

	var ports []PortInfo
	lines := strings.SplitSeq(string(out), "\n")
	for line := range lines {
		fields := strings.Fields(line)
		if len(fields) < 5 || fields[0] == "Netid" {
			continue
		}

		proto := fields[0]
		state := fields[1]
		localAddr := fields[4]

		if proto != "tcp" && proto != "udp" && proto != "tcp6" && proto != "udp6" {
			continue
		}
		if proto == "tcp" || proto == "tcp6" {
			if state != "LISTEN" {
				continue
			}
		}

		// Parse port from local address
		lastColon := strings.LastIndex(localAddr, ":")
		if lastColon == -1 {
			continue
		}
		portStr := localAddr[lastColon+1:]
		port, err := strconv.Atoi(portStr)
		if err != nil {
			continue
		}

		pi := PortInfo{
			Protocol:  proto,
			Port:      port,
			LocalAddr: localAddr,
			State:     state,
		}

		// Try to parse process info from the last column (e.g., users:(("nginx",pid=123,fd=6)))
		if len(fields) >= 7 {
			procField := fields[6]
			if strings.Contains(procField, "pid=") {
				pidIdx := strings.Index(procField, "pid=") + 4
				pidEnd := strings.IndexAny(procField[pidIdx:], ",)")
				if pidEnd != -1 {
					pid, _ := strconv.Atoi(procField[pidIdx : pidIdx+pidEnd])
					pi.PID = pid
				}
				nameStart := strings.Index(procField, `("`) + 2
				nameEnd := strings.Index(procField[nameStart:], `"`)
				if nameStart > 1 && nameEnd != -1 {
					pi.ProcessName = procField[nameStart : nameStart+nameEnd]
				}
			}
		}

		ports = append(ports, pi)
	}

	return ports
}

func getProcessName(pid int) string {
	comm, err := os.ReadFile(fmt.Sprintf("/proc/%d/comm", pid))
	if err == nil {
		return strings.TrimSpace(string(comm))
	}
	return ""
}

func getProcessUser(pid int) string {
	status, err := os.ReadFile(fmt.Sprintf("/proc/%d/status", pid))
	if err != nil {
		return ""
	}
	scanner := bufio.NewScanner(strings.NewReader(string(status)))
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

func (h *MonitorHandler) ListSystemProcesses(c *gin.Context) (any, error) {
	sortBy := c.DefaultQuery("sort_by", "memory")
	order := c.DefaultQuery("order", "desc")
	search := c.Query("search")
	limitStr := c.DefaultQuery("limit", "100")
	limit, _ := strconv.Atoi(limitStr)

	processes, err := h.monitorService.ListSystemProcesses(sortBy, order, search, limit)
	if err != nil {
		return nil, err
	}
	return processes, nil
}

func (h *MonitorHandler) GetSystemProcess(c *gin.Context) (any, error) {
	pidStr := c.Param("pid")
	pid, err := strconv.Atoi(pidStr)
	if err != nil {
		return nil, errx.BadRequest("无效的PID")
	}

	proc, err := h.monitorService.GetSystemProcess(pid)
	if err != nil {
		return nil, errx.NotFound("进程 %d 不存在", pid)
	}
	return proc, nil
}

// ============================================================
// Port availability check
// ============================================================

// CheckPort checks if a port is available (try to listen) and reports the
// occupying process when it is in use.
func (h *MonitorHandler) CheckPort(c *gin.Context) (any, error) {
	portStr := c.Query("port")
	if portStr == "" {
		return nil, errx.BadRequest("端口不能为空")
	}

	port, err := strconv.Atoi(portStr)
	if err != nil || port < 1 || port > 65535 {
		return nil, errx.BadRequest("无效的端口号 (1-65535)")
	}

	addr := fmt.Sprintf(":%d", port)
	listener, listenErr := safeListen(addr)
	if listenErr != nil {
		processInfo := h.getPortProcess(c.Request.Context(), port)
		return gin.H{ //nolint:nilerr // Port occupation is expected check status, returned in JSON payload
			"available": false,
			"port":      port,
			"process":   processInfo,
			"message":   fmt.Sprintf("端口 %d 已被占用", port),
		}, nil
	}
	if listener != nil {
		listener.Close()
	}

	return gin.H{
		"available": true,
		"port":      port,
		"message":   fmt.Sprintf("端口 %d 可用", port),
	}, nil
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
		lc := net.ListenConfig{}
		listener, err = lc.Listen(context.Background(), "tcp", addr)
	}()
	return listener, err
}

// getPortProcess finds the process using a given port via ss/netstat.
func (h *MonitorHandler) getPortProcess(ctx context.Context, port int) string {
	defer func() {
		if r := recover(); r != nil {
			// 解析过程中意外 panic 不扩散到整个接口，但必须留痕便于排查。
			log.Printf("monitor: getPortProcess(%d) panic recovered: %v", port, r)
		}
	}()

	out, err := exec.CommandContext(ctx, "ss", "-tlnp", fmt.Sprintf("sport = :%d", port)).CombinedOutput()
	if err == nil && strings.TrimSpace(string(out)) != "" {
		lines := strings.Split(strings.TrimSpace(string(out)), "\n")
		if len(lines) > 1 {
			for _, line := range lines[1:] {
				if strings.Contains(line, fmt.Sprintf(":%d", port)) {
					return strings.TrimSpace(line)
				}
			}
		}
	}

	out, err = exec.CommandContext(ctx, "netstat", "-tlnp").CombinedOutput()
	if err == nil {
		for line := range strings.SplitSeq(string(out), "\n") {
			if strings.Contains(line, fmt.Sprintf(":%d ", port)) || strings.Contains(line, fmt.Sprintf(":%d\t", port)) {
				return strings.TrimSpace(line)
			}
		}
	}

	return "unknown"
}

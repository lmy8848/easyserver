package monitor

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"easyserver/internal/infra"
)

const (
	cacheExpiry      = 60 * time.Second // 系统信息/Swap 缓存有效期
	maxHistoryPoints = 360              // 历史数据最大点数
)

// Evaluator evaluates monitor points for alerts.
// Defined locally to avoid circular dependency with service.AlertService.
type Evaluator interface {
	Evaluate(point *MonitorPoint)
}

// SystemEventLogger logs system-level audit events.
type SystemEventLogger interface {
	LogSystemEvent(ctx context.Context, summary string)
}

type MonitorClient struct {
	Send chan []byte
}

type MonitorHub struct {
	mu      sync.RWMutex
	clients map[*MonitorClient]struct{}
}

func NewMonitorHub() *MonitorHub {
	return &MonitorHub{
		clients: make(map[*MonitorClient]struct{}),
	}
}

func (h *MonitorHub) Register(c *MonitorClient) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.clients[c] = struct{}{}
}

func (h *MonitorHub) Unregister(c *MonitorClient) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, ok := h.clients[c]; ok {
		delete(h.clients, c)
		close(c.Send)
	}
}

func (h *MonitorHub) Broadcast(data []byte) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for c := range h.clients {
		select {
		case c.Send <- data:
		default:
			// Skip slow clients
		}
	}
}

type MonitorService struct {
	mu           sync.RWMutex
	monitorRepo  Repository
	interval     time.Duration
	retention    time.Duration
	hub          *MonitorHub
	prevIdle     uint64
	prevTotal    uint64
	prevSent     uint64
	prevRecv     uint64
	prevPktsSent uint64
	prevPktsRecv uint64
	stopCh       chan struct{}
	lastCleanup  time.Time
	ticker       *time.Ticker

	// 性能优化：系统信息缓存
	cachedSystemInfo *SystemInfo
	sysInfoExpire    time.Time
	cachedSwap       *SwapInfo
	swapExpire       time.Time

	// 告警与审计评估
	alertService    Evaluator
	auditService    SystemEventLogger
	lastAuditAlerts map[string]time.Time

	// 性能优化：环形缓冲 + 批量写入
	ringBuffer  []*MonitorPoint
	ringSize    int
	ringHead    int
	ringCount   int
	ringMu      sync.Mutex
	flushTicker *time.Ticker
}

func NewMonitorService(monitorRepo Repository, interval, retention time.Duration) *MonitorService {
	return &MonitorService{
		monitorRepo: monitorRepo,
		interval:    interval,
		retention:   retention,
		hub:         NewMonitorHub(),
		stopCh:      make(chan struct{}),
		ringBuffer:  make([]*MonitorPoint, 60), // 60 points buffer
		ringSize:    60,
	}
}

func (s *MonitorService) Hub() *MonitorHub {
	return s.hub
}

// SetAlertService sets the alert evaluation service
func (s *MonitorService) SetAlertService(e Evaluator) {
	s.alertService = e
}

// SetAuditService sets the audit event logger service.
func (s *MonitorService) SetAuditService(a SystemEventLogger) {
	s.auditService = a
}

// SetInterval updates the collection interval dynamically.
func (s *MonitorService) SetInterval(interval time.Duration) {
	if interval < time.Second {
		interval = time.Second
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.interval = interval
	if s.ticker != nil {
		s.ticker.Reset(interval)
	}
}

// SetRetention updates the history retention duration dynamically.
func (s *MonitorService) SetRetention(retention time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.retention = retention
}

func (s *MonitorService) Start() {
	ctx := context.Background()

	// 性能优化：创建时间戳索引（异步执行，不阻塞启动）
	infra.Go(func() {
		if err := s.monitorRepo.EnsureIndexes(ctx); err != nil {
			log.Printf("monitor: failed to create index: %v", err)
		}
	})

	s.mu.Lock()
	s.ticker = time.NewTicker(s.interval)
	ticker := s.ticker
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		if s.ticker != nil {
			s.ticker.Stop()
			s.ticker = nil
		}
		s.mu.Unlock()
	}()

	// 性能优化：批量写入 ticker（每 10 秒 flush 一次）
	s.flushTicker = time.NewTicker(10 * time.Second)
	defer s.flushTicker.Stop()

	// First collection
	s.collect()

	for {
		select {
		case <-ticker.C:
			s.collect()
		case <-s.flushTicker.C:
			go s.flushBuffer() // 异步 flush，不阻塞采集循环
		case <-s.stopCh:
			s.flushBuffer() // Final flush before stop
			return
		}
	}
}

func (s *MonitorService) Stop() {
	close(s.stopCh)
}

func (s *MonitorService) collect() {
	point := s.readAll()

	// 性能优化：添加到环形缓冲（不直接写 DB）
	s.addToBuffer(point)

	// Cleanup old data every 10 minutes
	if time.Since(s.lastCleanup) > 10*time.Minute {
		s.lastCleanup = time.Now()
		s.cleanup()
	}

	snapshot := point.ToSnapshot()

	// 性能优化：系统信息缓存（60秒刷新）
	snapshot.System = s.readSystemInfoCached()

	// 性能优化：Swap 缓存（复用 meminfo，60秒刷新）
	snapshot.Swap = s.readSwapCached()

	snapshot.Partitions = s.readPartitions()

	// 告警评估
	if s.alertService != nil {
		s.alertService.Evaluate(point)
	}

	// 审计日志触发（内存/磁盘使用率 ≥ 90%）
	if s.auditService != nil {
		s.checkAuditThresholds(snapshot)
	}

	data, err := json.Marshal(map[string]interface{}{
		"type": "stats",
		"data": snapshot,
	})
	if err != nil {
		log.Printf("monitor: marshal error: %v", err)
		return
	}

	s.hub.Broadcast(data)
}

func (s *MonitorService) checkAuditThresholds(snapshot *MonitorSnapshot) {
	if s.auditService == nil {
		return
	}
	now := time.Now()
	const cooldown = 5 * time.Minute

	s.mu.Lock()
	if s.lastAuditAlerts == nil {
		s.lastAuditAlerts = make(map[string]time.Time)
	}
	s.mu.Unlock()

	// 内存使用率 ≥ 90% 检查
	if snapshot.Memory.UsagePercent >= 90 {
		s.mu.Lock()
		last, exists := s.lastAuditAlerts["memory"]
		shouldAlert := !exists || now.Sub(last) >= cooldown
		if shouldAlert {
			s.lastAuditAlerts["memory"] = now
		}
		s.mu.Unlock()

		if shouldAlert {
			s.auditService.LogSystemEvent(context.Background(), fmt.Sprintf("内存使用率告警：%.1f%%", snapshot.Memory.UsagePercent))
		}
	}

	// 磁盘分区使用率 ≥ 90% 检查
	for _, p := range snapshot.Partitions {
		if p.UsagePercent >= 90 {
			key := "disk:" + p.MountPoint
			s.mu.Lock()
			last, exists := s.lastAuditAlerts[key]
			shouldAlert := !exists || now.Sub(last) >= cooldown
			if shouldAlert {
				s.lastAuditAlerts[key] = now
			}
			s.mu.Unlock()

			if shouldAlert {
				s.auditService.LogSystemEvent(context.Background(), fmt.Sprintf("磁盘使用率告警：%s %.1f%%", p.MountPoint, p.UsagePercent))
			}
		}
	}
}

// readSystemInfoCached 带缓存的系统信息读取（cacheExpiry 刷新）
func (s *MonitorService) readSystemInfoCached() *SystemInfo {
	s.mu.RLock()
	if s.cachedSystemInfo != nil && time.Now().Before(s.sysInfoExpire) {
		info := s.cachedSystemInfo
		s.mu.RUnlock()
		return info
	}
	s.mu.RUnlock()

	s.mu.Lock()
	defer s.mu.Unlock()
	// Double-check after acquiring write lock
	if s.cachedSystemInfo != nil && time.Now().Before(s.sysInfoExpire) {
		return s.cachedSystemInfo
	}
	s.cachedSystemInfo = s.readSystemInfo()
	s.sysInfoExpire = time.Now().Add(cacheExpiry)
	return s.cachedSystemInfo
}

// readSwapCached 带缓存的 Swap 读取（cacheExpiry 刷新）
func (s *MonitorService) readSwapCached() *SwapInfo {
	s.mu.RLock()
	if s.cachedSwap != nil && time.Now().Before(s.swapExpire) {
		swap := s.cachedSwap
		s.mu.RUnlock()
		return swap
	}
	s.mu.RUnlock()

	s.mu.Lock()
	defer s.mu.Unlock()
	// Double-check after acquiring write lock
	if s.cachedSwap != nil && time.Now().Before(s.swapExpire) {
		return s.cachedSwap
	}
	s.cachedSwap = s.readSwap()
	s.swapExpire = time.Now().Add(cacheExpiry)
	return s.cachedSwap
}

func (s *MonitorService) readAll() *MonitorPoint {
	now := time.Now().UTC().Format(time.RFC3339)
	p := &MonitorPoint{Timestamp: now}

	s.readCPU(p)
	s.readLoad(p)
	s.readMemory(p)
	s.readDisk(p)
	s.readNetwork(p)

	return p
}

// addToBuffer adds a point to the ring buffer
func (s *MonitorService) addToBuffer(p *MonitorPoint) {
	s.ringMu.Lock()
	s.ringBuffer[s.ringHead] = p
	s.ringHead = (s.ringHead + 1) % s.ringSize
	if s.ringCount < s.ringSize {
		s.ringCount++
	}
	s.ringMu.Unlock()
}

// flushBuffer writes all buffered points to the database
func (s *MonitorService) flushBuffer() {
	s.ringMu.Lock()
	if s.ringCount == 0 {
		s.ringMu.Unlock()
		return
	}
	// Copy points to flush
	points := make([]*MonitorPoint, s.ringCount)
	tail := (s.ringHead - s.ringCount + s.ringSize) % s.ringSize
	for i := 0; i < s.ringCount; i++ {
		points[i] = s.ringBuffer[(tail+i)%s.ringSize]
	}
	s.ringCount = 0
	s.ringMu.Unlock()

	// Batch insert using repository
	if err := s.monitorRepo.SaveBatch(context.Background(), points); err != nil {
		log.Printf("monitor: flush error: %v", err)
	}
}

// GetLatestPoint returns the latest point from the ring buffer
func (s *MonitorService) GetLatestPoint() *MonitorPoint {
	s.ringMu.Lock()
	defer s.ringMu.Unlock()
	if s.ringCount == 0 {
		return nil
	}
	idx := (s.ringHead - 1 + s.ringSize) % s.ringSize
	return s.ringBuffer[idx]
}

func (s *MonitorService) cleanup() {
	ctx := context.Background()
	since := time.Now().UTC().Add(-s.retention)
	rows, err := s.monitorRepo.Clean(ctx, since)
	if err != nil {
		log.Printf("monitor: cleanup error: %v", err)
		return
	}
	if rows > 0 {
		log.Printf("monitor: cleaned up %d old records", rows)
	}
}

func (s *MonitorService) GetCurrentStats(ctx context.Context) (*MonitorSnapshot, error) {
	// 性能优化：从环形缓冲读取最新数据
	p := s.GetLatestPoint()
	if p == nil {
		// Fallback to repository if buffer is empty
		var err error
		p, err = s.monitorRepo.GetLatest(ctx)
		if err != nil {
			return nil, err
		}
		if p == nil {
			return nil, fmt.Errorf("no data yet")
		}
	}
	snapshot := p.ToSnapshot()
	snapshot.System = s.readSystemInfoCached()
	snapshot.Swap = s.readSwapCached()
	snapshot.Partitions = s.readPartitions()
	return snapshot, nil
}

func (s *MonitorService) GetHistory(ctx context.Context, start, end time.Time) ([]MonitorPoint, error) {
	points, err := s.monitorRepo.GetHistory(ctx, start, end)
	if err != nil {
		return nil, err
	}

	// 性能优化：降采样到最多 maxHistoryPoints 个点
	if len(points) > maxHistoryPoints {
		step := len(points) / maxHistoryPoints
		sampled := make([]MonitorPoint, 0, maxHistoryPoints)
		for i := 0; i < len(points); i += step {
			sampled = append(sampled, points[i])
		}
		// 确保包含最后一点
		if len(points) > 0 && (len(sampled) == 0 || sampled[len(sampled)-1].Timestamp != points[len(points)-1].Timestamp) {
			sampled = append(sampled, points[len(points)-1])
		}
		return sampled, nil
	}

	return points, nil
}

func (s *MonitorService) readCPU(p *MonitorPoint) {
	data, err := os.ReadFile("/proc/stat")
	if err != nil {
		log.Printf("monitor: failed to read /proc/stat: %v", err)
		return
	}

	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "cpu ") {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < 5 {
			return
		}

		var total uint64
		for i := 1; i < len(fields); i++ {
			v, _ := strconv.ParseUint(fields[i], 10, 64)
			total += v
		}

		idle, _ := strconv.ParseUint(fields[4], 10, 64)

		if s.prevTotal > 0 {
			diffTotal := total - s.prevTotal
			diffIdle := idle - s.prevIdle
			if diffTotal > 0 {
				p.CPUPercent = math.Round((1-float64(diffIdle)/float64(diffTotal))*100*100) / 100
			}
		}

		s.prevIdle = idle
		s.prevTotal = total
		return
	}
}

func (s *MonitorService) readLoad(p *MonitorPoint) {
	data, err := os.ReadFile("/proc/loadavg")
	if err != nil {
		log.Printf("monitor: failed to read /proc/loadavg: %v", err)
		return
	}

	fields := strings.Fields(string(data))
	if len(fields) < 3 {
		return
	}

	p.CPULoad1m, _ = strconv.ParseFloat(fields[0], 64)
	p.CPULoad5m, _ = strconv.ParseFloat(fields[1], 64)
	p.CPULoad15m, _ = strconv.ParseFloat(fields[2], 64)
}

func (s *MonitorService) readMemory(p *MonitorPoint) {
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		log.Printf("monitor: failed to read /proc/meminfo: %v", err)
		return
	}

	mem := make(map[string]uint64)
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		valStr := strings.TrimSpace(parts[1])
		valStr = strings.TrimSuffix(valStr, " kB")
		valStr = strings.TrimSpace(valStr)
		val, _ := strconv.ParseUint(valStr, 10, 64)
		mem[key] = val * 1024
	}

	p.MemTotal = mem["MemTotal"]
	p.MemAvailable = mem["MemAvailable"]
	if p.MemTotal > 0 {
		used := mem["MemFree"] + mem["Buffers"] + mem["Cached"]
		p.MemUsed = deltaU64(p.MemTotal, used)
		p.MemPercent = math.Round(float64(p.MemUsed)/float64(p.MemTotal)*100*100) / 100
	}
}

func (s *MonitorService) readDisk(p *MonitorPoint) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs("/", &stat); err != nil {
		log.Printf("monitor: failed to statfs /: %v", err)
		return
	}

	p.DiskTotal = stat.Blocks * uint64(stat.Bsize)
	p.DiskFree = stat.Bfree * uint64(stat.Bsize)
	p.DiskUsed = p.DiskTotal - p.DiskFree
	if p.DiskTotal > 0 {
		p.DiskPercent = math.Round(float64(p.DiskUsed)/float64(p.DiskTotal)*100*100) / 100
	}
}

func (s *MonitorService) readNetwork(p *MonitorPoint) {
	data, err := os.ReadFile("/proc/net/dev")
	if err != nil {
		log.Printf("monitor: failed to read /proc/net/dev: %v", err)
		return
	}

	var totalSent, totalRecv, totalPktsSent, totalPktsRecv uint64
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.Contains(line, ":") {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		iface := strings.TrimSpace(parts[0])
		if strings.HasPrefix(iface, "lo") {
			continue
		}

		fields := strings.Fields(parts[1])
		if len(fields) < 10 {
			continue
		}

		recvBytes, _ := strconv.ParseUint(fields[0], 10, 64)
		recvPkts, _ := strconv.ParseUint(fields[1], 10, 64)
		sentBytes, _ := strconv.ParseUint(fields[8], 10, 64)
		sentPkts, _ := strconv.ParseUint(fields[9], 10, 64)

		totalSent += sentBytes
		totalRecv += recvBytes
		totalPktsSent += sentPkts
		totalPktsRecv += recvPkts
	}

	if s.prevSent > 0 {
		p.NetBytesSent = deltaU64(totalSent, s.prevSent)
		p.NetBytesRecv = deltaU64(totalRecv, s.prevRecv)
		p.NetPktsSent = deltaU64(totalPktsSent, s.prevPktsSent)
		p.NetPktsRecv = deltaU64(totalPktsRecv, s.prevPktsRecv)
	}

	s.prevSent = totalSent
	s.prevRecv = totalRecv
	s.prevPktsSent = totalPktsSent
	s.prevPktsRecv = totalPktsRecv
}

func deltaU64(cur, prev uint64) uint64 {
	if cur < prev {
		return 0
	}
	return cur - prev
}

func (s *MonitorService) readSystemInfo() *SystemInfo {
	info := &SystemInfo{
		Arch:     runtime.GOARCH,
		CPUCores: runtime.NumCPU(),
	}

	// Hostname
	if data, err := os.ReadFile("/proc/sys/kernel/hostname"); err == nil {
		info.Hostname = strings.TrimSpace(string(data))
	}

	// Kernel version
	if data, err := os.ReadFile("/proc/sys/kernel/osrelease"); err == nil {
		info.Kernel = strings.TrimSpace(string(data))
	}

	// OS name from /etc/os-release
	if data, err := os.ReadFile("/etc/os-release"); err == nil {
		scanner := bufio.NewScanner(strings.NewReader(string(data)))
		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, "PRETTY_NAME=") {
				val := strings.TrimPrefix(line, "PRETTY_NAME=")
				val = strings.Trim(val, "\"")
				info.OS = val
				break
			}
		}
	}
	if info.OS == "" {
		info.OS = "Linux"
	}

	// Uptime from /proc/uptime
	if data, err := os.ReadFile("/proc/uptime"); err == nil {
		fields := strings.Fields(string(data))
		if len(fields) >= 1 {
			if uptime, err := strconv.ParseFloat(fields[0], 64); err == nil {
				info.UptimeSeconds = int64(uptime)
			}
		}
	}

	return info
}

// readSwap reads swap memory info from /proc/meminfo
func (s *MonitorService) readSwap() *SwapInfo {
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return nil
	}

	mem := make(map[string]uint64)
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		valStr := strings.TrimSpace(parts[1])
		valStr = strings.TrimSuffix(valStr, " kB")
		valStr = strings.TrimSpace(valStr)
		val, _ := strconv.ParseUint(valStr, 10, 64)
		mem[key] = val * 1024
	}

	swap := &SwapInfo{
		TotalBytes: mem["SwapTotal"],
		FreeBytes:  mem["SwapFree"],
	}
	swap.UsedBytes = swap.TotalBytes - swap.FreeBytes
	if swap.TotalBytes > 0 {
		swap.UsagePercent = math.Round(float64(swap.UsedBytes)/float64(swap.TotalBytes)*100*100) / 100
	}

	return swap
}

// readPartitions reads all physical/logical disk partitions using sysfs and /proc/mounts
func (s *MonitorService) readPartitions() []DiskPartition {
	data, err := os.ReadFile("/proc/mounts")
	if err != nil {
		return nil
	}

	var partitions []DiskPartition
	seen := make(map[string]bool)

	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		line := scanner.Text()
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}

		device := fields[0]
		mountPoint := fields[1]
		fsType := fields[2]

		// 必须以 /dev/ 开头的设备
		if !strings.HasPrefix(device, "/dev/") {
			continue
		}

		devName := filepath.Base(device)
		sysBlockPath := filepath.Join("/sys/class/block", devName)

		// 优雅判断：利用 Linux sysfs 内核属性甄别真实块设备与 loop 卷
		// 1. 如果在 /sys/class/block 中不存在该块设备（如 overlay, procfs 等伪设备），跳过
		if _, err := os.Stat(sysBlockPath); err != nil {
			continue
		}
		// 2. 如果存在 loop 属性（如 /sys/class/block/loopX/loop），说明是 snap 或文件挂载镜像，跳过
		if _, err := os.Stat(filepath.Join(sysBlockPath, "loop")); err == nil {
			continue
		}

		// 过滤虚拟/临时文件系统类型
		if fsType == "squashfs" || fsType == "overlay" || fsType == "tmpfs" || fsType == "devtmpfs" {
			continue
		}

		// 避免同一个挂载点重复展示
		if seen[mountPoint] {
			continue
		}
		seen[mountPoint] = true

		var stat syscall.Statfs_t
		if err := syscall.Statfs(mountPoint, &stat); err != nil {
			continue
		}

		total := stat.Blocks * uint64(stat.Bsize)
		free := stat.Bfree * uint64(stat.Bsize)
		used := total - free
		var percent float64
		if total > 0 {
			percent = math.Round(float64(used)/float64(total)*100*100) / 100
		}

		partitions = append(partitions, DiskPartition{
			MountPoint:   mountPoint,
			Device:       device,
			FSType:       fsType,
			TotalBytes:   total,
			UsedBytes:    used,
			FreeBytes:    free,
			UsagePercent: percent,
		})
	}

	return partitions
}

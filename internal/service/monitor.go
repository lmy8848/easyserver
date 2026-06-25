package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"easyserver/internal/model"
	"easyserver/internal/repository"
)

const (
	processCollectInterval = 5 * time.Second  // 进程采集间隔
	cacheExpiry            = 60 * time.Second // 系统信息/Swap 缓存有效期
	maxHistoryPoints       = 360              // 历史数据最大点数
)

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
	monitorRepo  repository.MonitorRepository
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

	// 性能优化：进程采集降频
	processInterval    time.Duration
	lastProcessCollect time.Time
	cachedProcesses    []model.ProcessInfo
	processMu          sync.RWMutex

	// 性能优化：系统信息缓存
	cachedSystemInfo *model.SystemInfo
	sysInfoExpire    time.Time
	cachedSwap       *model.SwapInfo
	swapExpire       time.Time

	// 性能优化：uid→username 缓存
	uidCache map[string]string

	// 告警评估
	alertService *AlertService

	// 性能优化：环形缓冲 + 批量写入
	ringBuffer  []*model.MonitorPoint
	ringSize    int
	ringHead    int
	ringCount   int
	ringMu      sync.Mutex
	flushTicker *time.Ticker
}

func NewMonitorService(monitorRepo repository.MonitorRepository, interval, retention time.Duration) *MonitorService {
	return &MonitorService{
		monitorRepo:     monitorRepo,
		interval:        interval,
		retention:       retention,
		hub:             NewMonitorHub(),
		stopCh:          make(chan struct{}),
		processInterval: processCollectInterval,
		uidCache:        make(map[string]string),
		ringBuffer:      make([]*model.MonitorPoint, 60), // 60 points buffer
		ringSize:        60,
	}
}

func (s *MonitorService) Hub() *MonitorHub {
	return s.hub
}

// SetAlertService sets the alert evaluation service
func (s *MonitorService) SetAlertService(alertService *AlertService) {
	s.alertService = alertService
}

func (s *MonitorService) Start() {
	ctx := context.Background()

	// 性能优化：创建时间戳索引（异步执行，不阻塞启动）
	go func() {
		if err := s.monitorRepo.EnsureIndexes(ctx); err != nil {
			log.Printf("monitor: failed to create index: %v", err)
		}
	}()

	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

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

	// 性能优化：进程数据降频（5秒采集一次，其余时间用缓存）
	s.processMu.RLock()
	snapshot.TopProcess = s.cachedProcesses
	needRefresh := time.Since(s.lastProcessCollect) >= s.processInterval
	s.processMu.RUnlock()

	// 异步刷新进程数据（不阻塞主采集循环）
	if needRefresh {
		go s.refreshProcesses()
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

// refreshProcesses 异步刷新进程缓存
func (s *MonitorService) refreshProcesses() {
	processes := s.readTopProcesses()
	s.processMu.Lock()
	s.cachedProcesses = processes
	s.lastProcessCollect = time.Now()
	s.processMu.Unlock()
}

// readSystemInfoCached 带缓存的系统信息读取（cacheExpiry 刷新）
func (s *MonitorService) readSystemInfoCached() *model.SystemInfo {
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
func (s *MonitorService) readSwapCached() *model.SwapInfo {
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

func (s *MonitorService) readAll() *model.MonitorPoint {
	now := time.Now().UTC().Format(time.RFC3339)
	p := &model.MonitorPoint{Timestamp: now}

	s.readCPU(p)
	s.readLoad(p)
	s.readMemory(p)
	s.readDisk(p)
	s.readNetwork(p)

	return p
}

// Platform-specific functions are defined in monitor_linux.go.

// savePoint saves a single point using the repository
func (s *MonitorService) savePoint(p *model.MonitorPoint) error {
	return s.monitorRepo.Save(context.Background(), p)
}

// addToBuffer adds a point to the ring buffer
func (s *MonitorService) addToBuffer(p *model.MonitorPoint) {
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
	points := make([]*model.MonitorPoint, s.ringCount)
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
func (s *MonitorService) GetLatestPoint() *model.MonitorPoint {
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

func (s *MonitorService) GetCurrentStats(ctx context.Context) (*model.MonitorSnapshot, error) {
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
	s.processMu.RLock()
	snapshot.TopProcess = s.cachedProcesses
	s.processMu.RUnlock()
	return snapshot, nil
}

func (s *MonitorService) GetHistory(ctx context.Context, start, end time.Time) ([]model.MonitorPoint, error) {
	points, err := s.monitorRepo.GetHistory(ctx, start, end)
	if err != nil {
		return nil, err
	}

	// 性能优化：降采样到最多 maxHistoryPoints 个点
	if len(points) > maxHistoryPoints {
		step := len(points) / maxHistoryPoints
		sampled := make([]model.MonitorPoint, 0, maxHistoryPoints)
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

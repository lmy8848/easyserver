package service

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"easyserver/internal/model"
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
	delete(h.clients, c)
	close(c.Send)
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
	db           *sql.DB
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
	sysInfo      *model.SystemInfo
}

func NewMonitorService(db *sql.DB, interval, retention time.Duration) *MonitorService {
	return &MonitorService{
		db:        db,
		interval:  interval,
		retention: retention,
		hub:       NewMonitorHub(),
		stopCh:    make(chan struct{}),
	}
}

func (s *MonitorService) Hub() *MonitorHub {
	return s.hub
}

func (s *MonitorService) Start() {
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	// First collection
	s.collect()

	for {
		select {
		case <-ticker.C:
			s.collect()
		case <-s.stopCh:
			return
		}
	}
}

func (s *MonitorService) Stop() {
	close(s.stopCh)
}

func (s *MonitorService) collect() {
	point := s.readAll()

	if err := s.savePoint(point); err != nil {
		log.Printf("monitor: save error: %v", err)
	}

	// Cleanup old data every 10 minutes (not every second)
	if time.Now().Minute()%10 == 0 && time.Now().Second() < int(s.interval.Seconds()) {
		s.cleanup()
	}

	snapshot := point.ToSnapshot()

	// Add extra data (system, swap, partitions, top processes)
	snapshot.System = s.readSystemInfo()
	snapshot.Swap = s.readSwap()
	snapshot.Partitions = s.readPartitions()
	snapshot.TopProcess = s.readTopProcesses()

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

// Platform-specific functions defined in:
// - monitor_linux.go: readCPU, readLoad, readMemory, readDisk, readNetwork
// - monitor_windows.go: readCPU, readLoad, readMemory, readDisk, readNetwork

func (s *MonitorService) savePoint(p *model.MonitorPoint) error {
	_, err := s.db.Exec(
		`INSERT INTO monitor_data
		(cpu, cpu_load_1m, cpu_load_5m, cpu_load_15m,
		 mem_total, mem_used, mem_available, mem_usage,
		 disk_total, disk_used, disk_free, disk_usage,
		 net_bytes_sent, net_bytes_recv, net_packets_sent, net_packets_recv, timestamp)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		p.CPUPercent, p.CPULoad1m, p.CPULoad5m, p.CPULoad15m,
		p.MemTotal, p.MemUsed, p.MemAvailable, p.MemPercent,
		p.DiskTotal, p.DiskUsed, p.DiskFree, p.DiskPercent,
		p.NetBytesSent, p.NetBytesRecv, p.NetPktsSent, p.NetPktsRecv, p.Timestamp,
	)
	return err
}

func (s *MonitorService) cleanup() {
	since := time.Now().UTC().Add(-s.retention)
	result, err := s.db.Exec("DELETE FROM monitor_data WHERE timestamp < ?", since.Format(time.RFC3339))
	if err != nil {
		log.Printf("monitor: cleanup error: %v", err)
		return
	}
	rows, _ := result.RowsAffected()
	if rows > 0 {
		log.Printf("monitor: cleaned up %d old records", rows)
	}
}

func (s *MonitorService) GetCurrentStats() (*model.MonitorSnapshot, error) {
	p := &model.MonitorPoint{}
	err := s.db.QueryRow(
		`SELECT cpu, cpu_load_1m, cpu_load_5m, cpu_load_15m,
		        mem_total, mem_used, mem_available, mem_usage,
		        disk_total, disk_used, disk_free, disk_usage,
		        net_bytes_sent, net_bytes_recv, net_packets_sent, net_packets_recv, timestamp
		 FROM monitor_data ORDER BY id DESC LIMIT 1`,
	).Scan(
		&p.CPUPercent, &p.CPULoad1m, &p.CPULoad5m, &p.CPULoad15m,
		&p.MemTotal, &p.MemUsed, &p.MemAvailable, &p.MemPercent,
		&p.DiskTotal, &p.DiskUsed, &p.DiskFree, &p.DiskPercent,
		&p.NetBytesSent, &p.NetBytesRecv, &p.NetPktsSent, &p.NetPktsRecv, &p.Timestamp,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("no data yet")
		}
		return nil, err
	}
	snapshot := p.ToSnapshot()
	snapshot.System = s.readSystemInfo()
	snapshot.Swap = s.readSwap()
	snapshot.Partitions = s.readPartitions()
	snapshot.TopProcess = s.readTopProcesses()
	return snapshot, nil
}

func (s *MonitorService) GetHistory(start, end time.Time) ([]model.MonitorPoint, error) {
	rows, err := s.db.Query(
		`SELECT cpu, cpu_load_1m, cpu_load_5m, cpu_load_15m,
		        mem_total, mem_used, mem_available, mem_usage,
		        disk_total, disk_used, disk_free, disk_usage,
		        net_bytes_sent, net_bytes_recv, net_packets_sent, net_packets_recv, timestamp
		 FROM monitor_data
		 WHERE timestamp >= ? AND timestamp <= ?
		 ORDER BY timestamp ASC`,
		start.Format(time.RFC3339), end.Format(time.RFC3339),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var points []model.MonitorPoint
	for rows.Next() {
		var p model.MonitorPoint
		if err := rows.Scan(
			&p.CPUPercent, &p.CPULoad1m, &p.CPULoad5m, &p.CPULoad15m,
			&p.MemTotal, &p.MemUsed, &p.MemAvailable, &p.MemPercent,
			&p.DiskTotal, &p.DiskUsed, &p.DiskFree, &p.DiskPercent,
			&p.NetBytesSent, &p.NetBytesRecv, &p.NetPktsSent, &p.NetPktsRecv, &p.Timestamp,
		); err != nil {
			return nil, err
		}
		points = append(points, p)
	}
	return points, nil
}

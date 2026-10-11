package main

import (
	"context"
	"fmt"
	"log"
	"sync"

	v1 "github.com/fatedier/frp/pkg/config/v1"
	"github.com/fatedier/frp/server"
)

// FRPSManager 管理嵌入的 frps 服务实例（文档：插件作为独立进程，
// frps 作为其内部组件随插件生命周期启停）。
type FRPSManager struct {
	mu      sync.Mutex
	service *server.Service
	running bool
	cfg     v1.ServerConfig
}

func NewFRPSManager() *FRPSManager { return &FRPSManager{} }

// Running 报告 frps 是否在运行。
func (m *FRPSManager) Running() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.running
}

// Start 以给定配置启动嵌入 frps（已在运行则先停止）。
func (m *FRPSManager) Start(ctx context.Context, cfg v1.ServerConfig) error {
	m.mu.Lock()
	if m.running {
		m.mu.Unlock()
		return fmt.Errorf("frps 已在运行")
	}
	m.cfg = cfg
	m.mu.Unlock()

	svc, err := server.NewService(&cfg)
	if err != nil {
		return fmt.Errorf("frps 初始化失败: %w", err)
	}
	m.mu.Lock()
	m.service = svc
	m.running = true
	m.mu.Unlock()

	go func() {
		defer m.markStopped()
		svc.Run(ctx)
		log.Printf("tunnel-frp: frps 退出")
	}()
	return nil
}

func (m *FRPSManager) markStopped() {
	m.mu.Lock()
	m.running = false
	m.service = nil
	m.mu.Unlock()
}

// Stop 停止 frps。
func (m *FRPSManager) Stop() {
	m.mu.Lock()
	svc := m.service
	m.running = false
	m.service = nil
	m.mu.Unlock()
	if svc != nil {
		_ = svc.Close()
	}
}

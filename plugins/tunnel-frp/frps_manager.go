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
//
// 生命周期由管理器自有 context 驱动，不继承调用方（如 HTTP 请求）的
// context——否则"保存并重启"这类经面板代理发起的请求一旦结束，服务会
// 随请求 ctx 被误杀。
type FRPSManager struct {
	mu      sync.Mutex
	service *server.Service
	cancel  context.CancelFunc
	cfg     v1.ServerConfig
	running bool
}

func NewFRPSManager() *FRPSManager { return &FRPSManager{} }

// Running 报告 frps 是否在运行。
func (m *FRPSManager) Running() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.running
}

// Start 启动 frps；已有实例则原子替换（停旧启新，改配置即重启）。
// 服务本体运行在管理器自有生命周期上，不随调用方 context 结束。
func (m *FRPSManager) Start(cfg v1.ServerConfig) error {
	svc, err := server.NewService(&cfg)
	if err != nil {
		return fmt.Errorf("frps 初始化失败: %w", err)
	}
	runCtx, cancel := context.WithCancel(context.Background())

	m.mu.Lock()
	oldSvc, oldCancel := m.service, m.cancel
	m.service = svc
	m.cancel = cancel
	m.cfg = cfg
	m.running = true
	m.mu.Unlock()

	// 替换：先停旧实例，释放端口后再由新实例接管
	if oldCancel != nil {
		oldCancel()
	}
	if oldSvc != nil {
		_ = oldSvc.Close()
	}

	go func() {
		svc.Run(runCtx)
		m.mu.Lock()
		current := m.service == svc // 仅最新实例的退出才更新状态
		if current {
			m.service = nil
			m.cancel = nil
			m.running = false
		}
		m.mu.Unlock()
		if current {
			log.Printf("tunnel-frp: frps 已停止")
		}
	}()
	return nil
}

// Stop 停止 frps。
func (m *FRPSManager) Stop() {
	m.mu.Lock()
	svc, cancel := m.service, m.cancel
	m.service = nil
	m.cancel = nil
	m.running = false
	m.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if svc != nil {
		_ = svc.Close()
	}
}

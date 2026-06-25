//go:build !linux
// +build !linux

package service

import (
	"context"
	"fmt"
	"sync"

	"easyserver/internal/executor"
)

type ServiceInfo struct {
	Name        string  `json:"name"`
	Description string  `json:"description"`
	State       string  `json:"state"`
	SubState    string  `json:"sub_state"`
	Enabled     bool    `json:"enabled"`
	PID         int     `json:"pid"`
	MemoryBytes uint64  `json:"memory_bytes"`
	CPUPercent  float64 `json:"cpu_percent"`
	UptimeSeconds int64 `json:"uptime_seconds"`
}

type LogLine struct {
	Time     string `json:"time"`
	Message  string `json:"message"`
	Priority string `json:"priority"`
}

type ServiceManager struct {
	mu       sync.RWMutex
	executor executor.CommandExecutor
}

func NewServiceManager(exec executor.CommandExecutor) *ServiceManager {
	return &ServiceManager{executor: exec}
}

func (m *ServiceManager) List(ctx context.Context) ([]ServiceInfo, error) {
	return nil, fmt.Errorf("service management is only available on Linux")
}

func (m *ServiceManager) Get(ctx context.Context, name string) (*ServiceInfo, error) {
	return nil, fmt.Errorf("service management is only available on Linux")
}

func (m *ServiceManager) Start(ctx context.Context, name string) error {
	return fmt.Errorf("service management is only available on Linux")
}

func (m *ServiceManager) Stop(ctx context.Context, name string) error {
	return fmt.Errorf("service management is only available on Linux")
}

func (m *ServiceManager) Restart(ctx context.Context, name string) error {
	return fmt.Errorf("service management is only available on Linux")
}

func (m *ServiceManager) Enable(ctx context.Context, name string) error {
	return fmt.Errorf("service management is only available on Linux")
}

func (m *ServiceManager) Disable(ctx context.Context, name string) error {
	return fmt.Errorf("service management is only available on Linux")
}

func (m *ServiceManager) GetLogs(ctx context.Context, name string, tail int, since string) ([]LogLine, error) {
	return nil, fmt.Errorf("service management is only available on Linux")
}

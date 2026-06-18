//go:build !linux
// +build !linux

package service

import (
	"fmt"
	"sync"
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
	mu sync.RWMutex
}

func NewServiceManager() *ServiceManager {
	return &ServiceManager{}
}

func (m *ServiceManager) List() ([]ServiceInfo, error) {
	return nil, fmt.Errorf("service management is only available on Linux")
}

func (m *ServiceManager) Get(name string) (*ServiceInfo, error) {
	return nil, fmt.Errorf("service management is only available on Linux")
}

func (m *ServiceManager) Start(name string) error {
	return fmt.Errorf("service management is only available on Linux")
}

func (m *ServiceManager) Stop(name string) error {
	return fmt.Errorf("service management is only available on Linux")
}

func (m *ServiceManager) Restart(name string) error {
	return fmt.Errorf("service management is only available on Linux")
}

func (m *ServiceManager) Enable(name string) error {
	return fmt.Errorf("service management is only available on Linux")
}

func (m *ServiceManager) Disable(name string) error {
	return fmt.Errorf("service management is only available on Linux")
}

func (m *ServiceManager) GetLogs(name string, tail int, since string) ([]LogLine, error) {
	return nil, fmt.Errorf("service management is only available on Linux")
}

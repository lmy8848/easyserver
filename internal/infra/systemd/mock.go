package systemd

import (
	"context"
	"slices"
	"sync"

	"github.com/coreos/go-systemd/v22/dbus"
	godbus "github.com/godbus/dbus/v5"
)

// MockSystemdClient is an in-memory mock implementation of SystemdClient for tests.
type MockSystemdClient struct {
	mu sync.Mutex

	Available  bool
	Units      []dbus.UnitStatus
	Properties map[string]map[string]any
	CallLog    []string

	StartUnitFunc   func(ctx context.Context, name string, mode string) (string, error)
	StopUnitFunc    func(ctx context.Context, name string, mode string) (string, error)
	RestartUnitFunc func(ctx context.Context, name string, mode string) (string, error)
	ReloadUnitFunc  func(ctx context.Context, name string, mode string) (string, error)
	ReloadFunc      func(ctx context.Context) error
	EnableFunc      func(ctx context.Context, files []string, runtime bool, force bool) (bool, []dbus.EnableUnitFileChange, error)
	DisableFunc     func(ctx context.Context, files []string, runtime bool) ([]dbus.DisableUnitFileChange, error)
	ResetFailedFunc func(ctx context.Context, name string) error
	MaskFunc        func(ctx context.Context, files []string, runtime bool, force bool) ([]dbus.MaskUnitFileChange, error)
	UnmaskFunc      func(ctx context.Context, files []string, runtime bool) ([]dbus.UnmaskUnitFileChange, error)
}

// NewMockSystemdClient returns a new MockSystemdClient with default healthy state.
func NewMockSystemdClient() *MockSystemdClient {
	return &MockSystemdClient{
		Available:  true,
		Units:      make([]dbus.UnitStatus, 0),
		Properties: make(map[string]map[string]any),
		CallLog:    make([]string, 0),
	}
}

func (m *MockSystemdClient) record(call string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.CallLog = append(m.CallLog, call)
}

func (m *MockSystemdClient) GetCallLog() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	res := make([]string, len(m.CallLog))
	copy(res, m.CallLog)
	return res
}

func (m *MockSystemdClient) IsAvailable() bool {
	return m.Available
}

func (m *MockSystemdClient) ListUnitsContext(_ context.Context) ([]dbus.UnitStatus, error) {
	m.record("ListUnits")
	if !m.Available {
		return nil, ErrSystemdUnavailable
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.Units, nil
}

func (m *MockSystemdClient) ListUnitsByPatternsContext(_ context.Context, _ []string, patterns []string) ([]dbus.UnitStatus, error) {
	m.record("ListUnitsByPatterns")
	if !m.Available {
		return nil, ErrSystemdUnavailable
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(patterns) == 0 {
		return m.Units, nil
	}
	var res []dbus.UnitStatus
	for _, u := range m.Units {
		if slices.Contains(patterns, u.Name) {
			res = append(res, u)
		}
	}
	return res, nil
}

func (m *MockSystemdClient) GetUnitPropertiesContext(_ context.Context, unit string) (map[string]any, error) {
	m.record("GetUnitProperties:" + unit)
	if !m.Available {
		return nil, ErrSystemdUnavailable
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if props, ok := m.Properties[unit]; ok {
		return props, nil
	}
	return map[string]any{
		"ActiveState":   "active",
		"SubState":      "running",
		"LoadState":     "loaded",
		"UnitFileState": "enabled",
	}, nil
}

func (m *MockSystemdClient) GetUnitPropertyContext(_ context.Context, unit string, propertyName string) (*dbus.Property, error) {
	m.record("GetUnitProperty:" + unit + ":" + propertyName)
	if !m.Available {
		return nil, ErrSystemdUnavailable
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if props, ok := m.Properties[unit]; ok {
		if val, exists := props[propertyName]; exists {
			return &dbus.Property{Name: propertyName, Value: godbus.MakeVariant(val)}, nil
		}
	}
	return &dbus.Property{Name: propertyName, Value: godbus.MakeVariant("")}, nil
}

func (m *MockSystemdClient) GetUnitTypePropertiesContext(_ context.Context, unit string, dbusInterface string) (map[string]any, error) {
	m.record("GetUnitTypeProperties:" + unit + ":" + dbusInterface)
	if !m.Available {
		return nil, ErrSystemdUnavailable
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if props, ok := m.Properties[unit]; ok {
		return props, nil
	}
	return map[string]any{
		"ActiveState":   "active",
		"SubState":      "running",
		"LoadState":     "loaded",
		"UnitFileState": "enabled",
	}, nil
}

func (m *MockSystemdClient) StartUnitContext(ctx context.Context, name string, mode string) (string, error) {
	m.record("StartUnit:" + name)
	if !m.Available {
		return "", ErrSystemdUnavailable
	}
	if m.StartUnitFunc != nil {
		return m.StartUnitFunc(ctx, name, mode)
	}
	return "done", nil
}

func (m *MockSystemdClient) StopUnitContext(ctx context.Context, name string, mode string) (string, error) {
	m.record("StopUnit:" + name)
	if !m.Available {
		return "", ErrSystemdUnavailable
	}
	if m.StopUnitFunc != nil {
		return m.StopUnitFunc(ctx, name, mode)
	}
	return "done", nil
}

func (m *MockSystemdClient) RestartUnitContext(ctx context.Context, name string, mode string) (string, error) {
	m.record("RestartUnit:" + name)
	if !m.Available {
		return "", ErrSystemdUnavailable
	}
	if m.RestartUnitFunc != nil {
		return m.RestartUnitFunc(ctx, name, mode)
	}
	return "done", nil
}

func (m *MockSystemdClient) ReloadUnitContext(ctx context.Context, name string, mode string) (string, error) {
	m.record("ReloadUnit:" + name)
	if !m.Available {
		return "", ErrSystemdUnavailable
	}
	if m.ReloadUnitFunc != nil {
		return m.ReloadUnitFunc(ctx, name, mode)
	}
	return "done", nil
}

func (m *MockSystemdClient) ReloadContext(ctx context.Context) error {
	m.record("Reload")
	if !m.Available {
		return ErrSystemdUnavailable
	}
	if m.ReloadFunc != nil {
		return m.ReloadFunc(ctx)
	}
	return nil
}

func (m *MockSystemdClient) EnableUnitFilesContext(ctx context.Context, files []string, runtime bool, force bool) (bool, []dbus.EnableUnitFileChange, error) {
	m.record("EnableUnitFiles")
	if !m.Available {
		return false, nil, ErrSystemdUnavailable
	}
	if m.EnableFunc != nil {
		return m.EnableFunc(ctx, files, runtime, force)
	}
	return true, nil, nil
}

func (m *MockSystemdClient) DisableUnitFilesContext(ctx context.Context, files []string, runtime bool) ([]dbus.DisableUnitFileChange, error) {
	m.record("DisableUnitFiles")
	if !m.Available {
		return nil, ErrSystemdUnavailable
	}
	if m.DisableFunc != nil {
		return m.DisableFunc(ctx, files, runtime)
	}
	return nil, nil
}

func (m *MockSystemdClient) ResetFailedUnitContext(ctx context.Context, name string) error {
	m.record("ResetFailedUnit:" + name)
	if !m.Available {
		return ErrSystemdUnavailable
	}
	if m.ResetFailedFunc != nil {
		return m.ResetFailedFunc(ctx, name)
	}
	return nil
}

func (m *MockSystemdClient) MaskUnitFilesContext(ctx context.Context, files []string, runtime bool, force bool) ([]dbus.MaskUnitFileChange, error) {
	m.record("MaskUnitFiles")
	if !m.Available {
		return nil, ErrSystemdUnavailable
	}
	if m.MaskFunc != nil {
		return m.MaskFunc(ctx, files, runtime, force)
	}
	return nil, nil
}

func (m *MockSystemdClient) UnmaskUnitFilesContext(ctx context.Context, files []string, runtime bool) ([]dbus.UnmaskUnitFileChange, error) {
	m.record("UnmaskUnitFiles")
	if !m.Available {
		return nil, ErrSystemdUnavailable
	}
	if m.UnmaskFunc != nil {
		return m.UnmaskFunc(ctx, files, runtime)
	}
	return nil, nil
}

func (m *MockSystemdClient) Close() {}

package systemd_test

import (
	"context"
	"errors"
	"testing"

	"github.com/coreos/go-systemd/v22/dbus"

	infrasystemd "easyserver/internal/infra/systemd"
)

func TestMockSystemdClient_BasicOperations(t *testing.T) {
	mock := infrasystemd.NewMockSystemdClient()
	mock.Units = []dbus.UnitStatus{
		{Name: "easyserver-app.service", ActiveState: "active", SubState: "running"},
		{Name: "nginx.service", ActiveState: "active", SubState: "running"},
	}
	mock.Properties["easyserver-app.service"] = map[string]any{
		"ActiveState":   "active",
		"SubState":      "running",
		"ExecMainPID":   uint32(1234),
		"UnitFileState": "enabled",
	}

	ctx := context.Background()

	// ListUnits
	units, err := mock.ListUnitsContext(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(units) != 2 {
		t.Errorf("expected 2 units, got %d", len(units))
	}

	// ListUnitsByPatterns
	filtered, err := mock.ListUnitsByPatternsContext(ctx, nil, []string{"nginx.service"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(filtered) != 1 || filtered[0].Name != "nginx.service" {
		t.Errorf("expected 1 unit matching nginx.service, got %v", filtered)
	}

	// Properties
	props, err := mock.GetUnitPropertiesContext(ctx, "easyserver-app.service")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pid, ok := props["ExecMainPID"].(uint32); !ok || pid != 1234 {
		t.Errorf("expected ExecMainPID 1234, got %v", props["ExecMainPID"])
	}

	// Start / Stop / Restart / Reload
	res, err := mock.StartUnitContext(ctx, "easyserver-app.service", "replace")
	if err != nil || res != "done" {
		t.Errorf("expected start done, got %v, err: %v", res, err)
	}

	res, err = mock.StopUnitContext(ctx, "easyserver-app.service", "replace")
	if err != nil || res != "done" {
		t.Errorf("expected stop done, got %v, err: %v", res, err)
	}

	res, err = mock.RestartUnitContext(ctx, "easyserver-app.service", "replace")
	if err != nil || res != "done" {
		t.Errorf("expected restart done, got %v, err: %v", res, err)
	}

	if err := mock.ReloadContext(ctx); err != nil {
		t.Errorf("expected reload success, got err: %v", err)
	}

	// Check Call Log
	logs := mock.GetCallLog()
	if len(logs) == 0 {
		t.Errorf("expected recorded calls in mock, got 0")
	}
}

func TestMockSystemdClient_Unavailable(t *testing.T) {
	mock := infrasystemd.NewMockSystemdClient()
	mock.Available = false

	ctx := context.Background()

	if mock.IsAvailable() {
		t.Errorf("expected mock IsAvailable() == false")
	}

	_, err := mock.ListUnitsContext(ctx)
	if !errors.Is(err, infrasystemd.ErrSystemdUnavailable) {
		t.Errorf("expected ErrSystemdUnavailable, got %v", err)
	}

	_, err = mock.StartUnitContext(ctx, "test.service", "replace")
	if !errors.Is(err, infrasystemd.ErrSystemdUnavailable) {
		t.Errorf("expected ErrSystemdUnavailable on StartUnit, got %v", err)
	}

	err = mock.ReloadContext(ctx)
	if !errors.Is(err, infrasystemd.ErrSystemdUnavailable) {
		t.Errorf("expected ErrSystemdUnavailable on Reload, got %v", err)
	}
}

func TestDefaultClient_SetAndGet(t *testing.T) {
	mock := infrasystemd.NewMockSystemdClient()
	infrasystemd.SetDefaultClient(mock)

	client := infrasystemd.DefaultClient()
	if client != mock {
		t.Errorf("expected DefaultClient() to return injected mock")
	}
}

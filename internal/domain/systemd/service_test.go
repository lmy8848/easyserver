package systemd_test

import (
	"context"
	"testing"

	"github.com/coreos/go-systemd/v22/dbus"

	"easyserver/internal/domain/systemd"
	"easyserver/internal/infra/mise"
	infrasystemd "easyserver/internal/infra/systemd"
)

func TestServiceManager_ListWithDBus(t *testing.T) {
	mock := infrasystemd.NewMockSystemdClient()
	mock.Units = []dbus.UnitStatus{
		{
			Name:        "easyserver-svc-web.service",
			ActiveState: "active",
			SubState:    "running",
			Description: "Web Application",
			LoadState:   "loaded",
		},
		{
			Name:        "nginx.service",
			ActiveState: "active",
			SubState:    "running",
			Description: "Nginx Server",
			LoadState:   "loaded",
		},
		{
			Name:        "missing.service",
			ActiveState: "inactive",
			SubState:    "dead",
			Description: "Missing",
			LoadState:   "not-found",
		},
	}

	mgr := systemd.NewServiceManager(nil, mise.NewProvider())
	mgr.SetClient(mock)

	ctx := context.Background()
	services, err := mgr.List(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(services) != 2 {
		t.Fatalf("expected 2 services (excluding not-found), got %d", len(services))
	}

	// Verify first service
	if services[0].Name != "easyserver-svc-web" || services[0].ShortName != "web" {
		t.Errorf("expected easyserver-svc-web with shortName web, got %+v", services[0])
	}
}

func TestServiceManager_GetDetailsWithDBus(t *testing.T) {
	mock := infrasystemd.NewMockSystemdClient()
	mock.Properties["easyserver-svc-app.service"] = map[string]any{
		"ActiveState":   "active",
		"SubState":      "running",
		"MainPID":       uint32(4567),
		"MemoryCurrent": uint64(10485760),
		"UnitFileState": "enabled",
		"Description":   "Custom App",
	}

	mgr := systemd.NewServiceManager(nil, mise.NewProvider())
	mgr.SetClient(mock)

	ctx := context.Background()
	details, err := mgr.GetDetails(ctx, []string{"easyserver-svc-app"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(details) != 1 {
		t.Fatalf("expected 1 detail, got %d", len(details))
	}

	svc := details[0]
	if svc.PID != 4567 {
		t.Errorf("expected PID 4567, got %d", svc.PID)
	}
	if svc.MemoryBytes != 10485760 {
		t.Errorf("expected MemoryBytes 10485760, got %d", svc.MemoryBytes)
	}
	if !svc.Enabled {
		t.Errorf("expected Enabled true")
	}
	if svc.State != "active" {
		t.Errorf("expected State active, got %s", svc.State)
	}
}

func TestServiceManager_OperationsWithDBus(t *testing.T) {
	mock := infrasystemd.NewMockSystemdClient()
	mock.Properties["nginx.service"] = map[string]any{
		"ActiveState":   "inactive",
		"SubState":      "dead",
		"LoadState":     "loaded",
		"UnitFileState": "disabled",
	}

	mgr := systemd.NewServiceManager(nil, mise.NewProvider())
	mgr.SetClient(mock)

	ctx := context.Background()

	// Start
	if err := mgr.Start(ctx, "nginx"); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	// Update state for Stop
	mock.Properties["nginx.service"]["ActiveState"] = "active"
	mock.Properties["nginx.service"]["SubState"] = "running"

	// Stop
	if err := mgr.Stop(ctx, "nginx"); err != nil {
		t.Fatalf("stop failed: %v", err)
	}

	// Restart
	if err := mgr.Restart(ctx, "nginx"); err != nil {
		t.Fatalf("restart failed: %v", err)
	}

	// Enable
	if err := mgr.Enable(ctx, "nginx"); err != nil {
		t.Fatalf("enable failed: %v", err)
	}

	// Disable
	mock.Properties["nginx.service"]["UnitFileState"] = "enabled"
	if err := mgr.Disable(ctx, "nginx"); err != nil {
		t.Fatalf("disable failed: %v", err)
	}
}

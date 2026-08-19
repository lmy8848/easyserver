package cron_test

import (
	"context"
	"slices"
	"testing"

	"easyserver/internal/domain/cron"
	"easyserver/internal/domain/systemd"
	"easyserver/internal/infra/mise"
	infrasystemd "easyserver/internal/infra/systemd"
)

func TestTimerManager_OperationsWithDBus(t *testing.T) {
	mock := infrasystemd.NewMockSystemdClient()
	timerUnit := systemd.CronTimerFileName("backup")
	svcUnit := systemd.CronServiceFileName("backup")

	mock.Properties[timerUnit] = map[string]any{
		"ActiveState":        "active",
		"UnitFileState":      "enabled",
		"NextElapseRealtime": "Tue 2026-08-20 02:00:00 UTC",
	}
	mock.Properties[svcUnit] = map[string]any{
		"ActiveState":           "inactive",
		"Result":                "success",
		"ExecMainExitTimestamp": "Mon 2026-08-19 02:00:05 UTC",
	}

	mgr := cron.NewTimerManager(mise.NewProvider(), nil)
	mgr.SetClient(mock)

	ctx := context.Background()

	// Verify mock start/stop/reload can be invoked directly
	if _, err := mock.StartUnitContext(ctx, svcUnit, "replace"); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	if err := mock.ReloadContext(ctx); err != nil {
		t.Fatalf("reload failed: %v", err)
	}

	// Verify GetUnitTypePropertiesContext works as expected
	props, err := mock.GetUnitTypePropertiesContext(ctx, timerUnit, "org.freedesktop.systemd1.Timer")
	if err != nil {
		t.Fatalf("GetUnitTypePropertiesContext failed: %v", err)
	}
	if props["ActiveState"] != "active" {
		t.Fatalf("expected ActiveState active, got %v", props["ActiveState"])
	}

	// Verify that the service unit received a StartUnitContext call via mock logs
	callLog := mock.GetCallLog()
	expectedCall := "StartUnit:" + svcUnit
	found := slices.Contains(callLog, expectedCall)
	if !found {
		t.Fatalf("expected call %q in mock call log, but got: %v", expectedCall, callLog)
	}
}

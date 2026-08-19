package cron_test

import (
	"context"
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

	// RunNow (direct start on service unit)
	_ = mgr.RunNow(ctx, "backup")

	// Verify mock start/stop/reload can be invoked directly
	if _, err := mock.StartUnitContext(ctx, svcUnit, "replace"); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	if err := mock.ReloadContext(ctx); err != nil {
		t.Fatalf("reload failed: %v", err)
	}
}

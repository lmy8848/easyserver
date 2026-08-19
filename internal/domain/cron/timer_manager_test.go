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

	// RunNow should successfully start the service unit
	if err := mgr.RunNow(ctx, "backup"); err != nil {
		t.Fatalf("RunNow failed: %v", err)
	}

	// Verify that the service unit received a StartUnitContext call via mock logs
	callLog := mock.GetCallLog()
	expectedCall := "StartUnit:" + svcUnit
	found := false
	for _, call := range callLog {
		if call == expectedCall {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected call %q in mock call log, but got: %v", expectedCall, callLog)
	}
}

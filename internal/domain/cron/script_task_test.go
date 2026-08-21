package cron

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"easyserver/internal/infra/task"
)

// TestScriptTaskSurvivesCallerLifecycle 回归测试：脚本进程通过 task.Manager 执行，脱离调用方请求生命周期。
func TestScriptTaskSurvivesCallerLifecycle(t *testing.T) {
	svc := &Service{
		taskMgr:  task.NewManager(4),
		lastSeen: make(map[string]string),
	}

	scriptPath := writeTestScript(t, "#!/bin/bash\necho 'line1'\nsleep 3\necho 'line2'\n")
	script := &Script{ID: 101, Name: "survivor", Path: scriptPath}

	// 1) 启动脚本执行（即使传入一个随后被 cancel 的 ctx，任务也应脱离生命周期继续运行）
	callerCtx, cancel := context.WithCancel(context.Background())
	if err := svc.RunScript(callerCtx, script); err != nil {
		t.Fatalf("RunScript failed: %v", err)
	}
	cancel() // 取消调用方 ctx

	// 2) 验证 running 状态与 GetRunningScriptIDs
	time.Sleep(100 * time.Millisecond)
	running := svc.GetRunningScriptIDs()
	if len(running) != 1 || running[0] != 101 {
		t.Fatalf("expected running script [101], got %v", running)
	}

	tk, ok := svc.ScriptTask(101)
	if !ok || tk == nil {
		t.Fatal("ScriptTask(101) returned nil or false")
	}

	// 3) 验证日志捕获
	log := tk.Log()
	if log == nil {
		t.Fatal("task log is nil")
	}
	lines, _ := log.Tail(0)
	if len(lines) == 0 || lines[0] != "line1" {
		t.Errorf("expected line1 in log, got %v", lines)
	}

	// 4) 停止脚本 (StopScript)
	if !svc.StopScript(101) {
		t.Fatal("StopScript returned false for running script")
	}

	select {
	case <-tk.Done():
	case <-time.After(3 * time.Second):
		t.Fatal("task did not finish after StopScript")
	}

	if tk.Status() != task.StatusCanceled {
		t.Errorf("expected status %v, got %v", task.StatusCanceled, tk.Status())
	}
}

func writeTestScript(t *testing.T, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "test_script.sh")
	if err := os.WriteFile(p, []byte(content), 0755); err != nil {
		t.Fatalf("write test script: %v", err)
	}
	return p
}

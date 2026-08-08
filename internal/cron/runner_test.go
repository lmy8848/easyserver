package cron

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"easyserver/internal/infra/executor"
)

// TestScriptRunnerSurvivesCallerLifecycle 回归测试：脚本进程必须脱离调用方（WS）生命周期。
//
// 修复前 ScriptRunner.Start 用调用方 ctx 建 exec.CommandContext，而 WS 底层是 HTTP 请求，
// 前端刷新断开时 request ctx 被 cancel，exec.CommandContext 会 SIGKILL 进程 —— 导致
// 「刷新页面后运行中的脚本被误杀、列表不显示运行中」。
//
// 前半段先证明根因机制，后半段验证修复后进程与调用方无关地存活。
func TestScriptRunnerSurvivesCallerLifecycle(t *testing.T) {
	// 1) 根因机制：exec.CommandContext 绑定可取消 ctx，cancel 即杀进程。
	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, "sleep", "30")
	if err := cmd.Start(); err != nil {
		t.Fatalf("start CommandContext: %v", err)
	}
	cancel()
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case <-done:
		// 进程被 ctx cancel 杀掉（Wait 返回错误），符合原 bug 机制。
	case <-time.After(3 * time.Second):
		t.Fatal("ctx cancel 后 exec.CommandContext 进程未退出，原 bug 机制不成立")
	}

	// 2) 修复后：ScriptRunner.Start 不绑定调用方 ctx，进程应存活。
	runner := NewScriptRunner(executor.NewOSExecutor())
	script := &Script{ID: 1, Name: "survivor", Path: writeSurvivorScript(t)}
	rs, err := runner.Start(script)
	if err != nil {
		t.Fatalf("runner.Start: %v", err)
	}
	if ids := runner.RunningIDs(); len(ids) != 1 || ids[0] != 1 {
		t.Fatalf("启动后期望 RunningIDs=[1]，got %v", ids)
	}

	// 模拟「调用方（WS）断开」：进程与调用方无关，仍应存活。
	time.Sleep(1200 * time.Millisecond)
	if ids := runner.RunningIDs(); len(ids) != 1 || ids[0] != 1 {
		t.Fatalf("进程应存活（脱离调用方生命周期），got %v", ids)
	}

	// 收尾：Stop 杀进程，done 关闭。
	runner.Stop(1)
	select {
	case <-rs.Done():
	case <-time.After(3 * time.Second):
		t.Fatal("Stop 后进程未退出")
	}
}

// writeSurvivorScript 写一个 sleep 数秒的脚本，返回其路径。
func writeSurvivorScript(t *testing.T) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "survivor.sh")
	if err := os.WriteFile(p, []byte("#!/bin/bash\nsleep 5\necho done\n"), 0755); err != nil {
		t.Fatalf("write script: %v", err)
	}
	return p
}

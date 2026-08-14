package task

// Manager 的生命周期测试。测试全部经公开 API 观察行为：Done 关闭时机、Err、
// Status 迁移、Cancel 返回值、Active 过滤、Get 存在性、去重/并发拒绝。任务体用
// 闭包脚本化（预置错误/阻塞在 ctx.Done 上），不依赖 executor 或真实进程。

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// blockOnCancel 阻塞直到 ctx 被取消（模拟一个跑不完的任务）。
func blockOnCancel(ctx context.Context) error {
	<-ctx.Done()
	return ctx.Err()
}

// waitDone 等待任务完成，带超时防挂死。
func waitDone(t *testing.T, task *Task) {
	t.Helper()
	select {
	case <-task.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("task did not finish within 5s")
	}
}

func TestStartRunsAndSucceeds(t *testing.T) {
	m := NewManager(8)
	ran := false
	tk, err := m.Start(context.Background(), "k1", Options{}, func(ctx context.Context) error {
		ran = true
		return nil
	})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	waitDone(t, tk)
	if tk.Status() != StatusSucceeded {
		t.Fatalf("status = %v, want %v", tk.Status(), StatusSucceeded)
	}
	if tk.Err() != nil {
		t.Fatalf("err = %v, want nil", tk.Err())
	}
	if !ran {
		t.Fatal("fn was not called")
	}
	// 成功即清：任务从 manager 移除。
	if _, ok := m.Get("k1"); ok {
		t.Fatal("succeeded task must be removed from manager")
	}
}

func TestStartReturnsSyncErrorOnDuplicateKey(t *testing.T) {
	m := NewManager(8)
	if _, err := m.Start(context.Background(), "k1", Options{}, blockOnCancel); err != nil {
		t.Fatalf("first start: %v", err)
	}
	// 同 key 第二个任务立即拒绝（同步错误），且可被 errors.Is 识别。
	_, err := m.Start(context.Background(), "k1", Options{}, blockOnCancel)
	if err == nil {
		t.Fatal("expected duplicate key error")
	}
	if !errors.Is(err, ErrKeyBusy) {
		t.Fatalf("duplicate error must wrap ErrKeyBusy, got %v", err)
	}
	// 清理，避免挂起的 goroutine。
	m.Cancel("k1")
}

func TestConcurrencyLimitRejects(t *testing.T) {
	m := NewManager(2)
	release := make(chan struct{})
	_, err := m.Start(context.Background(), "k1", Options{}, func(ctx context.Context) error {
		<-release
		return nil
	})
	if err != nil {
		t.Fatalf("start 1: %v", err)
	}
	if _, err := m.Start(context.Background(), "k2", Options{}, blockOnCancel); err != nil {
		t.Fatalf("start 2: %v", err)
	}
	// 已占用 2 个并发槽位，第三个拒绝（即使 key 不同）。
	if _, err := m.Start(context.Background(), "k3", Options{}, blockOnCancel); err == nil {
		t.Fatal("expected concurrency limit rejection")
	}
	close(release)
	waitDone(t, mustGet(t, m, "k1"))
	m.Cancel("k2")
}

func TestCancelPriorityStopsRetry(t *testing.T) {
	m := NewManager(8)
	attempts := int32(0)
	tk, err := m.Start(context.Background(), "k1", Options{MaxRetries: 3, RetryInterval: 10 * time.Millisecond},
		func(ctx context.Context) error {
			atomic.AddInt32(&attempts, 1)
			<-ctx.Done() // 每次尝试都挂住等取消
			return ctx.Err()
		})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	time.Sleep(20 * time.Millisecond) // 确保第一次尝试已开始
	if !m.Cancel("k1") {
		t.Fatal("cancel returned false for running task")
	}
	waitDone(t, tk)
	if tk.Status() != StatusCanceled {
		t.Fatalf("status = %v, want %v", tk.Status(), StatusCanceled)
	}
	// 用户取消优先：即使配置了重试，也绝不进入下一次尝试。
	if got := atomic.LoadInt32(&attempts); got != 1 {
		t.Fatalf("attempts = %d, want 1 (cancel must not retry)", got)
	}
}

func TestTimeoutFailsAndRetries(t *testing.T) {
	m := NewManager(8)
	attempts := int32(0)
	tk, err := m.Start(context.Background(), "k1",
		Options{Timeout: 30 * time.Millisecond, MaxRetries: 2, RetryInterval: 5 * time.Millisecond},
		func(ctx context.Context) error {
			atomic.AddInt32(&attempts, 1)
			<-ctx.Done()
			return ctx.Err()
		})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	waitDone(t, tk)
	// 超时归入失败并触发重试：最多 MaxRetries+1 次尝试。
	if got := atomic.LoadInt32(&attempts); got != 3 {
		t.Fatalf("attempts = %d, want 3 (initial + 2 retries)", got)
	}
	if tk.Status() != StatusFailed {
		t.Fatalf("status = %v, want %v (timeout collapses into failed)", tk.Status(), StatusFailed)
	}
	if tk.Err() == nil {
		t.Fatal("expected final error after retries exhausted")
	}
}

func TestFailedTaskRetainedUntilSameKeyRestart(t *testing.T) {
	m := NewManager(8)
	failErr := errors.New("boom")
	_, err := m.Start(context.Background(), "k1", Options{MaxRetries: 0}, func(ctx context.Context) error {
		return failErr
	})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	tk, ok := m.Get("k1")
	if !ok {
		t.Fatal("failed task must stay in manager")
	}
	waitDone(t, tk)
	if tk.Status() != StatusFailed {
		t.Fatalf("status = %v, want %v", tk.Status(), StatusFailed)
	}
	if !errors.Is(tk.Err(), failErr) {
		t.Fatalf("err = %v, want %v", tk.Err(), failErr)
	}
	// 同 key 再次 Start 覆盖旧记录：旧失败任务被新任务取代，而不是共存。
	newTk, err := m.Start(context.Background(), "k1", Options{}, func(ctx context.Context) error { return nil })
	if err != nil {
		t.Fatalf("restart same key: %v", err)
	}
	if newTk == tk {
		t.Fatal("new run must be a distinct task, not the old failed one")
	}
	// 新任务取代旧记录后，Get 返回的是新任务（running 中）。
	if got, ok := m.Get("k1"); !ok || got != newTk {
		t.Fatalf("Get after restart = %+v, ok=%v; want the new task", got, ok)
	}
	waitDone(t, newTk)
}

func TestStartWithoutLogIsOptionless(t *testing.T) {
	// 无日志任务：Start（非 StartWithLog）不产生日志、不 panic。
	m := NewManager(8)
	tk, err := m.Start(context.Background(), "k1", Options{}, func(ctx context.Context) error { return nil })
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	waitDone(t, tk)
	if tk.Log() != nil {
		t.Fatal("plain Start must not attach a log")
	}
}

func TestTaskPanicRecoversAsFailed(t *testing.T) {
	// 任务体 panic 不拖垮执行器：归为 failed、done 关闭、同 key 可重提覆盖。
	m := NewManager(8)
	tk, err := m.Start(context.Background(), "k1", Options{}, func(ctx context.Context) error {
		panic("boom")
	})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	waitDone(t, tk)
	if tk.Status() != StatusFailed {
		t.Fatalf("status = %v, want %v", tk.Status(), StatusFailed)
	}
	if tk.Err() == nil || !strings.Contains(tk.Err().Error(), "panicked") {
		t.Fatalf("err = %v, want a panicked error", tk.Err())
	}
	// 同 key 重提不受 panic 残留影响：failed 记录可被覆盖。
	newTk, err := m.Start(context.Background(), "k1", Options{}, func(ctx context.Context) error { return nil })
	if err != nil {
		t.Fatalf("restart after panic: %v", err)
	}
	waitDone(t, newTk)
	if newTk.Status() != StatusSucceeded {
		t.Fatalf("new task status = %v, want %v", newTk.Status(), StatusSucceeded)
	}
}

func TestStartWithLogBuffersAndTail(t *testing.T) {
	m := NewManager(8)
	var log *TaskLog
	tk, err := m.StartWithLog(context.Background(), "k1", Options{}, func(ctx context.Context, l *TaskLog) error {
		log = l
		l.Append("first")
		l.Append("second")
		return nil
	})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	waitDone(t, tk)
	if log == nil {
		t.Fatal("log was not injected")
	}
	lines, next := log.Tail(0)
	if len(lines) != 2 || lines[0] != "first" || lines[1] != "second" {
		t.Fatalf("tail = %q, want [first second]", lines)
	}
	if next != 2 {
		t.Fatalf("next = %d, want 2", next)
	}
	// 游标续读：从 next 起无新行。
	if lines, _ := log.Tail(next); len(lines) != 0 {
		t.Fatalf("expected no lines after cursor, got %q", lines)
	}
}

// TestRetryIntervalKeepsRunningAndActive 验证重试期间状态仍为 running、Active 仍
// 含该任务（PRD：重试期间 Active 不消失）。
func TestRetryIntervalKeepsRunningAndActive(t *testing.T) {
	m := NewManager(8)
	attempts := int32(0)
	// 第一次失败，第二次成功；间隔设长些以便在重试等待期间观察状态。
	tk, err := m.StartWithLog(context.Background(), "k1",
		Options{MaxRetries: 1, RetryInterval: 200 * time.Millisecond},
		func(ctx context.Context, log *TaskLog) error {
			if atomic.AddInt32(&attempts, 1) == 1 {
				log.Append("第一次失败")
				return errors.New("first attempt fails")
			}
			return nil
		})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	// 等第一次尝试失败、进入重试等待区间。
	deadline := time.Now().Add(2 * time.Second)
	for atomic.LoadInt32(&attempts) < 1 {
		if time.Now().After(deadline) {
			t.Fatal("first attempt never ran")
		}
		time.Sleep(5 * time.Millisecond)
	}
	// 重试等待期间：状态 running（任务在 Active 视角不消失）。
	if tk.Status() != StatusRunning {
		t.Fatalf("during retry interval status = %v, want %v", tk.Status(), StatusRunning)
	}
	// 日志已含第一次失败的分隔行（"第 1/2 次尝试失败"）。
	if lines, _ := tk.Log().Tail(0); !strings.Contains(strings.Join(lines, "\n"), "第 1/2 次尝试失败") {
		t.Fatalf("retry separator missing in log: %q", lines)
	}
	waitDone(t, tk)
	if tk.Status() != StatusSucceeded {
		t.Fatalf("final status = %v, want %v", tk.Status(), StatusSucceeded)
	}
}

// TestSucceededCleanupDoesNotRemoveNewSameKeyTask 回归：成功任务的清理与同 key 重装
// 交错时，旧任务不得删掉新任务的记录（否则新任务从 Get/Active 消失、去重失效）。
// 通过连续"成功→立即同 key 重启"循环压测竞态窗口。
func TestSucceededCleanupDoesNotRemoveNewSameKeyTask(t *testing.T) {
	m := NewManager(8)
	for i := range 200 {
		tk1, err := m.Start(context.Background(), "k", Options{}, func(ctx context.Context) error { return nil })
		if err != nil {
			t.Fatalf("iter %d start 1: %v", i, err)
		}
		waitDone(t, tk1)
		// 立刻用同 key 重启（模拟 SSE done 事件触发的重装）。
		tk2, err := m.Start(context.Background(), "k", Options{}, func(ctx context.Context) error { return nil })
		if err != nil {
			t.Fatalf("iter %d start 2 (same key after success): %v", i, err)
		}
		waitDone(t, tk2)
	}
	// 循环后再次启动必须成功（无残留终态挡住去重），且 Get 反映最新任务。
	tk, err := m.Start(context.Background(), "k", Options{}, blockOnCancel)
	if err != nil {
		t.Fatalf("final start: %v", err)
	}
	if got, ok := m.Get("k"); !ok || got != tk || got.Status() != StatusRunning {
		t.Fatalf("Get after stress = %+v, ok=%v; want the running task", got, ok)
	}
	m.Cancel("k")
}

// mustGet 断言任务存在并返回。
func mustGet(t *testing.T, m *Manager, key string) *Task {
	t.Helper()
	tk, ok := m.Get(key)
	if !ok {
		t.Fatalf("task %q not found", key)
	}
	return tk
}

// TestTaskLogWrite 验证 TaskLog 实现 io.Writer：按换行切行、无换行的尾部也立即
// 入列（实时性——mise 进度条用 \r 无 \n，不能等任务结束才 flush）、返回字节数。
func TestTaskLogWrite(t *testing.T) {
	// 单次 Write 多行。
	l := &TaskLog{}
	if n, err := l.Write([]byte("a\nb\nc\n")); err != nil || n != 6 {
		t.Fatalf("Write = (%d, %v), want (6, nil)", n, err)
	}
	if lines, _ := l.Tail(0); len(lines) != 3 || lines[0] != "a" || lines[1] != "b" || lines[2] != "c" {
		t.Fatalf("tail = %q, want [a b c]", lines)
	}

	// 无换行的尾部立即入列（不 pending），保证实时：
	// "a\nb" 切出 "a"，尾部 "b" 也入列；再写 "c\nd\n" 得 [a b c d]。
	l2 := &TaskLog{}
	_, _ = l2.Write([]byte("a\nb"))
	if lines, _ := l2.Tail(0); len(lines) != 2 || lines[0] != "a" || lines[1] != "b" {
		t.Fatalf("after partial write tail = %q, want [a b]", lines)
	}
	_, _ = l2.Write([]byte("c\nd\n"))
	if lines, _ := l2.Tail(0); len(lines) != 4 || lines[3] != "d" {
		t.Fatalf("after sequential write tail = %q, want [a b c d]", lines)
	}

	// Append 与 Write 混合：各自独立成行。
	l3 := &TaskLog{}
	_, _ = l3.Write([]byte("x\ny"))
	l3.Append("z")
	if lines, _ := l3.Tail(0); len(lines) != 3 || lines[0] != "x" || lines[1] != "y" || lines[2] != "z" {
		t.Fatalf("mixed tail = %q, want [x y z]", lines)
	}
}

// TestTasksSnapshot 验证 Tasks() 返回 key→status 快照，含 running 与 failed 任务。
func TestTasksSnapshot(t *testing.T) {
	m := NewManager(8)
	_, err := m.Start(context.Background(), "k1", Options{}, blockOnCancel)
	if err != nil {
		t.Fatalf("start k1: %v", err)
	}
	// 失败任务保留在 byKey（直至同 key 重提覆盖）。
	if _, err := m.Start(context.Background(), "k2", Options{MaxRetries: 0}, func(ctx context.Context) error {
		return errors.New("boom")
	}); err != nil {
		t.Fatalf("start k2: %v", err)
	}
	// 等 k2 进入 failed 终态。
	snap := m.Tasks()
	for snap["k2"] != StatusFailed {
		snap = m.Tasks()
	}
	if got := snap["k1"]; got != StatusRunning {
		t.Fatalf("k1 status = %v, want %v", got, StatusRunning)
	}
	if got := snap["k2"]; got != StatusFailed {
		t.Fatalf("k2 status = %v, want %v", got, StatusFailed)
	}
	m.Cancel("k1")
}

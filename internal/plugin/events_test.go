package plugin

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

// TestEventBusSubscribePublish 验证发布 → 订阅者收到（事件投递桥依赖此语义）。
func TestEventBusSubscribePublish(t *testing.T) {
	b := NewEventBus()
	var got atomic.Int32
	b.Subscribe("sub1", []string{"easyserver.test.one"}, func(ctx context.Context, e *Event) error {
		if e.Name != "easyserver.test.one" || e.Source != "src" {
			t.Errorf("事件字段不符: %+v", e)
		}
		got.Add(1)
		return nil
	})
	b.Publish("easyserver.test.one", "src", map[string]string{"k": "v"})
	waitFor(t, func() bool { return got.Load() == 1 }, "事件未投递")
}

// TestEventBusUnsubscribeStopsDelivery 验证退订后不再投递（插件停用/卸载时调用）。
func TestEventBusUnsubscribeStopsDelivery(t *testing.T) {
	b := NewEventBus()
	var got atomic.Int32
	b.Subscribe("sub2", []string{"easyserver.test.two"}, func(ctx context.Context, e *Event) error {
		got.Add(1)
		return nil
	})
	b.Unsubscribe("sub2")
	b.Publish("easyserver.test.two", "src", nil)
	time.Sleep(150 * time.Millisecond)
	if got.Load() != 0 {
		t.Fatalf("退订后仍收到事件: %d", got.Load())
	}
}

// TestEventBusRetryOnError 验证 at-least-once：handler 失败会退避重试。
func TestEventBusRetryOnError(t *testing.T) {
	b := NewEventBus()
	var attempts atomic.Int32
	b.Subscribe("sub3", []string{"easyserver.test.three"}, func(ctx context.Context, e *Event) error {
		if attempts.Add(1) <= 2 {
			return context.DeadlineExceeded // 前两次失败触发重试
		}
		return nil
	})
	b.Publish("easyserver.test.three", "src", nil)
	// 退避序列 1s/4s，等第 3 次成功
	deadline := time.After(10 * time.Second)
	for attempts.Load() < 3 {
		select {
		case <-deadline:
			t.Fatalf("重试未达 3 次: %d", attempts.Load())
		case <-time.After(20 * time.Millisecond):
		}
	}
}

// TestEventBusSubscribeReplace 验证重复订阅同 id 覆盖旧订阅（重启时不泄漏）。
func TestEventBusSubscribeReplace(t *testing.T) {
	b := NewEventBus()
	var first, second atomic.Int32
	b.Subscribe("sub4", []string{"easyserver.test.four"}, func(ctx context.Context, e *Event) error {
		first.Add(1)
		return nil
	})
	b.Subscribe("sub4", []string{"easyserver.test.four"}, func(ctx context.Context, e *Event) error {
		second.Add(1)
		return nil
	})
	b.Publish("easyserver.test.four", "src", nil)
	waitFor(t, func() bool { return second.Load() == 1 }, "新订阅未生效")
	if first.Load() != 0 {
		t.Fatalf("旧订阅仍收到事件: %d", first.Load())
	}
}

func waitFor(t *testing.T, cond func() bool, msg string) {
	t.Helper()
	deadline := time.After(3 * time.Second)
	for !cond() {
		select {
		case <-deadline:
			t.Fatal(msg)
		case <-time.After(10 * time.Millisecond):
		}
	}
}

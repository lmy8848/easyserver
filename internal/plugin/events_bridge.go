package plugin

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// 面板 → 插件的事件投递桥（文档 §8）：插件在 manifest.eventsubs 声明订阅的
// 事件，由面板 EventBus 按 at-least-once 语义经插件 socket POST 到 /events。
// 订阅以插件名为标识随启停绑定；投递时按当前 socket 实时解析，插件崩溃
// 重启后订阅自动续接。

// startEventDelivery 订阅插件声明的事件。重复调用先取消旧订阅，避免
// 反复启动（如手动重启）时订阅 goroutine 泄漏。
func (m *Manager) startEventDelivery(name string, mft *Manifest) {
	subs := mft.Contributions.EventSubs
	if len(subs) == 0 {
		return
	}
	events := make([]string, 0, len(subs))
	for _, s := range subs {
		events = append(events, s.Event)
	}
	m.bus.Unsubscribe("plugin:" + name)
	m.bus.Subscribe("plugin:"+name, events, func(ctx context.Context, e *Event) error {
		sock, _, ok := m.sup.RunningEndpoint(name)
		if !ok {
			return nil // 插件未运行：事件跳过，避免对停机插件无效重试
		}
		return postEvent(ctx, sock, e)
	})
}

// stopEventDelivery 取消插件的事件订阅（停止/禁用/卸载时调用）。
func (m *Manager) stopEventDelivery(name string) {
	m.bus.Unsubscribe("plugin:" + name)
}

// postEvent 把事件 POST 到插件 socket 的 /events；非 5xx 视为已接收
// （重试语义由 EventBus 的 at-least-once 负责）。
func postEvent(ctx context.Context, sockPath string, e *Event) error {
	body, err := json.Marshal(e)
	if err != nil {
		return err
	}
	// 复用按 socket 缓存的 Transport（连接池），避免每次投递新建连接泄漏
	client := &http.Client{Transport: transportFor(sockPath), Timeout: 10 * time.Second}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://plugin/events", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 500 {
		return fmt.Errorf("插件事件投递失败: %d", resp.StatusCode)
	}
	return nil
}

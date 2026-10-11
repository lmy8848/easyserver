package plugin

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"
)

// EventBus 双向事件总线（文档 §8）：发布/订阅/投递/限流。
//
// 语义：at-least-once（投递失败指数退避重试 5 次），事件带唯一 event_id
// 供订阅方去重，订阅方必须幂等；同一订阅者按发布顺序投递；每订阅者队列
// 上限 1000，溢出丢最旧并告警。
type EventBus struct {
	mu   sync.Mutex
	subs map[string]map[string]*eventSubscription // event → subID → sub
}

type eventSubscription struct {
	id      string
	events  map[string]bool // 订阅的事件集合
	handler func(ctx context.Context, e *Event) error
	queue   chan *Event
	closed  chan struct{}
	once    sync.Once
}

// Event 是总线上的一次事件。
type Event struct {
	ID      string          `json:"event_id"`
	Name    string          `json:"event"`
	Source  string          `json:"source"`
	Payload json.RawMessage `json:"payload"`
	Time    string          `json:"time"`
}

const (
	eventQueueSize   = 1000
	eventMaxAttempts = 5
)

// NewEventBus 创建事件总线。
func NewEventBus() *EventBus {
	return &EventBus{subs: map[string]map[string]*eventSubscription{}}
}

var defaultBus atomic.Pointer[EventBus]

// SetDefaultBus 设置全局默认总线（面板启动时接线，供业务点一行发布）。
func SetDefaultBus(b *EventBus) { defaultBus.Store(b) }

// DefaultBus 返回全局默认总线（未接线时返回 nil）。
func DefaultBus() *EventBus { return defaultBus.Load() }

// GenerateEventID 生成事件唯一 ID。
func GenerateEventID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("evt-%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}

// Subscribe 注册订阅者。handler 在订阅者的独立 goroutine 中串行调用，
// 返回错误触发重试。重复 Subscribe 同 (id, event) 覆盖旧订阅。
func (b *EventBus) Subscribe(id string, events []string, handler func(ctx context.Context, e *Event) error) {
	b.mu.Lock()
	if b.subs == nil {
		b.subs = map[string]map[string]*eventSubscription{}
	}
	sub := &eventSubscription{
		id:      id,
		events:  map[string]bool{},
		handler: handler,
		queue:   make(chan *Event, eventQueueSize),
		closed:  make(chan struct{}),
	}
	for _, e := range events {
		if b.subs[e] == nil {
			b.subs[e] = map[string]*eventSubscription{}
		}
		b.subs[e][id] = sub
		sub.events[e] = true
	}
	b.mu.Unlock()
	go sub.consume()
}

// Unsubscribe 取消订阅并关闭投递。
func (b *EventBus) Unsubscribe(id string) {
	b.mu.Lock()
	for e, m := range b.subs {
		if sub, ok := m[id]; ok {
			sub.once.Do(func() { close(sub.closed) })
			delete(m, id)
		}
		if len(m) == 0 {
			delete(b.subs, e)
		}
	}
	b.mu.Unlock()
}

// Publish 发布事件到所有匹配订阅者（非阻塞入队；满则丢弃最旧并告警）。
func (b *EventBus) Publish(name, source string, payload any) {
	if b == nil {
		return
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		log.Printf("plugin: 事件 %s payload 序列化失败: %v", name, err)
		return
	}
	evt := &Event{ID: GenerateEventID(), Name: name, Source: source, Payload: raw, Time: time.Now().Format(time.RFC3339)}

	b.mu.Lock()
	defer b.mu.Unlock()
	for _, sub := range b.subs[name] {
		select {
		case sub.queue <- evt:
		default:
			// 队列满：丢最旧，保新
			select {
			case <-sub.queue:
			default:
			}
			select {
			case sub.queue <- evt:
			default:
			}
			log.Printf("plugin: 事件 %s 订阅者 %s 队列溢出，已丢弃最旧", name, sub.id)
		}
	}
}

// consume 订阅者的串行消费循环：出队 → 投递（失败退避重试 5 次）。
func (sub *eventSubscription) consume() {
	for {
		select {
		case <-sub.closed:
			return
		case evt := <-sub.queue:
			deliverWithRetry(sub, evt)
		}
	}
}

func deliverWithRetry(sub *eventSubscription, evt *Event) {
	delay := time.Second
	for attempt := 1; attempt <= eventMaxAttempts; attempt++ {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		err := sub.handler(ctx, evt)
		cancel()
		if err == nil {
			return
		}
		log.Printf("plugin: 事件 %s 投递到 %s 失败（第 %d 次）: %v", evt.Name, sub.id, attempt, err)
		if attempt == eventMaxAttempts {
			log.Printf("plugin: 事件 %s 投递到 %s 超过重试上限，放弃", evt.Name, sub.id)
			return
		}
		select {
		case <-sub.closed:
			return
		case <-time.After(delay):
		}
		delay *= 4
	}
}

// ----------------------------------------------------------------------------
// 内置事件名常量（文档 §8.4，集中管理）
// ----------------------------------------------------------------------------

const (
	EventAuthLogin           = "easyserver.auth.login"
	EventAuthLogout          = "easyserver.auth.logout"
	EventAuthPasswordChanged = "easyserver.auth.password_changed"
	EventFileUpload          = "easyserver.file.upload"
	EventFileDelete          = "easyserver.file.delete"
	EventWebsiteCreated      = "easyserver.website.created"
	EventWebsiteDeleted      = "easyserver.website.deleted"
	EventCronExecuted        = "easyserver.cron.executed"
	EventServiceStart        = "easyserver.service.start"
	EventServiceStop         = "easyserver.service.stop"
	EventFirewallChanged     = "easyserver.firewall.changed"
)

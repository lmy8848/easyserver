// Package restart 提供热重启的信号总线。
//
// Signal 是轻量、无依赖的信号载体：settings handler 调 Request 请求重启，
// App 的事件循环通过 C 接收并执行。独立成包（而非放在 main）是为了让
// api 层能引用 Signal 类型而不依赖 main 包（Go 禁止 import main）。
package infra

import "log"

// RestartOpts 配置一次重启。
type RestartOpts struct {
	ConfigPath string
	DevMode    bool
	Force      bool
}

// Signal 是重启信号总线。handler 调 Request 发信号，App 事件循环接收。
type Signal struct {
	ch chan RestartOpts
}

// NewSignal 创建带缓冲的 Signal。
func NewSignal() *Signal { return &Signal{ch: make(chan RestartOpts, 1)} }

// Request 请求重启。非阻塞：已有待处理请求时丢弃新请求。
func (s *Signal) Request(opts RestartOpts) {
	select {
	case s.ch <- opts:
	default:
		log.Printf("restart: request already pending, ignoring")
	}
}

// C 返回信号 channel 的接收端。
func (s *Signal) C() <-chan RestartOpts { return s.ch }

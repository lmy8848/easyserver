package logger

import (
	"bytes"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"
)

// floodWindow 是同类日志抑制统计的时间窗口。
const floodWindow = time.Minute

// floodMax 是窗口内同一签名消息的最大落盘条数；超过后抑制并汇总提示。
const floodMax = 50

// FloodGuard 防止攻击者/异常流量刷屏日志文件：按"签名化后的消息内容"
// 在时间窗口内计数，超过阈值则抑制后续同类消息，并周期性输出一条汇总
// （"已抑制 N 条同类日志"），既保留可追溯性又不被刷爆磁盘与可读性。
//
// 典型场景：TLS 握手错误扫描（畸形 ClientHello）、未授权请求探测等——
// 攻击者控制的流量不应被原样全量落盘。签名化剥离 IP/端口/随机数等
// 逐条变化的部分，使同一攻击模式的重复消息聚合到同一签名。
//
// 用法：作为 http.Server.ErrorLog 的输出，或包在任何高频日志源前。
// 放行（未超阈值）的行转发到全局 slog（文件+stderr），被抑制的行丢弃。
type FloodGuard struct {
	mu      sync.Mutex
	now     func() time.Time // 可注入时钟（测试）
	emit    func(string)     // 放行/汇总输出目标（默认 logger.Info）
	counts  map[string]*floodEntry
	lastLog time.Time
}

type floodEntry struct {
	first      time.Time
	count      int
	suppressed int
	sample     string
}

// NewFloodGuard 创建洪泛守卫（使用真实时钟，输出到全局运行日志）。
func NewFloodGuard() *FloodGuard {
	return &FloodGuard{
		now: time.Now,
		emit: func(s string) {
			Info(s)
		},
		counts: map[string]*floodEntry{},
	}
}

// setEmit 注入输出目标（测试用）。
func (g *FloodGuard) setEmit(fn func(string)) { g.emit = fn }

// Sig 归一化日志行：将纯数字 token（IPv4 段/端口/计数/随机数）替换为 '#',
// 使仅 IP/端口/随机数不同的同类消息归并为一个签名。IPv6 段与证书指纹等
// 含字母数字混合的保持原样（保留可读性，且攻击者难以逐条变化字母绕过）。
func Sig(line string) string {
	var b strings.Builder
	b.Grow(len(line))
	for i := 0; i < len(line); i++ {
		c := line[i]
		if c >= '0' && c <= '9' {
			// 跳过整个连续数字串，替换为单个占位
			for i < len(line) && line[i] >= '0' && line[i] <= '9' {
				i++
			}
			b.WriteByte('#')
			i--
			continue
		}
		b.WriteByte(c)
	}
	return b.String()
}

// Write 实现 io.Writer：计数抑制，放行行转发到全局 slog。始终返回 len(p)
// （sink 语义：不阻塞调用方）。
func (g *FloodGuard) Write(p []byte) (int, error) {
	line := string(bytes.TrimSpace(p))
	if line == "" {
		return len(p), nil
	}
	sig := Sig(line)

	g.mu.Lock()
	now := g.now()
	e, ok := g.counts[sig]
	if !ok || now.Sub(e.first) > floodWindow {
		// 新签名或窗口过期：重置并放行
		g.counts[sig] = &floodEntry{first: now, count: 1, sample: line}
		g.mu.Unlock()
		g.emit(line)
		return len(p), nil
	}

	e.count++
	if e.count > floodMax {
		// 超过阈值：抑制本行，周期性输出汇总（汇总自身也限流，防刷屏）
		e.suppressed++
		suppressed := e.suppressed
		emitSummary := g.lastLog.IsZero() || now.Sub(g.lastLog) > floodWindow
		if emitSummary {
			g.lastLog = now
		}
		g.mu.Unlock()
		if emitSummary {
			g.emit(fmt.Sprintf("logger: 已抑制 %d 条同类日志: %s", suppressed, truncate(e.sample, 120)))
		}
		return len(p), nil
	}

	g.mu.Unlock()
	g.emit(line)
	return len(p), nil
}

// ServerErrorLog 返回 http.Server.ErrorLog 使用的限流日志器：
// TLS 握手错误/HTTP 服务内部错误经洪泛守卫后仍进全局日志（文件+stderr），
// 但同类刷屏被抑制，避免攻击者撑爆运行日志。
func ServerErrorLog() *log.Logger {
	return log.New(NewFloodGuard(), "", 0)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

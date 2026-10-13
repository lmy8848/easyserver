package logger

import (
	"strings"
	"testing"
	"time"
)

// FloodGuard 的签名归并与抑制逻辑测试（防攻击者刷屏日志）。

func TestSigNormalizesNumbers(t *testing.T) {
	cases := []struct{ in, want string }{
		{"http: TLS handshake error from 1.2.3.4:443", "http: TLS handshake error from #.#.#.#:#"},
		{"http: TLS handshake error from 8.8.8.8:12345", "http: TLS handshake error from #.#.#.#:#"},
		{"tls: client offered only unsupported versions: [301]", "tls: client offered only unsupported versions: [#]"},
		{"tls: no cipher suite supported by both client and server; client offered: [c02b c02c]", "tls: no cipher suite supported by both client and server; client offered: [c#b c#c]"},
		{"user 42 login", "user # login"},
	}
	for _, c := range cases {
		if got := Sig(c.in); got != c.want {
			t.Errorf("Sig(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestFloodGuardSuppresses 验证：同类刷屏超阈值后被抑制（放行条数有上限），
// 且周期性输出一条汇总提示。
func TestFloodGuardSuppresses(t *testing.T) {
	var emitted []string
	clock := time.Unix(1000, 0)
	g := NewFloodGuard()
	g.now = func() time.Time { return clock }
	g.setEmit(func(s string) { emitted = append(emitted, s) })

	line := []byte("http: TLS handshake error from 1.2.3.4:443")

	// 写入超过阈值的同类消息
	total := floodMax + 200
	for range total {
		if _, err := g.Write(line); err != nil {
			t.Fatalf("Write: %v", err)
		}
	}

	// 放行条数 = 阈值（超过的全部被抑制，不重复落盘）
	emittedRaw := 0
	for _, s := range emitted {
		if !strings.HasPrefix(s, "logger: 已抑制") {
			emittedRaw++
		}
	}
	if emittedRaw != floodMax {
		t.Fatalf("放行条数应 = floodMax(%d)，got %d（emitted=%v）", floodMax, emittedRaw, emitted)
	}

	// 汇总提示至少一条
	hasSummary := false
	for _, s := range emitted {
		if strings.Contains(s, "已抑制") {
			hasSummary = true
			break
		}
	}
	if !hasSummary {
		t.Fatal("应输出抑制汇总提示")
	}
}

// TestFloodGuardDifferentSignatures 验证：不同签名不互相抑制（正常流量不受影响）。
// 注意：仅 IP/端口不同的同类消息会因签名归一化而聚合（这正是防刷屏所需），
// 此处用真正不同的消息（含义不同）验证互不干扰。
func TestFloodGuardDifferentSignatures(t *testing.T) {
	var emitted []string
	g := NewFloodGuard()
	g.now = func() time.Time { return time.Unix(1000, 0) }
	g.setEmit(func(s string) { emitted = append(emitted, s) })

	// 三种含义不同的消息（签名互不相同）
	lines := []string{
		"tls handshake error from 1.1.1.1:443",
		"monitor cleaned up 100 old records",
		"user login failed for admin",
	}
	for range floodMax * 3 {
		for _, l := range lines {
			if _, err := g.Write([]byte(l)); err != nil {
				t.Fatalf("Write: %v", err)
			}
		}
	}
	// 三种签名各自独立计数：总放行应 = 3 * floodMax（汇总提示不算放行）
	emittedRaw := 0
	for _, s := range emitted {
		if !strings.HasPrefix(s, "logger: 已抑制") {
			emittedRaw++
		}
	}
	if emittedRaw != 3*floodMax {
		t.Fatalf("不同签名应各自放行 floodMax 条，got %d（emitted=%v）", emittedRaw, emitted)
	}
}

// TestServerErrorLog wiring：确认返回 *log.Logger（编译期）。
func TestServerErrorLog(t *testing.T) {
	if lg := ServerErrorLog(); lg == nil {
		t.Fatal("ServerErrorLog 不应为 nil")
	}
}

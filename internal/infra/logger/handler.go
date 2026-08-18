package logger

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
)

// makeHandler 构造 slog handler（text 或 json，按 format 切换），
// 注入动态等级 levelVar、可选源码定位（withSource），并包上 hookHandler
// 使每条记录都触发观察钩子。withSource=false 用于 stdlib 桥接——
// 其消息已含真实文件行号，附加的 source 会指向桥内部帧（错误），故关闭。
func makeHandler(w writerSink, withSource bool, format string) slog.Handler {
	opts := &slog.HandlerOptions{
		Level:       levelVar,
		AddSource:   withSource,
		ReplaceAttr: replaceSourceAttr,
	}
	var base slog.Handler
	switch format {
	case "json":
		base = slog.NewJSONHandler(w, opts)
	default:
		base = slog.NewTextHandler(w, opts)
	}
	return &hookHandler{Handler: base, onRecord: fireHooks}
}

// writerSink 是 sinkSet 与 bytes.Buffer（测试）共有的最小接口，便于可插拔注入。
type writerSink interface{ Write(p []byte) (int, error) }

// hookHandler 包装底层 TextHandler/JSONHandler：Handle 时先触发 hooks 再落盘。
// WithAttrs/WithGroup 保持包装，保证 With 派生的子 logger 同样触发 hooks。
type hookHandler struct {
	slog.Handler
	onRecord func(slog.Record)
}

func (h *hookHandler) Handle(ctx context.Context, r slog.Record) error {
	h.onRecord(r)
	return h.Handler.Handle(ctx, r)
}

func (h *hookHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &hookHandler{Handler: h.Handler.WithAttrs(attrs), onRecord: h.onRecord}
}

func (h *hookHandler) WithGroup(name string) slog.Handler {
	return &hookHandler{Handler: h.Handler.WithGroup(name), onRecord: h.onRecord}
}

// replaceSourceAttr 把 slog 的 source 属性压缩为 `函数@文件:行`（如
// cmd/server.(*App).Run@app.go:72），满足"报错可定位到类与方法"。
func replaceSourceAttr(_ []string, a slog.Attr) slog.Attr {
	if a.Key != slog.SourceKey {
		return a
	}
	if src, ok := a.Value.Any().(*slog.Source); ok {
		fn := trimFunc(src.Function)
		a.Value = slog.StringValue(fmt.Sprintf("%s@%s:%d", fn, filepath.Base(src.File), src.Line))
	}
	return a
}

// trimFunc 去掉模块前缀段（easyserver/…），保留 包.方法 可读片段。
func trimFunc(fn string) string {
	if i := strings.Index(fn, "/"); i >= 0 {
		return fn[i+1:]
	}
	return fn
}

func errInvalidLevel(s string) error {
	return fmt.Errorf("logger: 无效的日志等级 %q，可选 debug|info|warn|error", s)
}

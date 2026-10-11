package sdk

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"sync"
)

// Logger 是插件的日志器：同时写 stderr（插件自身输出，面板 journald 可见）
// 与面板侧插件日志文件（经 Host API log.write，面板 UI 可查）。
//
// 用法：
//
//	lg := sdk.NewLogger(sdk.NewHostClient())
//	lg.Infof("插件启动，pid=%d", os.Getpid())
type Logger struct {
	host *HostClient
	mu   sync.Mutex
	out  io.Writer
}

// NewLogger 创建日志器；host 可为 nil（仅本地输出）。
func NewLogger(host *HostClient) *Logger {
	out := io.Writer(os.Stderr)
	if f, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0); err == nil {
		_ = f.Close() // 仅探测可写性，实际用 stderr
	}
	return &Logger{host: host, out: out}
}

// SetOutput 覆盖输出目标（默认 stderr；测试可注入）。
func (l *Logger) SetOutput(w io.Writer) {
	l.mu.Lock()
	l.out = w
	l.mu.Unlock()
}

// Infof 记录 info 级日志（本地 + 面板侧异步）。
func (l *Logger) Infof(format string, args ...any) {
	l.logf("info", format, args...)
}

// Warnf 记录 warn 级日志。
func (l *Logger) Warnf(format string, args ...any) {
	l.logf("warn", format, args...)
}

// Errorf 记录 error 级日志。
func (l *Logger) Errorf(format string, args ...any) {
	l.logf("error", format, args...)
}

func (l *Logger) logf(level, format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	l.mu.Lock()
	fmt.Fprintf(l.out, "%s %s\n", level, msg)
	l.mu.Unlock()
	if l.host != nil {
		// 面板侧异步写入，失败静默（面板不可达时不影响插件）
		go func() {
			_ = l.host.Log(context.Background(), fmt.Sprintf("%s %s", level, msg))
		}()
	}
}

// StandardLog 返回标准库 *log.Logger（供第三方库的 log 输出汇入插件日志）。
func (l *Logger) StandardLog() *log.Logger {
	return log.New(logWriter{l: l}, "", log.LstdFlags)
}

type logWriter struct{ l *Logger }

func (w logWriter) Write(p []byte) (int, error) {
	w.l.logf("info", "%s", string(p))
	return len(p), nil
}

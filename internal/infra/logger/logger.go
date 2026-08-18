// Package logger 提供全局运行日志：持久化落盘到应用根目录、分级过滤、
// 源码定位、运行时可调等级、高可用降级，并以可插拔的 Target / Hook 架构
// 面向后续扩展（面板流式、告警、JSON、远程聚合）。
//
// 设计要点：
//   - 分级基于标准库 log/slog，zero 第三方依赖；等级用 slog.LevelVar 原子切换。
//   - 现有项目内 225 处无等级 log.Printf 通过 stdlib 桥接按 Info 透传，
//     保留下划线真实文件行号，后续随改动逐步迁到本包的分级调用。
//   - 写文件失败不 panic、不阻塞、不丢日志（降级 stderr），进程照常运行。
package logger

import (
	"context"
	"fmt"
	"io"
	"log"
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"

	"easyserver/internal/infra/config"
)

// Hook 观察钩子：本包产生的每一条日志记录（含 stdlib 桥接）都会回调。
// 用于旁路能力（告警、转发、度量、上报）；实现必须快速返回、不得 panic。
type Hook func(ctx context.Context, r slog.Record)

// levelVar 全局动态等级：Init 按配置设置，面板可随时原子切换。
var levelVar = new(slog.LevelVar)

// sink 默认多写目标组合（file + stderr）；AddTarget 可运行时追加。
var sink = newSinkSet()

// root 根 logger，Init 后指向本包 handler；With 得到的子 logger 由此派生。
var root = slog.Default()

// bridgeLogger 仅用于 stdlib log 桥接（不附加 source，消息自带文件行号）。
var bridgeLogger = slog.Default()

// defaultPath 是 Init 解析出的实际日志文件路径（面板设置页展示用）。
var defaultPath string

var (
	hooksMu sync.RWMutex
	hooks   []Hook
)

// Init 初始化全局日志：文件落盘（应用根目录默认）+ stderr 双写、分级、源码定位、
// stdlib 桥接。返回的 Closer 在进程退出时关闭文件；打不开文件时返回 error，
// 但本包已保证 sink 至少含 stderr，调用方降级提示后进程照常运行。
//
// Init 需在进程最早的日志输出（如 mise bootstrap）之前调用。
func Init(cfg config.LogsConfig) (io.Closer, error) {
	// 1. 等级：解析失败回退 info（不阻断启动）。
	lv, err := parseLevel(cfg.Level)
	if err != nil {
		lv = slog.LevelInfo
	}
	levelVar.Set(lv)

	// 2. 文件目标：路径空则取应用根目录（DataRoot 派生，非硬编码）。
	path := cfg.Path
	if path == "" {
		path = filepath.Join(config.DataRoot, "easyserver.log")
	}
	defaultPath = path

	maxSize := int64(cfg.MaxSizeMB) * 1024 * 1024
	if maxSize <= 0 {
		maxSize = 10 * 1024 * 1024 // 默认 10MB
	}
	maxFiles := cfg.MaxFiles
	if maxFiles <= 0 {
		maxFiles = 3
	}

	// 3. 组合多写目标：stderr 先就位（保证 Init 失败也有输出），文件成功后再置前。
	sink.reset(stderrWriter{})
	ft, ferr := newFileTarget(path, maxSize, maxFiles)
	if ferr != nil {
		return nil, ferr // 调用方降级提示；sink 已含 stderr，后续日志仍可见
	}
	sink.reset(ft, stderrWriter{})

	// 4. handler：格式按配置（text 默认 / json）。
	root = slog.New(makeHandler(sink, true, cfg.Format))
	slog.SetDefault(root)

	// 5. stdlib log 桥接：保留 Lshortfile 真实行号，按 Info 进同一 sink/hook。
	bridgeLogger = slog.New(makeHandler(sink, false, cfg.Format))
	log.SetFlags(log.Lshortfile)
	log.SetOutput(logBridge{})

	return ft, nil
}

// LogPath 返回 Init 解析出的实际日志文件路径；未 Init 时返回空串。
func LogPath() string { return defaultPath }

// SetLevel 运行时切换日志等级（面板设置页调用），非法值返回 error 不生效。
func SetLevel(name string) error {
	lv, err := parseLevel(name)
	if err != nil {
		return err
	}
	levelVar.Set(lv)
	return nil
}

// GetLevel 返回当前等级名（debug|info|warn|error）。
func GetLevel() string { return levelName(levelVar.Level()) }

// Default 返回根 *slog.Logger，供需要 slog 完整能力（WithGroup 等）的调用方使用。
func Default() *slog.Logger { return root }

// With 返回带子系统标签的子 logger，如 logger.With("module", "auth")。
func With(keyvals ...any) *slog.Logger { return root.With(keyvals...) }

// AddTarget 运行时挂接新的输出目标（面板流式广播、远程聚合等扩展点）。
func AddTarget(t io.Writer) { sink.Add(t) }

// AddHook 运行时注册观察钩子（告警、转发等旁路能力扩展点），返回移除函数。
func AddHook(h Hook) (remove func()) {
	hooksMu.Lock()
	hooks = append(hooks, h)
	hooksMu.Unlock()
	return func() {
		hooksMu.Lock()
		defer hooksMu.Unlock()
		for i, hh := range hooks {
			if hh == nil {
				continue
			}
			// 函数指针比对（Hook 是 func 类型），精确移除已注册的那个。
			if reflect.ValueOf(hh).Pointer() == reflect.ValueOf(h).Pointer() {
				hooks = append(hooks[:i], hooks[i+1:]...)
				break
			}
		}
	}
}

// 分级便捷入口（包内主要使用方式）。
func Debug(msg string, args ...any) { root.Debug(msg, args...) }
func Info(msg string, args ...any)  { root.Info(msg, args...) }
func Warn(msg string, args ...any)  { root.Warn(msg, args...) }
func Error(msg string, args ...any) { root.Error(msg, args...) }

// fireHooks 不阻塞、不 panic：单个 hook 出错仅记录计数，不影响写日志主链路。
func fireHooks(r slog.Record) {
	hooksMu.RLock()
	hs := hooks
	hooksMu.RUnlock()
	if len(hs) == 0 {
		return
	}
	ctx := context.Background()
	for _, h := range hs {
		func() {
			defer func() {
				if v := recover(); v != nil {
					// 直写 stderr，绕开日志主链路，避免经桥接再次触发 hooks 造成递归。
					_, _ = io.WriteString(os.Stderr, fmt.Sprintf("logger: hook panic ignored: %v\n", v))
				}
			}()
			h(ctx, r)
		}()
	}
}

// logBridge 是 stdlib log 的输出桥：把一行（已含文件行号）按 Info 送进 sink/hook。
type logBridge struct{}

func (logBridge) Write(p []byte) (int, error) {
	msg := strings.TrimRight(string(p), "\n")
	bridgeLogger.Log(context.Background(), slog.LevelInfo, msg)
	return len(p), nil
}

// stderrWriter 让 sink 能以 Target 形式持有标准错误（保留 nohup/systemd 控制台输出）。
type stderrWriter struct{}

func (stderrWriter) Write(p []byte) (int, error) { return io.Writer(os.Stderr).Write(p) }

// parseLevel / levelName 在 debug|info|warn|error 间映射。
func parseLevel(s string) (slog.Level, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return slog.LevelDebug, nil
	case "info", "":
		return slog.LevelInfo, nil
	case "warn", "warning":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	}
	return 0, errInvalidLevel(s)
}

func levelName(l slog.Level) string {
	switch {
	case l >= slog.LevelError:
		return "error"
	case l >= slog.LevelWarn:
		return "warn"
	case l >= slog.LevelInfo:
		return "info"
	default:
		return "debug"
	}
}

package sdk

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"
)

// Config 是插件 socket 服务配置。
type Config struct {
	// SocketPath 插件 socket 路径。面板注入 EASYSERVER_PLUGIN_SOCKET 时
	// 以绝对路径为准（Supervisor 探测 /run/easyserver/plugins/ 下）；
	// 本地运行（无该环境变量）时用此相对路径（相对进程 CWD）。
	SocketPath string
	// Health 可选自定义健康检查；nil 视为健康。
	Health func() error
}

// Server 是插件的 HTTP 服务（监听 Unix socket）：
//   - 自动提供 GET /health（可自定义）与 POST /events
//   - 业务路由经 HandleFunc / Group 注册；Recovery 等中间件需 Use 显式追加
//     （中间件对 /health 与 /events 内建端点不生效）
//   - 经 Graceful 支持优雅退出
type Server struct {
	socketPath   string
	mux          *http.ServeMux
	middlewares  []func(http.HandlerFunc) http.HandlerFunc
	eventHandler func(ctx context.Context, e *Event) error
	readyFns     []func()
	mu           sync.Mutex
	srv          *http.Server
	ln           net.Listener
}

// NewServer 创建插件 socket 服务（调用 Serve 后开始监听）。
func NewServer(cfg Config) *Server {
	s := &Server{
		socketPath: cfg.SocketPath,
		mux:        http.NewServeMux(),
	}
	s.mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		if cfg.Health != nil {
			if err := cfg.Health(); err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
				return
			}
		}
		w.WriteHeader(http.StatusOK)
	})
	s.mux.HandleFunc("/events", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		var evt Event
		if err := json.NewDecoder(r.Body).Decode(&evt); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		s.mu.Lock()
		handler := s.eventHandler
		s.mu.Unlock()
		if handler == nil {
			w.WriteHeader(http.StatusOK)
			return
		}
		// 事件回调 panic 兜底：标记 500（面板按 at-least-once 重试）
		func() {
			defer func() {
				if p := recover(); p != nil {
					log.Printf("sdk: 事件 %s 处理 panic: %v", evt.Name, p)
					w.WriteHeader(http.StatusInternalServerError)
				}
			}()
			if err := handler(r.Context(), &evt); err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			w.WriteHeader(http.StatusOK)
		}()
	})
	return s
}

// HandleFunc 注册业务路由（含 /health、/events 之外的自定义端点）。
func (s *Server) HandleFunc(pattern string, h http.HandlerFunc) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.mux.HandleFunc(pattern, s.wrap(h))
}

// Group 返回同一前缀下的路由组（如 /api 组），简化路由组织。
func (s *Server) Group(prefix string) *Group {
	return &Group{s: s, prefix: prefix}
}

// Group 是一组带前缀的路由。
type Group struct {
	s      *Server
	prefix string
}

// HandleFunc 在组前缀下注册路由。
func (g *Group) HandleFunc(pattern string, h http.HandlerFunc) {
	g.s.HandleFunc(g.prefix+pattern, h)
}

// Use 追加全局中间件（先注册的先执行）。中间件形如
// func(next http.HandlerFunc) http.HandlerFunc。
// 注意：应在 HandleFunc/Serve 之前调用——wrap 在注册时快照中间件，
// 晚于 HandleFunc 注册的中间件对已注册路由不生效。
func (s *Server) Use(mw func(http.HandlerFunc) http.HandlerFunc) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.middlewares = append(s.middlewares, mw)
}

// wrap 按注册顺序套中间件。
func (s *Server) wrap(h http.HandlerFunc) http.HandlerFunc {
	for i := len(s.middlewares) - 1; i >= 0; i-- {
		h = s.middlewares[i](h)
	}
	return h
}

// OnEvent 注册面板事件回调（必须在 Serve 前调用）。
// 返回 error 表示处理失败，面板将按 at-least-once 语义重试。
func (s *Server) OnEvent(h func(ctx context.Context, e *Event) error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.eventHandler = h
}

// OnReady 注册 socket 监听成功后的回调（可用于输出就绪日志或预初始化）。
func (s *Server) OnReady(fn func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.readyFns = append(s.readyFns, fn)
}

// Serve 监听并服务（阻塞）。若面板注入了 EASYSERVER_PLUGIN_SOCKET，
// 优先监听该绝对路径。返回前清理旧 socket 并 chmod 0600。
func (s *Server) Serve() error {
	if abs := os.Getenv(EnvPluginSocket); abs != "" {
		s.socketPath = abs
	}
	if err := os.MkdirAll(filepath.Dir(s.socketPath), 0700); err != nil {
		return err
	}
	_ = os.Remove(s.socketPath)
	ln, err := net.Listen("unix", s.socketPath)
	if err != nil {
		return fmt.Errorf("listen unix socket: %w", err)
	}
	if err := os.Chmod(s.socketPath, 0600); err != nil {
		return err
	}
	s.mu.Lock()
	s.ln = ln
	s.srv = &http.Server{Handler: s.mux, ReadHeaderTimeout: 5 * time.Second}
	readyFns := append([]func(){}, s.readyFns...)
	s.mu.Unlock()

	for _, fn := range readyFns {
		fn()
	}
	return s.srv.Serve(ln)
}

// Close 关闭监听与连接，并清理 socket 文件（避免残留导致下次 Serve
// 误判端口占用/权限混乱）。
func (s *Server) Close() error {
	s.mu.Lock()
	srv := s.srv
	sockPath := s.socketPath
	s.mu.Unlock()
	if srv != nil {
		err := srv.Close()
		_ = os.Remove(sockPath)
		return err
	}
	return nil
}

// Graceful 阻塞等待 SIGTERM/SIGINT 后优雅关闭（插件 main 的收尾常用）：
//
//	srv.Graceful()
func (s *Server) Graceful() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	<-ctx.Done()
	_ = s.Close()
}

// ----------------------------------------------------------------------------
// JSON 响应辅助（插件 handler 高频使用）
// ----------------------------------------------------------------------------

// JSON 写 JSON 响应（content-type application/json + 状态码）。
func JSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

// Error 写标准错误响应体 {"error": msg}。
func Error(w http.ResponseWriter, code int, msg string) {
	JSON(w, code, map[string]string{"error": msg})
}

// OK 写成功 JSON {"message": msg}。
func OK(w http.ResponseWriter, msg string) {
	JSON(w, http.StatusOK, map[string]string{"message": msg})
}

// Recovery 中间件：panic 转 500，避免进程崩溃（默认已由事件处理内置，
// 业务路由也建议套用）。
func Recovery() func(http.HandlerFunc) http.HandlerFunc {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if p := recover(); p != nil {
					log.Printf("sdk: handler panic: %v", p)
					Error(w, http.StatusInternalServerError, "internal error")
				}
			}()
			next(w, r)
		}
	}
}

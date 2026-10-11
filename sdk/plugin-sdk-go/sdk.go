// Package sdk 是 EasyServer 插件的 Go SDK（文档 §16.1）：
// 封装插件 Unix socket 服务、面板事件接收与 Host API 客户端，
// 插件作者只需实现业务 handler。
//
// 用法：
//
//	srv := sdk.NewServer(sdk.Config{SocketPath: "./tunnel-frp.sock"})
//	srv.HandleFunc("/api/status", statusHandler)
//	srv.OnEvent(func(ctx context.Context, e *sdk.Event) { ... })
//	go srv.Serve()
//	hc := sdk.NewHostClient()
//	_ = hc.Audit(ctx, "做了点什么")
package sdk

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// 环境变量名（面板拉起插件时注入，文档 §7.1）。
const (
	EnvPluginToken = "EASYSERVER_PLUGIN_TOKEN"
	EnvHostSocket  = "EASYSERVER_HOST_SOCKET"
	EnvPluginID    = "EASYSERVER_PLUGIN_ID"
	EnvPluginSocket = "EASYSERVER_PLUGIN_SOCKET"
)

// Event 是面板投递的事件（文档 §8：带 event_id 供去重，插件需幂等）。
type Event struct {
	ID      string          `json:"event_id"`
	Name    string          `json:"event"`
	Source  string          `json:"source"`
	Payload json.RawMessage `json:"payload"`
	Time    string          `json:"time"`
}

// Config 是插件 socket 服务配置。
type Config struct {
	// SocketPath 插件 socket 路径（与 plugin.yaml runtime.socket 一致）。
	SocketPath string
	// Health 可选自定义健康检查；nil 视为健康。
	Health func() error
}

// Server 是插件的 Unix socket HTTP 服务：自动提供 /health 与 /events，
// 业务路由经 HandleFunc 注册。
type Server struct {
	socketPath   string
	mux          *http.ServeMux
	eventHandler func(ctx context.Context, e *Event) error
	srv          *http.Server
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
		if s.eventHandler == nil {
			w.WriteHeader(http.StatusOK)
			return
		}
		// 面板投递已有 30s 超时与重试；回调 panic 兜底为 500（不波及插件其它逻辑）
		func() {
			defer func() {
				if p := recover(); p != nil {
					log.Printf("sdk: 事件 %s 处理 panic: %v", evt.Name, p)
					w.WriteHeader(http.StatusInternalServerError)
				}
			}()
			if err := s.eventHandler(r.Context(), &evt); err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			w.WriteHeader(http.StatusOK)
		}()
	})
	return s
}

// HandleFunc 注册插件业务路由。
func (s *Server) HandleFunc(pattern string, h http.HandlerFunc) {
	s.mux.HandleFunc(pattern, h)
}

// OnEvent 注册面板事件回调（必须在 Serve 前调用）。
// 返回 error 表示处理失败，面板将按 at-least-once 语义重试。
func (s *Server) OnEvent(h func(ctx context.Context, e *Event) error) {
	s.eventHandler = h
}

// Serve 监听并服务（阻塞）。返回前清理旧 socket，监听后 chmod 0600。
// 面板注入的 EASYSERVER_PLUGIN_SOCKET 绝对路径优先（Supervisor 探测的是
// /run/easyserver/plugins/ 下的路径），Config.SocketPath 作为本地运行回退。
func (s *Server) Serve() error {
	if abs := os.Getenv(EnvPluginSocket); abs != "" {
		s.socketPath = abs
	}
	_ = os.Remove(s.socketPath)
	if err := os.MkdirAll(filepath.Dir(s.socketPath), 0700); err != nil {
		return err
	}
	ln, err := net.Listen("unix", s.socketPath)
	if err != nil {
		return fmt.Errorf("listen unix socket: %w", err)
	}
	if err := os.Chmod(s.socketPath, 0600); err != nil {
		return err
	}
	s.srv = &http.Server{Handler: s.mux, ReadHeaderTimeout: 5 * time.Second}
	return s.srv.Serve(ln)
}

// Close 关闭监听与连接。
func (s *Server) Close() error {
	if s.srv != nil {
		return s.srv.Close()
	}
	return nil
}

// HostClient 是面板 Host API 的客户端（经 host unix socket，Bearer token）。
type HostClient struct {
	token      string
	hostSocket string
	client     *http.Client
}

// NewHostClient 从面板注入的环境变量创建 Host API 客户端。
func NewHostClient() *HostClient {
	return &HostClient{
		token:      os.Getenv(EnvPluginToken),
		hostSocket: os.Getenv(EnvHostSocket),
		client:     &http.Client{Timeout: 30 * time.Second},
	}
}

func (h *HostClient) do(ctx context.Context, method, path string, body any, out any) error {
	if h.hostSocket == "" {
		return fmt.Errorf("Host API 未接线（%s 缺失）", EnvHostSocket)
	}
	tr := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, "unix", h.hostSocket)
		},
	}
	client := &http.Client{Transport: tr, Timeout: 30 * time.Second}

	var rdr *strings.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rdr = strings.NewReader(string(b))
	} else {
		rdr = strings.NewReader("")
	}
	req, err := http.NewRequestWithContext(ctx, method, "http://host/api/host/"+strings.TrimPrefix(path, "/"), rdr)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+h.token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("host api %s %s: %d", method, path, resp.StatusCode)
	}
	if out != nil {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return nil
}

// Audit 写面板审计。
func (h *HostClient) Audit(ctx context.Context, summary string) error {
	return h.do(ctx, http.MethodPost, "audit", map[string]string{"summary": summary}, nil)
}

// ConfigGet 读取插件配置项。
func (h *HostClient) ConfigGet(ctx context.Context, key string) (string, error) {
	var out struct {
		Key   string `json:"key"`
		Value string `json:"value"`
	}
	if err := h.do(ctx, http.MethodGet, "config/"+key, nil, &out); err != nil {
		return "", err
	}
	return out.Value, nil
}

// ConfigSet 写插件配置项。
func (h *HostClient) ConfigSet(ctx context.Context, key, value string) error {
	return h.do(ctx, http.MethodPut, "config", map[string]string{key: value}, nil)
}

// ConfigAll 读取全部配置。
func (h *HostClient) ConfigAll(ctx context.Context) (map[string]string, error) {
	out := map[string]string{}
	if err := h.do(ctx, http.MethodGet, "config", nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// Log 写插件日志（面板侧 /var/log/easyserver/plugins/<id>.log）。
func (h *HostClient) Log(ctx context.Context, message string) error {
	return h.do(ctx, http.MethodPost, "log", map[string]string{"message": message}, nil)
}

// PublishEvent 发布事件（事件名必须以 easyserver.<插件名>. 为域）。
func (h *HostClient) PublishEvent(ctx context.Context, event string, payload any) error {
	return h.do(ctx, http.MethodPost, "event/publish", map[string]any{"event": event, "payload": payload}, nil)
}

// Exec 有审计+超时的命令执行（等同 root，文档 §7.3）。
func (h *HostClient) Exec(ctx context.Context, cmd string, args []string, timeoutSeconds int) (stdoutStderr string, exitCode int, err error) {
	var out struct {
		ExitCode    int    `json:"exit_code"`
		StdoutStderr string `json:"stdout_stderr"`
	}
	if err := h.do(ctx, http.MethodPost, "exec", map[string]any{"cmd": cmd, "args": args, "timeout_seconds": timeoutSeconds}, &out); err != nil {
		return "", -1, err
	}
	return out.StdoutStderr, out.ExitCode, nil
}

// Notify 经面板通知通道发送消息。
func (h *HostClient) Notify(ctx context.Context, title, content string) error {
	return h.do(ctx, http.MethodPost, "notify", map[string]string{"title": title, "content": content}, nil)
}

// StorageRead 读取插件私有文件。
func (h *HostClient) StorageRead(ctx context.Context, path string) ([]byte, error) {
	if h.hostSocket == "" {
		return nil, fmt.Errorf("Host API 未接线")
	}
	tr := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, "unix", h.hostSocket)
		},
	}
	client := &http.Client{Transport: tr, Timeout: 30 * time.Second}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://host/api/host/storage/"+strings.TrimPrefix(path, "/"), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+h.token)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("storage read: %d", resp.StatusCode)
	}
	out := make([]byte, 0, 4096)
	buf := make([]byte, 4096)
	for {
		n, err := resp.Body.Read(buf)
		out = append(out, buf[:n]...)
		if err != nil {
			break
		}
	}
	return out, nil
}

// StorageWrite 写插件私有文件。
func (h *HostClient) StorageWrite(ctx context.Context, path string, data []byte) error {
	if h.hostSocket == "" {
		return fmt.Errorf("Host API 未接线")
	}
	tr := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, "unix", h.hostSocket)
		},
	}
	client := &http.Client{Transport: tr, Timeout: 30 * time.Second}
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, "http://host/api/host/storage/"+strings.TrimPrefix(path, "/"), strings.NewReader(string(data)))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+h.token)
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("storage write: %d", resp.StatusCode)
	}
	return nil
}

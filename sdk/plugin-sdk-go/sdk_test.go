package sdk

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestServerHealthAndRoutes 验证 socket 服务的 /health 与自定义路由。
func TestServerHealthAndRoutes(t *testing.T) {
	dir := t.TempDir()
	sock := filepath.Join(dir, "test.sock")

	srv := NewServer(Config{SocketPath: sock, Health: func() error { return nil }})
	srv.HandleFunc("/api/ping", func(w http.ResponseWriter, _ *http.Request) {
		JSON(w, http.StatusOK, map[string]string{"pong": "yes"})
	})
	// Recovery 中间件：panic 的路由不应崩溃进程
	srv.Use(Recovery())
	srv.HandleFunc("/api/panic", func(http.ResponseWriter, *http.Request) { panic("boom") })

	go func() { _ = srv.Serve() }()
	defer func() { _ = srv.Close() }()

	// 等 socket 就绪
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(sock); err == nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	c := unixClient(t, sock)
	if got := get(t, c, "/health"); got != 200 {
		t.Fatalf("health 期望 200，实际 %d", got)
	}
	if got := get(t, c, "/api/ping"); got != 200 {
		t.Fatalf("ping 期望 200，实际 %d", got)
	}
	if got := get(t, c, "/api/panic"); got != 500 {
		t.Fatalf("panic 路由期望 500，实际 %d", got)
	}
}

// TestJSONAndError 验证 JSON 响应辅助。
func TestJSONAndError(t *testing.T) {
	var buf bytes.Buffer
	rw := newRecorder(&buf)

	JSON(rw, 201, map[string]string{"id": "x"})
	if rw.code != 201 || !bytes.Contains(buf.Bytes(), []byte(`"id":"x"`)) {
		t.Fatalf("JSON 辅助异常: code=%d body=%s", rw.code, buf.String())
	}

	buf.Reset()
	Error(rw, 400, "bad")
	if rw.code != 400 || !bytes.Contains(buf.Bytes(), []byte(`"error":"bad"`)) {
		t.Fatalf("Error 辅助异常: code=%d body=%s", rw.code, buf.String())
	}
}

// TestEventDecode 验证事件载荷解码。
func TestEventDecode(t *testing.T) {
	evt := &Event{ID: "e1", Name: "easyserver.auth.login", Payload: json.RawMessage(`{"username":"admin"}`)}
	var p struct {
		Username string `json:"username"`
	}
	if err := evt.Decode(&p); err != nil || p.Username != "admin" {
		t.Fatalf("事件解码失败: %v %+v", err, p)
	}
	empty := &Event{Name: "x"}
	if err := empty.Decode(&p); err != nil {
		t.Fatalf("空载荷解码应成功: %v", err)
	}
}

// TestHostClientMock 用 mock host socket 验证 HostClient 调用链。
func TestHostClientMock(t *testing.T) {
	dir := t.TempDir()
	hostSock := filepath.Join(dir, "host.sock")
	ln, err := net.Listen("unix", hostSock)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ln.Close() }()

	mux := http.NewServeMux()
	mux.HandleFunc("/api/host/config", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if got := r.Header.Get("Authorization"); got != "Bearer tok123" {
			t.Errorf("期望 Bearer tok123，实际 %s", got)
		}
		JSON(w, http.StatusOK, map[string]string{"message": "ok"})
	})
	mux.HandleFunc("/api/host/audit", func(w http.ResponseWriter, _ *http.Request) {
		JSON(w, http.StatusOK, map[string]string{"message": "ok"})
	})
	srv := &http.Server{Handler: mux}
	go func() { _ = srv.Serve(ln) }()
	defer func() { _ = srv.Close() }()

	t.Setenv(EnvPluginToken, "tok123")
	t.Setenv(EnvHostSocket, hostSock)
	hc := NewHostClient()
	if !hc.Connected() {
		t.Fatal("Connected 应为 true")
	}
	if err := hc.ConfigSet(context.Background(), "k", "v"); err != nil {
		t.Fatalf("ConfigSet: %v", err)
	}
	if err := hc.Audit(context.Background(), "test"); err != nil {
		t.Fatalf("Audit: %v", err)
	}
}

// TestLoggerLocal 验证日志器本地输出。
func TestLoggerLocal(t *testing.T) {
	var buf bytes.Buffer
	lg := NewLogger(nil)
	lg.SetOutput(&buf)
	lg.Infof("hello %s", "world")
	if !bytes.Contains(buf.Bytes(), []byte("hello world")) {
		t.Fatalf("日志输出异常: %s", buf.String())
	}
}

// ---- helpers ----

func unixClient(t *testing.T, sock string) *http.Client {
	t.Helper()
	tr := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, "unix", sock)
		},
	}
	return &http.Client{Transport: tr, Timeout: 3 * time.Second}
}

func get(t *testing.T, c *http.Client, path string) int {
	t.Helper()
	resp, err := c.Get("http://plugin" + path)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	return resp.StatusCode
}

type recorder struct {
	code int
	w    io.Writer
	h    http.Header
}

func newRecorder(w io.Writer) *recorder { return &recorder{w: w, h: http.Header{}} }
func (r *recorder) Header() http.Header  { return r.h }
func (r *recorder) Write(b []byte) (int, error) {
	if r.code == 0 {
		r.code = 200
	}
	return r.w.Write(b)
}
func (r *recorder) WriteHeader(code int) { r.code = code }

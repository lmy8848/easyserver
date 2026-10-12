// tunnel-frp 插件：基于 frp 的内网穿透（家庭网络服务经服务器对外暴露）。
//
// 架构：插件进程内嵌 frps（frp 服务端库），家庭电脑跑 frpc（客户端配置
// 由本插件生成）主动连接本服务器的 frps 控制端口；访客连接服务器对外
// 端口后由 frps 转发进隧道。TCP/UDP 均支持。
//
// 配置经 Host API KV 持久化：bind_port（frps 控制端口，默认 7000）、
// server_addr（生成 frpc 配置时对外展示的地址）、auth_token（首次自动生成）。
package main

import (
	"context"
	"crypto/rand"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/lmy8848/easyserver-plugin-sdk-go"

	v1 "github.com/fatedier/frp/pkg/config/v1"
)

//go:embed web
var webFS embed.FS

const (
	cfgBindPort  = "bind_port"
	cfgServerArg = "server_addr"
	defaultPort  = 7000
)

var (
	mgr      = NewFRPSManager()
	host     = sdk.NewHostClient()
	cfgCache = &settingsCache{}
)

// ---- 配置 ----

// settingsCache 缓存面板 KV 配置（短 TTL），避免每个请求都经 Host API 全量拉取；
// 插件自身写入后调用 invalidate 主动失效。
type settingsCache struct {
	mu   sync.Mutex
	at   time.Time
	data map[string]string
}

func (c *settingsCache) load(ctx context.Context) (map[string]string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.data != nil && time.Since(c.at) < 5*time.Second {
		return c.data, nil
	}
	all, err := host.ConfigAll(ctx)
	if err != nil {
		return nil, err
	}
	c.data = all
	c.at = time.Now()
	return all, nil
}

func (c *settingsCache) invalidate() {
	c.mu.Lock()
	c.data = nil
	c.mu.Unlock()
}

// settings 是插件运行配置的解析结果。
type settings struct {
	bindPort   int    // frps 控制端口
	serverAddr string // 生成 frpc 配置时的对外地址
	token      string // frp 认证 token（自动生成并持久化）
}

// getSettings 读取并解析配置；auth_token 缺失时生成并持久化（fail-fast，失败即报错）。
func getSettings(ctx context.Context) (*settings, error) {
	all, err := cfgCache.load(ctx)
	if err != nil {
		return nil, err
	}
	s := &settings{serverAddr: all[cfgServerArg], token: all["auth_token"]}
	if raw := strings.TrimSpace(all[cfgBindPort]); raw != "" {
		n, err := parsePort(raw)
		if err != nil {
			return nil, fmt.Errorf("bind_port 无效: %v", err)
		}
		s.bindPort = n
	} else {
		s.bindPort = defaultPort
	}
	if s.token == "" {
		t, err := generateToken()
		if err != nil {
			return nil, fmt.Errorf("生成 auth_token 失败: %w", err)
		}
		if err := host.ConfigSet(ctx, "auth_token", t); err != nil {
			return nil, fmt.Errorf("持久化 auth_token 失败: %w", err)
		}
		s.token = t
		cfgCache.invalidate()
	}
	return s, nil
}

// generateToken 生成 24 字节随机 hex token；随机源失败直接报错，不用兜底字符串。
func generateToken() (string, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// parsePort 解析 1-65535 的端口。
func parsePort(s string) (int, error) {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil || n < 1 || n > 65535 {
		return 0, fmt.Errorf("端口 %q 无效（需 1-65535 的整数）", s)
	}
	return n, nil
}

// startFRPS 以当前配置启动（或重启）嵌入 frps。
func startFRPS(ctx context.Context) error {
	s, err := getSettings(ctx)
	if err != nil {
		return err
	}
	cfg := v1.ServerConfig{
		BindAddr: "0.0.0.0",
		BindPort: s.bindPort,
	}
	cfg.Auth.Token = s.token
	return mgr.Start(ctx, cfg)
}

// ---- 路由 ----

func registerRoutes(srv *sdk.Server) {
	srv.HandleFunc("/", uiHandler)
	srv.HandleFunc("/api/status", statusHandler)
	srv.HandleFunc("/api/settings", settingsHandler)
	srv.HandleFunc("/api/frpc-config", frpcConfigHandler)
	srv.HandleFunc("/api/restart", restartHandler)
}

func writeJSON(w http.ResponseWriter, code int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(body)
}

func statusHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	s, err := getSettings(ctx)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"running":      mgr.Running(),
		"bind_port":    s.bindPort,
		"server_addr":  s.serverAddr,
		"token_masked": mask(s.token),
	})
}

func settingsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	var req struct {
		BindPort   string `json:"bind_port"`
		ServerAddr string `json:"server_addr"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "请求体无效"})
		return
	}
	if v := strings.TrimSpace(req.BindPort); v != "" {
		if _, err := parsePort(v); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		if err := host.ConfigSet(r.Context(), cfgBindPort, v); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
	}
	if v := strings.TrimSpace(req.ServerAddr); v != "" {
		if err := host.ConfigSet(r.Context(), cfgServerArg, v); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
	}
	cfgCache.invalidate()
	if err := startFRPS(r.Context()); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	_ = host.Audit(r.Context(), "frps 配置已更新并重启")
	writeJSON(w, http.StatusOK, map[string]string{"message": "配置已保存，frps 已重启"})
}

var proxyNameRe = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,32}$`)

// validateFrpcParams 校验并解析 frpc 配置生成参数，非法输入一律拒绝。
func validateFrpcParams(q url.Values) (name, proto string, localPort, remotePort int, err error) {
	name = q.Get("name")
	if name == "" {
		name = "mc"
	}
	if !proxyNameRe.MatchString(name) {
		return "", "", 0, 0, fmt.Errorf("代理名称无效：仅字母/数字/下划线/横线，≤32 字符")
	}
	proto = q.Get("type")
	if proto == "" {
		proto = "tcp"
	}
	if proto != "tcp" && proto != "udp" {
		return "", "", 0, 0, fmt.Errorf("协议 %q 无效（仅支持 tcp/udp）", proto)
	}
	remotePort, err = parsePort(q.Get("remote_port"))
	if err != nil {
		return "", "", 0, 0, fmt.Errorf("对外端口无效: %v", err)
	}
	if q.Get("local_port") == "" {
		localPort = remotePort
	} else if localPort, err = parsePort(q.Get("local_port")); err != nil {
		return "", "", 0, 0, fmt.Errorf("本地端口无效: %v", err)
	}
	return name, proto, localPort, remotePort, nil
}

func frpcConfigHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	s, err := getSettings(ctx)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if s.serverAddr == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "请先在设置中填写对外地址（服务器公网 IP 或域名）"})
		return
	}
	name, proto, localPort, remotePort, err := validateFrpcParams(r.URL.Query())
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	cfg := fmt.Sprintf(`# EasyServer 内网穿透客户端配置（tunnel-frp 插件生成）
# 在家庭电脑运行： frpc -c frpc.toml
serverAddr = "%s"
serverPort = %d
auth.token = "%s"

[[proxies]]
name = "%s"
type = "%s"
localIP = "127.0.0.1"
localPort = %d
remotePort = %d
`, s.serverAddr, s.bindPort, s.token, name, proto, localPort, remotePort)

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="frpc.toml"`)
	_, _ = w.Write([]byte(cfg))
}

func restartHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	if err := startFRPS(r.Context()); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	_ = host.Audit(r.Context(), "frps 已手动重启")
	writeJSON(w, http.StatusOK, map[string]string{"message": "frps 已重启"})
}

func uiHandler(w http.ResponseWriter, _ *http.Request) {
	data, err := webFS.ReadFile("web/index.html")
	if err != nil {
		http.Error(w, "页面资源缺失", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(data)
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// 启动即拉起 frps（配置缺失时用默认值）
	if err := startFRPS(ctx); err != nil {
		_ = host.Log(ctx, "frps 启动失败: "+err.Error())
	}

	srv := sdk.NewServer(sdk.Config{SocketPath: "tunnel-frp.sock"})
	registerRoutes(srv)

	go func() {
		<-ctx.Done()
		mgr.Stop()
		_ = srv.Close()
	}()

	if err := srv.Serve(); err != nil {
		_ = host.Log(context.Background(), "插件服务退出: "+err.Error())
		os.Exit(1)
	}
}

func mask(s string) string {
	if len(s) <= 8 {
		return "****"
	}
	return s[:4] + "****" + s[len(s)-4:]
}

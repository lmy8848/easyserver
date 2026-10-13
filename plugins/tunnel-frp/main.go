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
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
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

	frptypes "github.com/fatedier/frp/pkg/config/types"
	v1 "github.com/fatedier/frp/pkg/config/v1"
	sdk "github.com/lmy8848/easyserver-plugin-sdk-go"
)

//go:embed web
var webFS embed.FS

const (
	cfgBindPort        = "bind_port"
	cfgServerArg       = "server_addr"
	cfgVhostHTTPPort   = "vhost_http_port"
	cfgSubdomainHost   = "subdomain_host"
	cfgAllowPortsStart = "allow_ports_start"
	cfgAllowPortsEnd   = "allow_ports_end"
	defaultPort        = 7000
	defaultVhostPort   = 8081
	defaultAllowStart  = 40000
	defaultAllowEnd    = 50000
)

var (
	mgr        = NewFRPSManager()
	host       = sdk.NewHostClient()
	cfgCache   = &settingsCache{}
	mainCancel context.CancelFunc
)

// requestRestart 触发插件进程级重启：短暂延迟后取消主 context，插件优雅
// 退出（释放全部端口），由 supervisor 按新配置重新拉起。
// 采用进程级而非 frps 进程内重启：frp v0.71.0 的 Close 不释放 vhost HTTP
// 端口监听器（NewService 里 net.Listen 后未跟踪），进程内重启会泄漏端口。
func requestRestart() {
	if mainCancel == nil {
		return
	}
	go func() {
		time.Sleep(500 * time.Millisecond) // 等当前响应刷出
		mainCancel()
	}()
}

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
	bindPort        int    // frps 控制端口
	serverAddr      string // 生成 frpc 配置时的对外地址
	token           string // frp 认证 token（自动生成并持久化）
	vhostHTTPPort   int    // HTTP vhost 端口（域名路由，0=不启用）
	subdomainHost   string // HTTP 子域父域名（如 tunnel.0xdead.icu）
	allowPortsStart int    // TCP/UDP 暴露端口段（frps AllowPorts 白名单）
	allowPortsEnd   int
}

// getSettings 读取并解析配置；auth_token 缺失时生成并持久化（fail-fast，失败即报错）。
func getSettings(ctx context.Context) (*settings, error) {
	all, err := cfgCache.load(ctx)
	if err != nil {
		return nil, err
	}
	s := &settings{
		serverAddr:      all[cfgServerArg],
		token:           all["auth_token"],
		subdomainHost:   all[cfgSubdomainHost],
		vhostHTTPPort:   defaultVhostPort,
		allowPortsStart: defaultAllowStart,
		allowPortsEnd:   defaultAllowEnd,
	}
	if raw := strings.TrimSpace(all[cfgBindPort]); raw != "" {
		n, err := parsePort(raw)
		if err != nil {
			return nil, fmt.Errorf("bind_port 无效: %w", err)
		}
		s.bindPort = n
	} else {
		s.bindPort = defaultPort
	}
	if raw := strings.TrimSpace(all[cfgVhostHTTPPort]); raw != "" {
		if n, err := parsePort(raw); err == nil {
			s.vhostHTTPPort = n
		}
	}
	if raw := strings.TrimSpace(all[cfgAllowPortsStart]); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n >= 1 && n < 65535 {
			s.allowPortsStart = n
		}
	}
	if raw := strings.TrimSpace(all[cfgAllowPortsEnd]); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > s.allowPortsStart && n <= 65535 {
			s.allowPortsEnd = n
		}
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
	if s.vhostHTTPPort > 0 {
		cfg.VhostHTTPPort = s.vhostHTTPPort // HTTP 域名路由端口（http 类型隧道）
	}
	cfg.SubDomainHost = s.subdomainHost
	if s.allowPortsStart > 0 && s.allowPortsEnd >= s.allowPortsStart {
		// 限制 frpc 只能申请高位段端口，降低暴露端口可预测性
		cfg.AllowPorts = []frptypes.PortsRange{{Start: s.allowPortsStart, End: s.allowPortsEnd}}
	}
	// Complete() 补全默认值：尤其把 Auth.Method 默认为 "token"——若不调用，
	// NewAuthVerifier 会因 Method 为空返回 nil，服务器拒绝一切 frpc 登录
	// （现象为 "EOF"/"broken pipe"）。
	if err := cfg.Complete(); err != nil {
		return fmt.Errorf("frps 配置补全失败: %w", err)
	}
	// frps 生命周期由管理器自有 context 管理，不随请求 ctx 结束
	return mgr.Start(cfg) //nolint:contextcheck
}

// ---- 路由 ----

func registerRoutes(srv *sdk.Server) {
	srv.HandleFunc("/", uiHandler)
	srv.HandleFunc("/api/status", statusHandler)
	srv.HandleFunc("/api/settings", settingsHandler)
	srv.HandleFunc("/api/frpc-config", frpcConfigHandler)
	srv.HandleFunc("/api/restart", restartHandler)
	srv.HandleFunc("/api/expose", exposeHandler)
	srv.HandleFunc("/api/revoke", revokeHandler)
	srv.HandleFunc("/api/ports", portsHandler)
	// 前端样式贡献（styles 贡献点）：提供 web/styles/ 下的 CSS 供面板注入
	srv.HandleFunc("/styles/", stylesHandler)
	// 可被调用的命令（commands 贡献点）：其他插件经面板 command/invoke 调用
	srv.HandleFunc("/api/commands/restart", commandRestartHandler)
	// 数据源（datasources 贡献点）：面板经 /api/plugins/<id>/datasources/<id> 聚合代理
	srv.HandleFunc("/datasources/status", datasourceStatusHandler)
}

// commandRestartHandler 是插件声明的 restart 命令：重启 frps。
// 经面板 Host API command/invoke 由其他插件调用（本插件自身不可调）。
func commandRestartHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	_ = host.Audit(r.Context(), "command.invoke: 重启 frps")
	requestRestart() // 进程级重启
	writeJSON(w, http.StatusOK, map[string]string{"message": "frps 将重启（插件进程）"})
}

// datasourceStatusHandler 是插件声明的 status 数据源（datasources 贡献点，
// kind=json）：对外暴露 frps 运行状态，供面板/其他插件聚合消费。
// 面板经 /api/plugins/tunnel-frp/datasources/status 代理获取。
func datasourceStatusHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	out := map[string]any{
		"running":   mgr.Running(),
		"bind_port": 0,
	}
	if s, err := getSettings(ctx); err == nil {
		out["bind_port"] = s.bindPort
		out["vhost_http_port"] = s.vhostHTTPPort
		out["subdomain_host"] = s.subdomainHost
		out["allow_ports"] = []int{s.allowPortsStart, s.allowPortsEnd}
	}
	writeJSON(w, http.StatusOK, out)
}

// stylesHandler 提供插件声明的前端样式资源（styles 贡献点）。
// 面板经 /api/plugins/<id>/styles/<file> 代理获取后注入 <link>。
func stylesHandler(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/styles/")
	if name == "" || strings.Contains(name, "..") {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	data, err := webFS.ReadFile("web/styles/" + name)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/css; charset=utf-8")
	_, _ = w.Write(data)
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
	out := map[string]any{
		"running":         mgr.Running(),
		"bind_port":       s.bindPort,
		"server_addr":     s.serverAddr,
		"vhost_http_port": s.vhostHTTPPort,
		"subdomain_host":  s.subdomainHost,
		"allow_ports":     []int{s.allowPortsStart, s.allowPortsEnd},
		"token_masked":    mask(s.token),
	}
	// 服务器负载（经 panel.api.proxy 只读代理面板监控，需声明并批准该权限）
	var resp struct {
		Data struct {
			CPU struct {
				UsagePercent float64 `json:"usage_percent"`
			} `json:"cpu"`
			Memory struct {
				UsagePercent float64 `json:"usage_percent"`
			} `json:"memory"`
		} `json:"data"`
	}
	if err := host.PanelGet(ctx, "/api/monitor/stats", &resp); err == nil {
		out["server_load"] = map[string]any{"cpu_percent": resp.Data.CPU.UsagePercent, "mem_percent": resp.Data.Memory.UsagePercent}
	}
	writeJSON(w, http.StatusOK, out)
}

func settingsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	var req struct {
		BindPort        string `json:"bind_port"`
		ServerAddr      string `json:"server_addr"`
		VhostHTTPPort   string `json:"vhost_http_port"`
		SubdomainHost   string `json:"subdomain_host"`
		AllowPortsStart string `json:"allow_ports_start"`
		AllowPortsEnd   string `json:"allow_ports_end"`
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
	if v := strings.TrimSpace(req.VhostHTTPPort); v != "" {
		if _, err := parsePort(v); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "vhost_http_port " + err.Error()})
			return
		}
		if err := host.ConfigSet(r.Context(), cfgVhostHTTPPort, v); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
	}
	if v := strings.TrimSpace(req.SubdomainHost); v != "" {
		if err := host.ConfigSet(r.Context(), cfgSubdomainHost, v); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
	}
	if v := strings.TrimSpace(req.AllowPortsStart); v != "" {
		if err := host.ConfigSet(r.Context(), cfgAllowPortsStart, v); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
	}
	if v := strings.TrimSpace(req.AllowPortsEnd); v != "" {
		if err := host.ConfigSet(r.Context(), cfgAllowPortsEnd, v); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
	}
	cfgCache.invalidate()
	_ = host.Audit(r.Context(), "frps 配置已更新，插件将重启")
	_ = host.Log(r.Context(), "frps 配置已更新，插件将重启")
	requestRestart() // 进程级重启（frp vhost 端口泄漏无法进程内重启）
	writeJSON(w, http.StatusOK, map[string]string{"message": "配置已保存，插件将重启"})
}

var proxyNameRe = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,32}$`)

// frpcParams 是 frpc 配置生成参数的解析结果。
type frpcParams struct {
	name       string
	proto      string // tcp | udp | http
	localPort  int
	remotePort int    // tcp/udp 对外端口；http 为 0
	domain     string // http 自定义域名
	httpUser   string // http basicAuth 账号
	httpPass   string
}

// validateFrpcParams 校验并解析 frpc 配置生成参数，非法输入一律拒绝。
// tcp/udp 未指定 remote_port 时从允许段随机选高位端口。
func validateFrpcParams(q url.Values, allowStart, allowEnd int) (*frpcParams, error) {
	p := &frpcParams{name: q.Get("name")}
	if p.name == "" {
		p.name = "mc"
	}
	if !proxyNameRe.MatchString(p.name) {
		return nil, errors.New("代理名称无效：仅字母/数字/下划线/横线，≤32 字符")
	}
	p.proto = q.Get("type")
	if p.proto == "" {
		p.proto = "tcp"
	}
	var err error
	switch p.proto {
	case "tcp", "udp":
		if q.Get("remote_port") == "" {
			if p.remotePort, err = randomPortInRange(allowStart, allowEnd); err != nil {
				return nil, fmt.Errorf("随机对外端口失败: %w", err)
			}
		} else if p.remotePort, err = parsePort(q.Get("remote_port")); err != nil {
			return nil, fmt.Errorf("对外端口无效: %w", err)
		}
		if q.Get("local_port") == "" {
			p.localPort = p.remotePort
		} else if p.localPort, err = parsePort(q.Get("local_port")); err != nil {
			return nil, fmt.Errorf("本地端口无效: %w", err)
		}
	case "http":
		p.domain = strings.TrimSpace(q.Get("domain"))
		if p.domain == "" {
			return nil, errors.New("http 隧道需填写自定义域名（custom domain）")
		}
		if p.localPort, err = parsePort(q.Get("local_port")); err != nil {
			return nil, fmt.Errorf("本地端口无效: %w", err)
		}
		p.httpUser = strings.TrimSpace(q.Get("http_user"))
		p.httpPass = q.Get("http_pass")
		if (p.httpUser == "") != (p.httpPass == "") {
			return nil, errors.New("HTTP 访问账号与密码需同时填写（basicAuth）")
		}
	default:
		return nil, fmt.Errorf("协议 %q 无效（仅支持 tcp/udp/http）", p.proto)
	}
	return p, nil
}

// randomPortInRange 在 [start,end] 内生成随机高位端口。
func randomPortInRange(start, end int) (int, error) {
	span := end - start + 1
	if span <= 0 || span > 65535 || start < 1 || end > 65535 {
		return 0, errors.New("端口段配置无效")
	}
	var b [2]byte
	if _, err := rand.Read(b[:]); err != nil {
		return 0, err
	}
	return start + int(binary.BigEndian.Uint16(b[:]))%span, nil
}

// buildFrpcConfig 按协议生成 frpc.toml 文本。
func buildFrpcConfig(s *settings, p *frpcParams) string {
	out := fmt.Sprintf(`# EasyServer 内网穿透客户端配置（tunnel-frp 插件生成）
# 在家庭电脑运行： frpc -c frpc.toml
serverAddr = "%s"
serverPort = %d
auth.token = "%s"

[[proxies]]
name = "%s"
type = "%s"
localIP = "127.0.0.1"
localPort = %d
`, s.serverAddr, s.bindPort, s.token, p.name, p.proto, p.localPort)
	if p.proto == "http" {
		out += fmt.Sprintf("customDomains = [\"%s\"]\n", p.domain)
		if p.httpUser != "" {
			out += fmt.Sprintf("httpUser = \"%s\"\nhttpPassword = \"%s\"\n", p.httpUser, p.httpPass)
		}
	} else {
		out += fmt.Sprintf("remotePort = %d\n", p.remotePort)
	}
	return out
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
	p, err := validateFrpcParams(r.URL.Query(), s.allowPortsStart, s.allowPortsEnd)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	cfg := buildFrpcConfig(s, p)

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="frpc.toml"`)
	_, _ = w.Write([]byte(cfg))
}

func restartHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	_ = host.Audit(r.Context(), "frps 手动重启（进程级）")
	_ = host.Log(r.Context(), "frps 手动重启（进程级）")
	requestRestart()
	writeJSON(w, http.StatusOK, map[string]string{"message": "插件将重启"})
}

// ---- 暴露端口访问控制（经面板 Host API，需 firewall.manage 权限）----

// exposeHandler 为暴露的 TCP/UDP 端口应用访问控制白名单：
// 仅列出的授权源 IP/CIDR 可访问，其余来源由面板防火墙 DROP。
func exposeHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	var req struct {
		Port     int      `json:"port"`
		Protocol string   `json:"protocol"`
		CIDRs    []string `json:"cidrs"`
		Remark   string   `json:"remark"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "请求体无效"})
		return
	}
	if err := host.ExposePort(r.Context(), req.Port, req.Protocol, req.CIDRs, req.Remark); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	_ = host.Audit(r.Context(), fmt.Sprintf("暴露端口 %d/%s 白名单已应用", req.Port, req.Protocol))
	writeJSON(w, http.StatusOK, map[string]string{"message": "端口白名单已应用"})
}

// revokeHandler 移除暴露端口的访问控制。
func revokeHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	var req struct {
		Port     int    `json:"port"`
		Protocol string `json:"protocol"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "请求体无效"})
		return
	}
	if err := host.RevokePort(r.Context(), req.Port, req.Protocol); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	_ = host.Audit(r.Context(), fmt.Sprintf("移除暴露端口 %d/%s 白名单", req.Port, req.Protocol))
	writeJSON(w, http.StatusOK, map[string]string{"message": "端口白名单已移除"})
}

// portsHandler 列出当前受面板管控的暴露端口。
func portsHandler(w http.ResponseWriter, r *http.Request) {
	items, err := host.ListPorts(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, items)
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
	// requestRestart 通过取消此 context 触发进程级重启（由 supervisor 重新拉起）
	ctx, mainCancel = context.WithCancel(ctx)

	// 启动即拉起 frps（配置缺失时用默认值）
	if err := startFRPS(ctx); err != nil {
		_ = host.Log(ctx, "frps 启动失败: "+err.Error())
	} else if s, e := getSettings(ctx); e == nil {
		_ = host.Log(ctx, fmt.Sprintf("frps 已启动，控制端口 %d", s.bindPort))
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

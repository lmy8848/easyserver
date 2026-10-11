// tunnel-frp 插件：基于 frp 的内网穿透（家庭网络服务经服务器对外暴露）。
//
// 架构：插件进程内嵌 frps（frp 服务端库），家庭电脑跑 frpc（客户端配置
// 由本插件生成）主动连接本服务器的 frps 控制端口；访客连接服务器对外
// 端口后由 frps 转发进隧道。TCP/UDP 均支持。
//
// 配置经 Host API KV 持久化：bind_port（frps 控制端口，默认 7000）、
// server_addr（生成 frpc 配置时对外展示的地址）。
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/lmy8848/easyserver-plugin-sdk-go"

	v1 "github.com/fatedier/frp/pkg/config/v1"
)

const (
	cfgBindPort  = "bind_port"
	cfgServerArg = "server_addr"
	defaultPort  = "7000"
)

var (
	mgr    = NewFRPSManager()
	host   = sdk.NewHostClient()
	plugin string
)

func main() {
	plugin = os.Getenv(sdk.EnvPluginID)
	if plugin == "" {
		plugin = "tunnel-frp"
	}

	srv := sdk.NewServer(sdk.Config{SocketPath: "tunnel-frp.sock"})
	registerRoutes(srv)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// 启动即拉起 frps（配置缺失时用默认值）
	if err := startFRPS(ctx); err != nil {
		host.Log(ctx, "frps 启动失败: "+err.Error())
	}

	go func() {
		<-ctx.Done()
		mgr.Stop()
		_ = srv.Close()
	}()

	if err := srv.Serve(); err != nil {
		host.Log(context.Background(), "插件服务退出: "+err.Error())
		os.Exit(1)
	}
}

// ---- 配置 ----

func getSettings(ctx context.Context) (bindPort, serverAddr, token string, err error) {
	all, err := host.ConfigAll(ctx)
	if err != nil {
		return "", "", "", err
	}
	bindPort = all[cfgBindPort]
	if bindPort == "" {
		bindPort = defaultPort
	}
	serverAddr = all[cfgServerArg]
	token = all["auth_token"]
	if token == "" {
		token = generateToken()
		_ = host.ConfigSet(ctx, "auth_token", token)
	}
	return bindPort, serverAddr, token, nil
}

func generateToken() string {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "easyserver-frp-token"
	}
	return hex.EncodeToString(b)
}

// startFRPS 以当前配置启动嵌入 frps。
func startFRPS(ctx context.Context) error {
	bindPort, _, token, err := getSettings(ctx)
	if err != nil {
		return err
	}
	port := 0
	_, _ = fmt.Sscanf(bindPort, "%d", &port)
	cfg := v1.ServerConfig{
		BindAddr: "0.0.0.0",
		BindPort: port,
	}
	cfg.Auth.Token = token
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
	bindPort, serverAddr, token, err := getSettings(ctx)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"running":     mgr.Running(),
		"bind_port":   bindPort,
		"server_addr": serverAddr,
		"token_masked": mask(token),
	})
}

func mask(s string) string {
	if len(s) <= 8 {
		return "****"
	}
	return s[:4] + "****" + s[len(s)-4:]
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
	if req.BindPort != "" {
		if err := host.ConfigSet(r.Context(), cfgBindPort, req.BindPort); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
	}
	if req.ServerAddr != "" {
		if err := host.ConfigSet(r.Context(), cfgServerArg, req.ServerAddr); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
	}
	if err := startFRPS(r.Context()); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	_ = host.Audit(r.Context(), "frps 配置已更新并重启")
	writeJSON(w, http.StatusOK, map[string]string{"message": "配置已保存，frps 已重启"})
}

func frpcConfigHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	bindPort, serverAddr, token, err := getSettings(ctx)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if serverAddr == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "请先在设置中填写对外地址（服务器公网 IP 或域名）"})
		return
	}
	remotePort := r.URL.Query().Get("remote_port")
	if remotePort == "" {
		remotePort = "25565"
	}
	name := r.URL.Query().Get("name")
	if name == "" {
		name = "mc"
	}
	localPort := r.URL.Query().Get("local_port")
	if localPort == "" {
		localPort = remotePort
	}
	proto := r.URL.Query().Get("type")
	if proto == "" {
		proto = "tcp"
	}

	cfg := fmt.Sprintf(`# EasyServer 内网穿透客户端配置（tunnel-frp 插件生成）
# 在家庭电脑运行： frpc -c frpc.toml
serverAddr = "%s"
serverPort = %s
auth.token = "%s"

[[proxies]]
name = "%s"
type = "%s"
localIP = "127.0.0.1"
localPort = %s
remotePort = %s
`, serverAddr, bindPort, token, name, proto, localPort, remotePort)

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
	const page = `<!DOCTYPE html>
<html lang="zh">
<head>
<meta charset="utf-8">
<title>内网穿透</title>
<script src="/host-sdk.js"></script>
<style>
  body { font-family: system-ui, sans-serif; max-width: 720px; margin: 24px auto; padding: 0 16px; color: #222; }
  .card { border: 1px solid #ddd; border-radius: 8px; padding: 16px; margin-bottom: 16px; }
  h2 { margin-top: 0; }
  input { width: 100%; padding: 6px; margin: 4px 0 12px; box-sizing: border-box; }
  button { padding: 8px 16px; cursor: pointer; }
  pre { background: #f5f5f5; padding: 12px; border-radius: 6px; overflow-x: auto; }
  .ok { color: #16a34a; } .bad { color: #dc2626; }
</style>
</head>
<body>
<h2>内网穿透（frp）</h2>
<div class="card">
  <h3>服务状态</h3>
  <div id="status">加载中…</div>
  <button onclick="doRestart()">重启 frps</button>
</div>
<div class="card">
  <h3>服务端设置</h3>
  <label>frps 控制端口</label><input id="bind_port">
  <label>对外地址（生成客户端配置用）</label><input id="server_addr" placeholder="服务器的公网 IP 或域名">
  <button onclick="doSave()">保存并重启</button>
</div>
<div class="card">
  <h3>家庭电脑（frpc）配置生成</h3>
  <label>代理名称</label><input id="p_name" value="mc">
  <label>协议</label>
  <select id="p_type"><option value="tcp">TCP</option><option value="udp">UDP</option></select>
  <label>家庭电脑服务端口</label><input id="p_local" value="25565">
  <label>服务器对外端口</label><input id="p_remote" value="25565">
  <button onclick="downloadCfg()">下载 frpc.toml</button>
  <pre id="cfg">填写上方参数后点击下载</pre>
</div>
<script>
if (window.EasyServerPlugin) {
  var s = window.EasyServerPlugin;
  function loadStatus() {
    fetch('/api/plugins/tunnel-frp/api/status').then(function (r) { return r.json(); }).then(function (d) {
      if (!d) return;
      document.getElementById('status').innerHTML = d.running
        ? '<span class="ok">frps 运行中，控制端口 ' + d.bind_port + '</span>'
        : '<span class="bad">frps 未运行</span>';
      document.getElementById('bind_port').value = d.bind_port || '';
      document.getElementById('server_addr').value = d.server_addr || '';
    }).catch(function (e) { document.getElementById('status').textContent = '加载失败: ' + e.message; });
  }
  function doSave() {
    var b = document.getElementById('bind_port').value;
    var a = document.getElementById('server_addr').value;
    fetch('/api/plugins/tunnel-frp/api/settings', {
      method: 'POST', headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ bind_port: b, server_addr: a })
    }).then(function (r) { return r.json(); }).then(function (d) {
      if (d.error) throw new Error(d.error);
      loadStatus(); alert(d.message || '已保存');
    }).catch(function (e) { alert('保存失败: ' + (e.message || e)); });
  }
  function doRestart() {
    fetch('/api/plugins/tunnel-frp/api/restart', { method: 'POST' })
      .then(function (r) { return r.json(); })
      .then(function (d) { alert(d.message || d.error); loadStatus(); });
  }
  function downloadCfg() {
    var n = document.getElementById('p_name').value;
    var t = document.getElementById('p_type').value;
    var lp = document.getElementById('p_local').value;
    var rp = document.getElementById('p_remote').value;
    fetch('/api/plugins/tunnel-frp/api/frpc-config?name=' + encodeURIComponent(n) +
      '&type=' + t + '&local_port=' + lp + '&remote_port=' + rp)
      .then(function (r) { return r.text(); })
      .then(function (t2) { document.getElementById('cfg').textContent = t2; });
  }
  window.doSave = doSave; window.doRestart = doRestart; window.downloadCfg = downloadCfg;
  loadStatus();
} else {
  document.body.innerHTML = '<p>Host SDK 未加载（请通过面板访问）</p>';
}
</script>
</body>
</html>`
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(page))
}

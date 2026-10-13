package plugin

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"easyserver/internal/httpx/middleware"
)

// HostAPI 是面板向插件供给能力的 Host Socket 服务（文档 §7）。
// 鉴权：Bearer <plugin_token>；能力默认拒绝，逐端点校验权限。
type HostAPI struct {
	socketPath string
	mgr        *Manager
	storage    *PluginStorage
	srv        *http.Server
	ln         net.Listener
}

// NewHostAPI 创建 Host API 服务。
func NewHostAPI(socketPath string, mgr *Manager) *HostAPI {
	return &HostAPI{
		socketPath: socketPath,
		mgr:        mgr,
		storage:    mgr.Storage(),
	}
}

// Start 启动 host socket 监听（goroutine 内服务，随 Stop 结束）。
func (h *HostAPI) Start(ctx context.Context) error {
	if err := os.MkdirAll(filepath.Dir(h.socketPath), 0700); err != nil {
		return err
	}
	_ = os.Remove(h.socketPath) // 残留 socket 清理
	var lc net.ListenConfig
	ln, err := lc.Listen(ctx, "unix", h.socketPath)
	if err != nil {
		return err
	}
	h.ln = ln
	if err := os.Chmod(h.socketPath, 0600); err != nil {
		return err
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/host/", h.serve) // 统一入口：token 鉴权后按子路径分发
	h.srv = &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		if err := h.srv.Serve(ln); err != nil && err != http.ErrServerClosed {
			logManagerf(" host socket 服务异常退出: %v", err)
		}
	}()
	logManagerf(" Host API 已监听 %s", h.socketPath)
	return nil
}

// Stop 关闭 host socket 服务并清理 socket 文件。
func (h *HostAPI) Stop(ctx context.Context) error {
	if h.srv == nil {
		return nil
	}
	shutdownCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	err := h.srv.Shutdown(shutdownCtx)
	h.srv = nil
	_ = os.Remove(h.socketPath)
	return err
}

// serve 统一入口：token → 插件 → 能力分发。
func (h *HostAPI) serve(w http.ResponseWriter, r *http.Request) {
	token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	name, err := h.mgr.PluginNameByToken(token)
	if err != nil {
		unauthorized(w)
		return
	}
	sub := strings.TrimPrefix(r.URL.Path, "/api/host/")
	switch {
	case sub == "audit" && r.Method == http.MethodPost:
		h.handleAudit(w, r, name)
	case sub == "config" && r.Method == http.MethodGet:
		h.handleConfigGet(w, r, name)
	case sub == "config" && r.Method == http.MethodPut:
		h.handleConfigPut(w, r, name)
	case strings.HasPrefix(sub, "config/") && r.Method == http.MethodGet:
		h.handleConfigGetOne(w, r, name, strings.TrimPrefix(sub, "config/"))
	case strings.HasPrefix(sub, "log") && r.Method == http.MethodPost:
		h.handleLog(w, r, name)
	case sub == "event/publish" && r.Method == http.MethodPost:
		h.handleEventPublish(w, r, name)
	case sub == "exec" && r.Method == http.MethodPost:
		h.handleExec(w, r, name, token)
	case sub == "notify" && r.Method == http.MethodPost:
		h.handleNotify(w, r, name)
	case strings.HasPrefix(sub, "storage/"):
		h.handleStorage(w, r, name, sub)
	case sub == "port/expose" && r.Method == http.MethodPost:
		h.handlePortExpose(w, r, name)
	case sub == "port/revoke" && r.Method == http.MethodPost:
		h.handlePortRevoke(w, r, name)
	case sub == "port/list" && r.Method == http.MethodGet:
		h.handlePortList(w, r, name)
	case sub == "panel/api" && r.Method == http.MethodPost:
		h.handlePanelAPI(w, r, name)
	case sub == "command/invoke" && r.Method == http.MethodPost:
		h.handleCommandInvoke(w, r, name)
	default:
		notFound(w)
	}
}

// tokenOf 从请求头提取 Bearer token。
func tokenOf(r *http.Request) string {
	return strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
}

// execExitError 表示命令以非零码退出。
type execExitError struct{ code int }

func (e *execExitError) Error() string { return fmt.Sprintf("exit %d", e.code) }

// execCommand 执行命令并捕获合并输出；ExitError 转为带退出码的错误。
func execCommand(ctx context.Context, cmd string, args ...string) (string, error) {
	c := exec.CommandContext(ctx, cmd, args...)
	out, err := c.CombinedOutput()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return string(out), &execExitError{code: ee.ExitCode()}
		}
		return string(out), err
	}
	return string(out), nil
}

// hasPermission 校验插件是否声明了某能力（不写响应，由调用方统一处理）。
func (h *HostAPI) hasPermission(r *http.Request, perm string) bool {
	return h.mgr.HasPermission(r.Context(), tokenOf(r), perm)
}

// requirePermission 校验并在缺失时写 403；返回是否放行。
func (h *HostAPI) requirePermission(w http.ResponseWriter, r *http.Request, perm string) bool {
	if h.hasPermission(r, perm) {
		return true
	}
	forbidden(w, "缺少能力: "+perm)
	return false
}

// requireAnyPermission 校验插件声明了任一能力（OR 语义），缺失时写 403。
func (h *HostAPI) requireAnyPermission(w http.ResponseWriter, r *http.Request, perms ...string) bool {
	for _, p := range perms {
		if h.hasPermission(r, p) {
			return true
		}
	}
	forbidden(w, "缺少能力: "+strings.Join(perms, " 或 "))
	return false
}

// ---- 能力端点 ----

func (h *HostAPI) handleAudit(w http.ResponseWriter, r *http.Request, name string) {
	if !h.requirePermission(w, r, PermAuditWrite) {
		return
	}
	var req struct {
		Summary string `json:"summary" binding:"required"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		badRequest(w, "请求体无效")
		return
	}
	_ = h.mgr.AuditWrite(r.Context(), name, req.Summary)
	writeOK(w, respMap{"message": "已审计"})
}

func (h *HostAPI) handleConfigGet(w http.ResponseWriter, r *http.Request, name string) {
	if !h.requireAnyPermission(w, r, PermConfigRead, PermConfigReadWrite) {
		return
	}
	all, err := h.mgr.repo.ListConfig(r.Context(), name)
	if err != nil {
		internalError(w, err.Error())
		return
	}
	writeJSON(w, all)
}

func (h *HostAPI) handleConfigGetOne(w http.ResponseWriter, r *http.Request, name, key string) {
	if !h.requireAnyPermission(w, r, PermConfigRead, PermConfigReadWrite) {
		return
	}
	v, err := h.mgr.repo.GetConfig(r.Context(), name, key)
	if err != nil {
		notFound(w)
		return
	}
	writeJSON(w, respMap{"key": key, "value": v})
}

func (h *HostAPI) handleConfigPut(w http.ResponseWriter, r *http.Request, name string) {
	if !h.requirePermission(w, r, PermConfigReadWrite) {
		return
	}
	var body map[string]string
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		badRequest(w, "请求体无效")
		return
	}
	for k, v := range body {
		if err := h.mgr.repo.SetConfig(r.Context(), name, k, v); err != nil {
			internalError(w, err.Error())
			return
		}
	}
	writeOK(w, respMap{"message": "配置已保存"})
}

func (h *HostAPI) handleLog(w http.ResponseWriter, r *http.Request, name string) {
	if !h.requirePermission(w, r, PermLogWrite) {
		return
	}
	var req struct {
		Message string `json:"message"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	dir := "/var/log/easyserver/plugins"
	_ = os.MkdirAll(dir, 0750)
	f, err := os.OpenFile(filepath.Join(dir, name+".log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0640)
	if err != nil {
		internalError(w, err.Error())
		return
	}
	defer func() { _ = f.Close() }()
	fmt.Fprintf(f, "%s %s\n", time.Now().Format(time.RFC3339), req.Message)
	writeOK(w, respMap{"message": "已记录"})
}

func (h *HostAPI) handleEventPublish(w http.ResponseWriter, r *http.Request, name string) {
	if !h.requirePermission(w, r, PermEventPublish) {
		return
	}
	var req struct {
		Event   string          `json:"event" binding:"required"`
		Payload json.RawMessage `json:"payload"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		badRequest(w, "请求体无效")
		return
	}
	// 事件名必须以插件名为域（easyserver.<plugin>.<action>）
	if !strings.HasPrefix(req.Event, "easyserver."+name+".") {
		forbidden(w, "插件事件必须以 easyserver."+name+". 为域")
		return
	}
	h.mgr.Bus().Publish(req.Event, "plugin:"+name, req.Payload)
	writeOK(w, respMap{"message": "事件已发布"})
}

func (h *HostAPI) handleExec(w http.ResponseWriter, r *http.Request, name, token string) {
	if !h.requirePermission(w, r, PermExecRestricted) {
		return
	}
	// 文档 §7.3：exec.restricted 等同有审计+超时的 root 执行
	var req struct {
		Cmd     string   `json:"cmd" binding:"required"`
		Args    []string `json:"args"`
		Timeout int      `json:"timeout_seconds"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		badRequest(w, "请求体无效")
		return
	}
	timeout := time.Duration(req.Timeout) * time.Second
	if timeout <= 0 || timeout > 5*time.Minute {
		timeout = 30 * time.Second
	}
	_ = h.mgr.AuditWrite(r.Context(), name, "exec: "+req.Cmd+" "+strings.Join(req.Args, " "))

	ctx, cancel := context.WithTimeout(r.Context(), timeout)
	defer cancel()
	out, err := execCommand(ctx, req.Cmd, req.Args...)
	exitCode := 0
	if err != nil {
		var ee *execExitError
		if errors.As(err, &ee) {
			exitCode = ee.code
		} else {
			internalError(w, err.Error())
			return
		}
	}
	writeJSON(w, respMap{"exit_code": exitCode, "stdout_stderr": out})
}

func (h *HostAPI) handleNotify(w http.ResponseWriter, r *http.Request, name string) {
	if !h.requirePermission(w, r, PermNotifySend) {
		return
	}
	var req struct {
		Title   string `json:"title"`
		Content string `json:"content" binding:"required"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		badRequest(w, "请求体无效")
		return
	}
	if err := h.mgr.Notify(r.Context(), name, req.Title, req.Content); err != nil {
		internalError(w, err.Error())
		return
	}
	writeOK(w, respMap{"message": "通知已发送"})
}

func (h *HostAPI) handleStorage(w http.ResponseWriter, r *http.Request, name, sub string) {
	if !h.requirePermission(w, r, PermStorageReadWrite) {
		return
	}
	rel := strings.TrimPrefix(sub, "storage")
	rel = strings.TrimPrefix(rel, "/")
	switch r.Method {
	case http.MethodGet:
		data, err := h.storage.ReadFile(name, rel)
		if err != nil {
			notFound(w)
			return
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write(data)
	case http.MethodPut:
		var data []byte
		buf := make([]byte, 4096)
		for {
			n, err := r.Body.Read(buf)
			data = append(data, buf[:n]...)
			if len(data) > 64<<20 {
				payloadTooLarge(w, "文件超过 64MB 限制")
				return
			}
			if err != nil {
				break
			}
		}
		if err := h.storage.WriteFile(name, rel, data); err != nil {
			internalError(w, err.Error())
			return
		}
		writeOK(w, respMap{"message": "已写入"})
	case http.MethodDelete:
		if err := h.storage.Delete(name, rel); err != nil {
			notFound(w)
			return
		}
		writeOK(w, respMap{"message": "已删除"})
	default:
		notFound(w)
	}
}

// ---- 暴露端口访问控制（权限 firewall.manage，面板 PortACL）----

func (h *HostAPI) handlePortExpose(w http.ResponseWriter, r *http.Request, name string) {
	if !h.requirePermission(w, r, PermFirewallManage) {
		return
	}
	var req struct {
		Port     int      `json:"port"`
		Protocol string   `json:"protocol"`
		CIDRs    []string `json:"cidrs"`
		Remark   string   `json:"remark"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		badRequest(w, "请求体无效")
		return
	}
	if h.mgr.portExp == nil {
		internalError(w, "暴露端口访问控制未接线")
		return
	}
	if err := h.mgr.portExp.Expose(r.Context(), req.Port, req.Protocol, req.CIDRs, req.Remark); err != nil {
		badRequest(w, err.Error())
		return
	}
	_ = h.mgr.AuditWrite(r.Context(), name, fmt.Sprintf("port.expose %d/%s cidrs=%v", req.Port, req.Protocol, req.CIDRs))
	writeOK(w, respMap{"message": "端口白名单已应用"})
}

func (h *HostAPI) handlePortRevoke(w http.ResponseWriter, r *http.Request, name string) {
	if !h.requirePermission(w, r, PermFirewallManage) {
		return
	}
	var req struct {
		Port     int    `json:"port"`
		Protocol string `json:"protocol"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		badRequest(w, "请求体无效")
		return
	}
	if h.mgr.portExp == nil {
		internalError(w, "暴露端口访问控制未接线")
		return
	}
	if err := h.mgr.portExp.Revoke(r.Context(), req.Port, req.Protocol); err != nil {
		internalError(w, err.Error())
		return
	}
	_ = h.mgr.AuditWrite(r.Context(), name, fmt.Sprintf("port.revoke %d/%s", req.Port, req.Protocol))
	writeOK(w, respMap{"message": "端口白名单已移除"})
}

func (h *HostAPI) handlePortList(w http.ResponseWriter, r *http.Request, name string) {
	if !h.requirePermission(w, r, PermFirewallManage) {
		return
	}
	if h.mgr.portExp == nil {
		writeJSON(w, []any{})
		return
	}
	items, err := h.mgr.portExp.List(r.Context())
	if err != nil {
		internalError(w, err.Error())
		return
	}
	writeJSON(w, items)
}

// ---- 面板服务级 API 代理（权限 panel.api.proxy，只读白名单）----

// panelAPIReadAllowlist 是 panel.api.proxy v1 可代理的面板只读路径前缀
// （监控 / 审计 / 告警 / 安全）。插件只能调用白名单内的 GET，杜绝写入与
// 敏感操作面。
var panelAPIReadAllowlist = []string{
	"/api/monitor/",
	"/api/audit-logs",
	"/api/alerts/",
	"/api/security/",
}

func panelAPIAllowed(path string) bool {
	for _, p := range panelAPIReadAllowlist {
		if strings.HasPrefix(path, p) {
			return true
		}
	}
	return false
}

// handlePanelAPI 代理插件对面板业务 API 的只读调用（非用户态服务级）：
// 校验 panel.api.proxy 权限 + 方法 GET + 路径白名单，再带面板服务 token
// 自请求面板自身，转发响应并审计。
func (h *HostAPI) handlePanelAPI(w http.ResponseWriter, r *http.Request, name string) {
	if !h.requirePermission(w, r, PermPanelAPIProxy) {
		return
	}
	var req struct {
		Method string          `json:"method"`
		Path   string          `json:"path"`
		Body   json.RawMessage `json:"body"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		badRequest(w, "请求体无效")
		return
	}
	if req.Method != http.MethodGet {
		forbidden(w, "panel.api.proxy v1 仅支持只读 GET")
		return
	}
	if !panelAPIAllowed(req.Path) {
		forbidden(w, "该路径不在 panel.api.proxy 只读白名单内")
		return
	}
	cfg := h.mgr.panelAPI
	if cfg.BaseURL == "" || cfg.Token == "" {
		internalError(w, "面板服务 API 代理未接线")
		return
	}
	// 自请求面板自身（服务级，带服务 token + 规范域名 Host 避免重定向）
	httpReq, err := http.NewRequestWithContext(r.Context(), http.MethodGet, cfg.BaseURL+req.Path, nil)
	if err != nil {
		internalError(w, err.Error())
		return
	}
	if cfg.Domain != "" {
		httpReq.Header.Set("Host", cfg.Domain)
	}
	httpReq.Header.Set(middleware.ServiceTokenHeader, cfg.Token)
	httpReq.Header.Set("X-Panel-Plugin", name)
	tr := &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}} // loopback 自请求
	client := &http.Client{Transport: tr, Timeout: 15 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		internalError(w, "面板 API 代理请求失败: "+err.Error())
		return
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	_ = h.mgr.AuditWrite(r.Context(), name, "panel.api GET "+req.Path+" -> "+strconv.Itoa(resp.StatusCode))
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	_, _ = w.Write(data)
}

// ---- 跨插件命令调用（权限 command.invoke，文档 §7.2 /command/invoke）----

const (
	commandMaxDepth     = 3
	commandDepthHeader  = "X-Panel-Command-Depth"
	commandCallerHeader = "X-Panel-Command-Caller"
)

// handleCommandInvoke 代理插件调用另一插件的 command：校验权限 + 禁自调 +
// 深度限制（防递归）+ 目标声明校验，再经目标插件 socket 转发
// /api/commands/<id>，全程审计 + 超时。
func (h *HostAPI) handleCommandInvoke(w http.ResponseWriter, r *http.Request, caller string) {
	if !h.requirePermission(w, r, PermCommandInvoke) {
		return
	}
	var req struct {
		Plugin  string          `json:"plugin"`
		Command string          `json:"command"`
		Params  json.RawMessage `json:"params"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		badRequest(w, "请求体无效")
		return
	}
	if req.Plugin == caller {
		forbidden(w, "禁止插件调用自身命令")
		return
	}
	depth := 1
	if v := r.Header.Get(commandDepthHeader); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 1 {
			depth = n
		}
	}
	if depth > commandMaxDepth {
		forbidden(w, "命令调用深度超过限制")
		return
	}
	mft, err := h.mgr.GetManifest(r.Context(), req.Plugin)
	if err != nil {
		notFound(w)
		return
	}
	declared := false
	for _, c := range mft.Contributions.Commands {
		if c.ID == req.Command {
			declared = true
			break
		}
	}
	if !declared {
		notFound(w)
		return
	}
	sock, _, ok := h.mgr.sup.RunningEndpoint(req.Plugin)
	if !ok {
		internalError(w, "目标插件未运行")
		return
	}
	body, _ := json.Marshal(map[string]any{"params": req.Params})
	httpReq, err := http.NewRequestWithContext(r.Context(), http.MethodPost,
		"http://plugin/api/commands/"+url.PathEscape(req.Command), bytes.NewReader(body))
	if err != nil {
		internalError(w, err.Error())
		return
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set(commandDepthHeader, strconv.Itoa(depth+1))
	httpReq.Header.Set(commandCallerHeader, caller)
	tr := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, "unix", sock)
		},
	}
	client := &http.Client{Transport: tr, Timeout: 15 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		internalError(w, "命令调用失败: "+err.Error())
		return
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	_ = h.mgr.AuditWrite(r.Context(), caller, fmt.Sprintf("command.invoke %s/%s -> %d", req.Plugin, req.Command, resp.StatusCode))
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	_, _ = w.Write(data)
}

// ---- 小工具（避免依赖 httpx/gin 包，host socket 是独立 HTTP 栈）----

type respMap map[string]any

func writeOK(w http.ResponseWriter, body respMap) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(body)
}

func writeJSON(w http.ResponseWriter, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(body)
}

func badRequest(w http.ResponseWriter, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadRequest)
	_ = json.NewEncoder(w).Encode(respMap{"message": msg})
}

func unauthorized(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	_ = json.NewEncoder(w).Encode(respMap{"message": "unauthorized"})
}

func forbidden(w http.ResponseWriter, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusForbidden)
	_ = json.NewEncoder(w).Encode(respMap{"message": msg})
}

func notFound(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusNotFound)
	_ = json.NewEncoder(w).Encode(respMap{"message": "not found"})
}

func internalError(w http.ResponseWriter, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusInternalServerError)
	_ = json.NewEncoder(w).Encode(respMap{"message": msg})
}

func payloadTooLarge(w http.ResponseWriter, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusRequestEntityTooLarge)
	_ = json.NewEncoder(w).Encode(respMap{"message": msg})
}

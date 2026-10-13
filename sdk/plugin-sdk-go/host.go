package sdk

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

// HostClient 是面板 Host API 客户端（经 host Unix socket + Bearer token）。
//
// 全部方法默认 30s 超时；面板不可达时返回 error（插件应容忍并重试，
// 如面板重启期间）。
type HostClient struct {
	token      string
	hostSocket string
}

// NewHostClient 从面板注入的环境变量创建 Host API 客户端。
func NewHostClient() *HostClient {
	return &HostClient{
		token:      os.Getenv(EnvPluginToken),
		hostSocket: os.Getenv(EnvHostSocket),
	}
}

// Connected 报告 Host API 是否已接线（面板注入了 socket 路径）。
func (h *HostClient) Connected() bool {
	return h.hostSocket != ""
}

func (h *HostClient) client() *http.Client {
	tr := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, "unix", h.hostSocket)
		},
	}
	return &http.Client{Transport: tr, Timeout: 30 * time.Second}
}

// do 请求 Host API；out 非空时解码 JSON 响应体。
func (h *HostClient) do(ctx context.Context, method, path string, body any, out any) error {
	if h.hostSocket == "" {
		return fmt.Errorf("Host API 未接线（%s 缺失）", EnvHostSocket)
	}
	var rdr *bytes.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rdr = bytes.NewReader(b)
	} else {
		rdr = bytes.NewReader(nil)
	}
	req, err := http.NewRequestWithContext(ctx, method, "http://host/api/host/"+strings.TrimPrefix(path, "/"), rdr)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+h.token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := h.client().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		// 尝试解析面板错误信息
		var e struct {
			Message string `json:"message"`
		}
		if json.NewDecoder(resp.Body).Decode(&e) == nil && e.Message != "" {
			return fmt.Errorf("host api %s %s: %d %s", method, path, resp.StatusCode, e.Message)
		}
		return fmt.Errorf("host api %s %s: %d", method, path, resp.StatusCode)
	}
	if out != nil {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return nil
}

// ---- 审计 ----

// Audit 写面板审计日志。
func (h *HostClient) Audit(ctx context.Context, summary string) error {
	return h.do(ctx, http.MethodPost, "audit", map[string]string{"summary": summary}, nil)
}

// ---- 配置（KV）----

// ConfigGet 读取插件配置项（不存在时报错）。
func (h *HostClient) ConfigGet(ctx context.Context, key string) (string, error) {
	var out struct {
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

// ConfigAll 读取插件全部配置。
func (h *HostClient) ConfigAll(ctx context.Context) (map[string]string, error) {
	out := map[string]string{}
	if err := h.do(ctx, http.MethodGet, "config", nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// DeleteConfig 删除插件配置项。
func (h *HostClient) DeleteConfig(ctx context.Context, key string) error {
	if h.hostSocket == "" {
		return fmt.Errorf("Host API 未接线")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, "http://host/api/host/config/"+key, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+h.token)
	resp, err := h.client().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("delete config %s: %d", key, resp.StatusCode)
	}
	return nil
}

// GetInt 读取整数配置；缺失或非法返回 def。
func (h *HostClient) GetInt(ctx context.Context, key string, def int) int {
	v, err := h.ConfigGet(ctx, key)
	if err != nil {
		return def
	}
	n, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil {
		return def
	}
	return n
}

// GetBool 读取布尔配置；缺失或非法返回 def（"1"/"true"/"yes"/"on" 为真）。
func (h *HostClient) GetBool(ctx context.Context, key string, def bool) bool {
	v, err := h.ConfigGet(ctx, key)
	if err != nil {
		return def
	}
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	}
	return def
}

// ---- 日志 ----

// Log 写插件日志（面板侧 /var/log/easyserver/plugins/<id>.log）。
func (h *HostClient) Log(ctx context.Context, message string) error {
	return h.do(ctx, http.MethodPost, "log", map[string]string{"message": message}, nil)
}

// ---- 事件 ----

// PublishEvent 发布事件（事件名必须以 easyserver.<插件名>. 为域）。
func (h *HostClient) PublishEvent(ctx context.Context, event string, payload any) error {
	return h.do(ctx, http.MethodPost, "event/publish", map[string]any{"event": event, "payload": payload}, nil)
}

// ---- 命令执行 ----

// Exec 执行命令（有审计 + 超时的 root 执行，文档 §7.3 等同 root shell）。
func (h *HostClient) Exec(ctx context.Context, cmd string, args []string, timeoutSeconds int) (stdoutStderr string, exitCode int, err error) {
	var out struct {
		ExitCode     int    `json:"exit_code"`
		StdoutStderr string `json:"stdout_stderr"`
	}
	if err := h.do(ctx, http.MethodPost, "exec", map[string]any{"cmd": cmd, "args": args, "timeout_seconds": timeoutSeconds}, &out); err != nil {
		return "", -1, err
	}
	return out.StdoutStderr, out.ExitCode, nil
}

// ---- 通知 ----

// Notify 经面板通知通道发送消息。
func (h *HostClient) Notify(ctx context.Context, title, content string) error {
	return h.do(ctx, http.MethodPost, "notify", map[string]string{"title": title, "content": content}, nil)
}

// ---- 私有文件区 ----

func (h *HostClient) rawRequest(ctx context.Context, method, path string, body []byte) (*http.Response, error) {
	if h.hostSocket == "" {
		return nil, fmt.Errorf("Host API 未接线")
	}
	req, err := http.NewRequestWithContext(ctx, method, "http://host/api/host/storage/"+strings.TrimPrefix(path, "/"), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+h.token)
	if body != nil {
		req.Header.Set("Content-Type", "application/octet-stream")
	}
	return h.client().Do(req)
}

// StorageRead 读取插件私有文件。
func (h *HostClient) StorageRead(ctx context.Context, path string) ([]byte, error) {
	resp, err := h.rawRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("storage read %s: %d", path, resp.StatusCode)
	}
	buf := make([]byte, 0, 4096)
	tmp := make([]byte, 4096)
	for {
		n, err := resp.Body.Read(tmp)
		buf = append(buf, tmp[:n]...)
		if err != nil {
			break
		}
	}
	return buf, nil
}

// StorageWrite 写插件私有文件（自动创建父目录）。
func (h *HostClient) StorageWrite(ctx context.Context, path string, data []byte) error {
	resp, err := h.rawRequest(ctx, http.MethodPut, path, data)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("storage write %s: %d", path, resp.StatusCode)
	}
	return nil
}

// StorageDelete 删除插件私有文件或目录。
func (h *HostClient) StorageDelete(ctx context.Context, path string) error {
	resp, err := h.rawRequest(ctx, http.MethodDelete, path, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("storage delete %s: %d", path, resp.StatusCode)
	}
	return nil
}

// ---- 暴露端口访问控制（需 manifest 声明 firewall.manage 权限）----

// PortACL 是暴露端口的访问控制条目（面板 PortACL）。
type PortACL struct {
	Port     int      `json:"port"`
	Protocol string   `json:"protocol"`
	CIDRs    []string `json:"cidrs"`
	Remark   string   `json:"remark"`
}

// ExposePort 为暴露端口应用访问控制白名单：仅列出的授权源 IP/CIDR 可访问，
// 其余来源由面板防火墙 DROP。用于确保暴露端口只对管理员授权的人开放。
func (h *HostClient) ExposePort(ctx context.Context, port int, protocol string, cidrs []string, remark string) error {
	return h.do(ctx, http.MethodPost, "port/expose", map[string]any{
		"port": port, "protocol": protocol, "cidrs": cidrs, "remark": remark,
	}, nil)
}

// RevokePort 移除暴露端口的访问控制（撤销白名单后端口不再受面板管控）。
func (h *HostClient) RevokePort(ctx context.Context, port int, protocol string) error {
	return h.do(ctx, http.MethodPost, "port/revoke", map[string]any{
		"port": port, "protocol": protocol,
	}, nil)
}

// ListPorts 列出当前受面板管控的暴露端口及授权源。
func (h *HostClient) ListPorts(ctx context.Context) ([]PortACL, error) {
	var out []PortACL
	if err := h.do(ctx, http.MethodGet, "port/list", nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// ---- 面板服务级 API 代理（需 manifest 声明 panel.api.proxy 权限）----

// PanelAPI 调用面板业务 API（非用户态服务级）。v1 仅支持只读 GET，
// 且路径必须在面板白名单内（监控 / 审计 / 告警 / 安全）。
func (h *HostClient) PanelAPI(ctx context.Context, method, path string, body, out any) error {
	return h.do(ctx, http.MethodPost, "panel/api", map[string]any{
		"method": method, "path": path, "body": body,
	}, out)
}

// PanelGet 便捷方法：调用面板只读 API（白名单内 GET）。
func (h *HostClient) PanelGet(ctx context.Context, path string, out any) error {
	return h.PanelAPI(ctx, http.MethodGet, path, nil, out)
}

// ---- 跨插件命令调用（需 manifest 声明 command.invoke 权限）----

// InvokeCommand 调用另一插件声明的 command（经面板 Host API /command/invoke，
// 禁自调、深度限 3）。目标插件需在自身 socket 提供 /api/commands/<id> 端点。
func (h *HostClient) InvokeCommand(ctx context.Context, pluginName, command string, params, out any) error {
	return h.do(ctx, http.MethodPost, "command/invoke", map[string]any{
		"plugin": pluginName, "command": command, "params": params,
	}, out)
}

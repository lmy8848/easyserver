package sdk

import (
	"encoding/json"
	"os"
)

// 面板拉起插件时注入的环境变量（文档 §7.1）。插件进程直接读取。
const (
	// EnvPluginToken 插件调用 Host API 的 Bearer token。
	EnvPluginToken = "EASYSERVER_PLUGIN_TOKEN"
	// EnvHostSocket 面板 Host API 的 Unix socket 路径。
	EnvHostSocket = "EASYSERVER_HOST_SOCKET"
	// EnvPluginID 插件名（与 plugin.yaml name 一致）。
	EnvPluginID = "EASYSERVER_PLUGIN_ID"
	// EnvPluginSocket 插件应监听的绝对 socket 路径（Supervisor 探测此路径）。
	EnvPluginSocket = "EASYSERVER_PLUGIN_SOCKET"
)

// PluginID 返回当前插件名（EnvPluginID），缺失时回退为空串。
func PluginID() string {
	return os.Getenv(EnvPluginID)
}

// Event 是面板投递的事件（文档 §8）。订阅方必须幂等，并按 event_id 去重。
type Event struct {
	// ID 事件唯一 ID（去重用）。
	ID string `json:"event_id"`
	// Name 事件名（反向域名，如 easyserver.auth.login）。
	Name string `json:"event"`
	// Source 发布来源（如 auth、filemanager、plugin:<name>）。
	Source string `json:"source"`
	// Payload 事件载荷（JSON）。
	Payload json.RawMessage `json:"payload"`
	// Time 事件时间（RFC3339）。
	Time string `json:"time"`
}

// Decode 把事件载荷解码到目标结构；空载荷返回 nil。
func (e *Event) Decode(v any) error {
	if len(e.Payload) == 0 {
		return nil
	}
	return json.Unmarshal(e.Payload, v)
}

// Package plugin 实现面板的进程外插件框架（设计文档：docs/plugin-system.md，
// 终稿 v1.0，见 git 历史 3b33d12 / gh-pages 站点）。
//
// 本文件是契约层：manifest 结构、权限常量、扩展点类型与加载前校验。
// 设计要点（详见文档 §2/§5/§6/§11）：
//   - 插件是独立进程，经 Unix socket 与面板通信，崩溃不波及主面板
//   - 权限默认拒绝：manifest 声明能力，管理员批准后绑定 token scope
//   - 未知 contribution 类型忽略 + 告警（向前兼容）
package plugin

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"easyserver/internal/infra/errx"

	"gopkg.in/yaml.v3"
)

// API 版本协商范围：插件 manifest 的 api_version 落在区间内才可加载。
const (
	APIVersionMin = 1
	APIVersionMax = 1
)

// Permission 常量：Host API 能力（文档 §7.2），默认拒绝、按需声明。
const (
	PermAuditWrite       = "audit.write"
	PermConfigRead       = "config.read"
	PermConfigReadWrite  = "config.readwrite"
	PermLogWrite         = "log.write"
	PermEventPublish     = "event.publish"
	PermEventSubscribe   = "event.subscribe"
	PermExecRestricted   = "exec.restricted" // 等同授予 root shell，文档明示
	PermNotifySend       = "notify.send"
	PermPanelAPIProxy    = "panel.api.proxy"
	PermStorageReadWrite = "storage.readwrite"
)

// KnownPermissions 是全部已知能力（manifest 声明白名单之外一律拒绝加载）。
var KnownPermissions = map[string]bool{
	PermAuditWrite: true, PermConfigRead: true, PermConfigReadWrite: true,
	PermLogWrite: true, PermEventPublish: true, PermEventSubscribe: true,
	PermExecRestricted: true, PermNotifySend: true, PermPanelAPIProxy: true,
	PermStorageReadWrite: true,
}

// 插件生命周期状态（文档 §12.1）。
const (
	StateRegistered = "registered"
	StateEnabled    = "enabled"
	StateStarting   = "starting"
	StateRunning    = "running"
	StateDegraded   = "degraded"
	StateDisabled   = "disabled"
	StateError      = "error"
)

// 插件目录与 socket 约定（文档 §3.2/§17）。
const (
	// PluginsDir 插件安装根目录（root-only 可写）。
	PluginsDir = "/opt/easyserver/plugins"
	// SocketDir 运行时 socket 目录（0700，socket 0600，tmpfs 重启即清）。
	SocketDir = "/run/easyserver/plugins"
	// HostSocketPath 面板 Host API socket。
	HostSocketPath = "/run/easyserver/host.sock"
)

var (
	nameRe  = regexp.MustCompile(`^[a-z0-9-]+$`)
	eventRe = regexp.MustCompile(`^[a-z0-9]+(\.[a-z0-9_-]+)+$`)
)

// Runtime 是插件的进程拉取配置（文档 §6.1）。
type Runtime struct {
	Exec           string            `yaml:"exec"`
	Args           []string          `yaml:"args"`
	Env            map[string]string `yaml:"env"`
	Socket         string            `yaml:"socket"`
	StartupTimeout time.Duration     `yaml:"startup_timeout"`
	RequestTimeout time.Duration     `yaml:"request_timeout"`
}

// HealthConfig 是健康检查配置。
type HealthConfig struct {
	Path          string        `yaml:"path"`
	Interval      time.Duration `yaml:"interval"`
	FailThreshold int           `yaml:"fail_threshold"`
}

// Manifest 是插件清单（plugin.yaml 加载后的结构）。
type Manifest struct {
	APIVersion            int           `yaml:"api_version" json:"api_version"`
	ManifestSchemaVersion int           `yaml:"manifest_schema_version" json:"manifest_schema_version"`
	Name                  string        `yaml:"name" json:"name"`
	Version               string        `yaml:"version" json:"version"`
	Author                string        `yaml:"author" json:"author"`
	Description           string        `yaml:"description" json:"description"`
	Runtime               Runtime       `yaml:"runtime" json:"runtime"`
	Health                HealthConfig  `yaml:"health" json:"health"`
	Permissions           []string      `yaml:"permissions" json:"permissions"`
	Contributions         Contributions `yaml:"contributions" json:"contributions"`
}

// Contributions 是插件向各扩展点贡献的内容（v1 八种，文档 §5）。
// 未知字段由 yaml 解码自然忽略（向前兼容）。
type Contributions struct {
	Menus     []MenuContribution     `yaml:"menus" json:"menus,omitempty"`
	Pages     []PageContribution     `yaml:"pages" json:"pages,omitempty"`
	Routes    []RouteContribution    `yaml:"routes" json:"routes,omitempty"`
	Widgets   []WidgetContribution   `yaml:"widgets" json:"widgets,omitempty"`
	Settings  []SettingsContribution `yaml:"settings" json:"settings,omitempty"`
	Services  []ServiceContribution  `yaml:"services" json:"services,omitempty"`
	EventSubs []EventSubContribution `yaml:"eventsubs" json:"eventsubs,omitempty"`
	EventPubs []EventPubContribution `yaml:"eventpubs" json:"eventpubs,omitempty"`
}

type MenuContribution struct {
	ID    string `yaml:"id" json:"id"`
	Title string `yaml:"title" json:"title"`
	Icon  string `yaml:"icon" json:"icon"`
	Path  string `yaml:"path" json:"path"`
	Order int    `yaml:"order" json:"order"`
}

type PageContribution struct {
	ID   string `yaml:"id" json:"id"`
	Path string `yaml:"path" json:"path"`
}

type RouteContribution struct {
	ID     string `yaml:"id" json:"id"`
	Prefix string `yaml:"prefix" json:"prefix"`
}

type WidgetContribution struct {
	ID    string `yaml:"id" json:"id"`
	Title string `yaml:"title" json:"title"`
	Slot  string `yaml:"slot" json:"slot"`
	Path  string `yaml:"path" json:"path"`
	Size  string `yaml:"size" json:"size"`
}

type SettingsContribution struct {
	ID    string `yaml:"id" json:"id"`
	Title string `yaml:"title" json:"title"`
	// ConfigSchema 用 any 承载（YAML flow map 或 JSON 对象），输出时经
	// encoding/json 转为 JSON；[]byte 无法直接承接 YAML 嵌套 map。
	ConfigSchema any `yaml:"config_schema" json:"config_schema,omitempty"`
}

type ServiceContribution struct {
	ID       string `yaml:"id" json:"id"`
	Schedule string `yaml:"schedule" json:"schedule"`
	Command  string `yaml:"command" json:"command"`
}

type EventSubContribution struct {
	Event string `yaml:"event" json:"event"`
}

type EventPubContribution struct {
	Event  string `yaml:"event" json:"event"`
	Schema any    `yaml:"schema" json:"schema,omitempty"`
}

// ManifestDefaults 填充可省略字段的标准值。
func (m *Manifest) ManifestDefaults() {
	if m.Runtime.StartupTimeout == 0 {
		m.Runtime.StartupTimeout = 30 * time.Second
	}
	if m.Runtime.RequestTimeout == 0 {
		m.Runtime.RequestTimeout = 30 * time.Second
	}
	if m.Health.Path == "" {
		m.Health.Path = "/health"
	}
	if m.Health.Interval == 0 {
		m.Health.Interval = 30 * time.Second
	}
	if m.Health.FailThreshold == 0 {
		m.Health.FailThreshold = 3
	}
}

// Validate 按文档 §6.2 做加载前严格校验；无效清单一律拒绝启动。
func (m *Manifest) Validate() error {
	if !nameRe.MatchString(m.Name) || len(m.Name) > 64 {
		return errx.BadRequest("插件名无效：仅小写字母/数字/横线，≤64 字符")
	}
	if m.APIVersion < APIVersionMin || m.APIVersion > APIVersionMax {
		return errx.BadRequest("api_version %d 不受支持（面板支持 %d-%d）",
			m.APIVersion, APIVersionMin, APIVersionMax)
	}
	if strings.TrimSpace(m.Version) == "" {
		return errx.BadRequest("version 不能为空")
	}
	if m.Runtime.Exec == "" {
		return errx.BadRequest("runtime.exec 不能为空")
	}
	if strings.Contains(m.Runtime.Exec, "..") || strings.HasPrefix(m.Runtime.Exec, "/") {
		return errx.BadRequest("runtime.exec 必须是插件目录内的相对路径")
	}
	for _, p := range m.Permissions {
		if !KnownPermissions[p] {
			return errx.BadRequest("未知权限: %s", p)
		}
	}
	return m.validateContributions()
}

func (m *Manifest) validateContributions() error {
	prefix := "/plugins/" + m.Name
	apiPrefix := "/api/plugins/" + m.Name
	seenMenuPath := map[string]bool{}
	for _, c := range m.Contributions.Menus {
		if !strings.HasPrefix(c.Path, prefix+"/") && c.Path != prefix {
			return errx.BadRequest("菜单路径 %s 必须以 %s 开头", c.Path, prefix)
		}
		if seenMenuPath[c.Path] {
			return errx.BadRequest("菜单路径重复: %s", c.Path)
		}
		seenMenuPath[c.Path] = true
	}
	for _, c := range m.Contributions.Pages {
		if !strings.HasPrefix(c.Path, prefix+"/") && c.Path != prefix {
			return errx.BadRequest("页面路径 %s 必须以 %s 开头", c.Path, prefix)
		}
	}
	for _, c := range m.Contributions.Routes {
		if c.Prefix != apiPrefix && !strings.HasPrefix(c.Prefix, apiPrefix+"/") {
			return errx.BadRequest("路由前缀 %s 必须是 %s 或其子前缀", c.Prefix, apiPrefix)
		}
	}
	for _, c := range m.Contributions.EventPubs {
		if !eventRe.MatchString(c.Event) {
			return errx.BadRequest("事件名 %s 必须是反向域名格式（a.b.c）", c.Event)
		}
	}
	for _, c := range m.Contributions.EventSubs {
		if !eventRe.MatchString(c.Event) {
			return errx.BadRequest("订阅事件名 %s 无效", c.Event)
		}
	}
	for _, c := range m.Contributions.Widgets {
		if !strings.HasPrefix(c.Path, prefix+"/") {
			return errx.BadRequest("小部件路径 %s 必须以 %s 开头", c.Path, prefix)
		}
	}
	// settings/services 的 schedule 与 schema 深度校验在渲染/调度侧执行（PR-2）
	return nil
}

// HasPermission 报告清单是否声明了某能力。
func (m *Manifest) HasPermission(perm string) bool {
	return slices.Contains(m.Permissions, perm)
}

// LoadManifest 从插件目录加载 plugin.yaml：解码 → 默认值 → 校验。
// dir 为插件安装目录（用于校验 exec 落点）。
func LoadManifest(path, dir string) (*Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, errx.Internal("读取 manifest 失败: %w", err)
	}
	var m Manifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, errx.BadRequest("manifest YAML 无效: %w", err)
	}
	m.ManifestDefaults()
	if err := m.Validate(); err != nil {
		return nil, err
	}
	// exec 必须真实存在于插件目录
	if _, err := os.Stat(filepath.Join(dir, m.Runtime.Exec)); err != nil {
		return nil, errx.Internal("插件可执行文件不存在: %s", filepath.Join(dir, m.Runtime.Exec))
	}
	return &m, nil
}

// Summary 供审计与日志使用的单行摘要。
func (m *Manifest) Summary() string {
	return fmt.Sprintf("%s@%s (api v%d)", m.Name, m.Version, m.APIVersion)
}

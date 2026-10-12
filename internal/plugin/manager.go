package plugin

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"sync"

	"easyserver/internal/infra/errx"
)

// Manager 是插件管理运行时：扫描注册、启用/禁用/卸载、token 签发、
// 配置读写与扩展点分发（文档 §3.2 的 PluginManager）。
type Manager struct {
	repo       PluginRepository
	sup        *Supervisor
	bus        *EventBus
	storage    *PluginStorage
	pluginsDir string

	mu        sync.Mutex
	manifests map[string]*Manifest // name → manifest（已加载）
	tokens    map[string]string    // token → name（Host API 鉴权）
	auditFn   func(ctx context.Context, plugin, summary string)
	notifyFn  func(ctx context.Context, title, content string) error
	portExp   PortExposer // 暴露端口访问控制（router 注入防火墙 PortACLService 适配）
	panelAPI  PanelAPIConfig

	svc *serviceScheduler // 定时任务调度（manifest.services 贡献点）
}

// PanelAPIConfig 是面板服务级 API 代理配置（权限 panel.api.proxy）。
// 插件经 Host API panel.api 自请求面板业务 API（只读白名单），非用户态。
type PanelAPIConfig struct {
	BaseURL string // 面板自身地址（自请求用），如 http://127.0.0.1:8080
	Token   string // 面板服务 token（自请求鉴权头，启动时随机生成）
	Domain  string // 规范域名（自请求 Host 头，避免域名重定向）
}

// ManagerOptions 是 Manager 的构造依赖。
type ManagerOptions struct {
	Repo       PluginRepository
	Supervisor *Supervisor
	Bus        *EventBus
	Storage    *PluginStorage
	PluginsDir string
	// AuditFn 插件审计写入口（复用面板审计服务，username=plugin:<name>）
	AuditFn func(ctx context.Context, plugin, summary string)
	// NotifyFn 插件通知写入口（复用面板 Webhook 通道）
	NotifyFn func(ctx context.Context, title, content string) error
	// PortExposer 暴露端口访问控制（面板防火墙 PortACL，权限 firewall.manage）
	PortExposer PortExposer
	// PanelAPI 面板服务级 API 代理（权限 panel.api.proxy，只读白名单）
	PanelAPI PanelAPIConfig
}

// NewManager 创建插件管理器。
func NewManager(o ManagerOptions) *Manager {
	if o.Bus == nil {
		o.Bus = NewEventBus()
	}
	m := &Manager{
		repo:       o.Repo,
		sup:        o.Supervisor,
		bus:        o.Bus,
		storage:    o.Storage,
		pluginsDir: o.PluginsDir,
		manifests:  map[string]*Manifest{},
		tokens:     map[string]string{},
		auditFn:    o.AuditFn,
		notifyFn:   o.NotifyFn,
		portExp:    o.PortExposer,
		panelAPI:   o.PanelAPI,
		svc:        newServiceScheduler(),
	}
	m.svc.start()
	return m
}

// Close 释放管理器持有的资源（定时任务调度），面板关停时调用。
func (m *Manager) Close() {
	if m.svc != nil {
		m.svc.stop()
	}
}

// ScanPlugins 扫描插件目录：加载全部 plugin.yaml，注册/刷新状态，
// 并对已启用插件拉起进程（幂等，面板启动时调用）。
func (m *Manager) ScanPlugins(ctx context.Context) error {
	entries, err := os.ReadDir(m.pluginsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // 目录尚不存在 = 无插件
		}
		return err
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		if !nameRe.MatchString(name) {
			continue
		}
		manifestPath := filepath.Join(m.pluginsDir, name, "plugin.yaml")
		mft, err := LoadManifest(manifestPath, filepath.Join(m.pluginsDir, name))
		if err != nil {
			// 无效插件：记录 error 不启动，不影响其他插件（文档 §3.3）
			_ = m.repo.SetPluginStatus(ctx, name, StateError, 0, err.Error())
			logManagerf(" 插件 %s 加载失败: %v", name, err)
			continue
		}
		m.mu.Lock()
		m.manifests[mft.Name] = mft
		m.mu.Unlock()

		existing, _ := m.repo.GetPlugin(ctx, mft.Name)
		enabled := existing != nil && existing.Enabled
		if err := m.repo.UpsertPlugin(ctx, &PluginRecord{
			Name:                mft.Name,
			Version:             mft.Version,
			APIVersion:          mft.APIVersion,
			ManifestPath:        manifestPath,
			Enabled:             enabled,
			Status:              StateRegistered,
			PermissionsApproved: mft.Permissions,
		}); err != nil {
			return err
		}
		if enabled {
			if err := m.Start(ctx, mft.Name); err != nil {
				logManagerf(" 启用插件 %s 启动失败: %v", mft.Name, err)
			}
		}
	}
	return nil
}

// List 返回全部已注册插件的清单与状态。
func (m *Manager) List(ctx context.Context) ([]PluginRecord, error) {
	return m.repo.ListPlugins(ctx)
}

// GetManifest 返回指定插件的 manifest。
func (m *Manager) GetManifest(_ context.Context, name string) (*Manifest, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if mft, ok := m.manifests[name]; ok {
		return mft, nil
	}
	return nil, errx.NotFound("插件 %s 未加载", name)
}

// Start 启用并启动插件（批准全部声明权限；按需收缩见 Approve）。
func (m *Manager) Start(ctx context.Context, name string) error {
	return m.StartWithApproved(ctx, name, nil)
}

// StartWithApproved 启用并启动插件，按给定权限子集批准（空/nil = 全部声明）。
func (m *Manager) StartWithApproved(ctx context.Context, name string, approved []string) error {
	m.mu.Lock()
	mft, ok := m.manifests[name]
	m.mu.Unlock()
	if !ok {
		return errx.NotFound("插件 %s 未加载", name)
	}
	if len(approved) == 0 {
		approved = mft.Permissions
	}
	token, err := GenerateToken()
	if err != nil {
		return err
	}
	m.mu.Lock()
	m.tokens[token] = name
	m.mu.Unlock()

	if err := m.sup.EnsureRunning(ctx, mft, token); err != nil {
		// 启动失败回滚刚签发的 token（锁已在上方释放，勿 Unlock）
		m.mu.Lock()
		delete(m.tokens, token)
		m.mu.Unlock()
		return err
	}
	if err := m.repo.SetPluginEnabled(ctx, name, true, approved); err != nil {
		return err
	}
	m.startEventDelivery(name, mft) //nolint:contextcheck // 事件投递运行在总线独立 goroutine，用总线自有 context
	m.svc.schedule(m, name, mft)    //nolint:contextcheck // 定时任务独立于请求上下文，调度器内部自建 context
	m.bus.Publish(EventServiceStart, "plugin:"+name, map[string]string{"plugin": name})
	return nil
}

// Approve 校验并批准权限子集（不可超声明）后启用启动插件（§11 权限模型）。
func (m *Manager) Approve(ctx context.Context, name string, approved []string) error {
	m.mu.Lock()
	mft, ok := m.manifests[name]
	m.mu.Unlock()
	if !ok {
		return errx.NotFound("插件 %s 未加载", name)
	}
	valid := make(map[string]bool, len(mft.Permissions))
	for _, p := range mft.Permissions {
		valid[p] = true
	}
	clean := make([]string, 0, len(approved))
	seen := make(map[string]bool)
	for _, p := range approved {
		if !valid[p] {
			return errx.BadRequest("权限 %s 未在插件声明中", p)
		}
		if !seen[p] {
			seen[p] = true
			clean = append(clean, p)
		}
	}
	return m.StartWithApproved(ctx, name, clean)
}

// Stop 停止插件进程（保持注册与启用状态）。
func (m *Manager) Stop(ctx context.Context, name string) error {
	if err := m.sup.StopPlugin(ctx, name); err != nil {
		return err
	}
	m.stopEventDelivery(name)
	m.svc.unschedule(name)
	m.bus.Publish(EventServiceStop, "plugin:"+name, map[string]string{"plugin": name})
	return nil
}

// Disable 停止并禁用插件。
func (m *Manager) Disable(ctx context.Context, name string) error {
	if err := m.Stop(ctx, name); err != nil {
		return err
	}
	// 注销 token：停用后 Host API 立即拒绝该插件的调用
	m.mu.Lock()
	for t, n := range m.tokens {
		if n == name {
			delete(m.tokens, t)
		}
	}
	m.mu.Unlock()
	return m.repo.SetPluginEnabled(ctx, name, false, nil)
}

// Remove 卸载插件：停止 + 删除文件区 + 删除注册/配置/密钥。
// 插件目录文件本身保留（由管理员手动删除），防误删用户数据。
func (m *Manager) Remove(ctx context.Context, name string) error {
	m.mu.Lock()
	_, loaded := m.manifests[name]
	m.mu.Unlock()
	if !loaded {
		return errx.NotFound("插件 %s 未加载", name)
	}
	if err := m.Disable(ctx, name); err != nil {
		return err
	}
	m.mu.Lock()
	delete(m.manifests, name)
	for t, n := range m.tokens {
		if n == name {
			delete(m.tokens, t)
		}
	}
	m.mu.Unlock()
	return m.repo.DeletePlugin(ctx, name)
}

// ---- Host API 支撑 ----

// ManifestByToken 用 token 反查插件 manifest（Host API 鉴权）。
func (m *Manager) ManifestByToken(ctx context.Context, token string) (*Manifest, error) {
	m.mu.Lock()
	name, ok := m.tokens[token]
	m.mu.Unlock()
	if !ok {
		return nil, errx.Unauthorized("无效的插件 token")
	}
	return m.GetManifest(ctx, name)
}

// HasPermission 报告 token 对应插件是否被授予某能力（默认拒绝）。
// 校验的是启用时管理员批准的子集（§11 权限模型），而非 manifest 声明全集。
func (m *Manager) HasPermission(ctx context.Context, token, perm string) bool {
	mft, err := m.ManifestByToken(ctx, token)
	if err != nil {
		return false
	}
	if !mft.HasPermission(perm) {
		return false // 未声明，必未批准
	}
	rec, err := m.repo.GetPlugin(ctx, mft.Name)
	if err != nil || rec == nil {
		return false
	}
	return slices.Contains(rec.PermissionsApproved, perm)
}

// PluginNameByToken 返回 token 对应插件名。
func (m *Manager) PluginNameByToken(token string) (string, error) {
	m.mu.Lock()
	name, ok := m.tokens[token]
	m.mu.Unlock()
	if !ok {
		return "", errx.Unauthorized("无效的插件 token")
	}
	return name, nil
}

// AuditWrite 插件写审计（Host API /audit）。
func (m *Manager) AuditWrite(ctx context.Context, plugin, summary string) error {
	if m.auditFn == nil {
		return nil
	}
	m.auditFn(ctx, plugin, summary)
	return nil
}

// Notify 插件发送通知（Host API /notify）。
func (m *Manager) Notify(ctx context.Context, plugin, title, content string) error {
	if m.notifyFn == nil {
		return errx.Internal("通知通道未接线")
	}
	_ = plugin
	return m.notifyFn(ctx, title, content)
}

// ConfigList 返回插件配置（面板前端经 /plugins/:id/config 消费）。
func (m *Manager) ConfigList(ctx context.Context, name string) (map[string]string, error) {
	return m.repo.ListConfig(ctx, name)
}

// ConfigSet 批量写插件配置。
func (m *Manager) ConfigSet(ctx context.Context, name string, kv map[string]string) error {
	for k, v := range kv {
		if err := m.repo.SetConfig(ctx, name, k, v); err != nil {
			return err
		}
	}
	m.bus.Publish("easyserver.plugin.config_changed", "plugin:"+name, map[string]string{"plugin": name})
	return nil
}

// Storage 返回插件私有文件区访问器。
func (m *Manager) Storage() *PluginStorage { return m.storage }

// Bus 返回事件总线。
func (m *Manager) Bus() *EventBus { return m.bus }

// PluginDir 返回插件安装目录。
func (m *Manager) PluginDir(name string) string {
	return filepath.Join(m.pluginsDir, name)
}

// PublishBusinessEvent 业务点发布的包装（供 auth/web/cron 等域调用）。
func PublishBusinessEvent(name, source string, payload any) {
	if b := DefaultBus(); b != nil {
		b.Publish(name, source, payload)
	}
}

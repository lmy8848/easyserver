package plugin

import (
	"context"
	"log"
	"os"
	"path/filepath"
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
}

// NewManager 创建插件管理器。
func NewManager(o ManagerOptions) *Manager {
	if o.Bus == nil {
		o.Bus = NewEventBus()
	}
	return &Manager{
		repo:       o.Repo,
		sup:        o.Supervisor,
		bus:        o.Bus,
		storage:    o.Storage,
		pluginsDir: o.PluginsDir,
		manifests:  map[string]*Manifest{},
		tokens:     map[string]string{},
		auditFn:    o.AuditFn,
		notifyFn:   o.NotifyFn,
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
			log.Printf("plugin: 插件 %s 加载失败: %v", name, err)
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
				log.Printf("plugin: 启用插件 %s 启动失败: %v", mft.Name, err)
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

// Start 启用并启动插件（等价管理员批准全部声明权限；审批 UI 在基座 PR-3）。
func (m *Manager) Start(ctx context.Context, name string) error {
	m.mu.Lock()
	mft, ok := m.manifests[name]
	m.mu.Unlock()
	if !ok {
		return errx.NotFound("插件 %s 未加载", name)
	}
	token, err := GenerateToken()
	if err != nil {
		return err
	}
	m.mu.Lock()
	m.tokens[token] = name
	m.mu.Unlock()

	if err := m.sup.EnsureRunning(ctx, mft, token); err != nil {
		m.mu.Unlock()
		return err
	}
	if err := m.repo.SetPluginEnabled(ctx, name, true, mft.Permissions); err != nil {
		return err
	}
	m.bus.Publish(EventServiceStart, "plugin:"+name, map[string]string{"plugin": name})
	return nil
}

// Stop 停止插件进程（保持注册与启用状态）。
func (m *Manager) Stop(ctx context.Context, name string) error {
	if err := m.sup.StopPlugin(ctx, name); err != nil {
		return err
	}
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

// HasPermission 报告 token 对应插件是否声明了某能力（默认拒绝）。
func (m *Manager) HasPermission(ctx context.Context, token, perm string) bool {
	mft, err := m.ManifestByToken(ctx, token)
	if err != nil {
		return false
	}
	return mft.HasPermission(perm)
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

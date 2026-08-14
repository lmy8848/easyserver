package config

import (
	"slices"
	"sync/atomic"
)

// Store 是运行期配置的唯一权威：写方（settings handler）经 Update 在副本上
// 修改后原子替换，读方 Get 无锁拿到一份完整一致的最新快照（copy-on-write）。
//
// 语义：Get 返回的指针属于某一代配置，之后 Update 不会改动它（旧代不再被
// 引用后由 GC 回收），因此任何读者都可以安全持有快照而不必担心 data race——
// 需要实时配置的服务每次读 store.Get()，需要启动快照的调用方取一次即可。
type Store struct {
	p atomic.Pointer[Config]
}

// NewStore 用初始配置构造 Store。cfg 随后被 Store 接管：不要在外面继续修改
// 传入的 cfg，一律通过 Update。
func NewStore(cfg *Config) *Store {
	s := &Store{}
	s.p.Store(cfg)
	return s
}

// Get 返回当前配置快照（只读，勿修改其字段）。
func (s *Store) Get() *Config { return s.p.Load() }

// Update 在副本上应用修改后原子替换：读方永远看不到"改了一半"的配置。
func (s *Store) Update(fn func(*Config)) {
	next := s.Get().clone()
	fn(next)
	s.p.Store(next)
}

// clone 深拷贝可变字段（slice），其余标量随结构体复制。
func (c *Config) clone() *Config {
	cp := *c
	cp.Server.AllowedOrigins = slices.Clone(c.Server.AllowedOrigins)
	cp.Server.TrustedProxies = slices.Clone(c.Server.TrustedProxies)
	cp.Alerts.Rules = slices.Clone(c.Alerts.Rules)
	return &cp
}

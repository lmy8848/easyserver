package mise

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/pelletier/go-toml/v2"
)

// ErrInvalidConfig 表示 config.toml 无法解析。面板绝不覆盖损坏的文件，
// 解析失败一律报错，把文件留给用户手动修复。
var ErrInvalidConfig = errors.New("mise config.toml 解析失败")

// EnvStore 以 config.toml 的 [env] 段为唯一权威存储（文件即权威，无 DB 副本、
// 无启用/禁用状态）。
//
// 所有写操作进程内互斥 + 原子写（temp + rename）；读改写只动 [env] 段，
// 文件里其它段（如将来的 [tools]）原样保留。
type EnvStore struct {
	mu   sync.Mutex
	path string
}

// NewEnvStore 创建一个指向面板私有 config.toml 的文件权威 env 存储。
func NewEnvStore() *EnvStore {
	return &EnvStore{path: filepath.Join(DataDir, "config.toml")}
}

// ListEnv 读取 [env] 段全部条目（key -> value）。
// 文件不存在视为空配置；文件存在但非法 TOML 返回错误（不覆盖）。
// [env] 段内的非字符串值（mise 的 path 列表等语法）跳过。
func (s *EnvStore) ListEnv() (map[string]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.readEnv()
}

// UpsertEnv 写入/更新单个 env key，保留文件其它内容。
func (s *EnvStore) UpsertEnv(key, value string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	root, err := s.readRoot()
	if err != nil {
		return err
	}
	env, _ := root["env"].(map[string]any)
	if env == nil {
		env = map[string]any{}
	}
	env[key] = value
	root["env"] = env
	return s.writeRoot(root)
}

// DeleteEnv 删除单个 env key，保留文件其它内容。
func (s *EnvStore) DeleteEnv(key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	root, err := s.readRoot()
	if err != nil {
		return err
	}
	env, ok := root["env"].(map[string]any)
	if !ok {
		// 没有 [env] 段，删除目标自然不存在。
		return nil
	}
	if _, exists := env[key]; !exists {
		return nil
	}
	delete(env, key)
	return s.writeRoot(root)
}

// readEnv 是 ListEnv 的锁内实现。
func (s *EnvStore) readEnv() (map[string]string, error) {
	root, err := s.readRoot()
	if err != nil {
		return nil, err
	}
	envs := make(map[string]string, len(root))
	if sec, ok := root["env"].(map[string]any); ok {
		for k, v := range sec {
			if str, ok := v.(string); ok {
				envs[k] = str
			}
		}
	}
	return envs, nil
}

// readRoot 解析整个 config.toml；文件不存在返回空根。
func (s *EnvStore) readRoot() (map[string]any, error) {
	data, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return map[string]any{}, nil
	}
	if err != nil {
		return nil, err
	}
	root := map[string]any{}
	if err := toml.Unmarshal(data, &root); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidConfig, err)
	}
	return root, nil
}

// writeRoot 整体序列化并原子写回 config.toml。
func (s *EnvStore) writeRoot(root map[string]any) error {
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data, err := toml.Marshal(root)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, "config.toml.*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpName, 0o644); err != nil {
		return err
	}
	return os.Rename(tmpName, s.path)
}

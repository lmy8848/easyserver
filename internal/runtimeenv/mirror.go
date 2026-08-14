package runtimeenv

import (
	"context"
	"sort"
	"strings"

	"easyserver/internal/infra/apperror"
	"easyserver/internal/infra/mise"
)

// MirrorEntry 镜像源条目：config.toml [env] 段中的一项。
// 文件即权威：没有 enabled/source/order 状态，写入即生效、删除即消失。
type MirrorEntry struct {
	EnvKey   string `json:"env_key"`
	EnvValue string `json:"env_value"`
}

// MirrorService 以 /opt/easyserver/mise/config.toml 的 [env] 段为权威存储，
// 独立于 env_configs（环境变量 API 保留 DB 存储，二者不再耦合）。
type MirrorService struct {
	store *mise.EnvStore
}

// NewMirrorService 创建一个文件权威的镜像源服务。
func NewMirrorService(store *mise.EnvStore) *MirrorService {
	return &MirrorService{store: store}
}

// List 返回当前 [env] 段全部字符串条目，按键排序。
func (s *MirrorService) List(ctx context.Context) ([]MirrorEntry, error) {
	envs, err := s.store.ListEnv()
	if err != nil {
		return nil, err
	}
	keys := make([]string, 0, len(envs))
	for k := range envs {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	entries := make([]MirrorEntry, 0, len(keys))
	for _, k := range keys {
		entries = append(entries, MirrorEntry{EnvKey: k, EnvValue: envs[k]})
	}
	return entries, nil
}

// Upsert 写入/更新镜像源，保存即生效（直写文件）。
func (s *MirrorService) Upsert(ctx context.Context, key, value string) error {
	if !isValidEnvKey(key) {
		return apperror.ErrBadRequest.WithMessage("无效的环境变量名：" + key)
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return apperror.ErrBadRequest.WithMessage("镜像地址不能为空")
	}
	return s.store.UpsertEnv(key, value)
}

// Delete 删除镜像源，删除即从文件消失。
func (s *MirrorService) Delete(ctx context.Context, key string) error {
	if !isValidEnvKey(key) {
		return apperror.ErrBadRequest.WithMessage("无效的环境变量名：" + key)
	}
	return s.store.DeleteEnv(key)
}

// isValidEnvKey 校验标准 POSIX 环境变量名：字母/数字/下划线，不以数字开头。
func isValidEnvKey(name string) bool {
	if len(name) == 0 || len(name) > 256 {
		return false
	}
	for i, c := range name {
		ok := (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || c == '_'
		if i > 0 && c >= '0' && c <= '9' {
			ok = true
		}
		if !ok {
			return false
		}
	}
	return true
}

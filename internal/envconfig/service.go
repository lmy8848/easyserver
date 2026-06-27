package envconfig

import (
	"context"
	"fmt"
	"strings"

	"easyserver/internal/infra/apperror"
)

// Service provides environment variable and global config management
type Service struct {
	repo Repository
}

// NewService creates a new Service
func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

// ListEnvConfigs returns all environment configurations
func (s *Service) ListEnvConfigs(ctx context.Context, runtimeID int64) ([]EnvConfig, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	return s.repo.ListEnvConfigs(ctx, runtimeID)
}

// GetEnvConfig returns a specific environment configuration
func (s *Service) GetEnvConfig(ctx context.Context, id int64) (*EnvConfig, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	return s.repo.GetEnvConfig(ctx, id)
}

// CreateEnvConfig creates a new environment configuration
func (s *Service) CreateEnvConfig(ctx context.Context, c *EnvConfig) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if !isValidEnvName(c.Name) {
		return apperror.ErrBadRequest.WithMessage(fmt.Sprintf("无效的环境变量名：%s", c.Name))
	}
	return s.repo.CreateEnvConfig(ctx, c)
}

// UpdateEnvConfig updates an environment configuration
func (s *Service) UpdateEnvConfig(ctx context.Context, c *EnvConfig) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if !isValidEnvName(c.Name) {
		return apperror.ErrBadRequest.WithMessage(fmt.Sprintf("无效的环境变量名：%s", c.Name))
	}
	return s.repo.UpdateEnvConfig(ctx, c)
}

// DeleteEnvConfig deletes an environment configuration
func (s *Service) DeleteEnvConfig(ctx context.Context, id int64) error {
	if ctx == nil {
		ctx = context.Background()
	}
	return s.repo.DeleteEnvConfig(ctx, id)
}

// ListPathEntries returns all PATH entries
func (s *Service) ListPathEntries(ctx context.Context, runtimeID int64) ([]PathEntry, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	return s.repo.ListPathEntries(ctx, runtimeID)
}

// CreatePathEntry creates a new PATH entry
func (s *Service) CreatePathEntry(ctx context.Context, e *PathEntry) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if !isValidPath(e.Path) {
		return apperror.ErrBadRequest.WithMessage(fmt.Sprintf("无效的路径：%s", e.Path))
	}
	return s.repo.CreatePathEntry(ctx, e)
}

// DeletePathEntry deletes a PATH entry
func (s *Service) DeletePathEntry(ctx context.Context, id int64) error {
	if ctx == nil {
		ctx = context.Background()
	}
	return s.repo.DeletePathEntry(ctx, id)
}

// ReorderPathEntries reorders PATH entries
func (s *Service) ReorderPathEntries(ctx context.Context, runtimeID int64, ids []int64) error {
	if ctx == nil {
		ctx = context.Background()
	}
	return s.repo.ReorderPathEntries(ctx, runtimeID, ids)
}

// GenerateEnvScript generates a shell script to set environment variables
func (s *Service) GenerateEnvScript(ctx context.Context, runtimeID int64) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	var script strings.Builder

	configs, err := s.ListEnvConfigs(ctx, runtimeID)
	if err != nil {
		return "", err
	}

	for _, c := range configs {
		escaped := shellEscapeDoubleQuote(c.Value)
		script.WriteString(fmt.Sprintf("export %s=\"%s\"\n", c.Name, escaped))
	}

	entries, err := s.ListPathEntries(ctx, runtimeID)
	if err != nil {
		return "", err
	}

	if len(entries) > 0 {
		script.WriteString("export PATH=\"")
		for i, e := range entries {
			if i > 0 {
				script.WriteString(":")
			}
			script.WriteString(shellEscapeDoubleQuote(e.Path))
		}
		script.WriteString(":$PATH\"\n")
	}

	return script.String(), nil
}

// ListGlobalConfigs returns all global configurations
func (s *Service) ListGlobalConfigs(ctx context.Context, category string) ([]GlobalConfig, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	return s.repo.ListGlobalConfigs(ctx, category)
}

// GetGlobalConfig returns a specific global configuration
func (s *Service) GetGlobalConfig(ctx context.Context, id int64) (*GlobalConfig, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	return s.repo.GetGlobalConfig(ctx, id)
}

// CreateGlobalConfig creates a new global configuration
func (s *Service) CreateGlobalConfig(ctx context.Context, c *GlobalConfig) error {
	if ctx == nil {
		ctx = context.Background()
	}
	return s.repo.CreateGlobalConfig(ctx, c)
}

// UpdateGlobalConfig updates a global configuration
func (s *Service) UpdateGlobalConfig(ctx context.Context, c *GlobalConfig) error {
	if ctx == nil {
		ctx = context.Background()
	}
	return s.repo.UpdateGlobalConfig(ctx, c)
}

// DeleteGlobalConfig deletes a global configuration
func (s *Service) DeleteGlobalConfig(ctx context.Context, id int64) error {
	if ctx == nil {
		ctx = context.Background()
	}
	return s.repo.DeleteGlobalConfig(ctx, id)
}

// InitDefaultGlobalConfigs initializes default global configurations
func (s *Service) InitDefaultGlobalConfigs(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	defaults := []GlobalConfig{
		{Category: "maven", Key: "mirror_url", Value: "https://maven.aliyun.com/repository/public", Description: "Maven 镜像地址"},
		{Category: "maven", Key: "local_repo", Value: "${user.home}/.m2/repository", Description: "Maven 本地仓库路径"},
		{Category: "npm", Key: "registry", Value: "https://registry.npmmirror.com", Description: "npm 镜像源"},
		{Category: "npm", Key: "cache", Value: "${HOME}/.npm", Description: "npm 缓存目录"},
		{Category: "pip", Key: "index_url", Value: "https://pypi.tuna.tsinghua.edu.cn/simple", Description: "pip 镜像源"},
		{Category: "pip", Key: "trusted_host", Value: "pypi.tuna.tsinghua.edu.cn", Description: "pip 可信主机"},
		{Category: "go", Key: "goproxy", Value: "https://goproxy.cn,direct", Description: "Go 模块代理"},
		{Category: "go", Key: "gonosumcheck", Value: "", Description: "Go 不校验 checksum 的模块"},
		{Category: "composer", Key: "repo_url", Value: "https://mirrors.aliyun.com/composer/", Description: "Composer 镜像地址"},
		{Category: "ruby", Key: "source", Value: "https://gems.ruby-china.com/", Description: "RubyGems 镜像源"},
	}

	for i := range defaults {
		if err := s.repo.CreateGlobalConfigIfNotExists(ctx, &defaults[i]); err != nil {
			return err
		}
	}

	return nil
}

// shellEscapeDoubleQuote escapes special characters for use inside a double-quoted shell string.
func shellEscapeDoubleQuote(s string) string {
	var b strings.Builder
	for _, c := range s {
		switch c {
		case '\\', '"', '$', '`', '!':
			b.WriteRune('\\')
		}
		b.WriteRune(c)
	}
	return b.String()
}

// isValidEnvName validates environment variable name
func isValidEnvName(name string) bool {
	if len(name) == 0 || len(name) > 256 {
		return false
	}
	for i, c := range name {
		if i == 0 {
			if !((c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || c == '_') {
				return false
			}
		} else {
			if !((c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '_') {
				return false
			}
		}
	}
	return true
}

// isValidPath validates PATH entry
func isValidPath(path string) bool {
	if len(path) == 0 || len(path) > 4096 {
		return false
	}
	if path[0] != '/' {
		return false
	}
	if strings.Contains(path, "..") {
		return false
	}
	shellMeta := "|&;()`${}<>'\"\\!#~"
	for _, c := range path {
		if strings.ContainsRune(shellMeta, c) {
			return false
		}
		if c < 32 {
			return false
		}
	}
	return true
}

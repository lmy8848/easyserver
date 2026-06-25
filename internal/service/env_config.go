package service

import (
	"context"
	"fmt"
	"strings"

	"easyserver/internal/model"
	"easyserver/internal/repository"
)

type EnvConfigService struct {
	repo repository.EnvConfigRepository
}

func NewEnvConfigService(repo repository.EnvConfigRepository) *EnvConfigService {
	return &EnvConfigService{repo: repo}
}

// ListEnvConfigs returns all environment configurations
func (s *EnvConfigService) ListEnvConfigs(ctx context.Context, runtimeID int64) ([]model.EnvConfig, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	return s.repo.ListEnvConfigs(ctx, runtimeID)
}

// GetEnvConfig returns a specific environment configuration
func (s *EnvConfigService) GetEnvConfig(ctx context.Context, id int64) (*model.EnvConfig, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	return s.repo.GetEnvConfig(ctx, id)
}

// CreateEnvConfig creates a new environment configuration
func (s *EnvConfigService) CreateEnvConfig(ctx context.Context, c *model.EnvConfig) error {
	if ctx == nil {
		ctx = context.Background()
	}
	// Validate name
	if !isValidEnvName(c.Name) {
		return fmt.Errorf("invalid environment variable name: %s", c.Name)
	}

	return s.repo.CreateEnvConfig(ctx, c)
}

// UpdateEnvConfig updates an environment configuration
func (s *EnvConfigService) UpdateEnvConfig(ctx context.Context, c *model.EnvConfig) error {
	if ctx == nil {
		ctx = context.Background()
	}
	// Validate name
	if !isValidEnvName(c.Name) {
		return fmt.Errorf("invalid environment variable name: %s", c.Name)
	}

	return s.repo.UpdateEnvConfig(ctx, c)
}

// DeleteEnvConfig deletes an environment configuration
func (s *EnvConfigService) DeleteEnvConfig(ctx context.Context, id int64) error {
	if ctx == nil {
		ctx = context.Background()
	}
	return s.repo.DeleteEnvConfig(ctx, id)
}

// ListPathEntries returns all PATH entries
func (s *EnvConfigService) ListPathEntries(ctx context.Context, runtimeID int64) ([]model.PathEntry, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	return s.repo.ListPathEntries(ctx, runtimeID)
}

// CreatePathEntry creates a new PATH entry
func (s *EnvConfigService) CreatePathEntry(ctx context.Context, e *model.PathEntry) error {
	if ctx == nil {
		ctx = context.Background()
	}
	// Validate path
	if !isValidPath(e.Path) {
		return fmt.Errorf("invalid path: %s", e.Path)
	}

	return s.repo.CreatePathEntry(ctx, e)
}

// DeletePathEntry deletes a PATH entry
func (s *EnvConfigService) DeletePathEntry(ctx context.Context, id int64) error {
	if ctx == nil {
		ctx = context.Background()
	}
	return s.repo.DeletePathEntry(ctx, id)
}

// ReorderPathEntries reorders PATH entries
func (s *EnvConfigService) ReorderPathEntries(ctx context.Context, runtimeID int64, ids []int64) error {
	if ctx == nil {
		ctx = context.Background()
	}
	return s.repo.ReorderPathEntries(ctx, runtimeID, ids)
}

// GenerateEnvScript generates a shell script to set environment variables
func (s *EnvConfigService) GenerateEnvScript(ctx context.Context, runtimeID int64) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	var script strings.Builder

	// Get environment variables
	configs, err := s.ListEnvConfigs(ctx, runtimeID)
	if err != nil {
		return "", err
	}

	for _, c := range configs {
		// Escape value for safe use inside double-quoted shell string
		escaped := shellEscapeDoubleQuote(c.Value)
		script.WriteString(fmt.Sprintf("export %s=\"%s\"\n", c.Name, escaped))
	}

	// Get PATH entries
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
			// PATH entries are already validated by isValidPath, but escape for safety
			script.WriteString(shellEscapeDoubleQuote(e.Path))
		}
		script.WriteString(":$PATH\"\n")
	}

	return script.String(), nil
}

// shellEscapeDoubleQuote escapes special characters for use inside a double-quoted shell string.
// Characters escaped: \ " $ ` !
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
			// First character must be letter or underscore
			if !((c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || c == '_') {
				return false
			}
		} else {
			// Subsequent characters can be letters, digits, or underscores
			if !((c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '_') {
				return false
			}
		}
	}
	return true
}

// isValidPath validates PATH entry — must be a safe absolute path with no shell metacharacters
func isValidPath(path string) bool {
	if len(path) == 0 || len(path) > 4096 {
		return false
	}
	// Must start with / (absolute path)
	if path[0] != '/' {
		return false
	}
	// Reject paths with .. for security
	if strings.Contains(path, "..") {
		return false
	}
	// Reject shell metacharacters that could enable command injection
	shellMeta := "|&;()`${}<>'\"\\!#~"
	for _, c := range path {
		if strings.ContainsRune(shellMeta, c) {
			return false
		}
		// Reject control characters
		if c < 32 {
			return false
		}
	}
	return true
}

// ListGlobalConfigs returns all global configurations
func (s *EnvConfigService) ListGlobalConfigs(ctx context.Context, category string) ([]model.GlobalConfig, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	return s.repo.ListGlobalConfigs(ctx, category)
}

// GetGlobalConfig returns a specific global configuration
func (s *EnvConfigService) GetGlobalConfig(ctx context.Context, id int64) (*model.GlobalConfig, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	return s.repo.GetGlobalConfig(ctx, id)
}

// CreateGlobalConfig creates a new global configuration
func (s *EnvConfigService) CreateGlobalConfig(ctx context.Context, c *model.GlobalConfig) error {
	if ctx == nil {
		ctx = context.Background()
	}
	return s.repo.CreateGlobalConfig(ctx, c)
}

// UpdateGlobalConfig updates a global configuration
func (s *EnvConfigService) UpdateGlobalConfig(ctx context.Context, c *model.GlobalConfig) error {
	if ctx == nil {
		ctx = context.Background()
	}
	return s.repo.UpdateGlobalConfig(ctx, c)
}

// DeleteGlobalConfig deletes a global configuration
func (s *EnvConfigService) DeleteGlobalConfig(ctx context.Context, id int64) error {
	if ctx == nil {
		ctx = context.Background()
	}
	return s.repo.DeleteGlobalConfig(ctx, id)
}

// InitDefaultGlobalConfigs initializes default global configurations
func (s *EnvConfigService) InitDefaultGlobalConfigs(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	defaults := []model.GlobalConfig{
		// Maven
		{Category: "maven", Key: "mirror_url", Value: "https://maven.aliyun.com/repository/public", Description: "Maven 镜像地址"},
		{Category: "maven", Key: "local_repo", Value: "${user.home}/.m2/repository", Description: "Maven 本地仓库路径"},

		// npm
		{Category: "npm", Key: "registry", Value: "https://registry.npmmirror.com", Description: "npm 镜像源"},
		{Category: "npm", Key: "cache", Value: "${HOME}/.npm", Description: "npm 缓存目录"},

		// pip
		{Category: "pip", Key: "index_url", Value: "https://pypi.tuna.tsinghua.edu.cn/simple", Description: "pip 镜像源"},
		{Category: "pip", Key: "trusted_host", Value: "pypi.tuna.tsinghua.edu.cn", Description: "pip 可信主机"},

		// Go
		{Category: "go", Key: "goproxy", Value: "https://goproxy.cn,direct", Description: "Go 模块代理"},
		{Category: "go", Key: "gonosumcheck", Value: "", Description: "Go 不校验 checksum 的模块"},

		// Composer (PHP)
		{Category: "composer", Key: "repo_url", Value: "https://mirrors.aliyun.com/composer/", Description: "Composer 镜像地址"},

		// Ruby
		{Category: "ruby", Key: "source", Value: "https://gems.ruby-china.com/", Description: "RubyGems 镜像源"},
	}

	for i := range defaults {
		if err := s.repo.CreateGlobalConfigIfNotExists(ctx, &defaults[i]); err != nil {
			return err
		}
	}

	return nil
}

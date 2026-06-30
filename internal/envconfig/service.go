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

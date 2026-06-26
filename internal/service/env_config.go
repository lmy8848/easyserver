package service

import (
	"easyserver/internal/envconfig"
)

// EnvConfigService is now defined in easyserver/internal/envconfig.Service.
// Kept as alias for backward compatibility.
type EnvConfigService = envconfig.Service

// NewEnvConfigService creates a new EnvConfigService.
// Delegates to envconfig.NewService.
func NewEnvConfigService(repo envconfig.Repository) *envconfig.Service {
	return envconfig.NewService(repo)
}

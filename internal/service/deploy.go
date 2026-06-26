package service

import "easyserver/internal/deploy"

// DeployService is now defined in easyserver/internal/deploy.Service.
// Kept as alias for backward compatibility.
type DeployService = deploy.Service

// NewDeployService creates a new deploy Service.
// This is a forwarding stub; the implementation lives in internal/deploy.
func NewDeployService(repo deploy.Repository, encryptionKey string) (*deploy.Service, error) {
	return deploy.NewService(repo, encryptionKey)
}

package service

import (
	"easyserver/internal/repository"
	"easyserver/internal/runtimeversion"
)

// RuntimeVersionService is kept as a compatibility alias for the runtimeversion module.
type RuntimeVersionService = runtimeversion.Service

func NewRuntimeVersionService(repo repository.RuntimeRepository) *RuntimeVersionService {
	return runtimeversion.NewService(repo)
}

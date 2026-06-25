package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"easyserver/internal/executor"
	"easyserver/internal/model"
)

// VolumeService manages Docker volumes
type VolumeService struct {
	executor executor.CommandExecutor
}

// NewVolumeService creates a new VolumeService
func NewVolumeService(exec executor.CommandExecutor) *VolumeService {
	return &VolumeService{executor: exec}
}

// ListVolumes returns all Docker volumes
func (s *VolumeService) ListVolumes(ctx context.Context) ([]model.Volume, error) {
	output, exitCode, err := s.executor.RunCombined(ctx, "docker", "volume", "ls", "--format", "{{json .}}")
	if err != nil || exitCode != 0 {
		return nil, fmt.Errorf("docker volume ls failed: %s", output)
	}

	var volumes []model.Volume
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		if line = strings.TrimSpace(line); line == "" {
			continue
		}
		var raw struct {
			Name       string `json:"Name"`
			Driver     string `json:"Driver"`
			Mountpoint string `json:"Mountpoint"`
			CreatedAt  string `json:"CreatedAt"`
		}
		if err := json.Unmarshal([]byte(line), &raw); err != nil {
			continue
		}
		volumes = append(volumes, model.Volume{
			Name:       raw.Name,
			Driver:     raw.Driver,
			Mountpoint: raw.Mountpoint,
			CreatedAt:  raw.CreatedAt,
		})
	}

	return volumes, nil
}

// CreateVolume creates a new Docker volume
func (s *VolumeService) CreateVolume(ctx context.Context, name, driver string) error {
	args := []string{"volume", "create"}
	if driver != "" {
		args = append(args, "--driver", driver)
	}
	args = append(args, name)

	_, exitCode, err := s.executor.RunCombined(ctx, "docker", args...)
	if err != nil || exitCode != 0 {
		return fmt.Errorf("docker volume create failed: %v", err)
	}
	return nil
}

// RemoveVolume removes a Docker volume
func (s *VolumeService) RemoveVolume(ctx context.Context, name string, force bool) error {
	args := []string{"volume", "rm"}
	if force {
		args = append(args, "-f")
	}
	args = append(args, name)

	_, exitCode, err := s.executor.RunCombined(ctx, "docker", args...)
	if err != nil || exitCode != 0 {
		return fmt.Errorf("docker volume rm failed: %v", err)
	}
	return nil
}

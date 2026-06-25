package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"easyserver/internal/executor"
	"easyserver/internal/model"
)

// NetworkService manages Docker networks
type NetworkService struct {
	executor executor.CommandExecutor
}

// NewNetworkService creates a new NetworkService
func NewNetworkService(exec executor.CommandExecutor) *NetworkService {
	return &NetworkService{executor: exec}
}

// ListNetworks returns all Docker networks
func (s *NetworkService) ListNetworks(ctx context.Context) ([]model.Network, error) {
	output, exitCode, err := s.executor.RunCombined(ctx, "docker", "network", "ls", "--format", "{{json .}}")
	if err != nil || exitCode != 0 {
		return nil, fmt.Errorf("docker network ls failed: %s", output)
	}

	var networks []model.Network
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		if line = strings.TrimSpace(line); line == "" {
			continue
		}
		var raw struct {
			ID      string `json:"ID"`
			Name    string `json:"Name"`
			Driver  string `json:"Driver"`
			Scope   string `json:"Scope"`
		}
		if err := json.Unmarshal([]byte(line), &raw); err != nil {
			continue
		}

		net := model.Network{
			ID:     raw.ID,
			Name:   raw.Name,
			Driver: raw.Driver,
			Scope:  raw.Scope,
		}

		// Get subnet and gateway by inspecting the network
		details := s.inspectNetwork(ctx, raw.ID)
		if details != nil {
			net.Subnet = details.Subnet
			net.Gateway = details.Gateway
		}

		networks = append(networks, net)
	}

	return networks, nil
}

type networkDetails struct {
	Subnet  string
	Gateway string
}

// inspectNetwork gets subnet and gateway for a network
func (s *NetworkService) inspectNetwork(ctx context.Context, id string) *networkDetails {
	output, exitCode, err := s.executor.RunCombined(ctx, "docker", "network", "inspect", "--format", "{{json .IPAM}}", id)
	if err != nil || exitCode != 0 {
		return nil
	}

	var ipam struct {
		Config []struct {
			Subnet  string `json:"Subnet"`
			Gateway string `json:"Gateway"`
		} `json:"Config"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &ipam); err != nil {
		return nil
	}

	if len(ipam.Config) > 0 {
		return &networkDetails{
			Subnet:  ipam.Config[0].Subnet,
			Gateway: ipam.Config[0].Gateway,
		}
	}
	return nil
}

// CreateNetwork creates a new Docker network
func (s *NetworkService) CreateNetwork(ctx context.Context, name, driver string) error {
	args := []string{"network", "create"}
	if driver != "" {
		args = append(args, "--driver", driver)
	}
	args = append(args, name)

	_, exitCode, err := s.executor.RunCombined(ctx, "docker", args...)
	if err != nil || exitCode != 0 {
		return fmt.Errorf("docker network create failed: %v", err)
	}
	return nil
}

// RemoveNetwork removes a Docker network
func (s *NetworkService) RemoveNetwork(ctx context.Context, id string) error {
	_, exitCode, err := s.executor.RunCombined(ctx, "docker", "network", "rm", id)
	if err != nil || exitCode != 0 {
		return fmt.Errorf("docker network rm failed: %v", err)
	}
	return nil
}

package service

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"easyserver/internal/executor"
	"easyserver/internal/model"
)

// ComposeService manages Docker Compose projects
type ComposeService struct {
	executor executor.CommandExecutor
}

// NewComposeService creates a new ComposeService
func NewComposeService(exec executor.CommandExecutor) *ComposeService {
	return &ComposeService{executor: exec}
}

// ListProjects lists all Docker Compose projects
func (s *ComposeService) ListProjects(ctx context.Context) ([]model.ComposeProject, error) {
	// Use docker compose ls --format json
	output, exitCode, err := s.executor.RunCombined(ctx, "docker", "compose", "ls", "--format", "json")
	if err != nil || exitCode != 0 {
		// Try older docker-compose command
		output, exitCode, err = s.executor.RunCombined(ctx, "docker-compose", "ls", "--format", "json")
		if err != nil || exitCode != 0 {
			return []model.ComposeProject{}, nil
		}
	}

	var projects []model.ComposeProject
	trimmed := strings.TrimSpace(output)
	if trimmed == "" {
		return []model.ComposeProject{}, nil
	}

	// docker compose ls may return one JSON object per line
	for _, line := range strings.Split(trimmed, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var raw struct {
			Name        string `json:"Name"`
			Status      string `json:"Status"`
			ConfigFiles string `json:"ConfigFiles"`
		}
		if err := json.Unmarshal([]byte(line), &raw); err != nil {
			continue
		}

		project := model.ComposeProject{
			Name:       raw.Name,
			Status:     raw.Status,
			ConfigFile: raw.ConfigFiles,
		}

		// Get services for this project
		services := s.getProjectServices(ctx, raw.Name, raw.ConfigFiles)
		project.Services = services

		projects = append(projects, project)
	}

	return projects, nil
}

// getProjectServices returns the list of services in a compose project
func (s *ComposeService) getProjectServices(ctx context.Context, name, configFile string) []string {
	// Parse services from config file
	if configFile == "" {
		return nil
	}

	output, exitCode, err := s.executor.RunCombined(ctx, "docker", "compose", "-f", configFile, "config", "--services")
	if err != nil || exitCode != 0 {
		return nil
	}

	var services []string
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			services = append(services, line)
		}
	}
	return services
}

// Up runs docker compose up -d for a project
func (s *ComposeService) Up(ctx context.Context, projectDir string) error {
	composeFile := s.findComposeFile(projectDir)
	args := s.composeArgs(composeFile, "up", "-d")

	output, exitCode, err := s.executor.RunCombined(ctx, "docker", args...)
	if err != nil || exitCode != 0 {
		return fmt.Errorf("compose up failed: %s", output)
	}
	return nil
}

// Down runs docker compose down for a project
func (s *ComposeService) Down(ctx context.Context, projectDir string) error {
	composeFile := s.findComposeFile(projectDir)
	args := s.composeArgs(composeFile, "down")

	output, exitCode, err := s.executor.RunCombined(ctx, "docker", args...)
	if err != nil || exitCode != 0 {
		return fmt.Errorf("compose down failed: %s", output)
	}
	return nil
}

// Restart runs docker compose restart for a project
func (s *ComposeService) Restart(ctx context.Context, projectDir string) error {
	composeFile := s.findComposeFile(projectDir)
	args := s.composeArgs(composeFile, "restart")

	output, exitCode, err := s.executor.RunCombined(ctx, "docker", args...)
	if err != nil || exitCode != 0 {
		return fmt.Errorf("compose restart failed: %s", output)
	}
	return nil
}

// GetLogs returns logs for a compose project
func (s *ComposeService) GetLogs(ctx context.Context, projectDir string, tail int) (string, error) {
	composeFile := s.findComposeFile(projectDir)
	args := s.composeArgs(composeFile, "logs", "--tail", fmt.Sprintf("%d", tail))

	output, exitCode, err := s.executor.RunCombined(ctx, "docker", args...)
	if err != nil || exitCode != 0 {
		return "", fmt.Errorf("compose logs failed: %s", output)
	}
	return output, nil
}

// GetConfig reads the docker-compose.yml content
func (s *ComposeService) GetConfig(ctx context.Context, projectDir string) (string, error) {
	composeFile := s.findComposeFile(projectDir)
	if composeFile == "" {
		return "", fmt.Errorf("no compose file found in %s", projectDir)
	}

	data, err := os.ReadFile(composeFile)
	if err != nil {
		return "", fmt.Errorf("read compose file: %w", err)
	}
	return string(data), nil
}

// SaveConfig writes content to docker-compose.yml
func (s *ComposeService) SaveConfig(ctx context.Context, projectDir, content string) error {
	composeFile := s.findComposeFile(projectDir)
	if composeFile == "" {
		// Default to docker-compose.yml
		composeFile = projectDir + "/docker-compose.yml"
	}

	if err := os.WriteFile(composeFile, []byte(content), 0644); err != nil {
		return fmt.Errorf("write compose file: %w", err)
	}
	return nil
}

// findComposeFile locates the compose file in a directory
func (s *ComposeService) findComposeFile(projectDir string) string {
	candidates := []string{
		"docker-compose.yml",
		"docker-compose.yaml",
		"compose.yml",
		"compose.yaml",
	}

	for _, name := range candidates {
		path := projectDir + "/" + name
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	return ""
}

// composeArgs builds the docker compose command args with optional -f flag
func (s *ComposeService) composeArgs(composeFile string, subcmd ...string) []string {
	args := []string{"compose"}
	if composeFile != "" {
		args = append(args, "-f", composeFile)
	}
	args = append(args, subcmd...)
	return args
}

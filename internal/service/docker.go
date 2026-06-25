package service

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"easyserver/internal/executor"
	"easyserver/internal/model"
)

// truncateOutput truncates output to maxLen characters
func truncateOutput(output string, maxLen int) string {
	if len(output) <= maxLen {
		return output
	}
	return output[:maxLen] + "..."
}

// DockerService manages Docker installation and system-level operations
type DockerService struct {
	executor executor.CommandExecutor
}

// NewDockerService creates a new DockerService
func NewDockerService(exec executor.CommandExecutor) *DockerService {
	return &DockerService{executor: exec}
}

// DetectDocker checks if Docker is installed, its version, compose version, running status, and OS
func (s *DockerService) DetectDocker(ctx context.Context) (*model.DockerStatus, error) {
	status := &model.DockerStatus{}

	// Detect OS
	status.OS = s.detectOS(ctx)

	// Check Docker version
	stdout, exitCode, err := s.executor.RunCombined(ctx, "docker", "version", "--format", "{{.Server.Version}}")
	if err != nil || exitCode != 0 {
		status.Installed = false
		return status, nil
	}
	status.Installed = true
	status.Version = strings.TrimSpace(stdout)

	// Check if Docker daemon is running
	_, exitCode, err = s.executor.RunCombined(ctx, "docker", "info", "--format", "{{.ServerVersion}}")
	if err != nil || exitCode != 0 {
		status.Running = false
	} else {
		status.Running = true
	}

	// Check compose version
	composeOut, exitCode, err := s.executor.RunCombined(ctx, "docker", "compose", "version", "--short")
	if err == nil && exitCode == 0 {
		status.ComposeVersion = strings.TrimSpace(composeOut)
	} else {
		// Try standalone docker-compose
		composeOut, exitCode, err = s.executor.RunCombined(ctx, "docker-compose", "version", "--short")
		if err == nil && exitCode == 0 {
			status.ComposeVersion = strings.TrimSpace(composeOut)
		}
	}

	return status, nil
}

// detectOS detects the Linux distribution
func (s *DockerService) detectOS(ctx context.Context) string {
	stdout, _, err := s.executor.RunCombined(ctx, "cat", "/etc/os-release")
	if err != nil {
		return "unknown"
	}

	lower := strings.ToLower(stdout)
	switch {
	case strings.Contains(lower, "debian"):
		return "debian"
	case strings.Contains(lower, "ubuntu"):
		return "ubuntu"
	case strings.Contains(lower, "centos"):
		return "centos"
	case strings.Contains(lower, "rhel") || strings.Contains(lower, "red hat"):
		return "rhel"
	case strings.Contains(lower, "fedora"):
		return "fedora"
	case strings.Contains(lower, "alpine"):
		return "alpine"
	case strings.Contains(lower, "arch"):
		return "arch"
	default:
		return "linux"
	}
}

// InstallDocker installs Docker using the official convenience script
func (s *DockerService) InstallDocker(ctx context.Context) error {
	log.Println("docker: starting installation...")

	// Step 1: Check if curl is available
	_, exitCode, err := s.executor.RunCombined(ctx, "which", "curl")
	if err != nil || exitCode != 0 {
		return fmt.Errorf("curl 未安装，请先安装 curl: %v", err)
	}

	// Step 2: Download and run the official Docker install script (10 min timeout)
	// Download to file first to allow inspection and prevent pipe-to-sh risks
	log.Println("docker: downloading install script...")
	output, _, exitCode, err := s.executor.RunWithTimeout(ctx, 2*time.Minute, "bash", "-c",
		"curl -fsSL https://get.docker.com -o /tmp/get-docker.sh")
	if err != nil || exitCode != 0 {
		return fmt.Errorf("下载 Docker 安装脚本失败 (exit=%d): %s", exitCode, truncateOutput(output, 500))
	}

	log.Println("docker: running install script...")
	output, _, exitCode, err = s.executor.RunWithTimeout(ctx, 10*time.Minute, "sh", "/tmp/get-docker.sh")
	if err != nil || exitCode != 0 {
		log.Printf("docker: installation failed: %s", output)
		return fmt.Errorf("Docker 安装脚本执行失败 (exit=%d): %s", exitCode, truncateOutput(output, 500))
	}
	log.Printf("docker: installation script completed")

	// Step 3: Enable Docker service
	log.Println("docker: enabling service...")
	output, exitCode, err = s.executor.RunCombined(ctx, "systemctl", "enable", "docker")
	if err != nil || exitCode != 0 {
		log.Printf("docker: enable failed: %s", output)
		return fmt.Errorf("启用 Docker 服务失败: %s", truncateOutput(output, 200))
	}

	// Step 4: Start Docker service
	log.Println("docker: starting service...")
	output, exitCode, err = s.executor.RunCombined(ctx, "systemctl", "start", "docker")
	if err != nil || exitCode != 0 {
		log.Printf("docker: start failed: %s", output)
		return fmt.Errorf("启动 Docker 服务失败: %s", truncateOutput(output, 200))
	}

	log.Println("docker: installation completed successfully")
	return nil
}

// StartDocker starts the Docker service
func (s *DockerService) StartDocker(ctx context.Context) error {
	output, exitCode, err := s.executor.RunCombined(ctx, "systemctl", "start", "docker")
	if err != nil || exitCode != 0 {
		return fmt.Errorf("failed to start docker: %s", output)
	}
	return nil
}

// StopDocker stops the Docker service
func (s *DockerService) StopDocker(ctx context.Context) error {
	output, exitCode, err := s.executor.RunCombined(ctx, "systemctl", "stop", "docker")
	if err != nil || exitCode != 0 {
		return fmt.Errorf("failed to stop docker: %s", output)
	}
	return nil
}

// RestartDocker restarts the Docker service
func (s *DockerService) RestartDocker(ctx context.Context) error {
	output, exitCode, err := s.executor.RunCombined(ctx, "systemctl", "restart", "docker")
	if err != nil || exitCode != 0 {
		return fmt.Errorf("failed to restart docker: %s", output)
	}
	return nil
}

// GetDockerInfo returns Docker system info as a map
func (s *DockerService) GetDockerInfo(ctx context.Context) (map[string]interface{}, error) {
	output, exitCode, err := s.executor.RunCombined(ctx, "docker", "info", "--format", "{{json .}}")
	if err != nil || exitCode != 0 {
		return nil, fmt.Errorf("docker info failed: %s", output)
	}

	var info map[string]interface{}
	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &info); err != nil {
		return nil, fmt.Errorf("parse docker info: %w", err)
	}

	return info, nil
}

// ConfigureMirror configures Docker registry mirror
func (s *DockerService) ConfigureMirror(ctx context.Context, mirrorURL string) error {
	// Read existing daemon.json
	existing := "{}"
	stdout, exitCode, err := s.executor.RunCombined(ctx, "cat", "/etc/docker/daemon.json")
	if err == nil && exitCode == 0 {
		existing = strings.TrimSpace(stdout)
		if existing == "" {
			existing = "{}"
		}
	}

	var config map[string]interface{}
	if err := json.Unmarshal([]byte(existing), &config); err != nil {
		config = make(map[string]interface{})
	}

	if mirrorURL == "" {
		// Remove mirrors
		delete(config, "registry-mirrors")
	} else {
		config["registry-mirrors"] = []string{mirrorURL}
	}

	newConfig, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	// Write new daemon.json using base64 encoding to avoid shell injection
	encoded := base64.StdEncoding.EncodeToString(newConfig)
	writeCmd := fmt.Sprintf("mkdir -p /etc/docker && echo '%s' | base64 -d > /etc/docker/daemon.json", encoded)
	_, exitCode, err = s.executor.RunCombined(ctx, "bash", "-c", writeCmd)
	if err != nil || exitCode != 0 {
		return fmt.Errorf("failed to write daemon.json: %v", err)
	}

	// Restart Docker to apply changes
	return s.RestartDocker(ctx)
}

package container

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"easyserver/internal/executor"
)

// 容器管理常量
const (
	ImagePullTimeout = 10 * time.Minute // 镜像拉取超时
	DefaultLogTail   = 100              // 默认日志行数
	MaxLogTail       = 10000            // 最大日志行数
)

// Service manages Docker containers, images, compose, volumes, and networks.
type Service struct {
	executor executor.CommandExecutor
}

// NewService creates a new container Service.
func NewService(exec executor.CommandExecutor) *Service {
	return &Service{executor: exec}
}

// --- Container operations ---

// CheckDocker checks if Docker is installed and accessible.
func (s *Service) CheckDocker(ctx context.Context) error {
	_, exitCode, err := s.executor.RunCombined(ctx, "docker", "version", "--format", "{{.Server.Version}}")
	if err != nil || exitCode != 0 {
		return fmt.Errorf("docker is not installed or not accessible")
	}
	return nil
}

// ListContainers returns all containers.
func (s *Service) ListContainers(ctx context.Context, all bool) ([]Container, error) {
	if err := s.CheckDocker(ctx); err != nil {
		return nil, err
	}

	args := []string{"ps", "--format", "{{json .}}"}
	if all {
		args = append(args, "-a")
	}

	output, exitCode, err := s.executor.RunCombined(ctx, "docker", args...)
	if err != nil || exitCode != 0 {
		return nil, fmt.Errorf("docker ps failed: %s", output)
	}

	var containers []Container
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		if line = strings.TrimSpace(line); line == "" {
			continue
		}
		var c Container
		if err := json.Unmarshal([]byte(line), &c); err != nil {
			log.Printf("container: parse container json error: %v, line: %s", err, line[:min(100, len(line))])
			continue
		}
		containers = append(containers, c)
	}

	return containers, nil
}

// GetContainer returns details of a specific container.
func (s *Service) GetContainer(ctx context.Context, id string) (*Container, error) {
	output, exitCode, err := s.executor.RunCombined(ctx, "docker", "inspect", "--format", "{{json .}}", id)
	if err != nil || exitCode != 0 {
		return nil, fmt.Errorf("docker inspect failed: %s", output)
	}

	var containers []Container
	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &containers); err != nil {
		var c Container
		if err2 := json.Unmarshal([]byte(strings.TrimSpace(output)), &c); err2 != nil {
			return nil, fmt.Errorf("parse container: %w", err2)
		}
		return &c, nil
	}

	if len(containers) == 0 {
		return nil, fmt.Errorf("container not found: %s", id)
	}

	return &containers[0], nil
}

func (s *Service) containerAction(ctx context.Context, action, id string) error {
	_, exitCode, err := s.executor.RunCombined(ctx, "docker", action, id)
	if err != nil || exitCode != 0 {
		return fmt.Errorf("docker %s failed: %v", action, err)
	}
	return nil
}

// StartContainer starts a container.
func (s *Service) StartContainer(ctx context.Context, id string) error {
	return s.containerAction(ctx, "start", id)
}

// StopContainer stops a container.
func (s *Service) StopContainer(ctx context.Context, id string) error {
	return s.containerAction(ctx, "stop", id)
}

// RestartContainer restarts a container.
func (s *Service) RestartContainer(ctx context.Context, id string) error {
	return s.containerAction(ctx, "restart", id)
}

// PauseContainer pauses a container.
func (s *Service) PauseContainer(ctx context.Context, id string) error {
	return s.containerAction(ctx, "pause", id)
}

// UnpauseContainer unpauses a container.
func (s *Service) UnpauseContainer(ctx context.Context, id string) error {
	return s.containerAction(ctx, "unpause", id)
}

// RemoveContainer removes a container.
func (s *Service) RemoveContainer(ctx context.Context, id string, force bool) error {
	args := []string{"rm"}
	if force {
		args = append(args, "-f")
	}
	args = append(args, id)

	_, exitCode, err := s.executor.RunCombined(ctx, "docker", args...)
	if err != nil || exitCode != 0 {
		return fmt.Errorf("docker rm failed: %v", err)
	}
	return nil
}

// GetContainerLogs returns container logs.
func (s *Service) GetContainerLogs(ctx context.Context, id string, tail int) (string, error) {
	args := []string{"logs", "--tail", fmt.Sprintf("%d", tail), id}
	output, exitCode, err := s.executor.RunCombined(ctx, "docker", args...)
	if err != nil || exitCode != 0 {
		return "", fmt.Errorf("docker logs failed: %s", output)
	}
	return output, nil
}

// ExecInContainer executes a command in a running container.
func (s *Service) ExecInContainer(ctx context.Context, id string, cmd string) (string, error) {
	if strings.ContainsRune(cmd, '\x00') {
		return "", fmt.Errorf("command contains null byte")
	}
	const maxCmdLen = 4096
	if len(cmd) > maxCmdLen {
		return "", fmt.Errorf("command exceeds maximum length (%d bytes)", maxCmdLen)
	}
	if strings.TrimSpace(cmd) == "" {
		return "", fmt.Errorf("command cannot be empty")
	}

	output, exitCode, err := s.executor.RunCombined(ctx, "docker", "exec", id, "sh", "-c", cmd)
	if err != nil || exitCode != 0 {
		return output, fmt.Errorf("docker exec failed: %s", output)
	}
	return output, nil
}

// CreateContainer creates a new container.
func (s *Service) CreateContainer(ctx context.Context, req CreateRequest) (string, error) {
	args := []string{"create"}

	if req.Name != "" {
		args = append(args, "--name", req.Name)
	}

	for _, p := range req.Ports {
		args = append(args, "-p", fmt.Sprintf("%s:%s/%s", p.HostPort, p.ContainerPort, p.Protocol))
	}

	for k, v := range req.EnvVars {
		args = append(args, "-e", fmt.Sprintf("%s=%s", k, v))
	}

	for _, v := range req.Volumes {
		mode := ""
		if v.Mode != "" {
			mode = ":" + v.Mode
		}
		args = append(args, "-v", fmt.Sprintf("%s:%s%s", v.Source, v.Destination, mode))
	}

	for _, n := range req.Networks {
		args = append(args, "--network", n)
	}

	if req.RestartPolicy != "" {
		args = append(args, "--restart", req.RestartPolicy)
	}

	for k, v := range req.Labels {
		args = append(args, "--label", fmt.Sprintf("%s=%s", k, v))
	}

	if req.AutoRemove {
		args = append(args, "--rm")
	}

	if req.Memory > 0 {
		args = append(args, "--memory", fmt.Sprintf("%d", req.Memory))
	}
	if req.CPUs > 0 {
		args = append(args, "--cpus", fmt.Sprintf("%.2f", req.CPUs))
	}

	args = append(args, req.Image)

	if req.Command != "" {
		args = append(args, strings.Fields(req.Command)...)
	}

	output, exitCode, err := s.executor.RunCombined(ctx, "docker", args...)
	if err != nil || exitCode != 0 {
		return "", fmt.Errorf("docker create failed: %s", output)
	}

	return strings.TrimSpace(output), nil
}

// ListImages returns all Docker images.
func (s *Service) ListImages(ctx context.Context) ([]Image, error) {
	if err := s.CheckDocker(ctx); err != nil {
		return nil, err
	}

	output, exitCode, err := s.executor.RunCombined(ctx, "docker", "images", "--format", "{{json .}}")
	if err != nil || exitCode != 0 {
		return nil, fmt.Errorf("docker images failed: %s", output)
	}

	var images []Image
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		if line = strings.TrimSpace(line); line == "" {
			continue
		}
		var img Image
		if err := json.Unmarshal([]byte(line), &img); err != nil {
			log.Printf("container: parse image json error: %v, line: %s", err, line[:min(100, len(line))])
			continue
		}
		images = append(images, img)
	}

	return images, nil
}

// PullImage pulls a Docker image.
func (s *Service) PullImage(ctx context.Context, image string) error {
	_, _, exitCode, err := s.executor.RunWithTimeout(ctx, ImagePullTimeout, "docker", "pull", image)
	if err != nil || exitCode != 0 {
		return fmt.Errorf("docker pull failed: %v", err)
	}
	return nil
}

// RemoveImage removes a Docker image.
func (s *Service) RemoveImage(ctx context.Context, id string, force bool) error {
	args := []string{"rmi"}
	if force {
		args = append(args, "-f")
	}
	args = append(args, id)

	_, exitCode, err := s.executor.RunCombined(ctx, "docker", args...)
	if err != nil || exitCode != 0 {
		return fmt.Errorf("docker rmi failed: %v", err)
	}
	return nil
}

// GetContainerStats returns real-time resource usage stats for a container.
func (s *Service) GetContainerStats(ctx context.Context, id string) (*Stats, error) {
	output, exitCode, err := s.executor.RunCombined(ctx, "docker", "stats", id, "--no-stream", "--format",
		`{"cpu_percent":"{{.CPUPerc}}","mem_usage":"{{.MemUsage}}","mem_percent":"{{.MemPerc}}","net_rx":"{{.NetIO}}","block_read":"{{.BlockIO}}","pids":"{{.PIDs}}"}`)
	if err != nil || exitCode != 0 {
		return nil, fmt.Errorf("docker stats failed: %s", output)
	}

	var raw struct {
		CPUPercent string `json:"cpu_percent"`
		MemUsage   string `json:"mem_usage"`
		MemPercent string `json:"mem_percent"`
		NetRx      string `json:"net_rx"`
		BlockRead  string `json:"block_read"`
		PIDs       string `json:"pids"`
	}

	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &raw); err != nil {
		return nil, fmt.Errorf("parse stats: %w", err)
	}

	stats := &Stats{}

	cpuStr := strings.TrimSuffix(raw.CPUPercent, "%")
	if v, err := strconv.ParseFloat(cpuStr, 64); err == nil {
		stats.CPUPercent = v
	}

	memParts := strings.Split(raw.MemUsage, " / ")
	if len(memParts) == 2 {
		stats.MemUsage = parseBytes(strings.TrimSpace(memParts[0]))
		stats.MemLimit = parseBytes(strings.TrimSpace(memParts[1]))
	}

	memPctStr := strings.TrimSuffix(raw.MemPercent, "%")
	if v, err := strconv.ParseFloat(memPctStr, 64); err == nil {
		stats.MemPercent = v
	}

	netParts := strings.Split(raw.NetRx, " / ")
	if len(netParts) == 2 {
		stats.NetRx = parseBytes(strings.TrimSpace(netParts[0]))
		stats.NetTx = parseBytes(strings.TrimSpace(netParts[1]))
	}

	blockParts := strings.Split(raw.BlockRead, " / ")
	if len(blockParts) == 2 {
		stats.BlockRead = parseBytes(strings.TrimSpace(blockParts[0]))
		stats.BlockWrite = parseBytes(strings.TrimSpace(blockParts[1]))
	}

	if v, err := strconv.Atoi(raw.PIDs); err == nil {
		stats.PIDs = v
	}

	return stats, nil
}

func parseBytes(s string) int64 {
	s = strings.TrimSpace(s)
	if s == "" || s == "--" {
		return 0
	}

	var numStr string
	var unit string
	for i, c := range s {
		if (c >= '0' && c <= '9') || c == '.' {
			numStr += string(c)
		} else {
			unit = s[i:]
			break
		}
	}

	if numStr == "" {
		return 0
	}

	val, err := strconv.ParseFloat(numStr, 64)
	if err != nil {
		return 0
	}

	unit = strings.TrimSpace(strings.ToUpper(unit))
	switch unit {
	case "B", "":
		return int64(val)
	case "KB", "KIB":
		return int64(val * 1024)
	case "MB", "MIB":
		return int64(val * 1024 * 1024)
	case "GB", "GIB":
		return int64(val * 1024 * 1024 * 1024)
	case "TB", "TIB":
		return int64(val * 1024 * 1024 * 1024 * 1024)
	default:
		return int64(val)
	}
}

// GetContainerTop returns the list of processes running inside a container.
func (s *Service) GetContainerTop(ctx context.Context, id string) ([]ProcessInfo, error) {
	output, exitCode, err := s.executor.RunCombined(ctx, "docker", "top", id, "-eo", "user,pid,ppid,%cpu,%mem,vsz,rss,tty,stat,start,time,comm")
	if err != nil || exitCode != 0 {
		return nil, fmt.Errorf("docker top failed: %s", output)
	}

	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) < 2 {
		return []ProcessInfo{}, nil
	}

	var processes []ProcessInfo
	for _, line := range lines[1:] {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) < 12 {
			continue
		}
		processes = append(processes, ProcessInfo{
			User:    fields[0],
			PID:     fields[1],
			PPID:    fields[2],
			CPU:     fields[3],
			MEM:     fields[4],
			VSZ:     fields[5],
			RSS:     fields[6],
			TTY:     fields[7],
			Stat:    fields[8],
			Start:   fields[9],
			Time:    fields[10],
			Command: strings.Join(fields[11:], " "),
		})
	}

	return processes, nil
}

// CopyToContainer copies a file from host to container.
func (s *Service) CopyToContainer(ctx context.Context, id, srcPath, destPath string) error {
	_, exitCode, err := s.executor.RunCombined(ctx, "docker", "cp", srcPath, id+":"+destPath)
	if err != nil || exitCode != 0 {
		return fmt.Errorf("docker cp to container failed: %v", err)
	}
	return nil
}

// CopyFromContainer copies a file from container to host.
func (s *Service) CopyFromContainer(ctx context.Context, id, srcPath, destPath string) error {
	_, exitCode, err := s.executor.RunCombined(ctx, "docker", "cp", id+":"+srcPath, destPath)
	if err != nil || exitCode != 0 {
		return fmt.Errorf("docker cp from container failed: %v", err)
	}
	return nil
}

// RenameContainer renames a container.
func (s *Service) RenameContainer(ctx context.Context, id, newName string) error {
	if strings.TrimSpace(id) == "" {
		return fmt.Errorf("container ID cannot be empty")
	}
	if strings.TrimSpace(newName) == "" {
		return fmt.Errorf("new container name cannot be empty")
	}
	if len(newName) > 128 {
		return fmt.Errorf("container name too long (max 128 characters)")
	}
	for i, ch := range newName {
		if !((ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') || ch == '_' || ch == '.' || ch == '-') {
			return fmt.Errorf("invalid character '%c' in container name at position %d", ch, i)
		}
	}
	if newName[0] == '.' || newName[0] == '-' {
		return fmt.Errorf("container name cannot start with '%c'", newName[0])
	}

	_, exitCode, err := s.executor.RunCombined(ctx, "docker", "rename", id, newName)
	if err != nil || exitCode != 0 {
		return fmt.Errorf("docker rename failed: %v", err)
	}
	return nil
}

// UpdateContainer updates container resource limits.
func (s *Service) UpdateContainer(ctx context.Context, id string, req UpdateRequest) error {
	args := []string{"update"}

	if req.Memory > 0 {
		args = append(args, "--memory", fmt.Sprintf("%d", req.Memory))
	}
	if req.CPUs > 0 {
		args = append(args, "--cpus", fmt.Sprintf("%.2f", req.CPUs))
	}
	if req.Restart != "" {
		args = append(args, "--restart", req.Restart)
	}

	args = append(args, id)

	output, exitCode, err := s.executor.RunCombined(ctx, "docker", args...)
	if err != nil || exitCode != 0 {
		return fmt.Errorf("docker update failed: %s", output)
	}
	return nil
}

// --- Docker system operations ---

// DetectDocker checks if Docker is installed, its version, compose version, running status, and OS.
func (s *Service) DetectDocker(ctx context.Context) (*DockerStatus, error) {
	status := &DockerStatus{}

	status.OS = s.detectOS(ctx)

	stdout, exitCode, err := s.executor.RunCombined(ctx, "docker", "version", "--format", "{{.Server.Version}}")
	if err != nil || exitCode != 0 {
		status.Installed = false
		return status, nil
	}
	status.Installed = true
	status.Version = strings.TrimSpace(stdout)

	_, exitCode, err = s.executor.RunCombined(ctx, "docker", "info", "--format", "{{.ServerVersion}}")
	if err != nil || exitCode != 0 {
		status.Running = false
	} else {
		status.Running = true
	}

	composeOut, exitCode, err := s.executor.RunCombined(ctx, "docker", "compose", "version", "--short")
	if err == nil && exitCode == 0 {
		status.ComposeVersion = strings.TrimSpace(composeOut)
	} else {
		composeOut, exitCode, err = s.executor.RunCombined(ctx, "docker-compose", "version", "--short")
		if err == nil && exitCode == 0 {
			status.ComposeVersion = strings.TrimSpace(composeOut)
		}
	}

	return status, nil
}

func (s *Service) detectOS(ctx context.Context) string {
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

// InstallDocker installs Docker using the official convenience script.
func (s *Service) InstallDocker(ctx context.Context) error {
	log.Println("docker: starting installation...")

	_, exitCode, err := s.executor.RunCombined(ctx, "which", "curl")
	if err != nil || exitCode != 0 {
		return fmt.Errorf("curl 未安装，请先安装 curl: %v", err)
	}

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

	log.Println("docker: enabling service...")
	output, exitCode, err = s.executor.RunCombined(ctx, "systemctl", "enable", "docker")
	if err != nil || exitCode != 0 {
		log.Printf("docker: enable failed: %s", output)
		return fmt.Errorf("启用 Docker 服务失败: %s", truncateOutput(output, 200))
	}

	log.Println("docker: starting service...")
	output, exitCode, err = s.executor.RunCombined(ctx, "systemctl", "start", "docker")
	if err != nil || exitCode != 0 {
		log.Printf("docker: start failed: %s", output)
		return fmt.Errorf("启动 Docker 服务失败: %s", truncateOutput(output, 200))
	}

	log.Println("docker: installation completed successfully")
	return nil
}

// StartDocker starts the Docker service.
func (s *Service) StartDocker(ctx context.Context) error {
	output, exitCode, err := s.executor.RunCombined(ctx, "systemctl", "start", "docker")
	if err != nil || exitCode != 0 {
		return fmt.Errorf("failed to start docker: %s", output)
	}
	return nil
}

// StopDocker stops the Docker service.
func (s *Service) StopDocker(ctx context.Context) error {
	output, exitCode, err := s.executor.RunCombined(ctx, "systemctl", "stop", "docker")
	if err != nil || exitCode != 0 {
		return fmt.Errorf("failed to stop docker: %s", output)
	}
	return nil
}

// RestartDocker restarts the Docker service.
func (s *Service) RestartDocker(ctx context.Context) error {
	output, exitCode, err := s.executor.RunCombined(ctx, "systemctl", "restart", "docker")
	if err != nil || exitCode != 0 {
		return fmt.Errorf("failed to restart docker: %s", output)
	}
	return nil
}

// GetDockerInfo returns Docker system info as a map.
func (s *Service) GetDockerInfo(ctx context.Context) (map[string]interface{}, error) {
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

// ConfigureMirror configures Docker registry mirror.
func (s *Service) ConfigureMirror(ctx context.Context, mirrorURL string) error {
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
		delete(config, "registry-mirrors")
	} else {
		config["registry-mirrors"] = []string{mirrorURL}
	}

	newConfig, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	encoded := base64.StdEncoding.EncodeToString(newConfig)
	writeCmd := fmt.Sprintf("mkdir -p /etc/docker && echo '%s' | base64 -d > /etc/docker/daemon.json", encoded)
	_, exitCode, err = s.executor.RunCombined(ctx, "bash", "-c", writeCmd)
	if err != nil || exitCode != 0 {
		return fmt.Errorf("failed to write daemon.json: %v", err)
	}

	return s.RestartDocker(ctx)
}

func truncateOutput(output string, maxLen int) string {
	if len(output) <= maxLen {
		return output
	}
	return output[:maxLen] + "..."
}

// --- Compose operations ---

// ListProjects lists all Docker Compose projects.
func (s *Service) ListProjects(ctx context.Context) ([]ComposeProject, error) {
	output, exitCode, err := s.executor.RunCombined(ctx, "docker", "compose", "ls", "--format", "json")
	if err != nil || exitCode != 0 {
		output, exitCode, err = s.executor.RunCombined(ctx, "docker-compose", "ls", "--format", "json")
		if err != nil || exitCode != 0 {
			return []ComposeProject{}, nil
		}
	}

	var projects []ComposeProject
	trimmed := strings.TrimSpace(output)
	if trimmed == "" {
		return []ComposeProject{}, nil
	}

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

		project := ComposeProject{
			Name:       raw.Name,
			Status:     raw.Status,
			ConfigFile: raw.ConfigFiles,
		}

		services := s.getProjectServices(ctx, raw.Name, raw.ConfigFiles)
		project.Services = services

		projects = append(projects, project)
	}

	return projects, nil
}

func (s *Service) getProjectServices(ctx context.Context, name, configFile string) []string {
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

// ComposeUp runs docker compose up -d for a project.
func (s *Service) ComposeUp(ctx context.Context, projectDir string) error {
	composeFile := s.findComposeFile(projectDir)
	args := s.composeArgs(composeFile, "up", "-d")

	output, exitCode, err := s.executor.RunCombined(ctx, "docker", args...)
	if err != nil || exitCode != 0 {
		return fmt.Errorf("compose up failed: %s", output)
	}
	return nil
}

// ComposeDown runs docker compose down for a project.
func (s *Service) ComposeDown(ctx context.Context, projectDir string) error {
	composeFile := s.findComposeFile(projectDir)
	args := s.composeArgs(composeFile, "down")

	output, exitCode, err := s.executor.RunCombined(ctx, "docker", args...)
	if err != nil || exitCode != 0 {
		return fmt.Errorf("compose down failed: %s", output)
	}
	return nil
}

// ComposeRestart runs docker compose restart for a project.
func (s *Service) ComposeRestart(ctx context.Context, projectDir string) error {
	composeFile := s.findComposeFile(projectDir)
	args := s.composeArgs(composeFile, "restart")

	output, exitCode, err := s.executor.RunCombined(ctx, "docker", args...)
	if err != nil || exitCode != 0 {
		return fmt.Errorf("compose restart failed: %s", output)
	}
	return nil
}

// ComposeGetLogs returns logs for a compose project.
func (s *Service) ComposeGetLogs(ctx context.Context, projectDir string, tail int) (string, error) {
	composeFile := s.findComposeFile(projectDir)
	args := s.composeArgs(composeFile, "logs", "--tail", fmt.Sprintf("%d", tail))

	output, exitCode, err := s.executor.RunCombined(ctx, "docker", args...)
	if err != nil || exitCode != 0 {
		return "", fmt.Errorf("compose logs failed: %s", output)
	}
	return output, nil
}

// ComposeGetConfig reads the docker-compose.yml content.
func (s *Service) ComposeGetConfig(ctx context.Context, projectDir string) (string, error) {
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

// ComposeSaveConfig writes content to docker-compose.yml.
func (s *Service) ComposeSaveConfig(ctx context.Context, projectDir, content string) error {
	composeFile := s.findComposeFile(projectDir)
	if composeFile == "" {
		composeFile = projectDir + "/docker-compose.yml"
	}

	if err := os.WriteFile(composeFile, []byte(content), 0644); err != nil {
		return fmt.Errorf("write compose file: %w", err)
	}
	return nil
}

func (s *Service) findComposeFile(projectDir string) string {
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

func (s *Service) composeArgs(composeFile string, subcmd ...string) []string {
	args := []string{"compose"}
	if composeFile != "" {
		args = append(args, "-f", composeFile)
	}
	args = append(args, subcmd...)
	return args
}

// --- Volume operations ---

// ListVolumes returns all Docker volumes.
func (s *Service) ListVolumes(ctx context.Context) ([]Volume, error) {
	output, exitCode, err := s.executor.RunCombined(ctx, "docker", "volume", "ls", "--format", "{{json .}}")
	if err != nil || exitCode != 0 {
		return nil, fmt.Errorf("docker volume ls failed: %s", output)
	}

	var volumes []Volume
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
		volumes = append(volumes, Volume{
			Name:       raw.Name,
			Driver:     raw.Driver,
			Mountpoint: raw.Mountpoint,
			CreatedAt:  raw.CreatedAt,
		})
	}

	return volumes, nil
}

// CreateVolume creates a new Docker volume.
func (s *Service) CreateVolume(ctx context.Context, name, driver string) error {
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

// RemoveVolume removes a Docker volume.
func (s *Service) RemoveVolume(ctx context.Context, name string, force bool) error {
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

// --- Network operations ---

type networkDetails struct {
	Subnet  string
	Gateway string
}

// ListNetworks returns all Docker networks.
func (s *Service) ListNetworks(ctx context.Context) ([]Network, error) {
	output, exitCode, err := s.executor.RunCombined(ctx, "docker", "network", "ls", "--format", "{{json .}}")
	if err != nil || exitCode != 0 {
		return nil, fmt.Errorf("docker network ls failed: %s", output)
	}

	var networks []Network
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		if line = strings.TrimSpace(line); line == "" {
			continue
		}
		var raw struct {
			ID     string `json:"ID"`
			Name   string `json:"Name"`
			Driver string `json:"Driver"`
			Scope  string `json:"Scope"`
		}
		if err := json.Unmarshal([]byte(line), &raw); err != nil {
			continue
		}

		net := Network{
			ID:     raw.ID,
			Name:   raw.Name,
			Driver: raw.Driver,
			Scope:  raw.Scope,
		}

		details := s.inspectNetwork(ctx, raw.ID)
		if details != nil {
			net.Subnet = details.Subnet
			net.Gateway = details.Gateway
		}

		networks = append(networks, net)
	}

	return networks, nil
}

func (s *Service) inspectNetwork(ctx context.Context, id string) *networkDetails {
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

// CreateNetwork creates a new Docker network.
func (s *Service) CreateNetwork(ctx context.Context, name, driver string) error {
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

// RemoveNetwork removes a Docker network.
func (s *Service) RemoveNetwork(ctx context.Context, id string) error {
	_, exitCode, err := s.executor.RunCombined(ctx, "docker", "network", "rm", id)
	if err != nil || exitCode != 0 {
		return fmt.Errorf("docker network rm failed: %v", err)
	}
	return nil
}

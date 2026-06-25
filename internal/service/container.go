package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"easyserver/internal/executor"
	"easyserver/internal/model"
)

// 容器管理常量
const (
	ImagePullTimeout = 10 * time.Minute // 镜像拉取超时
	DefaultLogTail   = 100              // 默认日志行数
	MaxLogTail       = 10000            // 最大日志行数
)

// ContainerService manages Docker containers
type ContainerService struct {
	executor executor.CommandExecutor
}

// NewContainerService creates a new ContainerService
func NewContainerService(exec executor.CommandExecutor) *ContainerService {
	return &ContainerService{executor: exec}
}

// CheckDocker checks if Docker is installed and accessible
func (s *ContainerService) CheckDocker(ctx context.Context) error {
	_, exitCode, err := s.executor.RunCombined(ctx, "docker", "version", "--format", "{{.Server.Version}}")
	if err != nil || exitCode != 0 {
		return fmt.Errorf("docker is not installed or not accessible")
	}
	return nil
}

// ListContainers returns all containers
func (s *ContainerService) ListContainers(ctx context.Context, all bool) ([]model.Container, error) {
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

	var containers []model.Container
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		if line = strings.TrimSpace(line); line == "" {
			continue
		}
		var c model.Container
		if err := json.Unmarshal([]byte(line), &c); err != nil {
			log.Printf("container: parse container json error: %v, line: %s", err, line[:min(100, len(line))])
			continue
		}
		containers = append(containers, c)
	}

	return containers, nil
}

// GetContainer returns details of a specific container
func (s *ContainerService) GetContainer(ctx context.Context, id string) (*model.Container, error) {
	output, exitCode, err := s.executor.RunCombined(ctx, "docker", "inspect", "--format", "{{json .}}", id)
	if err != nil || exitCode != 0 {
		return nil, fmt.Errorf("docker inspect failed: %s", output)
	}

	var containers []model.Container
	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &containers); err != nil {
		var c model.Container
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

// containerAction performs a Docker container action (start, stop, restart, pause, unpause)
func (s *ContainerService) containerAction(ctx context.Context, action, id string) error {
	_, exitCode, err := s.executor.RunCombined(ctx, "docker", action, id)
	if err != nil || exitCode != 0 {
		return fmt.Errorf("docker %s failed: %v", action, err)
	}
	return nil
}

// StartContainer starts a container
func (s *ContainerService) StartContainer(ctx context.Context, id string) error {
	return s.containerAction(ctx, "start", id)
}

// StopContainer stops a container
func (s *ContainerService) StopContainer(ctx context.Context, id string) error {
	return s.containerAction(ctx, "stop", id)
}

// RestartContainer restarts a container
func (s *ContainerService) RestartContainer(ctx context.Context, id string) error {
	return s.containerAction(ctx, "restart", id)
}

// PauseContainer pauses a container
func (s *ContainerService) PauseContainer(ctx context.Context, id string) error {
	return s.containerAction(ctx, "pause", id)
}

// UnpauseContainer unpauses a container
func (s *ContainerService) UnpauseContainer(ctx context.Context, id string) error {
	return s.containerAction(ctx, "unpause", id)
}

// RemoveContainer removes a container
func (s *ContainerService) RemoveContainer(ctx context.Context, id string, force bool) error {
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

// GetContainerLogs returns container logs
func (s *ContainerService) GetContainerLogs(ctx context.Context, id string, tail int) (string, error) {
	args := []string{"logs", "--tail", fmt.Sprintf("%d", tail), id}
	output, exitCode, err := s.executor.RunCombined(ctx, "docker", args...)
	if err != nil || exitCode != 0 {
		return "", fmt.Errorf("docker logs failed: %s", output)
	}
	return output, nil
}

// ExecInContainer executes a command in a running container
func (s *ContainerService) ExecInContainer(ctx context.Context, id string, cmd string) (string, error) {
	// Sanitize: reject null bytes and enforce max length to prevent injection
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

// CreateContainer creates a new container
func (s *ContainerService) CreateContainer(ctx context.Context, req model.CreateContainerRequest) (string, error) {
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

// ListImages returns all Docker images
func (s *ContainerService) ListImages(ctx context.Context) ([]model.Image, error) {
	if err := s.CheckDocker(ctx); err != nil {
		return nil, err
	}

	output, exitCode, err := s.executor.RunCombined(ctx, "docker", "images", "--format", "{{json .}}")
	if err != nil || exitCode != 0 {
		return nil, fmt.Errorf("docker images failed: %s", output)
	}

	var images []model.Image
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		if line = strings.TrimSpace(line); line == "" {
			continue
		}
		var img model.Image
		if err := json.Unmarshal([]byte(line), &img); err != nil {
			log.Printf("container: parse image json error: %v, line: %s", err, line[:min(100, len(line))])
			continue
		}
		images = append(images, img)
	}

	return images, nil
}

// PullImage pulls a Docker image
func (s *ContainerService) PullImage(ctx context.Context, image string) error {
	// docker pull 可能需要较长时间
	_, _, exitCode, err := s.executor.RunWithTimeout(ctx, ImagePullTimeout, "docker", "pull", image)
	if err != nil || exitCode != 0 {
		return fmt.Errorf("docker pull failed: %v", err)
	}
	return nil
}

// RemoveImage removes a Docker image
func (s *ContainerService) RemoveImage(ctx context.Context, id string, force bool) error {
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

// GetContainerStats returns real-time resource usage stats for a container
func (s *ContainerService) GetContainerStats(ctx context.Context, id string) (*model.ContainerStats, error) {
	// Use --no-stream for a single snapshot
	output, exitCode, err := s.executor.RunCombined(ctx, "docker", "stats", id, "--no-stream", "--format",
		`{"cpu_percent":"{{.CPUPerc}}","mem_usage":"{{.MemUsage}}","mem_percent":"{{.MemPerc}}","net_rx":"{{.NetIO}}","block_read":"{{.BlockIO}}","pids":"{{.PIDs}}"}`)
	if err != nil || exitCode != 0 {
		return nil, fmt.Errorf("docker stats failed: %s", output)
	}

	// Parse the JSON output
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

	stats := &model.ContainerStats{}

	// Parse CPU percent (e.g., "1.23%")
	cpuStr := strings.TrimSuffix(raw.CPUPercent, "%")
	if v, err := strconv.ParseFloat(cpuStr, 64); err == nil {
		stats.CPUPercent = v
	}

	// Parse memory usage (e.g., "123.4MiB / 1.5GiB")
	memParts := strings.Split(raw.MemUsage, " / ")
	if len(memParts) == 2 {
		stats.MemUsage = parseBytes(strings.TrimSpace(memParts[0]))
		stats.MemLimit = parseBytes(strings.TrimSpace(memParts[1]))
	}

	// Parse memory percent
	memPctStr := strings.TrimSuffix(raw.MemPercent, "%")
	if v, err := strconv.ParseFloat(memPctStr, 64); err == nil {
		stats.MemPercent = v
	}

	// Parse network IO (e.g., "1.23kB / 4.56kB")
	netParts := strings.Split(raw.NetRx, " / ")
	if len(netParts) == 2 {
		stats.NetRx = parseBytes(strings.TrimSpace(netParts[0]))
		stats.NetTx = parseBytes(strings.TrimSpace(netParts[1]))
	}

	// Parse block IO (e.g., "1.23MB / 4.56kB")
	blockParts := strings.Split(raw.BlockRead, " / ")
	if len(blockParts) == 2 {
		stats.BlockRead = parseBytes(strings.TrimSpace(blockParts[0]))
		stats.BlockWrite = parseBytes(strings.TrimSpace(blockParts[1]))
	}

	// Parse PIDs
	if v, err := strconv.Atoi(raw.PIDs); err == nil {
		stats.PIDs = v
	}

	return stats, nil
}

// parseBytes parses Docker's byte format (e.g., "123.4MiB", "1.5GiB", "100B")
func parseBytes(s string) int64 {
	s = strings.TrimSpace(s)
	if s == "" || s == "--" {
		return 0
	}

	// Extract numeric part and unit
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

// GetContainerTop returns the list of processes running inside a container
func (s *ContainerService) GetContainerTop(ctx context.Context, id string) ([]model.ContainerProcessInfo, error) {
	output, exitCode, err := s.executor.RunCombined(ctx, "docker", "top", id, "-eo", "user,pid,ppid,%cpu,%mem,vsz,rss,tty,stat,start,time,comm")
	if err != nil || exitCode != 0 {
		return nil, fmt.Errorf("docker top failed: %s", output)
	}

	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) < 2 {
		return []model.ContainerProcessInfo{}, nil
	}

	var processes []model.ContainerProcessInfo
	for _, line := range lines[1:] { // skip header
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) < 12 {
			continue
		}
		processes = append(processes, model.ContainerProcessInfo{
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

// CopyToContainer copies a file from host to container
func (s *ContainerService) CopyToContainer(ctx context.Context, id, srcPath, destPath string) error {
	_, exitCode, err := s.executor.RunCombined(ctx, "docker", "cp", srcPath, id+":"+destPath)
	if err != nil || exitCode != 0 {
		return fmt.Errorf("docker cp to container failed: %v", err)
	}
	return nil
}

// CopyFromContainer copies a file from container to host
func (s *ContainerService) CopyFromContainer(ctx context.Context, id, srcPath, destPath string) error {
	_, exitCode, err := s.executor.RunCombined(ctx, "docker", "cp", id+":"+srcPath, destPath)
	if err != nil || exitCode != 0 {
		return fmt.Errorf("docker cp from container failed: %v", err)
	}
	return nil
}

// RenameContainer renames a container
func (s *ContainerService) RenameContainer(ctx context.Context, id, newName string) error {
	// Validate container ID
	if strings.TrimSpace(id) == "" {
		return fmt.Errorf("container ID cannot be empty")
	}
	// Validate new name: Docker container names must match [a-zA-Z0-9][a-zA-Z0-9_.-]+
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

// UpdateContainer updates container resource limits
func (s *ContainerService) UpdateContainer(ctx context.Context, id string, req model.UpdateContainerRequest) error {
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

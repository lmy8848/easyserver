package container

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"easyserver/internal/infra/executor"
)

// 容器管理常量
const (
	ImagePullTimeout = 10 * time.Minute // 镜像拉取超时
	DefaultLogTail   = 100              // 默认日志行数
	MaxLogTail       = 10000            // 最大日志行数
)

// portMappingRE parses a single `docker ps` port token:
//
//	[ip:]hostPort->containerPort/protocol
var portMappingRE = regexp.MustCompile(`^(?:(.*?):)?(\d+)->(\d+)/(.+)$`)

// engineBinary maps a engine name to the CLI binary that manages it.
func engineBinary(engine Engine) string {
	if engine == "podman" {
		return "podman"
	}
	return "docker"
}

// Service manages Docker containers, images, compose, volumes, and networks.
type Service struct {
	executor executor.CommandExecutor
}

// NewService creates a new container Service.
func NewService(exec executor.CommandExecutor) *Service {
	return &Service{executor: exec}
}

// dockerPSRow mirrors the uppercase-keyed JSON emitted by `docker ps --format json`,
// used as an unmarshal-only shim. toContainer() maps it onto the public lowercase Container.
type dockerPSRow struct {
	ID         string `json:"ID"`
	Names      string `json:"Names"`
	Image      string `json:"Image"`
	Status     string `json:"Status"`
	State      string `json:"State"`
	Ports      string `json:"Ports"`
	CreatedAt  string `json:"CreatedAt"`
	Command    string `json:"Command"`
	Labels     string `json:"Labels"`
	Mounts     string `json:"Mounts"`
	Networks   string `json:"Networks"`
	Size       string `json:"Size"`
	RunningFor string `json:"RunningFor"`
}

func (d dockerPSRow) toContainer() Container {
	return Container{
		ID:         d.ID,
		Name:       strings.TrimPrefix(d.Names, "/"),
		Image:      d.Image,
		Status:     d.Status,
		State:      d.State,
		Ports:      parsePortsString(d.Ports),
		CreatedAt:  d.CreatedAt,
		Command:    d.Command,
		Labels:     d.Labels,
		Mounts:     d.Mounts,
		Networks:   d.Networks,
		Size:       d.Size,
		RunningFor: d.RunningFor,
	}
}

// parsePortsString turns the comma-separated `docker ps` Ports string
// (e.g. "0.0.0.0:8080->80/tcp, :::8080->80/tcp") into structured PortMappings.
// Falls back to a host-only entry for tokens that don't match the expected shape.
func parsePortsString(s string) []PortMapping {
	if s = strings.TrimSpace(s); s == "" {
		return []PortMapping{}
	}
	out := make([]PortMapping, 0)
	for _, tok := range strings.Split(s, ",") {
		t := strings.TrimSpace(tok)
		if t == "" {
			continue
		}
		// match optional "ip:" prefix, then "hostPort->containerPort/protocol"
		m := portMappingRE.FindStringSubmatch(t)
		if m != nil {
			ip := ""
			if m[1] != "" {
				ip = m[1] + ":"
			}
			out = append(out, PortMapping{
				HostPort:      ip + m[2],
				ContainerPort: m[3],
				Protocol:      m[4],
			})
			continue
		}
		out = append(out, PortMapping{HostPort: t})
	}
	return out
}

// dockerImageRow mirrors the uppercase-keyed JSON emitted by `docker images --format json`.
type dockerImageRow struct {
	ID         string            `json:"ID"`
	Repository string            `json:"Repository"`
	Tag        string            `json:"Tag"`
	Size       string            `json:"Size"`
	CreatedAt  string            `json:"CreatedAt"`
	Labels     map[string]string `json:"Labels"`
}

func (d dockerImageRow) toImage() Image {
	return Image{
		ID:         d.ID,
		Repository: d.Repository,
		Tag:        d.Tag,
		Size:       d.Size,
		CreatedAt:  d.CreatedAt,
		Labels:     d.Labels,
	}
}

// podmanPSRow mirrors a single element of the JSON array emitted by
// `podman ps --format json`. Fields differ in casing and type from Docker's
// NDJSON rows (arrays where Docker uses strings), so it needs its own shim.
type podmanPSRow struct {
	ID        string            `json:"Id"`
	Names     []string          `json:"Names"`
	Image     string            `json:"Image"`
	State     string            `json:"State"`
	Ports     json.RawMessage   `json:"Ports"`
	CreatedAt string            `json:"CreatedAt"`
	Command   []string          `json:"Command"`
	Labels    map[string]string `json:"Labels"`
	Mounts    []string          `json:"Mounts"`
	Networks  []string          `json:"Networks"`
	Size      int64             `json:"Size"`
}

type podmanPort struct {
	HostIP        string `json:"host_ip"`
	HostPort      string `json:"host_port"`
	ContainerPort string `json:"container_port"`
	Protocol      string `json:"protocol"`
}

func (p podmanPSRow) toContainer() Container {
	ports := make([]PortMapping, 0)
	if len(p.Ports) > 0 && string(p.Ports) != "null" {
		var pp []podmanPort
		if json.Unmarshal(p.Ports, &pp) == nil {
			for _, pr := range pp {
				hostPort := pr.HostPort
				if pr.HostIP != "" {
					hostPort = pr.HostIP + ":" + hostPort
				}
				ports = append(ports, PortMapping{
					HostPort:      hostPort,
					ContainerPort: pr.ContainerPort,
					Protocol:      pr.Protocol,
				})
			}
		}
	}
	return Container{
		ID:    p.ID,
		Name:  strings.Join(p.Names, ","),
		Image: p.Image,
		// podman JSON has no Status field; State is the closest equivalent.
		Status:    p.State,
		State:     p.State,
		Ports:     ports,
		CreatedAt: p.CreatedAt,
		Command:   strings.Join(p.Command, " "),
		Labels:    fmt.Sprint(p.Labels),
		Mounts:    strings.Join(p.Mounts, ","),
		Networks:  strings.Join(p.Networks, ","),
		Size:      fmt.Sprint(p.Size),
	}
}

// podmanImageRow mirrors one element of `podman images --format json`.
type podmanImageRow struct {
	ID         string            `json:"Id"`
	Repository string            `json:"Repository"`
	Tag        string            `json:"Tag"`
	Size       int64             `json:"Size"`
	CreatedAt  string            `json:"CreatedAt"` // RFC3339
	Labels     map[string]string `json:"Labels"`
}

func (p podmanImageRow) toImage() Image {
	return Image{
		ID:         p.ID,
		Repository: p.Repository,
		Tag:        p.Tag,
		Size:       fmt.Sprint(p.Size),
		CreatedAt:  p.CreatedAt,
		Labels:     p.Labels,
	}
}

// parseJSONRows splits engine CLI output into rows. Docker emits one JSON
// object per line (NDJSON); Podman emits a single JSON array. Both are
// accepted, and each element is passed to `mapRow`.
func parseJSONRows(output string, mapRow func([]byte) (any, bool)) ([]any, error) {
	trimmed := strings.TrimSpace(output)
	if trimmed == "" {
		return []any{}, nil
	}
	if strings.HasPrefix(trimmed, "[") {
		var raw []json.RawMessage
		if err := json.Unmarshal([]byte(trimmed), &raw); err != nil {
			return nil, err
		}
		out := make([]any, 0, len(raw))
		for _, r := range raw {
			if v, ok := mapRow(r); ok {
				out = append(out, v)
			}
		}
		return out, nil
	}
	out := make([]any, 0)
	for _, line := range strings.Split(trimmed, "\n") {
		if line = strings.TrimSpace(line); line == "" {
			continue
		}
		if v, ok := mapRow([]byte(line)); ok {
			out = append(out, v)
		}
	}
	return out, nil
}

func isPodmanEngine(engine Engine) bool { return engineBinary(engine) == "podman" }

// --- Container operations ---

// checkEngine checks if the given engine binary is installed and accessible.
func (s *Service) checkEngine(ctx context.Context, engine Engine) error {
	_, exitCode, err := s.executor.RunCombined(ctx, engineBinary(engine), "version", "--format", "{{.Server.Version}}")
	if err != nil || exitCode != 0 {
		return fmt.Errorf("%s is not installed or not accessible", engine)
	}
	return nil
}

// ListContainers returns all containers for the given engine.
func (s *Service) ListContainers(ctx context.Context, engine Engine, all bool) ([]Container, error) {
	if err := s.checkEngine(ctx, engine); err != nil {
		return nil, err
	}

	args := []string{"ps", "--format", "json"}
	if all {
		args = append(args, "-a")
	}

	output, exitCode, err := s.executor.RunCombined(ctx, engineBinary(engine), args...)
	if err != nil || exitCode != 0 {
		return nil, fmt.Errorf("%s ps failed: %s", engine, output)
	}

	rows, err := parseJSONRows(output, func(line []byte) (any, bool) {
		if isPodmanEngine(engine) {
			var d podmanPSRow
			if err := json.Unmarshal(line, &d); err != nil {
				log.Printf("container: parse podman container json error: %v, line: %s", err, line[:min(100, len(line))])
				return nil, false
			}
			return d.toContainer(), true
		}
		var d dockerPSRow
		if err := json.Unmarshal(line, &d); err != nil {
			log.Printf("container: parse docker container json error: %v, line: %s", err, line[:min(100, len(line))])
			return nil, false
		}
		return d.toContainer(), true
	})
	if err != nil {
		return nil, err
	}

	containers := make([]Container, 0, len(rows))
	for _, r := range rows {
		containers = append(containers, r.(Container))
	}
	return containers, nil
}

// GetContainer returns details of a specific container.
func (s *Service) GetContainer(ctx context.Context, engine Engine, id string) (*Container, error) {
	output, exitCode, err := s.executor.RunCombined(ctx, engineBinary(engine), "inspect", "--format", "{{json .}}", id)
	if err != nil || exitCode != 0 {
		return nil, fmt.Errorf("%s inspect failed: %s", engine, output)
	}

	trimmed := strings.TrimSpace(output)
	var rows []dockerPSRow
	if err := json.Unmarshal([]byte(trimmed), &rows); err != nil {
		var d dockerPSRow
		if err2 := json.Unmarshal([]byte(trimmed), &d); err2 != nil {
			return nil, fmt.Errorf("parse container: %w", err2)
		}
		c := d.toContainer()
		return &c, nil
	}

	if len(rows) == 0 {
		return nil, fmt.Errorf("container not found: %s", id)
	}

	c := rows[0].toContainer()
	return &c, nil
}

func (s *Service) containerAction(ctx context.Context, engine Engine, action, id string) error {
	output, exitCode, err := s.executor.RunCombined(ctx, engineBinary(engine), action, id)
	if err != nil || exitCode != 0 {
		if output != "" {
			return fmt.Errorf("%s %s failed: %s", engine, action, output)
		}
		return fmt.Errorf("%s %s failed: %v", engine, action, err)
	}
	return nil
}

// StartContainer starts a container.
func (s *Service) StartContainer(ctx context.Context, engine Engine, id string) error {
	return s.containerAction(ctx, engine, "start", id)
}

// StopContainer stops a container.
func (s *Service) StopContainer(ctx context.Context, engine Engine, id string) error {
	return s.containerAction(ctx, engine, "stop", id)
}

// RestartContainer restarts a container.
func (s *Service) RestartContainer(ctx context.Context, engine Engine, id string) error {
	return s.containerAction(ctx, engine, "restart", id)
}

// PauseContainer pauses a container.
func (s *Service) PauseContainer(ctx context.Context, engine Engine, id string) error {
	return s.containerAction(ctx, engine, "pause", id)
}

// UnpauseContainer unpauses a container.
func (s *Service) UnpauseContainer(ctx context.Context, engine Engine, id string) error {
	return s.containerAction(ctx, engine, "unpause", id)
}

// RemoveContainer removes a container.
func (s *Service) RemoveContainer(ctx context.Context, engine Engine, id string, force bool) error {
	args := []string{"rm"}
	if force {
		args = append(args, "-f")
	}
	args = append(args, id)

	_, exitCode, err := s.executor.RunCombined(ctx, engineBinary(engine), args...)
	if err != nil || exitCode != 0 {
		return fmt.Errorf("%s rm failed: %v", engine, err)
	}
	return nil
}

// GetContainerLogs returns container logs.
func (s *Service) GetContainerLogs(ctx context.Context, engine Engine, id string, tail int) (string, error) {
	args := []string{"logs", "--tail", fmt.Sprintf("%d", tail), id}
	output, exitCode, err := s.executor.RunCombined(ctx, engineBinary(engine), args...)
	if err != nil || exitCode != 0 {
		return "", fmt.Errorf("%s logs failed: %s", engine, output)
	}
	return output, nil
}

// ExecInContainer executes a command in a running container.
func (s *Service) ExecInContainer(ctx context.Context, engine Engine, id, cmd string) (string, error) {
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

	output, exitCode, err := s.executor.RunCombined(ctx, engineBinary(engine), "exec", id, "sh", "-c", cmd)
	if err != nil || exitCode != 0 {
		return output, fmt.Errorf("%s exec failed: %s", engine, output)
	}
	return output, nil
}

// CreateContainer creates a new container.
func (s *Service) CreateContainer(ctx context.Context, engine Engine, req CreateRequest) (string, error) {
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

	output, exitCode, err := s.executor.RunCombined(ctx, engineBinary(engine), args...)
	if err != nil || exitCode != 0 {
		return "", fmt.Errorf("%s create failed: %s", engine, output)
	}

	return strings.TrimSpace(output), nil
}

// ListImages returns all images for the given engine.
func (s *Service) ListImages(ctx context.Context, engine Engine) ([]Image, error) {
	if err := s.checkEngine(ctx, engine); err != nil {
		return nil, err
	}

	output, exitCode, err := s.executor.RunCombined(ctx, engineBinary(engine), "images", "--format", "json")
	if err != nil || exitCode != 0 {
		return nil, fmt.Errorf("%s images failed: %s", engine, output)
	}

	rows, err := parseJSONRows(output, func(line []byte) (any, bool) {
		if isPodmanEngine(engine) {
			var d podmanImageRow
			if err := json.Unmarshal(line, &d); err != nil {
				log.Printf("container: parse podman image json error: %v, line: %s", err, line[:min(100, len(line))])
				return nil, false
			}
			return d.toImage(), true
		}
		var d dockerImageRow
		if err := json.Unmarshal(line, &d); err != nil {
			log.Printf("container: parse docker image json error: %v, line: %s", err, line[:min(100, len(line))])
			return nil, false
		}
		return d.toImage(), true
	})
	if err != nil {
		return nil, err
	}

	images := make([]Image, 0, len(rows))
	for _, r := range rows {
		images = append(images, r.(Image))
	}
	return images, nil
}

// PullImage pulls an image.
func (s *Service) PullImage(ctx context.Context, engine Engine, image string) error {
	_, _, exitCode, err := s.executor.RunWithTimeout(ctx, ImagePullTimeout, engineBinary(engine), "pull", image)
	if err != nil || exitCode != 0 {
		return fmt.Errorf("%s pull failed: %v", engine, err)
	}
	return nil
}

// RemoveImage removes an image.
func (s *Service) RemoveImage(ctx context.Context, engine Engine, id string, force bool) error {
	args := []string{"rmi"}
	if force {
		args = append(args, "-f")
	}
	args = append(args, id)

	_, exitCode, err := s.executor.RunCombined(ctx, engineBinary(engine), args...)
	if err != nil || exitCode != 0 {
		return fmt.Errorf("%s rmi failed: %v", engine, err)
	}
	return nil
}

// GetContainerStats returns real-time resource usage stats for a container.
func (s *Service) GetContainerStats(ctx context.Context, engine Engine, id string) (*Stats, error) {
	output, exitCode, err := s.executor.RunCombined(ctx, engineBinary(engine), "stats", id, "--no-stream", "--format",
		`{"cpu_percent":"{{.CPUPerc}}","mem_usage":"{{.MemUsage}}","mem_percent":"{{.MemPerc}}","net_rx":"{{.NetIO}}","block_read":"{{.BlockIO}}","pids":"{{.PIDs}}"}`)
	if err != nil || exitCode != 0 {
		return nil, fmt.Errorf("%s stats failed: %s", engine, output)
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
func (s *Service) GetContainerTop(ctx context.Context, engine Engine, id string) ([]ProcessInfo, error) {
	output, exitCode, err := s.executor.RunCombined(ctx, engineBinary(engine), "top", id, "-eo", "user,pid,ppid,%cpu,%mem,vsz,rss,tty,stat,start,time,comm")
	if err != nil || exitCode != 0 {
		return nil, fmt.Errorf("%s top failed: %s", engine, output)
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
func (s *Service) CopyToContainer(ctx context.Context, engine Engine, id, srcPath, destPath string) error {
	_, exitCode, err := s.executor.RunCombined(ctx, engineBinary(engine), "cp", srcPath, id+":"+destPath)
	if err != nil || exitCode != 0 {
		return fmt.Errorf("%s cp to container failed: %v", engine, err)
	}
	return nil
}

// CopyFromContainer copies a file from container to host.
func (s *Service) CopyFromContainer(ctx context.Context, engine Engine, id, srcPath, destPath string) error {
	_, exitCode, err := s.executor.RunCombined(ctx, engineBinary(engine), "cp", id+":"+srcPath, destPath)
	if err != nil || exitCode != 0 {
		return fmt.Errorf("%s cp from container failed: %v", engine, err)
	}
	return nil
}

// RenameContainer renames a container.
func (s *Service) RenameContainer(ctx context.Context, engine Engine, id, newName string) error {
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

	_, exitCode, err := s.executor.RunCombined(ctx, engineBinary(engine), "rename", id, newName)
	if err != nil || exitCode != 0 {
		return fmt.Errorf("%s rename failed: %v", engine, err)
	}
	return nil
}

// UpdateContainer updates container resource limits.
func (s *Service) UpdateContainer(ctx context.Context, engine Engine, id string, req UpdateRequest) error {
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

	output, exitCode, err := s.executor.RunCombined(ctx, engineBinary(engine), args...)
	if err != nil || exitCode != 0 {
		return fmt.Errorf("%s update failed: %s", engine, output)
	}
	return nil
}

// --- System operations ---

// Detect checks if the given engine is installed, its version, compose
// version, running status, and OS.
func (s *Service) Detect(ctx context.Context, engine Engine) (*DockerStatus, error) {
	status := &DockerStatus{}
	bin := engineBinary(engine)

	status.OS = s.detectOS(ctx)

	stdout, exitCode, err := s.executor.RunCombined(ctx, bin, "version", "--format", "{{.Server.Version}}")
	if err != nil || exitCode != 0 {
		status.Installed = false
		return status, nil
	}
	status.Installed = true
	status.Version = strings.TrimSpace(stdout)

	// Podman has no daemon; if the binary works it's usable. Docker's running
	// state is probed via `info` (differently-shaped template fields).
	if isPodmanEngine(engine) {
		status.Running = true
	} else {
		_, exitCode, err = s.executor.RunCombined(ctx, bin, "info", "--format", "{{.ServerVersion}}")
		status.Running = err == nil && exitCode == 0
	}

	status.ComposeVersion = s.detectComposeVersion(ctx, engine)

	return status, nil
}

func (s *Service) detectComposeVersion(ctx context.Context, engine Engine) string {
	if isPodmanEngine(engine) {
		composeOut, exitCode, err := s.executor.RunCombined(ctx, "podman-compose", "version")
		if err == nil && exitCode == 0 {
			return strings.TrimSpace(composeOut)
		}
		return ""
	}
	composeOut, exitCode, err := s.executor.RunCombined(ctx, "docker", "compose", "version", "--short")
	if err == nil && exitCode == 0 {
		return strings.TrimSpace(composeOut)
	}
	composeOut, exitCode, err = s.executor.RunCombined(ctx, "docker-compose", "version", "--short")
	if err == nil && exitCode == 0 {
		return strings.TrimSpace(composeOut)
	}
	return ""
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

// Install installs the given engine. Docker uses the official convenience
// script; Podman installs via the distro package manager (no official script).
func (s *Service) Install(ctx context.Context, engine Engine) error {
	if isPodmanEngine(engine) {
		return s.installPodman(ctx)
	}
	return s.installDocker(ctx)
}

func (s *Service) installDocker(ctx context.Context) error {
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

// installPodman installs Podman via the distro package manager.
func (s *Service) installPodman(ctx context.Context) error {
	os := s.detectOS(ctx)
	var pkgCmd string
	switch os {
	case "debian", "ubuntu":
		pkgCmd = "apt-get update && apt-get install -y podman podman-compose"
	case "centos", "rhel", "fedora":
		pkgCmd = "dnf install -y podman podman-compose"
	case "alpine":
		pkgCmd = "apk add podman podman-compose"
	case "arch":
		pkgCmd = "pacman -S --noconfirm podman podman-compose"
	default:
		return fmt.Errorf("不支持的发行版：%s，请手动安装 Podman", os)
	}

	log.Println("podman: starting installation...")
	output, _, exitCode, err := s.executor.RunWithTimeout(ctx, 10*time.Minute, "bash", "-c", pkgCmd)
	if err != nil || exitCode != 0 {
		return fmt.Errorf("Podman 安装失败 (exit=%d): %s", exitCode, truncateOutput(output, 500))
	}
	log.Printf("podman: installation completed")
	return nil
}

// serviceUnit returns the systemd unit backing the timeout engine's service.
func serviceUnit(engine Engine) string {
	if isPodmanEngine(engine) {
		return "podman.socket"
	}
	return "docker"
}

// StartEngine starts the engine's systemd service.
func (s *Service) StartEngine(ctx context.Context, engine Engine) error {
	output, exitCode, err := s.executor.RunCombined(ctx, "systemctl", "start", serviceUnit(engine))
	if err != nil || exitCode != 0 {
		return fmt.Errorf("failed to start %s: %s", engine, output)
	}
	return nil
}

// StopEngine stops the engine's systemd service.
func (s *Service) StopEngine(ctx context.Context, engine Engine) error {
	output, exitCode, err := s.executor.RunCombined(ctx, "systemctl", "stop", serviceUnit(engine))
	if err != nil || exitCode != 0 {
		return fmt.Errorf("failed to stop %s: %s", engine, output)
	}
	return nil
}

// RestartEngine restarts the engine's systemd service.
func (s *Service) RestartEngine(ctx context.Context, engine Engine) error {
	output, exitCode, err := s.executor.RunCombined(ctx, "systemctl", "restart", serviceUnit(engine))
	if err != nil || exitCode != 0 {
		return fmt.Errorf("failed to restart %s: %s", engine, output)
	}
	return nil
}

// GetInfo returns the engine's system info as a map.
func (s *Service) GetInfo(ctx context.Context, engine Engine) (map[string]interface{}, error) {
	output, exitCode, err := s.executor.RunCombined(ctx, engineBinary(engine), "info", "--format", "{{json .}}")
	if err != nil || exitCode != 0 {
		return nil, fmt.Errorf("%s info failed: %s", engine, output)
	}

	var info map[string]interface{}
	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &info); err != nil {
		return nil, fmt.Errorf("parse %s info: %w", engine, err)
	}

	return info, nil
}

// ConfigureMirror configures the engine's registry mirror. Docker writes
// /etc/docker/daemon.json; Podman writes unqualified-search-registries to
// /etc/containers/registries.conf.
func (s *Service) ConfigureMirror(ctx context.Context, engine Engine, mirrorURL string) error {
	if isPodmanEngine(engine) {
		return s.configurePodmanMirror(ctx, mirrorURL)
	}
	return s.configureDockerMirror(ctx, mirrorURL)
}

func (s *Service) configureDockerMirror(ctx context.Context, mirrorURL string) error {
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

	return s.RestartEngine(ctx, "docker")
}

// configurePodmanMirror writes an unqualified-search-registries entry to
// /etc/containers/registries.conf. Empty URL leaves the file untouched.
func (s *Service) configurePodmanMirror(ctx context.Context, mirrorURL string) error {
	if mirrorURL == "" {
		// reset to distro default: leave the file untouched
		return nil
	}
	// ponytail: single-mirror best-effort; multiple mirrors/regex blocks later if needed.
	config := fmt.Sprintf("unqualified-search-registries = [\"%s\"]\n", mirrorURL)
	encoded := base64.StdEncoding.EncodeToString([]byte(config))
	writeCmd := fmt.Sprintf("mkdir -p /etc/containers && echo '%s' | base64 -d > /etc/containers/registries.conf", encoded)
	_, exitCode, err := s.executor.RunCombined(ctx, "bash", "-c", writeCmd)
	if err != nil || exitCode != 0 {
		return fmt.Errorf("failed to write registries.conf: %v", err)
	}
	return nil
}

func truncateOutput(output string, maxLen int) string {
	if len(output) <= maxLen {
		return output
	}
	return output[:maxLen] + "..."
}

// --- Compose operations ---

// composeCommand returns the executable and base args for a compose invocation.
func (s *Service) composeCommand(engine Engine, composeFile string) (string, []string) {
	if isPodmanEngine(engine) {
		args := []string{}
		if composeFile != "" {
			args = append(args, "-f", composeFile)
		}
		return "podman-compose", args
	}
	args := []string{"compose"}
	if composeFile != "" {
		args = append(args, "-f", composeFile)
	}
	return "docker", args
}

// ListProjects lists all Compose projects for the given engine.
// ponytail: podman-compose has no `ls --format json`; unsupported there.
func (s *Service) ListProjects(ctx context.Context, engine Engine) ([]ComposeProject, error) {
	if isPodmanEngine(engine) {
		return []ComposeProject{}, nil
	}

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

// ComposeUp runs compose up -d for a project.
func (s *Service) ComposeUp(ctx context.Context, engine Engine, projectDir string) error {
	composeFile := s.findComposeFile(projectDir)
	bin, args := s.composeCommand(engine, composeFile)
	args = append(args, "up", "-d")

	output, exitCode, err := s.executor.RunCombined(ctx, bin, args...)
	if err != nil || exitCode != 0 {
		return fmt.Errorf("compose up failed: %s", output)
	}
	return nil
}

// ComposeDown runs compose down for a project.
func (s *Service) ComposeDown(ctx context.Context, engine Engine, projectDir string) error {
	composeFile := s.findComposeFile(projectDir)
	bin, args := s.composeCommand(engine, composeFile)
	args = append(args, "down")

	output, exitCode, err := s.executor.RunCombined(ctx, bin, args...)
	if err != nil || exitCode != 0 {
		return fmt.Errorf("compose down failed: %s", output)
	}
	return nil
}

// ComposeRestart runs compose restart for a project.
func (s *Service) ComposeRestart(ctx context.Context, engine Engine, projectDir string) error {
	composeFile := s.findComposeFile(projectDir)
	bin, args := s.composeCommand(engine, composeFile)
	args = append(args, "restart")

	output, exitCode, err := s.executor.RunCombined(ctx, bin, args...)
	if err != nil || exitCode != 0 {
		return fmt.Errorf("compose restart failed: %s", output)
	}
	return nil
}

// ComposeGetLogs returns logs for a compose project.
func (s *Service) ComposeGetLogs(ctx context.Context, engine Engine, projectDir string, tail int) (string, error) {
	composeFile := s.findComposeFile(projectDir)
	bin, args := s.composeCommand(engine, composeFile)
	args = append(args, "logs", "--tail", fmt.Sprintf("%d", tail))

	output, exitCode, err := s.executor.RunCombined(ctx, bin, args...)
	if err != nil || exitCode != 0 {
		return "", fmt.Errorf("compose logs failed: %s", output)
	}
	return output, nil
}

// ComposeGetConfig reads the compose file content.
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

// ComposeSaveConfig writes content to the compose file.
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

// --- Volume operations ---

// ListVolumes returns all volumes for the given engine.
func (s *Service) ListVolumes(ctx context.Context, engine Engine) ([]Volume, error) {
	output, exitCode, err := s.executor.RunCombined(ctx, engineBinary(engine), "volume", "ls", "--format", "json")
	if err != nil || exitCode != 0 {
		return nil, fmt.Errorf("%s volume ls failed: %s", engine, output)
	}

	rows, err := parseJSONRows(output, func(line []byte) (any, bool) {
		// Podman uses lowercase "name"/"driver"/"mountpoint"; Go's decoder is
		// case-insensitive, so the lowercase tags match both.
		var raw struct {
			Name       string `json:"name"`
			Driver     string `json:"driver"`
			Mountpoint string `json:"mountpoint"`
			CreatedAt  string `json:"createdat"`
		}
		if err := json.Unmarshal(line, &raw); err != nil {
			return nil, false
		}
		return Volume{
			Name:       raw.Name,
			Driver:     raw.Driver,
			Mountpoint: raw.Mountpoint,
			CreatedAt:  raw.CreatedAt,
		}, true
	})
	if err != nil {
		return nil, err
	}

	volumes := make([]Volume, 0, len(rows))
	for _, r := range rows {
		volumes = append(volumes, r.(Volume))
	}
	return volumes, nil
}

// CreateVolume creates a new volume.
func (s *Service) CreateVolume(ctx context.Context, engine Engine, name, driver string) error {
	args := []string{"volume", "create"}
	if driver != "" {
		args = append(args, "--driver", driver)
	}
	args = append(args, name)

	_, exitCode, err := s.executor.RunCombined(ctx, engineBinary(engine), args...)
	if err != nil || exitCode != 0 {
		return fmt.Errorf("%s volume create failed: %v", engine, err)
	}
	return nil
}

// RemoveVolume removes a volume.
func (s *Service) RemoveVolume(ctx context.Context, engine Engine, name string, force bool) error {
	args := []string{"volume", "rm"}
	if force {
		args = append(args, "-f")
	}
	args = append(args, name)

	_, exitCode, err := s.executor.RunCombined(ctx, engineBinary(engine), args...)
	if err != nil || exitCode != 0 {
		return fmt.Errorf("%s volume rm failed: %v", engine, err)
	}
	return nil
}

// --- Network operations ---

type networkDetails struct {
	Subnet  string
	Gateway string
}

// ListNetworks returns all networks for the given engine.
func (s *Service) ListNetworks(ctx context.Context, engine Engine) ([]Network, error) {
	output, exitCode, err := s.executor.RunCombined(ctx, engineBinary(engine), "network", "ls", "--format", "json")
	if err != nil || exitCode != 0 {
		return nil, fmt.Errorf("%s network ls failed: %s", engine, output)
	}

	rows, err := parseJSONRows(output, func(line []byte) (any, bool) {
		// Podman uses lowercase "id"/"name"/"driver"/"scope"; case-insensitive
		// unmarshal lets lowercase tags match both.
		var raw struct {
			ID     string `json:"id"`
			Name   string `json:"name"`
			Driver string `json:"driver"`
			Scope  string `json:"scope"`
		}
		if err := json.Unmarshal(line, &raw); err != nil {
			return nil, false
		}
		return Network{
			ID:     raw.ID,
			Name:   raw.Name,
			Driver: raw.Driver,
			Scope:  raw.Scope,
		}, true
	})
	if err != nil {
		return nil, err
	}

	networks := make([]Network, 0, len(rows))
	for _, r := range rows {
		net := r.(Network)
		details := s.inspectNetwork(ctx, engine, net.ID)
		if details != nil {
			net.Subnet = details.Subnet
			net.Gateway = details.Gateway
		}
		networks = append(networks, net)
	}

	return networks, nil
}

func (s *Service) inspectNetwork(ctx context.Context, engine Engine, id string) *networkDetails {
	output, exitCode, err := s.executor.RunCombined(ctx, engineBinary(engine), "network", "inspect", "--format", "{{json .IPAM}}", id)
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

// CreateNetwork creates a new network.
func (s *Service) CreateNetwork(ctx context.Context, engine Engine, name, driver string) error {
	args := []string{"network", "create"}
	if driver != "" {
		args = append(args, "--driver", driver)
	}
	args = append(args, name)

	_, exitCode, err := s.executor.RunCombined(ctx, engineBinary(engine), args...)
	if err != nil || exitCode != 0 {
		return fmt.Errorf("%s network create failed: %v", engine, err)
	}
	return nil
}

// RemoveNetwork removes a network.
func (s *Service) RemoveNetwork(ctx context.Context, engine Engine, id string) error {
	_, exitCode, err := s.executor.RunCombined(ctx, engineBinary(engine), "network", "rm", id)
	if err != nil || exitCode != 0 {
		return fmt.Errorf("%s network rm failed: %v", engine, err)
	}
	return nil
}

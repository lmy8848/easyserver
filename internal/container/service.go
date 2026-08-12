package container

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sort"
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
	for tok := range strings.SplitSeq(s, ",") {
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
	// 字段名与类型与 Image 一致，直接类型转换即可。
	return Image(d)
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
		Size:      humanSize(p.Size),
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
		Size:       humanSize(p.Size),
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
	for line := range strings.SplitSeq(trimmed, "\n") {
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

// rejectManaged refuses mutating operations on EasyServer-managed database
// containers. The generic Container resource may view but never take over,
// edit or delete a managed database container; its lifecycle belongs to the
// database module (PRD: generic Container cannot bypass database rules).
func (s *Service) rejectManaged(ctx context.Context, engine Engine, id string) error {
	out, exitCode, err := s.executor.RunCombined(ctx, engineBinary(engine), "inspect", "--format", "{{index .Config.Labels \"com.easyserver.managed\"}}", id)
	if err != nil || exitCode != 0 {
		return nil //nolint:nilerr // 非受管容器时返回 nil，让操作自然失败
	}
	if strings.TrimSpace(out) == "true" {
		return errors.New("受管数据库容器，请通过数据库模块操作")
	}
	return nil
}

// --- Container operations ---

// checkEngine checks if the given engine CLI is installed and accessible.
// Uses `--version` (client-only, no daemon) so a stopped Docker daemon is not
// misreported as "not installed".
func (s *Service) checkEngine(ctx context.Context, engine Engine) error {
	_, exitCode, err := s.executor.RunCombined(ctx, engineBinary(engine), "--version")
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
		return fmt.Errorf("%s %s failed: %w", engine, action, err)
	}
	return nil
}

// StartContainer starts a container.
func (s *Service) StartContainer(ctx context.Context, engine Engine, id string) error {
	if err := s.rejectManaged(ctx, engine, id); err != nil {
		return err
	}
	return s.containerAction(ctx, engine, "start", id)
}

// StopContainer stops a container.
func (s *Service) StopContainer(ctx context.Context, engine Engine, id string) error {
	if err := s.rejectManaged(ctx, engine, id); err != nil {
		return err
	}
	return s.containerAction(ctx, engine, "stop", id)
}

// RestartContainer restarts a container.
func (s *Service) RestartContainer(ctx context.Context, engine Engine, id string) error {
	if err := s.rejectManaged(ctx, engine, id); err != nil {
		return err
	}
	return s.containerAction(ctx, engine, "restart", id)
}

// PauseContainer pauses a container.
func (s *Service) PauseContainer(ctx context.Context, engine Engine, id string) error {
	if err := s.rejectManaged(ctx, engine, id); err != nil {
		return err
	}
	return s.containerAction(ctx, engine, "pause", id)
}

// UnpauseContainer unpauses a container.
func (s *Service) UnpauseContainer(ctx context.Context, engine Engine, id string) error {
	if err := s.rejectManaged(ctx, engine, id); err != nil {
		return err
	}
	return s.containerAction(ctx, engine, "unpause", id)
}

// RemoveContainer removes a container.
func (s *Service) RemoveContainer(ctx context.Context, engine Engine, id string, force bool) error {
	if err := s.rejectManaged(ctx, engine, id); err != nil {
		return err
	}
	args := []string{"rm"}
	if force {
		args = append(args, "-f")
	}
	args = append(args, id)

	_, exitCode, err := s.executor.RunCombined(ctx, engineBinary(engine), args...)
	if err != nil || exitCode != 0 {
		return fmt.Errorf("%s rm failed: %w", engine, err)
	}
	return nil
}

// GetContainerLogs returns container logs.
func (s *Service) GetContainerLogs(ctx context.Context, engine Engine, id string, tail int) (string, error) {
	args := []string{"logs", "--tail", strconv.Itoa(tail), id}
	output, exitCode, err := s.executor.RunCombined(ctx, engineBinary(engine), args...)
	if err != nil || exitCode != 0 {
		return "", fmt.Errorf("%s logs failed: %s", engine, output)
	}
	return output, nil
}

// ExecInContainer executes a command in a running container.
func (s *Service) ExecInContainer(ctx context.Context, engine Engine, id, cmd string) (string, error) {
	if err := s.rejectManaged(ctx, engine, id); err != nil {
		return "", err
	}
	if strings.ContainsRune(cmd, '\x00') {
		return "", errors.New("command contains null byte")
	}
	const maxCmdLen = 4096
	if len(cmd) > maxCmdLen {
		return "", fmt.Errorf("command exceeds maximum length (%d bytes)", maxCmdLen)
	}
	if strings.TrimSpace(cmd) == "" {
		return "", errors.New("command cannot be empty")
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
		args = append(args, "--memory", strconv.FormatInt(req.Memory, 10))
	}
	if req.CPUs > 0 {
		args = append(args, "--cpus", fmt.Sprintf("%.2f", req.CPUs))
	}

	// Podman can't resolve short image names without a registries.conf; expand
	// them to the docker.io namespace so creation works out of the box.
	image := req.Image
	if isPodmanEngine(engine) {
		image = expandImageRef(image)
	}
	args = append(args, image)

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
		return fmt.Errorf("%s pull failed: %w", engine, err)
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
		return fmt.Errorf("%s rmi failed: %w", engine, err)
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
	var numStrSb642 strings.Builder
	for i, c := range s {
		if (c >= '0' && c <= '9') || c == '.' {
			numStrSb642.WriteString(string(c))
		} else {
			unit = s[i:]
			break
		}
	}
	numStr += numStrSb642.String()

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
	if err := s.rejectManaged(ctx, engine, id); err != nil {
		return err
	}
	_, exitCode, err := s.executor.RunCombined(ctx, engineBinary(engine), "cp", srcPath, id+":"+destPath)
	if err != nil || exitCode != 0 {
		return fmt.Errorf("%s cp to container failed: %w", engine, err)
	}
	return nil
}

// CopyFromContainer copies a file from container to host.
func (s *Service) CopyFromContainer(ctx context.Context, engine Engine, id, srcPath, destPath string) error {
	if err := s.rejectManaged(ctx, engine, id); err != nil {
		return err
	}
	_, exitCode, err := s.executor.RunCombined(ctx, engineBinary(engine), "cp", id+":"+srcPath, destPath)
	if err != nil || exitCode != 0 {
		return fmt.Errorf("%s cp from container failed: %w", engine, err)
	}
	return nil
}

// RenameContainer renames a container.
func (s *Service) RenameContainer(ctx context.Context, engine Engine, id, newName string) error {
	if err := s.rejectManaged(ctx, engine, id); err != nil {
		return err
	}
	if strings.TrimSpace(id) == "" {
		return errors.New("container ID cannot be empty")
	}
	if strings.TrimSpace(newName) == "" {
		return errors.New("new container name cannot be empty")
	}
	if len(newName) > 128 {
		return errors.New("container name too long (max 128 characters)")
	}
	for i, ch := range newName {
		if (ch < 'a' || ch > 'z') && (ch < 'A' || ch > 'Z') && (ch < '0' || ch > '9') && ch != '_' && ch != '.' && ch != '-' {
			return fmt.Errorf("invalid character '%c' in container name at position %d", ch, i)
		}
	}
	if newName[0] == '.' || newName[0] == '-' {
		return fmt.Errorf("container name cannot start with '%c'", newName[0])
	}

	_, exitCode, err := s.executor.RunCombined(ctx, engineBinary(engine), "rename", id, newName)
	if err != nil || exitCode != 0 {
		return fmt.Errorf("%s rename failed: %w", engine, err)
	}
	return nil
}

// UpdateContainer updates container resource limits.
func (s *Service) UpdateContainer(ctx context.Context, engine Engine, id string, req UpdateRequest) error {
	if err := s.rejectManaged(ctx, engine, id); err != nil {
		return err
	}
	args := []string{"update"}

	if req.Memory > 0 {
		args = append(args, "--memory", strconv.FormatInt(req.Memory, 10))
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

	if isPodmanEngine(engine) {
		// Podman is self-contained (no daemon): CLI present = usable.
		stdout, exitCode, err := s.executor.RunCombined(ctx, bin, "--version")
		if err != nil || exitCode != 0 {
			status.Installed = false
			return status, nil //nolint:nilerr // 引擎未安装时返回未安装状态
		}
		status.Installed = true
		status.Version = extractVersion(stdout)
		status.Running = true
		status.SocketEnabled = s.socketEnabled(ctx)
	} else {
		// Docker: CLI and engine (docker.service) are separate packages.
		// "Installed" means the engine unit exists; "Running" means the
		// daemon is active. Version is the daemon (server) version, only
		// available while running.
		status.Installed = s.unitExists(ctx, "docker.service")
		if status.Installed {
			status.Running = s.unitActive(ctx, "docker.service")
			if status.Running {
				status.Version = s.dockerServerVersion(ctx)
			}
		}
	}

	return status, nil
}

// dockerServerVersion returns the Docker daemon (server) version. Only
// meaningful while the daemon is running.
func (s *Service) dockerServerVersion(ctx context.Context) string {
	stdout, exitCode, err := s.executor.RunCombined(ctx, engineBinary(EngineDocker), "version", "--format", "{{.Server.Version}}")
	if err != nil || exitCode != 0 {
		return ""
	}
	return strings.TrimSpace(stdout)
}

// unitExists reports whether a systemd unit file is present.
func (s *Service) unitExists(ctx context.Context, unit string) bool {
	_, exitCode, err := s.executor.RunCombined(ctx, "systemctl", "cat", unit)
	return err == nil && exitCode == 0
}

// unitActive reports whether a systemd unit is currently active.
func (s *Service) unitActive(ctx context.Context, unit string) bool {
	_, exitCode, err := s.executor.RunCombined(ctx, "systemctl", "is-active", "--quiet", unit)
	return err == nil && exitCode == 0
}

// versionRE extracts a semver from `docker --version` / `podman --version`
// banner output (e.g. "Docker version 24.0.7, build xxx" → "24.0.7").
var versionRE = regexp.MustCompile(`([0-9]+\.[0-9]+(?:\.[0-9]+)?)`)

func extractVersion(output string) string {
	if m := versionRE.FindStringSubmatch(output); m != nil {
		return m[1]
	}
	return strings.TrimSpace(output)
}

// expandImageRef qualifies a short image name (e.g. "nginx:latest") with the
// docker.io namespace so Podman can resolve it without a registries.conf.
// A reference whose first component looks like a registry host (contains '.'
// or ':', or is "localhost") is left untouched. Single-component names get the
// docker.io/library/ prefix (matching Podman's default short-name expansion);
// multi-component short names (e.g. "foo/bar") get docker.io/.
func expandImageRef(image string) string {
	first := image
	if before, _, ok := strings.Cut(image, "/"); ok {
		first = before
	} else if before, _, ok := strings.Cut(image, ":"); ok {
		first = before
	}
	if strings.ContainsAny(first, ".:") || first == "localhost" {
		return image
	}
	if strings.ContainsRune(image, '/') {
		return "docker.io/" + image
	}
	return "docker.io/library/" + image
}

// humanSize renders a raw byte count as a readable string (e.g. 164982104 →
// "165MB"). Podman's image/container JSON reports Size as int64 bytes; Docker
// already emits a readable string, so only podman paths go through here.
func humanSize(b int64) string {
	if b < 0 {
		return "0B"
	}
	const unit = 1000
	if b < unit {
		return fmt.Sprintf("%dB", b)
	}
	div, exp := int64(unit), uint(0)
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	s := fmt.Sprintf("%.1f", float64(b)/float64(div))
	return strings.TrimSuffix(s, ".0") + string("KMGTPE"[exp]) + "B"
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
		return fmt.Errorf("curl 未安装，请先安装 curl: %w", err)
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
		return fmt.Errorf("docker 安装脚本执行失败 (exit=%d): %s", exitCode, truncateOutput(output, 500))
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
		return fmt.Errorf("podman 安装失败 (exit=%d): %s", exitCode, truncateOutput(output, 500))
	}
	log.Printf("podman: installation completed")
	return nil
}

// serviceUnit returns the systemd unit backing the engine's service.
func serviceUnit(engine Engine) string {
	if isPodmanEngine(engine) {
		return "podman.socket"
	}
	return "docker"
}

// engineControlSupported reports whether the engine has a daemon/service that
// can be started/stopped. Podman has no daemon (CLI talks directly), so its
// engine-level start/stop is unsupported.
func engineControlSupported(engine Engine) bool { return !isPodmanEngine(engine) }

// StartEngine starts the engine's systemd service.
func (s *Service) StartEngine(ctx context.Context, engine Engine) error {
	if !engineControlSupported(engine) {
		return fmt.Errorf("%s 无守护进程，不支持启停", engine)
	}
	output, exitCode, err := s.executor.RunCombined(ctx, "systemctl", "start", serviceUnit(engine))
	if err != nil || exitCode != 0 {
		return fmt.Errorf("failed to start %s: %s", engine, output)
	}
	return nil
}

// StopEngine stops the engine's systemd service.
func (s *Service) StopEngine(ctx context.Context, engine Engine) error {
	if !engineControlSupported(engine) {
		return fmt.Errorf("%s 无守护进程，不支持启停", engine)
	}
	output, exitCode, err := s.executor.RunCombined(ctx, "systemctl", "stop", serviceUnit(engine))
	if err != nil || exitCode != 0 {
		return fmt.Errorf("failed to stop %s: %s", engine, output)
	}
	return nil
}

// RestartEngine restarts the engine's systemd service.
func (s *Service) RestartEngine(ctx context.Context, engine Engine) error {
	if !engineControlSupported(engine) {
		return fmt.Errorf("%s 无守护进程，不支持启停", engine)
	}
	output, exitCode, err := s.executor.RunCombined(ctx, "systemctl", "restart", serviceUnit(engine))
	if err != nil || exitCode != 0 {
		return fmt.Errorf("failed to restart %s: %s", engine, output)
	}
	return nil
}

// enableSocketUnit is the systemd unit for Podman's Docker-compatible API
// socket. Podman-only; Docker's daemon is via docker.service.
const enableSocketUnit = "podman.socket"

// socketEnabled reports whether Podman's API socket unit is enabled at boot.
func (s *Service) socketEnabled(ctx context.Context) bool {
	_, exitCode, err := s.executor.RunCombined(ctx, "systemctl", "is-enabled", enableSocketUnit)
	return err == nil && exitCode == 0
}

// EnableSocket enables Podman's API socket unit at boot.
func (s *Service) EnableSocket(ctx context.Context, engine Engine) error {
	if !isPodmanEngine(engine) {
		return errors.New("socket 操作仅支持 Podman")
	}
	output, exitCode, err := s.executor.RunCombined(ctx, "systemctl", "enable", enableSocketUnit)
	if err != nil || exitCode != 0 {
		return fmt.Errorf("failed to enable %s socket: %s", engine, output)
	}
	return nil
}

// DisableSocket disables Podman's API socket unit.
func (s *Service) DisableSocket(ctx context.Context, engine Engine) error {
	if !isPodmanEngine(engine) {
		return errors.New("socket 操作仅支持 Podman")
	}
	output, exitCode, err := s.executor.RunCombined(ctx, "systemctl", "disable", enableSocketUnit)
	if err != nil || exitCode != 0 {
		return fmt.Errorf("failed to disable %s socket: %s", engine, output)
	}
	return nil
}

// GetInfo returns the engine's system info as a map.
func (s *Service) GetInfo(ctx context.Context, engine Engine) (map[string]any, error) {
	output, exitCode, err := s.executor.RunCombined(ctx, engineBinary(engine), "info", "--format", "{{json .}}")
	if err != nil || exitCode != 0 {
		return nil, fmt.Errorf("%s info failed: %s", engine, output)
	}

	var info map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &info); err != nil {
		return nil, fmt.Errorf("parse %s info: %w", engine, err)
	}

	return info, nil
}

// ConfigureMirror configures the engine's registry mirror. Kept for backward
// compat with the single-mirror endpoint; richer config goes through
// SetRegistryConfig.
func (s *Service) ConfigureMirror(ctx context.Context, engine Engine, mirrorURL string) error {
	return s.SetRegistryConfig(ctx, engine, RegistryConfig{Mirrors: []string{mirrorURL}})
}

// GetRegistryConfig reads the engine's mirror + insecure registries.
// Docker writes /etc/docker/daemon.json; Podman writes /etc/containers/registries.conf.
func (s *Service) GetRegistryConfig(ctx context.Context, engine Engine) RegistryConfig {
	if isPodmanEngine(engine) {
		return s.getPodmanRegistryConfig(ctx)
	}
	return s.getDockerRegistryConfig(ctx)
}

// SetRegistryConfig persists the engine's mirror + insecure registries.
func (s *Service) SetRegistryConfig(ctx context.Context, engine Engine, cfg RegistryConfig) error {
	if isPodmanEngine(engine) {
		return s.setPodmanRegistryConfig(ctx, cfg)
	}
	return s.setDockerRegistryConfig(ctx, cfg)
}

func (s *Service) getDockerRegistryConfig(ctx context.Context) RegistryConfig {
	var cfg RegistryConfig
	stdout, _, _ := s.executor.RunCombined(ctx, "cat", "/etc/docker/daemon.json")
	var config map[string]any
	if err := json.Unmarshal([]byte(stdout), &config); err != nil {
		return cfg // daemon.json 不可解析时返回默认配置
	}
	if mirrors, ok := config["registry-mirrors"].([]any); ok {
		for _, v := range mirrors {
			cfg.Mirrors = append(cfg.Mirrors, fmt.Sprint(v))
		}
	}
	if ins, ok := config["insecure-registries"].([]any); ok {
		for _, v := range ins {
			cfg.InsecureRegistries = append(cfg.InsecureRegistries, fmt.Sprint(v))
		}
	}
	return cfg
}

func (s *Service) setDockerRegistryConfig(ctx context.Context, cfg RegistryConfig) error {
	existing := "{}"
	stdout, exitCode, err := s.executor.RunCombined(ctx, "cat", "/etc/docker/daemon.json")
	if err == nil && exitCode == 0 {
		existing = strings.TrimSpace(stdout)
		if existing == "" {
			existing = "{}"
		}
	}

	var config map[string]any
	if err := json.Unmarshal([]byte(existing), &config); err != nil {
		config = make(map[string]any)
	}

	if len(cfg.Mirrors) == 0 {
		delete(config, "registry-mirrors")
	} else {
		config["registry-mirrors"] = cfg.Mirrors
	}
	if len(cfg.InsecureRegistries) == 0 {
		delete(config, "insecure-registries")
	} else {
		config["insecure-registries"] = cfg.InsecureRegistries
	}

	newConfig, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	encoded := base64.StdEncoding.EncodeToString(newConfig)
	writeCmd := fmt.Sprintf("mkdir -p /etc/docker && echo '%s' | base64 -d > /etc/docker/daemon.json", encoded)
	_, exitCode, err = s.executor.RunCombined(ctx, "bash", "-c", writeCmd)
	if err != nil || exitCode != 0 {
		return fmt.Errorf("failed to write daemon.json: %w", err)
	}

	return s.RestartEngine(ctx, "docker")
}

// registriesConfInsecure matches the location value inside a [[registry]] block.
var registriesConfInsecure = regexp.MustCompile(`location\s*=\s*"([^"]*)"`)

func (s *Service) getPodmanRegistryConfig(ctx context.Context) RegistryConfig {
	var cfg RegistryConfig
	stdout, _, _ := s.executor.RunCombined(ctx, "cat", "/etc/containers/registries.conf")
	if m := regexp.MustCompile(`unqualified-search-registries\s*=\s*\[(.*?)\]`).FindStringSubmatch(stdout); len(m) == 2 {
		for _, item := range regexp.MustCompile(`"([^"]*)"`).FindAllStringSubmatch(m[1], -1) {
			cfg.Mirrors = append(cfg.Mirrors, item[1])
		}
	}
	for _, seg := range strings.Split(stdout, "[[registry]]")[1:] {
		if !regexp.MustCompile(`insecure\s*=\s*true`).MatchString(seg) {
			continue
		}
		if loc := registriesConfInsecure.FindStringSubmatch(seg); len(loc) == 2 {
			cfg.InsecureRegistries = append(cfg.InsecureRegistries, loc[1])
		}
	}
	return cfg
}

func (s *Service) setPodmanRegistryConfig(ctx context.Context, cfg RegistryConfig) error {
	// Empty config → leave the distro default file untouched (avoids breaking
	// short-name resolution by writing an empty registries.conf).
	if len(cfg.Mirrors) == 0 && len(cfg.InsecureRegistries) == 0 {
		return nil
	}
	var b strings.Builder
	if len(cfg.Mirrors) > 0 {
		quoted := make([]string, 0, len(cfg.Mirrors))
		for _, m := range cfg.Mirrors {
			quoted = append(quoted, `"`+m+`"`)
		}
		fmt.Fprintf(&b, "unqualified-search-registries = [%s]\n", strings.Join(quoted, ", "))
	}
	for _, loc := range cfg.InsecureRegistries {
		fmt.Fprintf(&b, "\n[[registry]]\nlocation = \"%s\"\ninsecure = true\n", loc)
	}
	encoded := base64.StdEncoding.EncodeToString([]byte(b.String()))
	writeCmd := fmt.Sprintf("mkdir -p /etc/containers && echo '%s' | base64 -d > /etc/containers/registries.conf", encoded)
	_, exitCode, err := s.executor.RunCombined(ctx, "bash", "-c", writeCmd)
	if err != nil || exitCode != 0 {
		return fmt.Errorf("failed to write registries.conf: %w", err)
	}
	return nil
}

// authPath returns the stable auth file path for the engine. It's derived from
// the panel's $HOME so login, pull, and reads all agree regardless of how the
// panel was launched (root vs sudo vs normal user). Being $HOME-based keeps it
// consistent with what the engine would otherwise default to for docker.
func authPath(engine Engine) string {
	home, _ := os.UserHomeDir()
	if isPodmanEngine(engine) {
		return filepath.Join(home, ".config", "containers", "auth.json")
	}
	return filepath.Join(home, ".docker", "config.json")
}

// SetAuthEnv forces REGISTRY_AUTH_FILE / DOCKER_CONFIG to stable $HOME-based
// paths at panel startup. Child processes inherit os.Environ(), so every
// docker/podman command (login, pull, create, run) reads and writes the same
// auth file. Without REGISTRY_AUTH_FILE, rootless podman would instead write to
// ${XDG_RUNTIME_DIR}/containers/auth.json (e.g. /run/user/1000/...), which
// differs once the panel is relaunched as root.
func SetAuthEnv() {
	home, err := os.UserHomeDir()
	if err != nil {
		return
	}
	os.Setenv("REGISTRY_AUTH_FILE", filepath.Join(home, ".config", "containers", "auth.json"))
	os.Setenv("DOCKER_CONFIG", filepath.Join(home, ".docker"))
}

// GetLoggedInRegistries lists the registries the engine has credentials for,
// with the decoded username. Docker keeps them in ~/.docker/config.json; Podman
// in ~/.config/containers/auth.json — both under the "auths" key at authPath().
// Each entry's "auth" holds base64("username:password").
func (s *Service) GetLoggedInRegistries(ctx context.Context, engine Engine) []LoggedInRegistry {
	stdout, _, _ := s.executor.RunCombined(ctx, "cat", authPath(engine))
	var auth struct {
		Auths map[string]struct {
			Auth string `json:"auth"`
		} `json:"auths"`
	}
	if err := json.Unmarshal([]byte(stdout), &auth); err != nil {
		return []LoggedInRegistry{} // auth.json 不可解析时返回空列表
	}
	out := make([]LoggedInRegistry, 0, len(auth.Auths))
	for server, entry := range auth.Auths {
		username := ""
		if decoded, err := base64.StdEncoding.DecodeString(entry.Auth); err == nil {
			if i := strings.IndexByte(string(decoded), ':'); i >= 0 {
				username = string(decoded)[:i]
			}
		}
		out = append(out, LoggedInRegistry{Server: server, Username: username})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Server < out[j].Server })
	return out
}

// RegistryLogin authenticates to a private registry. Password goes over stdin
// (--password-stdin) so it never appears in argv/ps.
func (s *Service) RegistryLogin(ctx context.Context, engine Engine, server, username, password string) error {
	output, exitCode, err := s.executor.RunWithOptions(ctx,
		executor.CommandOptions{Stdin: password + "\n"},
		engineBinary(engine), "login", server, "--username", username, "--password-stdin")
	if err != nil || exitCode != 0 {
		return fmt.Errorf("%s login failed: %s", engine, truncateOutput(output, 500))
	}
	return nil
}

// RegistryLogout clears stored credentials for a registry.
func (s *Service) RegistryLogout(ctx context.Context, engine Engine, server string) error {
	output, exitCode, err := s.executor.RunCombined(ctx, engineBinary(engine), "logout", server)
	if err != nil || exitCode != 0 {
		return fmt.Errorf("%s logout failed: %s", engine, truncateOutput(output, 500))
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
func (s *Service) ListProjects(ctx context.Context, engine Engine) []ComposeProject {
	if isPodmanEngine(engine) {
		return []ComposeProject{}
	}

	output, exitCode, err := s.executor.RunCombined(ctx, "docker", "compose", "ls", "--format", "json")
	if err != nil || exitCode != 0 {
		output, exitCode, err = s.executor.RunCombined(ctx, "docker-compose", "ls", "--format", "json")
		if err != nil || exitCode != 0 {
			return []ComposeProject{} // docker compose 未安装时返回空列表
		}
	}

	var projects []ComposeProject
	trimmed := strings.TrimSpace(output)
	if trimmed == "" {
		return []ComposeProject{}
	}

	for line := range strings.SplitSeq(trimmed, "\n") {
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

	return projects
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
	for line := range strings.SplitSeq(strings.TrimSpace(output), "\n") {
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
	args = append(args, "logs", "--tail", strconv.Itoa(tail))

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
func (s *Service) CreateVolume(ctx context.Context, engine Engine, name, driver string, labels map[string]string) error {
	args := []string{"volume", "create"}
	if driver != "" {
		args = append(args, "--driver", driver)
	}
	for k, v := range labels {
		args = append(args, "--label", k+"="+v)
	}
	args = append(args, name)

	_, exitCode, err := s.executor.RunCombined(ctx, engineBinary(engine), args...)
	if err != nil || exitCode != 0 {
		return fmt.Errorf("%s volume create failed: %w", engine, err)
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
		return fmt.Errorf("%s volume rm failed: %w", engine, err)
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
		return fmt.Errorf("%s network create failed: %w", engine, err)
	}
	return nil
}

// RemoveNetwork removes a network.
func (s *Service) RemoveNetwork(ctx context.Context, engine Engine, id string) error {
	_, exitCode, err := s.executor.RunCombined(ctx, engineBinary(engine), "network", "rm", id)
	if err != nil || exitCode != 0 {
		return fmt.Errorf("%s network rm failed: %w", engine, err)
	}
	return nil
}

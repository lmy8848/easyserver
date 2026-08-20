package container

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	infracontainer "easyserver/internal/infra/container"
	"easyserver/internal/infra/errx"
	infrasystemd "easyserver/internal/infra/systemd"
	"easyserver/internal/util"
)

// 容器管理常量
const (
	ImagePullTimeout = 10 * time.Minute // 镜像拉取超时
	DefaultLogTail   = 100              // 默认日志行数
	MaxLogTail       = 10000            // 最大日志行数
	maxStreamBytes   = 10 * 1024 * 1024 // 流式输出最大字节数限制（10MB）
)

// engineBinary maps a engine name to the CLI binary that manages it (for compose & file ops).
func engineBinary(engine Engine) string {
	if engine == "podman" {
		return "podman"
	}
	return "docker"
}

// Service manages Docker containers, images, compose, volumes, and networks.
type Service struct{}

// NewService creates a new container Service.
func NewService() *Service {
	return &Service{}
}

func mapSummaryToContainer(s infracontainer.ContainerSummary) Container {
	name := ""
	if len(s.Names) > 0 {
		name = strings.TrimPrefix(s.Names[0], "/")
	}

	ports := make([]PortMapping, 0, len(s.Ports))
	for _, p := range s.Ports {
		hostPort := ""
		if p.PublicPort > 0 {
			if p.IP != "" && p.IP != "0.0.0.0" {
				hostPort = fmt.Sprintf("%s:%d", p.IP, p.PublicPort)
			} else {
				hostPort = strconv.Itoa(int(p.PublicPort))
			}
		}
		ports = append(ports, PortMapping{
			HostPort:      hostPort,
			ContainerPort: strconv.Itoa(int(p.PrivatePort)),
			Protocol:      p.Type,
		})
	}

	mounts := make([]string, 0, len(s.Mounts))
	for _, m := range s.Mounts {
		mounts = append(mounts, m.Source+":"+m.Destination)
	}

	networks := make([]string, 0, len(s.NetworkSettings.Networks))
	for netName := range s.NetworkSettings.Networks {
		networks = append(networks, netName)
	}

	labelsStr := ""
	if len(s.Labels) > 0 {
		labelPairs := make([]string, 0, len(s.Labels))
		for k, v := range s.Labels {
			labelPairs = append(labelPairs, k+"="+v)
		}
		labelsStr = strings.Join(labelPairs, ",")
	}

	createdAt := ""
	if s.Created > 0 {
		createdAt = time.Unix(s.Created, 0).Format(time.RFC3339)
	}

	return Container{
		ID:         s.ID,
		Name:       name,
		Image:      s.Image,
		Status:     s.Status,
		State:      s.State,
		Ports:      ports,
		CreatedAt:  createdAt,
		Command:    s.Command,
		Labels:     labelsStr,
		Mounts:     strings.Join(mounts, ","),
		Networks:   strings.Join(networks, ","),
		Size:       humanSize(s.SizeRw),
		RunningFor: "",
	}
}

func mapInspectToContainer(insp infracontainer.ContainerInspect) Container {
	name := strings.TrimPrefix(insp.Name, "/")

	ports := make([]PortMapping, 0)
	for portProto, bindings := range insp.NetworkSettings.Ports {
		parts := strings.Split(portProto, "/")
		cPort := parts[0]
		proto := "tcp"
		if len(parts) > 1 {
			proto = parts[1]
		}
		if len(bindings) == 0 {
			ports = append(ports, PortMapping{
				HostPort:      "",
				ContainerPort: cPort,
				Protocol:      proto,
			})
		} else {
			for _, b := range bindings {
				hPort := b.HostPort
				if b.HostIP != "" && b.HostIP != "0.0.0.0" {
					hPort = b.HostIP + ":" + hPort
				}
				ports = append(ports, PortMapping{
					HostPort:      hPort,
					ContainerPort: cPort,
					Protocol:      proto,
				})
			}
		}
	}

	mounts := make([]string, 0, len(insp.Mounts))
	for _, m := range insp.Mounts {
		mounts = append(mounts, m.Source+":"+m.Destination)
	}

	networks := make([]string, 0, len(insp.NetworkSettings.Networks))
	for netName := range insp.NetworkSettings.Networks {
		networks = append(networks, netName)
	}

	labelsStr := ""
	if len(insp.Config.Labels) > 0 {
		labelPairs := make([]string, 0, len(insp.Config.Labels))
		for k, v := range insp.Config.Labels {
			labelPairs = append(labelPairs, k+"="+v)
		}
		labelsStr = strings.Join(labelPairs, ",")
	}

	state := insp.State.Status
	if state == "" {
		if insp.State.Running {
			state = "running"
		} else {
			state = "exited"
		}
	}

	cmdStr := ""
	if len(insp.Config.Cmd) > 0 {
		cmdStr = strings.Join(insp.Config.Cmd, " ")
	} else if insp.Path != "" {
		cmdStr = insp.Path + " " + strings.Join(insp.Args, " ")
	}

	return Container{
		ID:         insp.ID,
		Name:       name,
		Image:      insp.Config.Image,
		Status:     state,
		State:      state,
		Ports:      ports,
		CreatedAt:  insp.Created,
		Command:    strings.TrimSpace(cmdStr),
		Labels:     labelsStr,
		Mounts:     strings.Join(mounts, ","),
		Networks:   strings.Join(networks, ","),
		Size:       "",
		RunningFor: "",
	}
}

func mapSummaryToImages(sum infracontainer.ImageSummary) []Image {
	createdAt := ""
	if sum.Created > 0 {
		createdAt = time.Unix(sum.Created, 0).Format(time.RFC3339)
	}

	if len(sum.RepoTags) == 0 {
		return []Image{{
			ID:         sum.ID,
			Repository: "<none>",
			Tag:        "<none>",
			Size:       humanSize(sum.Size),
			CreatedAt:  createdAt,
			Labels:     sum.Labels,
		}}
	}

	images := make([]Image, 0, len(sum.RepoTags))
	for _, rt := range sum.RepoTags {
		repo, tag := splitRepoTag(rt)
		images = append(images, Image{
			ID:         sum.ID,
			Repository: repo,
			Tag:        tag,
			Size:       humanSize(sum.Size),
			CreatedAt:  createdAt,
			Labels:     sum.Labels,
		})
	}
	return images
}

func splitRepoTag(rt string) (string, string) {
	if i := strings.LastIndex(rt, ":"); i != -1 {
		// Only split at colon if the substring after it contains no slash
		// (a colon after a slash, like in host:5000/repo, is a registry port, not a tag separator)
		if !strings.Contains(rt[i+1:], "/") {
			return rt[:i], rt[i+1:]
		}
	}
	return rt, "latest"
}

func isPodmanEngine(engine Engine) bool { return engineBinary(engine) == "podman" }

// rejectManaged refuses mutating operations on EasyServer-managed database
// containers. The generic Container resource may view but never take over,
// edit or delete a managed database container; its lifecycle belongs to the
// database module (PRD: generic Container cannot bypass database rules).
func (s *Service) rejectManaged(ctx context.Context, engine Engine, id string) error {
	insp, err := infracontainer.DefaultClient().ContainerInspect(ctx, infracontainer.Engine(engine), id)
	if err != nil {
		return nil //nolint:nilerr // 非受管容器时返回 nil，让后续操作自然处理
	}
	if insp.Config.Labels != nil && insp.Config.Labels["com.easyserver.managed"] == "true" {
		return errors.New("受管数据库容器，请通过数据库模块操作")
	}
	return nil
}

// --- Container operations ---

// checkEngine checks if the given engine socket is accessible.
func (s *Service) checkEngine(ctx context.Context, engine Engine) error {
	_, err := infracontainer.DefaultClient().Ping(ctx, infracontainer.Engine(engine))
	if err != nil {
		return errx.Unavailable("%s is not running or socket is not accessible: %w", engine, err)
	}
	return nil
}

// ListContainers returns all containers for the given engine.
func (s *Service) ListContainers(ctx context.Context, engine Engine, all bool) ([]Container, error) {
	if err := s.checkEngine(ctx, engine); err != nil {
		return nil, err
	}

	summaries, err := infracontainer.DefaultClient().ContainerList(ctx, infracontainer.Engine(engine), all)
	if err != nil {
		return nil, errx.Internal("%s ps failed: %w", engine, err)
	}

	containers := make([]Container, 0, len(summaries))
	for _, sum := range summaries {
		containers = append(containers, mapSummaryToContainer(sum))
	}
	return containers, nil
}

// GetContainer returns details of a specific container.
func (s *Service) GetContainer(ctx context.Context, engine Engine, id string) (*Container, error) {
	insp, err := infracontainer.DefaultClient().ContainerInspect(ctx, infracontainer.Engine(engine), id)
	if err != nil {
		return nil, errx.NotFound("container not found: %s (%v)", id, err)
	}
	c := mapInspectToContainer(insp)
	return &c, nil
}

// StartContainer starts a container.
func (s *Service) StartContainer(ctx context.Context, engine Engine, id string) error {
	if err := s.rejectManaged(ctx, engine, id); err != nil {
		return err
	}
	if err := infracontainer.DefaultClient().ContainerStart(ctx, infracontainer.Engine(engine), id); err != nil {
		return errx.Internal("%s start failed: %w", engine, err)
	}
	return nil
}

// StopContainer stops a container.
func (s *Service) StopContainer(ctx context.Context, engine Engine, id string) error {
	if err := s.rejectManaged(ctx, engine, id); err != nil {
		return err
	}
	if err := infracontainer.DefaultClient().ContainerStop(ctx, infracontainer.Engine(engine), id, 10); err != nil {
		return errx.Internal("%s stop failed: %w", engine, err)
	}
	return nil
}

// RestartContainer restarts a container.
func (s *Service) RestartContainer(ctx context.Context, engine Engine, id string) error {
	if err := s.rejectManaged(ctx, engine, id); err != nil {
		return err
	}
	if err := infracontainer.DefaultClient().ContainerRestart(ctx, infracontainer.Engine(engine), id, 10); err != nil {
		return errx.Internal("%s restart failed: %w", engine, err)
	}
	return nil
}

// PauseContainer pauses a container.
func (s *Service) PauseContainer(ctx context.Context, engine Engine, id string) error {
	if err := s.rejectManaged(ctx, engine, id); err != nil {
		return err
	}
	if err := infracontainer.DefaultClient().ContainerPause(ctx, infracontainer.Engine(engine), id); err != nil {
		return errx.Internal("%s pause failed: %w", engine, err)
	}
	return nil
}

// UnpauseContainer unpauses a container.
func (s *Service) UnpauseContainer(ctx context.Context, engine Engine, id string) error {
	if err := s.rejectManaged(ctx, engine, id); err != nil {
		return err
	}
	if err := infracontainer.DefaultClient().ContainerUnpause(ctx, infracontainer.Engine(engine), id); err != nil {
		return errx.Internal("%s unpause failed: %w", engine, err)
	}
	return nil
}

// RemoveContainer removes a container.
func (s *Service) RemoveContainer(ctx context.Context, engine Engine, id string, force bool) error {
	if err := s.rejectManaged(ctx, engine, id); err != nil {
		return err
	}
	if err := infracontainer.DefaultClient().ContainerRemove(ctx, infracontainer.Engine(engine), id, force); err != nil {
		return errx.Internal("%s rm failed: %w", engine, err)
	}
	return nil
}

// GetContainerLogs returns container logs.
func (s *Service) GetContainerLogs(ctx context.Context, engine Engine, id string, tail int) (string, error) {
	// Apply default tail when not specified
	if tail <= 0 {
		tail = DefaultLogTail
	}
	rc, err := infracontainer.DefaultClient().ContainerLogs(ctx, infracontainer.Engine(engine), id, tail, true, true)
	if err != nil {
		return "", errx.Internal("%s logs failed: %w", engine, err)
	}
	defer rc.Close()

	rawBytes, err := io.ReadAll(io.LimitReader(rc, maxStreamBytes))
	if err != nil {
		return "", errx.Internal("read logs failed: %w", err)
	}
	if len(rawBytes) == 0 {
		return "", nil
	}

	var stdout, stderr bytes.Buffer
	if err := infracontainer.DemuxLogs(bytes.NewReader(rawBytes), &stdout, &stderr); err == nil && (stdout.Len() > 0 || stderr.Len() > 0) {
		return stdout.String() + stderr.String(), nil
	}
	return string(rawBytes), nil
}

var validContainerIDRE = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]{0,127}$`)

func validateContainerID(id string) error {
	if id == "" || strings.HasPrefix(id, "-") || !validContainerIDRE.MatchString(id) {
		return errx.BadRequest("invalid container ID or name: %s", id)
	}
	return nil
}

// ExecInContainer executes a command in a running container.
func (s *Service) ExecInContainer(ctx context.Context, engine Engine, id, cmd string) (string, error) {
	if err := validateContainerID(id); err != nil {
		return "", err
	}
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
		return "", errx.BadRequest("command cannot be empty")
	}

	createResp, err := infracontainer.DefaultClient().ContainerExecCreate(ctx, infracontainer.Engine(engine), id, infracontainer.ExecCreateRequest{
		AttachStdout: true,
		AttachStderr: true,
		Cmd:          []string{"sh", "-c", cmd},
	})
	if err != nil {
		return "", errx.Internal("%s exec create failed: %w", engine, err)
	}

	rc, err := infracontainer.DefaultClient().ContainerExecStart(ctx, infracontainer.Engine(engine), createResp.ID, infracontainer.ExecStartRequest{})
	if err != nil {
		return "", errx.Internal("%s exec start failed: %w", engine, err)
	}
	defer rc.Close()

	rawBytes, err := io.ReadAll(io.LimitReader(rc, maxStreamBytes))
	if err != nil {
		return "", errx.Internal("%s exec read failed: %w", engine, err)
	}

	var stdout, stderr bytes.Buffer
	output := string(rawBytes)
	if err := infracontainer.DemuxLogs(bytes.NewReader(rawBytes), &stdout, &stderr); err == nil && (stdout.Len() > 0 || stderr.Len() > 0) {
		output = stdout.String() + stderr.String()
	}

	inspectResp, err := infracontainer.DefaultClient().ContainerExecInspect(ctx, infracontainer.Engine(engine), createResp.ID)
	if err != nil {
		return output, errx.Internal("%s exec inspect failed: %w", engine, err)
	}
	if inspectResp.ExitCode != 0 {
		return output, errx.Internal("%s exec failed: exit code %d: %s", engine, inspectResp.ExitCode, output)
	}
	return output, nil
}

// CreateContainer creates a new container.
func (s *Service) CreateContainer(ctx context.Context, engine Engine, req CreateRequest) (string, error) {
	imageRef := req.Image
	if isPodmanEngine(engine) {
		imageRef = expandImageRef(imageRef)
	}

	var cmd []string
	if req.Command != "" {
		cmd = strings.Fields(req.Command)
	}

	env := make([]string, 0, len(req.EnvVars))
	for k, v := range req.EnvVars {
		env = append(env, fmt.Sprintf("%s=%s", k, v))
	}

	exposedPorts := make(map[string]struct{})
	portBindings := make(map[string][]infracontainer.PortBinding)
	for _, p := range req.Ports {
		proto := p.Protocol
		if proto == "" {
			proto = "tcp"
		}
		key := fmt.Sprintf("%s/%s", p.ContainerPort, proto)
		exposedPorts[key] = struct{}{}
		if p.HostPort != "" {
			hostIP := ""
			hostPort := p.HostPort
			if strings.Contains(hostPort, ":") {
				// Split at the final colon to handle IPv6 addresses like ::1
				idx := strings.LastIndex(hostPort, ":")
				hostIP = hostPort[:idx]
				hostPort = hostPort[idx+1:]
			}
			portBindings[key] = append(portBindings[key], infracontainer.PortBinding{
				HostIP:   hostIP,
				HostPort: hostPort,
			})
		}
	}

	binds := make([]string, 0, len(req.Volumes))
	for _, v := range req.Volumes {
		mode := ""
		if v.Mode != "" {
			mode = ":" + v.Mode
		}
		binds = append(binds, fmt.Sprintf("%s:%s%s", v.Source, v.Destination, mode))
	}

	hostConfig := &infracontainer.HostConfig{
		Binds:        binds,
		PortBindings: portBindings,
		AutoRemove:   req.AutoRemove,
		Memory:       req.Memory,
		NanoCPUs:     int64(req.CPUs * 1e9),
	}

	if req.RestartPolicy != "" {
		hostConfig.RestartPolicy = &infracontainer.RestartPolicy{
			Name: req.RestartPolicy,
		}
	}

	if len(req.Networks) > 1 {
		return "", errx.BadRequest("multiple networks not supported; please specify only one network")
	}
	if len(req.Networks) > 0 {
		hostConfig.NetworkMode = req.Networks[0]
	}

	createReq := infracontainer.ContainerCreateRequest{
		ContainerConfig: infracontainer.ContainerConfig{
			Image:        imageRef,
			Cmd:          cmd,
			Env:          env,
			Labels:       req.Labels,
			ExposedPorts: exposedPorts,
		},
		HostConfig: hostConfig,
	}

	resp, err := infracontainer.DefaultClient().ContainerCreate(ctx, infracontainer.Engine(engine), req.Name, createReq)
	if err != nil {
		return "", errx.Internal("%s create failed: %w", engine, err)
	}
	return resp.ID, nil
}

// ListImages returns all images for the given engine.
func (s *Service) ListImages(ctx context.Context, engine Engine) ([]Image, error) {
	if err := s.checkEngine(ctx, engine); err != nil {
		return nil, err
	}

	summaries, err := infracontainer.DefaultClient().ImageList(ctx, infracontainer.Engine(engine))
	if err != nil {
		return nil, errx.Internal("%s images failed: %w", engine, err)
	}

	var images []Image
	for _, sum := range summaries {
		images = append(images, mapSummaryToImages(sum)...)
	}
	return images, nil
}

// PullImage pulls an image.
func (s *Service) PullImage(ctx context.Context, engine Engine, image string) error {
	pullCtx, cancel := context.WithTimeout(ctx, ImagePullTimeout)
	defer cancel()

	rc, err := infracontainer.DefaultClient().ImagePull(pullCtx, infracontainer.Engine(engine), image, "")
	if err != nil {
		return errx.Internal("%s pull failed: %w", engine, err)
	}
	defer rc.Close()

	// Decode JSON stream line-by-line and check for errors
	decoder := json.NewDecoder(rc)
	for {
		var event struct {
			Error       string `json:"error"`
			ErrorDetail struct {
				Message string `json:"message"`
			} `json:"errorDetail"`
		}
		if err := decoder.Decode(&event); err != nil {
			if err == io.EOF {
				break
			}
			return errx.Internal("failed to decode pull response: %w", err)
		}
		if event.Error != "" {
			return errx.Internal("image pull error: %s", event.Error)
		}
		if event.ErrorDetail.Message != "" {
			return errx.Internal("image pull error: %s", event.ErrorDetail.Message)
		}
	}
	return nil
}

// RemoveImage removes an image.
func (s *Service) RemoveImage(ctx context.Context, engine Engine, id string, force bool) error {
	_, err := infracontainer.DefaultClient().ImageRemove(ctx, infracontainer.Engine(engine), id, force)
	if err != nil {
		return errx.Internal("%s rmi failed: %w", engine, err)
	}
	return nil
}

// PruneImages cleans up unused images.
func (s *Service) PruneImages(ctx context.Context, engine Engine) (*infracontainer.ImagesPruneReport, error) {
	report, err := infracontainer.DefaultClient().ImagesPrune(ctx, infracontainer.Engine(engine))
	if err != nil {
		return nil, errx.Internal("%s prune images failed: %w", engine, err)
	}
	return &report, nil
}

// GetContainerStats returns real-time resource usage stats for a container.
func (s *Service) GetContainerStats(ctx context.Context, engine Engine, id string) (*Stats, error) {
	rc, err := infracontainer.DefaultClient().ContainerStats(ctx, infracontainer.Engine(engine), id, false)
	if err != nil {
		return nil, errx.Internal("%s stats failed: %w", engine, err)
	}
	defer rc.Close()

	var raw infracontainer.StatsJSON
	if err := json.NewDecoder(rc).Decode(&raw); err != nil {
		return nil, fmt.Errorf("parse stats json: %w", err)
	}

	stats := &Stats{}
	// CPU calculation: (cpu_delta / system_cpu_delta) * number_cpus * 100.0
	// Check if precpu_stats is valid (Podman may return zeroed precpu_stats on first sample)
	preCPUValid := raw.PreCPUStats.CPUUsage.TotalUsage > 0 || raw.PreCPUStats.SystemCPUUsage > 0
	if preCPUValid {
		cpuDelta := float64(raw.CPUStats.CPUUsage.TotalUsage) - float64(raw.PreCPUStats.CPUUsage.TotalUsage)
		systemDelta := float64(raw.CPUStats.SystemCPUUsage) - float64(raw.PreCPUStats.SystemCPUUsage)
		numCPUs := float64(raw.CPUStats.OnlineCPUs)
		if numCPUs == 0 {
			numCPUs = float64(len(raw.CPUStats.CPUUsage.PercpuUsage))
		}
		if numCPUs == 0 {
			numCPUs = 1
		}
		if systemDelta > 0 && cpuDelta > 0 {
			stats.CPUPercent = (cpuDelta / systemDelta) * numCPUs * 100.0
		}
	}

	stats.MemUsage = int64(raw.MemoryStats.Usage)
	stats.MemLimit = int64(raw.MemoryStats.Limit)
	if stats.MemLimit > 0 {
		stats.MemPercent = (float64(stats.MemUsage) / float64(stats.MemLimit)) * 100.0
	}

	for _, net := range raw.Networks {
		stats.NetRx += int64(net.RxBytes)
		stats.NetTx += int64(net.TxBytes)
	}

	for _, bio := range raw.BlkioStats.IOServiceBytesRecursive {
		op := strings.ToLower(bio.Op)
		switch op {
		case "read":
			stats.BlockRead += int64(bio.Value)
		case "write":
			stats.BlockWrite += int64(bio.Value)
		}
	}

	stats.PIDs = int(raw.PidsStats.Current)

	return stats, nil
}

// RenameContainer renames a container.
func (s *Service) RenameContainer(ctx context.Context, engine Engine, id, newName string) error {
	if err := s.rejectManaged(ctx, engine, id); err != nil {
		return err
	}
	if strings.TrimSpace(id) == "" {
		return errx.BadRequest("container ID cannot be empty")
	}
	if strings.TrimSpace(newName) == "" {
		return errx.BadRequest("new container name cannot be empty")
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

	if err := s.checkEngine(ctx, engine); err != nil {
		return err
	}
	if err := infracontainer.DefaultClient().ContainerRename(ctx, infracontainer.Engine(engine), id, newName); err != nil {
		return errx.Internal("%s rename failed: %w", engine, err)
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

	output, err := exec.CommandContext(ctx, engineBinary(engine), args...).CombinedOutput()
	if err != nil {
		return errx.Internal("%s update failed: %s", engine, output)
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
		// Podman: "Installed" means binary exists. "Running" means podman.socket is accessible.
		stdout, err := exec.CommandContext(ctx, bin, "--version").CombinedOutput()
		if err != nil {
			status.Installed = false
			return status, nil //nolint:nilerr // 引擎未安装时返回未安装状态
		}
		status.Installed = true
		status.Version = extractVersion(string(stdout))
		_, pingErr := infracontainer.DefaultClient().Ping(ctx, infracontainer.EnginePodman)
		status.Running = (pingErr == nil)
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
	stdout, err := exec.CommandContext(ctx, engineBinary(EngineDocker), "version", "--format", "{{.Server.Version}}").CombinedOutput()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(stdout))
}

// unitExists reports whether a systemd unit file is present.
func (s *Service) unitExists(ctx context.Context, unit string) bool {
	props, err := infrasystemd.DefaultClient().GetUnitPropertiesContext(ctx, unit)
	if err == nil && props != nil {
		if ls, ok := props["LoadState"].(string); ok && ls != "" && ls != "not-found" {
			return true
		}
	}
	return false
}

// unitActive reports whether a systemd unit is currently active.
func (s *Service) unitActive(ctx context.Context, unit string) bool {
	return util.SystemdUnitActive(ctx, unit)
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
	stdout, err := exec.CommandContext(ctx, "cat", "/etc/os-release").CombinedOutput()
	if err != nil {
		return "unknown"
	}

	lower := strings.ToLower(string(stdout))
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

	_, err := exec.CommandContext(ctx, "which", "curl").CombinedOutput()
	if err != nil {
		return errx.Internal("curl 未安装，请先安装 curl: %w", err)
	}

	log.Println("docker: downloading install script...")
	dlCtx, dlCancel := context.WithTimeout(ctx, 2*time.Minute)
	defer dlCancel()
	output, err := exec.CommandContext(dlCtx, "bash", "-c",
		"curl -fsSL https://get.docker.com -o /tmp/get-docker.sh").CombinedOutput()
	if err != nil {
		return fmt.Errorf("下载 Docker 安装脚本失败: %s", truncateOutput(string(output), 500))
	}

	log.Println("docker: running install script...")
	runCtx, runCancel := context.WithTimeout(ctx, 10*time.Minute)
	defer runCancel()
	output, err = exec.CommandContext(runCtx, "sh", "/tmp/get-docker.sh").CombinedOutput()
	if err != nil {
		log.Printf("docker: installation failed: %s", output)
		return fmt.Errorf("docker 安装脚本执行失败: %s", truncateOutput(string(output), 500))
	}
	log.Println("docker: enabling service...")
	client := infrasystemd.DefaultClient()
	if _, _, err := client.EnableUnitFilesContext(ctx, []string{"docker.service"}, false, false); err != nil {
		log.Printf("docker: enable failed: %v", err)
		return fmt.Errorf("启用 Docker 服务失败: %w", err)
	}

	log.Println("docker: starting service...")
	if _, err := client.StartUnitContext(ctx, "docker.service", "replace"); err != nil {
		log.Printf("docker: start failed: %v", err)
		return fmt.Errorf("启动 Docker 服务失败: %w", err)
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
		pkgCmd = "apt-get update && apt-get install -y podman"
	case "centos", "rhel", "fedora", "almalinux", "rocky":
		pkgCmd = "dnf install -y podman || yum install -y podman"
	case "alpine":
		pkgCmd = "apk add podman"
	case "arch":
		pkgCmd = "pacman -Sy --noconfirm podman"
	default:
		return fmt.Errorf("不支持的操作系统类型: %s，请手动安装 Podman", os)
	}

	log.Printf("podman: running install command: %s", pkgCmd)
	runCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	output, err := exec.CommandContext(runCtx, "sh", "-c", pkgCmd).CombinedOutput()
	if err != nil {
		log.Printf("podman: installation failed: %s", output)
		return fmt.Errorf("podman 安装失败: %s", truncateOutput(string(output), 500))
	}

	log.Println("podman: package installation completed, enabling and starting podman.socket...")
	if err := s.EnableSocket(ctx, EnginePodman); err != nil {
		log.Printf("podman: failed enabling socket: %v", err)
		return fmt.Errorf("启用并启动 podman.socket 失败: %w", err)
	}

	log.Println("podman: installation completed successfully")
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
// can be started/stopped. Both Docker (docker.service) and Podman (podman.socket)
// are supported.
func engineControlSupported(_ Engine) bool { return true }

// StartEngine starts the engine's systemd service.
func (s *Service) StartEngine(ctx context.Context, engine Engine) error {
	if !engineControlSupported(engine) {
		return errx.Unavailable("%s 不支持启停", engine)
	}
	unit := serviceUnit(engine)
	if _, err := infrasystemd.DefaultClient().StartUnitContext(ctx, unit, "replace"); err != nil {
		return fmt.Errorf("failed to start %s: %w", engine, err)
	}
	return nil
}

// StopEngine stops the engine's systemd service.
func (s *Service) StopEngine(ctx context.Context, engine Engine) error {
	if !engineControlSupported(engine) {
		return errx.Unavailable("%s 不支持启停", engine)
	}
	unit := serviceUnit(engine)
	if _, err := infrasystemd.DefaultClient().StopUnitContext(ctx, unit, "replace"); err != nil {
		return fmt.Errorf("failed to stop %s: %w", engine, err)
	}
	return nil
}

// RestartEngine restarts the engine's systemd service.
func (s *Service) RestartEngine(ctx context.Context, engine Engine) error {
	if !engineControlSupported(engine) {
		return errx.Unavailable("%s 不支持启停", engine)
	}
	unit := serviceUnit(engine)
	if _, err := infrasystemd.DefaultClient().RestartUnitContext(ctx, unit, "replace"); err != nil {
		return fmt.Errorf("failed to restart %s: %w", engine, err)
	}
	return nil
}

// enableSocketUnit is the systemd unit for Podman's Docker-compatible API
// socket. Podman-only; Docker's daemon is via docker.service.
const enableSocketUnit = "podman.socket"

// socketEnabled reports whether Podman's API socket unit is enabled at boot.
func (s *Service) socketEnabled(ctx context.Context) bool {
	return util.SystemdUnitEnabled(ctx, enableSocketUnit)
}

// EnableSocket enables and starts Podman's API socket unit.
func (s *Service) EnableSocket(ctx context.Context, engine Engine) error {
	if !isPodmanEngine(engine) {
		return errors.New("socket 操作仅支持 Podman")
	}
	if _, _, err := infrasystemd.DefaultClient().EnableUnitFilesContext(ctx, []string{enableSocketUnit}, false, false); err != nil {
		return fmt.Errorf("failed to enable %s socket: %w", engine, err)
	}
	if _, err := infrasystemd.DefaultClient().StartUnitContext(ctx, enableSocketUnit, "replace"); err != nil {
		return fmt.Errorf("failed to start %s socket: %w", engine, err)
	}
	return nil
}

// GetInfo returns the engine's system info as a map.
func (s *Service) GetInfo(ctx context.Context, engine Engine) (map[string]any, error) {
	if err := s.checkEngine(ctx, engine); err != nil {
		return nil, err
	}

	info, err := infracontainer.DefaultClient().Info(ctx, infracontainer.Engine(engine))
	if err != nil {
		return nil, errx.Internal("%s info failed: %w", engine, err)
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
	stdout, _ := exec.CommandContext(ctx, "cat", "/etc/docker/daemon.json").CombinedOutput()
	var config map[string]any
	if err := json.Unmarshal(stdout, &config); err != nil {
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
	stdout, err := exec.CommandContext(ctx, "cat", "/etc/docker/daemon.json").CombinedOutput()
	if err == nil {
		existing = strings.TrimSpace(string(stdout))
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
	_, err = exec.CommandContext(ctx, "bash", "-c", writeCmd).CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to write daemon.json: %w", err)
	}

	return s.RestartEngine(ctx, "docker")
}

// registriesConfInsecure matches the location value inside a [[registry]] block.
var registriesConfInsecure = regexp.MustCompile(`location\s*=\s*"([^"]*)"`)

func (s *Service) getPodmanRegistryConfig(ctx context.Context) RegistryConfig {
	stdout, _ := exec.CommandContext(ctx, "cat", "/etc/containers/registries.conf").CombinedOutput()
	return parseRegistriesConf(string(stdout))
}

// parseRegistriesConf 解析 /etc/containers/registries.conf：mirror（unqualified
// search 列表）与 insecure registry（带 insecure=true 的 [[registry]] 块）。
func parseRegistriesConf(content string) RegistryConfig {
	var cfg RegistryConfig
	if m := regexp.MustCompile(`unqualified-search-registries\s*=\s*\[(.*?)\]`).FindStringSubmatch(content); len(m) == 2 {
		for _, item := range regexp.MustCompile(`"([^"]*)"`).FindAllStringSubmatch(m[1], -1) {
			cfg.Mirrors = append(cfg.Mirrors, item[1])
		}
	}
	for _, seg := range strings.Split(content, "[[registry]]")[1:] {
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
	_, err := exec.CommandContext(ctx, "bash", "-c", writeCmd).CombinedOutput()
	if err != nil {
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
	stdout, _ := exec.CommandContext(ctx, "cat", authPath(engine)).CombinedOutput()
	var auth struct {
		Auths map[string]struct {
			Auth string `json:"auth"`
		} `json:"auths"`
	}
	if err := json.Unmarshal(stdout, &auth); err != nil {
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
	cmd := exec.CommandContext(ctx, engineBinary(engine), "login", server, "--username", username, "--password-stdin")
	cmd.Stdin = strings.NewReader(password + "\n")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return errx.Internal("%s login failed: %s", engine, truncateOutput(string(out), 500))
	}
	return nil
}

// RegistryLogout clears stored credentials for a registry.
func (s *Service) RegistryLogout(ctx context.Context, engine Engine, server string) error {
	output, err := exec.CommandContext(ctx, engineBinary(engine), "logout", server).CombinedOutput()
	if err != nil {
		return errx.Internal("%s logout failed: %s", engine, truncateOutput(string(output), 500))
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

	output, err := exec.CommandContext(ctx, "docker", "compose", "ls", "--format", "json").CombinedOutput()
	if err != nil {
		output, err = exec.CommandContext(ctx, "docker-compose", "ls", "--format", "json").CombinedOutput()
		if err != nil {
			return []ComposeProject{} // docker compose 未安装时返回空列表
		}
	}

	var projects []ComposeProject
	trimmed := strings.TrimSpace(string(output))
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

	output, err := exec.CommandContext(ctx, "docker", "compose", "-f", configFile, "config", "--services").CombinedOutput()
	if err != nil {
		return nil
	}

	var services []string
	for line := range strings.SplitSeq(strings.TrimSpace(string(output)), "\n") {
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

	output, err := exec.CommandContext(ctx, bin, args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("compose up failed: %s", output)
	}
	return nil
}

// ComposeDown runs compose down for a project.
func (s *Service) ComposeDown(ctx context.Context, engine Engine, projectDir string) error {
	composeFile := s.findComposeFile(projectDir)
	bin, args := s.composeCommand(engine, composeFile)
	args = append(args, "down")

	output, err := exec.CommandContext(ctx, bin, args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("compose down failed: %s", output)
	}
	return nil
}

// ComposeRestart runs compose restart for a project.
func (s *Service) ComposeRestart(ctx context.Context, engine Engine, projectDir string) error {
	composeFile := s.findComposeFile(projectDir)
	bin, args := s.composeCommand(engine, composeFile)
	args = append(args, "restart")

	output, err := exec.CommandContext(ctx, bin, args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("compose restart failed: %s", output)
	}
	return nil
}

// ComposeGetLogs returns logs for a compose project.
func (s *Service) ComposeGetLogs(ctx context.Context, engine Engine, projectDir string, tail int) (string, error) {
	composeFile := s.findComposeFile(projectDir)
	bin, args := s.composeCommand(engine, composeFile)
	args = append(args, "logs", "--tail", strconv.Itoa(tail))

	output, err := exec.CommandContext(ctx, bin, args...).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("compose logs failed: %s", output)
	}
	return string(output), nil
}

func isSafeProjectPath(projectDir string) (string, error) {
	if projectDir == "" || !filepath.IsAbs(projectDir) || strings.Contains(projectDir, "\x00") || strings.Contains(projectDir, "..") {
		return "", errors.New("invalid project directory")
	}
	cleanDir := filepath.Clean(projectDir)
	if !filepath.IsAbs(cleanDir) || cleanDir == "/" || cleanDir == "." || strings.Contains(cleanDir, "..") {
		return "", errors.New("invalid project directory path")
	}
	return cleanDir, nil
}

// ComposeGetConfig reads the compose file content.
func (s *Service) ComposeGetConfig(ctx context.Context, projectDir string) (string, error) {
	cleanDir, err := isSafeProjectPath(projectDir)
	if err != nil {
		return "", errx.BadRequest("invalid project directory: must be an absolute path without traversal")
	}
	composeFile := s.findComposeFile(cleanDir)
	if composeFile == "" {
		return "", fmt.Errorf("no compose file found in %s", cleanDir)
	}
	rel, err := filepath.Rel(cleanDir, composeFile)
	if err != nil || strings.HasPrefix(rel, "..") || rel == ".." {
		return "", errx.BadRequest("invalid compose file path")
	}

	f, err := os.OpenFile(composeFile, os.O_RDONLY, 0)
	if err != nil {
		return "", fmt.Errorf("read compose file: %w", err)
	}
	defer f.Close()
	data, err := io.ReadAll(f)
	if err != nil {
		return "", fmt.Errorf("read compose file: %w", err)
	}
	return string(data), nil
}

// ComposeSaveConfig writes content to the compose file.
func (s *Service) ComposeSaveConfig(ctx context.Context, projectDir, content string) error {
	cleanDir, err := isSafeProjectPath(projectDir)
	if err != nil {
		return errx.BadRequest("invalid project directory: must be an absolute path without traversal")
	}
	composeFile := s.findComposeFile(cleanDir)
	if composeFile == "" {
		baseName := filepath.Base("docker-compose.yml")
		composeFile = filepath.Join(cleanDir, baseName)
	}
	rel, err := filepath.Rel(cleanDir, composeFile)
	if err != nil || strings.HasPrefix(rel, "..") || rel == ".." {
		return errx.BadRequest("invalid compose file path")
	}
	if fi, err := os.Lstat(composeFile); err == nil && !fi.Mode().IsRegular() {
		return errx.BadRequest("cannot write to non-regular file or symlink")
	}

	f, err := os.OpenFile(composeFile, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return fmt.Errorf("open compose file: %w", err)
	}
	defer f.Close()
	if _, err := f.Write([]byte(content)); err != nil {
		return fmt.Errorf("write compose file: %w", err)
	}
	return nil
}

func (s *Service) findComposeFile(projectDir string) string {
	cleanDir, err := isSafeProjectPath(projectDir)
	if err != nil {
		return ""
	}
	candidates := []string{
		"docker-compose.yml",
		"docker-compose.yaml",
		"compose.yml",
		"compose.yaml",
	}

	for _, name := range candidates {
		baseName := filepath.Base(filepath.Clean(name))
		path := filepath.Join(cleanDir, baseName)
		rel, err := filepath.Rel(cleanDir, path)
		if err != nil || strings.HasPrefix(rel, "..") || rel == ".." {
			continue
		}
		fi, err := os.Lstat(path)
		if err == nil && fi.Mode().IsRegular() {
			return path
		}
	}
	return ""
}

// --- Volume operations ---

// ListVolumes returns all volumes for the given engine.
func (s *Service) ListVolumes(ctx context.Context, engine Engine) ([]Volume, error) {
	if err := s.checkEngine(ctx, engine); err != nil {
		return nil, err
	}

	resp, err := infracontainer.DefaultClient().VolumeList(ctx, infracontainer.Engine(engine))
	if err != nil {
		return nil, errx.Internal("%s volume ls failed: %w", engine, err)
	}

	volumes := make([]Volume, 0, len(resp.Volumes))
	for _, v := range resp.Volumes {
		volumes = append(volumes, Volume{
			Name:       v.Name,
			Driver:     v.Driver,
			Mountpoint: v.Mountpoint,
			CreatedAt:  v.CreatedAt,
		})
	}
	return volumes, nil
}

// CreateVolume creates a new volume.
func (s *Service) CreateVolume(ctx context.Context, engine Engine, name, driver string, labels map[string]string) error {
	_, err := infracontainer.DefaultClient().VolumeCreate(ctx, infracontainer.Engine(engine), infracontainer.VolumeCreateRequest{
		Name:   name,
		Driver: driver,
		Labels: labels,
	})
	if err != nil {
		return errx.Internal("%s volume create failed: %w", engine, err)
	}
	return nil
}

// RemoveVolume removes a volume.
func (s *Service) RemoveVolume(ctx context.Context, engine Engine, name string, force bool) error {
	if err := infracontainer.DefaultClient().VolumeRemove(ctx, infracontainer.Engine(engine), name, force); err != nil {
		return errx.Internal("%s volume rm failed: %w", engine, err)
	}
	return nil
}

// PruneVolumes removes unused volumes.
func (s *Service) PruneVolumes(ctx context.Context, engine Engine) (*infracontainer.VolumesPruneReport, error) {
	report, err := infracontainer.DefaultClient().VolumesPrune(ctx, infracontainer.Engine(engine))
	if err != nil {
		return nil, errx.Internal("%s volume prune failed: %w", engine, err)
	}
	return &report, nil
}

// --- Network operations ---

// ListNetworks returns all networks for the given engine.
func (s *Service) ListNetworks(ctx context.Context, engine Engine) ([]Network, error) {
	if err := s.checkEngine(ctx, engine); err != nil {
		return nil, err
	}

	summaries, err := infracontainer.DefaultClient().NetworkList(ctx, infracontainer.Engine(engine))
	if err != nil {
		return nil, errx.Internal("%s network ls failed: %w", engine, err)
	}

	networks := make([]Network, 0, len(summaries))
	for _, net := range summaries {
		subnet := ""
		gateway := ""
		if len(net.IPAM.Config) > 0 {
			subnet = net.IPAM.Config[0].Subnet
			gateway = net.IPAM.Config[0].Gateway
		}
		networks = append(networks, Network{
			ID:      net.ID,
			Name:    net.Name,
			Driver:  net.Driver,
			Scope:   net.Scope,
			Subnet:  subnet,
			Gateway: gateway,
		})
	}
	return networks, nil
}

// CreateNetwork creates a new network.
func (s *Service) CreateNetwork(ctx context.Context, engine Engine, name, driver string) error {
	_, err := infracontainer.DefaultClient().NetworkCreate(ctx, infracontainer.Engine(engine), infracontainer.NetworkCreateRequest{
		Name:   name,
		Driver: driver,
	})
	if err != nil {
		return errx.Internal("%s network create failed: %w", engine, err)
	}
	return nil
}

// RemoveNetwork removes a network.
func (s *Service) RemoveNetwork(ctx context.Context, engine Engine, id string) error {
	if err := infracontainer.DefaultClient().NetworkRemove(ctx, infracontainer.Engine(engine), id); err != nil {
		return errx.Internal("%s network rm failed: %w", engine, err)
	}
	return nil
}

// PruneNetworks removes unused networks.
func (s *Service) PruneNetworks(ctx context.Context, engine Engine) (*infracontainer.NetworksPruneReport, error) {
	report, err := infracontainer.DefaultClient().NetworksPrune(ctx, infracontainer.Engine(engine))
	if err != nil {
		return nil, errx.Internal("%s network prune failed: %w", engine, err)
	}
	return &report, nil
}

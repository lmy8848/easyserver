package database

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"sort"
	"strconv"
	"strings"
	"time"

	infracontainer "easyserver/internal/infra/container"
)

// ContainerSpec describes a database container without exposing CLI details to callers.
type ContainerSpec struct {
	ContainerEngine string
	Name            string
	Image           string
	Volume          string
	DataDir         string
	ConfigVolume    string
	ConfigDir       string
	BindAddress     string
	HostPort        int
	ContainerPort   int
	Environment     map[string]string
	Labels          map[string]string
	HealthCommand   string
	Command         []string
}

// ContainerStatus is the externally observable state of a managed database container.
type ContainerStatus struct {
	State  string
	Health string
}

// DatabaseRuntime is the seam between database lifecycle logic and Docker/Podman.
type DatabaseRuntime interface {
	Create(ctx context.Context, spec ContainerSpec) error
	Start(ctx context.Context, runtime, name string) error
	Stop(ctx context.Context, runtime, name string) error
	Restart(ctx context.Context, runtime, name string) error
	Remove(ctx context.Context, runtime, name string) error
	Status(ctx context.Context, runtime, name string) (ContainerStatus, error)
	Logs(ctx context.Context, runtime, name string, lines int) (string, error)
	Exec(ctx context.Context, runtime, name string, args ...string) (string, error)
	Exists(ctx context.Context, runtime, name string) (bool, error)
}

// SocketContainerRuntime implements DatabaseRuntime using Docker/Podman Unix Socket REST API.
type SocketContainerRuntime struct {
	client     infracontainer.EngineClient
	outputHook func(string)
}

// NewSocketContainerRuntime creates a new SocketContainerRuntime.
func NewSocketContainerRuntime(client ...infracontainer.EngineClient) *SocketContainerRuntime {
	var c infracontainer.EngineClient
	if len(client) > 0 && client[0] != nil {
		c = client[0]
	}
	return &SocketContainerRuntime{client: c}
}

func (r *SocketContainerRuntime) getClient() infracontainer.EngineClient {
	if r.client != nil {
		return r.client
	}
	return infracontainer.DefaultClient()
}

// SetOutputHook installs a per-line callback for image pull and create output.
func (r *SocketContainerRuntime) SetOutputHook(fn func(string)) {
	r.outputHook = fn
}

func toEngine(runtime string) infracontainer.Engine {
	if strings.ToLower(strings.TrimSpace(runtime)) == "podman" {
		return infracontainer.EnginePodman
	}
	return infracontainer.EngineDocker
}

func isValidImageName(image string) bool {
	if image == "" || len(image) > 255 {
		return false
	}
	for _, r := range image {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') ||
			r == '.' || r == '_' || r == '-' || r == '/' || r == ':' || r == '@' {
			continue
		}
		return false
	}
	return true
}

func (r *SocketContainerRuntime) Create(ctx context.Context, spec ContainerSpec) error {
	if spec.Name == "" || spec.Image == "" || spec.Volume == "" || spec.DataDir == "" {
		return errors.New("container name, image, volume and data directory are required")
	}
	if !isValidImageName(spec.Image) {
		return errors.New("invalid container image name")
	}
	if spec.HostPort < 1 || spec.HostPort > 65535 || spec.ContainerPort < 1 || spec.ContainerPort > 65535 {
		return errors.New("invalid container port")
	}
	labels := make(map[string]string, len(spec.Labels)+2)
	maps.Copy(labels, spec.Labels)
	labels["com.easyserver.managed"] = "true"
	labels["com.easyserver.kind"] = "database"
	spec.Labels = labels

	eng := toEngine(spec.ContainerEngine)

	// Always pull the image before container creation and stream progress
	rc, err := r.getClient().ImagePull(ctx, eng, spec.Image, "")
	if err != nil {
		return fmt.Errorf("pull image %s: %w", spec.Image, err)
	}
	if rc != nil {
		defer rc.Close()
		dec := json.NewDecoder(rc)
		for {
			var ev struct {
				Status   string `json:"status"`
				Progress string `json:"progress"`
				ID       string `json:"id"`
				Error    string `json:"error"`
			}
			if err := dec.Decode(&ev); err != nil {
				if errors.Is(err, io.EOF) {
					break
				}
				return fmt.Errorf("decode pull image response: %w", err)
			}
			if ev.Error != "" {
				if r.outputHook != nil {
					r.outputHook("Error: " + ev.Error)
				}
				return fmt.Errorf("pull image %s: %s", spec.Image, ev.Error)
			}
			if r.outputHook != nil {
				line := ev.Status
				if ev.ID != "" {
					line = ev.ID + ": " + line
				}
				if ev.Progress != "" {
					line += " " + ev.Progress
				}
				if line != "" {
					r.outputHook(line)
				}
			}
		}
	}

	var env []string
	for _, k := range sortedKeys(spec.Environment) {
		env = append(env, k+"="+spec.Environment[k])
	}

	portKey := fmt.Sprintf("%d/tcp", spec.ContainerPort)
	exposedPorts := map[string]struct{}{
		portKey: {},
	}
	portBindings := map[string][]infracontainer.PortBinding{
		portKey: {
			{
				HostIP:   spec.BindAddress,
				HostPort: strconv.Itoa(spec.HostPort),
			},
		},
	}

	binds := []string{
		spec.Volume + ":" + spec.DataDir,
	}
	if spec.ConfigVolume != "" && spec.ConfigDir != "" {
		binds = append(binds, spec.ConfigVolume+":"+spec.ConfigDir)
	}

	var healthcheck *infracontainer.HealthcheckConfig
	if spec.HealthCommand != "" {
		healthcheck = &infracontainer.HealthcheckConfig{
			Test:     []string{"CMD-SHELL", spec.HealthCommand},
			Interval: int64(5 * time.Second),
			Timeout:  int64(3 * time.Second),
			Retries:  20,
		}
	}

	req := infracontainer.ContainerCreateRequest{
		ContainerConfig: infracontainer.ContainerConfig{
			Image:        spec.Image,
			Cmd:          spec.Command,
			Env:          env,
			Labels:       labels,
			ExposedPorts: exposedPorts,
			Healthcheck:  healthcheck,
		},
		HostConfig: &infracontainer.HostConfig{
			Binds:        binds,
			PortBindings: portBindings,
			RestartPolicy: &infracontainer.RestartPolicy{
				Name: "unless-stopped",
			},
		},
	}

	if _, err := r.getClient().ContainerCreate(ctx, eng, spec.Name, req); err != nil {
		return fmt.Errorf("create database container: %w", err)
	}
	return nil
}

func (r *SocketContainerRuntime) Start(ctx context.Context, runtime, name string) error {
	return r.getClient().ContainerStart(ctx, toEngine(runtime), name)
}

func (r *SocketContainerRuntime) Stop(ctx context.Context, runtime, name string) error {
	return r.getClient().ContainerStop(ctx, toEngine(runtime), name, 10)
}

func (r *SocketContainerRuntime) Restart(ctx context.Context, runtime, name string) error {
	return r.getClient().ContainerRestart(ctx, toEngine(runtime), name, 10)
}

func (r *SocketContainerRuntime) Remove(ctx context.Context, runtime, name string) error {
	err := r.getClient().ContainerRemove(ctx, toEngine(runtime), name, true)
	if err != nil && notFound(err, "no such", "not found", "404") {
		return nil
	}
	return err
}

func notFound(err error, markers ...string) bool {
	if err == nil {
		return false
	}
	s := strings.ToLower(err.Error())
	for _, m := range markers {
		if strings.Contains(s, m) {
			return true
		}
	}
	return false
}

func (r *SocketContainerRuntime) Status(ctx context.Context, runtime, name string) (ContainerStatus, error) {
	insp, err := r.getClient().ContainerInspect(ctx, toEngine(runtime), name)
	if err != nil {
		return ContainerStatus{}, err
	}
	status := ContainerStatus{State: insp.State.Status}
	if insp.State.Health != nil {
		status.Health = insp.State.Health.Status
	}
	return status, nil
}

func (r *SocketContainerRuntime) Logs(ctx context.Context, runtime, name string, lines int) (string, error) {
	if lines <= 0 {
		lines = 200
	}
	if lines > 5000 {
		lines = 5000
	}
	rc, err := r.getClient().ContainerLogs(ctx, toEngine(runtime), name, lines, true, true)
	if err != nil {
		return "", err
	}
	defer rc.Close()

	rawBytes, err := io.ReadAll(rc)
	if err != nil {
		return "", err
	}

	var stdout, stderr bytes.Buffer
	if err := infracontainer.DemuxLogs(bytes.NewReader(rawBytes), &stdout, &stderr); err == nil && (stdout.Len() > 0 || stderr.Len() > 0) {
		return stdout.String() + stderr.String(), nil
	}
	return string(rawBytes), nil
}

func (r *SocketContainerRuntime) Exec(ctx context.Context, runtime, name string, args ...string) (string, error) {
	if name == "" || len(args) == 0 {
		return "", errors.New("container name and command are required")
	}

	var env []string
	for len(args) >= 2 && args[0] == "-e" {
		env = append(env, args[1])
		args = args[2:]
	}

	eng := toEngine(runtime)
	createResp, err := r.getClient().ContainerExecCreate(ctx, eng, name, infracontainer.ExecCreateRequest{
		AttachStdout: true,
		AttachStderr: true,
		Env:          env,
		Cmd:          args,
	})
	if err != nil {
		return "", fmt.Errorf("create exec: %w", err)
	}

	rc, err := r.getClient().ContainerExecStart(ctx, eng, createResp.ID, infracontainer.ExecStartRequest{})
	if err != nil {
		return "", fmt.Errorf("start exec: %w", err)
	}
	defer rc.Close()

	rawBytes, err := io.ReadAll(rc)
	if err != nil {
		return "", fmt.Errorf("read exec output: %w", err)
	}

	var stdout, stderr bytes.Buffer
	output := string(rawBytes)
	if err := infracontainer.DemuxLogs(bytes.NewReader(rawBytes), &stdout, &stderr); err == nil && (stdout.Len() > 0 || stderr.Len() > 0) {
		output = stdout.String()
	}

	insp, err := r.getClient().ContainerExecInspect(ctx, eng, createResp.ID)
	if err != nil {
		return output, fmt.Errorf("inspect exec: %w", err)
	}
	if insp.ExitCode != 0 {
		detail := strings.TrimSpace(output)
		if stderr.Len() > 0 {
			detail = strings.TrimSpace(detail + "\n" + stderr.String())
		}
		return output, fmt.Errorf("exec exited with code %d: %s", insp.ExitCode, detail)
	}

	return output, nil
}

func (r *SocketContainerRuntime) Exists(ctx context.Context, runtime, name string) (bool, error) {
	if name == "" {
		return false, errors.New("container name is required")
	}
	_, err := r.getClient().ContainerInspect(ctx, toEngine(runtime), name)
	if err != nil {
		if notFound(err, "no such", "not found", "404") {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func sortedKeys(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func waitForHealthy(ctx context.Context, runtime DatabaseRuntime, runtimeName, container string, timeout time.Duration) (ContainerStatus, error) {
	// timeout > 0 enforces a deadline; timeout <= 0 waits indefinitely (the
	// install path relies on container exit or user cancel to terminate).
	var deadline time.Time
	if timeout > 0 {
		deadline = time.Now().Add(timeout)
	}
	for {
		status, err := runtime.Status(ctx, runtimeName, container)
		if err != nil {
			return status, err
		}
		if status.State == "running" && status.Health == "healthy" {
			return status, nil
		}
		if status.State == "exited" || status.State == "dead" {
			return status, errors.New("database container stopped before becoming healthy")
		}
		if !deadline.IsZero() && time.Now().After(deadline) {
			return status, fmt.Errorf("database container did not become healthy within %s", timeout)
		}
		select {
		case <-ctx.Done():
			return status, ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
}

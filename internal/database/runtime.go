package database

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"easyserver/internal/infra/executor"
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
	CopyFrom(ctx context.Context, runtime, name, source, destination string) error
	CopyTo(ctx context.Context, runtime, name, source, destination string) error
	RemoveVolume(ctx context.Context, runtime, volume string) error
}

// CLIContainerRuntime implements DatabaseRuntime with Docker or rootful Podman.
type CLIContainerRuntime struct {
	executor executor.CommandExecutor
	// lastSpec records the most recent Create spec, exposed for tests to assert
	// the structured contract (label, volume, port binding) without inspecting
	// command concatenation.
	lastSpec ContainerSpec
	// outputHook, when set, receives every non-empty trimmed command output line
	// (e.g. image pull progress). The installer wires it to the install log.
	outputHook func(string)
}

func NewCLIContainerRuntime(exec executor.CommandExecutor) *CLIContainerRuntime {
	return &CLIContainerRuntime{executor: exec}
}

// SetOutputHook installs a per-line callback for all command output. Used by
// the installer to stream pull/create/start output into the install log.
func (r *CLIContainerRuntime) SetOutputHook(fn func(string)) {
	r.outputHook = fn
}

func containerBinary(runtime string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(runtime)) {
	case "docker", "":
		return "docker", nil
	case "podman":
		return "podman", nil
	default:
		return "", fmt.Errorf("unsupported container runtime %q", runtime)
	}
}

// command runs a lifecycle command with stdout+stderr combined — install/log
// paths want the whole stream (pull progress, container logs, errors).
func (r *CLIContainerRuntime) command(ctx context.Context, runtime string, args ...string) (string, error) {
	return r.run(ctx, true, runtime, args...)
}

// execCommand runs a command inside the container with stdout and stderr
// separated — query output (mysql/psql listings, SQL results) is parsed, so
// stderr warnings (e.g. mysql's "password on the command line" notice) must
// never be merged into it. stderr only surfaces in the error when the command
// itself fails.
func (r *CLIContainerRuntime) execCommand(ctx context.Context, runtime, name string, args ...string) (string, error) {
	return r.run(ctx, false, runtime, append([]string{"exec", name}, args...)...)
}

func (r *CLIContainerRuntime) run(ctx context.Context, combine bool, runtime string, args ...string) (string, error) {
	bin, err := containerBinary(runtime)
	if err != nil {
		return "", err
	}
	var out, stderr string
	var code int
	var runErr error
	if combine {
		out, code, runErr = r.executor.RunCombined(ctx, bin, args...)
	} else {
		out, stderr, code, runErr = r.executor.Run(ctx, bin, args...)
	}
	if r.outputHook != nil {
		for _, line := range strings.Split(out, "\n") {
			if line = strings.TrimSpace(line); line != "" {
				r.outputHook(line)
			}
		}
	}
	if runErr != nil || code != 0 {
		if runErr != nil {
			return out, fmt.Errorf("%s: %w", bin, runErr)
		}
		// For separated runs, append stderr to the detail so the real error
		// isn't hidden by an empty stdout.
		detail := strings.TrimSpace(out)
		if !combine {
			if s := strings.TrimSpace(stderr); s != "" {
				detail = strings.TrimSpace(detail + "\n" + s)
			}
		}
		return out, fmt.Errorf("%s exited with code %d: %s", bin, code, detail)
	}
	return out, nil
}

func (r *CLIContainerRuntime) Create(ctx context.Context, spec ContainerSpec) error {
	if spec.Name == "" || spec.Image == "" || spec.Volume == "" || spec.DataDir == "" {
		return fmt.Errorf("container name, image, volume and data directory are required")
	}
	if spec.HostPort < 1 || spec.HostPort > 65535 || spec.ContainerPort < 1 || spec.ContainerPort > 65535 {
		return fmt.Errorf("invalid container port")
	}
	if spec.Labels == nil {
		spec.Labels = map[string]string{}
	}
	spec.Labels["com.easyserver.managed"] = "true"
	r.lastSpec = spec

	args := []string{"create", "--name", spec.Name}
	args = append(args, "--label", "com.easyserver.managed=true", "--label", "com.easyserver.kind=database")
	for _, key := range sortedKeys(spec.Labels) {
		args = append(args, "--label", key+"="+spec.Labels[key])
	}
	args = append(args, "--publish", fmt.Sprintf("%s:%d:%d", spec.BindAddress, spec.HostPort, spec.ContainerPort))
	// Named volumes are auto-created by the engine on first mount — no explicit
	// `volume create` needed (and none left orphaned on create failure).
	args = append(args, "--volume", spec.Volume+":"+spec.DataDir)
	if spec.ConfigVolume != "" && spec.ConfigDir != "" {
		args = append(args, "--volume", spec.ConfigVolume+":"+spec.ConfigDir)
	}
	for _, key := range sortedKeys(spec.Environment) {
		args = append(args, "--env", key+"="+spec.Environment[key])
	}
	if spec.HealthCommand != "" {
		args = append(args, "--health-cmd", spec.HealthCommand, "--health-interval", "5s", "--health-timeout", "3s", "--health-retries", "20")
	}
	args = append(args, spec.Image)
	args = append(args, spec.Command...)
	if _, err := r.command(ctx, spec.ContainerEngine, args...); err != nil {
		return fmt.Errorf("create database container: %w", err)
	}
	return nil
}

func (r *CLIContainerRuntime) Start(ctx context.Context, runtime, name string) error {
	_, err := r.command(ctx, runtime, "start", name)
	return err
}

func (r *CLIContainerRuntime) Stop(ctx context.Context, runtime, name string) error {
	_, err := r.command(ctx, runtime, "stop", name)
	return err
}

func (r *CLIContainerRuntime) Restart(ctx context.Context, runtime, name string) error {
	_, err := r.command(ctx, runtime, "restart", name)
	return err
}

func (r *CLIContainerRuntime) Remove(ctx context.Context, runtime, name string) error {
	_, err := r.command(ctx, runtime, "rm", "--force", name)
	return err
}

func (r *CLIContainerRuntime) Status(ctx context.Context, runtime, name string) (ContainerStatus, error) {
	out, err := r.command(ctx, runtime, "inspect", "--format", "{{.State.Status}}|{{if .State.Health}}{{.State.Health.Status}}{{end}}", name)
	if err != nil {
		return ContainerStatus{}, err
	}
	parts := strings.SplitN(strings.TrimSpace(out), "|", 2)
	status := ContainerStatus{State: parts[0]}
	if len(parts) == 2 {
		status.Health = parts[1]
	}
	return status, nil
}

func (r *CLIContainerRuntime) Logs(ctx context.Context, runtime, name string, lines int) (string, error) {
	if lines <= 0 {
		lines = 200
	}
	if lines > 5000 {
		lines = 5000
	}
	return r.command(ctx, runtime, "logs", "--tail", fmt.Sprintf("%d", lines), name)
}

func (r *CLIContainerRuntime) Exec(ctx context.Context, runtime, name string, args ...string) (string, error) {
	if name == "" || len(args) == 0 {
		return "", fmt.Errorf("container name and command are required")
	}
	return r.execCommand(ctx, runtime, name, args...)
}

func (r *CLIContainerRuntime) CopyFrom(ctx context.Context, runtime, name, source, destination string) error {
	_, err := r.command(ctx, runtime, "cp", name+":"+source, destination)
	return err
}

func (r *CLIContainerRuntime) CopyTo(ctx context.Context, runtime, name, source, destination string) error {
	_, err := r.command(ctx, runtime, "cp", source, name+":"+destination)
	return err
}

func (r *CLIContainerRuntime) RemoveVolume(ctx context.Context, runtime, volume string) error {
	if volume == "" {
		return fmt.Errorf("volume name is required")
	}
	_, err := r.command(ctx, runtime, "volume", "rm", volume)
	return err
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
	if timeout <= 0 {
		timeout = time.Minute
	}
	deadline := time.Now().Add(timeout)
	for {
		status, err := runtime.Status(ctx, runtimeName, container)
		if err != nil {
			return status, err
		}
		if status.State == "running" && status.Health == "healthy" {
			return status, nil
		}
		if status.State == "exited" || status.State == "dead" {
			return status, fmt.Errorf("database container stopped before becoming healthy")
		}
		if time.Now().After(deadline) {
			return status, fmt.Errorf("database container did not become healthy within %s", timeout)
		}
		select {
		case <-ctx.Done():
			return status, ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
}

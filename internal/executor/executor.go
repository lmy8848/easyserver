package executor

import (
	"context"
	"io"
	"os"
	"os/exec"
	"syscall"
	"time"
)

// CommandExecutor defines the interface for executing system commands.
type CommandExecutor interface {
	Run(ctx context.Context, name string, args ...string) (stdout, stderr string, exitCode int, err error)
	RunWithTimeout(ctx context.Context, timeout time.Duration, name string, args ...string) (stdout, stderr string, exitCode int, err error)
	RunCombined(ctx context.Context, name string, args ...string) (output string, exitCode int, err error)
	RunWithOptions(ctx context.Context, opts CommandOptions, name string, args ...string) (output string, exitCode int, err error)
	Start(ctx context.Context, opts StartOptions, name string, args ...string) (Process, error)
	Command(ctx context.Context, opts StartOptions, name string, args ...string) *exec.Cmd
}

type StartOptions struct {
	WorkDir string
	Env     []string
	Setpgid bool
}

type Process interface {
	Pid() int
	StdoutPipe() (io.ReadCloser, error)
	StderrPipe() (io.ReadCloser, error)
	Wait() error
	Kill() error
	Signal(sig syscall.Signal) error
	Cmd() *exec.Cmd
}

type CommandOptions struct {
	WorkDir string
	Env     []string
	Timeout time.Duration
}

type OSExecutor struct{}

func NewOSExecutor() CommandExecutor { return &OSExecutor{} }

func (e *OSExecutor) Run(ctx context.Context, name string, args ...string) (string, string, int, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	cmd := exec.CommandContext(ctx, name, args...)
	stdoutBytes, err := cmd.Output()
	stdout := string(stdoutBytes)
	stderr := ""
	exitCode := 0

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			stderr = string(exitErr.Stderr)
			exitCode = exitErr.ExitCode()
		} else {
			return stdout, stderr, -1, err
		}
	}
	return stdout, stderr, exitCode, nil
}

func (e *OSExecutor) RunWithTimeout(ctx context.Context, timeout time.Duration, name string, args ...string) (string, string, int, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return e.Run(ctx, name, args...)
}

func (e *OSExecutor) RunCombined(ctx context.Context, name string, args ...string) (string, int, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	cmd := exec.CommandContext(ctx, name, args...)
	output, err := cmd.CombinedOutput()
	result := string(output)
	exitCode := 0

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			return result, -1, err
		}
	}
	return result, exitCode, nil
}

func (e *OSExecutor) RunWithOptions(ctx context.Context, opts CommandOptions, name string, args ...string) (string, int, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if opts.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, opts.Timeout)
		defer cancel()
	}

	cmd := exec.CommandContext(ctx, name, args...)
	applyCommandOptions(cmd, opts.WorkDir, opts.Env)
	output, err := cmd.CombinedOutput()
	result := string(output)
	exitCode := 0

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			return result, -1, err
		}
	}
	return result, exitCode, nil
}

type OSProcess struct{ cmd *exec.Cmd }

func (p *OSProcess) Pid() int                           { return p.cmd.Process.Pid }
func (p *OSProcess) StdoutPipe() (io.ReadCloser, error) { return p.cmd.StdoutPipe() }
func (p *OSProcess) StderrPipe() (io.ReadCloser, error) { return p.cmd.StderrPipe() }
func (p *OSProcess) Wait() error                        { return p.cmd.Wait() }
func (p *OSProcess) Kill() error                        { return p.cmd.Process.Kill() }
func (p *OSProcess) Signal(sig syscall.Signal) error    { return p.cmd.Process.Signal(sig) }
func (p *OSProcess) Cmd() *exec.Cmd                     { return p.cmd }

func (e *OSExecutor) Command(ctx context.Context, opts StartOptions, name string, args ...string) *exec.Cmd {
	if ctx == nil {
		ctx = context.Background()
	}
	cmd := exec.CommandContext(ctx, name, args...)
	applyCommandOptions(cmd, opts.WorkDir, opts.Env)
	if opts.Setpgid {
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	}
	return cmd
}

func (e *OSExecutor) Start(ctx context.Context, opts StartOptions, name string, args ...string) (Process, error) {
	cmd := e.Command(ctx, opts, name, args...)

	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return &OSProcess{cmd: cmd}, nil
}

func applyCommandOptions(cmd *exec.Cmd, workDir string, env []string) {
	if workDir != "" {
		cmd.Dir = workDir
	}
	if len(env) > 0 {
		cmd.Env = append(os.Environ(), env...)
	}
}

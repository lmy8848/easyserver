package executor

import (
	"context"
	"io"
	"os"
	"os/exec"
	"syscall"
	"time"
)

// CommandExecutor defines the interface for executing system commands
type CommandExecutor interface {
	// Run executes a command and returns stdout, stderr, exit code, and error
	Run(ctx context.Context, name string, args ...string) (stdout, stderr string, exitCode int, err error)

	// RunWithTimeout executes a command with a timeout
	RunWithTimeout(ctx context.Context, timeout time.Duration, name string, args ...string) (stdout, stderr string, exitCode int, err error)

	// RunCombined executes a command and returns combined output
	RunCombined(ctx context.Context, name string, args ...string) (output string, exitCode int, err error)

	// RunWithOptions executes a command with custom options (workdir, env)
	RunWithOptions(ctx context.Context, opts CommandOptions, name string, args ...string) (output string, exitCode int, err error)

	// Start starts a long-running process and returns a Process handle
	Start(ctx context.Context, opts StartOptions, name string, args ...string) (Process, error)
}

// StartOptions holds options for starting a process
type StartOptions struct {
	WorkDir string   // Working directory (empty = inherit)
	Env     []string // Additional environment variables (extend, not replace)
	Setpgid bool     // Create new process group
}

// Process represents a running process
type Process interface {
	// Pid returns the process ID
	Pid() int

	// StdoutPipe returns a reader for stdout
	StdoutPipe() (io.ReadCloser, error)

	// StderrPipe returns a reader for stderr
	StderrPipe() (io.ReadCloser, error)

	// Wait waits for the process to exit
	Wait() error

	// Kill kills the process
	Kill() error

	// Signal sends a signal to the process
	Signal(sig syscall.Signal) error

	// Cmd returns the underlying exec.Cmd (for PTY usage)
	Cmd() *exec.Cmd
}

// CommandOptions holds optional parameters for command execution
type CommandOptions struct {
	WorkDir string   // Working directory (empty = inherit)
	Env     []string // Additional environment variables (extend, not replace)
	Timeout time.Duration // Timeout (0 = no timeout)
}

// OSExecutor is the real implementation that executes OS commands
type OSExecutor struct{}

// NewOSExecutor creates a new OSExecutor
func NewOSExecutor() CommandExecutor {
	return &OSExecutor{}
}

// Run executes a command and returns stdout, stderr, exit code, and error
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

// RunWithTimeout executes a command with a timeout
func (e *OSExecutor) RunWithTimeout(ctx context.Context, timeout time.Duration, name string, args ...string) (string, string, int, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	return e.Run(ctx, name, args...)
}

// RunCombined executes a command and returns combined output
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

// RunWithOptions executes a command with custom options (workdir, env)
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

	if opts.WorkDir != "" {
		cmd.Dir = opts.WorkDir
	}

	if len(opts.Env) > 0 {
		cmd.Env = append(os.Environ(), opts.Env...)
	}

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

// OSProcess wraps exec.Cmd to implement the Process interface
type OSProcess struct {
	cmd *exec.Cmd
}

// Pid returns the process ID
func (p *OSProcess) Pid() int {
	return p.cmd.Process.Pid
}

// StdoutPipe returns a reader for stdout
func (p *OSProcess) StdoutPipe() (io.ReadCloser, error) {
	return p.cmd.StdoutPipe()
}

// StderrPipe returns a reader for stderr
func (p *OSProcess) StderrPipe() (io.ReadCloser, error) {
	return p.cmd.StderrPipe()
}

// Wait waits for the process to exit
func (p *OSProcess) Wait() error {
	return p.cmd.Wait()
}

// Kill kills the process
func (p *OSProcess) Kill() error {
	return p.cmd.Process.Kill()
}

// Signal sends a signal to the process
func (p *OSProcess) Signal(sig syscall.Signal) error {
	return p.cmd.Process.Signal(sig)
}

// Cmd returns the underlying exec.Cmd (for PTY usage)
func (p *OSProcess) Cmd() *exec.Cmd {
	return p.cmd
}

// Start starts a long-running process and returns a Process handle
func (e *OSExecutor) Start(ctx context.Context, opts StartOptions, name string, args ...string) (Process, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	cmd := exec.CommandContext(ctx, name, args...)

	if opts.WorkDir != "" {
		cmd.Dir = opts.WorkDir
	}

	if len(opts.Env) > 0 {
		cmd.Env = append(os.Environ(), opts.Env...)
	}

	if opts.Setpgid {
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	}

	if err := cmd.Start(); err != nil {
		return nil, err
	}

	return &OSProcess{cmd: cmd}, nil
}

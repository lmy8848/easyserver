package executor

import (
	"context"
	"os/exec"
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

//go:build !windows
// +build !windows

package executor

import (
	"os/exec"
	"syscall"
)

// applyPlatformStartOptions applies Unix-specific process attributes.
// Honors opts.Setpgid to put the child into its own process group, which
// lets the caller signal the whole group on shutdown.
func applyPlatformStartOptions(cmd *exec.Cmd, opts StartOptions) {
	if opts.Setpgid {
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	}
}

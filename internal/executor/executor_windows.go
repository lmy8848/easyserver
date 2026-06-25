//go:build windows
// +build windows

package executor

import (
	"os/exec"
	"syscall"
)

// applyPlatformStartOptions applies Windows-specific process attributes.
// On Windows, Setpgid is not applicable; we use CREATE_NEW_PROCESS_GROUP
// via the Windows-specific SysProcAttr when needed.
func applyPlatformStartOptions(cmd *exec.Cmd, opts StartOptions) {
	if opts.Setpgid {
		cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP}
	}
}

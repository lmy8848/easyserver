//go:build windows
// +build windows

package executor

import "os/exec"

// applyPlatformStartOptions is a no-op on Windows.
// opts.Setpgid has no equivalent on Windows (process groups work differently;
// see CREATE_NEW_PROCESS_GROUP if needed in the future).
func applyPlatformStartOptions(cmd *exec.Cmd, opts StartOptions) {
	_ = cmd
	_ = opts
}

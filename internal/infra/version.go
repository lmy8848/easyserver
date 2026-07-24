// Package version holds build-time version information injected via ldflags.
package infra

import (
	"fmt"
	"runtime"
)

// Version is the build version, set via ldflags at build time.
// Example: go build -ldflags "-X easyserver/internal/infra/version.Version=v0.1.2-83-g52565d5+0xd3a"
var Version = "dev"

// BuildID is a short unique identifier for each build/deploy, set via ldflags.
// Example: 0xd3a. Empty if not set (dev builds).
var BuildID = ""

// GoVersion is the Go toolchain version used to build the binary.
var GoVersion = runtime.Version()

// DisplayVersion returns the version string with optional build ID suffix.
// Example: "v0.1.2-83-g52565d5+0xd3a" or "v0.1.2-83-g52565d5" or "dev".
func DisplayVersion() string {
	if BuildID != "" {
		return Version + "+" + BuildID
	}
	return Version
}

// GetFullVersionString returns the formatted version information.
func GetFullVersionString() string {
	return fmt.Sprintf("EasyServer version %s (Go: %s, Platform: %s/%s)", DisplayVersion(), GoVersion, runtime.GOOS, runtime.GOARCH)
}

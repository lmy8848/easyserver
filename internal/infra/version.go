// Package version holds build-time version information injected via ldflags.
package infra

import (
	"fmt"
	"runtime"
)

// Version is the build version, set via ldflags at build time.
// Example: go build -ldflags "-X easyserver/internal/infra/version.Version=1.0.0"
var Version = "dev"

// GoVersion is the Go toolchain version used to build the binary.
var GoVersion = runtime.Version()

// GetFullVersionString returns the formatted version information.
func GetFullVersionString() string {
	return fmt.Sprintf("EasyServer version %s (Go: %s, Platform: %s/%s)", Version, GoVersion, runtime.GOOS, runtime.GOARCH)
}

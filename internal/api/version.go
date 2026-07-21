package api

import (
	"fmt"
	"runtime"
)

// Version information set via ldflags at build time.
// Example: go build -ldflags "-X easyserver/internal/api.Version=1.0.0"
var (
	Version   = "dev"
	GoVersion = runtime.Version()
)

// GetFullVersionString returns the formatted version information
func GetFullVersionString() string {
	return fmt.Sprintf("EasyServer version %s (Go: %s, Platform: %s/%s)", Version, GoVersion, runtime.GOOS, runtime.GOARCH)
}

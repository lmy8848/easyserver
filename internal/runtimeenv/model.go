package runtimeenv

import "time"

// RuntimeEnvironment represents a runtime environment (Java, Node.js, PHP, Python, Go)
type RuntimeEnvironment struct {
	ID           int64     `json:"id"`
	Name         string    `json:"name"`          // java, node, php, python, go
	Version      string    `json:"version"`       // 17, 18.17.0, 8.2, 3.11, 1.21
	Path         string    `json:"path"`          // Installation path
	IsDefault    bool      `json:"is_default"`    // Is this the default version
	Status       string    `json:"status"`        // installed, installing, failed
	Progress     int       `json:"progress"`      // Installation progress 0-100
	ProgressStep string    `json:"progress_step"` // Current step: pending, downloading, compiling, configuring, done
	Logs         string    `json:"logs"`          // Installation logs
	ErrorMessage string    `json:"error_message"` // Error message if failed
	InstalledAt  time.Time `json:"installed_at"`
}

// RuntimeInstallRequest represents a request to install a runtime environment
type RuntimeInstallRequest struct {
	Name    string `json:"name" binding:"required"`    // java, node, php, python, go
	Version string `json:"version" binding:"required"` // 17, 18.17.0, 8.2, 3.11, 1.21
}

// RuntimeUninstallRequest represents a request to uninstall a runtime environment
type RuntimeUninstallRequest struct {
	Name    string `json:"name" binding:"required"`    // java, node, php, python, go
	Version string `json:"version" binding:"required"` // 17, 18.17.0, 8.2, 3.11, 1.21
}

// RuntimeSetDefaultRequest represents a request to set default version
type RuntimeSetDefaultRequest struct {
	Name    string `json:"name" binding:"required"`    // java, node, php, python, go
	Version string `json:"version" binding:"required"` // 17, 18.17.0, 8.2, 3.11, 1.21
}

// RuntimeDetectResult represents detected runtime environments on the system

// RuntimeMirror represents a mirror source configuration
type RuntimeMirror struct {
	ID        int64     `json:"id"`
	Lang      string    `json:"lang"`
	EnvKey    string    `json:"env_key"`
	EnvValue  string    `json:"env_value"`
	Enabled   int       `json:"enabled"` // 0 or 1
	Source    string    `json:"source"`  // 'seed' or 'user'
	UpdatedAt time.Time `json:"updated_at"`
}

// RuntimeMirrorCreateRequest represents a request to create a mirror
type RuntimeMirrorCreateRequest struct {
	Lang     string `json:"lang" binding:"required,oneof=node python go java php"`
	EnvKey   string `json:"env_key" binding:"required"`
	EnvValue string `json:"env_value" binding:"required"`
	Enabled  *int   `json:"enabled"`
}

// RuntimeMirrorUpdateRequest represents a request to update a mirror
type RuntimeMirrorUpdateRequest struct {
	EnvValue *string `json:"env_value"`
	Enabled  *int    `json:"enabled"`
}

// GlobalDefaultEntry is one row of the global_default table joined with runtime_version,
// used by the mise config generator to render the [tools] section.
type GlobalDefaultEntry struct {
	Lang  string // node / python / go / java / php
	Exact string // exact version e.g. 20.11.0
}

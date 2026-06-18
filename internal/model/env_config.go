package model

import "time"

// EnvConfig represents an environment variable configuration
type EnvConfig struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`       // JAVA_HOME, GOPATH, etc.
	Value     string    `json:"value"`      // /usr/lib/jvm/java-17, /opt/go, etc.
	RuntimeID int64     `json:"runtime_id"` // Associated runtime environment ID (0 for global)
	IsGlobal  bool      `json:"is_global"`  // Is this a global configuration
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// PathEntry represents a PATH entry
type PathEntry struct {
	ID        int64     `json:"id"`
	Path      string    `json:"path"`       // /usr/lib/jvm/java-17/bin, /opt/go/bin, etc.
	RuntimeID int64     `json:"runtime_id"` // Associated runtime environment ID (0 for global)
	IsGlobal  bool      `json:"is_global"`  // Is this a global configuration
	Order     int       `json:"order"`      // Order in PATH (lower = higher priority)
	CreatedAt time.Time `json:"created_at"`
}

// GlobalConfig represents a global configuration entry
type GlobalConfig struct {
	ID          int64     `json:"id"`
	Category    string    `json:"category"`    // maven, npm, pip, etc.
	Key         string    `json:"key"`         // registry, mirror, proxy, etc.
	Value       string    `json:"value"`       // https://registry.npmmirror.com, etc.
	Description string    `json:"description"` // Human-readable description
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

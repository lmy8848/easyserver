package envconfig

import "time"

// EnvConfig represents an environment variable configuration
type EnvConfig struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`  // JAVA_HOME, GOPATH, etc.
	Value     string    `json:"value"` // /usr/lib/jvm/java-17, /opt/go, etc.
	Enabled   bool      `json:"enabled"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// PathEntry represents a PATH entry
type PathEntry struct {
	ID        int64     `json:"id"`
	Path      string    `json:"path"` // /usr/lib/jvm/java-17/bin, /opt/go/bin, etc.
	Enabled   bool      `json:"enabled"`
	Order     int       `json:"order"` // Order in PATH (lower = higher priority)
	CreatedAt time.Time `json:"created_at"`
}

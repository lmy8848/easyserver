package model

import "time"

// RuntimeVersion represents an available version for a runtime environment
type RuntimeVersion struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`       // java, node, go, python, php
	Version   string    `json:"version"`    // 17.0.19, 20.10.0, 1.21.5
	LTS       bool      `json:"lts"`        // Long Term Support
	Stable    bool      `json:"stable"`     // Stable release
	Installed bool      `json:"installed"`  // Whether this version is installed
	IsDefault bool      `json:"is_default"` // Whether this is the default version
	UpdatedAt time.Time `json:"updated_at"`
}

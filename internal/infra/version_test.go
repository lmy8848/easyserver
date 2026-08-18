package infra

import (
	"testing"
)

func TestIsNewerVersion(t *testing.T) {
	tests := []struct {
		latest  string
		current string
		want    bool
	}{
		{"v0.2.0", "v0.1.0", true},
		{"v1.0.0", "v0.9.9", true},
		{"v0.1.2", "v0.1.1", true},
		{"v0.1.0", "v0.1.0", false},
		{"v0.1.0", "v0.2.0", false},
		{"v0.1.0", "dev", true},
		{"v0.1.0+0xd3a", "v0.1.0", false},
		{"v0.2.0", "v0.1.2-83-g52565d5+0xd3a", true},
		{"v0.1.0", "v0.2.0-83-g52565d5+0xd3a", false},
		{"", "v0.1.0", false},
		{"v0.1.0", "", true},
	}

	for _, tt := range tests {
		got := IsNewerVersion(tt.latest, tt.current)
		if got != tt.want {
			t.Errorf("IsNewerVersion(%q, %q) = %v, want %v", tt.latest, tt.current, got, tt.want)
		}
	}
}

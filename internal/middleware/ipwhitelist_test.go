package middleware

import (
	"easyserver/internal/config"
	"testing"
)

func TestIPWhitelist(t *testing.T) {
	cfg := &config.AuthConfig{
		IPWhitelist: []string{
			"192.168.1.0/24",
			"10.0.0.1",
		},
	}

	wl := NewIPWhitelist(cfg)

	tests := []struct {
		ip       string
		expected bool
	}{
		{"192.168.1.1", true},
		{"192.168.1.100", true},
		{"192.168.2.1", false},
		{"10.0.0.1", true},
		{"10.0.0.2", false},
		{"172.16.0.1", false},
	}

	for _, test := range tests {
		result := wl.IsAllowed(test.ip)
		if result != test.expected {
			t.Errorf("IP '%s': expected %v, got %v", test.ip, test.expected, result)
		}
	}
}

func TestIPWhitelistDisabled(t *testing.T) {
	cfg := &config.AuthConfig{
		IPWhitelist: []string{},
	}

	wl := NewIPWhitelist(cfg)

	// When disabled, all IPs should be allowed
	if !wl.IsAllowed("192.168.1.1") {
		t.Error("Expected all IPs allowed when whitelist is empty")
	}
	if !wl.IsAllowed("10.0.0.1") {
		t.Error("Expected all IPs allowed when whitelist is empty")
	}
}

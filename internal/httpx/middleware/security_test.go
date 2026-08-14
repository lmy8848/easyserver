package middleware

import "testing"

func TestIsLoopbackOrigin(t *testing.T) {
	tests := []struct {
		name   string
		origin string
		want   bool
	}{
		// 本机回环源 —— 应信任
		{"ipv4 loopback", "http://127.0.0.1:5173", true},
		{"ipv4 loopback no port", "http://127.0.0.1", true},
		{"ipv4 loopback whole /8", "http://127.8.9.10:8080", true},
		{"ipv6 loopback", "http://[::1]:5173", true},
		{"localhost", "http://localhost:5173", true},
		{"localhost no port", "http://localhost", true},
		{"localhost case-insensitive", "http://LOCALHOST:5173", true},
		{"trailing slash", "http://127.0.0.1:5173/", true},
		{"ipv4-mapped ipv6 loopback", "http://[::ffff:127.0.0.1]:5173", true},

		// 非回环源 —— 不应信任
		{"empty", "", false},
		{"lan ip", "http://192.168.31.66:5173", false},
		{"public ip", "http://8.8.8.8", false},
		{"other hostname", "http://panel.example.com", false},
		{"localhost subdomain is not loopback", "http://localhost.evil.com", false},
		{"lookalike ip", "http://127.0.0.1.nip.io", false},
		{"no scheme", "127.0.0.1:5173", false},
		{"null origin", "null", false},
		{"garbage", ":::not-an-origin:::", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isLoopbackOrigin(tt.origin); got != tt.want {
				t.Errorf("isLoopbackOrigin(%q) = %v, want %v", tt.origin, got, tt.want)
			}
		})
	}
}

func TestTrustedOrigin(t *testing.T) {
	tests := []struct {
		name           string
		origin         string
		selfOrigin     string
		allowedOrigins []string
		want           bool
	}{
		// 自身 origin（同源部署）
		{"same origin", "http://panel.example.com", "http://panel.example.com", nil, true},
		{"same origin trailing slash normalized", "http://panel.example.com/", "http://panel.example.com", nil, true},
		{"same origin different scheme", "https://panel.example.com", "http://panel.example.com", nil, false},
		{"same origin different host", "http://other.example.com", "http://panel.example.com", nil, false},
		{"selfOrigin empty disables self compare", "http://panel.example.com", "", nil, false},

		// 本机回环源默认信任，无需白名单
		{"loopback origin", "http://127.0.0.1:5173", "http://panel.example.com", nil, true},

		// 白名单
		{"whitelisted", "http://192.168.31.66:5173", "", []string{"http://192.168.31.66:5173"}, true},
		{"whitelist trailing slash normalized", "http://192.168.31.66:5173", "", []string{"http://192.168.31.66:5173/"}, true},
		{"whitelist entry with spaces trimmed", "http://192.168.31.66:5173", "", []string{" http://192.168.31.66:5173 "}, true},
		{"whitelist wildcard not trusted", "http://evil.example.com", "", []string{"*"}, false},
		{"not whitelisted", "http://192.168.31.66:5173", "", []string{"http://localhost:5173"}, false},

		// 边界
		{"empty origin", "", "", nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := trustedOrigin(tt.origin, tt.selfOrigin, tt.allowedOrigins); got != tt.want {
				t.Errorf("trustedOrigin(%q, %q, %v) = %v, want %v",
					tt.origin, tt.selfOrigin, tt.allowedOrigins, got, tt.want)
			}
		})
	}
}

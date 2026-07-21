package http

import (
	"testing"
)

func TestSanitizeCSVField(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"empty string", "", ""},
		{"normal text", "hello", "hello"},
		{"starts with =", "=cmd|' /C calc'!A0", "'=cmd|' /C calc'!A0"},
		{"starts with +", "+1+1", "'+1+1"},
		{"starts with -", "-SUM(A1)", "'-SUM(A1)"},
		{"starts with @", "@SUM(A1:A10)", "'@SUM(A1:A10)"},
		{"starts with tab", "\tdata", "'\tdata"},
		{"starts with carriage return", "\rdata", "'\rdata"},
		{"contains = but not first", "a=b", "a=b"},
		{"contains + but not first", "a+b", "a+b"},
		{"numeric string", "12345", "12345"},
		{"ip address", "192.168.1.1", "192.168.1.1"},
		{"special chars in middle", "hello@world", "hello@world"},
		{"unicode text", "中文测试", "中文测试"},
		{"single = sign", "=", "'="},
		{"single @ sign", "@", "'@"},
		{"starts with multiple dangerous", "=+-@", "'=+-@"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sanitizeCSVField(tt.input)
			if result != tt.expected {
				t.Errorf("sanitizeCSVField(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

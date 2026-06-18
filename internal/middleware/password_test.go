package middleware

import (
	"testing"
)

func TestPasswordStrength(t *testing.T) {
	ps := DefaultPasswordStrength()

	tests := []struct {
		password string
		expected bool
		message  string
	}{
		{"Abc12345", true, ""},
		{"abc12345", false, "密码需要包含大写字母"},
		{"ABC12345", false, "密码需要包含小写字母"},
		{"Abcdefgh", false, "密码需要包含数字"},
		{"Abc123", false, "密码长度不足"},
		{"Abcdefghij12345", true, ""},
	}

	for _, test := range tests {
		valid, msg := ps.Validate(test.password)
		if valid != test.expected {
			t.Errorf("Password '%s': expected %v, got %v", test.password, test.expected, valid)
		}
		if msg != test.message {
			t.Errorf("Password '%s': expected message '%s', got '%s'", test.password, test.message, msg)
		}
	}
}

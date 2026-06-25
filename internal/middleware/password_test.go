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

func TestValidatePasswordStrength(t *testing.T) {
	ps := DefaultPasswordStrength()

	tests := []struct {
		name     string
		password string
		expected bool
		message  string
	}{
		// Valid passwords
		{"valid basic", "Abc12345", true, ""},
		{"valid with special", "Abc@1234!", true, ""},
		{"valid long", "Abcdefghij12345", true, ""},
		{"valid exactly 8", "Abcdefg1", true, ""},
		{"valid 128 chars", buildTestPassword(128), true, ""},
		{"valid unicode upper", "Αβγ1abcD", true, ""},

		// Too short
		{"empty", "", false, "密码长度不足"},
		{"1 char", "A", false, "密码长度不足"},
		{"7 chars", "Abc1234", false, "密码长度不足"},

		// Too long
		{"129 chars", buildTestPassword(129), false, "密码长度不能超过128个字符"},

		// Missing uppercase
		{"no upper", "abcdefg1", false, "密码需要包含大写字母"},
		{"no upper with special", "abc@1234", false, "密码需要包含大写字母"},

		// Missing lowercase
		{"no lower", "ABCDEFG1", false, "密码需要包含小写字母"},
		{"no lower with special", "ABC@1234", false, "密码需要包含小写字母"},

		// Missing digit
		{"no digit", "Abcdefgh", false, "密码需要包含数字"},
		{"no digit with special", "Abc@efgh", false, "密码需要包含数字"},

		// Only one type
		{"only digits", "12345678", false, "密码需要包含大写字母"},
		{"only lower", "abcdefgh", false, "密码需要包含大写字母"},
		{"only upper", "ABCDEFGH", false, "密码需要包含小写字母"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			valid, msg := ps.Validate(tt.password)
			if valid != tt.expected {
				t.Errorf("Validate(%q) valid = %v, want %v", tt.password, valid, tt.expected)
			}
			if msg != tt.message {
				t.Errorf("Validate(%q) message = %q, want %q", tt.password, msg, tt.message)
			}
		})
	}
}

func TestPasswordStrength_RequireSpecial(t *testing.T) {
	ps := &PasswordStrength{
		MinLength:      8,
		RequireUpper:   true,
		RequireLower:   true,
		RequireDigit:   true,
		RequireSpecial: true,
	}

	tests := []struct {
		name     string
		password string
		expected bool
		message  string
	}{
		{"with special", "Abc@1234!", true, ""},
		{"without special", "Abcdefg1", false, "密码需要包含特殊字符"},
		{"only special", "!@#$%^&*", false, "密码需要包含大写字母"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			valid, msg := ps.Validate(tt.password)
			if valid != tt.expected {
				t.Errorf("Validate(%q) valid = %v, want %v", tt.password, valid, tt.expected)
			}
			if msg != tt.message {
				t.Errorf("Validate(%q) message = %q, want %q", tt.password, msg, tt.message)
			}
		})
	}
}

func TestPasswordStrength_CustomMinLength(t *testing.T) {
	ps := &PasswordStrength{
		MinLength:      12,
		RequireUpper:   true,
		RequireLower:   true,
		RequireDigit:   true,
		RequireSpecial: false,
	}

	valid, msg := ps.Validate("Abc123456789")
	if !valid {
		t.Errorf("expected valid, got false: %s", msg)
	}

	valid, msg = ps.Validate("Abc12345")
	if valid {
		t.Error("expected invalid for 8-char password with min 12")
	}
	if msg != "密码长度不足" {
		t.Errorf("message = %q, want %q", msg, "密码长度不足")
	}
}

func TestPasswordStrength_NoRequirements(t *testing.T) {
	ps := &PasswordStrength{
		MinLength:      1,
		RequireUpper:   false,
		RequireLower:   false,
		RequireDigit:   false,
		RequireSpecial: false,
	}

	valid, _ := ps.Validate("x")
	if !valid {
		t.Error("expected valid with no requirements")
	}
}

func buildTestPassword(length int) string {
	b := make([]byte, length)
	for i := range b {
		switch i % 3 {
		case 0:
			b[i] = 'A'
		case 1:
			b[i] = 'a'
		case 2:
			b[i] = '1'
		}
	}
	return string(b)
}

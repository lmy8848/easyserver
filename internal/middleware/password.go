package middleware

import (
	"unicode"
)

// PasswordStrength checks password strength
type PasswordStrength struct {
	MinLength      int
	RequireUpper   bool
	RequireLower   bool
	RequireDigit   bool
	RequireSpecial bool
}

func DefaultPasswordStrength() *PasswordStrength {
	return &PasswordStrength{
		MinLength:      8,
		RequireUpper:   true,
		RequireLower:   true,
		RequireDigit:   true,
		RequireSpecial: false,
	}
}

// Validate checks if password meets requirements
func (ps *PasswordStrength) Validate(password string) (bool, string) {
	if len(password) < ps.MinLength {
		return false, "密码长度不足"
	}
	if len(password) > 128 {
		return false, "密码长度不能超过128个字符"
	}

	var hasUpper, hasLower, hasDigit, hasSpecial bool

	for _, char := range password {
		switch {
		case unicode.IsUpper(char):
			hasUpper = true
		case unicode.IsLower(char):
			hasLower = true
		case unicode.IsDigit(char):
			hasDigit = true
		case unicode.IsPunct(char) || unicode.IsSymbol(char):
			hasSpecial = true
		}
	}

	if ps.RequireUpper && !hasUpper {
		return false, "密码需要包含大写字母"
	}
	if ps.RequireLower && !hasLower {
		return false, "密码需要包含小写字母"
	}
	if ps.RequireDigit && !hasDigit {
		return false, "密码需要包含数字"
	}
	if ps.RequireSpecial && !hasSpecial {
		return false, "密码需要包含特殊字符"
	}

	return true, ""
}

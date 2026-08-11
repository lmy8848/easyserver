package database

import (
	"fmt"
	"strings"
	"testing"
)

// --- SanitizeSQLError tests ---

func TestSanitizeSQLError(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "strips paths, keeps message",
			input: "ERROR 1045 (28000): Access denied for user 'root'@'localhost' (using password: YES)\n/usr/bin/mysql",
			want:  "ERROR 1045 (28000): Access denied for user 'root'@'localhost' (using password: YES)\n[...]",
		},
		{
			name:  "multiple paths on one line",
			input: "error at /var/lib/mysql/data and /etc/mysql/conf",
			want:  "error at [...] and [...]",
		},
		{
			name:  "empty after trimming",
			input: "   \n   \n   ",
			want:  "",
		},
		{
			name:  "no paths",
			input: "ERROR: syntax error at or near \"SELEC\"",
			want:  "ERROR: syntax error at or near \"SELEC\"",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SanitizeSQLError(tt.input)
			if got != tt.want {
				t.Errorf("SanitizeSQLError() = %q, want %q", got, tt.want)
			}
		})
	}
}

// --- ValidateTableName tests ---

func TestValidateTableName(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{"users", true},
		{"_internal", true},
		{"table_01", true},
		{"", false},
		{"a]b", false},
		{"table name", false},
		{"table-name", false},
		{"a", true},
		{strings.Repeat("a", 65), false}, // 65 chars, too long
		{strings.Repeat("a", 64), true},  // 64 chars, max
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("%q", tt.name), func(t *testing.T) {
			if got := ValidateTableName(tt.name); got != tt.want {
				t.Errorf("ValidateTableName(%q) = %v, want %v", tt.name, got, tt.want)
			}
		})
	}
}

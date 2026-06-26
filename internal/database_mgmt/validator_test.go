package database_mgmt

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
		{"table-name", false}, // hyphens not allowed (differs from SQLValidator.ValidateIdentifier)
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

// --- TestIsValidDBName (via SQLValidator) ---

func TestIsValidDBName(t *testing.T) {
	v := NewSQLValidator("sqlite")

	tests := []struct {
		name    string
		dbName  string
		isValid bool
	}{
		{"valid simple", "mydb", true},
		{"valid with underscore", "my_database", true},
		{"valid with hyphen", "my-database", true},
		{"valid with digits", "db123", true},
		{"valid mixed", "My_DB-123", true},
		{"empty name", "", false},
		{"starts with digit", "1database", true}, // digits are valid
		{"contains space", "my database", false},
		{"contains special @", "my@db", false},
		{"contains dot", "my.db", false},
		{"contains slash", "my/db", false},
		{"contains semicolon", "my;db", false},
		{"contains quote", "my'db", false},
		{"64 chars", buildString(64, 'a'), true},
		{"65 chars", buildString(65, 'a'), false},
		{"sql injection attempt", "db; DROP TABLE users", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := v.ValidateDatabaseName(tt.dbName)
			if result.Valid != tt.isValid {
				t.Errorf("ValidateDatabaseName(%q) valid = %v, want %v", tt.dbName, result.Valid, tt.isValid)
			}
		})
	}
}

func buildString(length int, ch byte) string {
	b := make([]byte, length)
	for i := range b {
		b[i] = ch
	}
	return string(b)
}

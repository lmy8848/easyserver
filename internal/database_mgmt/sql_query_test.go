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

// --- ListTables output parsing tests ---

func TestParseMySQLListTables(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want int // expected table count
	}{
		{
			name: "normal",
			raw:  "Tables_in_mydb\nusers\norders\nproducts\n",
			want: 3,
		},
		{
			name: "empty database",
			raw:  "Tables_in_mydb\n",
			want: 0,
		},
		{
			name: "blank lines skipped",
			raw:  "Tables_in_mydb\nusers\n\norders\n\n",
			want: 2,
		},
		{
			name: "single table",
			raw:  "Tables_in_mydb\nmy_table\n",
			want: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tables := parseMySQLListTables(tt.raw)
			if len(tables) != tt.want {
				t.Errorf("got %d tables, want %d. tables: %v", len(tables), tt.want, tables)
			}
		})
	}
}

func TestParsePostgreSQLListTables(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want int
	}{
		{
			name: "normal",
			raw:  " tablename \n----------\n users\n orders\n(3 rows)\n",
			want: 2,
		},
		{
			name: "zero rows",
			raw:  " tablename \n----------\n(0 rows)\n",
			want: 0,
		},
		{
			name: "empty output",
			raw:  " tablename \n----------\n",
			want: 0,
		},
		{
			name: "single table",
			raw:  " tablename \n----------\n my_table\n(1 row)\n",
			want: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tables := parsePGListTables(tt.raw)
			if len(tables) != tt.want {
				t.Errorf("got %d tables, want %d. tables: %v", len(tables), tt.want, tables)
			}
		})
	}
}

// --- QueryTable output parsing tests ---

func TestParseMySQLQueryTable(t *testing.T) {
	tests := []struct {
		name     string
		raw      string
		wantCols int
		wantRows int
	}{
		{
			name:     "normal",
			raw:      "id\tname\temail\n1\tAlice\talice@example.com\n2\tBob\tbob@example.com\n",
			wantCols: 3,
			wantRows: 2,
		},
		{
			name:     "empty result",
			raw:      "id\tname\temail\n",
			wantCols: 3,
			wantRows: 0,
		},
		{
			name:     "NULL display",
			raw:      "id\tname\n1\tNULL\n2\t\\N\n",
			wantCols: 2,
			wantRows: 2,
		},
		{
			name:     "single column",
			raw:      "count\n42\n",
			wantCols: 1,
			wantRows: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			headers, rows := parseMySQLQueryResult(tt.raw)
			if len(headers) != tt.wantCols {
				t.Errorf("got %d cols, want %d", len(headers), tt.wantCols)
			}
			if len(rows) != tt.wantRows {
				t.Errorf("got %d rows, want %d", len(rows), tt.wantRows)
			}
		})
	}
}

func TestParsePostgreSQLQueryTable(t *testing.T) {
	tests := []struct {
		name     string
		raw      string
		wantCols int
		wantRows int
	}{
		{
			name:     "normal",
			raw:      " id | name  |       email\n----+-------+-------------------\n  1 | Alice | alice@example.com\n  2 | Bob   | bob@example.com\n(2 rows)\n",
			wantCols: 3,
			wantRows: 2,
		},
		{
			name:     "zero rows",
			raw:      " id | name\n----+------\n(0 rows)\n",
			wantCols: 2,
			wantRows: 0,
		},
		{
			name:     "empty result with separator",
			raw:      " id | name\n----+------\n",
			wantCols: 2,
			wantRows: 0,
		},
		{
			name:     "NULL display as empty",
			raw:      " id | name\n----+------\n  1 | \n  2 | Alice\n(2 rows)\n",
			wantCols: 2,
			wantRows: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			headers, rows := parsePGQueryResult(tt.raw)
			if len(headers) != tt.wantCols {
				t.Errorf("got %d cols, want %d. headers: %v", len(headers), tt.wantCols, headers)
			}
			if len(rows) != tt.wantRows {
				t.Errorf("got %d rows, want %d", len(rows), tt.wantRows)
			}
		})
	}
}

// --- Parsing helper functions (extracted for testability) ---
// These mirror the logic in ListTables / QueryTable methods.

func parseMySQLListTables(raw string) []map[string]interface{} {
	var tables []map[string]interface{}
	lines := splitTrimLines(raw)
	for i, line := range lines {
		if i == 0 {
			continue
		} // skip header
		if line != "" {
			tables = append(tables, map[string]interface{}{"name": line})
		}
	}
	return tables
}

func parsePGListTables(raw string) []map[string]interface{} {
	var tables []map[string]interface{}
	lines := splitTrimLines(raw)
	for i, line := range lines {
		if i < 2 || line == "" || line == "(0 rows)" || startsWith(line, "-") || startsWith(line, "(") {
			continue
		}
		tables = append(tables, map[string]interface{}{"name": line})
	}
	return tables
}

func parseMySQLQueryResult(raw string) (headers []string, rows [][]interface{}) {
	lines := splitTrimLines(raw)
	for i, line := range lines {
		fields := splitTab(line)
		if i == 0 {
			headers = fields
		} else {
			var row []interface{}
			for _, f := range fields {
				row = append(row, f)
			}
			rows = append(rows, row)
		}
	}
	return
}

func parsePGQueryResult(raw string) (headers []string, rows [][]interface{}) {
	lines := splitTrimLines(raw)
	for i, line := range lines {
		fields := splitPipe(line)
		if i == 0 {
			headers = fields
		} else if i >= 2 && !startsWith(line, "(") && line != "" {
			var row []interface{}
			for _, f := range fields {
				row = append(row, f)
			}
			rows = append(rows, row)
		}
	}
	return
}

// --- Tiny helpers (avoid importing strings in test) ---

func splitTrimLines(s string) []string {
	var out []string
	for _, line := range splitNewline(s) {
		out = append(out, trimSpace(line))
	}
	return out
}

func splitNewline(s string) []string {
	var result []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			result = append(result, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		result = append(result, s[start:])
	}
	return result
}

func splitTab(s string) []string  { return splitChar(s, '\t') }
func splitPipe(s string) []string { return splitChar(s, '|') }

func splitChar(s string, sep byte) []string {
	var result []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == sep {
			result = append(result, trimSpace(s[start:i]))
			start = i + 1
		}
	}
	result = append(result, trimSpace(s[start:]))
	return result
}

func trimSpace(s string) string {
	start, end := 0, len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t' || s[start] == '\r') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t' || s[end-1] == '\r') {
		end--
	}
	return s[start:end]
}

func startsWith(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}

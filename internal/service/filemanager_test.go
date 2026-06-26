package service

import (
	"os"
	"path/filepath"
	"testing"

	"easyserver/internal/database_mgmt"
	"easyserver/internal/firewall"
)

// --- TestValidatePath ---

func TestFileManagerValidatePath(t *testing.T) {
	// Create temp directory for testing
	tmpDir, err := os.MkdirTemp("", "filemanager-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Create subdirectory for testing
	os.MkdirAll(filepath.Join(tmpDir, "subdir"), 0755)

	fm, err := NewFileManager(tmpDir)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		path      string
		expectErr bool
	}{
		{"test.txt", false},
		{"subdir/test.txt", false},
		{"../../../etc/passwd", true}, // Path traversal
		{"test/../test.txt", false},   // Clean path
	}

	for _, test := range tests {
		_, err := fm.ValidatePath(test.path)
		if (err != nil) != test.expectErr {
			t.Errorf("Path '%s': expected error=%v, got error=%v", test.path, test.expectErr, err != nil)
		}
	}
}

func TestValidatePath_TraversalAttempts(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "filemanager-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Create subdirectories for testing
	os.MkdirAll(filepath.Join(tmpDir, "sub", "dir"), 0755)
	os.MkdirAll(filepath.Join(tmpDir, "a", "b", "c"), 0755)

	fm, err := NewFileManager(tmpDir)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name      string
		path      string
		expectErr bool
	}{
		{"simple relative", "file.txt", false},
		{"subdir", "sub/dir/file.txt", false},
		{"dot-dot traversal", "../../../etc/passwd", true},
		{"dot-dot in middle", "sub/../../../etc/passwd", true},
		{"dot-dot clean to base", "sub/..", false}, // cleans to base itself
		{"single dot", ".", false},                 // current dir = base
		{"dot-dot from base", "..", true},          // escapes base
		{"nested dot-dot", "a/b/c/../../../../d", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := fm.ValidatePath(tt.path)
			if (err != nil) != tt.expectErr {
				t.Errorf("ValidatePath(%q) error = %v, wantErr %v", tt.path, err, tt.expectErr)
			}
		})
	}
}

func TestValidatePath_AbsolutePathRejected(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "filemanager-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	fm, err := NewFileManager(tmpDir)
	if err != nil {
		t.Fatal(err)
	}

	// Absolute paths should be rejected
	_, err = fm.ValidatePath("/etc/passwd")
	if err == nil {
		t.Error("expected error for absolute path")
	}

	_, err = fm.ValidatePath("/tmp/test.txt")
	if err == nil {
		t.Error("expected error for absolute path")
	}
}

func TestNewFileManager_InvalidPaths(t *testing.T) {
	// Empty base path
	_, err := NewFileManager("")
	if err == nil {
		t.Error("expected error for empty base path")
	}

	// Root path
	_, err = NewFileManager("/")
	if err == nil {
		t.Error("expected error for root base path")
	}
}

// --- TestCopy ---

func TestFileManagerCopy(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "filemanager-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	fm, err := NewFileManager(tmpDir)
	if err != nil {
		t.Fatal(err)
	}

	// Create test file
	testFile := filepath.Join(tmpDir, "test.txt")
	os.WriteFile(testFile, []byte("hello"), 0644)

	// Test copy to same file should fail
	err = fm.Copy("test.txt", "test.txt")
	if err == nil {
		t.Error("Expected error when copying to same file")
	}

	// Test copy to different file should succeed
	err = fm.Copy("test.txt", "copy.txt")
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	// Verify content
	destFile := filepath.Join(tmpDir, "copy.txt")
	content, err := os.ReadFile(destFile)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "hello" {
		t.Errorf("Expected 'hello', got '%s'", string(content))
	}
}

// --- TestIsProtectedPort ---

func TestIsProtectedPort(t *testing.T) {
	fw := firewall.NewService(nil, nil)
	fw.SetProtectedPorts([]string{"22", "80", "443", "8080"})

	tests := []struct {
		name     string
		port     string
		expected bool
	}{
		{"protected port 22", "22", true},
		{"protected port 80", "80", true},
		{"protected port 443", "443", true},
		{"protected port 8080", "8080", true},
		{"unprotected port 8081", "8081", false},
		{"unprotected port 3000", "3000", false},
		{"empty port", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := fw.IsProtectedPort(t.Context(), tt.port)
			if result != tt.expected {
				t.Errorf("IsProtectedPort(%q) = %v, want %v", tt.port, result, tt.expected)
			}
		})
	}
}

func TestIsProtectedPort_Range(t *testing.T) {
	fw := firewall.NewService(nil, nil)
	fw.SetProtectedPorts([]string{"22", "8080"})

	tests := []struct {
		name     string
		port     string
		expected bool
	}{
		{"range includes protected", "8079-8081", true},
		{"range excludes protected", "9000-9010", false},
		{"range at start", "22-25", true},
		{"single in range", "8080-8080", true},
		{"invalid range", "abc-def", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := fw.IsProtectedPort(t.Context(), tt.port)
			if result != tt.expected {
				t.Errorf("IsProtectedPort(%q) = %v, want %v", tt.port, result, tt.expected)
			}
		})
	}
}

func TestIsProtectedPort_SetProtectedPorts(t *testing.T) {
	fw := firewall.NewService(nil, nil)
	fw.SetProtectedPorts([]string{"22"})

	if !fw.IsProtectedPort(t.Context(), "22") {
		t.Error("port 22 should be protected by default")
	}
	if fw.IsProtectedPort(t.Context(), "3306") {
		t.Error("port 3306 should not be protected initially")
	}

	fw.SetProtectedPorts([]string{"22", "3306", "5432"})

	if !fw.IsProtectedPort(t.Context(), "3306") {
		t.Error("port 3306 should be protected after SetProtectedPorts")
	}
	if !fw.IsProtectedPort(t.Context(), "5432") {
		t.Error("port 5432 should be protected after SetProtectedPorts")
	}
}

// --- TestIsValidDBName (via SQLValidator) ---

func TestIsValidDBName(t *testing.T) {
	v := database_mgmt.NewSQLValidator(database_mgmt.DBTypeMySQL)

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

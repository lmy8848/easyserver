package filemanager

import (
	"os"
	"path/filepath"
	"testing"
)

// --- TestValidatePath ---

func TestFileManagerValidatePath(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "filemanager-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	os.MkdirAll(filepath.Join(tmpDir, "subdir"), 0755)

	fm, err := NewManager(tmpDir)
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

	os.MkdirAll(filepath.Join(tmpDir, "sub", "dir"), 0755)
	os.MkdirAll(filepath.Join(tmpDir, "a", "b", "c"), 0755)

	fm, err := NewManager(tmpDir)
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

	fm, err := NewManager(tmpDir)
	if err != nil {
		t.Fatal(err)
	}

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
	_, err := NewManager("")
	if err == nil {
		t.Error("expected error for empty base path")
	}

	_, err = NewManager("/")
	if err == nil {
		t.Error("expected error for root base path")
	}
}

// --- TestCopy ---

func TestFileManagerCopy(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "filemanager-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	fm, err := NewManager(tmpDir)
	if err != nil {
		t.Fatal(err)
	}

	testFile := filepath.Join(tmpDir, "test.txt")
	os.WriteFile(testFile, []byte("hello"), 0644)

	err = fm.Copy("test.txt", "test.txt")
	if err == nil {
		t.Error("Expected error when copying to same file")
	}

	err = fm.Copy("test.txt", "copy.txt")
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	destFile := filepath.Join(tmpDir, "copy.txt")
	content, err := os.ReadFile(destFile)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "hello" {
		t.Errorf("Expected 'hello', got '%s'", string(content))
	}
}


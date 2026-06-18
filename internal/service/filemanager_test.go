package service

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFileManagerValidatePath(t *testing.T) {
	// Create temp directory for testing
	tmpDir, err := os.MkdirTemp("", "filemanager-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	fm := NewFileManager(tmpDir)

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

func TestFileManagerCopy(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "filemanager-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	fm := NewFileManager(tmpDir)

	// Create test file
	testFile := filepath.Join(tmpDir, "test.txt")
	os.WriteFile(testFile, []byte("hello"), 0644)

	// Test copy to same file should fail
	err = fm.Copy(testFile, testFile)
	if err == nil {
		t.Error("Expected error when copying to same file")
	}

	// Test copy to different file should succeed
	destFile := filepath.Join(tmpDir, "copy.txt")
	err = fm.Copy(testFile, destFile)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	// Verify content
	content, err := os.ReadFile(destFile)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "hello" {
		t.Errorf("Expected 'hello', got '%s'", string(content))
	}
}

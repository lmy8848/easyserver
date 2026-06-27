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

func TestValidatePath_AbsolutePathMapped(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "filemanager-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	fm, err := NewManager(tmpDir)
	if err != nil {
		t.Fatal(err)
	}

	// Create directories inside the sandbox so EvalSymlinks parent directory resolving succeeds
	err = os.MkdirAll(filepath.Join(tmpDir, "etc"), 0755)
	if err != nil {
		t.Fatal(err)
	}
	err = os.MkdirAll(filepath.Join(tmpDir, "tmp"), 0755)
	if err != nil {
		t.Fatal(err)
	}

	path, err := fm.ValidatePath("/etc/passwd")
	if err != nil {
		t.Errorf("unexpected error for absolute path: %v", err)
	}
	expected := filepath.Join(tmpDir, "etc/passwd")
	if path != expected {
		t.Errorf("expected %q, got %q", expected, path)
	}

	path, err = fm.ValidatePath("/tmp/test.txt")
	if err != nil {
		t.Errorf("unexpected error for absolute path: %v", err)
	}
	expectedTmp := filepath.Join(tmpDir, "tmp/test.txt")
	if path != expectedTmp {
		t.Errorf("expected %q, got %q", expectedTmp, path)
	}
}

func TestNewFileManager_InvalidPaths(t *testing.T) {
	_, err := NewManager("")
	if err == nil {
		t.Error("expected error for empty base path")
	}

	// Root base path is now allowed for server management
	_, err = NewManager("/")
	if err != nil {
		t.Errorf("unexpected error for root base path: %v", err)
	}

	// ~ should expand to home directory
	fm, err := NewManager("~")
	if err != nil {
		t.Errorf("unexpected error for ~ base path: %v", err)
	}
	home, _ := os.UserHomeDir()
	if fm.BasePath() != home {
		t.Errorf("expected %s, got %s", home, fm.BasePath())
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

func TestIsSubPath(t *testing.T) {
	tests := []struct {
		parent string
		child  string
		want   bool
	}{
		{"/a/b", "/a/b", true},
		{"/a/b", "/a/b/c", true},
		{"/a/b", "/a/b/c/d", true},
		{"/a/b", "/a/bc", false},  // Prefix bypass attempt
		{"/a/b", "/a/b-c", false}, // Prefix bypass attempt
		{"/a/b", "/a/d", false},
		{"/a/b", "/a", false},
	}

	for _, tt := range tests {
		got := isSubPath(tt.parent, tt.child)
		if got != tt.want {
			t.Errorf("isSubPath(%q, %q) = %v; want %v", tt.parent, tt.child, got, tt.want)
		}
	}
}

func TestToRelativePath(t *testing.T) {
	fm, err := NewManager("/a/b")
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		abs  string
		want string
	}{
		{"/a/b", "/"},
		{"/a/b/c", "/c"},
		{"/a/b/c/d", "/c/d"},
		{"/a/b-c", "/a/b-c"}, // Should not be truncated
		{"/a/bc", "/a/bc"},   // Should not be truncated
		{"/other", "/other"}, // Outside base path
	}

	for _, tt := range tests {
		got := fm.toRelativePath(tt.abs)
		if got != tt.want {
			t.Errorf("toRelativePath(%q) = %q; want %q", tt.abs, got, tt.want)
		}
	}
}

func TestValidatePath_MultilevelNotExist(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "filemanager-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	fm, err := NewManager(tmpDir)
	if err != nil {
		t.Fatal(err)
	}

	// a/b/c does not exist. It should be parsed as valid.
	path, err := fm.ValidatePath("a/b/c")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	expected := filepath.Join(tmpDir, "a/b/c")
	if path != expected {
		t.Errorf("expected %q, got %q", expected, path)
	}
}

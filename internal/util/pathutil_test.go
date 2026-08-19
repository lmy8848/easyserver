package util

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIsTraversal(t *testing.T) {
	tests := []struct {
		path     string
		expected bool
	}{
		{"", false},
		{"normal.txt", false},
		{"sub/dir/file.txt", false},
		{"../evil.txt", true},
		{"sub/../evil.txt", true},
		{"sub/dir/..", true},
		{"file\x00.txt", true},
	}

	for _, tt := range tests {
		got := IsTraversal(tt.path)
		if got != tt.expected {
			t.Errorf("IsTraversal(%q) = %v, expected %v", tt.path, got, tt.expected)
		}
	}
}

func TestValidateFilename(t *testing.T) {
	tests := []struct {
		name      string
		expectErr bool
	}{
		{"", true},
		{".", true},
		{"..", true},
		{"foo/bar", true},
		{"foo\\bar", true},
		{"../bar", true},
		{"bar..", true},
		{"valid_file-123.txt", false},
		{"easyserver-cron-test.timer", false},
		{"null\x00byte", true},
	}

	for _, tt := range tests {
		err := ValidateFilename(tt.name)
		if (err != nil) != tt.expectErr {
			t.Errorf("ValidateFilename(%q) error = %v, expectErr = %v", tt.name, err, tt.expectErr)
		}
	}
}

func TestIsSubPath(t *testing.T) {
	tests := []struct {
		parent   string
		child    string
		expected bool
	}{
		{"/var/data", "/var/data", true},
		{"/var/data", "/var/data/sub", true},
		{"/var/data", "/var/data/sub/file.txt", true},
		{"/var/data", "/var/dataset", false},
		{"/var/data", "/var/data/../other", false},
		{"/var/data", "/etc/passwd", false},
		{"/", "/any/path", true},
		{"/var/data/", "/var/data/sub", true},
	}

	for _, tt := range tests {
		got := IsSubPath(tt.parent, tt.child)
		if got != tt.expected {
			t.Errorf("IsSubPath(%q, %q) = %v, expected %v", tt.parent, tt.child, got, tt.expected)
		}
	}
}

func TestJoinSafe(t *testing.T) {
	base := "/var/data"
	tests := []struct {
		subpaths  []string
		expectErr bool
		expected  string
	}{
		{[]string{"sub", "file.txt"}, false, "/var/data/sub/file.txt"},
		{[]string{"sub/.."}, false, "/var/data"},
		{[]string{".."}, true, ""},
		{[]string{"sub", "../../etc/passwd"}, true, ""},
		{[]string{"null\x00"}, true, ""},
	}

	for _, tt := range tests {
		got, err := JoinSafe(base, tt.subpaths...)
		if (err != nil) != tt.expectErr {
			t.Errorf("JoinSafe(%q, %v) error = %v, expectErr = %v", base, tt.subpaths, err, tt.expectErr)
		}
		if err == nil && got != tt.expected {
			t.Errorf("JoinSafe(%q, %v) = %q, expected %q", base, tt.subpaths, got, tt.expected)
		}
	}
}

func TestResolveInSandbox(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "sandbox-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	subDir := filepath.Join(tmpDir, "sub")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create a safe symlink pointing inside
	if err := os.Symlink(subDir, filepath.Join(tmpDir, "link_inside")); err != nil {
		t.Fatal(err)
	}

	// Create an escaping symlink pointing outside
	if err := os.Symlink("/etc", filepath.Join(tmpDir, "link_outside")); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name      string
		path      string
		expectErr bool
	}{
		{"empty", "", false},
		{"dot", ".", false},
		{"root", "/", false},
		{"simple relative", "sub/file.txt", false},
		{"simple absolute", "/sub/file.txt", false},
		{"dot-dot traversal", "../etc/passwd", true},
		{"dot-dot nested escape", "sub/../../etc/passwd", true},
		{"symlink inside", "link_inside/file.txt", false},
		{"symlink outside escape", "link_outside/passwd", true},
		{"null byte", "file\x00.txt", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ResolveInSandbox(tmpDir, tt.path)
			if (err != nil) != tt.expectErr {
				t.Errorf("ResolveInSandbox(%q) error = %v, expectErr = %v", tt.path, err, tt.expectErr)
			}
		})
	}
}

func TestResolveShareSubpath(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "share-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	shareDir := filepath.Join(tmpDir, "shared_folder")
	if err := os.MkdirAll(filepath.Join(shareDir, "sub"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(shareDir, "sub", "file.txt"), []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name      string
		subpath   string
		expectErr bool
	}{
		{"empty subpath", "", false},
		{"slash subpath", "/", false},
		{"valid subpath", "sub/file.txt", false},
		{"traversal in subpath", "../escape.txt", true},
		{"null byte in subpath", "file\x00.txt", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ResolveShareSubpath(tmpDir, shareDir, tt.subpath)
			if (err != nil) != tt.expectErr {
				t.Errorf("ResolveShareSubpath(%q) error = %v, expectErr = %v", tt.subpath, err, tt.expectErr)
			}
		})
	}
}

package filemanager

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Helper to create a zip archive
func createZip(t *testing.T, files map[string]string, symlinks map[string]string) string {
	path := filepath.Join(t.TempDir(), "test.zip")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	w := zip.NewWriter(f)

	for name, content := range files {
		fw, err := w.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		_, err = fw.Write([]byte(content))
		if err != nil {
			t.Fatal(err)
		}
	}

	for name, target := range symlinks {
		fh := &zip.FileHeader{
			Name:   name,
			Method: zip.Store,
		}
		fh.SetMode(os.ModeSymlink | 0777)
		fw, err := w.CreateHeader(fh)
		if err != nil {
			t.Fatal(err)
		}
		_, err = fw.Write([]byte(target))
		if err != nil {
			t.Fatal(err)
		}
	}

	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

// Helper to create a tar.gz archive
func createTarGz(t *testing.T, files map[string]string, symlinks map[string]string, hardlinks map[string]string) string {
	path := filepath.Join(t.TempDir(), "test.tar.gz")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	gzw := gzip.NewWriter(f)
	defer gzw.Close()
	tw := tar.NewWriter(gzw)
	defer tw.Close()

	for name, content := range files {
		hdr := &tar.Header{
			Name:     name,
			Mode:     0644,
			Size:     int64(len(content)),
			Typeflag: tar.TypeReg,
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}

	for name, target := range symlinks {
		hdr := &tar.Header{
			Name:     name,
			Linkname: target,
			Typeflag: tar.TypeSymlink,
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatal(err)
		}
	}

	for name, target := range hardlinks {
		hdr := &tar.Header{
			Name:     name,
			Linkname: target,
			Typeflag: tar.TypeLink,
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatal(err)
		}
	}
	return path
}

func TestExtract_PathTraversal(t *testing.T) {
	baseDir := t.TempDir()
	m, _ := NewManager(baseDir)

	zipPath := createZip(t, map[string]string{"../escape.txt": "evil"}, nil)
	err := m.extractZip(zipPath, baseDir)
	if err == nil || !strings.Contains(err.Error(), "path traversal") {
		t.Errorf("expected path traversal error, got %v", err)
	}

	tarPath := createTarGz(t, map[string]string{"../escape.txt": "evil"}, nil, nil)
	err = m.extractTarGz(tarPath, baseDir)
	if err == nil || !strings.Contains(err.Error(), "path traversal") {
		t.Errorf("expected path traversal error, got %v", err)
	}
}

func TestExtract_Symlink(t *testing.T) {
	baseDir := t.TempDir()
	m, _ := NewManager(baseDir)

	zipPath := createZip(t, nil, map[string]string{"link.txt": "/etc/passwd"})
	err := m.extractZip(zipPath, baseDir)
	if err == nil || !strings.Contains(err.Error(), "symlinks not allowed") {
		t.Errorf("expected symlink error, got %v", err)
	}

	tarPath := createTarGz(t, nil, map[string]string{"link.txt": "/etc/passwd"}, nil)
	err = m.extractTarGz(tarPath, baseDir)
	if err == nil || !strings.Contains(err.Error(), "symlinks not allowed") {
		t.Errorf("expected symlink error, got %v", err)
	}
}

func TestExtract_Hardlink(t *testing.T) {
	baseDir := t.TempDir()
	m, _ := NewManager(baseDir)

	tarPath := createTarGz(t, map[string]string{"a.txt": "a"}, nil, map[string]string{"link.txt": "a.txt"})
	err := m.extractTarGz(tarPath, baseDir)
	if err == nil || !strings.Contains(err.Error(), "hard links not allowed") {
		t.Errorf("expected hard link error, got %v", err)
	}
}

func TestExtract_MaxExtractSize(t *testing.T) {
	origSize := maxExtractSize
	maxExtractSize = 10
	defer func() { maxExtractSize = origSize }()

	baseDir := t.TempDir()
	m, _ := NewManager(baseDir)

	zipPath := createZip(t, map[string]string{"big.txt": "this is more than 10 bytes"}, nil)
	err := m.extractZip(zipPath, baseDir)
	if err == nil || !strings.Contains(err.Error(), "extraction limit exceeded") {
		t.Errorf("expected size limit error, got %v", err)
	}
}

func TestExtract_MaxFileCount(t *testing.T) {
	origCount := maxExtractFileCount
	maxExtractFileCount = 2
	defer func() { maxExtractFileCount = origCount }()

	baseDir := t.TempDir()
	m, _ := NewManager(baseDir)

	zipPath := createZip(t, map[string]string{"1.txt": "1", "2.txt": "2", "3.txt": "3"}, nil)
	err := m.extractZip(zipPath, baseDir)
	if err == nil || !strings.Contains(err.Error(), "too many files") {
		t.Errorf("expected count limit error, got %v", err)
	}
}

func TestExtract_Rollback(t *testing.T) {
	origSize := maxExtractSize
	maxExtractSize = 10
	defer func() { maxExtractSize = origSize }()

	baseDir := t.TempDir()
	m, _ := NewManager(baseDir)

	// First file succeeds, second file triggers size limit error, everything should be rolled back.
	zipPath := createZip(t, map[string]string{
		"dir/ok.txt":  "ok",
		"dir/bad.txt": "this is too big for the limit",
	}, nil)

	err := m.extractZip(zipPath, baseDir)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	// Check that dir and dir/ok.txt are gone
	if _, err := os.Stat(filepath.Join(baseDir, "dir")); !os.IsNotExist(err) {
		t.Errorf("expected rollback to remove 'dir', but it exists or error is different: %v", err)
	}
}

// Regression test for the Walk-callback symlink-escape attack: a symlink that
// lives inside basePath but points outside must not be traversed by Search /
// SearchContent / Compress. Earlier ValidatePath inside the Walk callback was
// dead code (double-Join), letting an attacker exfiltrate /etc/passwd via
// SearchContent or pack it into a Compress zip.
func TestWalkCallback_RejectsSymlinkEscape(t *testing.T) {
	base := t.TempDir()
	outside := filepath.Join(t.TempDir(), "secret.txt")
	if err := os.WriteFile(outside, []byte("TOP-SECRET-deadbeef"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(base, "leak")); err != nil {
		t.Fatal(err)
	}

	m, err := NewManager(base)
	if err != nil {
		t.Fatal(err)
	}

	results, err := m.SearchContent("/", "deadbeef", 10)
	if err != nil {
		t.Fatalf("SearchContent err: %v", err)
	}
	for _, r := range results {
		t.Errorf("symlink content leaked into SearchContent results: %+v", r)
	}

	destZip := "/out.zip" // sandbox-relative — Compress() maps it to base/out.zip
	if err := m.Compress([]string{"/"}, destZip); err != nil {
		t.Fatalf("Compress err: %v", err)
	}
	zr, err := zip.OpenReader(filepath.Join(base, "out.zip"))
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	for _, f := range zr.File {
		if strings.HasSuffix(f.Name, "/leak") || f.Name == "leak" {
			rc, _ := f.Open()
			data, _ := io.ReadAll(rc)
			rc.Close()
			if strings.Contains(string(data), "deadbeef") {
				t.Errorf("Compress packed symlink target into archive: %q", data)
			}
		}
	}
}

func TestArchiveMode(t *testing.T) {
	tests := []struct {
		in       os.FileMode
		expected os.FileMode
	}{
		{04755, 0755}, // setuid 丢弃
		{02644, 0644}, // setgid 丢弃
		{01755, 0755}, // sticky 丢弃
		{01644, 0644}, // sticky 丢弃
		{06777, 0755}, // setuid+setgid+sticky 全丢
		{07644, 0644}, // setuid+setgid+sticky 全丢，无 exec
		{00644, 0644},
		{00755, 0755},
		{00600, 0644},
	}

	for _, tt := range tests {
		got := archiveMode(tt.in)
		if got != tt.expected {
			t.Errorf("archiveMode(%o) = %o, want %o", tt.in, got, tt.expected)
		}
	}
}

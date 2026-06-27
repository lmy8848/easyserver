package filemanager

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
)

// FileEntry represents a file or directory entry.
type FileEntry struct {
	Name       string `json:"name"`
	Path       string `json:"path"`
	IsDir      bool   `json:"is_dir"`
	SizeBytes  int64  `json:"size_bytes"`
	Mode       string `json:"mode"`
	ModifiedAt string `json:"modified_at"`
	IsSymlink  bool   `json:"is_symlink"`
}

// FileContent represents file content.
type FileContent struct {
	Path     string `json:"path"`
	Content  string `json:"content"`
	Encoding string `json:"encoding"`
}

// SearchResult represents a search result.
type SearchResult struct {
	Path  string `json:"path"`
	Name  string `json:"name"`
	IsDir bool   `json:"is_dir"`
	Size  int64  `json:"size"`
	Match string `json:"match,omitempty"`
}

// Manager manages file operations within a sandboxed base path.
type Manager struct {
	basePath string
	// ponytail: global lock on structural mutations.
	// FS panel is low-QPS; per-path locks if throughput ever matters.
	mu sync.Mutex
}

// BasePath returns the base path of the file manager.
func (m *Manager) BasePath() string {
	return m.basePath
}

// NewManager creates a new file Manager with a required base path.
func NewManager(basePath string) (*Manager, error) {
	if basePath == "" {
		return nil, fmt.Errorf("filemanager base_path is required")
	}

	// Expand ~ to home directory
	if basePath == "~" || strings.HasPrefix(basePath, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("get home directory: %w", err)
		}
		basePath = home + basePath[1:]
	}

	absBase, err := filepath.Abs(basePath)
	if err != nil {
		return nil, fmt.Errorf("invalid base_path: %w", err)
	}

	// Resolve symlinks for the base path itself so subpath checking is correct
	resolvedBase, err := filepath.EvalSymlinks(absBase)
	if err == nil {
		absBase = resolvedBase
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("resolve base_path symlinks: %w", err)
	}

	return &Manager{
		basePath: absBase,
	}, nil
}

// ValidatePath checks if path is safe (no path traversal, no symlink escape).
func (m *Manager) ValidatePath(path string) (string, error) {
	if strings.Contains(path, "\x00") {
		return "", fmt.Errorf("invalid path: contains null byte")
	}
	cleanPath := filepath.Clean(path)
	absBase := m.basePath
	var absPath string

	if filepath.IsAbs(cleanPath) {
		// Convert absolute path to sandbox path
		// e.g., basePath="/home/user", input="/etc/passwd" -> "/home/user/etc/passwd"
		absPath = filepath.Join(absBase, cleanPath)
	} else {
		absPath = filepath.Join(absBase, cleanPath)
	}

	// Resolve symlinks by climbing up to the first existing parent directory.
	checkPath := absPath
	var resolvedPath string
	for {
		resolved, err := filepath.EvalSymlinks(checkPath)
		if err == nil {
			// Found the closest existing path.
			rel, err := filepath.Rel(checkPath, absPath)
			if err != nil {
				return "", fmt.Errorf("calculate relative path: %w", err)
			}
			resolvedPath = filepath.Join(resolved, rel)
			break
		}
		if !os.IsNotExist(err) {
			return "", fmt.Errorf("cannot resolve path: %w", err)
		}

		parent := filepath.Dir(checkPath)
		if parent == checkPath {
			// Reached system root directory and it still doesn't exist
			resolvedPath = filepath.Clean(absPath)
			break
		}
		checkPath = parent
	}

	if !isSubPath(absBase, resolvedPath) {
		return "", fmt.Errorf("path traversal detected: path escapes base directory")
	}

	return resolvedPath, nil
}

// validateRealPath checks that an already-resolved filesystem path stays in basePath.
// Use this for internal callers (e.g. filepath.Walk callbacks) that already hold a
// real absolute path. Do NOT route Walk paths through ValidatePath — that one treats
// absolute input as user-facing sandbox-relative and would double-Join the basePath,
// silently turning the symlink-escape check into dead code.
func (m *Manager) validateRealPath(realPath string) error {
	if strings.Contains(realPath, "\x00") {
		return fmt.Errorf("invalid path: contains null byte")
	}
	resolved, err := filepath.EvalSymlinks(realPath)
	if err != nil {
		return fmt.Errorf("resolve symlinks: %w", err)
	}
	if !isSubPath(m.basePath, resolved) {
		return fmt.Errorf("path traversal detected: path escapes base directory")
	}
	return nil
}

// ListRoot returns files in the base directory.
func (m *Manager) ListRoot() ([]FileEntry, error) {
	return m.listDir(m.basePath)
}

// List returns files in a directory.
func (m *Manager) List(path string) ([]FileEntry, error) {
	validPath, err := m.ValidatePath(path)
	if err != nil {
		return nil, err
	}
	return m.listDir(validPath)
}

// toRelativePath converts an absolute path to a relative path (relative to basePath)
func (m *Manager) toRelativePath(absolutePath string) string {
	if absolutePath == m.basePath {
		return "/"
	}
	baseSep := m.basePath
	if !strings.HasSuffix(baseSep, string(filepath.Separator)) {
		baseSep += string(filepath.Separator)
	}
	if strings.HasPrefix(absolutePath, baseSep) {
		rel := absolutePath[len(baseSep):]
		return "/" + filepath.ToSlash(rel)
	}
	return absolutePath
}

func (m *Manager) listDir(dirPath string) ([]FileEntry, error) {
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return nil, fmt.Errorf("readdir %s: %w", dirPath, err)
	}

	var files []FileEntry
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			log.Printf("filemanager: info error for %s: %v", entry.Name(), err)
			continue
		}

		fullPath := filepath.Join(dirPath, entry.Name())
		files = append(files, FileEntry{
			Name:       entry.Name(),
			Path:       m.toRelativePath(fullPath),
			IsDir:      info.IsDir(),
			SizeBytes:  info.Size(),
			Mode:       info.Mode().String(),
			ModifiedAt: info.ModTime().Format("2006-01-02T15:04:05Z"),
			IsSymlink:  entry.Type()&os.ModeSymlink != 0,
		})
	}

	return files, nil
}

// Mkdir creates a directory.
func (m *Manager) Mkdir(path string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	validPath, err := m.ValidatePath(path)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(validPath, 0755); err != nil {
		return fmt.Errorf("mkdir %s: %w", path, err)
	}
	return nil
}

const maxReadFileSize = 10 * 1024 * 1024

// ReadContent reads file content. Rejects files larger than 10MB.
func (m *Manager) ReadContent(path string) (*FileContent, error) {
	validPath, err := m.ValidatePath(path)
	if err != nil {
		return nil, err
	}

	info, err := os.Stat(validPath)
	if err != nil {
		return nil, fmt.Errorf("stat %s: %w", path, err)
	}

	if info.IsDir() {
		return nil, fmt.Errorf("cannot read content of a directory")
	}

	if info.Size() > maxReadFileSize {
		return nil, fmt.Errorf("file too large (%d bytes), max %d bytes", info.Size(), maxReadFileSize)
	}

	// O_NOFOLLOW: refuse to read through a symlink — closes TOCTOU between
	// ValidatePath's EvalSymlinks and the actual read.
	f, err := os.OpenFile(validPath, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	data, err := io.ReadAll(f)
	f.Close()
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}

	return &FileContent{
		Path:     path,
		Content:  string(data),
		Encoding: "utf-8",
	}, nil
}

// WriteContent writes content to a file.
func (m *Manager) WriteContent(path, content string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	validPath, err := m.ValidatePath(path)
	if err != nil {
		return err
	}

	f, err := os.OpenFile(validPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC|syscall.O_NOFOLLOW, 0644)
	if err != nil {
		return fmt.Errorf("write open %s: %w", path, err)
	}
	defer f.Close()

	if _, err := f.Write([]byte(content)); err != nil {
		return fmt.Errorf("write content %s: %w", path, err)
	}
	return nil
}

// Upload writes content from a reader to a file.
func (m *Manager) Upload(src io.Reader, path string) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	validPath, err := m.ValidatePath(path)
	if err != nil {
		return 0, err
	}

	flags := os.O_CREATE | os.O_WRONLY | os.O_TRUNC | syscall.O_NOFOLLOW
	dst, err := os.OpenFile(validPath, flags, 0644)
	if err != nil {
		if os.IsExist(err) || strings.Contains(err.Error(), "not a directory") {
			return 0, fmt.Errorf("upload %s: target is a symlink, refused for security", path)
		}
		return 0, fmt.Errorf("create %s: %w", path, err)
	}
	defer dst.Close()

	n, err := io.Copy(dst, src)
	if err != nil {
		return n, fmt.Errorf("upload %s: %w", path, err)
	}
	return n, nil
}

// Delete deletes a file or directory.
func (m *Manager) Delete(path string, recursive bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	validPath, err := m.ValidatePath(path)
	if err != nil {
		return err
	}

	// Prevent deleting the base path itself
	if validPath == m.basePath {
		return fmt.Errorf("cannot delete the root data directory")
	}

	// Prevent deleting parent of base path
	if strings.HasPrefix(m.basePath, validPath+string(os.PathSeparator)) {
		return fmt.Errorf("cannot delete a parent of the data directory")
	}

	if recursive {
		if err := os.RemoveAll(validPath); err != nil {
			return fmt.Errorf("remove %s: %w", path, err)
		}
		return nil
	}

	if err := os.Remove(validPath); err != nil {
		return fmt.Errorf("remove %s: %w", path, err)
	}
	return nil
}

// Rename renames/moves a file.
func (m *Manager) Rename(oldPath, newPath string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	validOld, err := m.ValidatePath(oldPath)
	if err != nil {
		return err
	}

	validNew, err := m.ValidatePath(newPath)
	if err != nil {
		return err
	}

	if err := os.Rename(validOld, validNew); err != nil {
		return fmt.Errorf("rename %s -> %s: %w", oldPath, newPath, err)
	}
	return nil
}

// Copy copies a file.
func (m *Manager) Copy(src, dst string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	validSrc, err := m.ValidatePath(src)
	if err != nil {
		return err
	}

	validDst, err := m.ValidatePath(dst)
	if err != nil {
		return err
	}

	if validSrc == validDst {
		return fmt.Errorf("source and destination are the same")
	}

	srcFile, err := os.OpenFile(validSrc, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return fmt.Errorf("open source %s: %w", src, err)
	}
	defer srcFile.Close()

	srcInfo, err := srcFile.Stat()
	if err != nil {
		return fmt.Errorf("stat source %s: %w", src, err)
	}

	if srcInfo.IsDir() {
		return fmt.Errorf("copying directories is not supported")
	}

	// Use O_NOFOLLOW to prevent symlink attacks (TOCTOU)
	dstFile, err := os.OpenFile(validDst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC|syscall.O_NOFOLLOW, 0644)
	if err != nil {
		return fmt.Errorf("create dest %s: %w", dst, err)
	}
	defer dstFile.Close()

	if _, err := io.Copy(dstFile, srcFile); err != nil {
		return fmt.Errorf("copy %s -> %s: %w", src, dst, err)
	}
	return nil
}

// Move moves files to a destination directory.
func (m *Manager) Move(paths []string, dest string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	validDest, err := m.ValidatePath(dest)
	if err != nil {
		return err
	}

	validPaths := make([]string, len(paths))
	for i, path := range paths {
		validPath, err := m.ValidatePath(path)
		if err != nil {
			return err
		}
		validPaths[i] = validPath
	}

	if err := os.MkdirAll(validDest, 0755); err != nil {
		return fmt.Errorf("create dest %s: %w", dest, err)
	}

	for _, validPath := range validPaths {
		filename := filepath.Base(validPath)
		newPath := filepath.Join(validDest, filename)

		if err := os.Rename(validPath, newPath); err != nil {
			return fmt.Errorf("move %s -> %s: %w", validPath, newPath, err)
		}
	}

	return nil
}

// isSubPath checks if childPath is under parentPath (or is equal to it).
// Both parentPath and childPath should be cleaned before calling if needed.
func isSubPath(parentPath, childPath string) bool {
	cleanParent := filepath.Clean(parentPath)
	cleanChild := filepath.Clean(childPath)
	if cleanParent == cleanChild {
		return true
	}
	if !strings.HasSuffix(cleanParent, string(filepath.Separator)) {
		cleanParent += string(filepath.Separator)
	}
	return strings.HasPrefix(cleanChild, cleanParent)
}

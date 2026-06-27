package filemanager

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
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
	if basePath == "/" {
		return nil, fmt.Errorf("filemanager base_path cannot be '/' for security reasons")
	}

	absBase, err := filepath.Abs(basePath)
	if err != nil {
		return nil, fmt.Errorf("invalid base_path: %w", err)
	}

	return &Manager{
		basePath: absBase,
	}, nil
}

// ValidatePath checks if path is safe (no path traversal, no symlink escape).
func (m *Manager) ValidatePath(path string) (string, error) {
	cleanPath := filepath.Clean(path)

	if filepath.IsAbs(cleanPath) {
		return "", fmt.Errorf("absolute paths are not allowed, use relative paths")
	}

	absBase := m.basePath
	absPath := filepath.Join(absBase, cleanPath)

	resolvedPath, err := filepath.EvalSymlinks(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			parent := filepath.Dir(absPath)
			resolvedParent, err := filepath.EvalSymlinks(parent)
			if err != nil {
				return "", fmt.Errorf("cannot resolve path: %w", err)
			}
			resolvedPath = filepath.Join(resolvedParent, filepath.Base(absPath))
		} else {
			return "", fmt.Errorf("cannot resolve path: %w", err)
		}
	}

	basePrefix := absBase
	if !strings.HasSuffix(basePrefix, string(filepath.Separator)) {
		basePrefix += string(filepath.Separator)
	}

	if resolvedPath != absBase && !strings.HasPrefix(resolvedPath, basePrefix) {
		return "", fmt.Errorf("path traversal detected: path escapes base directory")
	}

	return resolvedPath, nil
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
			Path:       fullPath,
			IsDir:      entry.IsDir(),
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

	data, err := os.ReadFile(validPath)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}

	return &FileContent{
		Path:     validPath,
		Content:  string(data),
		Encoding: "utf-8",
	}, nil
}

// WriteContent writes content to a file.
func (m *Manager) WriteContent(path, content string) error {
	validPath, err := m.ValidatePath(path)
	if err != nil {
		return err
	}

	if err := os.WriteFile(validPath, []byte(content), 0644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

// Upload writes content from a reader to a file.
func (m *Manager) Upload(src io.Reader, path string) (int64, error) {
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

	srcFile, err := os.Open(validSrc)
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

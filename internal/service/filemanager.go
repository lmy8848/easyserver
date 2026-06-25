package service

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

type FileEntry struct {
	Name        string `json:"name"`
	Path        string `json:"path"`
	IsDir       bool   `json:"is_dir"`
	SizeBytes   int64  `json:"size_bytes"`
	Mode        string `json:"mode"`
	ModifiedAt  string `json:"modified_at"`
	IsSymlink   bool   `json:"is_symlink"`
}

type FileContent struct {
	Path     string `json:"path"`
	Content  string `json:"content"`
	Encoding string `json:"encoding"`
}

type FileManager struct {
	basePath string // Root path for security (required)
}

// BasePath returns the base path of the file manager
func (m *FileManager) BasePath() string {
	return m.basePath
}

// NewFileManager creates a new FileManager with a required base path
// Returns error if basePath is empty or "/" (root)
func NewFileManager(basePath string) (*FileManager, error) {
	if basePath == "" {
		return nil, fmt.Errorf("filemanager base_path is required")
	}
	if basePath == "/" {
		return nil, fmt.Errorf("filemanager base_path cannot be '/' for security reasons")
	}

	// Clean and resolve the base path
	absBase, err := filepath.Abs(basePath)
	if err != nil {
		return nil, fmt.Errorf("invalid base_path: %w", err)
	}

	return &FileManager{
		basePath: absBase,
	}, nil
}

// ValidatePath checks if path is safe (no path traversal, no symlink escape)
func (m *FileManager) ValidatePath(path string) (string, error) {
	// Clean the path
	cleanPath := filepath.Clean(path)

	// Reject absolute paths from user input - only relative paths allowed
	if filepath.IsAbs(cleanPath) {
		return "", fmt.Errorf("absolute paths are not allowed, use relative paths")
	}

	// Join basePath with the relative path to get absolute path
	absBase := m.basePath

	absPath := filepath.Join(absBase, cleanPath)

	// Resolve symlinks to prevent symlink attacks
	resolvedPath, err := filepath.EvalSymlinks(absPath)
	if err != nil {
		// If file doesn't yet, check parent directory
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

	// Ensure base path ends with separator for proper prefix check
	basePrefix := absBase
	if !strings.HasSuffix(basePrefix, string(filepath.Separator)) {
		basePrefix += string(filepath.Separator)
	}

	// Check if resolved path is within base
	if resolvedPath != absBase && !strings.HasPrefix(resolvedPath, basePrefix) {
		return "", fmt.Errorf("path traversal detected: path escapes base directory")
	}

	return resolvedPath, nil
}

// ListRoot returns files in the base directory (no path validation needed)
func (m *FileManager) ListRoot() ([]FileEntry, error) {
	return m.listDir(m.basePath)
}

// List returns files in a directory
func (m *FileManager) List(path string) ([]FileEntry, error) {
	validPath, err := m.ValidatePath(path)
	if err != nil {
		return nil, err
	}
	return m.listDir(validPath)
}

// listDir reads directory entries (common logic)
func (m *FileManager) listDir(dirPath string) ([]FileEntry, error) {
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return nil, fmt.Errorf("readdir %s: %w", dirPath, err)
	}

	var files []FileEntry
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			// Log but continue - entry may have been deleted between readdir and info
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

// Mkdir creates a directory
func (m *FileManager) Mkdir(path string) error {
	validPath, err := m.ValidatePath(path)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(validPath, 0755); err != nil {
		return fmt.Errorf("mkdir %s: %w", path, err)
	}
	return nil
}

// maxReadFileSize is the maximum file size for ReadContent (10MB)
const maxReadFileSize = 10 * 1024 * 1024

// ReadContent reads file content. Rejects files larger than 10MB to prevent OOM.
func (m *FileManager) ReadContent(path string) (*FileContent, error) {
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

// WriteContent writes content to a file
func (m *FileManager) WriteContent(path, content string) error {
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
// Uses O_NOFOLLOW to prevent symlink-based TOCTOU attacks: if the final
// path component is a symlink, the open fails instead of following it.
func (m *FileManager) Upload(src io.Reader, path string) (int64, error) {
	validPath, err := m.ValidatePath(path)
	if err != nil {
		return 0, err
	}

	// Open the file with O_CREATE|O_WRONLY|O_TRUNC and O_NOFOLLOW to prevent
	// symlink race conditions between ValidatePath and file creation.
	flags := os.O_CREATE | os.O_WRONLY | os.O_TRUNC | syscall.O_NOFOLLOW
	dst, err := os.OpenFile(validPath, flags, 0644)
	if err != nil {
		// If O_NOFOLLOW fails because the target is a symlink, return a clear error
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

// Delete deletes a file or directory
func (m *FileManager) Delete(path string, recursive bool) error {
	validPath, err := m.ValidatePath(path)
	if err != nil {
		return err
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

// Rename renames/moves a file
func (m *FileManager) Rename(oldPath, newPath string) error {
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

// Copy copies a file
func (m *FileManager) Copy(src, dst string) error {
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

	dstFile, err := os.Create(validDst)
	if err != nil {
		return fmt.Errorf("create dest %s: %w", dst, err)
	}
	defer dstFile.Close()

	if _, err := io.Copy(dstFile, srcFile); err != nil {
		return fmt.Errorf("copy %s -> %s: %w", src, dst, err)
	}
	return nil
}

// Move moves files to a destination directory
func (m *FileManager) Move(paths []string, dest string) error {
	validDest, err := m.ValidatePath(dest)
	if err != nil {
		return err
	}

	// Validate all paths first
	validPaths := make([]string, len(paths))
	for i, path := range paths {
		validPath, err := m.ValidatePath(path)
		if err != nil {
			return err
		}
		validPaths[i] = validPath
	}

	// Ensure destination exists
	if err := os.MkdirAll(validDest, 0755); err != nil {
		return fmt.Errorf("create dest %s: %w", dest, err)
	}

	// Execute moves
	for _, validPath := range validPaths {
		filename := filepath.Base(validPath)
		newPath := filepath.Join(validDest, filename)

		if err := os.Rename(validPath, newPath); err != nil {
			return fmt.Errorf("move %s -> %s: %w", validPath, newPath, err)
		}
	}

	return nil
}

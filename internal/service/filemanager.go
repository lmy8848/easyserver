package service

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
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

	// Join basePath with the relative path to get absolute path
	absBase := m.basePath

	// If path is relative, join with basePath
	var absPath string
	if filepath.IsAbs(cleanPath) {
		absPath = cleanPath
	} else {
		absPath = filepath.Join(absBase, cleanPath)
	}

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

// List returns files in a directory
func (m *FileManager) List(path string) ([]FileEntry, error) {
	validPath, err := m.ValidatePath(path)
	if err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(validPath)
	if err != nil {
		return nil, err
	}

	var files []FileEntry
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			continue
		}

		fullPath := filepath.Join(validPath, entry.Name())
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

	return os.MkdirAll(validPath, 0755)
}

// ReadContent reads file content
func (m *FileManager) ReadContent(path string) (*FileContent, error) {
	validPath, err := m.ValidatePath(path)
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(validPath)
	if err != nil {
		return nil, err
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

	return os.WriteFile(validPath, []byte(content), 0644)
}

// Upload writes content from a reader to a file
func (m *FileManager) Upload(src io.Reader, path string) (int64, error) {
	validPath, err := m.ValidatePath(path)
	if err != nil {
		return 0, err
	}

	dst, err := os.Create(validPath)
	if err != nil {
		return 0, err
	}
	defer dst.Close()

	return io.Copy(dst, src)
}

// Delete deletes a file or directory
func (m *FileManager) Delete(path string, recursive bool) error {
	validPath, err := m.ValidatePath(path)
	if err != nil {
		return err
	}

	if recursive {
		return os.RemoveAll(validPath)
	}

	return os.Remove(validPath)
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

	return os.Rename(validOld, validNew)
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
		return err
	}
	defer srcFile.Close()

	srcInfo, err := srcFile.Stat()
	if err != nil {
		return err
	}

	if srcInfo.IsDir() {
		return fmt.Errorf("copying directories is not supported")
	}

	dstFile, err := os.Create(validDst)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	_, err = io.Copy(dstFile, srcFile)
	return err
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
		return err
	}

	// Execute moves
	for _, validPath := range validPaths {
		filename := filepath.Base(validPath)
		newPath := filepath.Join(validDest, filename)

		if err := os.Rename(validPath, newPath); err != nil {
			return err
		}
	}

	return nil
}

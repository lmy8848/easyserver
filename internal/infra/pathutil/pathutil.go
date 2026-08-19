package pathutil

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"easyserver/internal/infra/errx"
)

// ErrInvalidPath indicates an invalid or malicious path input.
var ErrInvalidPath = errors.New("invalid path")

// IsTraversal checks if a raw path contains directory traversal sequences or null bytes.
func IsTraversal(path string) bool {
	return strings.Contains(path, "\x00") || strings.Contains(path, "..")
}

// ValidateFilename checks if a filename is a valid single component without path separators or traversal.
func ValidateFilename(name string) error {
	if name == "" {
		return errors.New("filename cannot be empty")
	}
	if strings.Contains(name, "\x00") || strings.Contains(name, "/") || strings.Contains(name, "\\") || strings.Contains(name, "..") {
		return errors.New("invalid filename: contains path separator or traversal")
	}
	clean := filepath.Clean(name)
	if filepath.Base(clean) != clean || clean == "." || clean == ".." {
		return errors.New("invalid filename: not a single path component")
	}
	return nil
}

// HasPathPrefix checks if target path starts with baseDir or equals baseDir (after cleaning).
func HasPathPrefix(target, baseDir string) bool {
	cleanTarget := filepath.Clean(target)
	cleanBase := filepath.Clean(baseDir)
	if cleanTarget == cleanBase {
		return true
	}
	baseWithSep := cleanBase
	if !strings.HasSuffix(baseWithSep, string(filepath.Separator)) {
		baseWithSep += string(filepath.Separator)
	}
	return strings.HasPrefix(cleanTarget, baseWithSep)
}

// IsSubPath checks if childPath is equal to parentPath or is located strictly within parentPath.
func IsSubPath(parentPath, childPath string) bool {
	cleanParent := filepath.Clean(parentPath)
	cleanChild := filepath.Clean(childPath)
	if cleanParent == cleanChild {
		return true
	}
	if !HasPathPrefix(cleanChild, cleanParent) {
		return false
	}
	rel, err := filepath.Rel(cleanParent, cleanChild)
	if err != nil || strings.HasPrefix(rel, "..") || rel == ".." {
		return false
	}
	return true
}

// JoinSafe safely joins subpaths under baseDir, ensuring the resulting path cannot escape baseDir.
func JoinSafe(baseDir string, subpaths ...string) (string, error) {
	for _, sub := range subpaths {
		if strings.Contains(sub, "\x00") {
			return "", errors.New("invalid path: contains null byte")
		}
	}
	cleanBase := filepath.Clean(baseDir)
	joined := filepath.Join(append([]string{cleanBase}, subpaths...)...)
	if !HasPathPrefix(joined, cleanBase) || !IsSubPath(cleanBase, joined) {
		return "", errx.Forbidden("path traversal detected: path escapes base directory")
	}
	return joined, nil
}

// ResolveInSandbox resolves a user-provided relative or absolute path inside a sandbox basePath.
// It normalizes path representations, climbs existing parent directories to resolve symlinks,
// and guarantees the target stays within basePath.
func ResolveInSandbox(basePath, userPath string) (string, error) {
	if strings.Contains(userPath, "\x00") {
		return "", errors.New("invalid path: contains null byte")
	}
	absBase := filepath.Clean(basePath)
	if userPath == "" || userPath == "." || userPath == "/" {
		return absBase, nil
	}

	cleanPath := filepath.Clean(userPath)
	if strings.HasPrefix(cleanPath, "..") {
		return "", errx.Forbidden("path traversal detected: path escapes base directory")
	}

	var absPath string
	if filepath.IsAbs(cleanPath) {
		// Treat leading "/" as relative to sandbox base
		absPath = filepath.Join(absBase, strings.TrimPrefix(cleanPath, "/"))
	} else {
		absPath = filepath.Join(absBase, cleanPath)
	}

	rel, err := filepath.Rel(absBase, absPath)
	if err != nil || strings.HasPrefix(rel, "..") || rel == ".." {
		return "", errx.Forbidden("path traversal detected: path escapes base directory")
	}

	// Resolve symlinks by climbing up to the first existing parent directory
	checkPath := absPath
	var resolvedPath string
	for {
		resolved, err := filepath.EvalSymlinks(checkPath)
		if err == nil {
			relSub, err := filepath.Rel(checkPath, absPath)
			if err != nil {
				return "", fmt.Errorf("calculate relative path: %w", err)
			}
			resolvedPath = filepath.Join(resolved, relSub)
			break
		}
		if !os.IsNotExist(err) {
			return "", errx.Forbidden("路径解析失败，拒绝访问: %w", err)
		}

		parent := filepath.Dir(checkPath)
		if parent == checkPath {
			resolvedPath = filepath.Clean(absPath)
			break
		}
		checkPath = parent
	}

	cleanResolved := filepath.Clean(resolvedPath)
	if !HasPathPrefix(cleanResolved, absBase) || !IsSubPath(absBase, cleanResolved) {
		return "", errx.Forbidden("path traversal detected: path escapes base directory")
	}

	return cleanResolved, nil
}

// ValidateRealPath verifies that an already-resolved filesystem path (e.g. from filepath.Walk)
// stays within basePath after evaluating symlinks.
func ValidateRealPath(basePath, realPath string) error {
	if strings.Contains(realPath, "\x00") {
		return errors.New("invalid path: contains null byte")
	}
	resolved, err := filepath.EvalSymlinks(realPath)
	if err != nil {
		return fmt.Errorf("resolve symlinks: %w", err)
	}
	cleanResolved := filepath.Clean(resolved)
	absBase := filepath.Clean(basePath)
	if !HasPathPrefix(cleanResolved, absBase) || !IsSubPath(absBase, cleanResolved) {
		return errx.Forbidden("path traversal detected: path escapes base directory")
	}
	return nil
}

// ResolveShareSubpath anchors a subpath under shareRoot (a validated directory) and
// confirms the target path stays within both basePath and shareRoot after symlink evaluation.
func ResolveShareSubpath(basePath, shareRoot, subpath string) (string, error) {
	if IsTraversal(subpath) {
		return "", errors.New("invalid path: contains null byte or parent reference")
	}
	cleanSub := filepath.Clean(filepath.Join("/", subpath))
	if cleanSub == "/" {
		cleanSub = ""
	} else {
		cleanSub = strings.TrimPrefix(cleanSub, "/")
	}

	target := shareRoot
	if cleanSub != "" {
		target = filepath.Join(shareRoot, cleanSub)
	}

	resolved, err := filepath.EvalSymlinks(target)
	if err != nil {
		return "", err
	}
	cleanResolved := filepath.Clean(resolved)
	absBase := filepath.Clean(basePath)
	if !HasPathPrefix(cleanResolved, absBase) || !IsSubPath(absBase, cleanResolved) {
		return "", errx.Forbidden("path traversal detected: path escapes base directory")
	}

	cleanShareRoot := filepath.Clean(shareRoot)
	if !HasPathPrefix(cleanResolved, cleanShareRoot) || !IsSubPath(cleanShareRoot, cleanResolved) {
		return "", errx.Forbidden("path traversal detected: path escapes share root")
	}

	return target, nil
}

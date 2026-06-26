package filemanager

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

var errSearchLimit = fmt.Errorf("search result limit reached")

// Search searches for files by name pattern.
func (m *Manager) Search(rootPath, pattern string, maxResults int) ([]SearchResult, error) {
	validPath, err := m.ValidatePath(rootPath)
	if err != nil {
		return nil, err
	}

	if maxResults <= 0 || maxResults > 1000 {
		maxResults = 100
	}

	var results []SearchResult
	pattern = strings.ToLower(pattern)

	err = filepath.Walk(validPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}

		if _, vErr := m.ValidatePath(path); vErr != nil {
			return filepath.SkipDir
		}

		if len(results) >= maxResults {
			return errSearchLimit
		}

		name := strings.ToLower(info.Name())
		if strings.Contains(name, pattern) {
			results = append(results, SearchResult{
				Path:  path,
				Name:  info.Name(),
				IsDir: info.IsDir(),
				Size:  info.Size(),
				Match: "name",
			})
		}

		return nil
	})

	if err != nil && err != errSearchLimit {
		return results, fmt.Errorf("search walk: %w", err)
	}

	return results, nil
}

var binaryExtensions = map[string]bool{
	".jpg": true, ".jpeg": true, ".png": true, ".gif": true,
	".bmp": true, ".ico": true, ".svg": true, ".webp": true,
	".mp3": true, ".mp4": true, ".avi": true, ".mov": true,
	".zip": true, ".tar": true, ".gz": true, ".rar": true,
	".exe": true, ".dll": true, ".so": true, ".dylib": true,
	".pdf": true, ".doc": true, ".docx": true, ".xls": true,
}

// SearchContent searches for files containing the specified text.
func (m *Manager) SearchContent(rootPath, text string, maxResults int) ([]SearchResult, error) {
	validPath, err := m.ValidatePath(rootPath)
	if err != nil {
		return nil, err
	}

	if maxResults <= 0 || maxResults > 100 {
		maxResults = 50
	}

	var results []SearchResult
	text = strings.ToLower(text)

	err = filepath.Walk(validPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}

		if _, vErr := m.ValidatePath(path); vErr != nil {
			return filepath.SkipDir
		}

		if len(results) >= maxResults {
			return errSearchLimit
		}

		if info.IsDir() || info.Size() > 10*1024*1024 {
			return nil
		}

		ext := strings.ToLower(filepath.Ext(info.Name()))
		if binaryExtensions[ext] {
			return nil
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}

		content := strings.ToLower(string(data))
		if strings.Contains(content, text) {
			results = append(results, SearchResult{
				Path:  path,
				Name:  info.Name(),
				IsDir: false,
				Size:  info.Size(),
				Match: "content",
			})
		}

		return nil
	})

	if err != nil && err != errSearchLimit {
		return results, fmt.Errorf("content search walk: %w", err)
	}

	return results, nil
}

// Compress creates a zip archive.
func (m *Manager) Compress(sourcePaths []string, destPath string) error {
	validDest, err := m.ValidatePath(destPath)
	if err != nil {
		return err
	}

	if !strings.HasSuffix(validDest, ".zip") {
		validDest += ".zip"
	}

	zipFile, err := os.Create(validDest)
	if err != nil {
		return err
	}
	defer zipFile.Close()

	zipWriter := zip.NewWriter(zipFile)
	defer zipWriter.Close()

	for _, sourcePath := range sourcePaths {
		validSource, err := m.ValidatePath(sourcePath)
		if err != nil {
			return err
		}

		err = filepath.Walk(validSource, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}

			if _, vErr := m.ValidatePath(path); vErr != nil {
				return filepath.SkipDir
			}

			header, err := zip.FileInfoHeader(info)
			if err != nil {
				return err
			}

			header.Method = zip.Deflate

			relPath, err := filepath.Rel(filepath.Dir(validSource), path)
			if err != nil {
				return err
			}
			header.Name = relPath

			writer, err := zipWriter.CreateHeader(header)
			if err != nil {
				return err
			}

			if info.IsDir() {
				return nil
			}

			file, err := os.Open(path)
			if err != nil {
				return err
			}
			_, err = io.Copy(writer, file)
			file.Close()
			return err
		})

		if err != nil {
			return err
		}
	}

	return nil
}

// Extract extracts a zip or tar.gz archive.
func (m *Manager) Extract(archivePath, destPath string) error {
	validArchive, err := m.ValidatePath(archivePath)
	if err != nil {
		return err
	}

	validDest, err := m.ValidatePath(destPath)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(validDest, 0755); err != nil {
		return err
	}

	if strings.HasSuffix(validArchive, ".tar.gz") || strings.HasSuffix(validArchive, ".tgz") {
		return m.extractTarGz(validArchive, validDest)
	}

	ext := strings.ToLower(filepath.Ext(validArchive))
	switch ext {
	case ".zip":
		return m.extractZip(validArchive, validDest)
	case ".gz":
		return m.extractGzip(validArchive, validDest)
	default:
		return fmt.Errorf("unsupported archive format: %s", ext)
	}
}

func (m *Manager) extractZip(zipPath, destPath string) error {
	reader, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer reader.Close()

	for _, file := range reader.File {
		path := filepath.Join(destPath, file.Name)

		if !strings.HasPrefix(filepath.Clean(path), filepath.Clean(destPath)) {
			return fmt.Errorf("invalid file path: %s", file.Name)
		}

		if filepath.IsAbs(file.Name) {
			return fmt.Errorf("absolute path not allowed in archive: %s", file.Name)
		}

		if strings.Contains(file.Name, "..") {
			return fmt.Errorf("path traversal not allowed in archive: %s", file.Name)
		}

		if file.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("symlinks not allowed in archive: %s", file.Name)
		}

		if file.FileInfo().IsDir() {
			if err := os.MkdirAll(path, 0755); err != nil {
				return err
			}
			continue
		}

		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return err
		}

		outFile, err := os.Create(path)
		if err != nil {
			return err
		}

		inFile, err := file.Open()
		if err != nil {
			outFile.Close()
			return err
		}

		_, err = io.Copy(outFile, inFile)
		inFile.Close()
		outFile.Close()
		if err != nil {
			return err
		}
	}

	return nil
}

func (m *Manager) extractTarGz(tarPath, destPath string) error {
	file, err := os.Open(tarPath)
	if err != nil {
		return err
	}
	defer file.Close()

	gzReader, err := gzip.NewReader(file)
	if err != nil {
		return err
	}
	defer gzReader.Close()

	tarReader := tar.NewReader(gzReader)

	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		path := filepath.Join(destPath, header.Name)

		if !strings.HasPrefix(filepath.Clean(path), filepath.Clean(destPath)) {
			return fmt.Errorf("invalid file path: %s", header.Name)
		}

		if filepath.IsAbs(header.Name) {
			return fmt.Errorf("absolute path not allowed in archive: %s", header.Name)
		}

		if strings.Contains(header.Name, "..") {
			return fmt.Errorf("path traversal not allowed in archive: %s", header.Name)
		}

		switch header.Typeflag {
		case tar.TypeSymlink:
			return fmt.Errorf("symlinks not allowed in archive: %s", header.Name)
		case tar.TypeLink:
			return fmt.Errorf("hard links not allowed in archive: %s", header.Name)
		case tar.TypeDir:
			if err := os.MkdirAll(path, 0755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
				return err
			}

			outFile, err := os.Create(path)
			if err != nil {
				return err
			}

			if _, err := io.Copy(outFile, tarReader); err != nil {
				outFile.Close()
				return err
			}
			outFile.Close()
		}
	}

	return nil
}

func (m *Manager) extractGzip(gzPath, destPath string) error {
	file, err := os.Open(gzPath)
	if err != nil {
		return err
	}
	defer file.Close()

	gzReader, err := gzip.NewReader(file)
	if err != nil {
		return err
	}
	defer gzReader.Close()

	outPath := strings.TrimSuffix(destPath, ".gz")
	if outPath == destPath {
		outPath = destPath + ".extracted"
	}

	outFile, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer outFile.Close()

	_, err = io.Copy(outFile, gzReader)
	return err
}

// Chmod changes file permissions.
func (m *Manager) Chmod(path string, mode os.FileMode) error {
	validPath, err := m.ValidatePath(path)
	if err != nil {
		return err
	}

	if mode&(os.ModeSetuid|os.ModeSetgid|os.ModeSticky) != 0 {
		return fmt.Errorf("setuid/setgid/sticky bits are not allowed")
	}

	if mode.Perm()&0002 != 0 {
		return fmt.Errorf("world-writable permissions (o+w) are not allowed")
	}

	return os.Chmod(validPath, mode)
}

// Chown changes file ownership.
func (m *Manager) Chown(path string, uid, gid int) error {
	validPath, err := m.ValidatePath(path)
	if err != nil {
		return err
	}

	return os.Chown(validPath, uid, gid)
}

// GetFileDetails returns detailed file information.
func (m *Manager) GetFileDetails(path string) (map[string]interface{}, error) {
	validPath, err := m.ValidatePath(path)
	if err != nil {
		return nil, err
	}

	info, err := os.Stat(validPath)
	if err != nil {
		return nil, fmt.Errorf("stat %s: %w", path, err)
	}

	result := map[string]interface{}{
		"name":        info.Name(),
		"path":        validPath,
		"is_dir":      info.IsDir(),
		"size_bytes":  info.Size(),
		"mode":        info.Mode().String(),
		"mode_octal":  fmt.Sprintf("%04o", info.Mode().Perm()),
		"modified_at": info.ModTime().Format("2006-01-02T15:04:05Z"),
		"is_symlink":  info.Mode()&os.ModeSymlink != 0,
	}

	if sys := info.Sys(); sys != nil {
		if stat, ok := sys.(*syscall.Stat_t); ok {
			result["uid"] = stat.Uid
			result["gid"] = stat.Gid
			result["nlink"] = stat.Nlink
		}
	}

	return result, nil
}

// GetDiskUsage returns disk usage information.
func (m *Manager) GetDiskUsage(path string) (map[string]interface{}, error) {
	validPath, err := m.ValidatePath(path)
	if err != nil {
		return nil, err
	}

	var stat syscall.Statfs_t
	if err := syscall.Statfs(validPath, &stat); err != nil {
		return nil, err
	}

	total := stat.Blocks * uint64(stat.Bsize)
	free := stat.Bfree * uint64(stat.Bsize)
	used := total - free

	var usagePercent float64
	if total > 0 {
		usagePercent = float64(used) / float64(total) * 100
	}

	return map[string]interface{}{
		"total_bytes":   total,
		"used_bytes":    used,
		"free_bytes":    free,
		"usage_percent": usagePercent,
	}, nil
}

var mimeTypes = map[string]string{
	".html": "text/html",
	".css":  "text/css",
	".js":   "application/javascript",
	".json": "application/json",
	".xml":  "application/xml",
	".txt":  "text/plain",
	".md":   "text/markdown",
	".csv":  "text/csv",
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".png":  "image/png",
	".gif":  "image/gif",
	".svg":  "image/svg+xml",
	".webp": "image/webp",
	".ico":  "image/x-icon",
	".mp3":  "audio/mpeg",
	".mp4":  "video/mp4",
	".avi":  "video/x-msvideo",
	".mov":  "video/quicktime",
	".pdf":  "application/pdf",
	".zip":  "application/zip",
	".tar":  "application/x-tar",
	".gz":   "application/gzip",
	".sh":   "application/x-sh",
	".py":   "text/x-python",
	".go":   "text/x-go",
	".java": "text/x-java",
	".c":    "text/x-c",
	".cpp":  "text/x-c++",
	".h":    "text/x-c",
	".rs":   "text/x-rust",
	".rb":   "text/x-ruby",
	".php":  "text/x-php",
	".sql":  "text/x-sql",
	".yaml": "text/x-yaml",
	".yml":  "text/x-yaml",
	".toml": "text/x-toml",
	".ini":  "text/x-ini",
	".conf": "text/x-conf",
	".log":  "text/x-log",
}

// GetMimeType returns the MIME type of a file based on its extension.
func (m *Manager) GetMimeType(path string) (string, error) {
	if _, err := m.ValidatePath(path); err != nil {
		return "", err
	}

	ext := strings.ToLower(filepath.Ext(path))
	if mime, ok := mimeTypes[ext]; ok {
		return mime, nil
	}

	return "application/octet-stream", nil
}

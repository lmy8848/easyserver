package filemanager

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

var errSearchLimit = fmt.Errorf("search result limit reached")

const (
	// MinUserUID is the minimum UID/GID allowed for chown operations.
	// System users (0-999) are rejected to prevent privilege escalation.
	MinUserUID = 1000
)

// Search searches for files by name pattern.
func (m *Manager) Search(rootPath, pattern string, maxResults int) ([]SearchResult, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
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

		if vErr := m.validateRealPath(path); vErr != nil {
			return filepath.SkipDir
		}

		if len(results) >= maxResults {
			return errSearchLimit
		}

		name := strings.ToLower(info.Name())
		if strings.Contains(name, pattern) {
			results = append(results, SearchResult{
				Path:  m.toRelativePath(path),
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
	m.mu.RLock()
	defer m.mu.RUnlock()
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

		if vErr := m.validateRealPath(path); vErr != nil {
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

		// O_NOFOLLOW: refuse to read through a symlink even if EvalSymlinks said OK above
		// (TOCTOU defense — the leaf could have been swapped to a symlink in the window).
		f, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
		if err != nil {
			return nil
		}
		data, err := io.ReadAll(io.LimitReader(f, 10*1024*1024))
		f.Close()
		if err != nil {
			return nil
		}

		content := strings.ToLower(string(data))
		if strings.Contains(content, text) {
			results = append(results, SearchResult{
				Path:  m.toRelativePath(path),
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
	m.mu.Lock()
	defer m.mu.Unlock()

	validDest, err := m.ValidatePath(destPath)
	if err != nil {
		return err
	}

	if !strings.HasSuffix(validDest, ".zip") {
		validDest += ".zip"
	}

	zipFile, err := os.OpenFile(validDest, os.O_WRONLY|os.O_CREATE|os.O_TRUNC|syscall.O_NOFOLLOW, 0644)
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

			if vErr := m.validateRealPath(path); vErr != nil {
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

			// O_NOFOLLOW: don't pack the contents of a symlink target — at this point
			// validateRealPath has confirmed the resolved path is inside basePath, but
			// the entry itself could still be a symlink whose target is outside.
			file, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
			if err != nil {
				return nil
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

// archiveMode normalizes an archive entry's mode to 0755 or 0644,
// preserving the executable bit but dropping dangerous bits like setuid/setgid.
func archiveMode(m os.FileMode) os.FileMode {
	if m&0111 != 0 {
		return 0755
	}
	return 0644
}

// Extract extracts a zip or tar.gz archive.
func (m *Manager) Extract(archivePath, destPath string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

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

	var totalExtractedBytes int64
	var fileCount int
	var createdPaths []string
	var success bool

	defer func() {
		if !success {
			// Clean up created files/dirs in reverse order
			for i := len(createdPaths) - 1; i >= 0; i-- {
				if err := os.Remove(createdPaths[i]); err != nil {
					log.Printf("rollback remove failed for %s: %v", createdPaths[i], err)
				}
			}
		}
	}()

	for _, file := range reader.File {
		path := filepath.Join(destPath, file.Name)
		validPath, err := m.ValidatePath(m.toRelativePath(path))
		if err != nil || !isSubPath(destPath, validPath) {
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

		fileCount++
		if fileCount > maxExtractFileCount {
			return fmt.Errorf("archive contains too many files (max %d)", maxExtractFileCount)
		}

		if file.FileInfo().IsDir() {
			if err := m.mkdirAllWithRecord(validPath, 0755, &createdPaths); err != nil {
				return err
			}
			continue
		}

		if err := m.mkdirAllWithRecord(filepath.Dir(validPath), 0755, &createdPaths); err != nil {
			return err
		}

		outFile, err := os.OpenFile(validPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC|syscall.O_NOFOLLOW, archiveMode(file.Mode()))
		if err != nil {
			return fmt.Errorf("create file %s: %w", file.Name, err)
		}
		createdPaths = append(createdPaths, validPath)

		inFile, err := file.Open()
		if err != nil {
			outFile.Close()
			return err
		}

		lw := &limitWriter{w: outFile, total: &totalExtractedBytes, limit: maxExtractSize}
		_, err = io.Copy(lw, inFile)
		inFile.Close()
		outFile.Close()
		if err != nil {
			return err
		}
	}

	success = true
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

	var totalExtractedBytes int64
	var fileCount int
	var createdPaths []string
	var success bool

	defer func() {
		if !success {
			for i := len(createdPaths) - 1; i >= 0; i-- {
				if err := os.Remove(createdPaths[i]); err != nil {
					log.Printf("rollback remove failed for %s: %v", createdPaths[i], err)
				}
			}
		}
	}()

	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		path := filepath.Join(destPath, header.Name)
		validPath, err := m.ValidatePath(m.toRelativePath(path))
		if err != nil || !isSubPath(destPath, validPath) {
			return fmt.Errorf("invalid file path: %s", header.Name)
		}

		if filepath.IsAbs(header.Name) {
			return fmt.Errorf("absolute path not allowed in archive: %s", header.Name)
		}

		if strings.Contains(header.Name, "..") {
			return fmt.Errorf("path traversal not allowed in archive: %s", header.Name)
		}

		fileCount++
		if fileCount > maxExtractFileCount {
			return fmt.Errorf("archive contains too many files (max %d)", maxExtractFileCount)
		}

		switch header.Typeflag {
		case tar.TypeSymlink:
			return fmt.Errorf("symlinks not allowed in archive: %s", header.Name)
		case tar.TypeLink:
			return fmt.Errorf("hard links not allowed in archive: %s", header.Name)
		case tar.TypeDir:
			if err := m.mkdirAllWithRecord(validPath, 0755, &createdPaths); err != nil {
				return err
			}
		case tar.TypeReg, tar.TypeRegA:
			if err := m.mkdirAllWithRecord(filepath.Dir(validPath), 0755, &createdPaths); err != nil {
				return err
			}

			outFile, err := os.OpenFile(validPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC|syscall.O_NOFOLLOW, archiveMode(os.FileMode(header.Mode)))
			if err != nil {
				return fmt.Errorf("create file %s: %w", header.Name, err)
			}
			createdPaths = append(createdPaths, validPath)

			lw := &limitWriter{w: outFile, total: &totalExtractedBytes, limit: maxExtractSize}
			if _, err := io.Copy(lw, tarReader); err != nil {
				outFile.Close()
				return err
			}
			outFile.Close()
		}
	}

	success = true
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

	// destPath is the extraction directory (already validated by Extract). Write the
	// decompressed file inside it, using the archive basename with .gz stripped.
	baseName := strings.TrimSuffix(filepath.Base(gzPath), ".gz")
	if baseName == "" || baseName == filepath.Base(gzPath) {
		baseName = filepath.Base(gzPath) + ".extracted"
	}
	outPath := filepath.Join(destPath, baseName)

	validOutPath, err := m.ValidatePath(m.toRelativePath(outPath))
	if err != nil {
		return err
	}

	outFile, err := os.OpenFile(validOutPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC|syscall.O_NOFOLLOW, 0644)
	if err != nil {
		return err
	}
	defer outFile.Close()

	var totalExtractedBytes int64
	lw := &limitWriter{w: outFile, total: &totalExtractedBytes, limit: maxExtractSize}
	_, err = io.Copy(lw, gzReader)
	return err
}

// Chmod changes file permissions.
func (m *Manager) Chmod(path string, mode os.FileMode) error {
	m.mu.Lock()
	defer m.mu.Unlock()

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

// Chown changes file ownership. Rejects system user IDs (< 1000) to prevent
// privilege escalation.
func (m *Manager) Chown(path string, uid, gid int) error {
	if uid >= 0 && uid < MinUserUID {
		return fmt.Errorf("chown: uid %d is a system user (minimum: %d)", uid, MinUserUID)
	}
	if gid >= 0 && gid < MinUserUID {
		return fmt.Errorf("chown: gid %d is a system group (minimum: %d)", gid, MinUserUID)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	validPath, err := m.ValidatePath(path)
	if err != nil {
		return err
	}

	return os.Chown(validPath, uid, gid)
}

// GetFileDetails returns detailed file information.
func (m *Manager) GetFileDetails(path string) (map[string]interface{}, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
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
		"path":        m.toRelativePath(validPath),
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
	m.mu.RLock()
	defer m.mu.RUnlock()
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
	m.mu.RLock()
	defer m.mu.RUnlock()
	if _, err := m.ValidatePath(path); err != nil {
		return "", err
	}

	ext := strings.ToLower(filepath.Ext(path))
	if mime, ok := mimeTypes[ext]; ok {
		return mime, nil
	}

	return "application/octet-stream", nil
}

func (m *Manager) mkdirAllWithRecord(path string, perm os.FileMode, createdPaths *[]string) error {
	var toCreate []string
	curr := filepath.Clean(path)
	for {
		info, err := os.Stat(curr)
		if err == nil {
			if !info.IsDir() {
				return fmt.Errorf("not a directory: %s", curr)
			}
			break
		}
		if os.IsNotExist(err) {
			toCreate = append(toCreate, curr)
			parent := filepath.Dir(curr)
			if parent == curr || parent == "." || parent == "/" {
				break
			}
			curr = parent
		} else {
			return err
		}
	}

	for i := len(toCreate) - 1; i >= 0; i-- {
		p := toCreate[i]
		if err := os.Mkdir(p, perm); err != nil {
			if !os.IsExist(err) {
				return err
			}
		} else {
			*createdPaths = append(*createdPaths, p)
		}
	}
	return nil
}

var (
	maxExtractSize      int64 = 1024 * 1024 * 1024 // 1GB
	maxExtractFileCount int   = 10000              // 10k files
)

type limitWriter struct {
	w     io.Writer
	total *int64
	limit int64
}

func (lw *limitWriter) Write(p []byte) (int, error) {
	if *lw.total+int64(len(p)) > lw.limit {
		return 0, fmt.Errorf("extraction limit exceeded (max %d bytes)", lw.limit)
	}
	n, err := lw.w.Write(p)
	*lw.total += int64(n)
	return n, err
}

package http

import (
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"

	"easyserver/internal/domain/filemanager"
	"easyserver/internal/httpx"
	"easyserver/internal/httpx/middleware"
	"easyserver/internal/infra/errx"
	"easyserver/internal/util"

	"github.com/gin-gonic/gin"
)

// Debug helpers for upload diagnostics.
func keysOfMap(m map[string][]*multipart.FileHeader) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

func keysOfStringMap(m map[string][]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

type FileManagerHandler struct {
	fileManager   *filemanager.Manager
	maxUploadSize int64
}

func NewFileManagerHandler(fm *filemanager.Manager, maxUploadSize int64) *FileManagerHandler {
	if maxUploadSize <= 0 {
		maxUploadSize = 512 << 20 // 512 MB default
	}
	return &FileManagerHandler{
		fileManager:   fm,
		maxUploadSize: maxUploadSize,
	}
}

// List returns files in a directory
func (h *FileManagerHandler) List(c *gin.Context) (any, error) {
	path := c.Query("path")

	// Empty path means root - read basePath directly
	if path == "" {
		files, err := h.fileManager.ListRoot()
		if err != nil {
			return nil, err
		}
		return gin.H{
			"path":    "/",
			"parent":  "/",
			"entries": files,
		}, nil
	}

	files, err := h.fileManager.List(path)
	if err != nil {
		return nil, err
	}

	parent := "/"
	if path != "/" {
		parent = filepath.Dir(path)
	}

	return gin.H{
		"path":    path,
		"parent":  parent,
		"entries": files,
	}, nil
}

// Mkdir creates a directory
func (h *FileManagerHandler) Mkdir(c *gin.Context) (any, error) {
	var req struct {
		Path string `json:"path" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		return nil, errx.BadRequest("invalid request: %w", err)
	}

	middleware.AuditSummary(c, "创建目录 "+req.Path)
	if err := h.fileManager.Mkdir(req.Path); err != nil {
		return nil, err
	}

	return nil, nil
}

// Upload handles file upload
func (h *FileManagerHandler) Upload(c *gin.Context) (any, error) {
	file, header, err := c.Request.FormFile("file")
	if err != nil {
		// Debug: log what we received
		log.Printf("DEBUG upload: ContentType=%q, ContentLength=%d, err=%v", c.ContentType(), c.Request.ContentLength, err)
		if c.Request.MultipartForm != nil {
			log.Printf("DEBUG upload: MultipartForm.File keys=%v", keysOfMap(c.Request.MultipartForm.File))
			log.Printf("DEBUG upload: MultipartForm.Value keys=%v", keysOfStringMap(c.Request.MultipartForm.Value))
		}
		log.Printf("DEBUG upload: PostForm keys=%v", c.Request.PostForm)
		return nil, errx.BadRequest("no file provided")
	}
	defer file.Close()

	path := c.PostForm("path")
	// Sanitize path: strip any drive letters or illegal characters
	path = strings.ReplaceAll(path, "\\", "/")
	path = regexp.MustCompile(`^[A-Za-z]:`).ReplaceAllString(path, "")
	if path == "" {
		path = "/" + header.Filename
	}

	middleware.AuditSummary(c, "上传文件 "+header.Filename+" 到 "+path)
	// Use FileManager.Upload for secure file upload
	size, err := h.fileManager.Upload(file, path, h.maxUploadSize)
	if err != nil {
		return nil, errx.BadRequest("upload failed: %w", err)
	}

	return gin.H{
		"name": header.Filename,
		"path": path,
		"size": size,
	}, nil
}

// Download handles file download
func (h *FileManagerHandler) Download(c *gin.Context) (any, error) {
	path := c.Query("path")
	if path == "" {
		return nil, errx.BadRequest("path is required")
	}
	if util.IsTraversal(path) {
		return nil, errx.Forbidden("path traversal detected")
	}

	validPath, err := h.fileManager.ValidatePath(path)
	if err != nil {
		return nil, errx.BadRequest("invalid path: %w", err)
	}

	// Check if file exists
	info, err := os.Stat(validPath)
	if err != nil {
		return nil, errx.NotFound("file not found")
	}

	if info.IsDir() {
		return nil, errx.BadRequest("cannot download a directory")
	}

	// O_NOFOLLOW: TOCTOU defense between ValidatePath and serve.
	f, err := os.OpenFile(validPath, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return nil, errx.BadRequest("open file failed: %w", err)
	}
	defer f.Close()

	base := filepath.Base(validPath)

	// Media/image/PDF files are served inline so the browser can preview and
	// play them in <audio>/<video>/<img>/<iframe>. http.ServeContent handles
	// Range requests (206 Partial Content, required for streaming & seeking),
	// conditional requests (304), and Content-Type.
	//
	// Content-Type is set explicitly because X-Content-Type-Options: nosniff
	// forbids sniffing, so "application/octet-stream" would make <audio>/<video>
	// refuse to decode the file; mime.TypeByExtension often lacks audio entries
	// (e.g. .m4a) on Linux.
	if mt, ok := inlineMIME(base); ok {
		c.Header("Content-Type", mt)
		c.Header("Content-Disposition", httpx.FormatContentDisposition("inline", base))
		// PDF 预览通过 <iframe> 嵌入此接口返回的内容；SecurityMiddleware 默认的
		// frame-ancestors 'none' / X-Frame-Options: DENY 会阻止同源页面嵌入，
		// 这里放宽为同源可嵌入。<img>/<video>/<audio> 不受 frame 头约束，
		// 所以此覆盖实际上只作用于 PDF 的 iframe 渲染。文件响应非 HTML，
		// CSP 的其余指令对它无意义，故只保留 frame-ancestors。
		c.Header("Content-Security-Policy", "frame-ancestors 'self'")
		c.Header("X-Frame-Options", "SAMEORIGIN")
		http.ServeContent(c.Writer, c.Request, base, info.ModTime(), f)
		return nil, nil
	}

	// Regular download: force attachment so the browser saves it to disk.
	c.DataFromReader(200, info.Size(), "application/octet-stream", f, map[string]string{
		"Content-Disposition": httpx.FormatContentDisposition("attachment", base),
	})
	return nil, nil
}

// inlineMIMEByExt maps media/image/PDF extensions to their MIME types so the
// browser can play/preview them inline. Returns the MIME type and true for
// known previewable types; ("", false) means serve as a regular download.
var inlineMIMEByExt = map[string]string{
	// audio
	".mp3":  "audio/mpeg",
	".wav":  "audio/wav",
	".ogg":  "audio/ogg",
	".flac": "audio/flac",
	".m4a":  "audio/mp4",
	".aac":  "audio/aac",
	".opus": "audio/ogg",
	// video
	".mp4":  "video/mp4",
	".webm": "video/webm",
	".mkv":  "video/x-matroska",
	".mov":  "video/quicktime",
	".avi":  "video/x-msvideo",
	".m4v":  "video/mp4",
	// image
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".png":  "image/png",
	".gif":  "image/gif",
	".webp": "image/webp",
	".svg":  "image/svg+xml",
	".bmp":  "image/bmp",
	".ico":  "image/x-icon",
	".avif": "image/avif",
	// document
	".pdf": "application/pdf",
}

// inlineMIME returns the MIME type for previewable media/image/PDF files.
// The bool result is false for non-previewable types (served as attachment).
func inlineMIME(name string) (string, bool) {
	ext := strings.ToLower(filepath.Ext(name))
	mt, ok := inlineMIMEByExt[ext]
	return mt, ok
}

// Rename renames a file
func (h *FileManagerHandler) Rename(c *gin.Context) (any, error) {
	var req struct {
		OldPath string `json:"old_path" binding:"required"`
		NewPath string `json:"new_path" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		return nil, errx.BadRequest("invalid request: %w", err)
	}

	middleware.AuditSummary(c, "重命名 "+req.OldPath+" 为 "+req.NewPath)
	if err := h.fileManager.Rename(req.OldPath, req.NewPath); err != nil {
		return nil, err
	}

	return nil, nil
}

// Delete deletes a file or directory
func (h *FileManagerHandler) Delete(c *gin.Context) (any, error) {
	path := c.Query("path")
	if path == "" {
		return nil, errx.BadRequest("path is required")
	}

	recursive := c.Query("recursive") == "true"

	middleware.AuditSummary(c, "删除文件 "+path)
	if err := h.fileManager.Delete(path, recursive); err != nil {
		return nil, err
	}

	return nil, nil
}

// Move moves files
func (h *FileManagerHandler) Move(c *gin.Context) (any, error) {
	var req struct {
		Paths []string `json:"paths" binding:"required"`
		Dest  string   `json:"dest" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		return nil, errx.BadRequest("invalid request: %w", err)
	}

	middleware.AuditSummary(c, "移动文件到 "+req.Dest)
	if err := h.fileManager.Move(req.Paths, req.Dest); err != nil {
		return nil, err
	}

	return nil, nil
}

// Copy copies a file
func (h *FileManagerHandler) Copy(c *gin.Context) (any, error) {
	var req struct {
		Source string `json:"source" binding:"required"`
		Dest   string `json:"dest" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		return nil, errx.BadRequest("invalid request: %w", err)
	}

	middleware.AuditSummary(c, "复制文件 "+req.Source+" 到 "+req.Dest)
	if err := h.fileManager.Copy(req.Source, req.Dest); err != nil {
		return nil, err
	}

	return nil, nil
}

// GetContent returns file content
func (h *FileManagerHandler) GetContent(c *gin.Context) (any, error) {
	path := c.Query("path")
	if path == "" {
		return nil, errx.BadRequest("path is required")
	}

	content, err := h.fileManager.ReadContent(path)
	if err != nil {
		return nil, err
	}

	return content, nil
}

// SaveContent saves content to a file
func (h *FileManagerHandler) SaveContent(c *gin.Context) (any, error) {
	var req struct {
		Path    string `json:"path" binding:"required"`
		Content string `json:"content"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		return nil, errx.BadRequest("invalid request: %w", err)
	}

	middleware.AuditSummary(c, "保存文件内容 "+req.Path)
	if err := h.fileManager.WriteContent(req.Path, req.Content); err != nil {
		return nil, err
	}

	return nil, nil
}

// Search searches for files by name
func (h *FileManagerHandler) Search(c *gin.Context) (any, error) {
	rootPath := c.Query("path")
	if rootPath == "" {
		rootPath = "/"
	}
	pattern := c.Query("q")
	if pattern == "" {
		return nil, errx.BadRequest("search query is required")
	}

	maxResults := httpx.QueryInt(c, "limit", 100)

	results, err := h.fileManager.Search(rootPath, pattern, maxResults)
	if err != nil {
		return nil, err
	}

	return results, nil
}

// SearchContent searches for files containing text
func (h *FileManagerHandler) SearchContent(c *gin.Context) (any, error) {
	rootPath := c.Query("path")
	if rootPath == "" {
		rootPath = "/"
	}
	text := c.Query("q")
	if text == "" {
		return nil, errx.BadRequest("search query is required")
	}

	maxResults := httpx.QueryInt(c, "limit", 50)

	results, err := h.fileManager.SearchContent(rootPath, text, maxResults)
	if err != nil {
		return nil, err
	}

	return results, nil
}

// Compress creates a zip archive
func (h *FileManagerHandler) Compress(c *gin.Context) (any, error) {
	var req struct {
		Sources []string `json:"sources" binding:"required"`
		Dest    string   `json:"dest" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		return nil, errx.BadRequest("invalid request: %w", err)
	}

	middleware.AuditSummary(c, "压缩文件到 "+req.Dest)
	if err := h.fileManager.Compress(req.Sources, req.Dest); err != nil {
		return nil, err
	}

	return nil, nil
}

// Extract extracts an archive
func (h *FileManagerHandler) Extract(c *gin.Context) (any, error) {
	// Dest 允许为空：空串表示解压到根目录(basePath)，与 ValidatePath 的
	// "empty path or '.' is treated as basePath" 语义一致。前端 toRelativePath
	// 在根目录下返回空串，这里不能用 binding:"required"，否则根目录解压会被拒。
	var req struct {
		Source string `json:"source" binding:"required"`
		Dest   string `json:"dest"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		return nil, errx.BadRequest("invalid request: %w", err)
	}

	middleware.AuditSummary(c, "解压文件 "+req.Source+" 到 "+req.Dest)
	if err := h.fileManager.Extract(req.Source, req.Dest); err != nil {
		return nil, err
	}

	return nil, nil
}

// ArchiveList lists entries in an archive file (for preview).
func (h *FileManagerHandler) ArchiveList(c *gin.Context) (any, error) {
	path := c.Query("path")
	if path == "" {
		return nil, errx.BadRequest("缺少 path")
	}
	entries, err := h.fileManager.ListArchiveEntries(path)
	if err != nil {
		return nil, err
	}
	return httpx.Paginate(entries, httpx.ParsePagination(c, 50, 200)), nil
}

// Chmod changes file permissions
func (h *FileManagerHandler) Chmod(c *gin.Context) (any, error) {
	var req struct {
		Path string `json:"path" binding:"required"`
		Mode string `json:"mode" binding:"required"` // e.g., "0755", "644"
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		return nil, errx.BadRequest("invalid request: %w", err)
	}

	// Parse mode string
	mode, err := strconv.ParseUint(req.Mode, 8, 32)
	if err != nil {
		return nil, errx.BadRequest("invalid mode format")
	}

	middleware.AuditSummary(c, "修改文件权限 "+req.Path+" "+req.Mode)
	if err := h.fileManager.Chmod(req.Path, os.FileMode(mode)); err != nil {
		return nil, err
	}

	return nil, nil
}

// Chown changes file ownership
func (h *FileManagerHandler) Chown(c *gin.Context) (any, error) {
	var req struct {
		Path string `json:"path" binding:"required"`
		UID  int    `json:"uid"`
		GID  int    `json:"gid"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		return nil, errx.BadRequest("invalid request: %w", err)
	}

	middleware.AuditSummary(c, "修改文件所有者 "+req.Path)
	if err := h.fileManager.Chown(req.Path, req.UID, req.GID); err != nil {
		return nil, err
	}

	return nil, nil
}

// GetDetails returns detailed file information
func (h *FileManagerHandler) GetDetails(c *gin.Context) (any, error) {
	path := c.Query("path")
	if path == "" {
		return nil, errx.BadRequest("path is required")
	}

	details, err := h.fileManager.GetFileDetails(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, errx.NotFound("文件不存在")
		}
		return nil, err
	}

	return details, nil
}

// GetMimeType returns the MIME type of a file
func (h *FileManagerHandler) GetMimeType(c *gin.Context) (any, error) {
	path := c.Query("path")
	if path == "" {
		return nil, errx.BadRequest("path is required")
	}

	mimeType, err := h.fileManager.GetMimeType(path)
	if err != nil {
		return nil, errx.Forbidden("%w", err)
	}

	return gin.H{
		"path":      path,
		"mime_type": mimeType,
	}, nil
}

// RegisterRoutes registers file management routes.
// fileRoutesWithLargeBody is used only for the upload endpoint (larger body limit).
func RegisterRoutes(protected *gin.RouterGroup, fileRoutesWithLargeBody *gin.RouterGroup, fileManager *filemanager.Manager, maxUploadSize int64) {
	handler := NewFileManagerHandler(fileManager, maxUploadSize)
	protected.GET("/files", httpx.H(handler.List))
	protected.GET("/files/download", httpx.H(handler.Download))
	protected.GET("/files/content", httpx.H(handler.GetContent))
	protected.GET("/files/search", httpx.H(handler.Search))
	protected.GET("/files/search-content", httpx.H(handler.SearchContent))
	protected.GET("/files/details", httpx.H(handler.GetDetails))
	protected.GET("/files/mime-type", httpx.H(handler.GetMimeType))
	protected.POST("/files/mkdir", httpx.H(handler.Mkdir))
	// Upload uses the large-body route group
	if fileRoutesWithLargeBody != nil {
		fileRoutesWithLargeBody.POST("/upload", httpx.H(handler.Upload))
	} else {
		protected.POST("/files/upload", httpx.H(handler.Upload))
	}
	protected.PUT("/files/rename", httpx.H(handler.Rename))
	protected.DELETE("/files", httpx.H(handler.Delete))
	protected.POST("/files/move", httpx.H(handler.Move))
	protected.POST("/files/copy", httpx.H(handler.Copy))
	protected.PUT("/files/content", httpx.H(handler.SaveContent))
	protected.POST("/files/compress", httpx.H(handler.Compress))
	protected.POST("/files/extract", httpx.H(handler.Extract))
	protected.GET("/files/archive-list", httpx.H(handler.ArchiveList))
	protected.PUT("/files/chmod", httpx.H(handler.Chmod))
	protected.PUT("/files/chown", httpx.H(handler.Chown))
}

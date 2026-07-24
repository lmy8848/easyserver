package http

import (
	"fmt"
	"log"
	"mime/multipart"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"

	"easyserver/internal/filemanager"
	"easyserver/internal/httpx"
	"easyserver/internal/httpx/middleware"
	"easyserver/internal/infra/apperror"

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

// getUserInfo extracts user info from context

// List returns files in a directory
func (h *FileManagerHandler) List(c *gin.Context) {
	path := c.Query("path")

	// Empty path means root - read basePath directly
	if path == "" {
		files, err := h.fileManager.ListRoot()
		if err != nil {
			c.Error(apperror.WrapError(err))
			return
		}
		httpx.Success(c, gin.H{
			"path":    "/",
			"parent":  "/",
			"entries": files,
		})
		return
	}

	files, err := h.fileManager.List(path)
	if err != nil {
		c.Error(apperror.WrapError(err))
		return
	}

	parent := "/"
	if path != "/" {
		parent = filepath.Dir(path)
	}

	httpx.Success(c, gin.H{
		"path":    path,
		"parent":  parent,
		"entries": files,
	})
}

// Mkdir creates a directory
func (h *FileManagerHandler) Mkdir(c *gin.Context) {
	var req struct {
		Path string `json:"path" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperror.ErrBadRequest.Wrap(err))
		return
	}

	middleware.AuditSummary(c, "创建目录 "+req.Path)
	if err := h.fileManager.Mkdir(req.Path); err != nil {
		c.Error(apperror.WrapError(err))
		return
	}

	httpx.Success(c, nil)
}

// Upload handles file upload
func (h *FileManagerHandler) Upload(c *gin.Context) {
	file, header, err := c.Request.FormFile("file")
	if err != nil {
		// Debug: log what we received
		log.Printf("DEBUG upload: ContentType=%q, ContentLength=%d, err=%v", c.ContentType(), c.Request.ContentLength, err)
		if c.Request.MultipartForm != nil {
			log.Printf("DEBUG upload: MultipartForm.File keys=%v", keysOfMap(c.Request.MultipartForm.File))
			log.Printf("DEBUG upload: MultipartForm.Value keys=%v", keysOfStringMap(c.Request.MultipartForm.Value))
		}
		log.Printf("DEBUG upload: PostForm keys=%v", c.Request.PostForm)
		c.Error(apperror.ErrBadRequest.WithMessage("no file provided"))
		return
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
		c.Error(apperror.ErrBadRequest.Wrap(err))
		return
	}

	httpx.Success(c, gin.H{
		"name": header.Filename,
		"path": path,
		"size": size,
	})
}

// Download handles file download
func (h *FileManagerHandler) Download(c *gin.Context) {
	path := c.Query("path")
	if path == "" {
		c.Error(apperror.ErrBadRequest.WithMessage("path is required"))
		return
	}

	validPath, err := h.fileManager.ValidatePath(path)
	if err != nil {
		c.Error(apperror.ErrBadRequest.Wrap(err))
		return
	}

	// Check if file exists
	info, err := os.Stat(validPath)
	if err != nil {
		c.Error(apperror.ErrNotFound.WithMessage("file not found"))
		return
	}

	if info.IsDir() {
		c.Error(apperror.ErrBadRequest.WithMessage("cannot download a directory"))
		return
	}

	// O_NOFOLLOW: TOCTOU defense between ValidatePath and serve.
	f, err := os.OpenFile(validPath, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		c.Error(apperror.ErrBadRequest.Wrap(err))
		return
	}
	defer f.Close()

	c.DataFromReader(200, info.Size(), "application/octet-stream", f, map[string]string{
		"Content-Disposition": fmt.Sprintf("attachment; filename=%q", filepath.Base(validPath)),
	})
}

// Rename renames a file
func (h *FileManagerHandler) Rename(c *gin.Context) {
	var req struct {
		OldPath string `json:"old_path" binding:"required"`
		NewPath string `json:"new_path" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperror.ErrBadRequest.Wrap(err))
		return
	}

	middleware.AuditSummary(c, "重命名 "+req.OldPath+" 为 "+req.NewPath)
	if err := h.fileManager.Rename(req.OldPath, req.NewPath); err != nil {
		c.Error(apperror.WrapError(err))
		return
	}

	httpx.Success(c, nil)
}

// Delete deletes a file or directory
func (h *FileManagerHandler) Delete(c *gin.Context) {
	path := c.Query("path")
	if path == "" {
		c.Error(apperror.ErrBadRequest.WithMessage("path is required"))
		return
	}

	recursive := c.Query("recursive") == "true"

	middleware.AuditSummary(c, "删除文件 "+path)
	if err := h.fileManager.Delete(path, recursive); err != nil {
		c.Error(apperror.WrapError(err))
		return
	}

	httpx.Success(c, nil)
}

// Move moves files
func (h *FileManagerHandler) Move(c *gin.Context) {
	var req struct {
		Paths []string `json:"paths" binding:"required"`
		Dest  string   `json:"dest" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperror.ErrBadRequest.Wrap(err))
		return
	}

	middleware.AuditSummary(c, "移动文件到 "+req.Dest)
	if err := h.fileManager.Move(req.Paths, req.Dest); err != nil {
		c.Error(apperror.WrapError(err))
		return
	}

	httpx.Success(c, nil)
}

// Copy copies a file
func (h *FileManagerHandler) Copy(c *gin.Context) {
	var req struct {
		Source string `json:"source" binding:"required"`
		Dest   string `json:"dest" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperror.ErrBadRequest.Wrap(err))
		return
	}

	middleware.AuditSummary(c, "复制文件 "+req.Source+" 到 "+req.Dest)
	if err := h.fileManager.Copy(req.Source, req.Dest); err != nil {
		c.Error(apperror.WrapError(err))
		return
	}

	httpx.Success(c, nil)
}

// GetContent returns file content
func (h *FileManagerHandler) GetContent(c *gin.Context) {
	path := c.Query("path")
	if path == "" {
		c.Error(apperror.ErrBadRequest.WithMessage("path is required"))
		return
	}

	content, err := h.fileManager.ReadContent(path)
	if err != nil {
		c.Error(apperror.WrapError(err))
		return
	}

	httpx.Success(c, content)
}

// SaveContent saves content to a file
func (h *FileManagerHandler) SaveContent(c *gin.Context) {
	var req struct {
		Path    string `json:"path" binding:"required"`
		Content string `json:"content"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperror.ErrBadRequest.Wrap(err))
		return
	}

	middleware.AuditSummary(c, "保存文件内容 "+req.Path)
	if err := h.fileManager.WriteContent(req.Path, req.Content); err != nil {
		c.Error(apperror.WrapError(err))
		return
	}

	httpx.Success(c, nil)
}

// Search searches for files by name
func (h *FileManagerHandler) Search(c *gin.Context) {
	rootPath := c.Query("path")
	if rootPath == "" {
		rootPath = "/"
	}
	pattern := c.Query("q")
	if pattern == "" {
		c.Error(apperror.ErrBadRequest.WithMessage("search query is required"))
		return
	}

	maxResults, _ := strconv.Atoi(c.DefaultQuery("limit", "100"))

	results, err := h.fileManager.Search(rootPath, pattern, maxResults)
	if err != nil {
		c.Error(apperror.WrapError(err))
		return
	}

	httpx.Success(c, results)
}

// SearchContent searches for files containing text
func (h *FileManagerHandler) SearchContent(c *gin.Context) {
	rootPath := c.Query("path")
	if rootPath == "" {
		rootPath = "/"
	}
	text := c.Query("q")
	if text == "" {
		c.Error(apperror.ErrBadRequest.WithMessage("search query is required"))
		return
	}

	maxResults, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))

	results, err := h.fileManager.SearchContent(rootPath, text, maxResults)
	if err != nil {
		c.Error(apperror.WrapError(err))
		return
	}

	httpx.Success(c, results)
}

// Compress creates a zip archive
func (h *FileManagerHandler) Compress(c *gin.Context) {
	var req struct {
		Sources []string `json:"sources" binding:"required"`
		Dest    string   `json:"dest" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperror.ErrBadRequest.Wrap(err))
		return
	}

	middleware.AuditSummary(c, "压缩文件到 "+req.Dest)
	if err := h.fileManager.Compress(req.Sources, req.Dest); err != nil {
		c.Error(apperror.WrapError(err))
		return
	}

	httpx.Success(c, nil)
}

// Extract extracts an archive
func (h *FileManagerHandler) Extract(c *gin.Context) {
	// Dest 允许为空：空串表示解压到根目录(basePath)，与 ValidatePath 的
	// "empty path or '.' is treated as basePath" 语义一致。前端 toRelativePath
	// 在根目录下返回空串，这里不能用 binding:"required"，否则根目录解压会被拒。
	var req struct {
		Source string `json:"source" binding:"required"`
		Dest   string `json:"dest"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperror.ErrBadRequest.Wrap(err))
		return
	}

	middleware.AuditSummary(c, "解压文件 "+req.Source+" 到 "+req.Dest)
	if err := h.fileManager.Extract(req.Source, req.Dest); err != nil {
		c.Error(apperror.WrapError(err))
		return
	}

	httpx.Success(c, nil)
}

// ArchiveList lists entries in an archive file (for preview).
func (h *FileManagerHandler) ArchiveList(c *gin.Context) {
	path := c.Query("path")
	if path == "" {
		c.Error(apperror.ErrBadRequest.WithMessage("缺少 path"))
		return
	}
	entries, err := h.fileManager.ListArchiveEntries(path)
	if err != nil {
		c.Error(apperror.WrapError(err))
		return
	}
	httpx.Success(c, gin.H{"entries": entries})
}

// Chmod changes file permissions
func (h *FileManagerHandler) Chmod(c *gin.Context) {
	var req struct {
		Path string `json:"path" binding:"required"`
		Mode string `json:"mode" binding:"required"` // e.g., "0755", "644"
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperror.ErrBadRequest.Wrap(err))
		return
	}

	// Parse mode string
	mode, err := strconv.ParseUint(req.Mode, 8, 32)
	if err != nil {
		c.Error(apperror.ErrBadRequest.WithMessage("invalid mode format"))
		return
	}

	middleware.AuditSummary(c, "修改文件权限 "+req.Path+" "+req.Mode)
	if err := h.fileManager.Chmod(req.Path, os.FileMode(mode)); err != nil {
		c.Error(apperror.WrapError(err))
		return
	}

	httpx.Success(c, nil)
}

// Chown changes file ownership
func (h *FileManagerHandler) Chown(c *gin.Context) {
	var req struct {
		Path string `json:"path" binding:"required"`
		UID  int    `json:"uid"`
		GID  int    `json:"gid"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperror.ErrBadRequest.Wrap(err))
		return
	}

	middleware.AuditSummary(c, "修改文件所有者 "+req.Path)
	if err := h.fileManager.Chown(req.Path, req.UID, req.GID); err != nil {
		c.Error(apperror.WrapError(err))
		return
	}

	httpx.Success(c, nil)
}

// GetDetails returns detailed file information
func (h *FileManagerHandler) GetDetails(c *gin.Context) {
	path := c.Query("path")
	if path == "" {
		c.Error(apperror.ErrBadRequest.WithMessage("path is required"))
		return
	}

	details, err := h.fileManager.GetFileDetails(path)
	if err != nil {
		if os.IsNotExist(err) {
			c.Error(apperror.ErrNotFound.WithMessage("文件不存在"))
			return
		}
		c.Error(apperror.WrapError(err))
		return
	}

	httpx.Success(c, details)
}

// GetMimeType returns the MIME type of a file
func (h *FileManagerHandler) GetMimeType(c *gin.Context) {
	path := c.Query("path")
	if path == "" {
		c.Error(apperror.ErrBadRequest.WithMessage("path is required"))
		return
	}

	mimeType, err := h.fileManager.GetMimeType(path)
	if err != nil {
		c.Error(apperror.ErrForbidden.Wrap(err))
		return
	}

	httpx.Success(c, gin.H{
		"path":      path,
		"mime_type": mimeType,
	})
}

// RegisterRoutes registers file management routes.
// fileRoutesWithLargeBody is used only for the upload endpoint (larger body limit).
func RegisterRoutes(protected *gin.RouterGroup, fileRoutesWithLargeBody *gin.RouterGroup, fileManager *filemanager.Manager, maxUploadSize int64) {
	handler := NewFileManagerHandler(fileManager, maxUploadSize)
	protected.GET("/files", handler.List)
	protected.GET("/files/download", handler.Download)
	protected.GET("/files/content", handler.GetContent)
	protected.GET("/files/search", handler.Search)
	protected.GET("/files/search-content", handler.SearchContent)
	protected.GET("/files/details", handler.GetDetails)
	protected.GET("/files/mime-type", handler.GetMimeType)
	protected.POST("/files/mkdir", handler.Mkdir)
	// Upload uses the large-body route group
	if fileRoutesWithLargeBody != nil {
		fileRoutesWithLargeBody.POST("/upload", handler.Upload)
	} else {
		protected.POST("/files/upload", handler.Upload)
	}
	protected.PUT("/files/rename", handler.Rename)
	protected.DELETE("/files", handler.Delete)
	protected.POST("/files/move", handler.Move)
	protected.POST("/files/copy", handler.Copy)
	protected.PUT("/files/content", handler.SaveContent)
	protected.POST("/files/compress", handler.Compress)
	protected.POST("/files/extract", handler.Extract)
	protected.GET("/files/archive-list", handler.ArchiveList)
	protected.PUT("/files/chmod", handler.Chmod)
	protected.PUT("/files/chown", handler.Chown)
}

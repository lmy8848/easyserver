package api

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"syscall"

	"easyserver/internal/audit"
	"easyserver/internal/filemanager"

	"github.com/gin-gonic/gin"
)

type FileManagerHandler struct {
	fileManager  *filemanager.Manager
	auditService *audit.Service
}

func NewFileManagerHandler(fm *filemanager.Manager, auditService *audit.Service) *FileManagerHandler {
	return &FileManagerHandler{
		fileManager:  fm,
		auditService: auditService,
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
			c.Error(WrapError(err))
			return
		}
		Success(c, gin.H{
			"path":    "/",
			"parent":  "/",
			"entries": files,
		})
		return
	}

	files, err := h.fileManager.List(path)
	if err != nil {
		c.Error(WrapError(err))
		return
	}

	parent := "/"
	if path != "/" {
		parent = filepath.Dir(path)
	}

	Success(c, gin.H{
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
		c.Error(ErrBadRequest.Wrap(err))
		return
	}

	if err := h.fileManager.Mkdir(req.Path); err != nil {
		c.Error(WrapError(err))
		return
	}

	Success(c, nil)
}

// Upload handles file upload
func (h *FileManagerHandler) Upload(c *gin.Context) {
	file, header, err := c.Request.FormFile("file")
	if err != nil {
		c.Error(ErrBadRequest.WithMessage("no file provided"))
		return
	}
	defer file.Close()

	path := c.PostForm("path")
	if path == "" {
		path = "/" + header.Filename
	}

	// Use FileManager.Upload for secure file upload
	size, err := h.fileManager.Upload(file, path)
	if err != nil {
		c.Error(ErrBadRequest.Wrap(err))
		return
	}

	Success(c, gin.H{
		"name": header.Filename,
		"path": path,
		"size": size,
	})
}

// Download handles file download
func (h *FileManagerHandler) Download(c *gin.Context) {
	path := c.Query("path")
	if path == "" {
		c.Error(ErrBadRequest.WithMessage("path is required"))
		return
	}

	validPath, err := h.fileManager.ValidatePath(path)
	if err != nil {
		c.Error(ErrBadRequest.Wrap(err))
		return
	}

	// Check if file exists
	info, err := os.Stat(validPath)
	if err != nil {
		c.Error(ErrNotFound.WithMessage("file not found"))
		return
	}

	if info.IsDir() {
		c.Error(ErrBadRequest.WithMessage("cannot download a directory"))
		return
	}

	// O_NOFOLLOW: TOCTOU defense between ValidatePath and serve.
	f, err := os.OpenFile(validPath, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		c.Error(ErrBadRequest.Wrap(err))
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
		c.Error(ErrBadRequest.Wrap(err))
		return
	}

	if err := h.fileManager.Rename(req.OldPath, req.NewPath); err != nil {
		c.Error(WrapError(err))
		return
	}

	Success(c, nil)
}

// Delete deletes a file or directory
func (h *FileManagerHandler) Delete(c *gin.Context) {
	path := c.Query("path")
	if path == "" {
		c.Error(ErrBadRequest.WithMessage("path is required"))
		return
	}

	recursive := c.Query("recursive") == "true"

	if err := h.fileManager.Delete(path, recursive); err != nil {
		c.Error(WrapError(err))
		return
	}

	Success(c, nil)
}

// Move moves files
func (h *FileManagerHandler) Move(c *gin.Context) {
	var req struct {
		Paths []string `json:"paths" binding:"required"`
		Dest  string   `json:"dest" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(ErrBadRequest.Wrap(err))
		return
	}

	if err := h.fileManager.Move(req.Paths, req.Dest); err != nil {
		c.Error(WrapError(err))
		return
	}

	Success(c, nil)
}

// Copy copies a file
func (h *FileManagerHandler) Copy(c *gin.Context) {
	var req struct {
		Source string `json:"source" binding:"required"`
		Dest   string `json:"dest" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(ErrBadRequest.Wrap(err))
		return
	}

	if err := h.fileManager.Copy(req.Source, req.Dest); err != nil {
		c.Error(WrapError(err))
		return
	}

	Success(c, nil)
}

// GetContent returns file content
func (h *FileManagerHandler) GetContent(c *gin.Context) {
	path := c.Query("path")
	if path == "" {
		c.Error(ErrBadRequest.WithMessage("path is required"))
		return
	}

	content, err := h.fileManager.ReadContent(path)
	if err != nil {
		c.Error(WrapError(err))
		return
	}

	Success(c, content)
}

// SaveContent saves content to a file
func (h *FileManagerHandler) SaveContent(c *gin.Context) {
	var req struct {
		Path    string `json:"path" binding:"required"`
		Content string `json:"content"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(ErrBadRequest.Wrap(err))
		return
	}

	if err := h.fileManager.WriteContent(req.Path, req.Content); err != nil {
		c.Error(WrapError(err))
		return
	}

	Success(c, nil)
}

// Search searches for files by name
func (h *FileManagerHandler) Search(c *gin.Context) {
	rootPath := c.Query("path")
	if rootPath == "" {
		rootPath = "/"
	}
	pattern := c.Query("q")
	if pattern == "" {
		c.Error(ErrBadRequest.WithMessage("search query is required"))
		return
	}

	maxResults, _ := strconv.Atoi(c.DefaultQuery("limit", "100"))

	results, err := h.fileManager.Search(rootPath, pattern, maxResults)
	if err != nil {
		c.Error(WrapError(err))
		return
	}

	Success(c, results)
}

// SearchContent searches for files containing text
func (h *FileManagerHandler) SearchContent(c *gin.Context) {
	rootPath := c.Query("path")
	if rootPath == "" {
		rootPath = "/"
	}
	text := c.Query("q")
	if text == "" {
		c.Error(ErrBadRequest.WithMessage("search query is required"))
		return
	}

	maxResults, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))

	results, err := h.fileManager.SearchContent(rootPath, text, maxResults)
	if err != nil {
		c.Error(WrapError(err))
		return
	}

	Success(c, results)
}

// Compress creates a zip archive
func (h *FileManagerHandler) Compress(c *gin.Context) {
	var req struct {
		Sources []string `json:"sources" binding:"required"`
		Dest    string   `json:"dest" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(ErrBadRequest.Wrap(err))
		return
	}

	if err := h.fileManager.Compress(req.Sources, req.Dest); err != nil {
		c.Error(WrapError(err))
		return
	}

	Success(c, nil)
}

// Extract extracts an archive
func (h *FileManagerHandler) Extract(c *gin.Context) {
	var req struct {
		Source string `json:"source" binding:"required"`
		Dest   string `json:"dest" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(ErrBadRequest.Wrap(err))
		return
	}

	if err := h.fileManager.Extract(req.Source, req.Dest); err != nil {
		c.Error(WrapError(err))
		return
	}

	Success(c, nil)
}

// Chmod changes file permissions
func (h *FileManagerHandler) Chmod(c *gin.Context) {
	var req struct {
		Path string `json:"path" binding:"required"`
		Mode string `json:"mode" binding:"required"` // e.g., "0755", "644"
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(ErrBadRequest.Wrap(err))
		return
	}

	// Parse mode string
	mode, err := strconv.ParseUint(req.Mode, 8, 32)
	if err != nil {
		c.Error(ErrBadRequest.WithMessage("invalid mode format"))
		return
	}

	if err := h.fileManager.Chmod(req.Path, os.FileMode(mode)); err != nil {
		c.Error(WrapError(err))
		return
	}

	Success(c, nil)
}

// Chown changes file ownership
func (h *FileManagerHandler) Chown(c *gin.Context) {
	var req struct {
		Path string `json:"path" binding:"required"`
		UID  int    `json:"uid"`
		GID  int    `json:"gid"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(ErrBadRequest.Wrap(err))
		return
	}

	if err := h.fileManager.Chown(req.Path, req.UID, req.GID); err != nil {
		c.Error(WrapError(err))
		return
	}

	Success(c, nil)
}

// GetDetails returns detailed file information
func (h *FileManagerHandler) GetDetails(c *gin.Context) {
	path := c.Query("path")
	if path == "" {
		c.Error(ErrBadRequest.WithMessage("path is required"))
		return
	}

	details, err := h.fileManager.GetFileDetails(path)
	if err != nil {
		if os.IsNotExist(err) {
			c.Error(ErrNotFound.WithMessage("文件不存在"))
			return
		}
		c.Error(WrapError(err))
		return
	}

	Success(c, details)
}

// GetMimeType returns the MIME type of a file
func (h *FileManagerHandler) GetMimeType(c *gin.Context) {
	path := c.Query("path")
	if path == "" {
		c.Error(ErrBadRequest.WithMessage("path is required"))
		return
	}

	mimeType, err := h.fileManager.GetMimeType(path)
	if err != nil {
		c.Error(ErrForbidden.Wrap(err))
		return
	}

	Success(c, gin.H{
		"path":      path,
		"mime_type": mimeType,
	})
}

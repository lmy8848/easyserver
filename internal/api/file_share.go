package api

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"easyserver/internal/api/middleware"
	"easyserver/internal/filemanager"
	"easyserver/internal/fileshare"

	"github.com/gin-gonic/gin"
)

type FileShareHandler struct {
	shareRepo   fileshare.Repository
	fileManager *filemanager.Manager
}

func NewFileShareHandler(shareRepo fileshare.Repository, fm *filemanager.Manager) *FileShareHandler {
	return &FileShareHandler{shareRepo: shareRepo, fileManager: fm}
}

// detectContentType returns a MIME type based on file extension.
// Embedded images, PDFs and text files render inline in the browser;
// everything else forces a download.
func detectContentType(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".png":
		return "image/png"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	case ".svg":
		return "image/svg+xml"
	case ".bmp":
		return "image/bmp"
	case ".ico":
		return "image/x-icon"
	case ".avif":
		return "image/avif"
	case ".pdf":
		return "application/pdf"
	case ".txt":
		return "text/plain; charset=utf-8"
	case ".md":
		return "text/markdown; charset=utf-8"
	case ".html", ".htm":
		return "text/html; charset=utf-8"
	case ".json":
		return "application/json"
	case ".xml":
		return "application/xml"
	case ".csv":
		return "text/csv; charset=utf-8"
	case ".yaml", ".yml":
		return "text/yaml; charset=utf-8"
	case ".toml":
		return "text/toml; charset=utf-8"
	case ".css":
		return "text/css; charset=utf-8"
	case ".js":
		return "text/javascript; charset=utf-8"
	case ".ts":
		return "text/typescript; charset=utf-8"
	case ".go":
		return "text/x-go; charset=utf-8"
	case ".py":
		return "text/x-python; charset=utf-8"
	case ".sh":
		return "text/x-shellscript; charset=utf-8"
	case ".zip":
		return "application/zip"
	case ".gz", ".gzip":
		return "application/gzip"
	case ".tar":
		return "application/x-tar"
	case ".bz2":
		return "application/x-bzip2"
	case ".xz":
		return "application/x-xz"
	case ".7z":
		return "application/x-7z-compressed"
	case ".rar":
		return "application/vnd.rar"
	case ".mp4":
		return "video/mp4"
	case ".mp3":
		return "audio/mpeg"
	case ".webm":
		return "video/webm"
	case ".avi":
		return "video/x-msvideo"
	case ".mov":
		return "video/quicktime"
	case ".wav":
		return "audio/wav"
	case ".flac":
		return "audio/flac"
	case ".ogg":
		return "audio/ogg"
	case ".woff", ".woff2":
		return "font/woff2"
	case ".ttf":
		return "font/ttf"
	default:
		return "application/octet-stream"
	}
}

// fileShareTokenBytes is the number of random bytes for share tokens (64 hex chars).
const fileShareTokenBytes = 32

// fileShareMaxSize is the maximum file size allowed for sharing (500 MB).
const fileShareMaxSize = 500 * 1024 * 1024

// generateToken creates a secure random token for file sharing
func generateToken() (string, error) {
	b := make([]byte, fileShareTokenBytes)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate token: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// CreateShare creates a new file share link
func (h *FileShareHandler) CreateShare(c *gin.Context) {
	var req fileshare.CreateShareRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(ErrBadRequest.Wrap(err))
		return
	}

	// Validate file path
	validPath, err := h.fileManager.ValidatePath(req.FilePath)
	if err != nil {
		c.Error(ErrBadRequest.WithMessage("文件路径无效"))
		return
	}

	// Check file exists
	info, err := os.Stat(validPath)
	if err != nil {
		c.Error(ErrNotFound.WithMessage("文件不存在"))
		return
	}
	if info.IsDir() {
		c.Error(ErrBadRequest.WithMessage("不支持分享目录"))
		return
	}

	// Check file size limit (max 500MB)
	if info.Size() > fileShareMaxSize {
		c.Error(ErrBadRequest.WithMessage("文件超过500MB，不支持分享"))
		return
	}

	middleware.AuditSummary(c, "生成文件外链 "+req.FilePath)

	userID, _ := c.Get("user_id")
	uid, _ := userID.(int64)

	// Validate expires_at format if provided
	if req.ExpiresAt != "" {
		// Accept "1h", "1d", "7d", or absolute "2026-07-01 12:00:00"
		if strings.HasSuffix(req.ExpiresAt, "h") || strings.HasSuffix(req.ExpiresAt, "d") {
			// Relative time - calculate absolute
			durStr := req.ExpiresAt
			duration := time.Hour
			if strings.HasSuffix(durStr, "d") {
				durStr = strings.TrimSuffix(durStr, "d")
				if val, err := strconv.Atoi(durStr); err == nil {
					duration = time.Duration(val) * 24 * time.Hour
				}
			} else {
				durStr = strings.TrimSuffix(durStr, "h")
				if val, err := strconv.Atoi(durStr); err == nil {
					duration = time.Duration(val) * time.Hour
				}
			}
			req.ExpiresAt = time.Now().Add(duration).Format("2006-01-02 15:04:05")
		}
	}

	token, err := generateToken()
	if err != nil {
		c.Error(ErrInternal.Wrap(err))
		return
	}

	share := &fileshare.FileShare{
		FilePath:      req.FilePath,
		FileName:      info.Name(),
		FileSize:      info.Size(),
		Token:         token,
		Password:      req.Password,
		ExpiresAt:     req.ExpiresAt,
		MaxDownloads:  req.MaxDownloads,
		DownloadCount: 0,
		CreatedBy:     uid,
	}

	id, err := h.shareRepo.Create(c.Request.Context(), share)
	if err != nil {
		c.Error(ErrInternal.Wrap(err))
		return
	}
	share.ID = id
	// Don't expose password in response
	share.Password = ""
	Success(c, share)
}

// ShareListItem is an enriched share record with current file status.
type ShareListItem struct {
	fileshare.FileShare
	FileExists  bool  `json:"file_exists"`
	CurrentSize int64 `json:"current_size"`
}

// ListShares lists all file shares for the current user
func (h *FileShareHandler) ListShares(c *gin.Context) {
	userID, _ := c.Get("user_id")
	uid, _ := userID.(int64)

	shares, err := h.shareRepo.List(c.Request.Context(), uid)
	if err != nil {
		c.Error(ErrInternal.Wrap(err))
		return
	}

	items := make([]ShareListItem, 0, len(shares))
	for _, s := range shares {
		s.Password = ""
		item := ShareListItem{FileShare: s}
		validPath, err := h.fileManager.ValidatePath(s.FilePath)
		if err == nil {
			if info, err := os.Stat(validPath); err == nil && !info.IsDir() {
				item.FileExists = true
				item.CurrentSize = info.Size()
			}
		}
		items = append(items, item)
	}
	Success(c, items)
}

// DeleteShare revokes a file share
func (h *FileShareHandler) DeleteShare(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.Error(ErrBadRequest.WithMessage("无效的ID"))
		return
	}

	middleware.AuditSummary(c, "撤销文件外链 #"+idStr)
	if err := h.shareRepo.Delete(c.Request.Context(), id); err != nil {
		c.Error(ErrInternal.Wrap(err))
		return
	}
	Success(c, nil)
}

// CleanupExpired removes expired file shares
func (h *FileShareHandler) CleanupExpired(c *gin.Context) {
	count, err := h.shareRepo.DeleteExpired(c.Request.Context())
	if err != nil {
		c.Error(ErrInternal.Wrap(err))
		return
	}
	Success(c, gin.H{"deleted": count})
}

// PublicDownload handles public file download via share token (no auth required)
func (h *FileShareHandler) PublicDownload(c *gin.Context) {
	token := c.Param("token")
	if token == "" {
		c.Error(ErrBadRequest.WithMessage("缺少分享令牌"))
		return
	}

	share, err := h.shareRepo.GetByToken(c.Request.Context(), token)
	if err != nil {
		c.Error(ErrInternal.Wrap(err))
		return
	}
	if share == nil {
		c.Error(ErrNotFound.WithMessage("分享链接不存在或已失效"))
		return
	}

	// Check password
	if share.Password != "" {
		password := c.Query("password")
		if password == "" {
			c.Error(ErrForbidden.WithMessage("需要密码访问，请在链接后添加 ?password=xxx"))
			return
		}
		if password != share.Password {
			c.Error(ErrForbidden.WithMessage("密码错误"))
			return
		}
	}

	// Check expiration
	if share.ExpiresAt != "" {
		expires, err := time.Parse("2006-01-02 15:04:05", share.ExpiresAt)
		if err == nil && time.Now().After(expires) {
			// Auto-cleanup expired share
			h.shareRepo.Delete(c.Request.Context(), share.ID)
			c.Error(ErrNotFound.WithMessage("分享链接已过期"))
			return
		}
	}

	// Check max downloads
	if share.MaxDownloads > 0 && share.DownloadCount >= share.MaxDownloads {
		h.shareRepo.Delete(c.Request.Context(), share.ID)
		c.Error(ErrNotFound.WithMessage("分享链接下载次数已达上限"))
		return
	}

	// Validate file path
	validPath, err := h.fileManager.ValidatePath(share.FilePath)
	if err != nil {
		c.Error(ErrNotFound.WithMessage("文件不存在或已移动"))
		return
	}

	// Check file still exists
	info, err := os.Stat(validPath)
	if err != nil {
		// File was moved/deleted - remove share
		h.shareRepo.Delete(c.Request.Context(), share.ID)
		c.Error(ErrNotFound.WithMessage("文件不存在或已移动"))
		return
	}

	if info.IsDir() {
		h.shareRepo.Delete(c.Request.Context(), share.ID)
		c.Error(ErrBadRequest.WithMessage("文件类型无效"))
		return
	}

	// Increment download count
	h.shareRepo.IncrementDownloads(c.Request.Context(), share.ID)

	// Serve file
	f, err := os.OpenFile(validPath, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		c.Error(ErrInternal.Wrap(err))
		return
	}
	defer f.Close()

	contentType := detectContentType(validPath)
	extraHeaders := map[string]string{
		"Content-Disposition": fmt.Sprintf("inline; filename=%q", filepath.Base(validPath)),
	}
	if contentType == "application/octet-stream" {
		extraHeaders["Content-Disposition"] = fmt.Sprintf("attachment; filename=%q", filepath.Base(validPath))
	}

	c.DataFromReader(200, info.Size(), contentType, f, extraHeaders)
}

// registerFileShareRoutes registers file share management routes (protected)
func registerFileShareRoutes(protected *gin.RouterGroup, shareRepo fileshare.Repository, fileManager *filemanager.Manager) {
	handler := NewFileShareHandler(shareRepo, fileManager)

	protected.POST("/file-shares", handler.CreateShare)
	protected.GET("/file-shares", handler.ListShares)
	protected.DELETE("/file-shares/:id", handler.DeleteShare)
	protected.POST("/file-shares/cleanup", handler.CleanupExpired)
}

// RegisterPublicShareRoute registers the public download route (no auth)
func RegisterPublicShareRoute(e *gin.Engine, shareRepo fileshare.Repository, fileManager *filemanager.Manager) {
	handler := NewFileShareHandler(shareRepo, fileManager)
	e.GET("/share/:token", handler.PublicDownload)
}

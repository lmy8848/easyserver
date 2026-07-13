package api

import (
	"crypto/rand"
	"crypto/subtle"
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

// parseExpiresAt normalizes an expiry input to an absolute "2006-01-02 15:04:05"
// string. Accepts relative ("1h", "7d") or absolute ("2026-07-01 12:00:00").
// Empty input yields "". Malformed input yields an error (guard: the original
// create flow silently ignored bad relative input).
func parseExpiresAt(s string) (string, error) {
	if s == "" {
		return "", nil
	}
	if strings.HasSuffix(s, "h") || strings.HasSuffix(s, "d") {
		var duration time.Duration
		if strings.HasSuffix(s, "d") {
			val, err := strconv.Atoi(strings.TrimSuffix(s, "d"))
			if err != nil || val <= 0 {
				return "", fmt.Errorf("无效的过期时间: %s", s)
			}
			duration = time.Duration(val) * 24 * time.Hour
		} else {
			val, err := strconv.Atoi(strings.TrimSuffix(s, "h"))
			if err != nil || val <= 0 {
				return "", fmt.Errorf("无效的过期时间: %s", s)
			}
			duration = time.Duration(val) * time.Hour
		}
		return time.Now().Add(duration).Format("2006-01-02 15:04:05"), nil
	}
	if _, err := time.Parse("2006-01-02 15:04:05", s); err != nil {
		return "", fmt.Errorf("过期时间格式无效，支持 1h、7d 或 2026-07-01 12:00:00")
	}
	return s, nil
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

	// Validate + normalize expires_at (relative or absolute). Empty =永久有效.
	expiresAt, err := parseExpiresAt(req.ExpiresAt)
	if err != nil {
		c.Error(ErrBadRequest.WithMessage(err.Error()))
		return
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
		ExpiresAt:     expiresAt,
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
	HasPassword bool  `json:"has_password"`
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
		hasPwd := s.Password != ""
		s.Password = ""
		item := ShareListItem{FileShare: s, HasPassword: hasPwd}
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

// GetShare returns a single share owned by the current user, including its
// password so the frontend can reconstruct a full shareable link. Owner check
// is enforced; non-owners get 404 (no existence leak).
func (h *FileShareHandler) GetShare(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.Error(ErrBadRequest.WithMessage("无效的ID"))
		return
	}

	userID, _ := c.Get("user_id")
	uid, _ := userID.(int64)

	share, err := h.shareRepo.GetByID(c.Request.Context(), id)
	if err != nil {
		c.Error(ErrInternal.Wrap(err))
		return
	}
	if share == nil || share.CreatedBy != uid {
		c.Error(ErrNotFound.WithMessage("外链不存在"))
		return
	}
	Success(c, share)
}

// UpdateShare modifies a share's access-control fields (password / expiry /
// download cap). File path and token are immutable. Owner-gated; expires_at
// format and max_downloads are validated as guards.
func (h *FileShareHandler) UpdateShare(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.Error(ErrBadRequest.WithMessage("无效的ID"))
		return
	}

	var req fileshare.UpdateShareRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(ErrBadRequest.Wrap(err))
		return
	}

	userID, _ := c.Get("user_id")
	uid, _ := userID.(int64)

	share, err := h.shareRepo.GetByID(c.Request.Context(), id)
	if err != nil {
		c.Error(ErrInternal.Wrap(err))
		return
	}
	if share == nil || share.CreatedBy != uid {
		c.Error(ErrNotFound.WithMessage("外链不存在"))
		return
	}

	// Guard: validate max_downloads if provided.
	if req.MaxDownloads != nil && *req.MaxDownloads < 0 {
		c.Error(ErrBadRequest.WithMessage("最大下载次数不能为负数"))
		return
	}

	// Guard: validate + normalize expires_at (unless explicitly clearing).
	if !req.ClearExpiry {
		parsed, perr := parseExpiresAt(req.ExpiresAt)
		if perr != nil {
			c.Error(ErrBadRequest.WithMessage(perr.Error()))
			return
		}
		req.ExpiresAt = parsed
	}

	if err := h.shareRepo.Update(c.Request.Context(), id, &req); err != nil {
		c.Error(ErrInternal.Wrap(err))
		return
	}

	middleware.AuditSummary(c, "更新文件外链 #"+c.Param("id"))

	// Return refreshed record without leaking password to the list view.
	updated, err := h.shareRepo.GetByID(c.Request.Context(), id)
	if err != nil {
		c.Error(ErrInternal.Wrap(err))
		return
	}
	if updated != nil {
		updated.Password = ""
	}
	Success(c, updated)
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

// ShareInfoResponse is the public metadata for a share link. It intentionally
// excludes password, file_path, created_by and token so the public download
// page can render without leaking sensitive data.
type ShareInfoResponse struct {
	FileName      string `json:"file_name"`
	FileSize      int64  `json:"file_size"`
	Exists        bool   `json:"exists"`
	NeedsPassword bool   `json:"needs_password"`
	Expired       bool   `json:"expired"`
	DownloadsLeft int    `json:"downloads_left"` // -1 = unlimited
	DownloadCount int    `json:"download_count"`
	MaxDownloads  int    `json:"max_downloads"`
	ExpiresAt     string `json:"expires_at"`
}

// ShareInfo returns public, non-sensitive metadata about a share so the
// download page can render before requesting the file. No auth required.
func (h *FileShareHandler) ShareInfo(c *gin.Context) {
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

	resp := ShareInfoResponse{
		FileName:      share.FileName,
		FileSize:      share.FileSize,
		NeedsPassword: share.Password != "",
		MaxDownloads:  share.MaxDownloads,
		DownloadCount: share.DownloadCount,
		ExpiresAt:     share.ExpiresAt,
		DownloadsLeft: -1,
	}
	if share.MaxDownloads > 0 {
		resp.DownloadsLeft = share.MaxDownloads - share.DownloadCount
		if resp.DownloadsLeft < 0 {
			resp.DownloadsLeft = 0
		}
	}
	if share.ExpiresAt != "" {
		if expires, perr := time.Parse("2006-01-02 15:04:05", share.ExpiresAt); perr == nil {
			resp.Expired = time.Now().After(expires)
		}
	}
	// Check current file existence/size without leaking the path.
	if validPath, verr := h.fileManager.ValidatePath(share.FilePath); verr == nil {
		if info, serr := os.Stat(validPath); serr == nil && !info.IsDir() {
			resp.Exists = true
			resp.FileSize = info.Size()
		}
	}

	Success(c, resp)
}

// VerifyShare checks password/expiry/download-cap without serving the file or
// incrementing the download counter. Lets the download page validate a typed
// password before navigating to /download (which would otherwise render raw
// JSON on mismatch). No auth required.
func (h *FileShareHandler) VerifyShare(c *gin.Context) {
	token := c.Param("token")
	if token == "" {
		c.Error(ErrBadRequest.WithMessage("缺少分享令牌"))
		return
	}

	var body struct {
		Password string `json:"password"`
	}
	_ = c.ShouldBindJSON(&body)

	share, err := h.shareRepo.GetByToken(c.Request.Context(), token)
	if err != nil {
		c.Error(ErrInternal.Wrap(err))
		return
	}
	if share == nil {
		c.Error(ErrNotFound.WithMessage("分享链接不存在或已失效"))
		return
	}

	if share.Password != "" && subtle.ConstantTimeCompare([]byte(body.Password), []byte(share.Password)) != 1 {
		c.Error(ErrForbidden.WithMessage("密码错误"))
		return
	}
	if share.ExpiresAt != "" {
		if expires, perr := time.Parse("2006-01-02 15:04:05", share.ExpiresAt); perr == nil && time.Now().After(expires) {
			c.Error(ErrNotFound.WithMessage("分享链接已过期"))
			return
		}
	}
	if share.MaxDownloads > 0 && share.DownloadCount >= share.MaxDownloads {
		c.Error(ErrNotFound.WithMessage("分享链接下载次数已达上限"))
		return
	}
	if validPath, verr := h.fileManager.ValidatePath(share.FilePath); verr == nil {
		if info, serr := os.Stat(validPath); serr != nil || info.IsDir() {
			c.Error(ErrNotFound.WithMessage("文件不存在或已移动"))
			return
		}
	}

	Success(c, gin.H{"ok": true})
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

	// Check password (constant-time to avoid timing oracle)
	if share.Password != "" {
		password := c.Query("password")
		if password == "" {
			c.Error(ErrForbidden.WithMessage("需要密码访问，请在链接后添加 ?password=xxx"))
			return
		}
		if subtle.ConstantTimeCompare([]byte(password), []byte(share.Password)) != 1 {
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

	// Atomically increment download count, refusing if the cap is reached. This
	// closes the check-then-increment race where concurrent requests could push
	// DownloadCount past MaxDownloads.
	allowed, err := h.shareRepo.IncrementDownloadsIfUnderLimit(c.Request.Context(), share.ID)
	if err != nil {
		c.Error(ErrInternal.Wrap(err))
		return
	}
	if !allowed {
		h.shareRepo.Delete(c.Request.Context(), share.ID)
		c.Error(ErrNotFound.WithMessage("分享链接下载次数已达上限"))
		return
	}

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
	protected.GET("/file-shares/:id", handler.GetShare)
	protected.PUT("/file-shares/:id", handler.UpdateShare)
	protected.DELETE("/file-shares/:id", handler.DeleteShare)
	protected.POST("/file-shares/cleanup", handler.CleanupExpired)
}

// RegisterPublicShareRoute registers the public share routes (no auth).
// /share/:token is intentionally NOT registered here so it falls through to
// the SPA fallback (NoRoute) and renders the React download page; the info
// and download sub-paths are explicit and take precedence over NoRoute.
func RegisterPublicShareRoute(e *gin.Engine, shareRepo fileshare.Repository, fileManager *filemanager.Manager, rateLimit int, rateInterval time.Duration) {
	handler := NewFileShareHandler(shareRepo, fileManager)
	// Public share endpoints are unauthenticated, so rate-limit by IP to blunt
	// password brute-force on /verify and download abuse on /download.
	g := e.Group("/share")
	if rateLimit > 0 {
		g.Use(middleware.RateLimitMiddleware("share", rateLimit, rateInterval))
	}
	g.GET("/:token/info", handler.ShareInfo)
	g.POST("/:token/verify", handler.VerifyShare)
	g.GET("/:token/download", handler.PublicDownload)
}

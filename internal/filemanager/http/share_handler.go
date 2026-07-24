package http

import (
	archive_zip "archive/zip"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"easyserver/internal/filemanager"
	"easyserver/internal/httpx"
	"easyserver/internal/httpx/middleware"
	"easyserver/internal/infra/apperror"
	"easyserver/internal/infra/config"

	"github.com/gin-gonic/gin"
)

type FileShareHandler struct {
	shareRepo   filemanager.ShareRepository
	fileManager *filemanager.Manager
	cfg         *config.Config
}

func NewFileShareHandler(shareRepo filemanager.ShareRepository, fm *filemanager.Manager, cfg *config.Config) *FileShareHandler {
	return &FileShareHandler{shareRepo: shareRepo, fileManager: fm, cfg: cfg}
}

// (detectContentType removed to reuse filemanager.GetMimeType)

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
	if strings.HasSuffix(s, "m") || strings.HasSuffix(s, "h") || strings.HasSuffix(s, "d") {
		var duration time.Duration
		if strings.HasSuffix(s, "d") {
			val, err := strconv.Atoi(strings.TrimSuffix(s, "d"))
			if err != nil || val <= 0 {
				return "", fmt.Errorf("无效的过期时间: %s", s)
			}
			duration = time.Duration(val) * 24 * time.Hour
		} else if strings.HasSuffix(s, "h") {
			val, err := strconv.Atoi(strings.TrimSuffix(s, "h"))
			if err != nil || val <= 0 {
				return "", fmt.Errorf("无效的过期时间: %s", s)
			}
			duration = time.Duration(val) * time.Hour
		} else {
			val, err := strconv.Atoi(strings.TrimSuffix(s, "m"))
			if err != nil || val <= 0 {
				return "", fmt.Errorf("无效的过期时间: %s", s)
			}
			duration = time.Duration(val) * time.Minute
		}
		return time.Now().Add(duration).Format("2006-01-02 15:04:05"), nil
	}
	if _, err := time.Parse("2006-01-02 15:04:05", s); err != nil {
		return "", fmt.Errorf("过期时间格式无效，支持 30m、1h、7d 或 2026-07-01 12:00:00")
	}
	return s, nil
}

// CreateShare creates a new file share link
func (h *FileShareHandler) CreateShare(c *gin.Context) {
	var req filemanager.CreateShareRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperror.ErrBadRequest.Wrap(err))
		return
	}

	// Validate file path
	validPath, err := h.fileManager.ValidatePath(req.FilePath)
	if err != nil {
		c.Error(apperror.ErrBadRequest.WithMessage("文件路径无效"))
		return
	}

	// Check file exists
	info, err := os.Stat(validPath)
	if err != nil {
		c.Error(apperror.ErrNotFound.WithMessage("文件不存在"))
		return
	}

	// Check file size limit (max 500MB)
	if info.Size() > fileShareMaxSize {
		c.Error(apperror.ErrBadRequest.WithMessage("文件超过500MB，不支持分享"))
		return
	}

	middleware.AuditSummary(c, "生成文件外链 "+req.FilePath)

	userID, _ := c.Get("user_id")
	uid, _ := userID.(int64)

	// Validate + normalize expires_at (relative or absolute). Empty =永久有效.
	expiresAt, err := parseExpiresAt(req.ExpiresAt)
	if err != nil {
		c.Error(apperror.ErrBadRequest.WithMessage(err.Error()))
		return
	}

	token, err := generateToken()
	if err != nil {
		c.Error(apperror.ErrInternal.Wrap(err))
		return
	}

	share := &filemanager.FileShare{
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
		c.Error(apperror.ErrInternal.Wrap(err))
		return
	}
	share.ID = id
	// Don't expose password in response
	share.Password = ""
	httpx.Success(c, share)
}

// ShareListItem is an enriched share record with current file status.
type ShareListItem struct {
	filemanager.FileShare
	FileExists  bool  `json:"file_exists"`
	IsDir       bool  `json:"is_dir"`
	CurrentSize int64 `json:"current_size"`
	HasPassword bool  `json:"has_password"`
}

// ListShares lists all file shares for the current user
func (h *FileShareHandler) ListShares(c *gin.Context) {
	userID, _ := c.Get("user_id")
	uid, _ := userID.(int64)

	shares, err := h.shareRepo.List(c.Request.Context(), uid)
	if err != nil {
		c.Error(apperror.ErrInternal.Wrap(err))
		return
	}

	items := make([]ShareListItem, 0, len(shares))
	for _, s := range shares {
		hasPwd := s.Password != ""
		s.Password = ""
		item := ShareListItem{FileShare: s, HasPassword: hasPwd}
		validPath, err := h.fileManager.ValidatePath(s.FilePath)
		if err == nil {
			if info, err := os.Stat(validPath); err == nil {
				item.IsDir = info.IsDir()
				item.FileExists = true
				item.CurrentSize = info.Size()
			}
		}
		items = append(items, item)
	}
	httpx.Success(c, items)
}

// GetShare returns a single share owned by the current user, including its
// password so the frontend can reconstruct a full shareable link. Owner check
// is enforced; non-owners get 404 (no existence leak).
func (h *FileShareHandler) GetShare(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.Error(apperror.ErrBadRequest.WithMessage("无效的ID"))
		return
	}

	userID, _ := c.Get("user_id")
	uid, _ := userID.(int64)

	share, err := h.shareRepo.GetByID(c.Request.Context(), id)
	if err != nil {
		c.Error(apperror.ErrInternal.Wrap(err))
		return
	}
	if share == nil || share.CreatedBy != uid {
		c.Error(apperror.ErrNotFound.WithMessage("外链不存在"))
		return
	}
	httpx.Success(c, share)
}

// UpdateShare modifies a share's access-control fields (password / expiry /
// download cap). File path and token are immutable. Owner-gated; expires_at
// format and max_downloads are validated as guards.
func (h *FileShareHandler) UpdateShare(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.Error(apperror.ErrBadRequest.WithMessage("无效的ID"))
		return
	}

	var req filemanager.UpdateShareRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperror.ErrBadRequest.Wrap(err))
		return
	}

	userID, _ := c.Get("user_id")
	uid, _ := userID.(int64)

	share, err := h.shareRepo.GetByID(c.Request.Context(), id)
	if err != nil {
		c.Error(apperror.ErrInternal.Wrap(err))
		return
	}
	if share == nil || share.CreatedBy != uid {
		c.Error(apperror.ErrNotFound.WithMessage("外链不存在"))
		return
	}

	// Guard: validate max_downloads if provided.
	if req.MaxDownloads != nil && *req.MaxDownloads < 0 {
		c.Error(apperror.ErrBadRequest.WithMessage("最大下载次数不能为负数"))
		return
	}

	// Guard: validate + normalize expires_at (unless explicitly clearing).
	if !req.ClearExpiry {
		parsed, perr := parseExpiresAt(req.ExpiresAt)
		if perr != nil {
			c.Error(apperror.ErrBadRequest.WithMessage(perr.Error()))
			return
		}
		req.ExpiresAt = parsed
	}

	if err := h.shareRepo.Update(c.Request.Context(), id, &req); err != nil {
		c.Error(apperror.ErrInternal.Wrap(err))
		return
	}

	middleware.AuditSummary(c, "更新文件外链 #"+c.Param("id"))

	// Return refreshed record without leaking password to the list view.
	updated, err := h.shareRepo.GetByID(c.Request.Context(), id)
	if err != nil {
		c.Error(apperror.ErrInternal.Wrap(err))
		return
	}
	if updated != nil {
		updated.Password = ""
	}
	httpx.Success(c, updated)
}

// DeleteShare revokes a file share
func (h *FileShareHandler) DeleteShare(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.Error(apperror.ErrBadRequest.WithMessage("无效的ID"))
		return
	}

	middleware.AuditSummary(c, "撤销文件外链 #"+idStr)
	if err := h.shareRepo.Delete(c.Request.Context(), id); err != nil {
		c.Error(apperror.ErrInternal.Wrap(err))
		return
	}
	httpx.Success(c, nil)
}

// CleanupExpired removes expired file shares
func (h *FileShareHandler) CleanupExpired(c *gin.Context) {
	count, err := h.shareRepo.DeleteExpired(c.Request.Context())
	if err != nil {
		c.Error(apperror.ErrInternal.Wrap(err))
		return
	}
	httpx.Success(c, gin.H{"deleted": count})
}

// ShareInfoResponse is the public metadata for a share link. It intentionally
// excludes password, file_path, created_by and token so the public download
// page can render without leaking sensitive data.
type ShareInfoResponse struct {
	FileName      string `json:"file_name"`
	FileSize      int64  `json:"file_size"`
	IsDir         bool   `json:"is_dir"`
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
		c.Error(apperror.ErrBadRequest.WithMessage("缺少分享令牌"))
		return
	}

	share, err := h.shareRepo.GetByToken(c.Request.Context(), token)
	if err != nil {
		c.Error(apperror.ErrInternal.Wrap(err))
		return
	}
	if share == nil {
		c.Error(apperror.ErrNotFound.WithMessage("分享链接不存在或已失效"))
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
		if info, serr := os.Stat(validPath); serr == nil {
			resp.IsDir = info.IsDir()
			resp.Exists = true
			resp.FileSize = info.Size()
		}
	}

	httpx.Success(c, resp)
}

var (
	ticketSecret []byte
	ticketInit   sync.Once
)

func getTicketSecret() []byte {
	ticketInit.Do(func() {
		ticketSecret = make([]byte, 32)
		rand.Read(ticketSecret)
	})
	return ticketSecret
}

type TicketRequest struct {
	Password string `json:"password"`
}

// GetTicket verifies access and issues a stateless download ticket.
// This is the ONLY place where download counts are incremented, preventing
// double counting from browser retries or multi-threaded download managers.
func (h *FileShareHandler) GetTicket(c *gin.Context) {
	token := c.Param("token")
	if token == "" {
		c.Error(apperror.ErrBadRequest.WithMessage("缺少分享令牌"))
		return
	}

	var req TicketRequest
	c.ShouldBindJSON(&req) // ignore error, password might be empty

	share, err := h.shareRepo.GetByToken(c.Request.Context(), token)
	if err != nil {
		c.Error(apperror.ErrInternal.Wrap(err))
		return
	}
	if share == nil {
		c.Error(apperror.ErrNotFound.WithMessage("分享链接不存在或已失效"))
		return
	}

	// Check password
	if share.Password != "" {
		if req.Password == "" {
			c.Error(apperror.ErrForbidden.WithMessage("需要输入密码"))
			return
		}
		if subtle.ConstantTimeCompare([]byte(req.Password), []byte(share.Password)) != 1 {
			c.Error(apperror.ErrForbidden.WithMessage("密码错误"))
			return
		}
	}

	// Check expiration
	if share.ExpiresAt != "" {
		expires, err := time.Parse("2006-01-02 15:04:05", share.ExpiresAt)
		if err == nil && time.Now().After(expires) {
			h.shareRepo.Delete(c.Request.Context(), share.ID)
			c.Error(apperror.ErrNotFound.WithMessage("分享链接已过期"))
			return
		}
	}

	// Validate file exists
	validPath, err := h.fileManager.ValidatePath(share.FilePath)
	if err != nil {
		c.Error(apperror.ErrNotFound.WithMessage("文件不存在或已移动"))
		return
	}
	_, err = os.Stat(validPath)
	if err != nil {
		h.shareRepo.Delete(c.Request.Context(), share.ID)
		c.Error(apperror.ErrNotFound.WithMessage("文件不可用"))
		return
	}

	// Atomically increment count
	allowed, err := h.shareRepo.IncrementDownloadsIfUnderLimit(c.Request.Context(), share.ID)
	if err != nil {
		c.Error(apperror.ErrInternal.Wrap(err))
		return
	}
	if !allowed {
		h.shareRepo.Delete(c.Request.Context(), share.ID)
		c.Error(apperror.ErrNotFound.WithMessage("分享链接下载次数已达上限"))
		return
	}

	// Issue stateless ticket valid for 6 hours
	exp := time.Now().Add(6 * time.Hour).Unix()
	msg := fmt.Sprintf("%d.%d", share.ID, exp)
	mac := hmac.New(sha256.New, getTicketSecret())
	mac.Write([]byte(msg))
	sig := hex.EncodeToString(mac.Sum(nil))

	httpx.Success(c, gin.H{"ticket": fmt.Sprintf("%s.%s", msg, sig)})
}

// PublicDownload handles public file download via a stateless ticket.
// Turnstile is NOT checked here: the download endpoint is already protected by
// IP rate limiting, password verification, and download-count caps. The SPA
// download page (/share/:token) can optionally enforce Turnstile before
// redirecting users here.

func (h *FileShareHandler) validateTicket(share *filemanager.FileShare, ticket string) error {
	parts := strings.Split(ticket, ".")
	if len(parts) != 3 {
		return fmt.Errorf("凭证无效")
	}
	if parts[0] != strconv.FormatInt(share.ID, 10) {
		return fmt.Errorf("凭证无效")
	}
	exp, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil || time.Now().Unix() > exp {
		return fmt.Errorf("凭证已过期")
	}

	msg := parts[0] + "." + parts[1]
	mac := hmac.New(sha256.New, getTicketSecret())
	mac.Write([]byte(msg))
	expectedSig := hex.EncodeToString(mac.Sum(nil))
	if subtle.ConstantTimeCompare([]byte(parts[2]), []byte(expectedSig)) != 1 {
		return fmt.Errorf("凭证无效")
	}
	return nil
}

func (h *FileShareHandler) PublicList(c *gin.Context) {
	token := c.Param("token")
	ticket := c.Query("ticket")
	subpath := c.Query("subpath")
	if token == "" || ticket == "" {
		c.Error(apperror.ErrBadRequest.WithMessage("缺少令牌或凭证"))
		return
	}

	share, err := h.shareRepo.GetByToken(c.Request.Context(), token)
	if err != nil {
		c.Error(apperror.ErrInternal.Wrap(err))
		return
	}
	if share == nil {
		c.Error(apperror.ErrNotFound.WithMessage("分享链接已失效"))
		return
	}

	if err := h.validateTicket(share, ticket); err != nil {
		c.Error(apperror.ErrForbidden.WithMessage(err.Error()))
		return
	}

	validPath, err := h.fileManager.ValidatePath(share.FilePath)
	if err != nil {
		c.Error(apperror.ErrNotFound.WithMessage("文件不存在或已移动"))
		return
	}

	info, err := os.Stat(validPath)
	if err != nil || !info.IsDir() {
		c.Error(apperror.ErrBadRequest.WithMessage("该分享不是一个文件夹"))
		return
	}

	targetDir, err := h.fileManager.ResolveShareSubpath(validPath, subpath)
	if err != nil {
		c.Error(apperror.ErrBadRequest.WithMessage("非法路径"))
		return
	}

	entries, err := os.ReadDir(targetDir)
	if err != nil {
		c.Error(apperror.ErrInternal.Wrap(err))
		return
	}

	type Entry struct {
		Name  string `json:"name"`
		Size  int64  `json:"size"`
		IsDir bool   `json:"is_dir"`
	}
	var res []Entry
	for _, e := range entries {
		info, err := e.Info()
		if err != nil {
			continue
		}
		res = append(res, Entry{
			Name:  e.Name(),
			Size:  info.Size(),
			IsDir: e.IsDir(),
		})
	}
	httpx.Success(c, res)
}

func (h *FileShareHandler) PublicDownload(c *gin.Context) {
	token := c.Param("token")
	ticket := c.Query("ticket")
	subpath := c.Query("subpath")

	if token == "" || ticket == "" {
		c.Error(apperror.ErrBadRequest.WithMessage("缺少令牌或凭证"))
		return
	}

	share, err := h.shareRepo.GetByToken(c.Request.Context(), token)
	if err != nil {
		c.Error(apperror.ErrInternal.Wrap(err))
		return
	}
	if share == nil {
		c.Error(apperror.ErrNotFound.WithMessage("分享链接已失效"))
		return
	}

	if err := h.validateTicket(share, ticket); err != nil {
		c.Error(apperror.ErrForbidden.WithMessage(err.Error()))
		return
	}

	validPath, err := h.fileManager.ValidatePath(share.FilePath)
	if err != nil {
		c.Error(apperror.ErrNotFound.WithMessage("文件不存在或已移动"))
		return
	}
	info, err := os.Stat(validPath)
	if err != nil {
		c.Error(apperror.ErrNotFound.WithMessage("文件不可用"))
		return
	}

	targetPath := validPath
	if info.IsDir() && subpath != "" {
		resolved, err := h.fileManager.ResolveShareSubpath(validPath, subpath)
		if err != nil {
			c.Error(apperror.ErrBadRequest.WithMessage("非法路径"))
			return
		}
		targetPath = resolved
	}

	targetInfo, err := os.Stat(targetPath)
	if err != nil {
		c.Error(apperror.ErrNotFound.WithMessage("文件不可用"))
		return
	}

	if targetInfo.IsDir() {
		// Zip and stream the directory
		c.Header("Content-Type", "application/zip")
		c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filepath.Base(targetPath)+".zip"))
		c.Writer.WriteHeader(200)

		zw := archive_zip.NewWriter(c.Writer)
		defer zw.Close()

		filepath.Walk(targetPath, func(path string, winfo os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if winfo.IsDir() {
				return nil
			}
			// validateRealPath: refuse to read through a symlink planted inside the
			// shared tree whose target points outside (TOCTOU + symlink-escape guard).
			if vErr := h.fileManager.ValidateWalkPath(path); vErr != nil {
				return nil
			}
			rel, err := filepath.Rel(targetPath, path)
			if err != nil {
				return err
			}
			f, err := zw.Create(rel)
			if err != nil {
				return err
			}
			// O_NOFOLLOW: don't pack the contents of a symlink — the entry itself
			// could be a symlink whose target is outside the sandbox.
			sf, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
			if err != nil {
				return nil
			}
			defer sf.Close()
			_, err = io.Copy(f, sf)
			return err
		})
		return
	}

	// Serve single file — O_NOFOLLOW guards the TOCTOU window between
	// ResolveShareSubpath's EvalSymlinks and the actual read.
	f, err := os.OpenFile(targetPath, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		c.Error(apperror.ErrInternal.Wrap(err))
		return
	}
	defer f.Close()

	contentType, _ := h.fileManager.GetMimeType(targetPath)
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	extraHeaders := map[string]string{
		"Content-Disposition": fmt.Sprintf("inline; filename=%q", filepath.Base(targetPath)),
	}
	if contentType == "application/octet-stream" {
		extraHeaders["Content-Disposition"] = fmt.Sprintf("attachment; filename=%q", filepath.Base(targetPath))
	}

	c.DataFromReader(200, targetInfo.Size(), contentType, f, extraHeaders)
}

// RegisterRoutes registers file share management routes (protected)
func RegisterShareRoutes(protected *gin.RouterGroup, shareRepo filemanager.ShareRepository, fileManager *filemanager.Manager, cfg *config.Config) {
	handler := NewFileShareHandler(shareRepo, fileManager, cfg)

	protected.POST("/shares", handler.CreateShare)
	protected.GET("/shares", handler.ListShares)
	protected.GET("/shares/:id", handler.GetShare)
	protected.PUT("/shares/:id", handler.UpdateShare)
	protected.DELETE("/shares/:id", handler.DeleteShare)
	protected.POST("/shares/cleanup", handler.CleanupExpired)
}

// RegisterPublicShareRoute registers the public share routes (no auth).
func RegisterPublicShareRoute(public *gin.RouterGroup, shareRepo filemanager.ShareRepository, fileManager *filemanager.Manager, rateLimit int, rateInterval time.Duration, cfg *config.Config) {
	handler := NewFileShareHandler(shareRepo, fileManager, cfg)
	g := public.Group("/shares/public")
	if rateLimit > 0 {
		g.Use(middleware.RateLimitMiddleware("share", rateLimit, rateInterval))
	}
	g.GET("/:token/info", handler.ShareInfo)
	g.POST("/:token/ticket", handler.GetTicket)
	g.GET("/:token/download", handler.PublicDownload)
	g.GET("/:token/list", handler.PublicList)
}

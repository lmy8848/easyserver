package http

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
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
	if info.IsDir() {
		c.Error(apperror.ErrBadRequest.WithMessage("不支持分享目录"))
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
			if info, err := os.Stat(validPath); err == nil && !info.IsDir() {
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
		if info, serr := os.Stat(validPath); serr == nil && !info.IsDir() {
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
	info, err := os.Stat(validPath)
	if err != nil || info.IsDir() {
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
func (h *FileShareHandler) PublicDownload(c *gin.Context) {
	token := c.Param("token")
	ticket := c.Query("ticket")
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

	// Verify stateless ticket
	parts := strings.Split(ticket, ".")
	if len(parts) != 3 {
		c.Error(apperror.ErrForbidden.WithMessage("无效的下载凭证"))
		return
	}
	if parts[0] != strconv.FormatInt(share.ID, 10) {
		c.Error(apperror.ErrForbidden.WithMessage("凭证不匹配"))
		return
	}
	exp, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil || time.Now().Unix() > exp {
		c.Error(apperror.ErrForbidden.WithMessage("凭证已过期，请刷新页面重新获取"))
		return
	}

	msg := parts[0] + "." + parts[1]
	mac := hmac.New(sha256.New, getTicketSecret())
	mac.Write([]byte(msg))
	expectedSig := hex.EncodeToString(mac.Sum(nil))
	if subtle.ConstantTimeCompare([]byte(parts[2]), []byte(expectedSig)) != 1 {
		c.Error(apperror.ErrForbidden.WithMessage("凭证签名无效"))
		return
	}

	// Validate file path
	validPath, err := h.fileManager.ValidatePath(share.FilePath)
	if err != nil {
		c.Error(apperror.ErrNotFound.WithMessage("文件不存在或已移动"))
		return
	}
	info, err := os.Stat(validPath)
	if err != nil || info.IsDir() {
		c.Error(apperror.ErrNotFound.WithMessage("文件不可用"))
		return
	}

	f, err := os.OpenFile(validPath, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		c.Error(apperror.ErrInternal.Wrap(err))
		return
	}
	defer f.Close()

	contentType, _ := h.fileManager.GetMimeType(share.FilePath)
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	extraHeaders := map[string]string{
		"Content-Disposition": fmt.Sprintf("inline; filename=%q", filepath.Base(validPath)),
	}
	if contentType == "application/octet-stream" {
		extraHeaders["Content-Disposition"] = fmt.Sprintf("attachment; filename=%q", filepath.Base(validPath))
	}

	c.DataFromReader(200, info.Size(), contentType, f, extraHeaders)
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
}

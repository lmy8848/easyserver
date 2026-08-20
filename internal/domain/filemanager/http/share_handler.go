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

	"easyserver/internal/domain/filemanager"
	"easyserver/internal/httpx"
	"easyserver/internal/httpx/middleware"
	"easyserver/internal/infra/config"
	"easyserver/internal/infra/errx"
	"easyserver/internal/util"

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
	return util.RandomHex(fileShareTokenBytes)
}

// parseExpiresAt normalizes an expiry input to an absolute util.TimeLayout
// string. Accepts relative ("1h", "7d") or absolute ("YYYY-MM-DD HH:MM:SS").
// Empty input yields "". Malformed input yields an error (guard: the original
// create flow silently ignored bad relative input).
func parseExpiresAt(s string) (string, error) {
	if s == "" {
		return "", nil
	}
	if strings.HasSuffix(s, "m") || strings.HasSuffix(s, "h") || strings.HasSuffix(s, "d") {
		var duration time.Duration
		if before, ok := strings.CutSuffix(s, "d"); ok {
			val, err := strconv.Atoi(before)
			if err != nil || val <= 0 {
				return "", fmt.Errorf("无效的过期时间: %s", s)
			}
			duration = time.Duration(val) * 24 * time.Hour
		} else if before, ok := strings.CutSuffix(s, "h"); ok {
			val, err := strconv.Atoi(before)
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
		return time.Now().Add(duration).Format(util.TimeLayout), nil
	}
	if _, err := time.Parse(util.TimeLayout, s); err != nil {
		return "", errx.BadRequest("过期时间格式无效，支持 30m、1h、7d 或 YYYY-MM-DD HH:MM:SS")
	}
	return s, nil
}

// CreateShare creates a new file share link
func (h *FileShareHandler) CreateShare(c *gin.Context) (any, error) {
	req, err := httpx.BindJSON[filemanager.CreateShareRequest](c)
	if err != nil {
		return nil, err
	}

	// Validate file path
	if util.IsTraversal(req.FilePath) {
		return nil, errx.Forbidden("文件路径包含非法字符")
	}
	validPath, err := h.fileManager.ValidatePath(req.FilePath)
	if err != nil {
		return nil, errx.BadRequest("文件路径无效")
	}

	// Check file exists
	info, err := os.Stat(validPath)
	if err != nil {
		return nil, errx.NotFound("文件不存在")
	}

	// Check file size limit (max 500MB)
	if info.Size() > fileShareMaxSize {
		return nil, errx.BadRequest("文件超过500MB，不支持分享")
	}

	middleware.AuditSummary(c, "生成文件外链 "+req.FilePath)

	userID, _ := c.Get("user_id")
	uid, _ := userID.(int64)

	// Validate + normalize expires_at (relative or absolute). Empty =永久有效.
	expiresAt, err := parseExpiresAt(req.ExpiresAt)
	if err != nil {
		return nil, errx.BadRequest("%w", err)
	}

	token, err := generateToken()
	if err != nil {
		return nil, errx.Internal("生成令牌失败: %w", err)
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
		return nil, err
	}
	share.ID = id
	// Don't expose password in response
	share.Password = ""
	return share, nil
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
func (h *FileShareHandler) ListShares(c *gin.Context) (any, error) {
	p := httpx.ParsePagination(c, 20, 100)
	userID, _ := c.Get("user_id")
	uid, _ := userID.(int64)

	shares, err := h.shareRepo.List(c.Request.Context(), uid)
	if err != nil {
		return nil, err
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
	return httpx.Paginate(items, p), nil
}

// GetShare returns a single share owned by the current user, including its
// password so the frontend can reconstruct a full shareable link. Owner check
// is enforced; non-owners get 404 (no existence leak).
func (h *FileShareHandler) GetShare(c *gin.Context) (any, error) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return nil, errx.BadRequest("无效的ID")
	}

	userID, _ := c.Get("user_id")
	uid, _ := userID.(int64)

	share, err := h.shareRepo.GetByID(c.Request.Context(), id)
	if err != nil {
		return nil, err
	}
	if share == nil || share.CreatedBy != uid {
		return nil, errx.NotFound("外链不存在")
	}
	return share, nil
}

// UpdateShare modifies a share's access-control fields (password / expiry /
// download cap). File path and token are immutable. Owner-gated; expires_at
// format and max_downloads are validated as guards.
func (h *FileShareHandler) UpdateShare(c *gin.Context) (any, error) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return nil, errx.BadRequest("无效的ID")
	}

	req, err := httpx.BindJSON[filemanager.UpdateShareRequest](c)
	if err != nil {
		return nil, err
	}

	userID, _ := c.Get("user_id")
	uid, _ := userID.(int64)

	share, err := h.shareRepo.GetByID(c.Request.Context(), id)
	if err != nil {
		return nil, err
	}
	if share == nil || share.CreatedBy != uid {
		return nil, errx.NotFound("外链不存在")
	}

	// Guard: validate max_downloads if provided.
	if req.MaxDownloads != nil && *req.MaxDownloads < 0 {
		return nil, errx.BadRequest("最大下载次数不能为负数")
	}

	// Guard: validate + normalize expires_at (unless explicitly clearing).
	if !req.ClearExpiry {
		parsed, perr := parseExpiresAt(req.ExpiresAt)
		if perr != nil {
			return nil, errx.BadRequest("%w", perr)
		}
		req.ExpiresAt = parsed
	}

	if err := h.shareRepo.Update(c.Request.Context(), id, &req); err != nil {
		return nil, err
	}

	middleware.AuditSummary(c, "更新文件外链 #"+c.Param("id"))

	// Return refreshed record without leaking password to the list view.
	updated, err := h.shareRepo.GetByID(c.Request.Context(), id)
	if err != nil {
		return nil, err
	}
	if updated != nil {
		updated.Password = ""
	}
	return updated, nil
}

// DeleteShare revokes a file share
func (h *FileShareHandler) DeleteShare(c *gin.Context) (any, error) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		return nil, errx.BadRequest("无效的ID")
	}

	middleware.AuditSummary(c, "撤销文件外链 #"+idStr)
	if err := h.shareRepo.Delete(c.Request.Context(), id); err != nil {
		return nil, err
	}
	return nil, nil
}

// CleanupExpired removes expired file shares
func (h *FileShareHandler) CleanupExpired(c *gin.Context) (any, error) {
	count, err := h.shareRepo.DeleteExpired(c.Request.Context())
	if err != nil {
		return nil, err
	}
	return gin.H{"deleted": count}, nil
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
func (h *FileShareHandler) ShareInfo(c *gin.Context) (any, error) {
	token := c.Param("token")
	if token == "" {
		return nil, errx.BadRequest("缺少分享令牌")
	}

	share, err := h.shareRepo.GetByToken(c.Request.Context(), token)
	if err != nil {
		return nil, err
	}
	if share == nil {
		return nil, errx.NotFound("分享链接不存在或已失效")
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
		resp.DownloadsLeft = max(share.MaxDownloads-share.DownloadCount, 0)
	}
	if share.ExpiresAt != "" {
		if expires, perr := time.Parse(util.TimeLayout, share.ExpiresAt); perr == nil {
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

	return resp, nil
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
func (h *FileShareHandler) GetTicket(c *gin.Context) (any, error) {
	token := c.Param("token")
	if token == "" {
		return nil, errx.BadRequest("缺少分享令牌")
	}

	req, err := httpx.BindJSON[TicketRequest](c)
	if err != nil {
		return nil, err
	}

	share, err := h.shareRepo.GetByToken(c.Request.Context(), token)
	if err != nil {
		return nil, err
	}
	if share == nil {
		return nil, errx.NotFound("分享链接不存在或已失效")
	}

	// Check password
	if share.Password != "" {
		if req.Password == "" {
			return nil, errx.Forbidden("需要输入密码")
		}
		if subtle.ConstantTimeCompare([]byte(req.Password), []byte(share.Password)) != 1 {
			return nil, errx.Forbidden("密码错误")
		}
	}

	// Check expiration
	if share.ExpiresAt != "" {
		expires, err := time.Parse(util.TimeLayout, share.ExpiresAt)
		if err == nil && time.Now().After(expires) {
			if delErr := h.shareRepo.Delete(c.Request.Context(), share.ID); delErr != nil {
				return nil, delErr
			}
			return nil, errx.NotFound("分享链接已过期")
		}
	}

	// Validate file exists
	validPath, err := h.fileManager.ValidatePath(share.FilePath)
	if err != nil {
		return nil, errx.NotFound("文件不存在或已移动")
	}
	_, err = os.Stat(validPath)
	if err != nil {
		if delErr := h.shareRepo.Delete(c.Request.Context(), share.ID); delErr != nil {
			return nil, delErr
		}
		return nil, errx.NotFound("文件不可用")
	}

	// Atomically increment count
	allowed, err := h.shareRepo.IncrementDownloadsIfUnderLimit(c.Request.Context(), share.ID)
	if err != nil {
		return nil, err
	}
	if !allowed {
		if delErr := h.shareRepo.Delete(c.Request.Context(), share.ID); delErr != nil {
			return nil, delErr
		}
		return nil, errx.NotFound("分享链接下载次数已达上限")
	}

	// Issue stateless ticket valid for 6 hours
	exp := time.Now().Add(6 * time.Hour).Unix()
	msg := fmt.Sprintf("%d.%d", share.ID, exp)
	mac := hmac.New(sha256.New, getTicketSecret())
	mac.Write([]byte(msg))
	sig := hex.EncodeToString(mac.Sum(nil))

	return gin.H{"ticket": fmt.Sprintf("%s.%s", msg, sig)}, nil
}

// PublicDownload handles public file download via a stateless ticket.
// Turnstile is NOT checked here: the download endpoint is already protected by
// IP rate limiting, password verification, and download-count caps. The SPA
// download page (/share/:token) can optionally enforce Turnstile before
// redirecting users here.

func (h *FileShareHandler) validateTicket(share *filemanager.FileShare, ticket string) error {
	parts := strings.Split(ticket, ".")
	if len(parts) != 3 {
		return filemanager.ErrInvalidTicket
	}
	if parts[0] != strconv.FormatInt(share.ID, 10) {
		return filemanager.ErrInvalidTicket
	}
	exp, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil || time.Now().Unix() > exp {
		return filemanager.ErrTicketExpired
	}

	msg := parts[0] + "." + parts[1]
	mac := hmac.New(sha256.New, getTicketSecret())
	mac.Write([]byte(msg))
	expectedSig := hex.EncodeToString(mac.Sum(nil))
	if subtle.ConstantTimeCompare([]byte(parts[2]), []byte(expectedSig)) != 1 {
		return filemanager.ErrInvalidTicket
	}
	return nil
}

func (h *FileShareHandler) PublicList(c *gin.Context) (any, error) {
	token := c.Param("token")
	ticket := c.Query("ticket")
	subpath := c.Query("subpath")
	if util.IsTraversal(subpath) {
		return nil, errx.Forbidden("非法路径")
	}
	if token == "" || ticket == "" {
		return nil, errx.BadRequest("缺少令牌或凭证")
	}

	share, err := h.shareRepo.GetByToken(c.Request.Context(), token)
	if err != nil {
		return nil, err
	}
	if share == nil {
		return nil, errx.NotFound("分享链接已失效")
	}

	if err := h.validateTicket(share, ticket); err != nil {
		return nil, errx.Forbidden("%w", err)
	}

	validPath, err := h.fileManager.ValidatePath(share.FilePath)
	if err != nil {
		return nil, errx.NotFound("文件不存在或已移动")
	}

	info, err := os.Stat(validPath)
	if err != nil || !info.IsDir() {
		return nil, errx.BadRequest("该分享不是一个文件夹")
	}

	targetDir, err := h.fileManager.ResolveShareSubpath(validPath, subpath)
	if err != nil {
		return nil, errx.BadRequest("非法路径")
	}

	entries, err := os.ReadDir(targetDir)
	if err != nil {
		return nil, err
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
	return res, nil
}

func (h *FileShareHandler) PublicDownload(c *gin.Context) (any, error) {
	token := c.Param("token")
	ticket := c.Query("ticket")
	subpath := c.Query("subpath")
	if util.IsTraversal(subpath) {
		return nil, errx.Forbidden("非法路径")
	}

	if token == "" || ticket == "" {
		return nil, errx.BadRequest("缺少令牌或凭证")
	}

	share, err := h.shareRepo.GetByToken(c.Request.Context(), token)
	if err != nil {
		return nil, err
	}
	if share == nil {
		return nil, errx.NotFound("分享链接已失效")
	}

	if err := h.validateTicket(share, ticket); err != nil {
		return nil, errx.Forbidden("%w", err)
	}

	validPath, err := h.fileManager.ValidatePath(share.FilePath)
	if err != nil {
		return nil, errx.NotFound("文件不存在或已移动")
	}
	info, err := os.Stat(validPath)
	if err != nil {
		return nil, errx.NotFound("文件不可用")
	}

	targetPath := validPath
	if info.IsDir() && subpath != "" {
		resolved, err := h.fileManager.ResolveShareSubpath(validPath, subpath)
		if err != nil {
			return nil, errx.BadRequest("非法路径")
		}
		targetPath = resolved
	}

	targetInfo, err := os.Stat(targetPath)
	if err != nil {
		return nil, errx.NotFound("文件不可用")
	}

	if targetInfo.IsDir() {
		// Zip and stream the directory
		c.Header("Content-Type", "application/zip")
		c.Header("Content-Disposition", httpx.FormatContentDisposition("attachment", filepath.Base(targetPath)+".zip"))
		c.Writer.WriteHeader(200)

		zw := archive_zip.NewWriter(c.Writer)
		defer zw.Close()

		// 响应头已写入，无法再返回错误响应，Walk 失败只能显式忽略
		_ = filepath.Walk(targetPath, func(path string, winfo os.FileInfo, err error) error {
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
		return nil, nil
	}

	// Serve single file — O_NOFOLLOW guards the TOCTOU window between
	// ResolveShareSubpath's EvalSymlinks and the actual read.
	f, err := os.OpenFile(targetPath, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	contentType, _ := h.fileManager.GetMimeType(targetPath)
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	extraHeaders := map[string]string{
		"Content-Disposition": httpx.FormatContentDisposition("inline", filepath.Base(targetPath)),
	}
	if contentType == "application/octet-stream" {
		extraHeaders["Content-Disposition"] = httpx.FormatContentDisposition("attachment", filepath.Base(targetPath))
	}

	c.DataFromReader(200, targetInfo.Size(), contentType, f, extraHeaders)
	return nil, nil
}

// RegisterRoutes registers file share management routes (protected)
func RegisterShareRoutes(protected *gin.RouterGroup, shareRepo filemanager.ShareRepository, fileManager *filemanager.Manager, cfg *config.Config) {
	handler := NewFileShareHandler(shareRepo, fileManager, cfg)

	protected.POST("/shares", httpx.H(handler.CreateShare))
	protected.GET("/shares", httpx.H(handler.ListShares))
	protected.GET("/shares/:id", httpx.H(handler.GetShare))
	protected.PUT("/shares/:id", httpx.H(handler.UpdateShare))
	protected.DELETE("/shares/:id", httpx.H(handler.DeleteShare))
	protected.POST("/shares/cleanup", httpx.H(handler.CleanupExpired))
}

// RegisterPublicShareRoute registers the public share routes (no auth).
func RegisterPublicShareRoute(public *gin.RouterGroup, shareRepo filemanager.ShareRepository, fileManager *filemanager.Manager, rateLimit int, rateInterval time.Duration, cfg *config.Config) {
	handler := NewFileShareHandler(shareRepo, fileManager, cfg)
	g := public.Group("/shares/public")
	if rateLimit > 0 {
		g.Use(middleware.RateLimitMiddleware("share", rateLimit, rateInterval))
	}
	g.GET("/:token/info", httpx.H(handler.ShareInfo))
	g.POST("/:token/ticket", httpx.H(handler.GetTicket))
	g.GET("/:token/download", httpx.H(handler.PublicDownload))
	g.GET("/:token/list", httpx.H(handler.PublicList))
}

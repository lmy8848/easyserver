package http

import (
	"strconv"

	"easyserver/internal/domain/security"
	"easyserver/internal/httpx"
	"easyserver/internal/httpx/middleware"
	"easyserver/internal/infra/errx"

	"github.com/gin-gonic/gin"
)

// Handler exposes security-audit endpoints (CVE scan, upgrade, kernel status).
type Handler struct {
	svc *security.Service
}

func NewHandler(svc *security.Service) *Handler {
	return &Handler{svc: svc}
}

// Scan runs a CVE scan over installed apt packages via osv.dev.
func (h *Handler) Scan(c *gin.Context) (any, error) {
	middleware.AuditSummary(c, "CVE 漏洞扫描")
	vulns, err := h.svc.Scan(c.Request.Context())
	if err != nil {
		return nil, err
	}
	return gin.H{"vulnerabilities": vulns, "count": len(vulns)}, nil
}

// Upgrade runs apt-get install --only-upgrade for the requested packages.
func (h *Handler) Upgrade(c *gin.Context) (any, error) {
	var req struct {
		Packages []string `json:"packages" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		return nil, errx.BadRequest("invalid request: %w", err)
	}
	middleware.AuditSummary(c, "CVE 漏洞升级 "+strconv.Itoa(len(req.Packages))+" 个包")
	out, err := h.svc.Upgrade(c.Request.Context(), req.Packages)
	if err != nil {
		return nil, err
	}
	return gin.H{"message": "升级完成", "output": out}, nil
}

// Kernel returns running vs latest installed kernel.
func (h *Handler) Kernel(c *gin.Context) (any, error) {
	st, err := h.svc.KernelStatus(c.Request.Context())
	if err != nil {
		return nil, err
	}
	return st, nil
}

// Upgradable returns the count of pending apt upgrades.
func (h *Handler) Upgradable(c *gin.Context) (any, error) {
	count, err := h.svc.PackageUpdateCount(c.Request.Context())
	if err != nil {
		return nil, err
	}
	return gin.H{"count": count}, nil
}

// ScanBaseline builds the file integrity baseline.
func (h *Handler) ScanBaseline(c *gin.Context) (any, error) {
	middleware.AuditSummary(c, "FIM 建立基线")
	if err := h.svc.ScanBaseline(c.Request.Context()); err != nil {
		return nil, err
	}
	return gin.H{"message": "基线已建立"}, nil
}

// CheckChanges checks for file changes against baseline.
func (h *Handler) CheckChanges(c *gin.Context) (any, error) {
	middleware.AuditSummary(c, "FIM 检测变更")
	changes, err := h.svc.CheckChanges(c.Request.Context())
	if err != nil {
		return nil, err
	}
	return gin.H{"changes": changes, "count": len(changes)}, nil
}

// ListBaseline returns the FIM baseline.
func (h *Handler) ListBaseline(c *gin.Context) (any, error) {
	bl, err := h.svc.ListBaseline(c.Request.Context())
	if err != nil {
		return nil, err
	}
	return gin.H{"baseline": bl}, nil
}

// ListChanges returns recent FIM changes.
func (h *Handler) ListChanges(c *gin.Context) (any, error) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "100"))
	changes, err := h.svc.ListChanges(c.Request.Context(), limit)
	if err != nil {
		return nil, err
	}
	return gin.H{"changes": changes}, nil
}

// ResetBaseline resets the FIM baseline.
func (h *Handler) ResetBaseline(c *gin.Context) (any, error) {
	middleware.AuditSummary(c, "FIM 重置基线")
	if err := h.svc.ResetBaseline(c.Request.Context()); err != nil {
		return nil, err
	}
	return gin.H{"message": "基线已重置"}, nil
}

// RegisterRoutes registers security-audit routes.
func RegisterRoutes(protected *gin.RouterGroup, svc *security.Service) {
	h := NewHandler(svc)
	protected.POST("/security/cve/scan", httpx.H(h.Scan))
	protected.POST("/security/cve/upgrade", httpx.H(h.Upgrade))
	protected.GET("/security/cve/kernel", httpx.H(h.Kernel))
	protected.GET("/security/cve/upgradable", httpx.H(h.Upgradable))

	protected.POST("/security/fim/scan", httpx.H(h.ScanBaseline))
	protected.POST("/security/fim/check", httpx.H(h.CheckChanges))
	protected.GET("/security/fim/baseline", httpx.H(h.ListBaseline))
	protected.GET("/security/fim/changes", httpx.H(h.ListChanges))
	protected.POST("/security/fim/reset", httpx.H(h.ResetBaseline))
}

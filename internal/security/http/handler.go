package http

import (
	"strconv"

	"easyserver/internal/httpx"
	"easyserver/internal/httpx/middleware"
	"easyserver/internal/infra/apperror"
	"easyserver/internal/security"
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
func (h *Handler) Scan(c *gin.Context) {
	middleware.AuditSummary(c, "CVE 漏洞扫描")
	vulns, err := h.svc.Scan(c.Request.Context())
	if err != nil {
		c.Error(apperror.WrapError(err))
		return
	}
	httpx.Success(c, gin.H{"vulnerabilities": vulns, "count": len(vulns)})
}

// Upgrade runs apt-get install --only-upgrade for the requested packages.
func (h *Handler) Upgrade(c *gin.Context) {
	var req struct {
		Packages []string `json:"packages" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperror.WrapError(err))
		return
	}
	middleware.AuditSummary(c, "CVE 漏洞升级 "+strconv.Itoa(len(req.Packages))+" 个包")
	out, err := h.svc.Upgrade(c.Request.Context(), req.Packages)
	if err != nil {
		c.Error(apperror.WrapError(err))
		return
	}
	httpx.Success(c, gin.H{"message": "升级完成", "output": out})
}

// Kernel returns running vs latest installed kernel.
func (h *Handler) Kernel(c *gin.Context) {
	st, err := h.svc.KernelStatus(c.Request.Context())
	if err != nil {
		c.Error(apperror.WrapError(err))
		return
	}
	httpx.Success(c, st)
}

// Upgradable returns the count of pending apt upgrades.
func (h *Handler) Upgradable(c *gin.Context) {
	count, err := h.svc.PackageUpdateCount(c.Request.Context())
	if err != nil {
		c.Error(apperror.WrapError(err))
		return
	}
	httpx.Success(c, gin.H{"count": count})
}

// RegisterRoutes registers security-audit routes.
func RegisterRoutes(protected *gin.RouterGroup, svc *security.Service) {
	h := NewHandler(svc)
	protected.POST("/security/cve/scan", h.Scan)
	protected.POST("/security/cve/upgrade", h.Upgrade)
	protected.GET("/security/cve/kernel", h.Kernel)
	protected.GET("/security/cve/upgradable", h.Upgradable)
}

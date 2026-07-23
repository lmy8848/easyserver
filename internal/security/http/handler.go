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

// GetLoginHistory returns recent login activities.
func (h *Handler) GetLoginHistory(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "200"))
	events, err := h.svc.GetLoginHistory(c.Request.Context(), limit)
	if err != nil {
		c.Error(apperror.WrapError(err))
		return
	}
	httpx.Success(c, gin.H{"events": events})
}

// GetAnomalies returns detected brute-force anomalies.
func (h *Handler) GetAnomalies(c *gin.Context) {
	window, _ := strconv.Atoi(c.DefaultQuery("window", "5"))
	threshold, _ := strconv.Atoi(c.DefaultQuery("threshold", "10"))
	anomalies, err := h.svc.GetAnomalies(c.Request.Context(), window, threshold)
	if err != nil {
		c.Error(apperror.WrapError(err))
		return
	}
	httpx.Success(c, gin.H{"anomalies": anomalies})
}

// ListBannedIPs returns login-anomaly ban rules.
func (h *Handler) ListBannedIPs(c *gin.Context) {
	banned, err := h.svc.ListBannedIPs(c.Request.Context())
	if err != nil {
		c.Error(apperror.WrapError(err))
		return
	}
	httpx.Success(c, gin.H{"banned": banned})
}

// BanIP manually bans an IP.
func (h *Handler) BanIP(c *gin.Context) {
	var req struct {
		IP     string `json:"ip" binding:"required"`
		Reason string `json:"reason"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperror.WrapError(err))
		return
	}
	reason := req.Reason
	if reason == "" {
		reason = "手动封禁"
	}
	middleware.AuditSummary(c, "登录异常封禁 IP: "+req.IP)
	if err := h.svc.BanIP(c.Request.Context(), req.IP, reason); err != nil {
		c.Error(apperror.WrapError(err))
		return
	}
	httpx.Success(c, gin.H{"message": "已封禁"})
}

// UnbanIP removes a login-anomaly ban.
func (h *Handler) UnbanIP(c *gin.Context) {
	var req struct {
		IP string `json:"ip" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperror.WrapError(err))
		return
	}
	middleware.AuditSummary(c, "解封 IP: "+req.IP)
	if err := h.svc.UnbanIP(c.Request.Context(), req.IP); err != nil {
		c.Error(apperror.WrapError(err))
		return
	}
	httpx.Success(c, gin.H{"message": "已解封"})
}

// ScanBaseline builds the file integrity baseline.
func (h *Handler) ScanBaseline(c *gin.Context) {
	middleware.AuditSummary(c, "FIM 建立基线")
	if err := h.svc.ScanBaseline(c.Request.Context()); err != nil {
		c.Error(apperror.WrapError(err))
		return
	}
	httpx.Success(c, gin.H{"message": "基线已建立"})
}

// CheckChanges checks for file changes against baseline.
func (h *Handler) CheckChanges(c *gin.Context) {
	middleware.AuditSummary(c, "FIM 检测变更")
	changes, err := h.svc.CheckChanges(c.Request.Context())
	if err != nil {
		c.Error(apperror.WrapError(err))
		return
	}
	httpx.Success(c, gin.H{"changes": changes, "count": len(changes)})
}

// ListBaseline returns the FIM baseline.
func (h *Handler) ListBaseline(c *gin.Context) {
	bl, err := h.svc.ListBaseline(c.Request.Context())
	if err != nil {
		c.Error(apperror.WrapError(err))
		return
	}
	httpx.Success(c, gin.H{"baseline": bl})
}

// ListChanges returns recent FIM changes.
func (h *Handler) ListChanges(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "100"))
	changes, err := h.svc.ListChanges(c.Request.Context(), limit)
	if err != nil {
		c.Error(apperror.WrapError(err))
		return
	}
	httpx.Success(c, gin.H{"changes": changes})
}

// ResetBaseline resets the FIM baseline.
func (h *Handler) ResetBaseline(c *gin.Context) {
	middleware.AuditSummary(c, "FIM 重置基线")
	if err := h.svc.ResetBaseline(c.Request.Context()); err != nil {
		c.Error(apperror.WrapError(err))
		return
	}
	httpx.Success(c, gin.H{"message": "基线已重置"})
}

// RegisterRoutes registers security-audit routes.
func RegisterRoutes(protected *gin.RouterGroup, svc *security.Service) {
	h := NewHandler(svc)
	protected.POST("/security/cve/scan", h.Scan)
	protected.POST("/security/cve/upgrade", h.Upgrade)
	protected.GET("/security/cve/kernel", h.Kernel)
	protected.GET("/security/cve/upgradable", h.Upgradable)

	protected.GET("/security/login/history", h.GetLoginHistory)
	protected.GET("/security/login/anomalies", h.GetAnomalies)
	protected.GET("/security/login/banned", h.ListBannedIPs)
	protected.POST("/security/login/ban", h.BanIP)
	protected.POST("/security/login/unban", h.UnbanIP)

	protected.POST("/security/fim/scan", h.ScanBaseline)
	protected.POST("/security/fim/check", h.CheckChanges)
	protected.GET("/security/fim/baseline", h.ListBaseline)
	protected.GET("/security/fim/changes", h.ListChanges)
	protected.POST("/security/fim/reset", h.ResetBaseline)
}

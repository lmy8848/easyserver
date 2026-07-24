package http

import (
	"strconv"

	"easyserver/internal/httpx"
	"easyserver/internal/infra/apperror"
	"easyserver/internal/web/security"

	"github.com/gin-gonic/gin"
)

type SecurityHandler struct {
	svc *security.SecurityService
}

func NewSecurityHandler(svc *security.SecurityService) *SecurityHandler {
	return &SecurityHandler{svc: svc}
}

// RegisterRoutes registers website security routes under /api/websites/:id/security.
func (h *SecurityHandler) RegisterRoutes(g *gin.RouterGroup) {
	security := g.Group("/:id/security")
	{
		security.GET("/config", h.GetConfig)
		security.PUT("/config", h.UpdateConfig)
		security.GET("/banned", h.ListBannedIPs)
		security.POST("/ban", h.BanIP)
		security.POST("/unban/:ban_id", h.UnbanIP)
	}
}

// GetConfig returns the security config for a website.
func (h *SecurityHandler) GetConfig(c *gin.Context) {
	websiteID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.Error(apperror.ErrBadRequest.WithMessage("无效的网站ID"))
		return
	}
	cfg, err := h.svc.GetConfig(c.Request.Context(), websiteID)
	if err != nil {
		c.Error(err)
		return
	}
	httpx.Success(c, cfg)
}

// UpdateConfig updates rate-limit and auto-ban settings.
func (h *SecurityHandler) UpdateConfig(c *gin.Context) {
	websiteID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.Error(apperror.ErrBadRequest.WithMessage("无效的网站ID"))
		return
	}
	var req struct {
		RateLimitEnabled    bool `json:"rate_limit_enabled"`
		RateLimitRate       int  `json:"rate_limit_rate"`
		RateLimitBurst      int  `json:"rate_limit_burst"`
		LimitConn           int  `json:"limit_conn"`
		AutoBanEnabled      bool `json:"auto_ban_enabled"`
		AutoBanThreshold    int  `json:"auto_ban_threshold"`
		AutoBan404Threshold int  `json:"auto_ban_404_threshold"`
		AutoBanDuration     int  `json:"auto_ban_duration"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperror.ErrBadRequest.Wrap(err))
		return
	}
	cfg, err := h.svc.UpdateConfig(c.Request.Context(), websiteID, func(cfg *security.SecurityConfig) {
		cfg.RateLimitEnabled = req.RateLimitEnabled
		cfg.RateLimitRate = req.RateLimitRate
		cfg.RateLimitBurst = req.RateLimitBurst
		cfg.LimitConn = req.LimitConn
		cfg.AutoBanEnabled = req.AutoBanEnabled
		cfg.AutoBanThreshold = req.AutoBanThreshold
		cfg.AutoBan404Threshold = req.AutoBan404Threshold
		cfg.AutoBanDuration = req.AutoBanDuration
	})
	if err != nil {
		c.Error(err)
		return
	}
	httpx.Success(c, cfg)
}

// ListBannedIPs returns active bans for a website.
func (h *SecurityHandler) ListBannedIPs(c *gin.Context) {
	websiteID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.Error(apperror.ErrBadRequest.WithMessage("无效的网站ID"))
		return
	}
	bans, err := h.svc.ListBannedIPs(c.Request.Context(), websiteID)
	if err != nil {
		c.Error(err)
		return
	}
	httpx.Success(c, bans)
}

// BanIP manually bans an IP for a website.
func (h *SecurityHandler) BanIP(c *gin.Context) {
	websiteID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.Error(apperror.ErrBadRequest.WithMessage("无效的网站ID"))
		return
	}
	var req struct {
		IP       string `json:"ip" binding:"required"`
		Reason   string `json:"reason"`
		Duration int    `json:"duration"` // seconds, 0 = permanent
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperror.ErrBadRequest.Wrap(err))
		return
	}
	if req.Reason == "" {
		req.Reason = "手动封禁"
	}
	var wid *int64
	wid = &websiteID
	if err := h.svc.BanIP(c.Request.Context(), wid, req.IP, req.Reason, "manual", req.Duration); err != nil {
		c.Error(err)
		return
	}
	httpx.Success(c, gin.H{"message": "已封禁 " + req.IP})
}

// UnbanIP removes a ban by ID.
func (h *SecurityHandler) UnbanIP(c *gin.Context) {
	banID, err := strconv.ParseInt(c.Param("ban_id"), 10, 64)
	if err != nil {
		c.Error(apperror.ErrBadRequest.WithMessage("无效的封禁ID"))
		return
	}
	if err := h.svc.UnbanIP(c.Request.Context(), banID); err != nil {
		c.Error(err)
		return
	}
	httpx.Success(c, gin.H{"message": "已解封"})
}

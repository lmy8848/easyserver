package http

import (
	"strconv"

	"easyserver/internal/domain/web/security"
	"easyserver/internal/httpx"
	"easyserver/internal/infra/errx"

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
		security.GET("/config", httpx.H(h.GetConfig))
		security.PUT("/config", httpx.H(h.UpdateConfig))
		security.GET("/banned", httpx.H(h.ListBannedIPs))
		security.POST("/ban", httpx.H(h.BanIP))
		security.POST("/unban/:ban_id", httpx.H(h.UnbanIP))
	}
}

// GetConfig returns the security config for a website.
func (h *SecurityHandler) GetConfig(c *gin.Context) (any, error) {
	websiteID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return nil, errx.BadRequest("无效的网站ID")
	}
	cfg, err := h.svc.GetConfig(c.Request.Context(), websiteID)
	if err != nil {
		return nil, err
	}
	return cfg, nil
}

// UpdateConfig updates rate-limit and auto-ban settings.
func (h *SecurityHandler) UpdateConfig(c *gin.Context) (any, error) {
	websiteID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return nil, errx.BadRequest("无效的网站ID")
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
		return nil, errx.BadRequest("invalid request: %w", err)
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
		return nil, err
	}
	return cfg, nil
}

// ListBannedIPs returns active bans for a website.
func (h *SecurityHandler) ListBannedIPs(c *gin.Context) (any, error) {
	websiteID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return nil, errx.BadRequest("无效的网站ID")
	}
	bans, err := h.svc.ListBannedIPs(c.Request.Context(), websiteID)
	if err != nil {
		return nil, err
	}
	return bans, nil
}

// BanIP manually bans an IP for a website.
func (h *SecurityHandler) BanIP(c *gin.Context) (any, error) {
	websiteID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return nil, errx.BadRequest("无效的网站ID")
	}
	var req struct {
		IP       string `json:"ip" binding:"required"`
		Reason   string `json:"reason"`
		Duration int    `json:"duration"` // seconds, 0 = permanent
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		return nil, errx.BadRequest("invalid request: %w", err)
	}
	if req.Reason == "" {
		req.Reason = "手动封禁"
	}
	wid := &websiteID
	if err := h.svc.BanIP(c.Request.Context(), wid, req.IP, req.Reason, "manual", req.Duration); err != nil {
		return nil, err
	}
	return gin.H{"message": "已封禁 " + req.IP}, nil
}

// UnbanIP removes a ban by ID.
func (h *SecurityHandler) UnbanIP(c *gin.Context) (any, error) {
	banID, err := strconv.ParseInt(c.Param("ban_id"), 10, 64)
	if err != nil {
		return nil, errx.BadRequest("无效的封禁ID")
	}
	if err := h.svc.UnbanIP(c.Request.Context(), banID); err != nil {
		return nil, err
	}
	return gin.H{"message": "已解封"}, nil
}

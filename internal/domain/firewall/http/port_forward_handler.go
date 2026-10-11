package http

import (
	"strconv"

	"easyserver/internal/domain/firewall"
	"easyserver/internal/httpx"
	"easyserver/internal/httpx/middleware"
	"easyserver/internal/infra/errx"

	"github.com/gin-gonic/gin"
)

// PortForwardHandler handles port-forwarding API requests.
type PortForwardHandler struct {
	svc *firewall.PortForwardService
}

// NewPortForwardHandler creates a PortForwardHandler.
func NewPortForwardHandler(svc *firewall.PortForwardService) *PortForwardHandler {
	return &PortForwardHandler{svc: svc}
}

// ListPortForwards returns all port-forward rules with nft counters.
func (h *PortForwardHandler) ListPortForwards(c *gin.Context) (any, error) {
	rules, err := h.svc.ListPortForwards(c.Request.Context())
	if err != nil {
		return nil, err
	}
	return gin.H{"items": rules}, nil
}

// CreatePortForward validates and applies a new forwarding rule.
func (h *PortForwardHandler) CreatePortForward(c *gin.Context) (any, error) {
	rule, err := httpx.BindJSON[firewall.PortForwardRule](c)
	if err != nil {
		return nil, err
	}
	middleware.AuditSummary(c, "创建端口转发 "+rule.Protocol+"/"+rule.ListenPort+" -> "+rule.DestIP+":"+rule.DestPortOrListen())
	if err := h.svc.CreatePortForward(c.Request.Context(), &rule); err != nil {
		return nil, err
	}
	return gin.H{"message": "端口转发已创建并生效"}, nil
}

// UpdatePortForward updates a forwarding rule.
func (h *PortForwardHandler) UpdatePortForward(c *gin.Context) (any, error) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return nil, errx.BadRequest("无效的ID")
	}
	rule, err := httpx.BindJSON[firewall.PortForwardRule](c)
	if err != nil {
		return nil, err
	}
	rule.ID = id
	middleware.AuditSummary(c, "更新端口转发 #"+strconv.FormatInt(id, 10))
	if err := h.svc.UpdatePortForward(c.Request.Context(), &rule); err != nil {
		return nil, err
	}
	return gin.H{"message": "端口转发已更新"}, nil
}

// DeletePortForward removes a forwarding rule.
func (h *PortForwardHandler) DeletePortForward(c *gin.Context) (any, error) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return nil, errx.BadRequest("无效的ID")
	}
	middleware.AuditSummary(c, "删除端口转发 #"+strconv.FormatInt(id, 10))
	if err := h.svc.DeletePortForward(c.Request.Context(), id); err != nil {
		return nil, err
	}
	return gin.H{"message": "端口转发已删除"}, nil
}

// EnablePortForward applies a rule's kernel rules.
func (h *PortForwardHandler) EnablePortForward(c *gin.Context) (any, error) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return nil, errx.BadRequest("无效的ID")
	}
	middleware.AuditSummary(c, "启用端口转发 #"+strconv.FormatInt(id, 10))
	if err := h.svc.SetPortForwardEnabled(c.Request.Context(), id, true); err != nil {
		return nil, err
	}
	return gin.H{"message": "端口转发已启用"}, nil
}

// DisablePortForward removes a rule's kernel rules.
func (h *PortForwardHandler) DisablePortForward(c *gin.Context) (any, error) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return nil, errx.BadRequest("无效的ID")
	}
	middleware.AuditSummary(c, "禁用端口转发 #"+strconv.FormatInt(id, 10))
	if err := h.svc.SetPortForwardEnabled(c.Request.Context(), id, false); err != nil {
		return nil, err
	}
	return gin.H{"message": "端口转发已禁用"}, nil
}

// RegisterPortForwardRoutes registers port-forwarding routes.
func RegisterPortForwardRoutes(protected *gin.RouterGroup, svc *firewall.PortForwardService) {
	handler := NewPortForwardHandler(svc)
	protected.GET("/firewall/port-forwards", httpx.H(handler.ListPortForwards))
	protected.POST("/firewall/port-forwards", httpx.H(handler.CreatePortForward))
	protected.PUT("/firewall/port-forwards/:id", httpx.H(handler.UpdatePortForward))
	protected.DELETE("/firewall/port-forwards/:id", httpx.H(handler.DeletePortForward))
	protected.POST("/firewall/port-forwards/:id/enable", httpx.H(handler.EnablePortForward))
	protected.POST("/firewall/port-forwards/:id/disable", httpx.H(handler.DisablePortForward))
}

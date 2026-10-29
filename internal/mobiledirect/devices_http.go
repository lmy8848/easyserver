package mobiledirect

import (
	"strconv"

	"easyserver/internal/domain/auth"
	"easyserver/internal/httpx"

	"github.com/gin-gonic/gin"
)

// RegisterDeviceRoutes 把设备公钥管理接口挂到受保护（JWT）路由组上：
// 注册/查看/吊销都经面板登录态（通常走 CF），直连通道只做签名校验。
func RegisterDeviceRoutes(g *gin.RouterGroup, store *Store, refreshSvc *auth.RefreshService) {
	h := &deviceHandler{store: store, refresh: refreshSvc}
	g.POST("/auth/devices", httpx.H(h.create))
	g.GET("/auth/devices", httpx.H(h.list))
	g.DELETE("/auth/devices/:id", httpx.H(h.revoke))
}

type deviceHandler struct {
	store   *Store
	refresh *auth.RefreshService
}

type registerDeviceRequest struct {
	DeviceID  string `json:"device_id" binding:"required"`
	PublicKey string `json:"public_key" binding:"required"`
	Name      string `json:"name"`
}

func (h *deviceHandler) create(c *gin.Context) (any, error) {
	req, err := httpx.BindJSON[registerDeviceRequest](c)
	if err != nil {
		return nil, err
	}
	return h.store.Register(c.Request.Context(), c.GetInt64("user_id"), req.DeviceID, req.Name, req.PublicKey)
}

func (h *deviceHandler) list(c *gin.Context) (any, error) {
	return h.store.List(c.Request.Context(), c.GetInt64("user_id"))
}

func (h *deviceHandler) revoke(c *gin.Context) (any, error) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return nil, err
	}
	userID := c.GetInt64("user_id")
	dev, gerr := h.store.GetByID(c.Request.Context(), userID, id)
	if gerr != nil {
		return nil, gerr
	}
	if err := h.store.Revoke(c.Request.Context(), userID, id); err != nil {
		return nil, err
	}
	// 设备吊销必须连带吊销其 refresh 链，否则 90 天凭据仍可换新 access
	if h.refresh != nil {
		_ = h.refresh.RevokeFamily(c.Request.Context(), userID, dev.DeviceID)
	}
	return gin.H{"ok": true}, nil
}

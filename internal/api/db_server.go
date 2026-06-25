package api

import (
	"strconv"

	"easyserver/internal/dbserver"

	"github.com/gin-gonic/gin"
)

// DBServerHandler handles top-level DB server endpoints (list, get).
// Sub-domain endpoints are delegated to focused sub-handlers.
type DBServerHandler struct {
	dbServerService *dbserver.Service
}

func NewDBServerHandler(dbServerService *dbserver.Service) *DBServerHandler {
	return &DBServerHandler{dbServerService: dbServerService}
}

func (h *DBServerHandler) List(c *gin.Context) {
	ctx := c.Request.Context()
	h.dbServerService.RefreshAllStatus(ctx)
	servers, err := h.dbServerService.List(ctx)
	if err != nil {
		InternalError(c, err.Error())
		return
	}
	Success(c, servers)
}

func (h *DBServerHandler) Get(c *gin.Context) {
	ctx := c.Request.Context()
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		BadRequest(c, "无效的 ID")
		return
	}
	h.dbServerService.RefreshStatus(ctx, id)
	server, err := h.dbServerService.Get(ctx, id)
	if err != nil {
		InternalError(c, err.Error())
		return
	}
	if server == nil {
		NotFound(c, "数据库服务器不存在")
		return
	}
	Success(c, server)
}

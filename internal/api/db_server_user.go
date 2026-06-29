package api

import (
	"strconv"

	"easyserver/internal/api/middleware"
	"easyserver/internal/database_mgmt"
	"github.com/gin-gonic/gin"
)

// UserHandler handles DB user management endpoints.
type UserHandler struct {
	dbMgmtService *database_mgmt.Service
}

func NewUserHandler(dbMgmtService *database_mgmt.Service) *UserHandler {
	return &UserHandler{dbMgmtService: dbMgmtService}
}

func (h *UserHandler) ListDBUsers(c *gin.Context) {
	sid, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.Error(ErrBadRequest.WithMessage("无效的服务器ID"))
		return
	}
	users, err := h.dbMgmtService.ListDBUsers(c.Request.Context(), sid)
	if err != nil {
		c.Error(WrapError(err))
		return
	}
	Success(c, users)
}

func (h *UserHandler) CreateDBUser(c *gin.Context) {
	sid, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.Error(ErrBadRequest.WithMessage("无效的服务器ID"))
		return
	}
	var req database_mgmt.CreateDBUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(ErrBadRequest.Wrap(err))
		return
	}
	middleware.AuditSummary(c, "创建数据库用户 "+req.Username)
	user, err := h.dbMgmtService.CreateDBUser(c.Request.Context(), sid, &req)
	if err != nil {
		c.Error(WrapError(err))
		return
	}
	Success(c, user)
}

func (h *UserHandler) DeleteDBUser(c *gin.Context) {
	sid, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.Error(ErrBadRequest.WithMessage("无效的服务器ID"))
		return
	}
	uid, err := strconv.ParseInt(c.Param("uid"), 10, 64)
	if err != nil {
		c.Error(ErrBadRequest.WithMessage("无效的用户ID"))
		return
	}
	middleware.AuditSummary(c, "删除数据库用户 "+strconv.FormatInt(uid, 10))
	if err := h.dbMgmtService.DeleteDBUser(c.Request.Context(), sid, uid); err != nil {
		c.Error(WrapError(err))
		return
	}
	Success(c, nil)
}

func (h *UserHandler) GrantPrivileges(c *gin.Context) {
	sid, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.Error(ErrBadRequest.WithMessage("无效的服务器ID"))
		return
	}
	uid, err := strconv.ParseInt(c.Param("uid"), 10, 64)
	if err != nil {
		c.Error(ErrBadRequest.WithMessage("无效的用户ID"))
		return
	}
	var req database_mgmt.GrantRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(ErrBadRequest.Wrap(err))
		return
	}
	middleware.AuditSummary(c, "授权数据库用户 "+strconv.FormatInt(uid, 10))
	if err := h.dbMgmtService.GrantPrivileges(c.Request.Context(), sid, uid, &req); err != nil {
		c.Error(WrapError(err))
		return
	}
	Success(c, gin.H{"message": "权限已授予"})
}

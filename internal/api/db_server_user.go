package api

import (
	"strconv"

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
		BadRequest(c, "无效的服务器ID")
		return
	}
	users, err := h.dbMgmtService.ListDBUsers(c.Request.Context(), sid)
	if err != nil {
		InternalError(c, err.Error())
		return
	}
	Success(c, users)
}

func (h *UserHandler) CreateDBUser(c *gin.Context) {
	sid, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		BadRequest(c, "无效的服务器ID")
		return
	}

	var req database_mgmt.CreateDBUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequest(c, "无效的请求: "+err.Error())
		return
	}

	user, err := h.dbMgmtService.CreateDBUser(c.Request.Context(), sid, &req)
	if err != nil {
		InternalError(c, err.Error())
		return
	}
	Success(c, user)
}

func (h *UserHandler) DeleteDBUser(c *gin.Context) {
	sid, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		BadRequest(c, "无效的服务器ID")
		return
	}
	uid, err := strconv.ParseInt(c.Param("uid"), 10, 64)
	if err != nil {
		BadRequest(c, "无效的用户ID")
		return
	}

	if err := h.dbMgmtService.DeleteDBUser(c.Request.Context(), sid, uid); err != nil {
		InternalError(c, err.Error())
		return
	}
	Success(c, gin.H{"message": "用户已删除"})
}

func (h *UserHandler) GrantPrivileges(c *gin.Context) {
	sid, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		BadRequest(c, "无效的服务器ID")
		return
	}
	uid, err := strconv.ParseInt(c.Param("uid"), 10, 64)
	if err != nil {
		BadRequest(c, "无效的用户ID")
		return
	}

	var req database_mgmt.GrantRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequest(c, "无效的请求: "+err.Error())
		return
	}

	if err := h.dbMgmtService.GrantPrivileges(c.Request.Context(), sid, uid, &req); err != nil {
		InternalError(c, err.Error())
		return
	}
	Success(c, gin.H{"message": "权限已授予"})
}

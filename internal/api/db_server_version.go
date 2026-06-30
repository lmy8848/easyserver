package api

import (
	"strconv"

	"easyserver/internal/api/middleware"
	"easyserver/internal/dbserver"

	"github.com/gin-gonic/gin"
)

// VersionHandler handles DB version management endpoints.
type VersionHandler struct {
	dbServerService *dbserver.Service
}

func NewVersionHandler(dbServerService *dbserver.Service) *VersionHandler {
	return &VersionHandler{dbServerService: dbServerService}
}

func (h *VersionHandler) GetVersionTemplates(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.Error(ErrBadRequest.WithMessage("无效的 ID"))
		return
	}
	server, err := h.dbServerService.Get(c.Request.Context(), id)
	if err != nil || server == nil {
		c.Error(ErrNotFound.WithMessage("数据库服务器不存在"))
		return
	}
	templates := dbserver.GetVersionTemplates(server.Name)
	Success(c, templates)
}

func (h *VersionHandler) ListVersions(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.Error(ErrBadRequest.WithMessage("无效的 ID"))
		return
	}
	versions, err := h.dbServerService.ListVersions(c.Request.Context(), id)
	if err != nil {
		c.Error(WrapError(err))
		return
	}
	Success(c, versions)
}

func (h *VersionHandler) InstallVersion(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.Error(ErrBadRequest.WithMessage("无效的 ID"))
		return
	}
	var req dbserver.CreateDBVersionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(ErrBadRequest.Wrap(err))
		return
	}
	middleware.AuditSummary(c, "安装数据库版本 "+req.Version)
	version, err := h.dbServerService.InstallVersion(c.Request.Context(), id, &req)
	if err != nil {
		c.Error(WrapError(err))
		return
	}
	Success(c, version)
}

func (h *VersionHandler) UninstallVersion(c *gin.Context) {
	vid, err := strconv.ParseInt(c.Param("vid"), 10, 64)
	if err != nil {
		c.Error(ErrBadRequest.WithMessage("无效的版本ID"))
		return
	}
	middleware.AuditSummary(c, "卸载数据库版本 #"+strconv.FormatInt(vid, 10))
	if err := h.dbServerService.UninstallVersion(c.Request.Context(), vid); err != nil {
		c.Error(WrapError(err))
		return
	}
	Success(c, gin.H{"message": "已卸载"})
}

func (h *VersionHandler) StartVersion(c *gin.Context) {
	vid, err := strconv.ParseInt(c.Param("vid"), 10, 64)
	if err != nil {
		c.Error(ErrBadRequest.WithMessage("无效的版本ID"))
		return
	}
	middleware.AuditSummary(c, "启动数据库版本 #"+strconv.FormatInt(vid, 10))
	if err := h.dbServerService.StartVersion(c.Request.Context(), vid); err != nil {
		c.Error(WrapError(err))
		return
	}
	Success(c, gin.H{"status": "running"})
}

func (h *VersionHandler) StopVersion(c *gin.Context) {
	vid, err := strconv.ParseInt(c.Param("vid"), 10, 64)
	if err != nil {
		c.Error(ErrBadRequest.WithMessage("无效的版本ID"))
		return
	}
	middleware.AuditSummary(c, "停止数据库版本 #"+strconv.FormatInt(vid, 10))
	if err := h.dbServerService.StopVersion(c.Request.Context(), vid); err != nil {
		c.Error(WrapError(err))
		return
	}
	Success(c, gin.H{"status": "stopped"})
}

func (h *VersionHandler) RestartVersion(c *gin.Context) {
	vid, err := strconv.ParseInt(c.Param("vid"), 10, 64)
	if err != nil {
		c.Error(ErrBadRequest.WithMessage("无效的版本ID"))
		return
	}
	middleware.AuditSummary(c, "重启数据库版本 #"+strconv.FormatInt(vid, 10))
	if err := h.dbServerService.RestartVersion(c.Request.Context(), vid); err != nil {
		c.Error(WrapError(err))
		return
	}
	Success(c, gin.H{"status": "running"})
}

func (h *VersionHandler) UpdateVersionPort(c *gin.Context) {
	vid, err := strconv.ParseInt(c.Param("vid"), 10, 64)
	if err != nil {
		c.Error(ErrBadRequest.WithMessage("无效的版本ID"))
		return
	}

	var req struct {
		Port int `json:"port" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(ErrBadRequest.Wrap(err))
		return
	}
	vInfo, err := h.dbServerService.GetVersion(c.Request.Context(), vid)
	if err != nil {
		c.Error(ErrNotFound.WithMessage("数据库版本不存在"))
		return
	}
	middleware.AuditSummary(c, "更新数据库端口 ("+vInfo.ServiceName+") "+strconv.Itoa(vInfo.Port)+" -> "+strconv.Itoa(req.Port))

	if req.Port < 1 || req.Port > 65535 {
		c.Error(ErrBadRequest.WithMessage("端口必须在 1 到 65535 之间"))
		return
	}

	if err := h.dbServerService.UpdateVersionPort(c.Request.Context(), vid, req.Port); err != nil {
		c.Error(WrapError(err))
		return
	}

	Success(c, gin.H{"message": "端口已更新", "port": req.Port})
}

func (h *VersionHandler) GetVersionLogs(c *gin.Context) {
	vid, err := strconv.ParseInt(c.Param("vid"), 10, 64)
	if err != nil {
		c.Error(ErrBadRequest.WithMessage("无效的版本ID"))
		return
	}
	lines, _ := strconv.Atoi(c.DefaultQuery("lines", "200"))
	logs, err := h.dbServerService.GetVersionServiceLogs(c.Request.Context(), vid, lines)
	if err != nil {
		c.Error(WrapError(err))
		return
	}
	Success(c, gin.H{"logs": logs})
}

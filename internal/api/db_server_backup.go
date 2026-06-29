package api

import (
	"os"
	"strconv"

	"easyserver/internal/api/middleware"
	"easyserver/internal/database_mgmt"
	"easyserver/internal/dbserver"
	"github.com/gin-gonic/gin"
)

// BackupHandler handles database backup endpoints.
type BackupHandler struct {
	dbServerService *dbserver.Service
	dbMgmtService   *database_mgmt.Service
	dbBackupService *database_mgmt.Service
}

func NewBackupHandler(dbServerService *dbserver.Service, dbMgmtService *database_mgmt.Service) *BackupHandler {
	return &BackupHandler{
		dbServerService: dbServerService,
		dbMgmtService:   dbMgmtService,
	}
}

func (h *BackupHandler) CreateBackup(c *gin.Context) {
	did, err := strconv.ParseInt(c.Param("did"), 10, 64)
	if err != nil {
		c.Error(ErrBadRequest.WithMessage("无效的数据库ID"))
		return
	}
	middleware.AuditSummary(c, "创建数据库备份 #"+strconv.FormatInt(did, 10))

	// Get database info
	db, err := h.dbMgmtService.GetDatabaseByID(c.Request.Context(), did)
	if err != nil {
		c.Error(ErrNotFound.WithMessage("数据库不存在"))
		return
	}

	// Get db server info to determine type
	server, err := h.dbServerService.Get(c.Request.Context(), db.DBServerID)
	if err != nil {
		c.Error(ErrNotFound.WithMessage("数据库服务器不存在"))
		return
	}

	backup, err := h.dbMgmtService.CreateBackup(c.Request.Context(), db.DBServerID, db.DBVersionID, did, db.Name, server.Name)
	if err != nil {
		c.Error(WrapError(err))
		return
	}

	Success(c, backup)
}

func (h *BackupHandler) ListBackups(c *gin.Context) {
	did, err := strconv.ParseInt(c.Param("did"), 10, 64)
	if err != nil {
		c.Error(ErrBadRequest.WithMessage("无效的数据库ID"))
		return
	}

	backups, err := h.dbMgmtService.ListBackups(c.Request.Context(), did)
	if err != nil {
		c.Error(WrapError(err))
		return
	}

	if backups == nil {
		backups = []database_mgmt.DBBackup{}
	}

	Success(c, backups)
}

func (h *BackupHandler) DownloadBackup(c *gin.Context) {
	bid, err := strconv.ParseInt(c.Param("bid"), 10, 64)
	if err != nil {
		c.Error(ErrBadRequest.WithMessage("无效的备份ID"))
		return
	}

	backup, err := h.dbMgmtService.GetBackup(c.Request.Context(), bid)
	if err != nil {
		c.Error(ErrNotFound.WithMessage("备份不存在"))
		return
	}

	if backup.Status != "completed" {
		c.Error(ErrBadRequest.WithMessage("备份未完成"))
		return
	}

	// Check if file exists
	if _, err := os.Stat(backup.FilePath); os.IsNotExist(err) {
		c.Error(ErrNotFound.WithMessage("备份文件不存在"))
		return
	}

	c.File(backup.FilePath)
}

func (h *BackupHandler) RestoreBackup(c *gin.Context) {
	bid, err := strconv.ParseInt(c.Param("bid"), 10, 64)
	if err != nil {
		c.Error(ErrBadRequest.WithMessage("无效的备份ID"))
		return
	}

	// Require confirmation
	var req struct {
		Confirm bool `json:"confirm"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || !req.Confirm {
		c.Error(ErrBadRequest.WithMessage("请确认恢复，设置 {\"confirm\": true}"))
		return
	}
	middleware.AuditSummary(c, "恢复数据库备份 #"+strconv.FormatInt(bid, 10))

	backup, err := h.dbMgmtService.GetBackup(c.Request.Context(), bid)
	if err != nil {
		c.Error(ErrNotFound.WithMessage("备份不存在"))
		return
	}

	// Get db server info to determine type
	server, err := h.dbServerService.Get(c.Request.Context(), backup.DBServerID)
	if err != nil {
		c.Error(ErrNotFound.WithMessage("数据库服务器不存在"))
		return
	}

	if err := h.dbMgmtService.RestoreBackup(c.Request.Context(), bid, server.Name); err != nil {
		c.Error(WrapError(err))
		return
	}

	Success(c, gin.H{"message": "数据库恢复成功"})
}

func (h *BackupHandler) DeleteBackup(c *gin.Context) {
	bid, err := strconv.ParseInt(c.Param("bid"), 10, 64)
	if err != nil {
		c.Error(ErrBadRequest.WithMessage("无效的备份ID"))
		return
	}
	middleware.AuditSummary(c, "删除数据库备份 #"+strconv.FormatInt(bid, 10))

	if err := h.dbMgmtService.DeleteBackup(c.Request.Context(), bid); err != nil {
		c.Error(WrapError(err))
		return
	}

	Success(c, gin.H{"message": "备份已删除"})
}

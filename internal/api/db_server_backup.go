package api

import (
	"os"
	"strconv"

	"easyserver/internal/model"
	"easyserver/internal/service"

	"github.com/gin-gonic/gin"
)

// BackupHandler handles database backup endpoints.
type BackupHandler struct {
	dbServerService *service.DBServerService
	dbMgmtService   *service.DatabaseMgmtService
	dbBackupService *service.DBBackupService
}

func NewBackupHandler(dbServerService *service.DBServerService, dbMgmtService *service.DatabaseMgmtService, dbBackupService *service.DBBackupService) *BackupHandler {
	return &BackupHandler{
		dbServerService: dbServerService,
		dbMgmtService:   dbMgmtService,
		dbBackupService: dbBackupService,
	}
}

func (h *BackupHandler) CreateBackup(c *gin.Context) {
	did, err := strconv.ParseInt(c.Param("did"), 10, 64)
	if err != nil {
		BadRequest(c, "无效的数据库ID")
		return
	}

	// Get database info
	db, err := h.dbMgmtService.GetDatabaseByID(c.Request.Context(), did)
	if err != nil {
		NotFound(c, "数据库不存在")
		return
	}

	// Get db server info to determine type
	server, err := h.dbServerService.Get(c.Request.Context(), db.DBServerID)
	if err != nil {
		NotFound(c, "数据库服务器不存在")
		return
	}

	backup, err := h.dbBackupService.CreateBackup(c.Request.Context(), db.DBServerID, db.DBVersionID, did, db.Name, server.Name)
	if err != nil {
		InternalError(c, err.Error())
		return
	}

	Success(c, backup)
}

func (h *BackupHandler) ListBackups(c *gin.Context) {
	did, err := strconv.ParseInt(c.Param("did"), 10, 64)
	if err != nil {
		BadRequest(c, "无效的数据库ID")
		return
	}

	backups, err := h.dbBackupService.ListBackups(c.Request.Context(), did)
	if err != nil {
		InternalError(c, err.Error())
		return
	}

	if backups == nil {
		backups = []model.DBBackup{}
	}

	Success(c, backups)
}

func (h *BackupHandler) DownloadBackup(c *gin.Context) {
	bid, err := strconv.ParseInt(c.Param("bid"), 10, 64)
	if err != nil {
		BadRequest(c, "无效的备份ID")
		return
	}

	backup, err := h.dbBackupService.GetBackup(c.Request.Context(), bid)
	if err != nil {
		NotFound(c, "备份不存在")
		return
	}

	if backup.Status != "completed" {
		BadRequest(c, "备份未完成")
		return
	}

	// Check if file exists
	if _, err := os.Stat(backup.FilePath); os.IsNotExist(err) {
		NotFound(c, "备份文件不存在")
		return
	}

	c.File(backup.FilePath)
}

func (h *BackupHandler) RestoreBackup(c *gin.Context) {
	bid, err := strconv.ParseInt(c.Param("bid"), 10, 64)
	if err != nil {
		BadRequest(c, "无效的备份ID")
		return
	}

	// Require confirmation
	var req struct {
		Confirm bool `json:"confirm"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || !req.Confirm {
		BadRequest(c, "请确认恢复，设置 {\"confirm\": true}")
		return
	}

	backup, err := h.dbBackupService.GetBackup(c.Request.Context(), bid)
	if err != nil {
		NotFound(c, "备份不存在")
		return
	}

	// Get db server info to determine type
	server, err := h.dbServerService.Get(c.Request.Context(), backup.DBServerID)
	if err != nil {
		NotFound(c, "数据库服务器不存在")
		return
	}

	if err := h.dbBackupService.RestoreBackup(c.Request.Context(), bid, server.Name); err != nil {
		InternalError(c, err.Error())
		return
	}

	Success(c, gin.H{"message": "数据库恢复成功"})
}

func (h *BackupHandler) DeleteBackup(c *gin.Context) {
	bid, err := strconv.ParseInt(c.Param("bid"), 10, 64)
	if err != nil {
		BadRequest(c, "无效的备份ID")
		return
	}

	if err := h.dbBackupService.DeleteBackup(c.Request.Context(), bid); err != nil {
		InternalError(c, err.Error())
		return
	}

	Success(c, gin.H{"message": "备份已删除"})
}

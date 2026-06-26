package api

import (
	"os"
	"strconv"

	"easyserver/internal/database_mgmt"
	"easyserver/internal/dbserver"

	"github.com/gin-gonic/gin"
)

// BackupHandler handles database backup endpoints.
type BackupHandler struct {
	dbServerService *dbserver.Service
	dbMgmtService   *database_mgmt.Service
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
		BadRequest(c, "无效的数据库ID")
		return
	}

	// Get database info
	db, err := h.dbMgmtService.GetDatabaseByID(c.Request.Context(), did)
	if err != nil {
		NotFound(c, "数据库不存在")
		return
	}
	if db == nil {
		NotFound(c, "数据库不存在")
		return
	}

	// Get server info to determine db type
	server, err := h.dbMgmtService.GetServerByID(c.Request.Context(), db.DBServerID)
	if err != nil || server == nil {
		InternalError(c, "获取服务器信息失败")
		return
	}

	backup, err := h.dbMgmtService.CreateBackup(c.Request.Context(), db.DBServerID, db.DBVersionID, did, db.Name, server.Name)
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

	backups, err := h.dbMgmtService.ListBackups(c.Request.Context(), did)
	if err != nil {
		InternalError(c, err.Error())
		return
	}
	Success(c, backups)
}

func (h *BackupHandler) DownloadBackup(c *gin.Context) {
	bid, err := strconv.ParseInt(c.Param("bid"), 10, 64)
	if err != nil {
		BadRequest(c, "无效的备份ID")
		return
	}

	backup, err := h.dbMgmtService.GetBackup(c.Request.Context(), bid)
	if err != nil || backup == nil {
		NotFound(c, "备份不存在")
		return
	}

	if backup.Status != "completed" {
		BadRequest(c, "备份尚未完成")
		return
	}

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

	backup, err := h.dbMgmtService.GetBackup(c.Request.Context(), bid)
	if err != nil || backup == nil {
		NotFound(c, "备份不存在")
		return
	}

	// Get server info to determine db type
	server, err := h.dbMgmtService.GetServerByID(c.Request.Context(), backup.DBServerID)
	if err != nil || server == nil {
		InternalError(c, "获取服务器信息失败")
		return
	}

	if err := h.dbMgmtService.RestoreBackup(c.Request.Context(), bid, server.Name); err != nil {
		InternalError(c, err.Error())
		return
	}
	Success(c, gin.H{"message": "备份已恢复"})
}

func (h *BackupHandler) DeleteBackup(c *gin.Context) {
	bid, err := strconv.ParseInt(c.Param("bid"), 10, 64)
	if err != nil {
		BadRequest(c, "无效的备份ID")
		return
	}

	if err := h.dbMgmtService.DeleteBackup(c.Request.Context(), bid); err != nil {
		InternalError(c, err.Error())
		return
	}
	Success(c, gin.H{"message": "备份已删除"})
}

package http

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"easyserver/internal/database"
	"easyserver/internal/httpx"
	"easyserver/internal/httpx/middleware"
	"easyserver/internal/infra/apperror"

	"github.com/gin-gonic/gin"
)

// RegisterRoutes registers database management routes.
func RegisterRoutes(protected *gin.RouterGroup, svc *database.Service) {
	handler := NewDBServerHandler(svc)
	versionHandler := NewVersionHandler(svc)
	dbHandler := NewDatabaseHandler(svc)
	userHandler := NewUserHandler(svc)
	backupHandler := NewBackupHandler(svc)
	configHandler := NewConfigHandler()

	protected.GET("/db-instances", handler.List)
	protected.GET("/db-instances/:id", handler.Get)

	// Version management
	protected.GET("/db-instances/:id/versions", versionHandler.ListVersions)
	protected.POST("/db-instances/:id/versions", versionHandler.InstallVersion)
	protected.DELETE("/db-instances/versions/:vid", versionHandler.UninstallVersion)
	protected.DELETE("/db-instances/versions/:vid/data", versionHandler.DestroyVersion)
	protected.POST("/db-instances/versions/:vid/reset-password", versionHandler.ResetAdminPassword)
	protected.POST("/db-instances/versions/:vid/start", versionHandler.StartVersion)
	protected.POST("/db-instances/versions/:vid/stop", versionHandler.StopVersion)
	protected.POST("/db-instances/versions/:vid/restart", versionHandler.RestartVersion)
	protected.PUT("/db-instances/versions/:vid/port", versionHandler.UpdateVersionPort)
	protected.GET("/db-instances/versions/:vid/logs", versionHandler.GetVersionLogs)
	protected.GET("/db-instances/versions/:vid/config", versionHandler.GetVersionConfig)
	protected.PUT("/db-instances/versions/:vid/config", versionHandler.SaveVersionConfig)

	// Databases nested
	protected.GET("/db-instances/:id/databases", dbHandler.ListDatabases)
	protected.POST("/db-instances/:id/databases", dbHandler.CreateDatabase)
	protected.DELETE("/db-instances/:id/databases/:did", dbHandler.DeleteDatabase)

	// DB Users nested
	protected.GET("/db-instances/:id/users", userHandler.ListDBUsers)
	protected.POST("/db-instances/:id/users", userHandler.CreateDBUser)
	protected.DELETE("/db-instances/:id/users/:uid", userHandler.DeleteDBUser)
	protected.POST("/db-instances/:id/users/:uid/grant", userHandler.GrantPrivileges)

	// Database introspection
	protected.GET("/db-instances/databases/:did/tables", dbHandler.ListTables)
	protected.GET("/db-instances/databases/:did/describe", dbHandler.DescribeTable)
	protected.GET("/db-instances/databases/:did/query", dbHandler.QueryTable)
	protected.POST("/db-instances/databases/:did/execute", dbHandler.ExecuteSQL)
	protected.POST("/db-instances/databases/:did/insert", dbHandler.InsertRecord)
	protected.POST("/db-instances/databases/:did/update", dbHandler.UpdateRecord)
	protected.POST("/db-instances/databases/:did/delete", dbHandler.DeleteRecord)

	// Table management
	protected.POST("/db-instances/databases/:did/tables", dbHandler.CreateTable)
	protected.DELETE("/db-instances/databases/:did/tables", dbHandler.DropTable)

	// Database backup
	protected.POST("/db-instances/databases/:did/backup", backupHandler.CreateBackup)
	protected.GET("/db-instances/databases/:did/backups", backupHandler.ListBackups)
	protected.GET("/db-instances/backups/:bid/download", backupHandler.DownloadBackup)
	protected.POST("/db-instances/backups/:bid/restore", backupHandler.RestoreBackup)
	protected.DELETE("/db-instances/backups/:bid", backupHandler.DeleteBackup)

	protected.GET("/db-instances/mysql/common-params", configHandler.GetMySQLCommonParams)
	protected.GET("/db-instances/postgresql/common-params", configHandler.GetPGCommonParams)
	protected.GET("/db-instances/redis/common-params", configHandler.GetRedisCommonParams)
}

// DBServerHandler handles top-level DB server endpoints (list, get).
// Sub-domain endpoints are delegated to focused sub-handlers.
type DBServerHandler struct {
	svc *database.Service
}

func NewDBServerHandler(svc *database.Service) *DBServerHandler {
	return &DBServerHandler{svc: svc}
}

func (h *DBServerHandler) List(c *gin.Context) {
	ctx := c.Request.Context()
	h.svc.RefreshAllStatus(ctx)
	servers, err := h.svc.List(ctx)
	if err != nil {
		c.Error(apperror.WrapError(err))
		return
	}
	httpx.Success(c, servers)
}

func (h *DBServerHandler) Get(c *gin.Context) {
	ctx := c.Request.Context()
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.Error(apperror.ErrBadRequest.WithMessage("无效的 ID"))
		return
	}
	h.svc.RefreshStatus(ctx, id)
	server, err := h.svc.Get(ctx, id)
	if err != nil {
		c.Error(apperror.WrapError(err))
		return
	}
	if server == nil {
		c.Error(apperror.ErrNotFound.WithMessage("数据库服务器不存在"))
		return
	}
	httpx.Success(c, server)
}

// VersionHandler handles DB version management endpoints.
type VersionHandler struct {
	svc *database.Service
}

func NewVersionHandler(svc *database.Service) *VersionHandler {
	return &VersionHandler{svc: svc}
}

func (h *VersionHandler) ListVersions(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.Error(apperror.ErrBadRequest.WithMessage("无效的 ID"))
		return
	}
	versions, err := h.svc.ListVersions(c.Request.Context(), id)
	if err != nil {
		c.Error(apperror.WrapError(err))
		return
	}
	httpx.Success(c, versions)
}

func (h *VersionHandler) InstallVersion(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.Error(apperror.ErrBadRequest.WithMessage("无效的 ID"))
		return
	}
	var req database.CreateDBInstanceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperror.ErrBadRequest.Wrap(err))
		return
	}
	middleware.AuditSummary(c, "安装数据库版本 "+req.Version)
	version, err := h.svc.InstallVersion(c.Request.Context(), id, &req)
	if err != nil {
		c.Error(apperror.WrapError(err))
		return
	}
	httpx.Success(c, version)
}

func (h *VersionHandler) UninstallVersion(c *gin.Context) {
	vid, err := strconv.ParseInt(c.Param("vid"), 10, 64)
	if err != nil {
		c.Error(apperror.ErrBadRequest.WithMessage("无效的版本ID"))
		return
	}
	middleware.AuditSummary(c, "卸载数据库版本 #"+strconv.FormatInt(vid, 10))
	if err := h.svc.UninstallVersion(c.Request.Context(), vid); err != nil {
		c.Error(apperror.WrapError(err))
		return
	}
	httpx.Success(c, gin.H{"message": "已卸载"})
}

func (h *VersionHandler) DestroyVersion(c *gin.Context) {
	vid, err := strconv.ParseInt(c.Param("vid"), 10, 64)
	if err != nil {
		c.Error(apperror.ErrBadRequest.WithMessage("无效的实例ID"))
		return
	}
	middleware.AuditSummary(c, "销毁数据库实例数据 #"+strconv.FormatInt(vid, 10))
	if err := h.svc.DestroyVersion(c.Request.Context(), vid); err != nil {
		c.Error(apperror.WrapError(err))
		return
	}
	httpx.Success(c, gin.H{"message": "实例与数据卷已销毁"})
}

func (h *VersionHandler) ResetAdminPassword(c *gin.Context) {
	vid, err := strconv.ParseInt(c.Param("vid"), 10, 64)
	if err != nil {
		c.Error(apperror.ErrBadRequest.WithMessage("无效的实例ID"))
		return
	}
	password, err := h.svc.ResetAdminPassword(c.Request.Context(), vid)
	if err != nil {
		c.Error(apperror.WrapError(err))
		return
	}
	httpx.Success(c, gin.H{"admin_password": password})
}

func (h *VersionHandler) StartVersion(c *gin.Context) {
	vid, err := strconv.ParseInt(c.Param("vid"), 10, 64)
	if err != nil {
		c.Error(apperror.ErrBadRequest.WithMessage("无效的版本ID"))
		return
	}
	middleware.AuditSummary(c, "启动数据库版本 #"+strconv.FormatInt(vid, 10))
	if err := h.svc.StartVersion(c.Request.Context(), vid); err != nil {
		c.Error(apperror.WrapError(err))
		return
	}
	httpx.Success(c, gin.H{"status": "running"})
}

func (h *VersionHandler) StopVersion(c *gin.Context) {
	vid, err := strconv.ParseInt(c.Param("vid"), 10, 64)
	if err != nil {
		c.Error(apperror.ErrBadRequest.WithMessage("无效的版本ID"))
		return
	}
	middleware.AuditSummary(c, "停止数据库版本 #"+strconv.FormatInt(vid, 10))
	if err := h.svc.StopVersion(c.Request.Context(), vid); err != nil {
		c.Error(apperror.WrapError(err))
		return
	}
	httpx.Success(c, gin.H{"status": "stopped"})
}

func (h *VersionHandler) RestartVersion(c *gin.Context) {
	vid, err := strconv.ParseInt(c.Param("vid"), 10, 64)
	if err != nil {
		c.Error(apperror.ErrBadRequest.WithMessage("无效的版本ID"))
		return
	}
	middleware.AuditSummary(c, "重启数据库版本 #"+strconv.FormatInt(vid, 10))
	if err := h.svc.RestartVersion(c.Request.Context(), vid); err != nil {
		c.Error(apperror.WrapError(err))
		return
	}
	httpx.Success(c, gin.H{"status": "running"})
}

func (h *VersionHandler) UpdateVersionPort(c *gin.Context) {
	vid, err := strconv.ParseInt(c.Param("vid"), 10, 64)
	if err != nil {
		c.Error(apperror.ErrBadRequest.WithMessage("无效的版本ID"))
		return
	}

	var req struct {
		Port int `json:"port" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperror.ErrBadRequest.Wrap(err))
		return
	}
	vInfo, err := h.svc.GetVersion(c.Request.Context(), vid)
	if err != nil {
		c.Error(apperror.ErrNotFound.WithMessage("数据库版本不存在"))
		return
	}
	middleware.AuditSummary(c, "更新数据库端口 ("+vInfo.ContainerName+") "+strconv.Itoa(vInfo.Port)+" -> "+strconv.Itoa(req.Port))

	if req.Port < 1 || req.Port > 65535 {
		c.Error(apperror.ErrBadRequest.WithMessage("端口必须在 1 到 65535 之间"))
		return
	}

	if err := h.svc.UpdateVersionPort(c.Request.Context(), vid, req.Port); err != nil {
		c.Error(apperror.WrapError(err))
		return
	}

	httpx.Success(c, gin.H{"message": "端口已更新", "port": req.Port})
}

func (h *VersionHandler) GetVersionLogs(c *gin.Context) {
	vid, err := strconv.ParseInt(c.Param("vid"), 10, 64)
	if err != nil {
		c.Error(apperror.ErrBadRequest.WithMessage("无效的版本ID"))
		return
	}
	lines, _ := strconv.Atoi(c.DefaultQuery("lines", "200"))
	logs, err := h.svc.GetVersionServiceLogs(c.Request.Context(), vid, lines)
	if err != nil {
		c.Error(apperror.WrapError(err))
		return
	}
	httpx.Success(c, gin.H{"logs": logs})
}

func (h *VersionHandler) GetVersionConfig(c *gin.Context) {
	vid, err := strconv.ParseInt(c.Param("vid"), 10, 64)
	if err != nil {
		c.Error(apperror.ErrBadRequest.WithMessage("无效的实例ID"))
		return
	}
	content, path, err := h.svc.GetVersionConfig(c.Request.Context(), vid)
	if err != nil {
		c.Error(apperror.WrapError(err))
		return
	}
	httpx.Success(c, gin.H{"file_path": path, "content": content})
}

func (h *VersionHandler) SaveVersionConfig(c *gin.Context) {
	vid, err := strconv.ParseInt(c.Param("vid"), 10, 64)
	if err != nil {
		c.Error(apperror.ErrBadRequest.WithMessage("无效的实例ID"))
		return
	}
	var req struct {
		Content string `json:"content" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperror.ErrBadRequest.Wrap(err))
		return
	}
	if err := h.svc.SaveVersionConfig(c.Request.Context(), vid, req.Content); err != nil {
		c.Error(apperror.WrapError(err))
		return
	}
	httpx.Success(c, gin.H{"message": "实例配置已保存，重启后生效"})
}

// DatabaseHandler handles database CRUD, introspection, and table management endpoints.
type DatabaseHandler struct {
	svc *database.Service
}

func NewDatabaseHandler(svc *database.Service) *DatabaseHandler {
	return &DatabaseHandler{
		svc: svc,
	}
}

// --- Database CRUD ---

func (h *DatabaseHandler) ListDatabases(c *gin.Context) {
	sid, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.Error(apperror.ErrBadRequest.WithMessage("无效的服务器ID"))
		return
	}
	dbs, err := h.svc.ListDatabases(c.Request.Context(), sid)
	if err != nil {
		c.Error(apperror.WrapError(err))
		return
	}
	httpx.Success(c, dbs)
}

func (h *DatabaseHandler) CreateDatabase(c *gin.Context) {
	sid, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.Error(apperror.ErrBadRequest.WithMessage("无效的服务器ID"))
		return
	}
	var req database.CreateDatabaseRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperror.ErrBadRequest.Wrap(err))
		return
	}
	middleware.AuditSummary(c, "创建数据库 "+req.Name)
	db, err := h.svc.CreateDatabase(c.Request.Context(), sid, &req)
	if err != nil {
		c.Error(apperror.WrapError(err))
		return
	}
	httpx.Success(c, db)
}

func (h *DatabaseHandler) DeleteDatabase(c *gin.Context) {
	sid, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.Error(apperror.ErrBadRequest.WithMessage("无效的服务器ID"))
		return
	}
	dbID, err := strconv.ParseInt(c.Param("did"), 10, 64)
	if err != nil {
		c.Error(apperror.ErrBadRequest.WithMessage("无效的数据库ID"))
		return
	}
	dbInfo, err := h.svc.GetDatabaseByID(c.Request.Context(), dbID)
	if err != nil {
		c.Error(apperror.ErrNotFound.WithMessage("数据库不存在"))
		return
	}
	middleware.AuditSummary(c, "删除数据库 "+dbInfo.Name)
	if err := h.svc.DeleteDatabase(c.Request.Context(), sid, dbID); err != nil {
		c.Error(apperror.WrapError(err))
		return
	}
	httpx.Success(c, nil)
}

// --- Database introspection ---

func (h *DatabaseHandler) ListTables(c *gin.Context) {
	did, err := strconv.ParseInt(c.Param("did"), 10, 64)
	if err != nil {
		c.Error(apperror.ErrBadRequest.WithMessage("无效的数据库ID"))
		return
	}

	tables, err := h.svc.ListTables(c.Request.Context(), did)
	if err != nil {
		c.Error(apperror.WrapError(err))
		return
	}

	httpx.Success(c, tables)
}

func (h *DatabaseHandler) DescribeTable(c *gin.Context) {
	did, err := strconv.ParseInt(c.Param("did"), 10, 64)
	if err != nil {
		c.Error(apperror.ErrBadRequest.WithMessage("无效的数据库ID"))
		return
	}
	tableName := c.Query("table")
	if tableName == "" {
		c.Error(apperror.ErrBadRequest.WithMessage("表名不能为空"))
		return
	}
	if !database.ValidateTableName(tableName) {
		c.Error(apperror.ErrBadRequest.WithMessage("无效的表名"))
		return
	}

	result, err := h.svc.DescribeTable(c.Request.Context(), did, tableName)
	if err != nil {
		c.Error(apperror.WrapError(err))
		return
	}

	httpx.Success(c, result)
}

func (h *DatabaseHandler) QueryTable(c *gin.Context) {
	did, err := strconv.ParseInt(c.Param("did"), 10, 64)
	if err != nil {
		c.Error(apperror.ErrBadRequest.WithMessage("无效的数据库ID"))
		return
	}
	tableName := c.Query("table")
	if tableName == "" {
		c.Error(apperror.ErrBadRequest.WithMessage("表名不能为空"))
		return
	}
	if !database.ValidateTableName(tableName) {
		c.Error(apperror.ErrBadRequest.WithMessage("无效的表名"))
		return
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "50"))

	result, err := h.svc.QueryTable(c.Request.Context(), did, tableName, page, pageSize)
	if err != nil {
		c.Error(apperror.WrapError(err))
		return
	}

	httpx.Success(c, gin.H{
		"headers":   result.Headers,
		"rows":      result.Rows,
		"total":     result.Total,
		"page":      result.Page,
		"page_size": result.PageSize,
	})
}

func (h *DatabaseHandler) ExecuteSQL(c *gin.Context) {
	did, err := strconv.ParseInt(c.Param("did"), 10, 64)
	if err != nil {
		c.Error(apperror.ErrBadRequest.WithMessage("无效的数据库ID"))
		return
	}

	var req struct {
		SQL string `json:"sql" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperror.ErrBadRequest.Wrap(err))
		return
	}
	dbInfo, err := h.svc.GetDatabaseByID(c.Request.Context(), did)
	if err != nil {
		c.Error(apperror.ErrNotFound.WithMessage("数据库不存在"))
		return
	}
	middleware.AuditSummary(c, "执行SQL (数据库: "+dbInfo.Name+")")

	result, err := h.svc.ExecuteSQL(c.Request.Context(), did, req.SQL)
	if err != nil {
		c.Error(apperror.ErrNotFound.Wrap(err))
		return
	}

	if !result.Success {
		httpx.Success(c, gin.H{"success": false, "error": result.Error})
		return
	}

	httpx.Success(c, gin.H{"success": true, "output": result.Output})
}

func (h *DatabaseHandler) InsertRecord(c *gin.Context) {
	did, err := strconv.ParseInt(c.Param("did"), 10, 64)
	if err != nil {
		c.Error(apperror.ErrBadRequest.WithMessage("无效的数据库ID"))
		return
	}

	var req struct {
		Table string                 `json:"table" binding:"required"`
		Data  map[string]interface{} `json:"data" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperror.ErrBadRequest.Wrap(err))
		return
	}
	middleware.AuditSummary(c, "插入记录到表 "+req.Table)

	result, err := h.svc.InsertRecord(c.Request.Context(), did, req.Table, req.Data, c.Query("dry_run") == "true")
	if err != nil {
		c.Error(apperror.ErrBadRequest.Wrap(err))
		return
	}

	httpx.Success(c, result)
}

func (h *DatabaseHandler) UpdateRecord(c *gin.Context) {
	did, err := strconv.ParseInt(c.Param("did"), 10, 64)
	if err != nil {
		c.Error(apperror.ErrBadRequest.WithMessage("无效的数据库ID"))
		return
	}

	var req struct {
		Table      string                 `json:"table" binding:"required"`
		Data       map[string]interface{} `json:"data" binding:"required"`
		PrimaryKey string                 `json:"primary_key" binding:"required"`
		PrimaryVal interface{}            `json:"primary_val" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperror.ErrBadRequest.Wrap(err))
		return
	}
	middleware.AuditSummary(c, "更新表 "+req.Table+" 记录")

	result, err := h.svc.UpdateRecord(c.Request.Context(), did, req.Table, req.Data, req.PrimaryKey, req.PrimaryVal, c.Query("dry_run") == "true")
	if err != nil {
		c.Error(apperror.ErrBadRequest.Wrap(err))
		return
	}

	httpx.Success(c, result)
}

func (h *DatabaseHandler) DeleteRecord(c *gin.Context) {
	did, err := strconv.ParseInt(c.Param("did"), 10, 64)
	if err != nil {
		c.Error(apperror.ErrBadRequest.WithMessage("无效的数据库ID"))
		return
	}

	var req struct {
		Table      string      `json:"table" binding:"required"`
		PrimaryKey string      `json:"primary_key" binding:"required"`
		PrimaryVal interface{} `json:"primary_val" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperror.ErrBadRequest.Wrap(err))
		return
	}
	middleware.AuditSummary(c, "删除表 "+req.Table+" 记录")

	result, err := h.svc.DeleteRecord(c.Request.Context(), did, req.Table, req.PrimaryKey, req.PrimaryVal, c.Query("dry_run") == "true")
	if err != nil {
		c.Error(apperror.ErrBadRequest.Wrap(err))
		return
	}

	httpx.Success(c, result)
}

// --- Table management ---

func (h *DatabaseHandler) CreateTable(c *gin.Context) {
	did, err := strconv.ParseInt(c.Param("did"), 10, 64)
	if err != nil {
		c.Error(apperror.ErrBadRequest.WithMessage("无效的数据库ID"))
		return
	}

	var req struct {
		Name    string `json:"name" binding:"required"`
		Columns []struct {
			Name      string `json:"name"`
			Type      string `json:"type"`
			Nullable  bool   `json:"nullable"`
			IsPrimary bool   `json:"is_primary"`
			AutoIncr  bool   `json:"auto_incr"`
		} `json:"columns" binding:"required,min=1"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperror.ErrBadRequest.Wrap(err))
		return
	}
	middleware.AuditSummary(c, "创建表 "+req.Name)

	var columns []database.TableColumn
	for _, col := range req.Columns {
		columns = append(columns, database.TableColumn{
			Name:      col.Name,
			Type:      col.Type,
			Nullable:  col.Nullable,
			IsPrimary: col.IsPrimary,
			AutoIncr:  col.AutoIncr,
		})
	}

	if err := h.svc.CreateTable(c.Request.Context(), did, req.Name, columns); err != nil {
		if strings.HasPrefix(err.Error(), "无效") || strings.HasPrefix(err.Error(), "不支持") {
			c.Error(apperror.ErrBadRequest.Wrap(err))
		} else {
			c.Error(apperror.WrapError(err))
		}
		return
	}

	httpx.Success(c, gin.H{"message": "表已创建", "name": req.Name})
}

func (h *DatabaseHandler) DropTable(c *gin.Context) {
	did, err := strconv.ParseInt(c.Param("did"), 10, 64)
	if err != nil {
		c.Error(apperror.ErrBadRequest.WithMessage("无效的数据库ID"))
		return
	}

	tableName := c.Query("table")
	if tableName == "" {
		c.Error(apperror.ErrBadRequest.WithMessage("表名不能为空"))
		return
	}
	middleware.AuditSummary(c, "删除表 "+tableName)

	if err := h.svc.DropTable(c.Request.Context(), did, tableName); err != nil {
		if strings.HasPrefix(err.Error(), "无效") || strings.HasPrefix(err.Error(), "不支持") {
			c.Error(apperror.ErrBadRequest.Wrap(err))
		} else {
			c.Error(apperror.WrapError(err))
		}
		return
	}

	httpx.Success(c, gin.H{"message": "表已删除", "name": tableName})
}

// UserHandler handles DB user management endpoints.
type UserHandler struct {
	svc *database.Service
}

func NewUserHandler(svc *database.Service) *UserHandler {
	return &UserHandler{svc: svc}
}

func (h *UserHandler) ListDBUsers(c *gin.Context) {
	sid, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.Error(apperror.ErrBadRequest.WithMessage("无效的服务器ID"))
		return
	}
	users, err := h.svc.ListDBUsers(c.Request.Context(), sid)
	if err != nil {
		c.Error(apperror.WrapError(err))
		return
	}
	httpx.Success(c, users)
}

func (h *UserHandler) CreateDBUser(c *gin.Context) {
	sid, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.Error(apperror.ErrBadRequest.WithMessage("无效的服务器ID"))
		return
	}
	var req database.CreateDBUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperror.ErrBadRequest.Wrap(err))
		return
	}
	middleware.AuditSummary(c, "创建数据库用户 "+req.Username)
	user, err := h.svc.CreateDBUser(c.Request.Context(), sid, &req)
	if err != nil {
		c.Error(apperror.WrapError(err))
		return
	}
	httpx.Success(c, user)
}

func (h *UserHandler) DeleteDBUser(c *gin.Context) {
	sid, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.Error(apperror.ErrBadRequest.WithMessage("无效的服务器ID"))
		return
	}
	uid, err := strconv.ParseInt(c.Param("uid"), 10, 64)
	if err != nil {
		c.Error(apperror.ErrBadRequest.WithMessage("无效的用户ID"))
		return
	}
	middleware.AuditSummary(c, "删除数据库用户 "+strconv.FormatInt(uid, 10))
	if err := h.svc.DeleteDBUser(c.Request.Context(), sid, uid); err != nil {
		c.Error(apperror.WrapError(err))
		return
	}
	httpx.Success(c, nil)
}

func (h *UserHandler) GrantPrivileges(c *gin.Context) {
	sid, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.Error(apperror.ErrBadRequest.WithMessage("无效的服务器ID"))
		return
	}
	uid, err := strconv.ParseInt(c.Param("uid"), 10, 64)
	if err != nil {
		c.Error(apperror.ErrBadRequest.WithMessage("无效的用户ID"))
		return
	}
	var req database.GrantRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperror.ErrBadRequest.Wrap(err))
		return
	}
	middleware.AuditSummary(c, "授权数据库用户 "+strconv.FormatInt(uid, 10))
	if err := h.svc.GrantPrivileges(c.Request.Context(), sid, uid, &req); err != nil {
		c.Error(apperror.WrapError(err))
		return
	}
	httpx.Success(c, gin.H{"message": "权限已授予"})
}

// BackupHandler handles database backup endpoints.
type BackupHandler struct {
	svc *database.Service
}

func NewBackupHandler(svc *database.Service) *BackupHandler {
	return &BackupHandler{svc: svc}
}

func (h *BackupHandler) CreateBackup(c *gin.Context) {
	did, err := strconv.ParseInt(c.Param("did"), 10, 64)
	if err != nil {
		c.Error(apperror.ErrBadRequest.WithMessage("无效的数据库ID"))
		return
	}
	// Get database info
	db, err := h.svc.GetDatabaseByID(c.Request.Context(), did)
	if err != nil {
		c.Error(apperror.ErrNotFound.WithMessage("数据库不存在"))
		return
	}
	middleware.AuditSummary(c, "创建数据库备份 "+db.Name)

	// Get db server info to determine type
	server, err := h.svc.Get(c.Request.Context(), db.DBServerID)
	if err != nil {
		c.Error(apperror.ErrNotFound.WithMessage("数据库服务器不存在"))
		return
	}

	backup, err := h.svc.CreateBackup(c.Request.Context(), db.DBServerID, db.DBInstanceID, did, db.Name, server.Name)
	if err != nil {
		c.Error(apperror.WrapError(err))
		return
	}

	httpx.Success(c, backup)
}

func (h *BackupHandler) ListBackups(c *gin.Context) {
	did, err := strconv.ParseInt(c.Param("did"), 10, 64)
	if err != nil {
		c.Error(apperror.ErrBadRequest.WithMessage("无效的数据库ID"))
		return
	}

	backups, err := h.svc.ListBackups(c.Request.Context(), did)
	if err != nil {
		c.Error(apperror.WrapError(err))
		return
	}

	if backups == nil {
		backups = []database.DBBackup{}
	}

	httpx.Success(c, backups)
}

func (h *BackupHandler) DownloadBackup(c *gin.Context) {
	bid, err := strconv.ParseInt(c.Param("bid"), 10, 64)
	if err != nil {
		c.Error(apperror.ErrBadRequest.WithMessage("无效的备份ID"))
		return
	}

	backup, err := h.svc.GetBackup(c.Request.Context(), bid)
	if err != nil {
		c.Error(apperror.ErrNotFound.WithMessage("备份不存在"))
		return
	}

	if backup.Status != "completed" {
		c.Error(apperror.ErrBadRequest.WithMessage("备份未完成"))
		return
	}

	// Check if file exists
	if _, err := os.Stat(backup.FilePath); os.IsNotExist(err) {
		c.Error(apperror.ErrNotFound.WithMessage("备份文件不存在"))
		return
	}

	c.File(backup.FilePath)
}

func (h *BackupHandler) RestoreBackup(c *gin.Context) {
	bid, err := strconv.ParseInt(c.Param("bid"), 10, 64)
	if err != nil {
		c.Error(apperror.ErrBadRequest.WithMessage("无效的备份ID"))
		return
	}

	// Require confirmation
	var req struct {
		Confirm bool `json:"confirm"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || !req.Confirm {
		c.Error(apperror.ErrBadRequest.WithMessage("请确认恢复，设置 {\"confirm\": true}"))
		return
	}
	backup, err := h.svc.GetBackup(c.Request.Context(), bid)
	if err != nil {
		c.Error(apperror.ErrNotFound.WithMessage("备份不存在"))
		return
	}
	middleware.AuditSummary(c, "恢复数据库备份 "+backup.DatabaseName)

	// Get db server info to determine type
	server, err := h.svc.Get(c.Request.Context(), backup.DBServerID)
	if err != nil {
		c.Error(apperror.ErrNotFound.WithMessage("数据库服务器不存在"))
		return
	}

	if err := h.svc.RestoreBackup(c.Request.Context(), bid, server.Name); err != nil {
		c.Error(apperror.WrapError(err))
		return
	}

	httpx.Success(c, gin.H{"message": "数据库恢复成功"})
}

func (h *BackupHandler) DeleteBackup(c *gin.Context) {
	bid, err := strconv.ParseInt(c.Param("bid"), 10, 64)
	if err != nil {
		c.Error(apperror.ErrBadRequest.WithMessage("无效的备份ID"))
		return
	}
	backup, err := h.svc.GetBackup(c.Request.Context(), bid)
	if err != nil {
		c.Error(apperror.ErrNotFound.WithMessage("备份不存在"))
		return
	}
	middleware.AuditSummary(c, "删除数据库备份 "+backup.DatabaseName)

	if err := h.svc.DeleteBackup(c.Request.Context(), bid); err != nil {
		c.Error(apperror.WrapError(err))
		return
	}

	httpx.Success(c, gin.H{"message": "备份已删除"})
}

// ConfigHandler handles MySQL/PostgreSQL/Redis config management endpoints.
// These use package-level functions from the service package and have no service struct dependency.
type ConfigHandler struct{}

func NewConfigHandler() *ConfigHandler {
	return &ConfigHandler{}
}

// --- MySQL Config ---

func (h *ConfigHandler) GetMySQLConfig(c *gin.Context) {
	configPath := database.FindMySQLConfig()
	if configPath == "" {
		httpx.Success(c, gin.H{"found": false, "message": "未找到 MySQL 配置文件"})
		return
	}

	config, err := database.ParseMySQLConfig(configPath)
	if err != nil {
		c.Error(apperror.WrapError(err))
		return
	}

	// Build response with common params metadata
	response := gin.H{
		"found":    true,
		"config":   config,
		"sections": gin.H{},
	}
	sections := response["sections"].(gin.H)
	for _, section := range config.Sections {
		sections[section.Name] = gin.H{
			"params": section.Params,
			"meta":   database.GetCommonParams(section.Name),
		}
	}

	httpx.Success(c, response)
}

func (h *ConfigHandler) SaveMySQLConfig(c *gin.Context) {
	var req struct {
		Sections []struct {
			Name   string            `json:"name"`
			Params map[string]string `json:"params"`
		} `json:"sections"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperror.ErrBadRequest.Wrap(err))
		return
	}

	middleware.AuditSummary(c, "保存 MySQL 配置")
	configPath := database.FindMySQLConfig()
	if configPath == "" {
		c.Error(apperror.ErrBadRequest.WithMessage("未找到 MySQL 配置文件"))
		return
	}

	// Handle raw text save from the raw text modal
	if raw, ok := isRawConfigRequest(req.Sections); ok {
		if err := saveRawConfig(configPath, raw); err != nil {
			c.Error(apperror.WrapError(err))
			return
		}
		httpx.Success(c, gin.H{"message": "配置已保存", "path": configPath})
		return
	}

	config := &database.DBConfig{
		FilePath: configPath,
	}
	for _, s := range req.Sections {
		config.Sections = append(config.Sections, database.ConfigSection{
			Name:   s.Name,
			Params: s.Params,
		})
	}

	if err := database.SaveMySQLConfig(config); err != nil {
		c.Error(apperror.WrapError(err))
		return
	}

	httpx.Success(c, gin.H{"message": "配置已保存", "path": configPath})
}

func (h *ConfigHandler) GetMySQLCommonParams(c *gin.Context) {
	section := c.DefaultQuery("section", "mysqld")
	params := database.GetCommonParams(section)
	httpx.Success(c, params)
}

// --- PostgreSQL Config ---

func (h *ConfigHandler) GetPostgreSQLConfig(c *gin.Context) {
	configPath := database.FindPostgreSQLConfig()
	if configPath == "" {
		httpx.Success(c, gin.H{"found": false, "message": "未找到 PostgreSQL 配置文件"})
		return
	}

	config, err := database.ParsePostgreSQLConfig(configPath)
	if err != nil {
		c.Error(apperror.WrapError(err))
		return
	}

	response := gin.H{
		"found":    true,
		"config":   config,
		"sections": gin.H{},
	}
	sections := response["sections"].(gin.H)
	for _, section := range config.Sections {
		sections[section.Name] = gin.H{
			"params": section.Params,
			"meta":   database.GetPostgreSQLCommonParams(),
		}
	}

	httpx.Success(c, response)
}

func (h *ConfigHandler) SavePostgreSQLConfig(c *gin.Context) {
	var req struct {
		Sections []struct {
			Name   string            `json:"name"`
			Params map[string]string `json:"params"`
		} `json:"sections"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperror.ErrBadRequest.Wrap(err))
		return
	}

	middleware.AuditSummary(c, "保存 PostgreSQL 配置")
	configPath := database.FindPostgreSQLConfig()
	if configPath == "" {
		c.Error(apperror.ErrBadRequest.WithMessage("未找到 PostgreSQL 配置文件"))
		return
	}

	// Handle raw text save from the raw text modal
	if raw, ok := isRawConfigRequest(req.Sections); ok {
		if err := saveRawConfig(configPath, raw); err != nil {
			c.Error(apperror.WrapError(err))
			return
		}
		httpx.Success(c, gin.H{"message": "配置已保存", "path": configPath})
		return
	}

	config := &database.DBConfig{
		FilePath: configPath,
	}
	for _, s := range req.Sections {
		config.Sections = append(config.Sections, database.ConfigSection{
			Name:   s.Name,
			Params: s.Params,
		})
	}

	if err := database.SavePostgreSQLConfig(config); err != nil {
		c.Error(apperror.WrapError(err))
		return
	}

	httpx.Success(c, gin.H{"message": "配置已保存", "path": configPath})
}

func (h *ConfigHandler) GetPGCommonParams(c *gin.Context) {
	params := database.GetPostgreSQLCommonParams()
	httpx.Success(c, params)
}

// --- Redis Config ---

func (h *ConfigHandler) GetRedisConfig(c *gin.Context) {
	configPath := database.FindRedisConfig()
	if configPath == "" {
		httpx.Success(c, gin.H{"found": false, "message": "未找到 Redis 配置文件"})
		return
	}

	config, err := database.ParseRedisConfig(configPath)
	if err != nil {
		c.Error(apperror.WrapError(err))
		return
	}

	response := gin.H{
		"found":    true,
		"config":   config,
		"sections": gin.H{},
	}
	sections := response["sections"].(gin.H)
	for _, section := range config.Sections {
		sections[section.Name] = gin.H{
			"params": section.Params,
			"meta":   database.GetRedisCommonParams(),
		}
	}

	httpx.Success(c, response)
}

func (h *ConfigHandler) SaveRedisConfig(c *gin.Context) {
	var req struct {
		Sections []struct {
			Name   string            `json:"name"`
			Params map[string]string `json:"params"`
		} `json:"sections"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperror.ErrBadRequest.Wrap(err))
		return
	}

	middleware.AuditSummary(c, "保存 Redis 配置")
	configPath := database.FindRedisConfig()
	if configPath == "" {
		c.Error(apperror.ErrBadRequest.WithMessage("未找到 Redis 配置文件"))
		return
	}

	// Handle raw text save from the raw text modal
	if raw, ok := isRawConfigRequest(req.Sections); ok {
		if err := saveRawConfig(configPath, raw); err != nil {
			c.Error(apperror.WrapError(err))
			return
		}
		httpx.Success(c, gin.H{"message": "配置已保存", "path": configPath})
		return
	}

	config := &database.DBConfig{
		FilePath: configPath,
	}
	for _, s := range req.Sections {
		config.Sections = append(config.Sections, database.ConfigSection{
			Name:   s.Name,
			Params: s.Params,
		})
	}

	if err := database.SaveRedisConfig(config); err != nil {
		c.Error(apperror.WrapError(err))
		return
	}

	httpx.Success(c, gin.H{"message": "配置已保存", "path": configPath})
}

func (h *ConfigHandler) GetRedisCommonParams(c *gin.Context) {
	params := database.GetRedisCommonParams()
	httpx.Success(c, params)
}

// --- Helper functions ---

func saveRawConfig(filePath, content string) error {
	backupPath := filePath + ".bak." + time.Now().Format("20060102150405")
	if data, err := os.ReadFile(filePath); err == nil {
		if err := os.WriteFile(backupPath, data, 0644); err != nil {
			return fmt.Errorf("backup config: %w", err)
		}
	}
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	return os.WriteFile(filePath, []byte(content), 0644)
}

func isRawConfigRequest(sections []struct {
	Name   string            `json:"name"`
	Params map[string]string `json:"params"`
}) (string, bool) {
	if len(sections) == 1 && sections[0].Name == "custom" {
		if raw, ok := sections[0].Params["raw"]; ok {
			return raw, true
		}
	}
	return "", false
}

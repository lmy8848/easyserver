package http

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"easyserver/internal/database_mgmt"
	"easyserver/internal/dbserver"
	"easyserver/internal/httpx"
	"easyserver/internal/httpx/middleware"
	"easyserver/internal/infra/apperror"

	"github.com/gin-gonic/gin"
)

// RegisterRoutes registers database management routes
func RegisterRoutes(protected *gin.RouterGroup, dbServerService *dbserver.Service, dbMgmtService *database_mgmt.Service) {
	handler := NewDBServerHandler(dbServerService)
	versionHandler := NewVersionHandler(dbServerService)
	dbHandler := NewDatabaseHandler(dbMgmtService)
	userHandler := NewUserHandler(dbMgmtService)
	backupHandler := NewBackupHandler(dbServerService, dbMgmtService)
	configHandler := NewConfigHandler()

	protected.GET("/db-servers", handler.List)
	protected.GET("/db-servers/:id", handler.Get)

	// Version management
	protected.GET("/db-servers/:id/version-templates", versionHandler.GetVersionTemplates)
	protected.GET("/db-servers/:id/versions", versionHandler.ListVersions)
	protected.POST("/db-servers/:id/versions", versionHandler.InstallVersion)
	protected.DELETE("/db-servers/versions/:vid", versionHandler.UninstallVersion)
	protected.POST("/db-servers/versions/:vid/start", versionHandler.StartVersion)
	protected.POST("/db-servers/versions/:vid/stop", versionHandler.StopVersion)
	protected.POST("/db-servers/versions/:vid/restart", versionHandler.RestartVersion)
	protected.PUT("/db-servers/versions/:vid/port", versionHandler.UpdateVersionPort)
	protected.GET("/db-servers/versions/:vid/logs", versionHandler.GetVersionLogs)

	// Databases nested
	protected.GET("/db-servers/:id/databases", dbHandler.ListDatabases)
	protected.POST("/db-servers/:id/databases", dbHandler.CreateDatabase)
	protected.DELETE("/db-servers/:id/databases/:did", dbHandler.DeleteDatabase)

	// DB Users nested
	protected.GET("/db-servers/:id/users", userHandler.ListDBUsers)
	protected.POST("/db-servers/:id/users", userHandler.CreateDBUser)
	protected.DELETE("/db-servers/:id/users/:uid", userHandler.DeleteDBUser)
	protected.POST("/db-servers/:id/users/:uid/grant", userHandler.GrantPrivileges)

	// Database introspection
	protected.GET("/db-servers/databases/:did/tables", dbHandler.ListTables)
	protected.GET("/db-servers/databases/:did/describe", dbHandler.DescribeTable)
	protected.GET("/db-servers/databases/:did/query", dbHandler.QueryTable)
	protected.POST("/db-servers/databases/:did/execute", dbHandler.ExecuteSQL)
	protected.POST("/db-servers/databases/:did/insert", dbHandler.InsertRecord)
	protected.POST("/db-servers/databases/:did/update", dbHandler.UpdateRecord)
	protected.POST("/db-servers/databases/:did/delete", dbHandler.DeleteRecord)

	// Table management
	protected.POST("/db-servers/databases/:did/tables", dbHandler.CreateTable)
	protected.DELETE("/db-servers/databases/:did/tables", dbHandler.DropTable)

	// Database backup
	protected.POST("/db-servers/databases/:did/backup", backupHandler.CreateBackup)
	protected.GET("/db-servers/databases/:did/backups", backupHandler.ListBackups)
	protected.GET("/db-servers/backups/:bid/download", backupHandler.DownloadBackup)
	protected.POST("/db-servers/backups/:bid/restore", backupHandler.RestoreBackup)
	protected.DELETE("/db-servers/backups/:bid", backupHandler.DeleteBackup)

	// MySQL config management
	protected.GET("/db-servers/mysql/config", configHandler.GetMySQLConfig)
	protected.POST("/db-servers/mysql/config", configHandler.SaveMySQLConfig)
	protected.GET("/db-servers/mysql/common-params", configHandler.GetMySQLCommonParams)

	// PostgreSQL config management
	protected.GET("/db-servers/postgresql/config", configHandler.GetPostgreSQLConfig)
	protected.POST("/db-servers/postgresql/config", configHandler.SavePostgreSQLConfig)
	protected.GET("/db-servers/postgresql/common-params", configHandler.GetPGCommonParams)

	// Redis config management
	protected.GET("/db-servers/redis/config", configHandler.GetRedisConfig)
	protected.POST("/db-servers/redis/config", configHandler.SaveRedisConfig)
	protected.GET("/db-servers/redis/common-params", configHandler.GetRedisCommonParams)
}

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
	h.dbServerService.RefreshStatus(ctx, id)
	server, err := h.dbServerService.Get(ctx, id)
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
	dbServerService *dbserver.Service
}

func NewVersionHandler(dbServerService *dbserver.Service) *VersionHandler {
	return &VersionHandler{dbServerService: dbServerService}
}

func (h *VersionHandler) GetVersionTemplates(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.Error(apperror.ErrBadRequest.WithMessage("无效的 ID"))
		return
	}
	server, err := h.dbServerService.Get(c.Request.Context(), id)
	if err != nil || server == nil {
		c.Error(apperror.ErrNotFound.WithMessage("数据库服务器不存在"))
		return
	}
	templates := dbserver.GetVersionTemplates(server.Name)
	httpx.Success(c, templates)
}

func (h *VersionHandler) ListVersions(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.Error(apperror.ErrBadRequest.WithMessage("无效的 ID"))
		return
	}
	versions, err := h.dbServerService.ListVersions(c.Request.Context(), id)
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
	var req dbserver.CreateDBVersionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperror.ErrBadRequest.Wrap(err))
		return
	}
	middleware.AuditSummary(c, "安装数据库版本 "+req.Version)
	version, err := h.dbServerService.InstallVersion(c.Request.Context(), id, &req)
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
	if err := h.dbServerService.UninstallVersion(c.Request.Context(), vid); err != nil {
		c.Error(apperror.WrapError(err))
		return
	}
	httpx.Success(c, gin.H{"message": "已卸载"})
}

func (h *VersionHandler) StartVersion(c *gin.Context) {
	vid, err := strconv.ParseInt(c.Param("vid"), 10, 64)
	if err != nil {
		c.Error(apperror.ErrBadRequest.WithMessage("无效的版本ID"))
		return
	}
	middleware.AuditSummary(c, "启动数据库版本 #"+strconv.FormatInt(vid, 10))
	if err := h.dbServerService.StartVersion(c.Request.Context(), vid); err != nil {
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
	if err := h.dbServerService.StopVersion(c.Request.Context(), vid); err != nil {
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
	if err := h.dbServerService.RestartVersion(c.Request.Context(), vid); err != nil {
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
	vInfo, err := h.dbServerService.GetVersion(c.Request.Context(), vid)
	if err != nil {
		c.Error(apperror.ErrNotFound.WithMessage("数据库版本不存在"))
		return
	}
	middleware.AuditSummary(c, "更新数据库端口 ("+vInfo.ServiceName+") "+strconv.Itoa(vInfo.Port)+" -> "+strconv.Itoa(req.Port))

	if req.Port < 1 || req.Port > 65535 {
		c.Error(apperror.ErrBadRequest.WithMessage("端口必须在 1 到 65535 之间"))
		return
	}

	if err := h.dbServerService.UpdateVersionPort(c.Request.Context(), vid, req.Port); err != nil {
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
	logs, err := h.dbServerService.GetVersionServiceLogs(c.Request.Context(), vid, lines)
	if err != nil {
		c.Error(apperror.WrapError(err))
		return
	}
	httpx.Success(c, gin.H{"logs": logs})
}

// DatabaseHandler handles database CRUD, introspection, and table management endpoints.
type DatabaseHandler struct {
	dbMgmtService *database_mgmt.Service
	sqlService    *database_mgmt.Service
}

func NewDatabaseHandler(dbMgmtService *database_mgmt.Service) *DatabaseHandler {
	return &DatabaseHandler{
		dbMgmtService: dbMgmtService,
	}
}

// --- Database CRUD ---

func (h *DatabaseHandler) ListDatabases(c *gin.Context) {
	sid, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.Error(apperror.ErrBadRequest.WithMessage("无效的服务器ID"))
		return
	}
	dbs, err := h.dbMgmtService.ListDatabases(c.Request.Context(), sid)
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
	var req database_mgmt.CreateDatabaseRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperror.ErrBadRequest.Wrap(err))
		return
	}
	middleware.AuditSummary(c, "创建数据库 "+req.Name)
	db, err := h.dbMgmtService.CreateDatabase(c.Request.Context(), sid, &req)
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
	dbInfo, err := h.dbMgmtService.GetDatabaseByID(c.Request.Context(), dbID)
	if err != nil {
		c.Error(apperror.ErrNotFound.WithMessage("数据库不存在"))
		return
	}
	middleware.AuditSummary(c, "删除数据库 "+dbInfo.Name)
	if err := h.dbMgmtService.DeleteDatabase(c.Request.Context(), sid, dbID); err != nil {
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

	tables, err := h.dbMgmtService.ListTables(c.Request.Context(), did)
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
	if !database_mgmt.ValidateTableName(tableName) {
		c.Error(apperror.ErrBadRequest.WithMessage("无效的表名"))
		return
	}

	result, err := h.dbMgmtService.DescribeTable(c.Request.Context(), did, tableName)
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
	if !database_mgmt.ValidateTableName(tableName) {
		c.Error(apperror.ErrBadRequest.WithMessage("无效的表名"))
		return
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "50"))

	result, err := h.dbMgmtService.QueryTable(c.Request.Context(), did, tableName, page, pageSize)
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
	dbInfo, err := h.dbMgmtService.GetDatabaseByID(c.Request.Context(), did)
	if err != nil {
		c.Error(apperror.ErrNotFound.WithMessage("数据库不存在"))
		return
	}
	middleware.AuditSummary(c, "执行SQL (数据库: "+dbInfo.Name+")")

	result, err := h.dbMgmtService.ExecuteSQL(c.Request.Context(), did, req.SQL)
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

	result, err := h.dbMgmtService.InsertRecord(c.Request.Context(), did, req.Table, req.Data, c.Query("dry_run") == "true")
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

	result, err := h.dbMgmtService.UpdateRecord(c.Request.Context(), did, req.Table, req.Data, req.PrimaryKey, req.PrimaryVal, c.Query("dry_run") == "true")
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

	result, err := h.dbMgmtService.DeleteRecord(c.Request.Context(), did, req.Table, req.PrimaryKey, req.PrimaryVal, c.Query("dry_run") == "true")
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

	var columns []database_mgmt.TableColumn
	for _, col := range req.Columns {
		columns = append(columns, database_mgmt.TableColumn{
			Name:      col.Name,
			Type:      col.Type,
			Nullable:  col.Nullable,
			IsPrimary: col.IsPrimary,
			AutoIncr:  col.AutoIncr,
		})
	}

	if err := h.dbMgmtService.CreateTable(c.Request.Context(), did, req.Name, columns); err != nil {
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

	if err := h.dbMgmtService.DropTable(c.Request.Context(), did, tableName); err != nil {
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
	dbMgmtService *database_mgmt.Service
}

func NewUserHandler(dbMgmtService *database_mgmt.Service) *UserHandler {
	return &UserHandler{dbMgmtService: dbMgmtService}
}

func (h *UserHandler) ListDBUsers(c *gin.Context) {
	sid, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.Error(apperror.ErrBadRequest.WithMessage("无效的服务器ID"))
		return
	}
	users, err := h.dbMgmtService.ListDBUsers(c.Request.Context(), sid)
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
	var req database_mgmt.CreateDBUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperror.ErrBadRequest.Wrap(err))
		return
	}
	middleware.AuditSummary(c, "创建数据库用户 "+req.Username)
	user, err := h.dbMgmtService.CreateDBUser(c.Request.Context(), sid, &req)
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
	if err := h.dbMgmtService.DeleteDBUser(c.Request.Context(), sid, uid); err != nil {
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
	var req database_mgmt.GrantRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperror.ErrBadRequest.Wrap(err))
		return
	}
	middleware.AuditSummary(c, "授权数据库用户 "+strconv.FormatInt(uid, 10))
	if err := h.dbMgmtService.GrantPrivileges(c.Request.Context(), sid, uid, &req); err != nil {
		c.Error(apperror.WrapError(err))
		return
	}
	httpx.Success(c, gin.H{"message": "权限已授予"})
}

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
		c.Error(apperror.ErrBadRequest.WithMessage("无效的数据库ID"))
		return
	}
	// Get database info
	db, err := h.dbMgmtService.GetDatabaseByID(c.Request.Context(), did)
	if err != nil {
		c.Error(apperror.ErrNotFound.WithMessage("数据库不存在"))
		return
	}
	middleware.AuditSummary(c, "创建数据库备份 "+db.Name)

	// Get db server info to determine type
	server, err := h.dbServerService.Get(c.Request.Context(), db.DBServerID)
	if err != nil {
		c.Error(apperror.ErrNotFound.WithMessage("数据库服务器不存在"))
		return
	}

	backup, err := h.dbMgmtService.CreateBackup(c.Request.Context(), db.DBServerID, db.DBVersionID, did, db.Name, server.Name)
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

	backups, err := h.dbMgmtService.ListBackups(c.Request.Context(), did)
	if err != nil {
		c.Error(apperror.WrapError(err))
		return
	}

	if backups == nil {
		backups = []database_mgmt.DBBackup{}
	}

	httpx.Success(c, backups)
}

func (h *BackupHandler) DownloadBackup(c *gin.Context) {
	bid, err := strconv.ParseInt(c.Param("bid"), 10, 64)
	if err != nil {
		c.Error(apperror.ErrBadRequest.WithMessage("无效的备份ID"))
		return
	}

	backup, err := h.dbMgmtService.GetBackup(c.Request.Context(), bid)
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
	backup, err := h.dbMgmtService.GetBackup(c.Request.Context(), bid)
	if err != nil {
		c.Error(apperror.ErrNotFound.WithMessage("备份不存在"))
		return
	}
	middleware.AuditSummary(c, "恢复数据库备份 "+backup.DatabaseName)

	// Get db server info to determine type
	server, err := h.dbServerService.Get(c.Request.Context(), backup.DBServerID)
	if err != nil {
		c.Error(apperror.ErrNotFound.WithMessage("数据库服务器不存在"))
		return
	}

	if err := h.dbMgmtService.RestoreBackup(c.Request.Context(), bid, server.Name); err != nil {
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
	backup, err := h.dbMgmtService.GetBackup(c.Request.Context(), bid)
	if err != nil {
		c.Error(apperror.ErrNotFound.WithMessage("备份不存在"))
		return
	}
	middleware.AuditSummary(c, "删除数据库备份 "+backup.DatabaseName)

	if err := h.dbMgmtService.DeleteBackup(c.Request.Context(), bid); err != nil {
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
	configPath := database_mgmt.FindMySQLConfig()
	if configPath == "" {
		httpx.Success(c, gin.H{"found": false, "message": "未找到 MySQL 配置文件"})
		return
	}

	config, err := database_mgmt.ParseMySQLConfig(configPath)
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
			"meta":   database_mgmt.GetCommonParams(section.Name),
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
	configPath := database_mgmt.FindMySQLConfig()
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

	config := &database_mgmt.DBConfig{
		FilePath: configPath,
	}
	for _, s := range req.Sections {
		config.Sections = append(config.Sections, database_mgmt.ConfigSection{
			Name:   s.Name,
			Params: s.Params,
		})
	}

	if err := database_mgmt.SaveMySQLConfig(config); err != nil {
		c.Error(apperror.WrapError(err))
		return
	}

	httpx.Success(c, gin.H{"message": "配置已保存", "path": configPath})
}

func (h *ConfigHandler) GetMySQLCommonParams(c *gin.Context) {
	section := c.DefaultQuery("section", "mysqld")
	params := database_mgmt.GetCommonParams(section)
	httpx.Success(c, params)
}

// --- PostgreSQL Config ---

func (h *ConfigHandler) GetPostgreSQLConfig(c *gin.Context) {
	configPath := database_mgmt.FindPostgreSQLConfig()
	if configPath == "" {
		httpx.Success(c, gin.H{"found": false, "message": "未找到 PostgreSQL 配置文件"})
		return
	}

	config, err := database_mgmt.ParsePostgreSQLConfig(configPath)
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
			"meta":   database_mgmt.GetPostgreSQLCommonParams(),
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
	configPath := database_mgmt.FindPostgreSQLConfig()
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

	config := &database_mgmt.DBConfig{
		FilePath: configPath,
	}
	for _, s := range req.Sections {
		config.Sections = append(config.Sections, database_mgmt.ConfigSection{
			Name:   s.Name,
			Params: s.Params,
		})
	}

	if err := database_mgmt.SavePostgreSQLConfig(config); err != nil {
		c.Error(apperror.WrapError(err))
		return
	}

	httpx.Success(c, gin.H{"message": "配置已保存", "path": configPath})
}

func (h *ConfigHandler) GetPGCommonParams(c *gin.Context) {
	params := database_mgmt.GetPostgreSQLCommonParams()
	httpx.Success(c, params)
}

// --- Redis Config ---

func (h *ConfigHandler) GetRedisConfig(c *gin.Context) {
	configPath := database_mgmt.FindRedisConfig()
	if configPath == "" {
		httpx.Success(c, gin.H{"found": false, "message": "未找到 Redis 配置文件"})
		return
	}

	config, err := database_mgmt.ParseRedisConfig(configPath)
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
			"meta":   database_mgmt.GetRedisCommonParams(),
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
	configPath := database_mgmt.FindRedisConfig()
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

	config := &database_mgmt.DBConfig{
		FilePath: configPath,
	}
	for _, s := range req.Sections {
		config.Sections = append(config.Sections, database_mgmt.ConfigSection{
			Name:   s.Name,
			Params: s.Params,
		})
	}

	if err := database_mgmt.SaveRedisConfig(config); err != nil {
		c.Error(apperror.WrapError(err))
		return
	}

	httpx.Success(c, gin.H{"message": "配置已保存", "path": configPath})
}

func (h *ConfigHandler) GetRedisCommonParams(c *gin.Context) {
	params := database_mgmt.GetRedisCommonParams()
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

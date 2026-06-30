package api

import (
	"strconv"
	"strings"

	"easyserver/internal/api/middleware"
	"easyserver/internal/database_mgmt"
	"github.com/gin-gonic/gin"
)

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
		c.Error(ErrBadRequest.WithMessage("无效的服务器ID"))
		return
	}
	dbs, err := h.dbMgmtService.ListDatabases(c.Request.Context(), sid)
	if err != nil {
		c.Error(WrapError(err))
		return
	}
	Success(c, dbs)
}

func (h *DatabaseHandler) CreateDatabase(c *gin.Context) {
	sid, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.Error(ErrBadRequest.WithMessage("无效的服务器ID"))
		return
	}
	var req database_mgmt.CreateDatabaseRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(ErrBadRequest.Wrap(err))
		return
	}
	middleware.AuditSummary(c, "创建数据库 "+req.Name)
	db, err := h.dbMgmtService.CreateDatabase(c.Request.Context(), sid, &req)
	if err != nil {
		c.Error(WrapError(err))
		return
	}
	Success(c, db)
}

func (h *DatabaseHandler) DeleteDatabase(c *gin.Context) {
	sid, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.Error(ErrBadRequest.WithMessage("无效的服务器ID"))
		return
	}
	dbID, err := strconv.ParseInt(c.Param("did"), 10, 64)
	if err != nil {
		c.Error(ErrBadRequest.WithMessage("无效的数据库ID"))
		return
	}
	dbInfo, err := h.dbMgmtService.GetDatabaseByID(c.Request.Context(), dbID)
	if err != nil {
		c.Error(ErrNotFound.WithMessage("数据库不存在"))
		return
	}
	middleware.AuditSummary(c, "删除数据库 "+dbInfo.Name)
	if err := h.dbMgmtService.DeleteDatabase(c.Request.Context(), sid, dbID); err != nil {
		c.Error(WrapError(err))
		return
	}
	Success(c, nil)
}

// --- Database introspection ---

func (h *DatabaseHandler) ListTables(c *gin.Context) {
	did, err := strconv.ParseInt(c.Param("did"), 10, 64)
	if err != nil {
		c.Error(ErrBadRequest.WithMessage("无效的数据库ID"))
		return
	}

	tables, err := h.dbMgmtService.ListTables(c.Request.Context(), did)
	if err != nil {
		c.Error(WrapError(err))
		return
	}

	Success(c, tables)
}

func (h *DatabaseHandler) DescribeTable(c *gin.Context) {
	did, err := strconv.ParseInt(c.Param("did"), 10, 64)
	if err != nil {
		c.Error(ErrBadRequest.WithMessage("无效的数据库ID"))
		return
	}
	tableName := c.Query("table")
	if tableName == "" {
		c.Error(ErrBadRequest.WithMessage("表名不能为空"))
		return
	}
	if !database_mgmt.ValidateTableName(tableName) {
		c.Error(ErrBadRequest.WithMessage("无效的表名"))
		return
	}

	result, err := h.dbMgmtService.DescribeTable(c.Request.Context(), did, tableName)
	if err != nil {
		c.Error(WrapError(err))
		return
	}

	Success(c, result)
}

func (h *DatabaseHandler) QueryTable(c *gin.Context) {
	did, err := strconv.ParseInt(c.Param("did"), 10, 64)
	if err != nil {
		c.Error(ErrBadRequest.WithMessage("无效的数据库ID"))
		return
	}
	tableName := c.Query("table")
	if tableName == "" {
		c.Error(ErrBadRequest.WithMessage("表名不能为空"))
		return
	}
	if !database_mgmt.ValidateTableName(tableName) {
		c.Error(ErrBadRequest.WithMessage("无效的表名"))
		return
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "50"))

	result, err := h.dbMgmtService.QueryTable(c.Request.Context(), did, tableName, page, pageSize)
	if err != nil {
		c.Error(WrapError(err))
		return
	}

	Success(c, gin.H{
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
		c.Error(ErrBadRequest.WithMessage("无效的数据库ID"))
		return
	}

	var req struct {
		SQL string `json:"sql" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(ErrBadRequest.Wrap(err))
		return
	}
	dbInfo, err := h.dbMgmtService.GetDatabaseByID(c.Request.Context(), did)
	if err != nil {
		c.Error(ErrNotFound.WithMessage("数据库不存在"))
		return
	}
	middleware.AuditSummary(c, "执行SQL (数据库: "+dbInfo.Name+")")

	result, err := h.dbMgmtService.ExecuteSQL(c.Request.Context(), did, req.SQL)
	if err != nil {
		c.Error(ErrNotFound.Wrap(err))
		return
	}

	if !result.Success {
		Success(c, gin.H{"success": false, "error": result.Error})
		return
	}

	Success(c, gin.H{"success": true, "output": result.Output})
}

func (h *DatabaseHandler) InsertRecord(c *gin.Context) {
	did, err := strconv.ParseInt(c.Param("did"), 10, 64)
	if err != nil {
		c.Error(ErrBadRequest.WithMessage("无效的数据库ID"))
		return
	}

	var req struct {
		Table string                 `json:"table" binding:"required"`
		Data  map[string]interface{} `json:"data" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(ErrBadRequest.Wrap(err))
		return
	}
	middleware.AuditSummary(c, "插入记录到表 "+req.Table)

	result, err := h.dbMgmtService.InsertRecord(c.Request.Context(), did, req.Table, req.Data, c.Query("dry_run") == "true")
	if err != nil {
		c.Error(ErrBadRequest.Wrap(err))
		return
	}

	Success(c, result)
}

func (h *DatabaseHandler) UpdateRecord(c *gin.Context) {
	did, err := strconv.ParseInt(c.Param("did"), 10, 64)
	if err != nil {
		c.Error(ErrBadRequest.WithMessage("无效的数据库ID"))
		return
	}

	var req struct {
		Table      string                 `json:"table" binding:"required"`
		Data       map[string]interface{} `json:"data" binding:"required"`
		PrimaryKey string                 `json:"primary_key" binding:"required"`
		PrimaryVal interface{}            `json:"primary_val" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(ErrBadRequest.Wrap(err))
		return
	}
	middleware.AuditSummary(c, "更新表 "+req.Table+" 记录")

	result, err := h.dbMgmtService.UpdateRecord(c.Request.Context(), did, req.Table, req.Data, req.PrimaryKey, req.PrimaryVal, c.Query("dry_run") == "true")
	if err != nil {
		c.Error(ErrBadRequest.Wrap(err))
		return
	}

	Success(c, result)
}

func (h *DatabaseHandler) DeleteRecord(c *gin.Context) {
	did, err := strconv.ParseInt(c.Param("did"), 10, 64)
	if err != nil {
		c.Error(ErrBadRequest.WithMessage("无效的数据库ID"))
		return
	}

	var req struct {
		Table      string      `json:"table" binding:"required"`
		PrimaryKey string      `json:"primary_key" binding:"required"`
		PrimaryVal interface{} `json:"primary_val" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(ErrBadRequest.Wrap(err))
		return
	}
	middleware.AuditSummary(c, "删除表 "+req.Table+" 记录")

	result, err := h.dbMgmtService.DeleteRecord(c.Request.Context(), did, req.Table, req.PrimaryKey, req.PrimaryVal, c.Query("dry_run") == "true")
	if err != nil {
		c.Error(ErrBadRequest.Wrap(err))
		return
	}

	Success(c, result)
}

// --- Table management ---

func (h *DatabaseHandler) CreateTable(c *gin.Context) {
	did, err := strconv.ParseInt(c.Param("did"), 10, 64)
	if err != nil {
		c.Error(ErrBadRequest.WithMessage("无效的数据库ID"))
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
		c.Error(ErrBadRequest.Wrap(err))
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
			c.Error(ErrBadRequest.Wrap(err))
		} else {
			c.Error(WrapError(err))
		}
		return
	}

	Success(c, gin.H{"message": "表已创建", "name": req.Name})
}

func (h *DatabaseHandler) DropTable(c *gin.Context) {
	did, err := strconv.ParseInt(c.Param("did"), 10, 64)
	if err != nil {
		c.Error(ErrBadRequest.WithMessage("无效的数据库ID"))
		return
	}

	tableName := c.Query("table")
	if tableName == "" {
		c.Error(ErrBadRequest.WithMessage("表名不能为空"))
		return
	}
	middleware.AuditSummary(c, "删除表 "+tableName)

	if err := h.dbMgmtService.DropTable(c.Request.Context(), did, tableName); err != nil {
		if strings.HasPrefix(err.Error(), "无效") || strings.HasPrefix(err.Error(), "不支持") {
			c.Error(ErrBadRequest.Wrap(err))
		} else {
			c.Error(WrapError(err))
		}
		return
	}

	Success(c, gin.H{"message": "表已删除", "name": tableName})
}

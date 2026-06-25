package api

import (
	"strconv"
	"strings"

	"easyserver/internal/model"
	"easyserver/internal/service"

	"github.com/gin-gonic/gin"
)

// DatabaseHandler handles database CRUD, introspection, and table management endpoints.
type DatabaseHandler struct {
	dbMgmtService *service.DatabaseMgmtService
	sqlService    *service.SQLQueryService
}

func NewDatabaseHandler(dbMgmtService *service.DatabaseMgmtService, sqlService *service.SQLQueryService) *DatabaseHandler {
	return &DatabaseHandler{
		dbMgmtService: dbMgmtService,
		sqlService:    sqlService,
	}
}

// --- Database CRUD ---

func (h *DatabaseHandler) ListDatabases(c *gin.Context) {
	sid, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		BadRequest(c, "无效的服务器ID")
		return
	}
	dbs, err := h.dbMgmtService.ListDatabases(c.Request.Context(), sid)
	if err != nil {
		InternalError(c, err.Error())
		return
	}
	Success(c, dbs)
}

func (h *DatabaseHandler) CreateDatabase(c *gin.Context) {
	sid, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		BadRequest(c, "无效的服务器ID")
		return
	}
	var req model.CreateDatabaseRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequest(c, err.Error())
		return
	}
	db, err := h.dbMgmtService.CreateDatabase(c.Request.Context(), sid, &req)
	if err != nil {
		InternalError(c, err.Error())
		return
	}
	Success(c, db)
}

func (h *DatabaseHandler) DeleteDatabase(c *gin.Context) {
	sid, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		BadRequest(c, "无效的服务器ID")
		return
	}
	dbID, err := strconv.ParseInt(c.Param("did"), 10, 64)
	if err != nil {
		BadRequest(c, "无效的数据库ID")
		return
	}
	if err := h.dbMgmtService.DeleteDatabase(c.Request.Context(), sid, dbID); err != nil {
		InternalError(c, err.Error())
		return
	}
	Success(c, nil)
}

// --- Database introspection ---

func (h *DatabaseHandler) ListTables(c *gin.Context) {
	did, err := strconv.ParseInt(c.Param("did"), 10, 64)
	if err != nil {
		BadRequest(c, "无效的数据库ID")
		return
	}

	tables, err := h.sqlService.ListTables(c.Request.Context(), did)
	if err != nil {
		InternalError(c, err.Error())
		return
	}

	Success(c, tables)
}

func (h *DatabaseHandler) DescribeTable(c *gin.Context) {
	did, err := strconv.ParseInt(c.Param("did"), 10, 64)
	if err != nil {
		BadRequest(c, "无效的数据库ID")
		return
	}
	tableName := c.Query("table")
	if tableName == "" {
		BadRequest(c, "表名不能为空")
		return
	}
	if !service.ValidateTableName(tableName) {
		BadRequest(c, "无效的表名")
		return
	}

	result, err := h.sqlService.DescribeTable(c.Request.Context(), did, tableName)
	if err != nil {
		InternalError(c, err.Error())
		return
	}

	Success(c, result)
}

func (h *DatabaseHandler) QueryTable(c *gin.Context) {
	did, err := strconv.ParseInt(c.Param("did"), 10, 64)
	if err != nil {
		BadRequest(c, "无效的数据库ID")
		return
	}
	tableName := c.Query("table")
	if tableName == "" {
		BadRequest(c, "表名不能为空")
		return
	}
	if !service.ValidateTableName(tableName) {
		BadRequest(c, "无效的表名")
		return
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "50"))

	result, err := h.sqlService.QueryTable(c.Request.Context(), did, tableName, page, pageSize)
	if err != nil {
		InternalError(c, err.Error())
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
		BadRequest(c, "无效的数据库ID")
		return
	}

	var req struct {
		SQL string `json:"sql" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequest(c, err.Error())
		return
	}

	result, err := h.sqlService.ExecuteSQL(c.Request.Context(), did, req.SQL)
	if err != nil {
		NotFound(c, err.Error())
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
		BadRequest(c, "无效的数据库ID")
		return
	}

	var req struct {
		Table string                 `json:"table" binding:"required"`
		Data  map[string]interface{} `json:"data" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequest(c, err.Error())
		return
	}

	result, err := h.sqlService.InsertRecord(c.Request.Context(), did, req.Table, req.Data, c.Query("dry_run") == "true")
	if err != nil {
		BadRequest(c, err.Error())
		return
	}

	Success(c, result)
}

func (h *DatabaseHandler) UpdateRecord(c *gin.Context) {
	did, err := strconv.ParseInt(c.Param("did"), 10, 64)
	if err != nil {
		BadRequest(c, "无效的数据库ID")
		return
	}

	var req struct {
		Table      string                 `json:"table" binding:"required"`
		Data       map[string]interface{} `json:"data" binding:"required"`
		PrimaryKey string                 `json:"primary_key" binding:"required"`
		PrimaryVal interface{}            `json:"primary_val" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequest(c, err.Error())
		return
	}

	result, err := h.sqlService.UpdateRecord(c.Request.Context(), did, req.Table, req.Data, req.PrimaryKey, req.PrimaryVal, c.Query("dry_run") == "true")
	if err != nil {
		BadRequest(c, err.Error())
		return
	}

	Success(c, result)
}

func (h *DatabaseHandler) DeleteRecord(c *gin.Context) {
	did, err := strconv.ParseInt(c.Param("did"), 10, 64)
	if err != nil {
		BadRequest(c, "无效的数据库ID")
		return
	}

	var req struct {
		Table      string      `json:"table" binding:"required"`
		PrimaryKey string      `json:"primary_key" binding:"required"`
		PrimaryVal interface{} `json:"primary_val" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequest(c, err.Error())
		return
	}

	result, err := h.sqlService.DeleteRecord(c.Request.Context(), did, req.Table, req.PrimaryKey, req.PrimaryVal, c.Query("dry_run") == "true")
	if err != nil {
		BadRequest(c, err.Error())
		return
	}

	Success(c, result)
}

// --- Table management ---

func (h *DatabaseHandler) CreateTable(c *gin.Context) {
	did, err := strconv.ParseInt(c.Param("did"), 10, 64)
	if err != nil {
		BadRequest(c, "无效的数据库ID")
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
		BadRequest(c, err.Error())
		return
	}

	var columns []service.TableColumn
	for _, col := range req.Columns {
		columns = append(columns, service.TableColumn{
			Name:      col.Name,
			Type:      col.Type,
			Nullable:  col.Nullable,
			IsPrimary: col.IsPrimary,
			AutoIncr:  col.AutoIncr,
		})
	}

	if err := h.sqlService.CreateTable(c.Request.Context(), did, req.Name, columns); err != nil {
		if strings.HasPrefix(err.Error(), "无效") || strings.HasPrefix(err.Error(), "不支持") {
			BadRequest(c, err.Error())
		} else {
			InternalError(c, err.Error())
		}
		return
	}

	Success(c, gin.H{"message": "表已创建", "name": req.Name})
}

func (h *DatabaseHandler) DropTable(c *gin.Context) {
	did, err := strconv.ParseInt(c.Param("did"), 10, 64)
	if err != nil {
		BadRequest(c, "无效的数据库ID")
		return
	}

	tableName := c.Query("table")
	if tableName == "" {
		BadRequest(c, "表名不能为空")
		return
	}

	if err := h.sqlService.DropTable(c.Request.Context(), did, tableName); err != nil {
		if strings.HasPrefix(err.Error(), "无效") || strings.HasPrefix(err.Error(), "不支持") {
			BadRequest(c, err.Error())
		} else {
			InternalError(c, err.Error())
		}
		return
	}

	Success(c, gin.H{"message": "表已删除", "name": tableName})
}

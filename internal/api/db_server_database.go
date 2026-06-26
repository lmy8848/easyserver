package api

import (
	"strconv"
	"strings"

	"easyserver/internal/database_mgmt"

	"github.com/gin-gonic/gin"
)

// DatabaseHandler handles database CRUD, introspection, and table management endpoints.
type DatabaseHandler struct {
	dbMgmtService *database_mgmt.Service
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

	var req database_mgmt.CreateDatabaseRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequest(c, "无效的请求: "+err.Error())
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
	did, err := strconv.ParseInt(c.Param("did"), 10, 64)
	if err != nil {
		BadRequest(c, "无效的数据库ID")
		return
	}

	if err := h.dbMgmtService.DeleteDatabase(c.Request.Context(), sid, did); err != nil {
		InternalError(c, err.Error())
		return
	}
	Success(c, gin.H{"message": "数据库已删除"})
}

// --- Database Introspection ---

func (h *DatabaseHandler) ListTables(c *gin.Context) {
	did, err := strconv.ParseInt(c.Param("did"), 10, 64)
	if err != nil {
		BadRequest(c, "无效的数据库ID")
		return
	}

	tables, err := h.dbMgmtService.ListTables(c.Request.Context(), did)
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

	result, err := h.dbMgmtService.DescribeTable(c.Request.Context(), did, tableName)
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

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "50"))

	result, err := h.dbMgmtService.QueryTable(c.Request.Context(), did, tableName, page, pageSize)
	if err != nil {
		InternalError(c, err.Error())
		return
	}
	Success(c, result)
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
		BadRequest(c, "无效的请求: "+err.Error())
		return
	}

	result, err := h.dbMgmtService.ExecuteSQL(c.Request.Context(), did, req.SQL)
	if err != nil {
		InternalError(c, err.Error())
		return
	}
	Success(c, result)
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
		BadRequest(c, "无效的请求: "+err.Error())
		return
	}

	dryRun := c.Query("dry_run") == "true"
	result, err := h.dbMgmtService.InsertRecord(c.Request.Context(), did, req.Table, req.Data, dryRun)
	if err != nil {
		InternalError(c, err.Error())
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
		Table  string                 `json:"table" binding:"required"`
		Data   map[string]interface{} `json:"data" binding:"required"`
		PK     string                 `json:"pk" binding:"required"`
		PKVal  interface{}            `json:"pk_val" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequest(c, "无效的请求: "+err.Error())
		return
	}

	dryRun := c.Query("dry_run") == "true"
	result, err := h.dbMgmtService.UpdateRecord(c.Request.Context(), did, req.Table, req.Data, req.PK, req.PKVal, dryRun)
	if err != nil {
		InternalError(c, err.Error())
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
		Table string      `json:"table" binding:"required"`
		PK    string      `json:"pk" binding:"required"`
		PKVal interface{} `json:"pk_val" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequest(c, "无效的请求: "+err.Error())
		return
	}

	dryRun := c.Query("dry_run") == "true"
	result, err := h.dbMgmtService.DeleteRecord(c.Request.Context(), did, req.Table, req.PK, req.PKVal, dryRun)
	if err != nil {
		InternalError(c, err.Error())
		return
	}
	Success(c, result)
}

// --- Table Management ---

func (h *DatabaseHandler) CreateTable(c *gin.Context) {
	did, err := strconv.ParseInt(c.Param("did"), 10, 64)
	if err != nil {
		BadRequest(c, "无效的数据库ID")
		return
	}

	var req struct {
		Name    string                   `json:"name" binding:"required"`
		Columns []database_mgmt.TableColumn `json:"columns" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequest(c, "无效的请求: "+err.Error())
		return
	}

	if err := h.dbMgmtService.CreateTable(c.Request.Context(), did, req.Name, req.Columns); err != nil {
		InternalError(c, err.Error())
		return
	}
	Success(c, gin.H{"message": "表已创建"})
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

	if err := h.dbMgmtService.DropTable(c.Request.Context(), did, tableName); err != nil {
		InternalError(c, err.Error())
		return
	}
	Success(c, gin.H{"message": "表已删除"})
}

// sanitizeSQLError is a helper to strip file paths from SQL errors.
func sanitizeSQLError(err error) string {
	if err == nil {
		return ""
	}
	return database_mgmt.SanitizeSQLError(err.Error())
}

// validateTableName is a helper to validate table names.
func validateTableName(name string) bool {
	return database_mgmt.ValidateTableName(name)
}

// splitQualifiedName splits "schema.table" or "table" into (schema, table).
func splitQualifiedName(name string) (string, string) {
	parts := strings.SplitN(name, ".", 2)
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return "public", name
}

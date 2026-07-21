package http

import (
	"easyserver/internal/deploy"
	"easyserver/internal/httpx"
	"easyserver/internal/httpx/middleware"
	"easyserver/internal/infra/apperror"
	"github.com/gin-gonic/gin"
	"strconv"
	"strings"
)

type DeployHandler struct {
	deployService *deploy.Service
}

func NewDeployHandler(db *deploy.Service) *DeployHandler {
	return &DeployHandler{deployService: db}
}

// Server endpoints

func (h *DeployHandler) ListServers(c *gin.Context) {
	servers, err := h.deployService.ListServers(c.Request.Context())
	if err != nil {
		c.Error(apperror.WrapError(err))
		return
	}
	httpx.Success(c, servers)
}

func (h *DeployHandler) GetServer(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.Error(apperror.ErrBadRequest.WithMessage("invalid server id"))
		return
	}

	srv, err := h.deployService.GetServer(c.Request.Context(), id)
	if err != nil {
		c.Error(apperror.ErrNotFound.Wrap(err))
		return
	}

	httpx.Success(c, srv)
}

func (h *DeployHandler) CreateServer(c *gin.Context) {
	var srv deploy.Server
	if err := c.ShouldBindJSON(&srv); err != nil {
		c.Error(apperror.ErrBadRequest.Wrap(err))
		return
	}

	// Input validation
	if srv.Name == "" {
		c.Error(apperror.ErrBadRequest.WithMessage("server name is required"))
		return
	}
	if srv.Host == "" {
		c.Error(apperror.ErrBadRequest.WithMessage("host is required"))
		return
	}
	if srv.Port < 1 || srv.Port > 65535 {
		c.Error(apperror.ErrBadRequest.WithMessage("port must be between 1 and 65535"))
		return
	}
	if srv.Username == "" {
		c.Error(apperror.ErrBadRequest.WithMessage("username is required"))
		return
	}
	if srv.AuthType != "password" && srv.AuthType != "key" {
		c.Error(apperror.ErrBadRequest.WithMessage("auth_type must be 'password' or 'key'"))
		return
	}
	if srv.AuthData == "" {
		c.Error(apperror.ErrBadRequest.WithMessage("auth_data is required"))
		return
	}

	middleware.AuditSummary(c, "创建部署服务器 "+srv.Name)
	if err := h.deployService.CreateServer(c.Request.Context(), &srv); err != nil {
		c.Error(apperror.WrapError(err))
		return
	}

	// Never return auth data to client
	srv.AuthData = ""
	httpx.Success(c, srv)
}

func (h *DeployHandler) UpdateServer(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.Error(apperror.ErrBadRequest.WithMessage("invalid server id"))
		return
	}

	var srv deploy.Server
	if err := c.ShouldBindJSON(&srv); err != nil {
		c.Error(apperror.ErrBadRequest.Wrap(err))
		return
	}
	srv.ID = id

	middleware.AuditSummary(c, "更新部署服务器 "+c.Param("id"))
	if err := h.deployService.UpdateServer(c.Request.Context(), &srv); err != nil {
		c.Error(apperror.WrapError(err))
		return
	}

	// Never return auth data to client
	srv.AuthData = ""
	httpx.Success(c, srv)
}

func (h *DeployHandler) DeleteServer(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.Error(apperror.ErrBadRequest.WithMessage("invalid server id"))
		return
	}

	middleware.AuditSummary(c, "删除部署服务器 "+c.Param("id"))
	if err := h.deployService.DeleteServer(c.Request.Context(), id); err != nil {
		// Sub-resource conflict returns 409
		if strings.Contains(err.Error(), "tasks") || strings.Contains(err.Error(), "versions") {
			c.Error(apperror.ErrConflict.Wrap(err))
			return
		}
		c.Error(apperror.WrapError(err))
		return
	}

	httpx.Success(c, nil)
}

func (h *DeployHandler) TestConnection(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.Error(apperror.ErrBadRequest.WithMessage("invalid server id"))
		return
	}

	middleware.AuditSummary(c, "测试部署服务器连接 "+c.Param("id"))
	if err := h.deployService.TestConnection(c.Request.Context(), id); err != nil {
		c.Error(apperror.WrapError(err))
		return
	}

	httpx.Success(c, gin.H{"status": "online"})
}

// Task endpoints

func (h *DeployHandler) ListTasks(c *gin.Context) {
	tasks, err := h.deployService.ListTasks(c.Request.Context())
	if err != nil {
		c.Error(apperror.WrapError(err))
		return
	}
	httpx.Success(c, tasks)
}

func (h *DeployHandler) GetTask(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.Error(apperror.ErrBadRequest.WithMessage("invalid task id"))
		return
	}

	task, err := h.deployService.GetTask(c.Request.Context(), id)
	if err != nil {
		c.Error(apperror.ErrNotFound.Wrap(err))
		return
	}

	httpx.Success(c, task)
}

func (h *DeployHandler) CreateTask(c *gin.Context) {
	var task deploy.Task
	if err := c.ShouldBindJSON(&task); err != nil {
		c.Error(apperror.ErrBadRequest.Wrap(err))
		return
	}

	// Input validation
	if task.Name == "" {
		c.Error(apperror.ErrBadRequest.WithMessage("task name is required"))
		return
	}
	if task.Type != "sync" && task.Type != "command" && task.Type != "rollback" {
		c.Error(apperror.ErrBadRequest.WithMessage("task type must be 'sync', 'command', or 'rollback'"))
		return
	}
	if task.ServerID <= 0 {
		c.Error(apperror.ErrBadRequest.WithMessage("server_id is required"))
		return
	}

	middleware.AuditSummary(c, "创建部署任务 "+task.Name)
	if err := h.deployService.CreateTask(c.Request.Context(), &task); err != nil {
		if strings.Contains(err.Error(), "does not exist") {
			c.Error(apperror.ErrNotFound.Wrap(err))
			return
		}
		c.Error(apperror.WrapError(err))
		return
	}

	httpx.Success(c, task)
}

func (h *DeployHandler) DeleteTask(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.Error(apperror.ErrBadRequest.WithMessage("invalid task id"))
		return
	}

	middleware.AuditSummary(c, "删除部署任务 "+c.Param("id"))
	if err := h.deployService.DeleteTask(c.Request.Context(), id); err != nil {
		c.Error(apperror.WrapError(err))
		return
	}

	httpx.Success(c, nil)
}

func (h *DeployHandler) ExecuteTask(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.Error(apperror.ErrBadRequest.WithMessage("invalid task id"))
		return
	}

	middleware.AuditSummary(c, "执行部署任务 "+c.Param("id"))
	if err := h.deployService.ExecuteTask(c.Request.Context(), id); err != nil {
		if strings.Contains(err.Error(), "already running") {
			c.Error(apperror.ErrConflict.Wrap(err))
			return
		}
		c.Error(apperror.WrapError(err))
		return
	}

	httpx.Success(c, gin.H{"status": "executing"})
}

// Version endpoints

func (h *DeployHandler) ListVersions(c *gin.Context) {
	serverID, err := strconv.ParseInt(c.Query("server_id"), 10, 64)
	if err != nil {
		c.Error(apperror.ErrBadRequest.WithMessage("server_id is required"))
		return
	}

	versions, err := h.deployService.ListVersions(c.Request.Context(), serverID)
	if err != nil {
		c.Error(apperror.WrapError(err))
		return
	}

	httpx.Success(c, versions)
}

func (h *DeployHandler) RollbackVersion(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.Error(apperror.ErrBadRequest.WithMessage("invalid version id"))
		return
	}

	middleware.AuditSummary(c, "回滚部署版本 "+c.Param("id"))
	if err := h.deployService.RollbackVersion(c.Request.Context(), id); err != nil {
		c.Error(apperror.WrapError(err))
		return
	}

	httpx.Success(c, gin.H{"status": "rolling_back"})
}

func RegisterRoutes(protected *gin.RouterGroup, deployService *deploy.Service) {
	handler := NewDeployHandler(deployService)
	protected.GET("/deploy/servers", handler.ListServers)
	protected.POST("/deploy/servers", handler.CreateServer)
	protected.GET("/deploy/servers/:id", handler.GetServer)
	protected.PUT("/deploy/servers/:id", handler.UpdateServer)
	protected.DELETE("/deploy/servers/:id", handler.DeleteServer)
	protected.POST("/deploy/servers/:id/test", handler.TestConnection)
	protected.GET("/deploy/tasks", handler.ListTasks)
	protected.POST("/deploy/tasks", handler.CreateTask)
	protected.GET("/deploy/tasks/:id", handler.GetTask)
	protected.DELETE("/deploy/tasks/:id", handler.DeleteTask)
	protected.POST("/deploy/tasks/:id/exec", handler.ExecuteTask)
	protected.GET("/deploy/versions", handler.ListVersions)
	protected.POST("/deploy/versions/:id/rollback", handler.RollbackVersion)
}

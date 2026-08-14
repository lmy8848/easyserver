package http

import (
	"strconv"
	"strings"

	"easyserver/internal/deploy"
	"easyserver/internal/httpx"
	"easyserver/internal/httpx/middleware"
	"easyserver/internal/infra/errx"

	"github.com/gin-gonic/gin"
)

type DeployHandler struct {
	deployService *deploy.Service
}

func NewDeployHandler(db *deploy.Service) *DeployHandler {
	return &DeployHandler{deployService: db}
}

// Server endpoints

func (h *DeployHandler) ListServers(c *gin.Context) (any, error) {
	servers, err := h.deployService.ListServers(c.Request.Context())
	if err != nil {
		return nil, err
	}
	return servers, nil
}

func (h *DeployHandler) GetServer(c *gin.Context) (any, error) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return nil, errx.BadRequest("invalid server id")
	}

	srv, err := h.deployService.GetServer(c.Request.Context(), id)
	if err != nil {
		return nil, err
	}

	return srv, nil
}

func (h *DeployHandler) CreateServer(c *gin.Context) (any, error) {
	var srv deploy.Server
	if err := c.ShouldBindJSON(&srv); err != nil {
		return nil, errx.BadRequest("invalid request: %w", err)
	}

	// Input validation
	if srv.Name == "" {
		return nil, errx.BadRequest("server name is required")
	}
	if srv.Host == "" {
		return nil, errx.BadRequest("host is required")
	}
	if srv.Port < 1 || srv.Port > 65535 {
		return nil, errx.BadRequest("port must be between 1 and 65535")
	}
	if srv.Username == "" {
		return nil, errx.BadRequest("username is required")
	}
	if srv.AuthType != "password" && srv.AuthType != "key" {
		return nil, errx.BadRequest("auth_type must be 'password' or 'key'")
	}
	if srv.AuthData == "" {
		return nil, errx.BadRequest("auth_data is required")
	}

	middleware.AuditSummary(c, "创建部署服务器 "+srv.Name)
	if err := h.deployService.CreateServer(c.Request.Context(), &srv); err != nil {
		return nil, err
	}

	// Never return auth data to client
	srv.AuthData = ""
	return srv, nil
}

func (h *DeployHandler) UpdateServer(c *gin.Context) (any, error) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return nil, errx.BadRequest("invalid server id")
	}

	var srv deploy.Server
	if err := c.ShouldBindJSON(&srv); err != nil {
		return nil, errx.BadRequest("invalid request: %w", err)
	}
	srv.ID = id

	middleware.AuditSummary(c, "更新部署服务器 "+c.Param("id"))
	if err := h.deployService.UpdateServer(c.Request.Context(), &srv); err != nil {
		return nil, err
	}

	// Never return auth data to client
	srv.AuthData = ""
	return srv, nil
}

func (h *DeployHandler) DeleteServer(c *gin.Context) (any, error) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return nil, errx.BadRequest("invalid server id")
	}

	middleware.AuditSummary(c, "删除部署服务器 "+c.Param("id"))
	if err := h.deployService.DeleteServer(c.Request.Context(), id); err != nil {
		// Sub-resource conflict returns 409
		if strings.Contains(err.Error(), "tasks") || strings.Contains(err.Error(), "versions") {
			return nil, errx.Conflict("%w", err)
		}
		return nil, err
	}

	return nil, nil
}

func (h *DeployHandler) TestConnection(c *gin.Context) (any, error) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return nil, errx.BadRequest("invalid server id")
	}

	middleware.AuditSummary(c, "测试部署服务器连接 "+c.Param("id"))
	if err := h.deployService.TestConnection(c.Request.Context(), id); err != nil {
		return nil, err
	}

	return gin.H{"status": "online"}, nil
}

// Task endpoints

func (h *DeployHandler) ListTasks(c *gin.Context) (any, error) {
	tasks, err := h.deployService.ListTasks(c.Request.Context())
	if err != nil {
		return nil, err
	}
	return tasks, nil
}

func (h *DeployHandler) GetTask(c *gin.Context) (any, error) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return nil, errx.BadRequest("invalid task id")
	}

	task, err := h.deployService.GetTask(c.Request.Context(), id)
	if err != nil {
		return nil, err
	}

	return task, nil
}

func (h *DeployHandler) CreateTask(c *gin.Context) (any, error) {
	var task deploy.Task
	if err := c.ShouldBindJSON(&task); err != nil {
		return nil, errx.BadRequest("invalid request: %w", err)
	}

	// Input validation
	if task.Name == "" {
		return nil, errx.BadRequest("task name is required")
	}
	if task.Type != "sync" && task.Type != "command" && task.Type != "rollback" {
		return nil, errx.BadRequest("task type must be 'sync', 'command', or 'rollback'")
	}
	if task.ServerID <= 0 {
		return nil, errx.BadRequest("server_id is required")
	}

	middleware.AuditSummary(c, "创建部署任务 "+task.Name)
	if err := h.deployService.CreateTask(c.Request.Context(), &task); err != nil {
		if strings.Contains(err.Error(), "does not exist") {
			return nil, errx.NotFound("%w", err)
		}
		return nil, err
	}

	return task, nil
}

func (h *DeployHandler) DeleteTask(c *gin.Context) (any, error) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return nil, errx.BadRequest("invalid task id")
	}

	middleware.AuditSummary(c, "删除部署任务 "+c.Param("id"))
	if err := h.deployService.DeleteTask(c.Request.Context(), id); err != nil {
		return nil, err
	}

	return nil, nil
}

func (h *DeployHandler) ExecuteTask(c *gin.Context) (any, error) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return nil, errx.BadRequest("invalid task id")
	}

	middleware.AuditSummary(c, "执行部署任务 "+c.Param("id"))
	if err := h.deployService.ExecuteTask(id); err != nil {
		if strings.Contains(err.Error(), "already running") {
			return nil, errx.Conflict("%w", err)
		}
		return nil, err
	}

	return gin.H{"status": "executing"}, nil
}

// Version endpoints

func (h *DeployHandler) ListVersions(c *gin.Context) (any, error) {
	serverID, err := strconv.ParseInt(c.Query("server_id"), 10, 64)
	if err != nil {
		return nil, errx.BadRequest("server_id is required")
	}

	versions, err := h.deployService.ListVersions(c.Request.Context(), serverID)
	if err != nil {
		return nil, err
	}

	return versions, nil
}

func (h *DeployHandler) RollbackVersion(c *gin.Context) (any, error) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return nil, errx.BadRequest("invalid version id")
	}

	middleware.AuditSummary(c, "回滚部署版本 "+c.Param("id"))
	if err := h.deployService.RollbackVersion(c.Request.Context(), id); err != nil {
		return nil, err
	}

	return gin.H{"status": "rolling_back"}, nil
}

func RegisterRoutes(protected *gin.RouterGroup, deployService *deploy.Service) {
	handler := NewDeployHandler(deployService)
	protected.GET("/deploy/servers", httpx.H(handler.ListServers))
	protected.POST("/deploy/servers", httpx.H(handler.CreateServer))
	protected.GET("/deploy/servers/:id", httpx.H(handler.GetServer))
	protected.PUT("/deploy/servers/:id", httpx.H(handler.UpdateServer))
	protected.DELETE("/deploy/servers/:id", httpx.H(handler.DeleteServer))
	protected.POST("/deploy/servers/:id/test", httpx.H(handler.TestConnection))
	protected.GET("/deploy/tasks", httpx.H(handler.ListTasks))
	protected.POST("/deploy/tasks", httpx.H(handler.CreateTask))
	protected.GET("/deploy/tasks/:id", httpx.H(handler.GetTask))
	protected.DELETE("/deploy/tasks/:id", httpx.H(handler.DeleteTask))
	protected.POST("/deploy/tasks/:id/exec", httpx.H(handler.ExecuteTask))
	protected.GET("/deploy/versions", httpx.H(handler.ListVersions))
	protected.POST("/deploy/versions/:id/rollback", httpx.H(handler.RollbackVersion))
}

package api

import (
	"strconv"
	"strings"

	"easyserver/internal/service"

	"github.com/gin-gonic/gin"
)

type DeployHandler struct {
	deployService *service.DeployService
}

func NewDeployHandler(db *service.DeployService) *DeployHandler {
	return &DeployHandler{deployService: db}
}

// Server endpoints

func (h *DeployHandler) ListServers(c *gin.Context) {
	servers, err := h.deployService.ListServers(c.Request.Context())
	if err != nil {
		InternalError(c, err.Error())
		return
	}
	Success(c, servers)
}

func (h *DeployHandler) GetServer(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		BadRequest(c, "invalid server id")
		return
	}

	srv, err := h.deployService.GetServer(c.Request.Context(), id)
	if err != nil {
		NotFound(c, err.Error())
		return
	}

	Success(c, srv)
}

func (h *DeployHandler) CreateServer(c *gin.Context) {
	var srv service.DeployServer
	if err := c.ShouldBindJSON(&srv); err != nil {
		BadRequest(c, err.Error())
		return
	}

	// Input validation
	if srv.Name == "" {
		BadRequest(c, "server name is required")
		return
	}
	if srv.Host == "" {
		BadRequest(c, "host is required")
		return
	}
	if srv.Port < 1 || srv.Port > 65535 {
		BadRequest(c, "port must be between 1 and 65535")
		return
	}
	if srv.Username == "" {
		BadRequest(c, "username is required")
		return
	}
	if srv.AuthType != "password" && srv.AuthType != "key" {
		BadRequest(c, "auth_type must be 'password' or 'key'")
		return
	}
	if srv.AuthData == "" {
		BadRequest(c, "auth_data is required")
		return
	}

	if err := h.deployService.CreateServer(c.Request.Context(), &srv); err != nil {
		InternalError(c, err.Error())
		return
	}

	// Never return auth data to client
	srv.AuthData = ""
	Success(c, srv)
}

func (h *DeployHandler) UpdateServer(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		BadRequest(c, "invalid server id")
		return
	}

	var srv service.DeployServer
	if err := c.ShouldBindJSON(&srv); err != nil {
		BadRequest(c, err.Error())
		return
	}
	srv.ID = id

	if err := h.deployService.UpdateServer(c.Request.Context(), &srv); err != nil {
		InternalError(c, err.Error())
		return
	}

	// Never return auth data to client
	srv.AuthData = ""
	Success(c, srv)
}

func (h *DeployHandler) DeleteServer(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		BadRequest(c, "invalid server id")
		return
	}

	if err := h.deployService.DeleteServer(c.Request.Context(), id); err != nil {
		// Sub-resource conflict returns 409
		if strings.Contains(err.Error(), "tasks") || strings.Contains(err.Error(), "versions") {
			Conflict(c, err.Error())
			return
		}
		InternalError(c, err.Error())
		return
	}

	Success(c, nil)
}

func (h *DeployHandler) TestConnection(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		BadRequest(c, "invalid server id")
		return
	}

	if err := h.deployService.TestConnection(c.Request.Context(), id); err != nil {
		InternalError(c, err.Error())
		return
	}

	Success(c, gin.H{"status": "online"})
}

// Task endpoints

func (h *DeployHandler) ListTasks(c *gin.Context) {
	tasks, err := h.deployService.ListTasks(c.Request.Context())
	if err != nil {
		InternalError(c, err.Error())
		return
	}
	Success(c, tasks)
}

func (h *DeployHandler) GetTask(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		BadRequest(c, "invalid task id")
		return
	}

	task, err := h.deployService.GetTask(c.Request.Context(), id)
	if err != nil {
		NotFound(c, err.Error())
		return
	}

	Success(c, task)
}

func (h *DeployHandler) CreateTask(c *gin.Context) {
	var task service.DeployTask
	if err := c.ShouldBindJSON(&task); err != nil {
		BadRequest(c, err.Error())
		return
	}

	// Input validation
	if task.Name == "" {
		BadRequest(c, "task name is required")
		return
	}
	if task.Type != "sync" && task.Type != "command" && task.Type != "rollback" {
		BadRequest(c, "task type must be 'sync', 'command', or 'rollback'")
		return
	}
	if task.ServerID <= 0 {
		BadRequest(c, "server_id is required")
		return
	}

	if err := h.deployService.CreateTask(c.Request.Context(), &task); err != nil {
		if strings.Contains(err.Error(), "does not exist") {
			NotFound(c, err.Error())
			return
		}
		InternalError(c, err.Error())
		return
	}

	Success(c, task)
}

func (h *DeployHandler) DeleteTask(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		BadRequest(c, "invalid task id")
		return
	}

	if err := h.deployService.DeleteTask(c.Request.Context(), id); err != nil {
		InternalError(c, err.Error())
		return
	}

	Success(c, nil)
}

func (h *DeployHandler) ExecuteTask(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		BadRequest(c, "invalid task id")
		return
	}

	if err := h.deployService.ExecuteTask(c.Request.Context(), id); err != nil {
		if strings.Contains(err.Error(), "already running") {
			Conflict(c, err.Error())
			return
		}
		InternalError(c, err.Error())
		return
	}

	Success(c, gin.H{"status": "executing"})
}

// Version endpoints

func (h *DeployHandler) ListVersions(c *gin.Context) {
	serverID, err := strconv.ParseInt(c.Query("server_id"), 10, 64)
	if err != nil {
		BadRequest(c, "server_id is required")
		return
	}

	versions, err := h.deployService.ListVersions(c.Request.Context(), serverID)
	if err != nil {
		InternalError(c, err.Error())
		return
	}

	Success(c, versions)
}

func (h *DeployHandler) RollbackVersion(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		BadRequest(c, "invalid version id")
		return
	}

	if err := h.deployService.RollbackVersion(c.Request.Context(), id); err != nil {
		InternalError(c, err.Error())
		return
	}

	Success(c, gin.H{"status": "rolling_back"})
}

package api

import (
	"strconv"

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
	servers, err := h.deployService.ListServers()
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

	srv, err := h.deployService.GetServer(id)
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

	if err := h.deployService.CreateServer(&srv); err != nil {
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

	if err := h.deployService.UpdateServer(&srv); err != nil {
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

	if err := h.deployService.DeleteServer(id); err != nil {
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

	if err := h.deployService.TestConnection(id); err != nil {
		InternalError(c, err.Error())
		return
	}

	Success(c, gin.H{"status": "online"})
}

// Task endpoints

func (h *DeployHandler) ListTasks(c *gin.Context) {
	tasks, err := h.deployService.ListTasks()
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

	task, err := h.deployService.GetTask(id)
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

	if err := h.deployService.CreateTask(&task); err != nil {
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

	if err := h.deployService.DeleteTask(id); err != nil {
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

	if err := h.deployService.ExecuteTask(id); err != nil {
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

	versions, err := h.deployService.ListVersions(serverID)
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

	if err := h.deployService.RollbackVersion(id); err != nil {
		InternalError(c, err.Error())
		return
	}

	Success(c, gin.H{"status": "rolling_back"})
}

package api

import (
	"database/sql"
	"log"

	"easyserver/internal/service"

	"github.com/gin-gonic/gin"
)

// registerDeployRoutes registers deploy management routes
func registerDeployRoutes(protected *gin.RouterGroup, db *sql.DB, encryptionKey string) {
	deployService, err := service.NewDeployService(db, encryptionKey)
	if err != nil {
		log.Fatalf("FATAL: failed to init deploy service: %v", err)
	}
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

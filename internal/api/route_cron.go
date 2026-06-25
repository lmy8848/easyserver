package api

import (
	"context"
	"database/sql"
	"log"

	"easyserver/internal/executor"
	"easyserver/internal/service"

	"github.com/gin-gonic/gin"
)

// registerCronRoutes registers cron task management routes
func registerCronRoutes(protected *gin.RouterGroup, db *sql.DB) {
	cronService := service.NewCronService(db, executor.NewOSExecutor())
	// Seed default documentation (tables managed by migration system)
	if err := cronService.SeedDefaultDocs(context.Background()); err != nil {
		log.Printf("WARNING: seed default cron docs failed: %v", err)
	}
	handler := NewCronHandler(cronService)

	protected.GET("/cron/presets", handler.GetPresets)
	protected.GET("/cron/describe", handler.DescribeSchedule)
	protected.GET("/cron/next-runs", handler.GetNextRuns)
	protected.GET("/cron/tasks", handler.ListTasks)
	protected.POST("/cron/tasks", handler.CreateTask)
	protected.GET("/cron/tasks/:id", handler.GetTask)
	protected.PUT("/cron/tasks/:id", handler.UpdateTask)
	protected.DELETE("/cron/tasks/:id", handler.DeleteTask)
	protected.POST("/cron/tasks/:id/enable", handler.EnableTask)
	protected.POST("/cron/tasks/:id/disable", handler.DisableTask)
	protected.POST("/cron/tasks/:id/run", handler.RunTask)
	protected.GET("/cron/tasks/:id/logs", handler.GetTaskLogs)
	protected.GET("/cron/scripts", handler.ListScripts)
	protected.POST("/cron/scripts", handler.CreateScript)
	protected.GET("/cron/scripts/:id", handler.GetScript)
	protected.PUT("/cron/scripts/:id", handler.UpdateScript)
	protected.DELETE("/cron/scripts/:id", handler.DeleteScript)
	protected.GET("/cron/docs", handler.ListDocs)
	protected.POST("/cron/docs", handler.CreateDoc)
	protected.GET("/cron/docs/:id", handler.GetDoc)
	protected.PUT("/cron/docs/:id", handler.UpdateDoc)
	protected.DELETE("/cron/docs/:id", handler.DeleteDoc)
}

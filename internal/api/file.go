package api

import (
	"easyserver/internal/audit"
	"easyserver/internal/filemanager"

	"github.com/gin-gonic/gin"
)

// registerFileRoutes registers file management routes
func registerFileRoutes(protected *gin.RouterGroup, fileManager *filemanager.Manager, auditService *audit.Service) {
	handler := NewFileManagerHandler(fileManager, auditService)
	protected.GET("/files", handler.List)
	protected.GET("/files/base-path", handler.GetBasePath)
	protected.GET("/files/download", handler.Download)
	protected.GET("/files/content", handler.GetContent)
	protected.GET("/files/search", handler.Search)
	protected.GET("/files/search-content", handler.SearchContent)
	protected.GET("/files/details", handler.GetDetails)
	protected.GET("/files/mime-type", handler.GetMimeType)
	protected.POST("/files/mkdir", handler.Mkdir)
	protected.POST("/files/upload", handler.Upload)
	protected.PUT("/files/rename", handler.Rename)
	protected.DELETE("/files", handler.Delete)
	protected.POST("/files/move", handler.Move)
	protected.POST("/files/copy", handler.Copy)
	protected.PUT("/files/content", handler.SaveContent)
	protected.POST("/files/compress", handler.Compress)
	protected.POST("/files/extract", handler.Extract)
	protected.PUT("/files/chmod", handler.Chmod)
	protected.PUT("/files/chown", handler.Chown)
}

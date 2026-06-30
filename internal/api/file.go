package api

import (
	"easyserver/internal/api/middleware"
	"easyserver/internal/audit"
	"easyserver/internal/filemanager"

	"github.com/gin-gonic/gin"
)

// registerFileRoutes registers file management routes
func registerFileRoutes(protected *gin.RouterGroup, fileManager *filemanager.Manager, auditService *audit.Service) {
	handler := NewFileManagerHandler(fileManager, auditService)
	protected.GET("/files", handler.List)
	protected.GET("/files/download", handler.Download)
	protected.GET("/files/content", handler.GetContent)
	protected.GET("/files/search", handler.Search)
	protected.GET("/files/search-content", handler.SearchContent)
	protected.GET("/files/details", handler.GetDetails)
	protected.GET("/files/mime-type", handler.GetMimeType)
	protected.POST("/files/mkdir", middleware.SetAction("FILE_MKDIR"), handler.Mkdir)
	protected.POST("/files/upload", middleware.SetAction("FILE_UPLOAD"), handler.Upload)
	protected.PUT("/files/rename", middleware.SetAction("FILE_RENAME"), handler.Rename)
	protected.DELETE("/files", middleware.SetAction("FILE_DELETE"), handler.Delete)
	protected.POST("/files/move", middleware.SetAction("FILE_MOVE"), handler.Move)
	protected.POST("/files/copy", middleware.SetAction("FILE_COPY"), handler.Copy)
	protected.PUT("/files/content", middleware.SetAction("FILE_SAVE_CONTENT"), handler.SaveContent)
	protected.POST("/files/compress", middleware.SetAction("FILE_COMPRESS"), handler.Compress)
	protected.POST("/files/extract", middleware.SetAction("FILE_EXTRACT"), handler.Extract)
	protected.PUT("/files/chmod", middleware.SetAction("FILE_CHMOD"), handler.Chmod)
	protected.PUT("/files/chown", middleware.SetAction("FILE_CHOWN"), handler.Chown)
}

package api

import "github.com/gin-gonic/gin"

// registerTemplateRoutes registers template management routes
func registerTemplateRoutes(protected *gin.RouterGroup) {
	handler := NewTemplateHandler("templates/docker-images.json")
	protected.GET("/templates/docker-images", handler.GetDockerImages)
	protected.GET("/templates/categories", handler.GetTemplateCategories)
}

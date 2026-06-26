package api

import (
	"encoding/json"
		"os"

	"github.com/gin-gonic/gin"
)

// TemplateHandler handles template management
type TemplateHandler struct {
	templatePath       string
	scriptTemplatesPath string
}

// NewTemplateHandler creates a new TemplateHandler
func NewTemplateHandler(templatePath string, scriptTemplatesPath string) *TemplateHandler {
	return &TemplateHandler{
		templatePath:       templatePath,
		scriptTemplatesPath: scriptTemplatesPath,
	}
}

// GetDockerImages returns the Docker image templates
func (h *TemplateHandler) GetDockerImages(c *gin.Context) {
	data, err := os.ReadFile(h.templatePath)
	if err != nil {
		// Return default templates if file not found
		Success(c, getDefaultTemplates())
		return
	}

	var templates map[string]interface{}
	if err := json.Unmarshal(data, &templates); err != nil {
		Success(c, getDefaultTemplates())
		return
	}

	Success(c, templates)
}

// getDefaultTemplates returns hardcoded default templates
func getDefaultTemplates() map[string]interface{} {
	return map[string]interface{}{
		"version": 1,
		"categories": []map[string]interface{}{
			{
				"name": "Web 服务器",
				"images": []map[string]interface{}{
					{"name": "Nginx", "image": "nginx:latest", "description": "高性能 HTTP 服务器", "ports": []string{"80:80", "443:443"}},
					{"name": "Apache", "image": "httpd:latest", "description": "Apache HTTP 服务器", "ports": []string{"80:80"}},
				},
			},
			{
				"name": "数据库",
				"images": []map[string]interface{}{
					{"name": "MySQL 8", "image": "mysql:8", "description": "关系型数据库", "ports": []string{"3306:3306"}},
					{"name": "Redis 7", "image": "redis:7-alpine", "description": "内存数据库", "ports": []string{"6379:6379"}},
				},
			},
		},
	}
}

// GetTemplateCategories returns available template categories
func (h *TemplateHandler) GetTemplateCategories(c *gin.Context) {
	data, err := os.ReadFile(h.templatePath)
	if err != nil {
		Success(c, []string{})
		return
	}

	var templates map[string]interface{}
	if err := json.Unmarshal(data, &templates); err != nil {
		Success(c, []string{})
		return
	}

	categories, ok := templates["categories"].([]interface{})
	if !ok {
		Success(c, []string{})
		return
	}

	var names []string
	for _, cat := range categories {
		if m, ok := cat.(map[string]interface{}); ok {
			if name, ok := m["name"].(string); ok {
				names = append(names, name)
			}
		}
	}

	Success(c, names)
}

// GetScriptTemplates returns script templates
func (h *TemplateHandler) GetScriptTemplates(c *gin.Context) {
	data, err := os.ReadFile(h.scriptTemplatesPath)
	if err != nil {
		Success(c, gin.H{"categories": []interface{}{}})
		return
	}

	var templates map[string]interface{}
	if err := json.Unmarshal(data, &templates); err != nil {
		Success(c, gin.H{"categories": []interface{}{}})
		return
	}

	Success(c, templates)
}

func registerTemplateRoutes(protected *gin.RouterGroup) {
	handler := NewTemplateHandler("templates/docker-images.json", "templates/script-templates.json")
	protected.GET("/templates/docker-images", handler.GetDockerImages)
	protected.GET("/templates/categories", handler.GetTemplateCategories)
	protected.GET("/templates/scripts", handler.GetScriptTemplates)
}

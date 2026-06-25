package api

import (
	"encoding/json"
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
)

// TemplateHandler handles template management
type TemplateHandler struct {
	templatePath string
}

// NewTemplateHandler creates a new TemplateHandler
func NewTemplateHandler(templatePath string) *TemplateHandler {
	return &TemplateHandler{templatePath: templatePath}
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
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": []string{}})
		return
	}

	var templates map[string]interface{}
	if err := json.Unmarshal(data, &templates); err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": []string{}})
		return
	}

	categories, ok := templates["categories"].([]interface{})
	if !ok {
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": []string{}})
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

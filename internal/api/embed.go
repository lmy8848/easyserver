//go:build !dev
// +build !dev

package api

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

//go:embed all:web/dist
var webDist embed.FS

// ServeWeb serves the embedded frontend files
func ServeWeb(e *gin.Engine) {
	// Get the subdirectory
	webFS, err := fs.Sub(webDist, "web/dist")
	if err != nil {
		// If embed fails, serve empty page
		e.GET("/", func(c *gin.Context) {
			c.String(http.StatusOK, "EasyServer - Frontend not embedded")
		})
		return
	}

	// Serve static files
	e.GET("/", func(c *gin.Context) {
		c.FileFromFS("/", http.FS(webFS))
	})

	e.NoRoute(func(c *gin.Context) {
		path := c.Request.URL.Path

		// Try to serve the file directly
		if !strings.HasPrefix(path, "/api/") && !strings.HasPrefix(path, "/ws/") {
			// Check if file exists
			_, err := webFS.Open(strings.TrimPrefix(path, "/"))
			if err == nil {
				c.FileFromFS(path, http.FS(webFS))
				return
			}

			// For SPA routing, serve index.html
			c.FileFromFS("/", http.FS(webFS))
			return
		}

		c.JSON(http.StatusNotFound, gin.H{
			"code":    40400,
			"message": "页面未找到",
		})
	})
}

//go:build dev
// +build dev

package api

import (
	"github.com/gin-gonic/gin"
)

// ServeWeb is a no-op in dev mode
// Frontend is served by Vite dev server on port 5173
func ServeWeb(e *gin.Engine) {
	// In dev mode, frontend is served separately
}

//go:build !dev
// +build !dev

package api

import (
	"crypto/rand"
	"embed"
	"encoding/base64"
	"io/fs"
	"log"
	"net/http"
	"regexp"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
)

//go:embed all:web/dist
var webDist embed.FS

var (
	cspNonce        string
	cachedIndexHTML []byte
	initOnce        sync.Once
)

// scriptTagRegex matches <script tags to inject nonce attribute
var scriptTagRegex = regexp.MustCompile(`<script(>|\s)`)

// InitCSPNonce reads the embedded index.html, generates a static nonce,
// injects it into all <script> tags, and caches the result.
// Returns the nonce string for use in CSP headers.
func InitCSPNonce() string {
	initOnce.Do(func() {
		webFS, err := fs.Sub(webDist, "web/dist")
		if err != nil {
			log.Printf("embed: failed to get web/dist sub filesystem: %v", err)
			return
		}

		data, err := fs.ReadFile(webFS, "index.html")
		if err != nil {
			log.Printf("embed: failed to read index.html: %v", err)
			return
		}

		// Generate a static nonce (valid for the lifetime of this process)
		cspNonce = generateNonce()

		// Inject nonce into all <script> tags
		cachedIndexHTML = scriptTagRegex.ReplaceAllFunc(data, func(match []byte) []byte {
			if string(match) == "<script>" {
				return []byte(`<script nonce="` + cspNonce + `">`)
			}
			return []byte(`<script nonce="` + cspNonce + `" `)
		})
	})
	return cspNonce
}

// CSPNonce returns the pre-generated CSP nonce. Must call InitCSPNonce first.
func CSPNonce() string {
	return cspNonce
}

// generateNonce generates a random nonce for CSP
func generateNonce() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return base64.StdEncoding.EncodeToString(b)
}

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

	// Serve the cached index.html (with nonce injected) for root
	if len(cachedIndexHTML) > 0 {
		e.GET("/", func(c *gin.Context) {
			c.Data(http.StatusOK, "text/html; charset=utf-8", cachedIndexHTML)
		})
	} else {
		e.GET("/", func(c *gin.Context) {
			c.FileFromFS("/", http.FS(webFS))
		})
	}

	// Serve static assets (JS, CSS, images, etc.)
	e.GET("/assets/*filepath", func(c *gin.Context) {
		c.FileFromFS("/assets/"+c.Param("filepath"), http.FS(webFS))
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

			// For SPA routing, serve the cached index.html with nonce
			if len(cachedIndexHTML) > 0 {
				c.Data(http.StatusOK, "text/html; charset=utf-8", cachedIndexHTML)
			} else {
				c.FileFromFS("/", http.FS(webFS))
			}
			return
		}

		c.Error(ErrNotFound.WithMessage("页面未找到"))
	})
}

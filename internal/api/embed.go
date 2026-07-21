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

	"easyserver/internal/infra/apperror"

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

	// htmlNoCache sets Cache-Control to prevent CDN/browser caching of HTML.
	htmlNoCache := func(c *gin.Context) {
		c.Header("Cache-Control", "no-cache, no-store, must-revalidate")
		c.Header("Pragma", "no-cache")
		c.Header("Expires", "0")
	}

	// Serve the cached index.html (with nonce injected) for root
	if len(cachedIndexHTML) > 0 {
		e.GET("/", func(c *gin.Context) {
			htmlNoCache(c)
			c.Data(http.StatusOK, "text/html; charset=utf-8", cachedIndexHTML)
		})
	} else {
		e.GET("/", func(c *gin.Context) {
			htmlNoCache(c)
			c.FileFromFS("/", http.FS(webFS))
		})
	}

	// Serve static assets (JS/CSS with hash) - cache for 1 year
	e.GET("/assets/*filepath", func(c *gin.Context) {
		c.Header("Cache-Control", "public, max-age=31536000, immutable")
		c.FileFromFS("/assets/"+c.Param("filepath"), http.FS(webFS))
	})

	// favicon: no-cache so icon changes are visible immediately (browser & CDN).
	faviconHandler := func(c *gin.Context) {
		c.Header("Cache-Control", "no-cache, must-revalidate")
		c.FileFromFS("/favicon.svg", http.FS(webFS))
	}
	e.GET("/favicon.svg", faviconHandler)
	e.HEAD("/favicon.svg", faviconHandler)

	e.NoRoute(func(c *gin.Context) {
		path := c.Request.URL.Path

		// Try to serve the file directly
		if !strings.HasPrefix(path, "/api/") && !strings.HasPrefix(path, "/ws/") {
			// Check if file exists
			_, err := webFS.Open(strings.TrimPrefix(path, "/"))
			if err == nil {
				// Non-HTML assets: cache; HTML: no cache
				if strings.HasSuffix(path, ".html") {
					htmlNoCache(c)
				}
				c.FileFromFS(path, http.FS(webFS))
				return
			}

			// For SPA routing, serve the cached index.html with nonce
			htmlNoCache(c)
			if len(cachedIndexHTML) > 0 {
				c.Data(http.StatusOK, "text/html; charset=utf-8", cachedIndexHTML)
			} else {
				c.FileFromFS("/", http.FS(webFS))
			}
			return
		}

		c.Error(apperror.ErrNotFound.WithMessage("页面未找到"))
	})
}

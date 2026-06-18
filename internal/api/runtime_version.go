package api

import (
	"easyserver/internal/service"

	"github.com/gin-gonic/gin"
)

type RuntimeVersionHandler struct {
	versionService *service.RuntimeVersionService
}

func NewRuntimeVersionHandler(versionService *service.RuntimeVersionService) *RuntimeVersionHandler {
	return &RuntimeVersionHandler{versionService: versionService}
}

// List returns cached versions for a runtime with installed status
func (h *RuntimeVersionHandler) List(c *gin.Context) {
	name := c.Param("name")
	if name == "" {
		BadRequest(c, "runtime name is required")
		return
	}

	versions, err := h.versionService.ListWithInstalledStatus(name)
	if err != nil {
		InternalError(c, err.Error())
		return
	}

	Success(c, gin.H{
		"versions": versions,
	})
}

// Fetch fetches versions from external sources and caches them
func (h *RuntimeVersionHandler) Fetch(c *gin.Context) {
	name := c.Param("name")
	if name == "" {
		BadRequest(c, "runtime name is required")
		return
	}

	// Validate runtime name
	validRuntimes := map[string]bool{
		"java": true, "node": true, "go": true, "python": true, "php": true,
	}
	if !validRuntimes[name] {
		BadRequest(c, "unsupported runtime: "+name)
		return
	}

	cached, err := h.versionService.FetchAndCache(name)
	if err != nil {
		InternalError(c, err.Error())
		return
	}

	// Return updated list with installed status
	versions, err := h.versionService.ListWithInstalledStatus(name)
	if err != nil {
		InternalError(c, err.Error())
		return
	}

	Success(c, gin.H{
		"message":  "versions fetched successfully",
		"cached":   cached,
		"versions": versions,
	})
}

// ResolveAlias resolves a version alias to an actual version
func (h *RuntimeVersionHandler) ResolveAlias(c *gin.Context) {
	name := c.Param("name")
	alias := c.Param("alias")

	if name == "" || alias == "" {
		BadRequest(c, "runtime name and alias are required")
		return
	}

	resolved, err := h.versionService.ResolveAlias(name, alias)
	if err != nil {
		InternalError(c, err.Error())
		return
	}

	Success(c, gin.H{
		"alias":    alias,
		"resolved": resolved,
	})
}

// GetAliasSuggestions returns alias suggestions for a runtime
func (h *RuntimeVersionHandler) GetAliasSuggestions(c *gin.Context) {
	name := c.Param("name")
	if name == "" {
		BadRequest(c, "runtime name is required")
		return
	}

	suggestions := h.versionService.GetAliasSuggestions(name)

	Success(c, gin.H{
		"suggestions": suggestions,
	})
}

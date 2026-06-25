package api

import (
	"database/sql"

	"easyserver/internal/executor"
	"easyserver/internal/service"

	"github.com/gin-gonic/gin"
)

// registerRuntimeRoutes registers runtime and package management routes
func registerRuntimeRoutes(protected *gin.RouterGroup, db *sql.DB) {
	// Runtime environment management
	cmdExec := executor.NewOSExecutor()
	runtimeService := service.NewRuntimeService(db, cmdExec)
	runtimeHandler := NewRuntimeHandler(runtimeService)
	protected.GET("/runtime", runtimeHandler.List)
	protected.GET("/runtime/:name", runtimeHandler.ListByName)
	protected.POST("/runtime/install", runtimeHandler.Install)
	protected.POST("/runtime/uninstall", runtimeHandler.Uninstall)
	protected.POST("/runtime/set-default", runtimeHandler.SetDefault)
	protected.GET("/runtime/detect", runtimeHandler.Detect)
	protected.POST("/runtime/import-detected", runtimeHandler.ImportDetected)
	protected.GET("/runtime/progress/:id", runtimeHandler.GetProgress)
	protected.GET("/runtime/check-deps/:name", runtimeHandler.CheckDependencies)
	protected.GET("/runtime/logs/:id", runtimeHandler.GetLogs)
	protected.GET("/runtime/cleanup/:id", runtimeHandler.GetCleanupInfo)

	// Runtime version management
	runtimeVersionService := service.NewRuntimeVersionService(db)
	runtimeVersionHandler := NewRuntimeVersionHandler(runtimeVersionService)
	protected.GET("/runtime-versions/:name", runtimeVersionHandler.List)
	protected.POST("/runtime-versions/:name/fetch", runtimeVersionHandler.Fetch)
	protected.GET("/runtime-versions/:name/resolve/:alias", runtimeVersionHandler.ResolveAlias)
	protected.GET("/runtime-versions/:name/suggestions", runtimeVersionHandler.GetAliasSuggestions)

	// Package management
	packageService := service.NewPackageManagerService(db, cmdExec)
	packageHandler := NewPackageManagerHandler(packageService, runtimeService)
	protected.GET("/packages", packageHandler.ListPackages)
	protected.GET("/packages/scan/:id", packageHandler.ScanPackages)
	protected.GET("/packages/search", packageHandler.SearchPackages)
	protected.GET("/packages/versions/:name", packageHandler.GetPackageVersions)
	protected.POST("/packages/install", packageHandler.InstallPackage)
	protected.POST("/packages/uninstall", packageHandler.UninstallPackage)
	protected.POST("/packages/update", packageHandler.UpdatePackage)
}

package http

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"easyserver/internal/httpx"
	"easyserver/internal/httpx/middleware"
	"easyserver/internal/infra/apperror"
	"easyserver/internal/runtimeenv"

	"github.com/gin-gonic/gin"
)

// splitRuntimeBinding 把 "node@20.11.0" 拆成 (lang, exact)。非法格式返回空串。
func splitRuntimeBinding(s string) (lang, exact string) {
	lang, exact, _ = strings.Cut(s, "@")
	return
}

// getRuntimeByBinding 按 lang@exact 取已安装环境；不存在/非法 → 错误响应。
func (h *RuntimeHandler) getRuntimeByBinding(c *gin.Context, binding string) (*runtimeenv.RuntimeEnvironment, bool) {
	lang, exact := splitRuntimeBinding(binding)
	if lang == "" || exact == "" {
		c.Error(apperror.ErrBadRequest.WithMessage("无效的运行时绑定: " + binding))
		return nil, false
	}
	env, err := h.runtimeService.GetByLangExact(c.Request.Context(), lang, exact)
	if err != nil {
		c.Error(apperror.WrapError(err))
		return nil, false
	}
	if env == nil {
		c.Error(apperror.ErrNotFound.WithMessage("运行时环境不存在: " + binding))
		return nil, false
	}
	return env, true
}

// ============================================================
// RuntimeHandler — runtime environment management
// ============================================================

type RuntimeHandler struct {
	runtimeService *runtimeenv.Service
}

func NewRuntimeHandler(runtimeService *runtimeenv.Service) *RuntimeHandler {
	return &RuntimeHandler{runtimeService: runtimeService}
}

// List returns all installed runtime environments（installs/ 目录扫描，ADR-0009）
func (h *RuntimeHandler) List(c *gin.Context) {
	environments, err := h.runtimeService.ListAll(c.Request.Context())
	if err != nil {
		c.Error(apperror.WrapError(err))
		return
	}

	httpx.Success(c, gin.H{
		"environments": environments,
	})
}

// ListByName returns all versions of a specific runtime environment
func (h *RuntimeHandler) ListByName(c *gin.Context) {
	name := c.Param("name")
	if name == "" {
		c.Error(apperror.ErrBadRequest.WithMessage("运行时名称不能为空"))
		return
	}

	// Validate runtime name
	if !runtimeenv.IsSupported(name) {
		c.Error(apperror.ErrBadRequest.WithMessage("不支持的运行时: " + name))
		return
	}

	environments, err := h.runtimeService.ListByName(c.Request.Context(), name)
	if err != nil {
		c.Error(apperror.WrapError(err))
		return
	}

	httpx.Success(c, gin.H{
		"environments": environments,
	})
}

// Install installs a runtime environment
func (h *RuntimeHandler) Install(c *gin.Context) {
	var req runtimeenv.RuntimeInstallRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperror.ErrBadRequest.WithMessage("无效的请求: " + err.Error()))
		return
	}

	// Validate runtime name
	if !runtimeenv.IsSupported(req.Name) {
		c.Error(apperror.ErrBadRequest.WithMessage("不支持的运行时: " + req.Name))
		return
	}

	middleware.AuditSummary(c, "安装运行环境 "+req.Name+" "+req.Version)
	if err := h.runtimeService.Install(c.Request.Context(), req.Name, req.Version); err != nil {
		c.Error(apperror.WrapError(err))
		return
	}

	httpx.Success(c, gin.H{
		"message": "安装已启动",
	})
}

// Uninstall uninstalls a runtime environment
func (h *RuntimeHandler) Uninstall(c *gin.Context) {
	var req runtimeenv.RuntimeUninstallRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperror.ErrBadRequest.WithMessage("无效的请求: " + err.Error()))
		return
	}

	// Validate runtime name
	if !runtimeenv.IsSupported(req.Name) {
		c.Error(apperror.ErrBadRequest.WithMessage("不支持的运行时: " + req.Name))
		return
	}

	middleware.AuditSummary(c, "卸载运行环境 "+req.Name+" "+req.Version)
	if err := h.runtimeService.Uninstall(c.Request.Context(), req.Name, req.Version); err != nil {
		c.Error(apperror.WrapError(err))
		return
	}

	httpx.Success(c, gin.H{
		"message": "卸载成功",
	})
}

// GetLogs 流式返回安装/卸载日志（SSE）。先回放任务已缓冲的行（游标从 0 起），
// 再跟随实时行，任务结束推送 {type:"done", error} 并关闭。任务不存在（成功即清，
// 或面板重启后内存日志已失）时立即推送 done + 说明。绑定键 lang@exact 在路径
// 参数里（ADR-0009）。
func (h *RuntimeHandler) GetLogs(c *gin.Context) {
	lang, exact := splitRuntimeBinding(c.Param("binding"))
	if lang == "" || exact == "" {
		c.Error(apperror.ErrBadRequest.WithMessage("无效的运行时绑定"))
		return
	}

	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("X-Accel-Buffering", "no")
	c.Writer.Flush()

	// 行与结束都作为默认 data: 事件推送（JSON 信封 {type, ...}），前端
	// EventSource onmessage 解析。
	send := func(payload map[string]string) {
		b, _ := json.Marshal(payload)
		fmt.Fprintf(c.Writer, "data: %s\n\n", b)
		c.Writer.Flush()
	}

	tk, ok := h.runtimeService.InstallTask(lang, exact)
	if !ok {
		send(map[string]string{"type": "done", "error": "安装日志已丢失（服务可能已重启或安装已完成），无法查看"})
		return
	}
	log := tk.Log()

	cursor := 0
	for {
		select {
		case <-c.Request.Context().Done():
			return
		default:
		}
		lines, next := log.Tail(cursor)
		for _, line := range lines {
			send(map[string]string{"type": "line", "text": line})
		}
		cursor = next

		select {
		case <-tk.Done():
			// Flush anything that landed between the tail above and completion.
			if lines, _ := log.Tail(cursor); len(lines) > 0 {
				for _, line := range lines {
					send(map[string]string{"type": "line", "text": line})
				}
			}
			errMsg := ""
			if tk.Err() != nil {
				errMsg = tk.Err().Error()
			}
			send(map[string]string{"type": "done", "error": errMsg})
			return
		case <-time.After(300 * time.Millisecond):
		}
	}
}

// GetCleanupInfo 返回卸载确认所需的运行环境信息（卸载确认弹窗用）。
func (h *RuntimeHandler) GetCleanupInfo(c *gin.Context) {
	env, ok := h.getRuntimeByBinding(c, c.Param("binding"))
	if !ok {
		return
	}

	httpx.Success(c, gin.H{
		"runtime": env,
	})
}

// GetRemoteVersions gets available remote versions
func (h *RuntimeHandler) GetRemoteVersions(c *gin.Context) {
	name := c.Param("name")
	if name == "" {
		c.Error(apperror.ErrBadRequest.WithMessage("运行时名称不能为空"))
		return
	}

	if !runtimeenv.IsSupported(name) {
		c.Error(apperror.ErrBadRequest.WithMessage("不支持的运行时: " + name))
		return
	}

	versions, err := h.runtimeService.GetRemoteVersions(c.Request.Context(), name)
	if err != nil {
		c.Error(apperror.WrapError(err))
		return
	}

	httpx.Success(c, gin.H{"versions": versions})
}

// GetCatalog returns the catalog of supported runtimes
func (h *RuntimeHandler) GetCatalog(c *gin.Context) {
	httpx.Success(c, gin.H{
		"catalog": runtimeenv.GetCatalog(),
	})
}

// ============================================================
// PackageManagerHandler — package management for runtimes
// ============================================================

type PackageManagerHandler struct {
	packageService *runtimeenv.PackageService
	runtimeService *runtimeenv.Service
}

func NewPackageManagerHandler(packageService *runtimeenv.PackageService, runtimeService *runtimeenv.Service) *PackageManagerHandler {
	return &PackageManagerHandler{
		packageService: packageService,
		runtimeService: runtimeService,
	}
}

// getRuntimeFromBinding 按 lang@exact 取已安装环境并校验包管理支持。
// 成功后返回 (lang, exact, path)。binding 为空时报错。
func (h *PackageManagerHandler) getRuntimeFromBinding(c *gin.Context, binding string) (lang, exact, path string, ok bool) {
	if binding == "" {
		c.Error(apperror.ErrBadRequest.WithMessage("runtime 不能为空（格式 lang@exact）"))
		return
	}
	lang, exact = splitRuntimeBinding(binding)
	if lang == "" || exact == "" {
		c.Error(apperror.ErrBadRequest.WithMessage("无效的 runtime: " + binding))
		return
	}
	env, err := h.runtimeService.GetByLangExact(c.Request.Context(), lang, exact)
	if err != nil {
		c.Error(apperror.WrapError(err))
		return
	}
	if env == nil {
		c.Error(apperror.ErrNotFound.WithMessage("运行时不存在: " + binding))
		return
	}
	if !runtimeenv.SupportsGlobalPkgsFor(lang) {
		c.Error(apperror.ErrBadRequest.WithMessage(fmt.Sprintf("运行环境 %s 暂不支持面板全局包管理", lang)))
		return
	}
	return lang, exact, env.Path, true
}

// ListPackages returns all packages for a runtime by scanning the system
// package manager directly. There is no DB cache, so each call reflects the
// current state of the runtime's package manager.
func (h *PackageManagerHandler) ListPackages(c *gin.Context) {
	lang, _, path, ok := h.getRuntimeFromBinding(c, c.Query("runtime"))
	if !ok {
		return
	}
	packages, err := h.packageService.ListPackages(c.Request.Context(), lang, path)
	if err != nil {
		c.Error(apperror.WrapError(err))
		return
	}
	httpx.Success(c, gin.H{
		"packages": packages,
	})
}

// InstallPackage installs a package
func (h *PackageManagerHandler) InstallPackage(c *gin.Context) {
	var req runtimeenv.PackageInstallRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperror.ErrBadRequest.WithMessage("无效的请求: " + err.Error()))
		return
	}

	lang, _, path, ok := h.getRuntimeFromBinding(c, c.Query("runtime"))
	if !ok {
		return
	}

	summary := "安装包 " + req.Name
	if req.Manager != "" {
		summary = req.Manager + " 安装 " + req.Name
	}
	if req.Version != "" {
		summary += " (版本: " + req.Version + ")"
	}
	middleware.AuditSummary(c, summary)
	if err := h.packageService.InstallPackage(c.Request.Context(), &req, lang, path); err != nil {
		c.Error(apperror.WrapError(err))
		return
	}

	httpx.Success(c, gin.H{
		"message": "包安装已启动",
	})
}

// UninstallPackage uninstalls a package
func (h *PackageManagerHandler) UninstallPackage(c *gin.Context) {
	var req runtimeenv.PackageUninstallRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperror.ErrBadRequest.WithMessage("无效的请求: " + err.Error()))
		return
	}

	lang, _, path, ok := h.getRuntimeFromBinding(c, c.Query("runtime"))
	if !ok {
		return
	}

	summary := "卸载包 " + req.Name
	if req.Manager != "" {
		summary = req.Manager + " 卸载 " + req.Name
	}
	middleware.AuditSummary(c, summary)
	if err := h.packageService.UninstallPackage(c.Request.Context(), &req, lang, path); err != nil {
		c.Error(apperror.WrapError(err))
		return
	}

	httpx.Success(c, gin.H{
		"message": "包卸载成功",
	})
}

// UpdatePackage updates a package
func (h *PackageManagerHandler) UpdatePackage(c *gin.Context) {
	var req runtimeenv.PackageUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperror.ErrBadRequest.WithMessage("无效的请求: " + err.Error()))
		return
	}

	lang, _, path, ok := h.getRuntimeFromBinding(c, c.Query("runtime"))
	if !ok {
		return
	}

	// Try to get old version before update
	var oldVersion string
	pkgs, err := h.packageService.ListPackages(c.Request.Context(), lang, path)
	if err == nil {
		for _, p := range pkgs {
			if p.Name == req.Name {
				oldVersion = p.Version
				break
			}
		}
	}

	if err := h.packageService.UpdatePackage(c.Request.Context(), &req, lang, path); err != nil {
		c.Error(apperror.WrapError(err))
		return
	}

	// Try to get new version after update
	var newVersion string
	pkgsAfter, err := h.packageService.ListPackages(c.Request.Context(), lang, path)
	if err == nil {
		for _, p := range pkgsAfter {
			if p.Name == req.Name {
				newVersion = p.Version
				break
			}
		}
	}

	summary := "更新包 " + req.Name
	if req.Manager != "" {
		summary = req.Manager + " 更新 " + req.Name
	}
	if oldVersion != "" && newVersion != "" && oldVersion != newVersion {
		summary += " (" + oldVersion + " -> " + newVersion + ")"
	} else if newVersion != "" {
		summary += " (版本: " + newVersion + ")"
	}
	middleware.AuditSummary(c, summary)

	httpx.Success(c, gin.H{
		"message": "包更新成功",
	})
}

// SearchPackages searches for available packages
func (h *PackageManagerHandler) SearchPackages(c *gin.Context) {
	lang, _, _, ok := h.getRuntimeFromBinding(c, c.Query("runtime"))
	if !ok {
		return
	}
	query := c.Query("q")

	packages, err := h.packageService.SearchPackages(c.Request.Context(), lang, query)
	if err != nil {
		c.Error(apperror.WrapError(err))
		return
	}

	httpx.Success(c, gin.H{
		"packages": packages,
	})
}

// GetPackageVersions returns available versions for a package
func (h *PackageManagerHandler) GetPackageVersions(c *gin.Context) {
	lang, _, _, ok := h.getRuntimeFromBinding(c, c.Query("runtime"))
	if !ok {
		return
	}
	packageName := c.Query("name")
	if packageName == "" {
		c.Error(apperror.ErrBadRequest.WithMessage("缺少包名参数"))
		return
	}

	versions, err := h.packageService.GetPackageVersions(c.Request.Context(), lang, packageName)
	if err != nil {
		c.Error(apperror.WrapError(err))
		return
	}

	httpx.Success(c, gin.H{
		"versions": versions,
	})
}

// GetRegistry gets the package manager registry
func (h *PackageManagerHandler) GetRegistry(c *gin.Context) {
	lang, _, path, ok := h.getRuntimeFromBinding(c, c.Query("runtime"))
	if !ok {
		return
	}
	manager := c.Query("manager")

	registry, err := h.packageService.GetRegistry(c.Request.Context(), lang, path, manager)
	if err != nil {
		c.Error(apperror.WrapError(err))
		return
	}

	httpx.Success(c, gin.H{
		"registry": registry,
	})
}

// SetRegistry sets the package manager registry
func (h *PackageManagerHandler) SetRegistry(c *gin.Context) {
	var req struct {
		Runtime  string `json:"runtime"` // lang@exact
		Manager  string `json:"manager"`
		Registry string `json:"registry"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperror.ErrBadRequest.WithMessage("无效的请求: " + err.Error()))
		return
	}

	lang, _, path, ok := h.getRuntimeFromBinding(c, req.Runtime)
	if !ok {
		return
	}

	middleware.AuditSummary(c, "配置包管理器镜像源 "+req.Manager)
	if err := h.packageService.SetRegistry(c.Request.Context(), lang, path, req.Manager, req.Registry); err != nil {
		c.Error(apperror.WrapError(err))
		return
	}

	httpx.Success(c, gin.H{
		"message": "配置保存成功",
	})
}

// ============================================================
// MirrorHandler — 镜像源管理（config.toml [env] 文件即权威）
// ============================================================

type MirrorHandler struct {
	mirrorService *runtimeenv.MirrorService
}

func NewMirrorHandler(mirrorService *runtimeenv.MirrorService) *MirrorHandler {
	return &MirrorHandler{mirrorService: mirrorService}
}

// List 返回 config.toml [env] 段全部条目。
func (h *MirrorHandler) List(c *gin.Context) {
	entries, err := h.mirrorService.List(c.Request.Context())
	if err != nil {
		c.Error(apperror.WrapError(err))
		return
	}
	httpx.Success(c, gin.H{"mirrors": entries})
}

// Create 新增/覆盖一个镜像源（保存即写入文件生效）。
func (h *MirrorHandler) Create(c *gin.Context) {
	var req struct {
		EnvKey   string `json:"env_key" binding:"required"`
		EnvValue string `json:"env_value" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperror.ErrBadRequest.WithMessage("无效的请求: " + err.Error()))
		return
	}
	middleware.AuditSummary(c, "创建镜像源 "+req.EnvKey)
	if err := h.mirrorService.Upsert(c.Request.Context(), req.EnvKey, req.EnvValue); err != nil {
		c.Error(apperror.WrapError(err))
		return
	}
	httpx.Success(c, gin.H{"message": "已保存"})
}

// Update 更新指定 env_key 的镜像地址。
func (h *MirrorHandler) Update(c *gin.Context) {
	envKey := c.Param("env_key")
	var req struct {
		EnvValue string `json:"env_value" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperror.ErrBadRequest.WithMessage("无效的请求: " + err.Error()))
		return
	}
	middleware.AuditSummary(c, "更新镜像源 "+envKey)
	if err := h.mirrorService.Upsert(c.Request.Context(), envKey, req.EnvValue); err != nil {
		c.Error(apperror.WrapError(err))
		return
	}
	httpx.Success(c, gin.H{"message": "已保存"})
}

// Delete 从文件删除指定 env_key 的镜像源。
func (h *MirrorHandler) Delete(c *gin.Context) {
	envKey := c.Param("env_key")
	middleware.AuditSummary(c, "删除镜像源 "+envKey)
	if err := h.mirrorService.Delete(c.Request.Context(), envKey); err != nil {
		c.Error(apperror.WrapError(err))
		return
	}
	httpx.Success(c, gin.H{"message": "已删除"})
}

// ============================================================
// Route registration
// ============================================================

func RegisterRoutes(protected *gin.RouterGroup, runtimeService *runtimeenv.Service, packageService *runtimeenv.PackageService, mirrorService *runtimeenv.MirrorService) {
	// Runtime environment management
	runtimeHandler := NewRuntimeHandler(runtimeService)
	protected.GET("/runtime", runtimeHandler.List)
	protected.GET("/runtime/:name", runtimeHandler.ListByName)
	protected.GET("/runtime/:name/remote-versions", runtimeHandler.GetRemoteVersions)
	protected.POST("/runtime/install", runtimeHandler.Install)
	protected.POST("/runtime/uninstall", runtimeHandler.Uninstall)
	protected.GET("/runtime/logs/:binding", runtimeHandler.GetLogs)
	protected.GET("/runtime/cleanup/:binding", runtimeHandler.GetCleanupInfo)
	protected.GET("/runtime/catalog", runtimeHandler.GetCatalog)

	// Mirror source management（独立于环境变量 API，文件即权威）
	mirrorHandler := NewMirrorHandler(mirrorService)
	protected.GET("/runtime/mirrors", mirrorHandler.List)
	protected.POST("/runtime/mirrors", mirrorHandler.Create)
	protected.PUT("/runtime/mirrors/:env_key", mirrorHandler.Update)
	protected.DELETE("/runtime/mirrors/:env_key", mirrorHandler.Delete)

	// Package management
	packageHandler := NewPackageManagerHandler(packageService, runtimeService)
	protected.GET("/packages", packageHandler.ListPackages)
	protected.GET("/packages/search", packageHandler.SearchPackages)
	protected.GET("/packages/versions", packageHandler.GetPackageVersions)
	protected.POST("/packages/install", packageHandler.InstallPackage)
	protected.POST("/packages/uninstall", packageHandler.UninstallPackage)
	protected.POST("/packages/update", packageHandler.UpdatePackage)
	protected.GET("/packages/registry", packageHandler.GetRegistry)
	protected.POST("/packages/registry", packageHandler.SetRegistry)
}

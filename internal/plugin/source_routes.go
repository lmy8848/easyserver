package plugin

import (
	"strconv"

	"easyserver/internal/httpx"
	"easyserver/internal/httpx/middleware"
	"easyserver/internal/infra/errx"

	"github.com/gin-gonic/gin"
)

// registerSourceRoutes 注册插件源（市场入口）路由：源管理、拉取清单、从源安装。
func registerSourceRoutes(protected *gin.RouterGroup, m *Manager) {
	svc := m.SourceService()

	// 列出已配置的插件源
	protected.GET("/plugins/sources", func(c *gin.Context) {
		if svc == nil {
			httpx.Success(c, gin.H{"items": []PluginSource{}})
			return
		}
		items, err := svc.List(c.Request.Context())
		if err != nil {
			c.Error(err)
			return
		}
		httpx.Success(c, gin.H{"items": items})
	})

	// 添加插件源 {name, url}
	protected.POST("/plugins/sources", func(c *gin.Context) {
		if svc == nil {
			c.Error(errx.Unavailable("插件源功能未启用"))
			return
		}
		var body struct {
			Name string `json:"name" binding:"required"`
			URL  string `json:"url" binding:"required"`
		}
		if err := c.ShouldBindJSON(&body); err != nil {
			c.Error(errx.BadRequest("请求体无效"))
			return
		}
		src, err := svc.Add(c.Request.Context(), body.Name, body.URL)
		if err != nil {
			logManagerf(" 添加插件源失败（%s）: %v", body.URL, err)
			c.Error(errx.BadRequest("%v", err))
			return
		}
		logManagerf(" 添加插件源 %s（%s）", src.Name, src.URL)
		middleware.AuditSummary(c, "添加插件源 "+src.Name)
		httpx.Success(c, gin.H{"message": "插件源已添加", "source": src})
	})

	// 删除插件源
	protected.DELETE("/plugins/sources/:id", func(c *gin.Context) {
		if svc == nil {
			c.Error(errx.Unavailable("插件源功能未启用"))
			return
		}
		id, err := strconv.ParseInt(c.Param("id"), 10, 64)
		if err != nil {
			c.Error(errx.BadRequest("插件源 id 无效"))
			return
		}
		if err := svc.Delete(c.Request.Context(), id); err != nil {
			c.Error(err)
			return
		}
		logManagerf(" 删除插件源 id=%d", id)
		middleware.AuditSummary(c, "删除插件源 id="+c.Param("id"))
		httpx.Success(c, gin.H{"message": "插件源已删除"})
	})

	// 拉取插件源的插件清单（GET {url}/index.json）
	protected.POST("/plugins/sources/:id/fetch", func(c *gin.Context) {
		if svc == nil {
			c.Error(errx.Unavailable("插件源功能未启用"))
			return
		}
		id, err := strconv.ParseInt(c.Param("id"), 10, 64)
		if err != nil {
			c.Error(errx.BadRequest("插件源 id 无效"))
			return
		}
		items, err := svc.List(c.Request.Context())
		if err != nil {
			c.Error(err)
			return
		}
		var src *PluginSource
		for i := range items {
			if items[i].ID == id {
				src = &items[i]
				break
			}
		}
		if src == nil {
			c.Error(errx.NotFound("插件源不存在"))
			return
		}
		if !src.Enabled {
			c.Error(errx.BadRequest("插件源已禁用"))
			return
		}
		cat, err := svc.FetchCatalog(c.Request.Context(), src)
		if err != nil {
			logManagerf(" 拉取插件源 %s 清单失败: %v", src.Name, err)
			c.Error(errx.BadRequest("%v", err))
			return
		}
		logManagerf(" 拉取插件源 %s 清单（%d 个插件）", src.Name, len(cat.Plugins))
		httpx.Success(c, gin.H{"source": src, "catalog": cat})
	})

	// 从源安装：{url: 插件 zip 下载地址} → 下载并复用 InstallFromZip
	protected.POST("/plugins/sources/install", func(c *gin.Context) {
		if svc == nil {
			c.Error(errx.Unavailable("插件源功能未启用"))
			return
		}
		var body struct {
			URL string `json:"url" binding:"required"`
		}
		if err := c.ShouldBindJSON(&body); err != nil {
			c.Error(errx.BadRequest("请求体无效"))
			return
		}
		mft, err := svc.InstallFromURL(c.Request.Context(), body.URL)
		if err != nil {
			logManagerf(" 从源安装插件失败: %v", err)
			c.Error(errx.BadRequest("%v", err))
			return
		}
		middleware.AuditSummary(c, "从插件源安装 "+mft.Name+"@"+mft.Version)
		httpx.Success(c, gin.H{"message": "插件已从源安装", "plugin": mft.Name, "version": mft.Version})
	})
}

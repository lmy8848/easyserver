package plugin

import (
	"io"
	"net/http"

	"easyserver/internal/httpx/middleware"

	"github.com/gin-gonic/gin"
)

// RegisterRoutes 注册插件管理 API 与插件反向代理入口（面板 Protected 组）。
func RegisterRoutes(protected *gin.RouterGroup, m *Manager) {
	// 插件管理
	// 上传安装插件（zip 包）
	protected.POST("/plugins/upload", func(c *gin.Context) {
		fh, err := c.FormFile("file")
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"message": "缺少插件包文件字段 file"})
			return
		}
		f, err := fh.Open()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"message": "读取插件包失败"})
			return
		}
		defer func() { _ = f.Close() }()
		data, err := io.ReadAll(io.LimitReader(f, maxPluginArchiveSize+1))
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"message": "读取插件包失败"})
			return
		}
		mft, err := m.InstallFromZip(c.Request.Context(), data)
		if err != nil {
			c.Error(err)
			return
		}
		middleware.AuditSummary(c, "安装插件 "+mft.Name+"@"+mft.Version)
		c.JSON(http.StatusOK, gin.H{"message": "插件已安装", "plugin": mft.Name, "version": mft.Version})
	})

	protected.GET("/plugins", func(c *gin.Context) {
		list, err := m.List(c.Request.Context())
		if err != nil {
			c.Error(err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"items": list})
	})
	protected.POST("/plugins/:id/start", func(c *gin.Context) {
		if err := m.Start(c.Request.Context(), c.Param("id")); err != nil {
			c.Error(err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "插件已启动"})
	})
	protected.POST("/plugins/:id/stop", func(c *gin.Context) {
		if err := m.Stop(c.Request.Context(), c.Param("id")); err != nil {
			c.Error(err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "插件已停止"})
	})
	protected.POST("/plugins/:id/enable", func(c *gin.Context) {
		if err := m.Start(c.Request.Context(), c.Param("id")); err != nil {
			c.Error(err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "插件已启用"})
	})
	protected.POST("/plugins/:id/disable", func(c *gin.Context) {
		if err := m.Disable(c.Request.Context(), c.Param("id")); err != nil {
			c.Error(err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "插件已禁用"})
	})
	protected.DELETE("/plugins/:id", func(c *gin.Context) {
		if err := m.Remove(c.Request.Context(), c.Param("id")); err != nil {
			c.Error(err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "插件已卸载"})
	})
	// 聚合小部件下发：仪表盘消费（启用插件的 widgets contribution）
	protected.GET("/plugins/widgets", func(c *gin.Context) {
		list, err := m.List(c.Request.Context())
		if err != nil {
			c.Error(err)
			return
		}
		items := []gin.H{}
		for _, rec := range list {
			if !rec.Enabled {
				continue
			}
			mft, err := m.GetManifest(c.Request.Context(), rec.Name)
			if err != nil {
				continue
			}
			for _, w := range mft.Contributions.Widgets {
				items = append(items, gin.H{
					"plugin": rec.Name, "id": w.ID, "title": w.Title,
					"slot": w.Slot, "path": w.Path, "size": w.Size,
				})
			}
		}
		c.JSON(http.StatusOK, gin.H{"items": items})
	})

	// 聚合菜单下发：前端动态侧边栏消费（启用插件的 menus contribution）
	protected.GET("/plugins/menus", func(c *gin.Context) {
		list, err := m.List(c.Request.Context())
		if err != nil {
			c.Error(err)
			return
		}
		items := []gin.H{}
		for _, rec := range list {
			if !rec.Enabled {
				continue
			}
			mft, err := m.GetManifest(c.Request.Context(), rec.Name)
			if err != nil {
				continue
			}
			for _, menu := range mft.Contributions.Menus {
				items = append(items, gin.H{
					"plugin": rec.Name, "id": menu.ID, "title": menu.Title,
					"icon": menu.Icon, "path": menu.Path, "order": menu.Order,
				})
			}
		}
		c.JSON(http.StatusOK, gin.H{"items": items})
	})

	// 插件配置读写（面板前端经 config_schema 渲染表单后保存）
	protected.GET("/plugins/:id/config", func(c *gin.Context) {
		cfg, err := m.ConfigList(c.Request.Context(), c.Param("id"))
		if err != nil {
			c.Error(err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"config": cfg})
	})
	protected.PUT("/plugins/:id/config", func(c *gin.Context) {
		var body map[string]string
		if err := c.ShouldBindJSON(&body); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"message": "请求体无效"})
			return
		}
		if err := m.ConfigSet(c.Request.Context(), c.Param("id"), body); err != nil {
			c.Error(err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "配置已保存"})
	})

	protected.GET("/plugins/:id/manifest", func(c *gin.Context) {
		mft, err := m.GetManifest(c.Request.Context(), c.Param("id"))
		if err != nil {
			c.Error(err)
			return
		}
		c.JSON(http.StatusOK, mft)
	})

	// 插件业务 API 反向代理：/api/plugins/<id>/ 前缀 → 插件 socket
	protected.Any("/api/plugins/:id/*rest", func(c *gin.Context) {
		name := c.Param("id")
		sockPath, timeout, ok := m.sup.RunningEndpoint(name)
		if !ok {
			c.JSON(http.StatusServiceUnavailable, gin.H{"message": "插件未运行"})
			return
		}
		newPluginProxy(sockPath, timeout).ServeHTTP(c.Writer, c.Request)
	})

	// 前端动态菜单下发（menus contribution）
	protected.GET("/plugins/:id/menus", func(c *gin.Context) {
		mft, err := m.GetManifest(c.Request.Context(), c.Param("id"))
		if err != nil {
			c.Error(err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"items": mft.Contributions.Menus})
	})
}

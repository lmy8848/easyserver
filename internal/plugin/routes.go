package plugin

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// RegisterRoutes 注册插件管理 API 与插件反向代理入口（面板 Protected 组）。
func RegisterRoutes(protected *gin.RouterGroup, m *Manager) {
	// 插件管理
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

package plugin

import (
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"

	"easyserver/internal/httpx"
	"easyserver/internal/httpx/middleware"

	"github.com/gin-gonic/gin"
)

// proxyHandler 由 router 注入（已带 JWT 认证）；embed 的 NoRoute 兜底对
// /api/plugins/<id>/* 路径优先调用。插件业务路径无法作为 Gin 路由注册
// （catch-all 与 /plugins/:id/* 静态管理路由在同一节点冲突），故走 NoRoute。
var proxyHandler atomic.Pointer[gin.HandlerFunc]

// SetProxyHandler 注入插件代理处理器（router 接线，含认证）。
func SetProxyHandler(h gin.HandlerFunc) { proxyHandler.Store(&h) }

// ProxyHandler 返回已注入的插件代理处理器；nil 表示未接线。
func ProxyHandler() gin.HandlerFunc {
	if p := proxyHandler.Load(); p != nil {
		return *p
	}
	return nil
}

// NewProxyHandler 构造插件业务代理处理器：/api/plugins/<id>/* → 插件 socket
// （剥离前缀，认证由外层中间件保证）。
func NewProxyHandler(m *Manager) gin.HandlerFunc {
	return func(c *gin.Context) {
		rest := strings.TrimPrefix(c.Request.URL.Path, "/api/plugins/")
		name, _, _ := strings.Cut(rest, "/")
		if name == "" {
			c.JSON(http.StatusNotFound, gin.H{"code": 40400, "message": "页面未找到"})
			return
		}
		sockPath, timeout, ok := m.sup.RunningEndpoint(name)
		log.Printf("plugin[proxy]: path=%s name=%s sock=%s ok=%v timeout=%s", c.Request.URL.Path, name, sockPath, ok, timeout)
		if !ok {
			c.JSON(http.StatusServiceUnavailable, gin.H{"message": "插件未运行"})
			return
		}
		newPluginProxy(sockPath, timeout, "/api/plugins/"+name).ServeHTTP(c.Writer, c.Request)
	}
}

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
		httpx.Success(c, gin.H{"message": "插件已安装", "plugin": mft.Name, "version": mft.Version})
	})

	protected.GET("/plugins", func(c *gin.Context) {
		list, err := m.List(c.Request.Context())
		if err != nil {
			c.Error(err)
			return
		}
		httpx.Success(c, gin.H{"items": list})
	})
	protected.POST("/plugins/:id/start", func(c *gin.Context) {
		if err := m.Start(c.Request.Context(), c.Param("id")); err != nil {
			c.Error(err)
			return
		}
		httpx.Success(c, gin.H{"message": "插件已启动"})
	})
	protected.POST("/plugins/:id/stop", func(c *gin.Context) {
		if err := m.Stop(c.Request.Context(), c.Param("id")); err != nil {
			c.Error(err)
			return
		}
		httpx.Success(c, gin.H{"message": "插件已停止"})
	})
	protected.POST("/plugins/:id/enable", func(c *gin.Context) {
		if err := m.Start(c.Request.Context(), c.Param("id")); err != nil {
			c.Error(err)
			return
		}
		httpx.Success(c, gin.H{"message": "插件已启用"})
	})
	protected.POST("/plugins/:id/disable", func(c *gin.Context) {
		if err := m.Disable(c.Request.Context(), c.Param("id")); err != nil {
			c.Error(err)
			return
		}
		httpx.Success(c, gin.H{"message": "插件已禁用"})
	})
	protected.DELETE("/plugins/:id", func(c *gin.Context) {
		if err := m.Remove(c.Request.Context(), c.Param("id")); err != nil {
			c.Error(err)
			return
		}
		httpx.Success(c, gin.H{"message": "插件已卸载"})
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
		httpx.Success(c, gin.H{"items": items})
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
		httpx.Success(c, gin.H{"items": items})
	})

	// 插件日志（面板侧 /var/log/easyserver/plugins/<id>.log，tail 查看）
	protected.GET("/plugins/:id/logs", func(c *gin.Context) {
		name := c.Param("id")
		lines := 200
		if v := c.Query("lines"); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 2000 {
				lines = n
			}
		}
		path := filepath.Join("/var/log/easyserver/plugins", name+".log")
		data, err := os.ReadFile(path)
		if err != nil {
			httpx.Success(c, gin.H{"log": ""})
			return
		}
		// 取尾部 lines 行
		all := string(data)
		parts := strings.Split(all, "\n")
		if len(parts) > lines {
			parts = parts[len(parts)-lines:]
		}
		httpx.Success(c, gin.H{"log": strings.Join(parts, "\n")})
	})

	// 插件详情（manifest + 贡献点清单，供管理页展示）
	protected.GET("/plugins/:id/detail", func(c *gin.Context) {
		name := c.Param("id")
		rec, err := m.repo.GetPlugin(c.Request.Context(), name)
		if err != nil || rec == nil {
			c.JSON(http.StatusNotFound, gin.H{"message": "插件不存在"})
			return
		}
		mft, err := m.GetManifest(c.Request.Context(), name)
		if err != nil {
			c.Error(err)
			return
		}
		httpx.Success(c, gin.H{
			"record":        rec,
			"manifest":      mft,
			"contributions": mft.Contributions,
		})
	})

	// 插件配置读写（面板前端经 config_schema 渲染表单后保存）
	protected.GET("/plugins/:id/config", func(c *gin.Context) {
		cfg, err := m.ConfigList(c.Request.Context(), c.Param("id"))
		if err != nil {
			c.Error(err)
			return
		}
		httpx.Success(c, gin.H{"config": cfg})
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
		httpx.Success(c, gin.H{"message": "配置已保存"})
	})

	protected.GET("/plugins/:id/manifest", func(c *gin.Context) {
		mft, err := m.GetManifest(c.Request.Context(), c.Param("id"))
		if err != nil {
			c.Error(err)
			return
		}
		httpx.Success(c, mft)
	})

	// 前端动态菜单下发（menus contribution）
	protected.GET("/plugins/:id/menus", func(c *gin.Context) {
		mft, err := m.GetManifest(c.Request.Context(), c.Param("id"))
		if err != nil {
			c.Error(err)
			return
		}
		httpx.Success(c, gin.H{"items": mft.Contributions.Menus})
	})
}

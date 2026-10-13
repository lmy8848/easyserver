package plugin

import (
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"

	"easyserver/internal/httpx"
	"easyserver/internal/httpx/middleware"
	"easyserver/internal/infra/errx"

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
			c.Error(errx.NotFound("页面未找到"))
			return
		}
		sockPath, timeout, ok := m.sup.RunningEndpoint(name)
		if !ok {
			c.Error(errx.Unavailable("插件未运行"))
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
			logManagerf(" 安装插件失败: 缺少插件包文件字段 file")
			c.Error(errx.BadRequest("缺少插件包文件字段 file"))
			return
		}
		f, err := fh.Open()
		if err != nil {
			logManagerf(" 安装插件失败: 读取上传文件失败")
			c.Error(errx.Internal("读取插件包失败"))
			return
		}
		defer func() { _ = f.Close() }()
		data, err := io.ReadAll(io.LimitReader(f, maxPluginArchiveSize+1))
		if err != nil {
			logManagerf(" 安装插件失败: 读取插件包失败")
			c.Error(errx.Internal("读取插件包失败"))
			return
		}
		mft, err := m.InstallFromZip(c.Request.Context(), data)
		if err != nil {
			logManagerf(" 安装插件失败: %v", err)
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
		name := c.Param("id")
		if err := m.Start(c.Request.Context(), name); err != nil {
			logManagerf(" 启动插件 %s 失败: %v", name, err)
			c.Error(err)
			return
		}
		logManagerf(" 启动插件 %s", name)
		middleware.AuditSummary(c, "启动插件 "+name)
		httpx.Success(c, gin.H{"message": "插件已启动"})
	})
	protected.POST("/plugins/:id/stop", func(c *gin.Context) {
		name := c.Param("id")
		if err := m.Stop(c.Request.Context(), name); err != nil {
			logManagerf(" 停止插件 %s 失败: %v", name, err)
			c.Error(err)
			return
		}
		logManagerf(" 停止插件 %s", name)
		middleware.AuditSummary(c, "停止插件 "+name)
		httpx.Success(c, gin.H{"message": "插件已停止"})
	})
	protected.POST("/plugins/:id/enable", func(c *gin.Context) {
		name := c.Param("id")
		if err := m.Start(c.Request.Context(), name); err != nil {
			logManagerf(" 启用插件 %s 失败: %v", name, err)
			c.Error(err)
			return
		}
		logManagerf(" 启用插件 %s", name)
		middleware.AuditSummary(c, "启用插件 "+name)
		httpx.Success(c, gin.H{"message": "插件已启用"})
	})
	protected.POST("/plugins/:id/disable", func(c *gin.Context) {
		name := c.Param("id")
		if err := m.Disable(c.Request.Context(), name); err != nil {
			logManagerf(" 禁用插件 %s 失败: %v", name, err)
			c.Error(err)
			return
		}
		logManagerf(" 禁用插件 %s", name)
		middleware.AuditSummary(c, "禁用插件 "+name)
		httpx.Success(c, gin.H{"message": "插件已禁用"})
	})
	// 启用时的权限审批：批准声明权限的子集后启动（§11，不可超声明）
	protected.POST("/plugins/:id/approve", func(c *gin.Context) {
		name := c.Param("id")
		var body struct {
			Permissions []string `json:"permissions"`
		}
		if err := c.ShouldBindJSON(&body); err != nil {
			c.Error(errx.BadRequest("请求体无效"))
			return
		}
		if err := m.Approve(c.Request.Context(), name, body.Permissions); err != nil {
			logManagerf(" 审批插件 %s 权限并启动失败: %v", name, err)
			c.Error(err)
			return
		}
		logManagerf(" 审批插件 %s 权限并启动（%d 项）", name, len(body.Permissions))
		middleware.AuditSummary(c, "审批插件 "+name+" 权限并启动")
		httpx.Success(c, gin.H{"message": "权限已批准并启动插件"})
	})
	protected.DELETE("/plugins/:id", func(c *gin.Context) {
		name := c.Param("id")
		if err := m.Remove(c.Request.Context(), name); err != nil {
			logManagerf(" 卸载插件 %s 失败: %v", name, err)
			c.Error(err)
			return
		}
		logManagerf(" 卸载插件 %s", name)
		middleware.AuditSummary(c, "卸载插件 "+name)
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

	// 聚合样式下发：前端样式注入消费（启用插件的 styles contribution）
	protected.GET("/plugins/styles", func(c *gin.Context) {
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
			for _, s := range mft.Contributions.Styles {
				items = append(items, gin.H{
					"plugin": rec.Name, "id": s.ID,
					// 样式经插件代理提供：/api/plugins/<id>/<path>
					"url": "/api/plugins/" + rec.Name + "/" + s.Path,
				})
			}
		}
		httpx.Success(c, gin.H{"items": items})
	})

	// 聚合 web 服务器类型下发：面板网站管理并入插件注册的类型（web_servers 贡献点）
	protected.GET("/plugins/web-servers", func(c *gin.Context) {
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
			for _, w := range mft.Contributions.WebServers {
				items = append(items, gin.H{
					"plugin": rec.Name, "name": w.Name, "display_name": w.DisplayName,
					"description": w.Description, "default_port": w.DefaultPort,
				})
			}
		}
		httpx.Success(c, gin.H{"items": items})
	})

	// 聚合命令下发：面板/插件可调用的命令清单（commands 贡献点）
	protected.GET("/plugins/commands", func(c *gin.Context) {
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
			for _, cmd := range mft.Contributions.Commands {
				items = append(items, gin.H{
					"plugin": rec.Name, "id": cmd.ID, "title": cmd.Title,
				})
			}
		}
		httpx.Success(c, gin.H{"items": items})
	})

	// 聚合数据源下发：面板/前端可消费的插件数据源清单（datasources 贡献点）
	protected.GET("/plugins/datasources", func(c *gin.Context) {
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
			for _, ds := range mft.Contributions.Datasources {
				items = append(items, gin.H{
					"plugin": rec.Name, "id": ds.ID, "title": ds.Title, "kind": ds.Kind,
					// 数据经插件代理提供：/api/plugins/<id>/datasources/<id>
					"url": "/api/plugins/" + rec.Name + "/datasources/" + ds.ID,
				})
			}
		}
		httpx.Success(c, gin.H{"items": items})
	})

	// 插件源（市场入口）：源管理 + 拉取清单 + 从源安装。
	// 源是暴露 index.json 的 HTTP 端点；安装复用 InstallFromZip（zip slip/大小/校验防护）。
	registerSourceRoutes(protected, m)

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

	// 插件管理子系统全局日志（面板侧管理动作，见 manager_log.go）
	protected.GET("/plugins/system-logs", func(c *gin.Context) {
		lines := 200
		if v := c.Query("lines"); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 2000 {
				lines = n
			}
		}
		data, err := os.ReadFile(ManagerLogPath)
		if err != nil {
			httpx.Success(c, gin.H{"log": ""})
			return
		}
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
			c.Error(errx.NotFound("插件不存在"))
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
		name := c.Param("id")
		var body map[string]string
		if err := c.ShouldBindJSON(&body); err != nil {
			c.Error(errx.BadRequest("请求体无效"))
			return
		}
		if err := m.ConfigSet(c.Request.Context(), name, body); err != nil {
			logManagerf(" 更新插件 %s 配置失败: %v", name, err)
			c.Error(err)
			return
		}
		logManagerf(" 更新插件 %s 配置（%d 项）", name, len(body))
		middleware.AuditSummary(c, "更新插件 "+name+" 配置")
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
}

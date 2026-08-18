# 网站详情 Drawer（多 Tab 属性卡片）· 实现计划

## 目标
网站列表点"详情" -> 右侧 Drawer 弹出，内含多 Tab，每个 Tab 一个 table/内容区，聚合网站运维所需的属性、日志、SSL、进程、配置、统计、健康等功能。列表保持简洁，详情承载丰富功能。

## 现状与缺口
**已有可复用**：
- `GET /web-servers/:id/websites/:wid/logs` - 日志 tail 原始文本（access/error/app）
- `GET /web-servers/:id/websites/:wid/process` - 进程状态（ProcessStatus：Status/PID/Uptime/CPUPercent/MemoryMB/Restarts/LastError，字段够用）
- `POST /ssl`（certbot 申请）、`POST /ssl/upload`（上传证书路径）
- `POST /build`、`/process/start`、`/process/stop`、`/enable`、`/disable`

**缺口（需新增）**：
- 列表/详情不返回运行时版本名（只有 `runtime_version_id`）-> join `runtime_versions`
- 无读 SSL 证书详情（颁发者/到期日/序列号/域名）-> 新增 `GET /ssl`
- 无读网站 Nginx 配置文件内容 -> 新增 `GET /config`
- 日志是原始文本，无结构化解析 -> 新增解析接口（或前端解析）
- 无访问统计、健康探活 -> 新增

## UI 设计

### 容器：Drawer（右侧滑出，宽 70%）
- 触发：`WebsiteList` 操作列加"详情"按钮（或点行）。
- 头部：网站名 + 域名（可点访问）+ 状态徽章组（网站启停 / 进程 / SSL 到期 / 健康）。
- 内部：`Tabs`，8 个 Tab。

### Tabs（统一实现，全做）
| Tab | 内容（table 形式） | 数据源 / 接口 |
|-----|---------------------|---------------|
| 1 概览 | 属性键值 table：域名/监听端口/应用端口/项目类型+运行时版本/根目录/SSL状态/进程状态/创建更新时间/build+start 命令 | `GET /websites/:wid`（增强 join 版本名） |
| 2 访问日志 | 解析后 table（时间/IP/方法/路径/状态码/字节/UA）+ 原始文本切换 | `GET /logs?type=access` + 新增解析 |
| 3 错误日志 | 同上（type=error） | `GET /logs?type=error` |
| 4 SSL 证书 | 证书信息 table：Subject/Issuer/NotBefore/NotAfter/到期天数/序列号/DNSNames/SigAlg + 操作（certbot 申请/上传/重载） | **新增 `GET /ssl`** + 现有 POST |
| 5 进程 | 进程 table：状态/PID/Uptime/CPU/内存/重启次数/最后启动/最后错误 + 操作（启停/重启/编译） | `GET /process` + 现有 POST |
| 6 Nginx 配置 | 生成的配置文件内容（只读预览）+ 自定义片段编辑（CustomConfig）+ 语法测试/重载 | **新增 `GET /config`** + 现有 web-server test/reload |
| 7 访问统计 | table：总访问量/流量/状态码分布(2xx/3xx/4xx/5xx)/Top IP/Top 路径 | **新增 `GET /stats`**（解析访问日志） |
| 8 健康探活 | 探活记录 table：时间/状态码/耗时(ms) + 立即探活按钮 | **新增 `GET /health` + `POST /health/probe`** |

## 后端接口新增/增强清单

### 1. 列表/详情 join 运行时版本名（增强）
- `repo_sqlite_website.go` 的 List/Get 查询 join `runtime_versions`，返回 `runtime_lang`+`runtime_exact`（参照 `process` 包的 join 模式）。
- `Website` 结构加 `RuntimeLang string` + `RuntimeExact string`（json 只读）。

### 2. `GET /web-servers/:id/websites/:wid/ssl`（新增）
- 读 `w.SSLCertPath`，`os.ReadFile` + `x509.ParseCertificate`。
- 返回：`{enabled, subject, issuer, not_before, not_after, days_remaining, serial, dns_names, sig_algo, cert_path, key_path}`。
- 未启用证书 -> `{enabled:false}`。
- `WebsiteService.GetSSL(ctx, webServerID, id) (*SSLCertInfo, error)`。

### 3. `GET /web-servers/:id/websites/:wid/config`（新增）
- 读网站 Nginx 配置文件内容（`writeConfigForServer` 生成的文件路径，需确认路径规则）。
- 返回：`{config: string, custom_config: string, config_options: string}`。
- `WebsiteService.GetConfig(ctx, webServerID, id) (*WebsiteConfigInfo, error)`。

### 4. `GET /web-servers/:id/websites/:wid/logs/parse`（新增，结构化日志）
- 参数：`type=access|error`、`lines`（默认 500）。
- 后端解析 nginx 日志行（正则）为 `[]LogEntry{time, ip, method, path, status, bytes, ua}`。
- 兼容解析失败行（原始行放入 raw 字段）。
- `WebsiteService.GetParsedLogs(ctx, webServerID, id, logType string, lines int) ([]LogEntry, error)`。

### 5. `GET /web-servers/:id/websites/:wid/stats`（新增，访问统计）
- 解析访问日志（最近 N 行或全部），统计：总请求/总流量/状态码分布/Top10 IP/Top10 路径。
- 返回：`{total_requests, total_bytes, status_dist{2xx,3xx,4xx,5xx}, top_ips[], top_paths[]}`。
- `WebsiteService.GetStats(ctx, webServerID, id) (*WebsiteStats, error)`。

### 6. `GET /web-servers/:id/websites/:wid/health` + `POST /.../health/probe`（新增）
- `GET`：返回最近探活记录（内存缓存最近 20 条，或简单返回当前探活结果）。
- `POST probe`：后端 `http.Get("http://domain:port")` 探活，返回 `{status_code, latency_ms, ok, error}`，记录到内存历史。
- `WebsiteService.ProbeHealth(ctx, webServerID, id) (*HealthResult, error)`。
- 超时 5s，不跟随重定向（或跟随）。

## 前端组件结构

```
web/src/pages/Website/
  WebsiteList.tsx              # 操作列加"详情"按钮 -> 打开 Drawer
  DetailDrawer/
    index.tsx                  # Drawer + Tabs 容器，接收 website + webServerId
    OverviewTab.tsx            # 概览属性 table
    LogsTab.tsx                # 日志（access/error 切换 + 解析/原始切换）
    SSLTab.tsx                 # SSL 证书 table + 操作
    ProcessTab.tsx             # 进程 table + 操作
    ConfigTab.tsx              # Nginx 配置预览 + 自定义编辑 + 测试/重载
    StatsTab.tsx               # 访问统计 table + 简单图表
    HealthTab.tsx              # 探活记录 table + 立即探活
  types.ts                     # 加 SSLCertInfo/LogEntry/WebsiteStats/HealthResult 类型
```
- `api.ts` 加：`getWebsiteSSL`、`getWebsiteConfig`、`getWebsiteParsedLogs`、`getWebsiteStats`、`getWebsiteHealth`、`probeWebsiteHealth`。
- 复用现有 `getWebsiteLogs`、`getWebsiteProcessStatus`、`applyWebsiteSSL`、`uploadWebsiteSSL`、`buildWebsite`、`startWebsiteProcess`、`stopWebsiteProcess`。

## 实现步骤（下次执行）

### 后端
1. `model.go`：Website 加 `RuntimeLang`/`RuntimeExact`；新增 `SSLCertInfo`/`LogEntry`/`WebsiteStats`/`HealthResult` 结构。
2. `repo_sqlite_website.go`：List/Get join runtime_versions（参照 process 包）。
3. `website.go`：新增 `GetSSL`/`GetConfig`/`GetParsedLogs`/`GetStats`/`ProbeHealth` + `GetHealthHistory` 方法。
4. `web_server.go`：新增对应 handler + `webserver.go` 注册路由。
5. gofmt + go build（Linux 部署编译）。

### 前端
6. `types.ts`：加新类型。
7. `api.ts`：加新接口封装。
8. `DetailDrawer/` 各 Tab 组件。
9. `WebsiteList.tsx`：操作列加"详情"按钮，打开 Drawer。
10. tsc + eslint + vite build。

### 验证
11. 部署，手测各 Tab：概览属性全、日志解析+原始、SSL 证书详情+到期预警、进程信息+操作、配置预览+测试、统计图表、探活记录。

## 关键设计点
- **日志解析**：后端正则解析 nginx 默认日志格式，失败行降级为 raw。前端可切换"解析 table / 原始文本"。
- **SSL 到期预警**：`days_remaining <14` 黄、`<7` 红、`<0` 灰(已过期)，Drawer 头部徽章也显示。
- **配置文件路径**：需确认 `writeConfigForServer` 生成的配置文件路径规则（如 `/etc/nginx/conf.d/<domain>.conf`），`GetConfig` 读该文件。
- **统计/探活简化**：v1 统计基于解析访问日志（最近 N 行），探活内存缓存历史（进程重启丢失，可接受）；不做持久化统计（避免引入时序存储）。
- **Drawer 状态**：打开时加载概览，各 Tab 懒加载（切到才请求），关闭销毁。

## 不做（防范围蔓延）
- 不改网站列表表格本身（只加"详情"按钮）。
- 不做日志实时流（WebSocket tail）--先拉取最近 N 行，实时流后续。
- 不做持久化访问统计（时序数据库）--基于实时日志解析。
- 不做卡片/表格视图切换--列表保持表格，详情用 Drawer。

## 提交
- `feat(web): 网站详情 Drawer 多 Tab（概览/日志/SSL/进程/配置/统计/健康）`

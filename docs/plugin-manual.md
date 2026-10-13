# EasyServer 插件开发手册

> 面向插件开发者的官方文档。面板内「插件管理 → 插件开发手册」提供精简版并链接到本手册；
> 宣传网站 [www.0xdead.icu/plugin-manual](https://www.0xdead.icu/plugin-manual) 渲染同一份内容。
> 源码契约：`internal/plugin/contract.go`（manifest/权限/贡献点）、`internal/plugin/host.go`（Host API）、
> `sdk/plugin-sdk-go/`（Go SDK）、`web/public/host-sdk.js`（前端 Host SDK）。

EasyServer 的插件是**独立进程**（任意语言的可执行文件），经 Unix socket 与面板通信。
崩溃、阻塞、内存泄漏都不影响主面板。插件能力**默认拒绝**：manifest 声明、管理员批准后才可用，
未声明的能力一律拒绝。

---

## 一、插件目录结构

插件是一个目录，放入面板的 `/opt/easyserver/plugins/<插件名>/`，或通过面板「插件管理 → 导入插件」上传 zip 包。

```
my-plugin/            ← 目录名须与 plugin.yaml 的 name 一致（小写字母/数字/横线）
├── plugin.yaml       ← 清单（必填）
├── my-plugin         ← 可执行文件（任意语言，须有执行权限）
└── web/              ← 可选：插件前端页面/样式资源（经插件自身 HTTP 服务返回）
```

安装后点「启用」才启动进程；启用时管理员审批权限（默认审批声明的全部权限，也可只批子集）。

## 二、plugin.yaml 清单

```yaml
api_version: 1                # 面板支持的协议版本（当前 1）
manifest_schema_version: 1
name: my-plugin               # 仅小写字母/数字/横线，≤64 字符
version: 0.1.0
author: 你的名字              # 可选
description: 插件说明

runtime:
  exec: my-plugin             # 可执行文件（插件目录内相对路径，必填）
  args: []                    # 可选：附加参数
  env: {}                     # 可选：附加环境变量
  socket: my-plugin.sock      # 插件 socket 名（面板在 /run/easyserver/plugins/ 下探测）
  startup_timeout: 15s        # 默认 30s
  request_timeout: 30s        # 默认 30s

health:
  path: /health               # 健康检查端点（插件 HTTP 服务提供），默认 /health
  interval: 30s               # 默认 30s
  fail_threshold: 3           # 连续失败 N 次重启，默认 3

permissions:                  # 申请的能力（默认拒绝，按需声明，见第七节）
  - config.readwrite
  - log.write
  - audit.write
  # - exec.restricted         # 等同 root 执行，慎用
  # - firewall.manage         # 暴露端口访问控制（PortACL）

contributions:                # 向面板扩展点贡献的内容（全部可选，见第五节）
  menus:
    - { id: main, title: 我的插件, icon: rocket, path: /plugins/my-plugin, order: 150 }
  pages:
    - { id: main, path: /plugins/my-plugin }
  routes:
    - { id: api, prefix: /api/plugins/my-plugin }
  widgets:
    - { id: stat, title: 统计, slot: dashboard, path: /plugins/my-plugin/widget, size: md }
  settings:
    - { id: default, title: 设置, config_schema: { type: object, properties: {} } }
  services:
    - { id: sync, schedule: "@every 10m", command: "sync-data" }
  eventsubs:
    - { event: easyserver.auth.login }
  eventpubs:
    - { event: easyserver.my-plugin.done }
  styles:
    - { id: brand, path: styles/brand.css }
  web_servers:
    - { name: lighttpd, display_name: Lighttpd, description: ..., install_cmd: ..., service_name: lighttpd }
  commands:
    - { id: restart, title: 重启 }
  datasources:
    - { id: status, title: 运行状态, kind: json }
```

## 三、生命周期与状态

```
registered → (启用) → starting → (健康检查通过) → running
                                         ↘ (失败) → degraded / error → 自动重启
```

- 面板扫描注册 `registered` → 启用后拉起进程 `starting` → 健康检查通过 `running`。
- 进程崩溃自动退避重启（1s→60s，10 分钟窗口 >5 次转 `disabled`）；面板重启时对已启用插件自动重放。
- 面板注入环境变量：

  | 变量 | 说明 |
  |---|---|
  | `EASYSERVER_PLUGIN_TOKEN` | Host API 鉴权 token（secret） |
  | `EASYSERVER_HOST_SOCKET` | 面板 Host API socket 路径（默认 `/run/easyserver/host.sock`） |
  | `EASYSERVER_PLUGIN_ID` | 插件名 |
  | `EASYSERVER_PLUGIN_SOCKET` | 插件自身应监听的 socket 绝对路径 |

## 四、插件 HTTP 服务（必做）

插件必须提供 HTTP 服务，监听 `EASYSERVER_PLUGIN_SOCKET` 指定的 Unix socket，至少实现：

- `GET /health` — 健康检查（非 5xx 即健康）。
- `POST /events` — 接收面板事件（可选，订阅了 `eventsubs` 时需要，见 events_bridge）。
- 其余业务路由自由定义，经 `/api/plugins/<插件名>/*` 由面板反向代理给前端访问。

## 五、贡献点（12 种）

面板向插件开放**功能补充型**扩展点。核心面板操作（认证、防火墙、网站托管核心等）不开放给插件。

| 贡献点 | 作用 | 面板消费方式 |
|---|---|---|
| `menus` | 侧边栏菜单项 | `GET /plugins/menus` |
| `pages` | 插件前端页面（iframe 挂载） | `/plugins/<name>` |
| `routes` | 声明插件代理路由前缀 | 面板代理 `/api/plugins/<name>/*` |
| `widgets` | 仪表盘小部件 | `GET /plugins/widgets` |
| `settings` | 配置表单（JSON Schema 驱动渲染） | `GET/PUT /plugins/<name>/config` |
| `services` | 定时任务（cron 表达式） | 面板 robfig/cron 调度，POST `/services/<id>/run` |
| `eventsubs` | 订阅面板业务事件 | 面板事件总线投递到 `POST /events` |
| `eventpubs` | 声明插件可发布的事件 | `POST /api/host/event/publish` |
| `styles` | 面板前端样式注入（品牌定制/换肤） | `GET /plugins/styles`，面板注入 `<link>` |
| `web_servers` | 注册新的 Web 服务器类型（如 Lighttpd） | `GET /plugins/web-servers`，并入网站管理 |
| `commands` | 可被调用的动作（跨插件协作） | `GET /plugins/commands`，`POST /api/host/command/invoke` |
| `datasources` | 对外提供数据源（json/text/metrics） | `GET /plugins/datasources`，经插件代理消费 |

### 5.1 styles（样式注入）

插件提供 CSS 资源，面板在自身样式之后注入 `<link>`，随插件启停生效：

```yaml
styles:
  - { id: brand, path: styles/brand.css }   # 插件内相对路径
```

插件 HTTP 服务提供该资源（如 `GET /styles/brand.css`），面板经 `/api/plugins/<name>/styles/brand.css` 代理获取。

### 5.2 web_servers（Web 服务器类型扩展）

插件注册新的服务器类型，面板网站管理把其并入可选类型并提供安装/卸载/配置管理：

```yaml
web_servers:
  - name: lighttpd                  # 类型名（小写字母/数字/横线）
    display_name: Lighttpd
    description: 轻量级 Web 服务器
    install_cmd: apt-get install -y lighttpd
    uninstall_cmd: apt-get remove -y lighttpd
    config_path: /etc/lighttpd
    config_file: lighttpd.conf
    sites_available: /etc/lighttpd/conf-available
    sites_enabled: /etc/lighttpd/conf-enabled
    service_name: lighttpd          # systemd 单元名
    binary_path: /usr/sbin/lighttpd
    default_port: 80
    log_dir: /var/log/lighttpd
    test_cmd: lighttpd -t -f {config}   # 可选
    version_cmd: lighttpd -v            # 可选
```

### 5.3 commands（可调用命令）

插件声明命令，面板或其他插件可经面板 Host API `command/invoke` 调用。插件需在自身 socket 提供 `/commands/<id>` 端点（POST）：

```yaml
commands:
  - { id: restart, title: 重启服务 }
```

其他插件调用（需 `command.invoke` 权限）：
```go
err := hc.InvokeCommand(ctx, "tunnel-frp", "restart", nil, nil)
```

### 5.4 datasources（数据源）

插件声明对外提供的数据源（如监控指标、状态快照），经面板聚合下发、代理消费：

```yaml
datasources:
  - { id: status, title: 运行状态, kind: json }   # kind: json / text / metrics
```

插件在自身 socket 提供 `/datasources/<id>` 端点返回数据；面板 `GET /plugins/datasources` 列出全部，
经 `/api/plugins/<name>/datasources/<id>` 代理获取。

### 5.5 services（定时任务）

插件声明定时任务（robfig/cron 表达式），由面板统一调度，向插件 socket `POST /services/<id>/run`：

```yaml
services:
  - { id: sync, schedule: "@every 10m", command: "sync-data" }
```

## 六、Go SDK 快速开始

SDK 独立模块 `sdk/plugin-sdk-go`（仓库内），封装 socket 服务、事件接收与 Host API 客户端：

```go
package main

import (
  "net/http"
  sdk "github.com/lmy8848/easyserver-plugin-sdk-go"
)

func main() {
  srv := sdk.NewServer(sdk.Config{SocketPath: "my-plugin.sock"})
  srv.HandleFunc("/api/hello", func(w http.ResponseWriter, r *http.Request) {
    w.Write([]byte("hello from plugin"))
  })
  srv.OnEvent(func(ctx context.Context, e *sdk.Event) {
    // e.Name / e.Payload —— 面板事件（处理需幂等）
  })
  go srv.Serve()

  hc := sdk.NewHostClient()      // 读面板注入的环境变量
  hc.Audit(ctx, "插件启动")
  hc.Notify(ctx, "标题", "内容")     // 需 notify.send
  hc.ConfigGet(ctx, "key")          // 需 config.read / config.readwrite
  hc.Exec(ctx, "ls", []string{"-la"}, 10)  // 需 exec.restricted，有审计+超时
  select {}
}
```

## 七、Host API 能力清单与权限

插件凭 `EASYSERVER_PLUGIN_TOKEN`（Authorization: Bearer）调面板 host socket（`/run/easyserver/host.sock`）。

**权限常量（11 种）**：

| 权限 | 说明 |
|---|---|
| `audit.write` | 写入审计日志 |
| `config.read` | 读取配置 KV（只读） |
| `config.readwrite` | 读写配置 KV |
| `log.write` | 写入插件日志（`/var/log/easyserver/plugins/<name>.log`） |
| `event.publish` | 发布事件（事件名须以 `easyserver.<插件名>.` 为域） |
| `event.subscribe` | 订阅面板事件 |
| `exec.restricted` | 有审计+超时的 root 执行（等同 root shell，慎用） |
| `notify.send` | 发送面板通知 |
| `panel.api.proxy` | 只读代理面板 API（监控等只读端点白名单） |
| `storage.readwrite` | 插件私有文件区（`/var/lib/easyserver/plugins/<name>/`） |
| `firewall.manage` | 暴露端口访问控制（PortACL 授权源白名单） |
| `command.invoke` | 调用其他插件声明的 command |

**Host API 端点**：

| 权限 | 方法/路径 |
|---|---|
| `audit.write` | `POST /api/host/audit` |
| `config.read` / `config.readwrite` | `GET /api/host/config`、`GET /api/host/config/:key` |
| `config.readwrite` | `PUT /api/host/config` |
| `log.write` | `POST /api/host/log` |
| `event.publish` | `POST /api/host/event/publish` |
| `exec.restricted` | `POST /api/host/exec` |
| `notify.send` | `POST /api/host/notify` |
| `storage.readwrite` | `GET/PUT/DELETE /api/host/storage/:path` |
| `firewall.manage` | `POST /api/host/port/expose`、`POST /api/host/port/revoke`、`GET /api/host/port/list` |
| `panel.api.proxy` | `POST /api/host/panel/api`（只读白名单代理） |
| `command.invoke` | `POST /api/host/command/invoke` |

## 八、前端页面（iframe + Host SDK）

插件贡献的 `pages` 会以 iframe 挂载在 `/plugins/<插件名>`。插件页面在 HTML 引入
`<script src="/host-sdk.js"></script>`，通过 `window.EasyServerPlugin` 与面板通信：

```html
<script src="/host-sdk.js"></script>
<script>
  const s = window.EasyServerPlugin;
  s.request('/api/plugins/my-plugin/api/status')   // 受限 GET（带面板登录态）
    .then(res => { /* 处理 */ });
  s.navigate('/settings');                          // 面板内导航
  s.notify('标题', '内容');                         // 面板通知
</script>
```

iframe 使用 `sandbox="allow-scripts allow-forms allow-same-origin"`（不含 allow-top-navigation），
插件前端**不持有**面板 token——需要权限的操作经主前端代理执行。

## 九、打包与安装

把插件目录打成 zip（顶层直接是 plugin.yaml，不要套一层目录），面板「导入插件」上传即可；
或手动放入插件目录后重启面板。

```bash
cd my-plugin && zip -r ../my-plugin.zip .   # 顶层含 plugin.yaml
# 面板 → 插件管理 → 导入插件 → 选 my-plugin.zip
# 安装后点「启用」启动进程
```

## 十、安全模型

- **独立进程隔离**：插件崩溃/慢/泄漏不拖垮主面板；进程级重启规避 frp 等进程内端口泄漏。
- **权限默认拒绝**：manifest 声明、管理员批准、token 绑定 scope；未声明能力一律 403。
- **路径穿越防护**：存储/配置/样式路径全部规范化 + 前缀校验。
- **暴露端口访问控制**：`firewall.manage` 经 PortACL（nftables 授权源白名单）管控。
- **只读面板代理**：`panel.api.proxy` 仅放行只读端点白名单。

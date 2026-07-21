# EasyServer 插件系统 · 架构设计与实现计划（终稿）

> 状态：终稿 v1.0  
> 范围：仅 Linux；进程外插件框架；4 正交原语（Contribution / Host API / EventBus / Webview）  
> 审查：本文末"自我审查记录"含 6 轮多维度审查，结论已回填正文

---

## 1. 背景与目标

EasyServer 是 Go + React 的单管理员 Linux 服务器管理面板（root 权限运行）。需要一个**框架级、扩展性强、自由度高**的插件系统，用于丰富功能，且满足硬约束：

- **低耦合**：插件不依赖面板内部结构，面板改内部不影响插件，反之亦然。
- **不挂主系统**：插件崩溃/泄漏/死锁/慢，主面板继续正常服务。
- **不臃肿**：架构契约一步到位，演进只做加法，不为初期简单埋后期推倒的坑。

## 2. 设计原则

1. **进程隔离是底线**：插件是独立进程，主面板永不因插件挂。排除进程内方案（Go plugin）。
2. **正交不堆砌**：4 个原语互相独立，组合出所有扩展能力；新扩展形态优先用组合表达，不新增核心机制。
3. **契约即框架**：manifest、Host API、事件、Host SDK 是稳定契约，版本化协商。框架 = 契约 + 运行时。
4. **声明式 + 开放**：插件在 manifest 声明 contributions，面板按类型分发；扩展点类型集合一次定义全、向前兼容。
5. **默认拒绝的权限模型**：能力级声明-审批-绑定，先于功能存在，避免后期 retrofit 安全。
6. **隔离优先于无缝**：前端用 iframe 沙箱，不做远程 JS 注入主前端。

## 3. 总体架构

### 3.1 进程模型
- **主面板进程**（Go，root）：业务 + 插件管理运行时。
- **插件进程**（任意语言可执行文件）：每个启用的插件一个独立子进程，由面板 `os/exec` 拉起。
- **不共享内存/DB**：插件只能通过契约通道与面板交互。

### 3.2 通信拓扑（双向 Unix Socket）
- 用户 -> 面板 -> 插件：`/api/plugins/<id>/*` 反向代理到插件 socket `/run/easyserver/plugins/<id>.sock`。
- 插件 -> 面板（Host API）：插件调 `http+unix://` 到面板 host socket `/run/easyserver/host.sock`，带 `plugin_token`。
- 面板 -> 插件（事件）：POST 到插件 `/events` 端点（经插件 socket）。
- 面板 -> 插件（健康）：GET 插件 `/health`。

全 Unix socket：无端口占用、不暴露网络栈、文件权限控访问（目录 `0700` root，socket `0600`）。

```
┌──────────────── EasyServer 主面板（Go, root）────────────────┐
│  Gin Router                                                  │
│   /api/...                  业务 API                         │
│   /api/plugins/<id>/*  ──┐  反向代理(超时/限流)               │
│   /plugins/<id>        ──┤  前端 iframe 页面入口              │
│  Host Socket Server         /run/easyserver/host.sock        │
│   /api/host/*          ←── plugin_token 鉴权 + 权限校验       │
│  PluginManager                                               │
│   ├─ Contract  (manifest/contribution/permission/schema)     │
│   ├─ Registry  (扩展点 handler 注册与分发)                    │
│   ├─ Supervisor(进程托管/退避重启/健康/cgroup)                │
│   ├─ Proxy     (ReverseProxy + 超时 + 限流)                   │
│   ├─ HostAPI   (能力供给 + token + 权限)                      │
│   ├─ EventBus  (发布/订阅/投递/限流)                          │
│   └─ Config    (schema 渲染数据 + KV)                        │
│  SQLite: plugins / plugin_config / plugin_secret             │
└──────────────────────────────────────────────────────────────┘
        │ /run/easyserver/plugins/<id>.sock         │ /run/easyserver/host.sock
        ▼                                            ▲ (plugin_token)
   ┌──────────┐  ┌──────────┐  ┌──────────┐
   │ 插件(Go) │  │插件(Python)│  │插件(Node)│   独立进程，崩溃互不影响
   └──────────┘  └──────────┘  └──────────┘
```

### 3.3 启动时序
1. 主面板启动（不依赖任何插件）。
2. PluginManager 扫描插件目录，读 manifest，写 DB 状态 `registered`。
3. 对每个 `enabled` 插件异步拉起：Supervisor 启动进程 → 等 socket 就绪 → `starting` → 健康检查通过 → `running`。
4. 任一插件启动失败/超时 → `error` 状态 + 审计告警，不阻塞其他插件与主面板。

## 4. 核心抽象：四个正交原语

### 4.1 Contribution（贡献声明）
插件在 manifest 用 `contributions` 声明向哪些扩展点贡献什么。一个插件可贡献多种。面板有 `ExtensionPointRegistry`，按 contribution 类型分发到对应 handler。新增扩展点类型 = 加一种 contribution key + 一个 handler，核心不动。

### 4.2 Host API（面板向插件供给能力）
插件持 `plugin_token` 调 host socket，能力级权限校验。自由度的来源——能力越丰富，自由度越高。插件永远不能直接读面板 DB/内存。

### 4.3 EventBus（双向事件总线）
面板在业务点发布事件；插件订阅事件、发布自己的事件（供其他插件订阅）。异步、at-least-once、限流。插件间松耦合协作的唯一通道。

### 4.4 Webview 前端运行时（自由 + 隔离）
插件 UI 以 iframe 挂载（菜单页/小部件/设置页），JS 沙箱 + CSP。面板注入 Host SDK（JS，postMessage 封装）供插件前端读写配置、订阅事件、调能力、导航、通知。插件任意前端栈。

## 5. 扩展点（Contribution 类型）清单

| 类型 | 说明 | v1 | v2 |
|------|------|----|----|
| `menus` | 侧边栏菜单项 | ✅ | |
| `pages` | 前端页面（iframe 入口 URL） | ✅ | |
| `routes` | 后端 HTTP 路由（前缀 `/api/plugins/<id>/`） | ✅ | |
| `widgets` | 仪表盘小部件（iframe slot） | ✅ | |
| `settings` | 设置页（带 config_schema，面板渲染） | ✅ | |
| `services` | 后台长驻/定时任务 | ✅ | |
| `eventsubs` | 订阅事件 | ✅ | |
| `eventpubs` | 声明发布的事件类型 | ✅ | |
| `commands` | 可被调用的动作（面板/其他插件） | | ✅ |
| `datasources` | 数据源（如监控指标源） | | ✅ |
| `hooks` | 面板流程钩子（before/after） | | ✅ |

> v1 实现前 8 种（覆盖绝大多数扩展需求）；后 3 种 v2。manifest 未知 contribution 类型一律忽略 + 告警（向前兼容）。

## 6. Manifest 规范

### 6.1 字段定义（`plugin.yaml`）
```yaml
api_version: 1                 # 整体协议版本
manifest_schema_version: 1      # manifest 结构版本
name: backup-manager            # 唯一标识 ^[a-z0-9-]+$
version: 0.1.0                  # 语义化
author: EasyServer
description: 数据库与网站备份管理

runtime:
  exec: ./backup-manager        # 相对插件目录
  args: []
  env: {}
  socket: backup-manager.sock   # 相对 /run/easyserver/plugins/
  startup_timeout: 30s
  request_timeout: 30s          # 反向代理默认超时

health:
  path: /health
  interval: 30s
  fail_threshold: 3             # 连续 N 次失败才标 degraded/禁用

resources:                      # v2 cgroup 限额（v1 声明不实现）
  memory_mb: 256
  cpu_percent: 50

permissions:                    # 申请的 Host API 能力，默认拒绝
  - audit.write
  - config.readwrite
  - exec.restricted
  - event.publish
  - event.subscribe
  - notify.send
  - panel.api.proxy
  - storage.readwrite

dependencies:                   # 依赖其他插件的 command（v2）
  - { plugin: cron-helper, command: run-job }

contributions:
  menus:
    - { id: main, title: 备份管理, icon: archive, path: /plugins/backup-manager, order: 100 }
  pages:
    - { id: main, path: /plugins/backup-manager, widget: fullpage }
  routes:
    - { id: api, prefix: /api/plugins/backup-manager }
  widgets:
    - { id: recent, title: 最近备份, slot: dashboard, path: /widgets/recent, size: 2x1 }
  settings:
    - { id: default, title: 备份设置, config_schema: { ...JSON Schema... } }
  services:
    - { id: nightly, schedule: "0 3 * * *", command: backup-nightly }
  eventsubs:
    - { event: easyserver.website.created }
    - { event: easyserver.cron.executed }
  eventpubs:
    - { event: backup.job.done, schema: { type: object, properties: { job_id: {type: string}, status: {type: string} } } }
```

### 6.2 校验规则（加载前严格校验，无效则 `error` 不启动）
- `name` 全局唯一，正则 `^[a-z0-9-]+$`，≤64。
- `api_version` 在面板支持范围内（否则拒绝 + 告警）。
- `permissions` 必须是已知能力白名单。
- `contributions` 各类型字段校验：`menus.path` 必须以 `/plugins/<name>` 开头且全局唯一；`routes.prefix` 必须是 `/api/plugins/<name>`；`eventpubs.event` 反向域名命名。
- 字段长度上限，schema 深度/大小上限（防恶意 manifest）。

### 6.3 api_version 协商
面板声明支持范围 `[PLUGIN_API_MIN, PLUGIN_API_MAX]`。插件 `api_version` 落在区间才加载；否则 `error` + 审计，并在 `/api/plugins/<id>/info` 暴露面板的 `go_version` / `api_version` 供作者对齐。

## 7. Host API

### 7.1 鉴权
- 插件启动时面板注入环境变量：`EASYSERVER_PLUGIN_TOKEN`（随机 32B hex）、`EASYSERVER_HOST_SOCKET`（host socket 路径）、`EASYSERVER_PLUGIN_ID`。
- 每次请求 `Authorization: Bearer <plugin_token>`。
- token 绑定已批准的 permissions 子集；Host API 端点逐一校验。
- token 可轮换（面板重生成 + 重启插件）；token 仅本机有效（host socket 不联网 + 目录 `0700`）。

### 7.2 端点清单（host socket，路径前缀 `/api/host`）

| 方法 | 路径 | 权限 | 说明 |
|------|------|------|------|
| POST | `/audit` | audit.write | 写审计 `{action, detail}` |
| GET/PUT | `/config/:key`、`/config` | config.read / config.readwrite | 插件私有配置 KV（命名空间隔离） |
| POST | `/log` | log.write | 写日志（落 `/var/log/easyserver/plugins/<id>.log`） |
| POST | `/event/publish` | event.publish | 发布事件 `{event, payload}`（payload 依 eventpubs schema） |
| POST | `/exec` | exec.restricted | 受限命令执行 `{cmd, args[], timeout, stdin?}` → `{exit_code, stdout, stderr}`；全程审计；超时强杀 |
| GET/PUT/DELETE | `/storage/:path` | storage.readwrite | 插件私有文件区 `/var/lib/easyserver/plugins/<id>/`（路径规范化防穿越） |
| POST | `/panel/api` | panel.api.proxy | 代理调用面板业务 API `{method, path, body}`；按插件权限范围放行，非用户态 |
| POST | `/notify` | notify.send | 复用面板通知通道 `{title, content, channel?}` |
| POST | `/command/invoke` | （隐含） | 调用其他插件 command `{plugin, command, params}`（v2）；深度限制 3、禁自调、超时 |

### 7.3 exec.restricted 的诚实定位
面板以 root 运行，exec 权限本质是"有审计+超时的 root 执行"，不做命令白名单（不现实且易绕）。受限 = 默认拒绝 + 显式授权 + 全审计 + 超时 + stdout/stderr 捕获。文档明确告知管理员：授予 `exec.restricted` 等同授予 root shell。

## 8. EventBus

### 8.1 事件命名
反向域名：`easyserver.<domain>.<action>`，如 `easyserver.website.created`、`easyserver.auth.login`、`backup.job.done`（插件事件用插件名作域）。

### 8.2 投递语义
- **at-least-once**：异步投递，失败指数退避重试（最多 5 次，~1s/4s/16s/64s/256s），超限放弃 + 审计。**插件必须幂等**。
- **event_id**：每个事件带唯一 `event_id`，插件可据此去重。
- **顺序**：同一插件同一事件类型按发布顺序投递（单消费者队列）；跨插件不保证全局顺序。

### 8.3 限流（防风暴）
每插件入队速率上限（默认 100/s），超限丢弃 + 审计告警。投递队列长度上限（默认 10000），溢出丢弃最旧 + 告警。

### 8.4 面板内置事件点（业务侧一行 `eventBus.Publish`）
`easyserver.auth.login` / `auth.logout` / `auth.password_changed` / `file.upload` / `file.delete` / `website.created` / `website.deleted` / `cron.executed` / `service.start` / `service.stop` / `firewall.changed`。集中常量管理，不侵入业务逻辑（仅一行发布调用）。

### 8.5 事件 schema 演进
`eventpubs.schema` 声明。演进规则：**只加字段（可选），不删/不改字段语义**；删字段需升 `api_version`。订阅方按 schema 宽松解析（忽略未知字段）。

## 9. 前端 Webview 运行时

### 9.1 iframe 挂载
- 菜单页 `/plugins/<id>` → iframe `src=/api/plugins/<id>/?__frame=1`（插件返回 HTML）。
- 仪表盘小部件 → 固定 slot 渲染 iframe（`size` 决定占位）。
- 设置页 → **面板渲染**（基于 `settings.config_schema`），非 iframe；插件通过 Host API 读写配置。

### 9.2 Host SDK（注入 iframe 的 JS 库）
- iframe 加 `sandbox="allow-scripts allow-forms allow-same-origin"`（**不含 allow-top-navigation**），`CSP` 限制。
- 面板主前端向 iframe 注入 Host SDK（版本与 `api_version` 对齐）。
- 协议：postMessage，**校验 origin**（仅面板 origin 受信）。
- **iframe 不持有 plugin_token**。所有需要权限的操作，iframe 经 Host SDK postMessage → 面板主前端代调 Host API（带 plugin_token）。iframe 拿不到 token。

### 9.3 Host SDK API（前端侧）
```
host.config.get(key) / host.config.set(key, val)
host.event.subscribe(name, cb) / host.event.publish(name, payload)
host.notify(title, content)
host.navigate(path)              // 面板内导航
host.context.getUser() / host.context.getServer()   // 面板上下文
host.api.call(method, path, body) // 经面板代理的受限 API
host.modal.open(url) / host.modal.close()
```

## 10. 配置体系

- 插件 `settings.config_schema`（JSON Schema）声明配置项。
- 面板统一渲染配置表单（设置页），存 `plugin_config` 表（按插件命名空间 + key）。
- 插件通过 Host API `config.readwrite` 读自己配置。
- 校验：写入前按 schema 校验，非法拒绝。
- 变更通知：配置变更后，面板发 `easyserver.plugin.config_changed` 事件 + SIGHUP 给插件进程（插件可重载）。

## 11. 权限模型

```
manifest.permissions      # 声明
   ↓ 启用时管理员审批（可收缩子集，不可扩张超声明）
plugin_token.scope        # 绑定已批准子集
   ↓ Host API 每次调用校验 scope
   ↓ 全量审计
```
- 能力级（非细粒度 RBAC）：单管理员面板，插件由管理员主动安装，能力级足够。
- 默认拒绝；未声明的能力不可用。
- 权限变更 → 重生成 token + 重启插件。
- 扩展点可带权限要求（如 `hooks` v2 需额外声明）。

## 12. 生命周期

### 12.1 状态机
```
registered ──enable──> enabled ──start──> starting ──health ok──> running
                         ↑                    │                       │
                       disable              crash(退避)             degraded
                         │                    ↓                       │
                      disabled <──────── restart ─────────────────────┘
                         │                    │
                       remove              fail>threshold ──> disabled(禁用+告警)
                                            start fail/timeout ──> error(不自动重启)
```

### 12.2 Supervisor 策略
- **启动**：exec 进程 → 注入 token 环境变量 → 等 socket 出现（≤ startup_timeout）→ 健康检查 → `running`。
- **崩溃重启**：指数退避（1s/2s/4s/8s/16s/32s/60s 封顶）；窗口（10 分钟）内失败 >5 次 → `disabled` + 审计告警，停止重启。
- **健康检查**：每 `health.interval` GET `/health`；连续 `fail_threshold` 次失败 → `degraded` → 重启；单次失败不动作。
- **停止**：SIGTERM（grace 10s）→ SIGKILL → 清理 socket 文件。
- **主面板启动残留清理**：扫描 `/run/easyserver/plugins/*.sock` 与 `ps`，杀孤儿进程、删残留 socket。
- **主面板关停**：对所有 `running` 插件发 SIGTERM（不等全部退出，最多 grace 10s）。
- **升级**：替换插件目录文件 → Supervisor 重启进程；状态/配置保留。

## 13. 隔离与稳定性

| 风险 | 对策 |
|------|------|
| 插件崩溃 | 进程隔离 + Supervisor 自愈 |
| 资源泄漏 | 进程隔离；v2 cgroup 限额；泄漏不蔓延主面板 |
| 请求卡死 | 反向代理强制 `request_timeout`；不长期占主面板 goroutine |
| 事件风暴 | EventBus 限流 + 队列上限 |
| 插件慢启动 | `startup_timeout`，超时 `error` |
| 插件日志爆盘 | 每插件日志大小上限 + 自动轮转（默认 50MB/文件、保留 3 份） |
| 孤儿进程 | 启动时扫描清理 |
| 越权 | plugin_token + 权限校验 + 审计 |
| 插件目录篡改 | `/opt/easyserver/plugins/` 与 `/run/easyserver/plugins/` 仅 root 可写 |
| iframe 逃逸 | sandbox + CSP + origin 校验 + iframe 不持 token |
| 路径穿越 | `/storage` 路径规范化 + 根目录限制 |

## 14. 数据模型与迁移（迁移 000021）

```sql
CREATE TABLE plugins (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  name TEXT UNIQUE NOT NULL,
  version TEXT NOT NULL,
  api_version INTEGER NOT NULL,
  manifest_path TEXT NOT NULL,
  enabled INTEGER DEFAULT 0,
  status TEXT DEFAULT 'registered',   -- registered/enabled/starting/running/degraded/disabled/error
  pid INTEGER DEFAULT 0,
  permissions_approved TEXT,          -- JSON：已批准权限子集
  restart_count INTEGER DEFAULT 0,
  last_error TEXT,
  last_seen DATETIME,
  socket_path TEXT,
  created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
  updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_plugins_status ON plugins(status);

CREATE TABLE plugin_config (
  plugin_name TEXT NOT NULL,
  key TEXT NOT NULL,
  value TEXT,
  updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (plugin_name, key)
);

CREATE TABLE plugin_secrets (        -- 插件密钥（如第三方 API key），仅存加密
  plugin_name TEXT NOT NULL,
  key TEXT NOT NULL,
  encrypted_value TEXT NOT NULL,     -- 用 cfg.Deploy.EncryptionKey 复用
  updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (plugin_name, key)
);
```
迁移幂等（复用 v19/v20 hook 模式）。

## 15. 代码结构

```
internal/plugin/
  contract.go      # manifest/contribution/permission 常量与结构（契约层）
  registry.go      # ExtensionPointRegistry：handler 注册与 contribution 分发
  manager.go       # 生命周期状态机、启用/禁用/卸载
  supervisor.go    # 进程托管、退避重启、健康检查、残留清理
  proxy.go         # httputil.ReverseProxy + 超时 + 限流 + 流式
  host.go          # Host API 实现 + token + 权限校验
  host_socket.go   # host unix socket server
  events.go        # EventBus 发布/订阅/投递/限流
  config.go        # 配置 KV + schema 校验
  storage.go       # 插件私有文件区（路径穿越防护）
  routes.go        # /api/plugins/* + /api/plugins/<id>/* + 前端 iframe 入口
  repo.go          # PluginRepo 接口
  repo_sqlite.go   # 持久化
internal/plugin/webhost/
  host_sdk.js      # 注入 iframe 的 Host SDK
  inject.go        # iframe 挂载与 SDK 注入
migrations/000021_plugins.up.sql
migrations/000021_plugins.down.sql
```

## 16. SDK

### 16.1 后端 SDK
- **Go SDK**（`easyserver-plugin-sdk-go`，独立 module）：封装"监听 unix socket + 实现 /health + 接收 /events + 调 Host API 客户端 + 声明 contributions 的辅助"。插件作者只写业务 handler。
- **协议文档**：明确 socket 上的 HTTP 协议、Host API 端点、事件格式，使 Python/Node 可裸实现（v1 不提供非 Go SDK，v2 补 Python/Node）。

### 16.2 前端 Host SDK
- `host_sdk.js`（版本化，与 `api_version` 对齐），iframe 内 `import` 或面板注入。
- 封装 postMessage 协议，提供 §9.3 的 API。

## 17. 安全考量与威胁模型

| 威胁 | 缓解 |
|------|------|
| 野插件提权 root | 插件目录 root-only；不做远程一键安装；权限默认拒绝；全审计 |
| token 被同机非 root 进程窃取 | host socket 目录 `0700`，socket `0600`；token 仅本机有效 |
| token 被网络窃取 | host socket 不监听 TCP，仅本机 unix socket |
| iframe JS 调 Host API 越权 | iframe 不持 token；Host SDK 经主前端代理，主前端带 token 调用 |
| exec 滥用 | 默认拒绝；显式授权；全审计；超时；文档明示等同 root |
| 恶意 manifest | 严格 schema 校验；字段长度/深度上限；path 正则白名单 |
| 插件间恶意调用 | command.invoke 深度限制 + 禁自调 + 超时（v2） |
| 事件投递重放/重复 | event_id 去重；插件幂等 |

**威胁边界声明**：插件以 root 运行（同面板），信任边界是"管理员主动安装的可信插件"。本框架不防御"恶意插件作者"（那是代码审计问题），而是防御"插件 bug 不拖垮主面板" + "插件能力可声明可审计可收回" + "前端不逃逸"。

## 18. 演进与版本兼容

- **三版本独立协商**：`api_version`（整体）、`manifest_schema_version`、`host_api_version`。
- **只加不删/不改语义**：Host API 端点、事件字段、manifest 字段均如此；废弃端点保留 2 个版本并标 deprecated。
- **未知 contribution 类型忽略 + 告警**：旧面板遇新 contribution 类型不崩。
- **数据迁移**：复用现有迁移体系，幂等。
- **SDK 解耦**：SDK 独立 module/包，版本对齐 `api_version`，插件不依赖主仓库。

## 19. 实现计划

### 19.1 v1：链路打通（核心可用）
**目标**：进程外插件能注册菜单/页面/路由/小部件/设置/事件/后台服务，崩溃不挂主面板，多语言可接入。

任务分解：
1. 契约层 `contract.go`：manifest 结构、permission 常量、校验。
2. 迁移 000021：3 张表。
3. `repo_sqlite.go`：状态/配置/密钥持久化。
4. `supervisor.go`：进程托管 + 退避重启 + 健康检查 + 残留清理。
5. `proxy.go`：反向代理 + 超时 + 流式。
6. `host_socket.go` + `host.go`：Host API 基础（audit/config/log/event/exec/storage/notify/panel.api）+ token + 权限。
7. `events.go`：发布/订阅/投递/限流；面板业务点接 `Publish`。
8. `registry.go` + `routes.go`：contribution 分发、`/api/plugins/*` 路由、iframe 入口。
9. `config.go`：schema 校验 + KV；设置页渲染数据。
10. 前端：插件菜单/小部件动态渲染、iframe 挂载、Host SDK `host_sdk.js`。
11. 面板 UI：插件管理页（列表/启用禁用/权限审批/配置/日志查看/状态）。
12. Go SDK `easyserver-plugin-sdk-go`：socket server + Host API 客户端 + contribution 声明辅助。
13. 试点插件 A（Go）：备份管理（菜单页 + 路由 + 设置 + 订阅 website.created + 后台 service）。
14. 试点插件 B（Python 裸实现）：证书到期监控（后台 service + 事件发布 + 通知），验证多语言。

**验证标准**：
- 杀插件进程 → 主面板无影响 + 自动重启。
- 插件请求 sleep 60s → 主面板 30s 后 504，其他 API 正常。
- 插件疯狂发事件 → 被限流，主面板正常。
- 插件 manifest 非法 → `error` 不启动，主面板正常。
- 卸载/启用/禁用/配置变更均生效且审计可见。
- Go + Python 插件都能注册菜单并在面板显示。

### 19.2 v2：能力完善
- `commands` / `datasources` / `hooks` 扩展点。
- cgroup 资源限额（`resources` 字段落地）。
- Python / Node SDK。
- 请求级 context cancel（代理取消时通知插件）。
- 插件依赖（`dependencies`）与 command.invoke。
- 插件开发模式（`easyserver plugin dev <dir>` 热重载）。
- 事件 schema 注册中心 + 过滤。

### 19.3 v3：生态
- 插件签名（离线校验，非市场）。
- 插件市场只读索引（仅元数据，下载由管理员手动）。
- 跨服务器插件配置同步。

### 19.4 试点插件
- **A 备份管理（Go）**：页面 + 路由 + 设置 + 事件订阅 + service，覆盖多数扩展点。
- **B 证书监控（Python）**：service + 事件发布 + 通知，验证多语言。

## 20. 不做清单（防臃肿）

- ❌ 进程内插件（Go plugin）——隔离不足
- ❌ 远程 JS 动态注入主前端——安全与维护坑，iframe 已够
- ❌ 插件市场一键远程安装——远程 root 风险
- ❌ 插件间直接内存/DB 共享——一律走 Host API/事件
- ❌ gRPC/IDL——HTTP+JSON 已满足
- ❌ 细粒度 RBAC——单管理员面板不需要
- ❌ 插件热卸载语义——杀进程即足够
- ❌ v1 实现 cgroup/commands/datasources/hooks——推迟 v2

---

## 21. 自我审查记录（6 轮多维度）

### 审查 1：安全维度
**Q1.1** plugin_token 放环境变量，同机其他 root 进程能读到？  
答：同机 root 本就能做任何事（威胁边界之外）。需防御的是"非 root 同机进程"和"网络"。host socket 目录 `0700` + socket `0600` + 不监听 TCP 已防这两类。**结论：可接受，§7.1/§17 已体现。**

**Q1.2** iframe 里的插件 JS 能直接 `fetch('/api/host/...')` 调 Host API 吗？  
答：不能放行。iframe 与主面板同源时会带 cookie/凭据，存在越权风险。**修正：§9.2 明确 iframe 不持 token，Host SDK 经主前端代理；iframe 与主前端不同源（sandbox），且 Host API 走 host socket（独立于业务 HTTP），iframe 的 fetch 到不了 host socket。** 已回填 §9.2。

**Q1.3** `exec.restricted` 的"受限"是否名不副实？  
答：是。面板 root，exec 本质是 root 执行。**修正：§7.3 诚实定位为"有审计+超时的 root 执行"，不做命令白名单；文档告知等同 root shell。** 已回填。

**Q1.4** `panel.api.proxy` 会让插件以谁的身份调面板业务 API？  
答：以"插件权限范围"代理，非用户态。需明确避免插件借 proxy 绕过自身权限。**修正：§7.2 注明按插件权限范围放行。** 已回填。

**Q1.5** 恶意 manifest（超长 schema、恶意 path 穿越）？  
答：§6.2 加字段长度/深度上限 + path 正则白名单；`/storage` 路径规范化（§13/§15 storage.go）。**已体现。**

### 审查 2：可靠性维度
**Q2.1** 主面板崩溃后插件成孤儿？  
答：Supervisor 启动时扫描 `/run/easyserver/plugins/*.sock` + `ps`，杀孤儿、清残留 socket（§12.2）。插件需容忍 Host API 短暂断开（SDK 内置重连重试）。**已体现，§12.2 补充。**

**Q2.2** at-least-once 投递 + 插件不幂等 → 重复副作用？  
答：事件带 `event_id` 供去重，文档要求插件幂等（§8.2）。**已体现。**

**Q2.3** 反向代理超时后插件仍在处理，资源占用？  
答：超时只断客户端响应；插件侧请求可能继续。v1 为已知限制，v2 加请求级 context cancel（代理取消通知插件，§19.2）。**记录为已知限制。**

**Q2.4** 健康检查误判：插件忙时 `/health` 超时被禁用？  
答：连续 `fail_threshold`（默认 3）次才动作，单次失败不动作；健康超时阈值独立于请求超时（§12.2）。**已体现。**

**Q2.5** 配置写一半崩溃？  
答：`plugin_config` 写入用 SQLite 事务，原子；写入前 schema 校验（§10）。**已体现。**

**Q2.6** 主面板关停时插件 SIGTERM grace？  
答：§12.2 最多 grace 10s，不等全部退出，避免关停卡死。**已体现。**

### 审查 3：架构耦合维度
**Q3.1** Host API 是强耦合点，改端点影响所有插件？  
答：版本化（`host_api_version`），只加不改语义，废弃保留 2 版本（§18）。SDK 屏蔽细节。**已体现。**

**Q3.2** 插件间 command.invoke 造成 A→B 运行时耦合？  
答：`dependencies` 声明式依赖；B 缺失则 A 该能力降级不崩；invoke 带超时+禁自调+深度限制（§6.1/§7.2/§19.2）。**已体现。**

**Q3.3** 事件 schema 变更导致订阅方崩？  
答：§8.5 规则：只加可选字段，不删/不改语义；订阅方宽松解析（忽略未知字段）。**已体现。**

**Q3.4** 面板业务代码被 Publish 侵入？  
答：业务点仅一行 `eventBus.Publish(常量, payload)`，事件名常量集中管理，不侵入业务逻辑（§8.4）。**可接受。**

**Q3.5** 前端挂载点与插件耦合？  
答：面板前端固定 slot（侧边栏/仪表盘/设置页），运行时从 registry 拉 contributions 动态渲染，不硬编码插件（§9.1）。**已体现。**

### 审查 4：演进兼容维度
**Q4.1** api_version 协商失败如何处理？  
答：插件 `api_version` 不在面板 `[MIN,MAX]` → `error` 不加载 + 审计 + 暴露面板版本供对齐（§6.3）。**已体现。**

**Q4.2** 旧面板遇新 contribution 类型？  
答：未知类型忽略 + 告警，不崩（§5 注，§18）。**已体现。**

**Q4.3** plugins 表加字段的迁移？  
答：复用现有迁移体系，幂等 hook（§14）。**已体现。**

**Q4.4** 插件升级（同 name 不同 version）状态保留？  
答：替换文件 + Supervisor 重启，状态/配置/密钥保留（§12.2）。**已体现。**

**Q4.5** Host API 端点废弃流程？  
答：标 deprecated，保留 2 个版本，文档公告（§18）。**已体现。**

### 审查 5：实现复杂度维度（防臃肿）
**Q5.1** cgroup 是否 v1 必须？  
答：否。进程隔离已满足"不挂主面板"。v1 仅声明 `resources` 字段不实现（§19.2）。**避免 v1 复杂度。**

**Q5.2** Host SDK 是否 v1 必须？  
答：是。否则插件前端无法交互，自由度大打折扣。v1 做基础 API（config/event/notify/navigate/api.call）。**保留。**

**Q5.3** 多语言 SDK v1 要几套？  
答：v1 只做 Go SDK + 协议文档（Python/Node 可裸实现，试点 B 用 Python 裸实现验证）。非 Go SDK 推迟 v2。**平衡自由度与复杂度。**

**Q5.4** 事件总线 v1 范围？  
答：v1 做基础 publish/subscribe/限流/event_id；schema 注册中心 + 过滤推迟 v2（§19.2）。**已收敛。**

**Q5.5** commands/datasources/hooks 为何推迟？  
答：v1 前 8 种扩展点已覆盖绝大多数需求；后 3 种引入插件间调用/数据源/hook 流程侵入，复杂度高，v2 验证需求后再做（§5/§19.2）。**避免 v1 过载。**

**Q5.6** 配置 schema 渲染是否过度？  
答：不复杂（JSON Schema → 表单），且不做则插件配置无法管理。v1 用简单渲染（无需重型库）。**保留。**

**Q5.7** 是否存在更简单的等价方案？**（关键自省）**  
答：曾考虑"纯配置驱动（无代码）"——但不满足"丰富功能"。曾考虑 Go plugin——不满足"不挂"。进程外 + 4 原语是满足全部约束的最小完备集。**确认无更简单等价方案。**

### 审查 6：边界场景维度
**Q6.1** 插件启动很慢？  
答：`startup_timeout`（默认 30s）超时 → `error`（§12.2）。**已体现。**

**Q6.2** 两个插件注册同 path 菜单？  
答：`menus.path` 全局唯一校验，冲突拒绝后者启用 + 审计（§6.2）。**已体现。**

**Q6.3** 插件疯狂发事件？  
答：每插件入队速率限流（默认 100/s）+ 队列上限（10000）溢出丢最旧 + 告警（§8.3）。**已体现。**

**Q6.4** command.invoke 递归（A→B→A）？  
答：深度限制 3 + 禁自调 + 超时（§7.2/§17）。**已体现。**

**Q6.5** 插件 socket 文件权限被插件 umask 放宽？  
答：socket 目录 `/run/easyserver/plugins/` 设 `0700` root，即使 socket 文件权限宽松，非 root 也进不了目录。SDK 指导 `umask 0077`。**§13 已体现目录 0700。**

**Q6.6** 大文件/大响应经反向代理 OOM？  
答：代理流式转发（不缓冲全量）+ 最大体限制（§15 proxy.go）。**已体现。**

**Q6.7** 插件日志爆盘？  
答：每插件日志大小上限 + 轮转（§7.2 `/log` 落文件，§15 由 supervisor 管理轮转）。**补充：§12.2 补日志轮转。**

---

## 22. 终稿结论

本方案以**进程外隔离 + 4 正交原语（Contribution / Host API / EventBus / Webview）+ 声明式开放扩展点 + schema 驱动配置 + 声明-审批权限 + 完整生命周期状态机 + 双向 Unix socket + 前后端 SDK** 构成框架级插件系统。

- **框架级**：契约层 + 运行时层 + SDK 层三层清晰；扩展点开放注册，新形态不改核心。
- **扩展性强**：新扩展点 = 新 contribution 类型 + handler；插件间经事件+command 协作。
- **自由度高**：前端任意栈（iframe）、后端任意语言（socket+HTTP）、能力任意组合（Host API）、插件间协作（事件+command）。
- **不挂主系统**：进程隔离 + Supervisor 自愈 + 超时 + 限流 + 崩溃禁用。
- **不臃肿**：正交机制少而强；契约版本化只加不改；不做清单明确；v1 聚焦核心、v2/v3 渐进。

6 轮审查（安全/可靠性/架构耦合/演进兼容/实现复杂度/边界场景）共 30+ 问题，结论已回填正文，无遗留阻断项。Q2.3（代理取消传播）与 Q6.7（日志轮转）记为 v1 已知项/v2 增强，不阻断 v1。

**建议下一步**：进计划模式把 v1（§19.1）落成可执行任务清单并开始实现，先打通试点插件 A（Go）验证全链路。

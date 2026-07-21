# EasyServer Mobile — Flutter 架构方案

> 独立仓库, Flutter 3.x 跨平台(iOS/Android), 复用现有后端 API 和 WebSocket。
> 覆盖面板全功能领域（20+ 模块），支持用户配置服务器地址。

---

## 一、架构总览

采用 **Clean Architecture + Riverpod 依赖注入** 三层分离, 依赖方向严格指向内层:

```
┌──────────────────────────────────────────────────┐
│  Presentation (Flutter/Widget)                    │
│  screens/ widgets/ providers/                    │
│         ↓ 依赖                                   │
│  Domain (零外部依赖, 纯 Dart)                     │
│  entities/ repositories(接口)/ usecases/          │
│         ↑ 实现                                   │
│  Data                                            │
│  repositories/ datasources/ (API/WS/Storage)     │
└──────────────────────────────────────────────────┘
```

**核心原则:**
- **Domain 层零依赖** — 不 import dio、flutter、riverpod
- **依赖倒置** — Repository 接口定义在 Domain, 实现在 Data
- **Provider 只做桥接** — 不写业务逻辑, 只调用 UseCase 或 Repository
- **数据类不可变** — Freezed 生成, 保证单向数据流
- **服务器地址可配置** — 用户首次启动输入服务器地址，保存在 secure_storage

---

## 二、目录结构

```
easyserver_mobile/
├── lib/
│   ├── main.dart
│   ├── app.dart                        # MaterialApp + GoRouter
│   │
│   ├── domain/                         # ── 零外部依赖 ──
│   │   ├── entities/                   # Freezed 不可变数据模型
│   │   │   ├── user.dart
│   │   │   ├── auth_result.dart
│   │   │   ├── monitor_snapshot.dart
│   │   │   ├── service_info.dart
│   │   │   ├── file_entry.dart
│   │   │   ├── web_server.dart
│   │   │   ├── website.dart
│   │   │   ├── db_server.dart
│   │   │   ├── database.dart
│   │   │   ├── cron_task.dart
│   │   │   ├── firewall_rule.dart
│   │   │   ├── process_guard.dart
│   │   │   ├── container.dart
│   │   │   ├── cloud_instance.dart
│   │   │   ├── ssh_config.dart
│   │   │   ├── notification.dart
│   │   │   ├── audit_log.dart
│   │   │   ├── file_share.dart
│   │   │   ├── deploy_server.dart
│   │   │   ├── runtime_env.dart
│   │   │   └── app_settings.dart
│   │   ├── repositories/               # 抽象接口
│   │   │   ├── auth_repository.dart
│   │   │   ├── monitor_repository.dart
│   │   │   ├── service_repository.dart
│   │   │   ├── file_repository.dart
│   │   │   ├── terminal_repository.dart
│   │   │   ├── web_server_repository.dart
│   │   │   ├── db_server_repository.dart
│   │   │   ├── cron_repository.dart
│   │   │   ├── firewall_repository.dart
│   │   │   ├── process_repository.dart
│   │   │   ├── container_repository.dart
│   │   │   ├── cloud_repository.dart
│   │   │   ├── ssh_repository.dart
│   │   │   ├── notification_repository.dart
│   │   │   ├── audit_repository.dart
│   │   │   ├── file_share_repository.dart
│   │   │   ├── deploy_repository.dart
│   │   │   ├── runtime_repository.dart
│   │   │   ├── env_config_repository.dart
│   │   │   ├── system_repository.dart
│   │   │   └── settings_repository.dart
│   │   └── usecases/                    # 业务逻辑(按需)
│   │       ├── login_usecase.dart
│   │       ├── get_monitor_stream_usecase.dart
│   │       └── confirm_qr_login_usecase.dart
│   │
│   ├── data/                            # ── 实现 Domain 接口 ──
│   │   ├── repositories/                # Repository 实现
│   │   └── datasources/
│   │       ├── remote/
│   │       │   ├── api_client.dart              # dio 封装(动态 baseUrl)
│   │       │   ├── api_interceptor.dart          # JWT 自动注入 + 401 处理
│   │       │   └── ws_client.dart               # WebSocket 工厂
│   │       └── local/
│   │           ├── secure_storage.dart           # JWT + serverUrl 加密存储
│   │           └── settings_storage.dart         # SharedPreferences 缓存
│   │
│   ├── presentation/                    # ── UI + Provider ──
│   │   ├── providers/
│   │   │   ├── auth_provider.dart
│   │   │   ├── server_config_provider.dart  # 服务器地址管理
│   │   │   ├── dashboard_provider.dart
│   │   │   ├── service_provider.dart
│   │   │   ├── terminal_provider.dart
│   │   │   ├── file_provider.dart
│   │   │   ├── notification_provider.dart
│   │   │   └── ... (按模块)
│   │   ├── screens/
│   │   │   ├── splash/                  # 启动页(检查服务器地址/JWT)
│   │   │   ├── setup/                   # 首次配置服务器地址
│   │   │   ├── login/                   # 密码登录
│   │   │   ├── dashboard/               # 仪表盘(监控首页)
│   │   │   ├── services/                # 服务管理
│   │   │   ├── terminal/                # Web 终端
│   │   │   ├── files/                   # 文件管理器
│   │   │   ├── web_servers/             # Web 服务器
│   │   │   ├── databases/               # 数据库
│   │   │   ├── cron/                    # 计划任务
│   │   │   ├── firewall/                # 防火墙
│   │   │   ├── containers/              # 容器
│   │   │   ├── processes/               # 进程守护
│   │   │   ├── cloud/                   # 腾讯云
│   │   │   ├── ssh/                     # SSH 管理
│   │   │   ├── audit/                   # 审计日志
│   │   │   ├── deploy/                  # 部署
│   │   │   ├── runtime/                 # 运行环境
│   │   │   ├── env_config/              # 环境变量
│   │   │   ├── settings/                # 面板设置
│   │   │   └── about/                   # 关于
│   │   ├── widgets/                     # 通用 UI 组件
│   │   │   ├── status_badge.dart
│   │   │   ├── metric_card.dart
│   │   │   ├── file_icon.dart
│   │   │   ├── permission_check.dart
│   │   │   └── loading_overlay.dart
│   │   └── theme/
│   │       └── app_theme.dart
│   │
│   └── di/                              # 依赖注入配置
│       ├── providers.dart               # 全局 Provider 注册
│       └── router.dart                  # GoRouter 配置
│
├── test/
├── pubspec.yaml
└── README.md
```

---

## 三、导航结构

采用 **Drawer + BottomNavigationBar 混合导航**:

```
Splash → Setup(首次) → Login → Home
                                 │
                    ┌────────────┴────────────────┐
                    │  BottomNavigationBar (5 tabs)│
                    ├──────────────────────────────┤
                    │ ① 仪表盘  ② 服务  ③ 终端    │
                    │ ④ 文件    ⑤ 更多(Drawer)    │
                    └──────────────────────────────┘
```

**Drawer 菜单 (全部功能入口):**

| 分组 | 功能 | 说明 |
|------|------|------|
| **管理** | Web 服务器 | Nginx/Apache 安装与站点管理 |
| | 数据库 | MySQL/PostgreSQL/Redis 管理 |
| | 计划任务 | Cron 任务和脚本管理 |
| | 防火墙 | iptables/nftables 规则管理 |
| | 容器 | Docker 管理 |
| | 进程守护 | 托管进程生命周期 |
| **运维** | 部署 | Git 部署和版本回滚 |
| | 运行环境 | Node/Python/Go 版本管理 |
| | 环境变量 | 系统环境配置 |
| | SSH 管理 | SSH 配置与会话 |
| | 端口监听 | 系统端口查看 |
| **系统** | 审计日志 | 操作审计和导出 |
| | 通知中心 | 系统通知 |
| | 文件分享 | 临时分享链接 |
| | 面板设置 | 面板配置 |
| | 安全设置 | TOTP/IP 白名单/会话 |
| | 关于 | 版本信息 |

**独立路由:**
- `/login` — 密码登录
- `/setup` — 配置服务器地址
- `/splash` — 启动检查

---

## 四、核心 API 对接（完整覆盖）

### 4.1 认证 (Auth)

```
Endpoint                    Method   Description
POST   /api/auth/login             密码登录 (返回 JWT)
POST   /api/auth/verify-totp        TOTP 两步验证
POST   /api/auth/verify-backup      备用码验证
GET    /api/auth/me                 获取当前用户
POST   /api/auth/logout             登出
POST   /api/auth/change-password    修改密码
POST   /api/auth/totp/setup         生成 TOTP 密钥
POST   /api/auth/totp/enable        启用 TOTP
POST   /api/auth/totp/disable       关闭 TOTP
GET    /api/auth/totp/status        TOTP 状态
GET    /api/auth/sessions           会话列表
POST   /api/auth/sessions/kick      踢出会话
POST   /api/auth/sessions/kick-all  踢出所有其他会话
POST   /api/auth/qr/session         创建扫码登录会话
POST   /api/auth/qr/confirm         确认扫码登录(手机)
POST   /api/auth/qr/status          轮询扫码状态
POST   /api/auth/qr/cancel          取消扫码
GET    /api/auth/turnstile/config   获取 Turnstile 配置
```

#### 4.1.1 移动端登录契约（单设备绑定）

移动端登录（`/auth/login`、`/auth/verify-totp`、`/auth/verify-backup`）必须携带以下字段：

| 字段 | 说明 |
|------|------|
| `client_type` | 固定 `"mobile"`（Web 端为 `"web"`，缺省按 web 处理） |
| `device_id` | 每次安装 App 生成一个稳定 UUID，持久化于 `flutter_secure_storage`；卸载重装会生成新值 |
| `device_info` | 机型/系统描述，如 `"iPhone 15 / iOS 17.2"`、`"Pixel 8 / Android 14"` |

**绑定行为**（服务端 `MobileDeviceBinding` 开启，默认开）：
- 无活跃移动会话 → 创建会话（记录 device_id/device_info/IP）。
- 活跃移动会话且 `device_id` 相同 → 视为同设备刷新，替换旧移动会话，Web 会话不受影响。
- 活跃移动会话且 `device_id` 不同 → **拒绝**，HTTP 403，`message: "已有其他移动设备登录，请先在面板「会话管理」中解绑该设备后再试"`。用户需先在 Web 面板「安全设置 → 会话管理」踢出旧移动设备后再登录新设备。
- 仅当 `AllowMultiSession`（允许多端同时登录）开启时，移动端绑定才让 Web 与移动端共存；若 `AllowMultiSession` 关闭，全局单会话，每次新登录踢掉所有会话（含移动），绑定无效。

**安全说明**：`device_id` 是客户端自报的软标识，可被伪造，**不是安全边界**。真正的控制是"同一账号同时只允许一个活跃移动会话 + 面板可撤销"：即便凭证泄露，攻击者也无法在另一台手机登录（除非先解绑，而解绑会触发审计日志与通知，合法用户可察觉）。会话记录见 `GET /auth/sessions`（含 `client_type`/`device_info`/`is_current`），撤销见 `POST /auth/sessions/kick`。

### 4.2 监控 (Monitor)

```
GET    /api/monitor/stats           获取当前监控快照
GET    /api/monitor/history         获取历史数据(?start=&end=)
WS     /ws/monitor                  实时监控推送(每秒)
```

**WebSocket 协议:**
```json
// 服务端推送 (每秒)
{ "type": "snapshot", "data": { "cpu": {...}, "memory": {...}, "disk": [...], "network": {...}, "top_process": [...], "timestamp": "..." } }

// 客户端心跳
→ { "type": "ping" }
← { "type": "pong" }
```

### 4.3 服务管理 (Services)

```
GET    /api/services                服务列表
POST   /api/services/details        批量获取详情
GET    /api/services/:name          单个服务
GET    /api/services/:name/logs     服务日志(?tail=100)
POST   /api/services/:name/start    启动
POST   /api/services/:name/stop     停止
POST   /api/services/:name/restart  重启
POST   /api/services/:name/enable   启用自启
POST   /api/services/:name/disable  禁用自启
WS     /ws/services/:name/logs      实时日志流
```

### 4.4 终端 (Terminal)

```
WS     /ws/terminal/:id             PTY WebSocket 终端
```

**WebSocket 协议:**
```json
// 客户端 → 服务端
{ "type": "input", "data": "ls -la\n" }
{ "type": "resize", "cols": 80, "rows": 24 }

// 服务端 → 客户端
{ "type": "output", "data": "\u001b[32mtotal..." }
{ "type": "exit" }
{ "type": "pong" }
```

### 4.5 文件管理 (Files)

```
GET    /api/files?path=             列出目录
POST   /api/files/mkdir             创建目录
POST   /api/files/upload            上传文件(FormData)
GET    /api/files/download?path=    下载文件
PUT    /api/files/rename            重命名
DELETE /api/files?path=&recursive=  删除
POST   /api/files/move              移动
POST   /api/files/copy              复制
GET    /api/files/content?path=     读取文本内容
PUT    /api/files/content           保存文本内容
GET    /api/files/search?path=&q=   搜索文件名
GET    /api/files/search-content?   搜索文件内容
GET    /api/files/details?path=     文件详情
GET    /api/files/mime-type?path=    MIME 类型
POST   /api/files/compress          压缩
POST   /api/files/extract           解压
PUT    /api/files/chmod             修改权限
PUT    /api/files/chown             修改所有者
```

### 4.6 Web 服务器 (Web Server)

```
GET    /api/web-servers             列表
GET    /api/web-servers/:id         详情
POST   /api/web-servers             创建
DELETE /api/web-servers/:id         删除
POST   /api/web-servers/:id/install         安装
POST   /api/web-servers/:id/uninstall       卸载
POST   /api/web-servers/:id/start/stop/restart/reload
GET    /api/web-servers/:id/config          配置内容
PUT    /api/web-servers/:id/config          保存配置
GET    /api/web-servers/:id/logs            日志
GET    /api/web-servers/:id/process         进程信息

GET    /api/web-servers/:serverId/websites          站点列表
POST   /api/web-servers/:serverId/websites          创建站点
PUT    /api/web-servers/:serverId/websites/:id      更新站点
DELETE /api/web-servers/:serverId/websites/:id      删除站点
POST   .../websites/:id/enable/disable              启用/禁用
POST   .../websites/:id/ssl                         申请 SSL
POST   .../websites/:id/ssl/upload                  上传 SSL
POST   .../websites/:id/build                       构建
POST   .../websites/:id/process/start/stop          管理进程
```

### 4.7 数据库 (Database)

```
GET    /api/db-servers                       列表
GET    /api/db-servers/:id                   详情
GET    /api/db-servers/:id/versions          版本列表
POST   /api/db-servers/:id/versions          安装版本
POST   /api/db-servers/versions/:vid/start/stop/restart
GET    /api/db-servers/:id/databases         数据库列表
POST   /api/db-servers/:id/databases         创建数据库
POST   /api/db-servers/:id/users             创建用户
POST   /api/db-servers/:id/users/:userId/grant   授权
GET    /api/db-servers/databases/:dbId/tables     表列表
GET    /api/db-servers/databases/:dbId/query      查询数据
POST   /api/db-servers/databases/:dbId/execute    SQL 执行
POST   /api/db-servers/databases/:dbId/backup     备份
GET    /api/db-servers/backups/:backupId/download 下载备份
POST   /api/db-servers/backups/:backupId/restore  恢复备份
```

### 4.8 计划任务 (Cron)

```
GET    /api/cron/tasks                列表
POST   /api/cron/tasks                创建
PUT    /api/cron/tasks/:id            更新
DELETE /api/cron/tasks/:id            删除
POST   /api/cron/tasks/:id/enable/disable/run
GET    /api/cron/tasks/:id/logs       运行日志
GET    /api/cron/scripts              脚本列表
POST   /api/cron/scripts              创建脚本
GET    /api/cron/presets              预设表达式
GET    /api/cron/describe             解析表达式
GET    /api/cron/next-runs            计算下次运行
```

### 4.9 防火墙 (Firewall)

```
GET    /api/firewall/status           状态
POST   /api/firewall/enable/disable   启用/禁用
GET    /api/firewall/rules            规则列表
POST   /api/firewall/rules            创建规则
PUT    /api/firewall/rules/:id        更新规则
DELETE /api/firewall/rules/:id        删除规则
POST   /api/firewall/rules/:id/enable/disable
POST   /api/firewall/rules/bulk-*     批量操作
GET    /api/firewall/logs             防火墙日志
POST   /api/firewall/default-policy   默认策略
GET    /api/firewall/templates        规则模板
POST   /api/firewall/templates/apply  应用模板
```

### 4.10 容器 (Container)

```
GET    /api/containers                列表
POST   /api/containers                创建
DELETE /api/containers/:id            删除
POST   /api/containers/:id/start/stop/restart
GET    /api/containers/:id/logs       日志
GET    /api/containers/:id/inspect    详情
GET    /api/images                    镜像列表
POST   /api/images/pull               拉取镜像
DELETE /api/images/:id                删除镜像
GET    /api/containers/stats          资源统计
```

### 4.11 进程守护 (Process Guardian)

```
GET    /api/processes                 列表
POST   /api/processes                 创建
PUT    /api/processes/:id             更新
DELETE /api/processes/:id             删除
POST   /api/processes/:id/start/stop/restart
GET    /api/processes/:id/logs        日志
GET    /api/processes/:id/stats       资源统计
POST   /api/processes/batch/*         批量操作
GET    /api/process-groups            分组管理
```

### 4.12 部署 (Deploy)

```
GET    /api/deploy/servers            部署服务器列表
POST   /api/deploy/servers            创建服务器
PUT    /api/deploy/servers/:id        更新
DELETE /api/deploy/servers/:id        删除
POST   /api/deploy/servers/:id/test   测试连接
GET    /api/deploy/tasks              部署任务列表
POST   /api/deploy/tasks              创建任务
POST   /api/deploy/tasks/:id/exec     执行部署
GET    /api/deploy/versions           版本列表
POST   /api/deploy/versions/:id/rollback  回滚
```

### 4.13 运行环境 (Runtime)

```
GET    /api/runtime                   已安装环境列表
GET    /api/runtime/:name/remote-versions   远程可用版本
POST   /api/runtime/install/uninstall/set-default
GET    /api/runtime/progress/:id      安装进度
GET    /api/runtime/logs/:id          安装日志
GET    /api/runtime/catalog           支持的环境目录
GET    /api/packages                  包管理
POST   /api/packages/install/uninstall/update
```

### 4.14 腾讯云 (Cloud)

```
GET    /api/cloud/instances           实例列表
GET    /api/cloud/instances/:id       实例详情
POST   /api/cloud/instances/:id/start/stop/restart
GET    /api/cloud/monitor/:id         监控数据
GET    /api/cloud/firewall/:id        防火墙规则
POST   /api/cloud/firewall/:id        添加规则
DELETE /api/cloud/firewall/:id/:ruleId
GET    /api/cloud/snapshots           快照列表
POST   /api/cloud/snapshots           创建快照
GET    /api/cloud/traffic             流量信息
```

### 4.15 SSH 管理

```
GET    /api/ssh/config                SSH 配置
PUT    /api/ssh/config                保存配置
POST   /api/ssh/config/test           测试配置
POST   /api/ssh/config/reload         重载服务
GET    /api/ssh/sessions              活跃会话
POST   /api/ssh/sessions/:pid/kill    终止会话
GET    /api/ssh/logins                登录历史
```

### 4.16 其他

```
# 审计日志
GET    /api/audit-logs                列表(分页+筛选)
GET    /api/audit-logs/stats          统计
GET    /api/audit-logs/export         导出
DELETE /api/audit-logs/clean          清理

# 文件分享
POST   /api/file-shares               创建分享
GET    /api/file-shares               列表
DELETE /api/file-shares/:id           删除

# 通知
GET    /api/notifications             列表(?unread=true)
GET    /api/notifications/unread-count
PUT    /api/notifications/:id/read    标记已读
PUT    /api/notifications/read-all

# 系统
GET    /api/settings                  获取所有设置
PUT    /api/settings/server/auth/monitor/audit/notify/cloud
POST   /api/settings/restart          重启面板
GET    /api/settings/system           系统信息
GET    /api/system/ports              监听端口
GET    /api/system/ssh-logins         SSH 登录记录
GET    /api/system/check-port         端口检测
GET    /api/system/processes          系统进程列表

# 模板
GET    /api/templates/docker-images   Docker 模板
GET    /api/templates/scripts         脚本模板

# 环境变量
GET    /api/env-config                列表
POST   /api/env-config                创建
PUT    /api/env-config/:id            更新
DELETE /api/env-config/:id            删除
GET    /api/env-config/path           PATH 条目
GET    /api/env-config/script         生成脚本
```

---

## 五、认证与安全

### 5.1 登录流程

```
┌─────────┐    ┌──────────┐    ┌───────────┐    ┌────────────┐
│ Splash  │───→│  Setup   │───→│  Login    │───→│  TOTP(可选)│───→ Home
│ (检查   │    │ (首次/   │    │ (密码+    │    │ (如果启用) │
│  JWT)   │    │  切换)   │    │  JWT)    │    │            │
└─────────┘    └──────────┘    └──────────┘    └────────────┘
```

- 应用启动 → Splash 检查 secure_storage 中 JWT 和服务器地址
- 无服务器地址 → Setup 页面（输入协议+域名+端口）
- JWT 无效/过期 → Login 页面
- 服务器支持 TOTP 且用户已启用 → 登录后跳转 TOTP 验证

### 5.2 Token 管理

- JWT 存 `flutter_secure_storage`（加密存储）
- `dio` 拦截器自动注入 `Authorization: Bearer <token>`
- 401 响应 → 清除 token → 跳转 Login
- 应用前后台切换时检查 token 有效期

### 5.3 扫码授权

- 手机已登录状态，扫描 Web 面板二维码
- 解析 `esqr:<token>` → POST `/api/auth/qr/confirm`
- 确认后 Web 面板自动登录

---

## 六、WebSocket 总览

| 路径 | 用途 | 认证方式 |
|------|------|---------|
| `/ws/monitor` | 监控实时数据(每秒推送) | `Sec-WebSocket-Protocol: token` |
| `/ws/terminal/:id` | PTY 终端双向通信 | `Sec-WebSocket-Protocol: token` |
| `/ws/services/:name/logs` | 服务日志实时流 | `Sec-WebSocket-Protocol: token` |

连接方式：token 通过 `Sec-WebSocket-Protocol` 头传递:
```dart
final ws = WebSocket.connect(
  'ws://host/ws/monitor',
  protocols: ['token', jwtToken],
);
```

---

## 七、技术栈

| 层 | 包 | 用途 |
|---|------|------|
| 状态管理 | `flutter_riverpod` + `riverpod_annotation` | 编译期 DI, 自动释放 |
| 数据类 | `freezed` + `json_serializable` | 不可变 Entity, JSON 序列化 |
| HTTP | `dio` | 拦截器/重试/超时 |
| WebSocket | `web_socket_channel` | 监控流 + 终端 PTY |
| 安全存储 | `flutter_secure_storage` | JWT + 服务器地址加密存储 |
| 本地存储 | `shared_preferences` | UI 偏好缓存 |
| 路由 | `go_router` | 声明式路由, 深度链接 |
| 二维码 | `mobile_scanner` | 扫码授权 |
| 图表 | `fl_chart` | 监控趋势图 |
| 本地通知 | `flutter_local_notifications` | 告警推送(轮询) |
| 文件下载 | `dio` + `open_file` | 文件下载 + 打开 |

---

## 八、Dashboard 设计

仪表盘作为首页，展示核心监控信息和快速操作:

```
┌─────────────────────────────────┐
│  AppBar: EasyServer    🔔(未读) │
├─────────────────────────────────┤
│  CPU: ████████░░ 78%   1m 2.5   │
│  Mem: ██████░░░░ 62%  16/32 GB │
│  Disk: ███████░░░ 71%  128/180G│
│  Net: ↑1.2M ↓3.5M              │
│                                 │
│  折线图(1h CPU + 内存趋势)      │
│  ┌───────────────────────────┐  │
│  │  ╱╲    ╱╲    ╱╲           │  │
│  │ ╱  ╲  ╱  ╲  ╱  ╲         │  │
│  └───────────────────────────┘  │
│                                 │
│  Top 进程:                      │
│  ├─ nginx    2.1%  45MB        │
│  ├─ mysql    8.5%  1.2GB       │
│  └─ node    12.3%  256MB       │
│                                 │
│  快捷操作卡片:                  │
│  ┌──────┐ ┌──────┐ ┌──────┐   │
│  │ 服务  │ │ 终端  │ │ 文件  │   │
│  └──────┘ └──────┘ └──────┘   │
└─────────────────────────────────┘
```

---

## 九、离线与错误处理策略

| 场景 | 处理方式 |
|------|---------|
| 网络不可用 | 显示离线提示，展示缓存数据(如有) |
| 请求超时 | 友好提示"服务器无响应"，允许重试 |
| 401 未授权 | 清除 token，跳转登录页 |
| 429 限流 | 展示"请求过于频繁"，自动 retry-after |
| 服务器地址错误 | 提示连接失败，允许重新配置 |
| WebSocket 断开 | 自动重连(指数退避，最多 5 次) |
| 文件上传中断 | 断点续传(大文件分片) |
| 表单提交失败 | 保留用户输入，提示具体错误 |

---

## 十、开发顺序

| 阶段 | 内容 | 预计 |
|------|------|------|
| **P0 基础** | | |
| 1 | 工程搭建: Flutter 初始化 + 目录 + pubspec + 主题 | 1h |
| 2 | Domain 层: Entity(Freezed) + Repository 接口 | 2h |
| 3 | Data 层: ApiClient(动态baseUrl) + 拦截器 + SecureStorage + DTO | 3h |
| 4 | DI + 路由: Riverpod providers + GoRouter | 1h |
| 5 | 认证: 首次 Setup → Login → TOTP → Auto-login | 3h |
| **P1 核心** | | |
| 6 | Dashboard: 监控快照 + fl_chart + WebSocket 实时 | 4h |
| 7 | 服务管理: 列表 + 启停 + 状态标签 | 2h |
| 8 | Web 终端: WebSocket PTY + xterm 模拟 | 3h |
| 9 | 文件管理: 目录浏览 + 上传/下载 + 文本编辑 | 4h |
| **P2 增强** | | |
| 10 | Web 服务器: 安装/配置 + 站点管理 + SSL | 3h |
| 11 | 数据库: 版本管理 + 数据库/用户 CRUD + SQL 执行 | 4h |
| 12 | 计划任务: 任务管理 + 脚本 + 日志 | 2h |
| 13 | 防火墙: 规则管理 + 批量操作 + 日志 | 3h |
| **P3 完整** | | |
| 14 | 容器管理 + 进程守护 + 部署 | 4h |
| 15 | SSH 管理 + 腾讯云 + 审计日志 | 3h |
| 16 | 运行环境 + 环境变量 + 通知中心 | 2h |
| 17 | 面板设置 + 安全设置 + 文件分享 | 2h |
| 18 | 通知轮询 + 本地推送 + 离线缓存 | 2h |
| 19 | 扫码登录 + 国际化 + 打磨 | 2h |
| 20 | 测试 + Bug 修复 + 发布准备 | 3h |

---

## 十一、pubspec.yaml

```yaml
name: easyserver_mobile
description: EasyServer Mobile Client

environment:
  sdk: '>=3.2.0 <4.0.0'

dependencies:
  flutter:
    sdk: flutter
  flutter_riverpod: ^2.5.0
  riverpod_annotation: ^2.3.0
  freezed_annotation: ^2.4.0
  json_annotation: ^4.8.0
  dio: ^5.4.0
  web_socket_channel: ^2.4.0
  flutter_secure_storage: ^9.0.0
  shared_preferences: ^2.2.0
  go_router: ^14.0.0
  mobile_scanner: ^4.0.0
  fl_chart: ^0.68.0
  flutter_local_notifications: ^17.0.0
  google_fonts: ^6.1.0
  intl: ^0.19.0
  open_file: ^3.3.2
  path: ^1.9.0

dev_dependencies:
  flutter_test:
    sdk: flutter
  build_runner: ^2.4.0
  riverpod_generator: ^2.4.0
  freezed: ^2.5.0
  json_serializable: ^6.7.0
  mocktail: ^1.0.0
```

---

## 十二、可测试性

每层可独立测试, 不依赖真实网络：

```
test/
├── domain/
│   ├── login_usecase_test.dart
│   └── monitor_stream_usecase_test.dart
├── data/
│   ├── repositories/
│   │   ├── auth_repository_impl_test.dart
│   │   └── monitor_repository_impl_test.dart
│   └── datasources/
│       ├── api_client_test.dart
│       └── secure_storage_test.dart
└── presentation/
    ├── auth_provider_test.dart
    └── dashboard_screen_test.dart
```

测试策略:
- **Domain 层**: 纯单元测试，mock Repository 接口
- **Data 层**: 使用 MockAdapter (dio) 模拟 HTTP 响应
- **Presentation 层**: ProviderContainer 注入 mock，WidgetTester 验证 UI

---

## 十三、注意事项

1. **iOS ATS**: 自签名服务器需在 Info.plist 配置 NSAppTransportSecurity
2. **Android 8+**: 明文 HTTP 需在 network_security_config.xml 允许
3. **文件上传**: 大文件使用分片上传（尚无后端分片接口，可先实现单次上传+进度条）
4. **后台 WebSocket**: iOS 限制后台长连接，监控建议切到前台重连
5. **推送通知**: 面板无原生推送机制，采用定时轮询 `/api/notifications?unread=true`
6. **服务器地址**: 支持 HTTP/HTTPS 和自定义端口，首次配置后保存在 secure_storage
7. **WebSocket 重连**: 指数退避 1s/2s/4s/8s/16s 后停止，手动重连按钮

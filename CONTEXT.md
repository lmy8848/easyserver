# EasyServer 领域模型

> EasyServer 是一个 Linux 服务器管理面板。它让单个管理员通过 Web 界面管理服务器上的软件、网站、数据库、定时任务、防火墙等——无需直接 SSH。
>
> ⚠ **术语约定**：文档用 "Service 层" 指 `internal/domain/<domain>/` 下的业务逻辑（如 CronService、FirewallService）；"Systemd Service" 指 Linux 系统服务（如 nginx.service）。代码中两者都叫 "service"，靠上下文区分。

---

## 核心概念

### 1. 运行环境与认证安全

- **服务器 (Server)** — 运行 EasyServer 的那一台 Linux 主机。EasyServer 是单机单管理员设计，不存在多节点集群。
- **单管理员 (Single Admin)** — 整个面板只允许一个管理员账户登录。CLI 工具 `cmd/cli/main.go` 可在紧急时重置密码与用户名。
- **Panel Root（面板根）** — 面板私有工作根目录，**固定常量**（`internal/infra/config` 的 `DataRoot`，值为 `/opt/easyserver`，不支持配置修改），所有领域子目录（`mise/`、`scripts/`、`db/`）由代码从它派生拼接。改动它 = 全新安装语义（存量内容不迁移）。面板配置文件的位置独立于它（由 `--config` 指定）。
  _Avoid_: 在代码/文档中散落硬编码 `/opt/easyserver` 字面量，一律经 `config.DataRoot` 派生。
- **认证安全（Auth & Security）** — 面板访问控制体系：
  - **凭据管理**：管理员用户名与密码修改统一在「面板设置 → 认证安全」的管理员账户卡片中操作（路由 `#admin-account`）。
  - **两步验证 (2FA / TOTP)**：基于 TOTP 标准密钥与备用恢复码（Backup Codes）的双因素认证（路由 `#2fa`）。
  - **会话管理 (Session)**：记录管理员每次登录会话（IP、User-Agent、客户端类型、活跃时间、过期时间），支持踢出指定或全部其他会话。
  - **安全防护**：会话超时（Session Timeout）、空闲超时（Idle Timeout）、登录失败次数锁定与速率限制（Rate Limiting）、IP 白名单控制。

### 2. 网站托管

- **Web Server** — 已安装的 Web 服务器软件类型，如 Nginx、Apache、Tomcat、Caddy。每种都有预定义的安装/卸载命令、配置路径、服务名等属性。
- **Website** — 在某个 Web Server 下部署的站点。每个 Website 绑定一个域名、根目录、项目类型（静态 / Node.js / PHP / Python / Java / 反向代理），并由 Web Server 生成对应配置文件。
- **SSL 证书 (SSL Certificate)** — 站点绑定的 TLS/SSL 证书，支持自定义上传或通过 Let's Encrypt / ACME 自动化申请与续期。

### 3. 数据库管理

- **DB Type** — 数据库类型（MySQL、PostgreSQL、Redis）。每种类型有默认端口和预定义版本模板。代码中为 `DBType`（`internal/domain/database/model.go`）。
- **Database Instance** — 某 DB Type 的受管数据库容器实例（例如 MySQL 8.0 运行在端口 3306）。它独占容器、数据卷和配置；一种类型可以同时有多个实例，宿主机安装的数据库不属于 EasyServer 的管理范围。
  _Avoid_: DB Version
- **Managed Database Container** — 由 EasyServer 创建、标识并全生命周期管理的数据库容器。它是 Database Instance 的运行载体，不是通用容器列表中可任意编辑的普通 Container（见 ADR-0006）。
- **Direct Connection（直连驱动）** — 面板用数据库驱动（`database/sql` + go-sql-driver/mysql / pgx/v5）经宿主映射端口访问 Database Instance 的通道。MySQL 与 PostgreSQL 的库/用户/表/表格浏览/SQL 执行走直连，拿到结构化类型与参数化查询；备份/恢复与配置编辑仍走容器 CLI。Redis 目前保持 CLI 通道（见 ADR-0007）。
- **Database** — 某个 Database Instance 下的实际数据库（例如 `myapp`）。
- **DB User** — 数据库用户，跨版本共享（同一类型下所有版本共用用户列表）。
- **DB Backup** — 数据库备份记录，关联到特定 Database。

### 4. 定时任务

- **Cron Task** — 一条定时任务定义，全面承载为 systemd 的一对 `.timer`（OnCalendar 触发）+ `.service`（mise exec 执行）。状态由 `internal/infra/systemd` D-Bus 客户端读取，日志走 journald，重试/超时由 systemd 原生处理。任务用 UI 预设调度表单描述，后端转为 OnCalendar 表达式（见 ADR-0004、ADR-0010）。
  _Avoid_: Cron Log（已弃用，日志由 journald 承载；`cron_tasks` 表已由 systemd unit 替代为权威）。
- **Cron Script** — 可被 Cron Task 引用的可复用脚本（sh/bash/python）。内容落盘面板根（Panel Root）的 `scripts/` 子目录，DB `scripts` 表仅存元数据（名称/语言/描述）。

### 5. 进程守护与系统服务

- **Process Guardian（进程守护）** — 面板托管的持久化后台进程。底层生成独立的 systemd unit（`easyserver-<name>.service`），unit 文件即权威；执行命令统一使用 `mise exec <lang>@<exact> -- <cmd>` 隔离 PATH；生命周期、自动拉起与状态查看由 `internal/infra/systemd` D-Bus 客户端与 journald 承载（见 ADR-0010）。
- **System Process（系统服务）** — 宿主机系统级 systemd 服务的查看与启停管理（如 nginx.service、redis.service），统一经由 `infra/systemd` D-Bus 客户端直连通信（见 ADR-0010）。
- **Systemd D-Bus Client（D-Bus 客户端）** — 基础设施层（`internal/infra/systemd`）封装的 systemd 通信通道（基于 `coreos/go-systemd/v22/dbus`，纯 Go 零 C 依赖），提供长连接复用、断线重连与强类型单元控制，彻底消除 `systemctl` 子进程开销与文本解析脆弱性（见 ADR-0010）。
  _Avoid_: ProcessManager（旧架构术语，已废弃）。

### 6. 防火墙与网络

- **Firewall** — 系统防火墙抽象（支持 iptables / ufw / firewalld / nftables 后端）。管理端口放行/阻止规则、默认策略与端口转发规则。

### 7. 运行时

- **Runtime** — 编程语言运行时（PHP、Node.js、Python、Go、Java 等）。由 Rust 编写的多语言版本管理器 `mise` 接管（见 ADR-0001、ADR-0002）。
- **Runtime Version** — 由 `mise` 安装的精确版本号（如 `20.11.0`）。权威来源是 `$MISE_DATA_DIR/installs/<tool>/<version>/` 目录；面板安装成功会写入 `.easyserver-ok` 完成标记文件，扫描只认带标记的目录。手动 `mise install` 的版本不带标记，面板不显示（见 ADR-0009）。
- **Runtime Binding（运行时绑定）** — 用 `lang@exact`（如 `node@20.11.0`）唯一确定一个已安装版本，cron/进程守护通过它绑定执行环境。持久化形态是 systemd unit 文件的 `RuntimeLang`/`RuntimeExact` 注释；前端 Select 的值即此字符串（见 ADR-0009）。
- **Global Default** — 通过面板私有 `config.DataRoot/mise/config.toml` 写入的默认版本，仅作为未指定版本时的解析兜底，面板业务流强制显式声明版本。
- **Execution Shim** — 面板主动发起的执行（进程守护、定时任务）强制包裹为 `mise exec <lang>@<exact> -- <cmd>`，彻底隔离 PATH。
- **Mirror Source（镜像源）** — 面板根（Panel Root）下 `mise/config.toml` `[env]` 段的条目，文件即权威：无 DB 副本、无启用/禁用状态，UI 编辑保存即写入文件生效。
- **Package Registry（包管理源）** — 各语言包管理器（npm、pip、goproxy、composer、maven）的国内/官方源配置与镜像切换。

### 8. 容器管理

- **ContainerEngine** — 容器引擎类型：Docker 或 Podman。刻意避开 `Runtime`（该词已特指语言运行时），API 统一收在 `/container/*` 前缀下，引擎通过 `?engine=podman` query 参数区分（缺省 `docker`）（见 ADR-0005）。
- **Container** — 某个 ContainerEngine 下的容器实例，API 资源路径为 `/container/instances`。
- **Image** — 容器镜像，归属某个 ContainerEngine。
- **Compose Project** — 由 Compose 描述文件定义的一组服务容器。Docker 走 `docker compose`，Podman 走 `podman-compose`。
- **Volume** / **Network** — 容器数据卷与容器网络，均归属某个 ContainerEngine。

### 9. 文件管理与外链

- **File Manager** — 服务器文件系统管理器，支持文件与目录的浏览、创建、在线编辑（代码高亮）、重命名、移动、复制、权限变更（chmod/chown）、打包压缩与解压缩。
- **File Share（文件外链）** — 文件临时外链分享记录。支持配置提取密码、下载次数上限与过期时间，生成独立安全的分享下载链接（`/share/*`）。

### 10. 安全与完整性

- **Vulnerability Scan（漏洞扫描）** — 扫描并分析主机操作系统包与组件的安全漏洞与 CVE 风险。
- **FIM (File Integrity Monitoring，文件完整性监控)** — 关键系统目录与面板运行文件的哈希与完整性变动监控，可作为特性开关按需开启（`features.fim`）。

### 11. SSH 管理与 Web 终端

- **SSH Key** — 面板管理的 SSH 密钥对（公钥/私钥），支持一键生成与下载。
- **Authorized Keys** — 服务器 `~/.ssh/authorized_keys` 授权公钥的管理与分发。
- **Terminal（Web 终端）** — 基于 WebSocket + PTY（xterm.js）的网页交互式 Shell 终端会话。

### 12. 后台任务

- **Background Task（后台任务）** — 由通用任务执行器（`internal/infra/task`）管理的一次后台执行单元。执行器提供：Task Key 去重（同键同时只跑一个）、任务级超时、失败重试、全局并发上限控制、终态保留（失败/取消保留至同 key 重装，成功即清）（见 ADR-0008）。
- **Task Key** — 后台任务的去重键兼查找句柄（key 唯一标识一次执行）。
- **Task Log（任务日志）** — 后台任务的可选附件：内存环形缓冲 + 游标回放，订阅者先回放已缓冲行再收实时行；不接收日志则不产生流式成本。

### 13. 监控与告警

- **System Monitor（系统监控）** — 实时采集与历史存储系统性能指标。历史存储（`MonitorPoint` 表）包含：CPU、内存、磁盘（根分区）、磁盘 IO、网络吞吐、系统负载；分区状态（`DiskPartition`）与 Top 进程（`SystemProcess`）通过独立 API 按需查询，不进入历史表。采集层通过 `MetricSource` 接口解耦（见 ADR-0003）。
- **Alert Rule（告警规则）** — 监控指标告警阈值配置（如 CPU > 90% 持续 5 分钟），触发告警生命周期。

### 14. 通知系统

- **In-App Notification（站内通知）** — 面板右上角铃铛消息（告警触发、安全事件、系统通知），通过 SSE（`/api/notifications/stream`，需 JWT 与 Session 双重验证）实时推送至前端。
- **Notification Channel（站外推送）** — 站外通知渠道配置，当前已实现：通用 Webhook、钉钉（DingTalk）、飞书（Feishu）、企业微信（WeCom）；通过 URL 模式识别渠道类型并适配消息格式（见 `internal/domain/notify`）。用于将重要告警向管理员外部设备分发。
  _Avoid_: 用 "Notification" 同时模糊指代站内铃铛与站外推送。

### 15. 审计与环境配置

- **Audit Log（审计日志）** — 操作审计记录（记录操作人、模块、动作、目标对象、IP 地址、响应状态与时间戳）。
- **Env Config（环境配置）** — 面板统一纳管的系统/应用通用环境变量与全局配置项（`env_configs` 表）。

---

## 已废弃与移除的领域概念 (Deprecated / Removed)

为了保持架构整洁，以下旧模块与概念已全面移除或重构，开发与阅读代码时应避免混淆：

1. **Deploy（部署同步）** — ⚠ **已全面移除**（见 issue #23 / PR #23）：原 `internal/deploy` 业务包、前端 Deploy 页面与路由、数据库 `deploy_*` 表及加密密钥逻辑已彻底清理。面板不承担多节点部署同步职责。
2. **Cloud Provider（云厂商集成 / 腾讯云管理）** — ⚠ **已全面移除**：原 `internal/cloud` 业务包、前端 Cloud 页面与路由、Tencent Cloud SDK 依赖以及面板设置中的腾讯云配置已彻底清理。EasyServer 专注于单机管理面板核心能力。
3. **ProcessManager** — ⚠ **已废弃**：旧文档中的进程管理器已被基于 systemd unit 的 **Process Guardian（进程守护）** 与 **System Process（系统服务）** 取代。
4. **Cron Log & cron_tasks 表** — ⚠ **已废弃**：定时任务日志全部由 **journald** 承载，任务定义以 systemd timer/service unit 为唯一权威。
5. **runtime_version 表** — ⚠ **已废弃**：运行时已全面基于 `$MISE_DATA_DIR/installs/` 目录与 `.easyserver-ok` 完成标记文件进行扫描与状态判断。

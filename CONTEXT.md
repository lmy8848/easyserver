# EasyServer 领域模型

> EasyServer 是一个 Linux 服务器管理面板。它让单个管理员通过 Web 界面管理服务器上的软件、网站、数据库、定时任务、防火墙等——无需直接 SSH。
>
> ⚠ 术语约定：文档用 "Service 层" 指 `internal/service/` 下的业务逻辑（如 CronService、FirewallService）；"Systemd Service" 指 Linux 系统服务（如 nginx.service）。代码中两者都叫 "service"，靠上下文区分。

## 核心概念

### 运行环境

- **服务器 (Server)** — 运行 EasyServer 的那一台 Linux 主机。EasyServer 是单机单管理员设计，不存在多节点集群。
- **单管理员 (Single Admin)** — 整个面板只允许一个管理员账户登录。CLI 工具 `cmd/cli/main.go` 可在紧急时重置密码。

### 网站托管

- **Web Server** — 已安装的 Web 服务器软件类型，如 Nginx、Apache、Tomcat、Caddy。每种都有预定义的安装/卸载命令、配置路径、服务名等属性。
- **Website** — 在某个 Web Server 下部署的站点。每个 Website 绑定一个域名、根目录、项目类型（静态/Node.js/PHP/Python/Java/反向代理），并由 Web Server 生成对应配置文件。

### 数据库管理

- **DB Type** — 数据库类型（MySQL、PostgreSQL、Redis）。每种类型有默认端口和预定义版本模板。代码中为 `DBType`（`internal/database/model.go`）。
- **Database Instance** — 某 DB Type 的受管数据库容器实例（例如 MySQL 8.0 运行在端口 3306）。它独占容器、数据卷和配置；一种类型可以同时有多个实例，宿主机安装的数据库不属于 EasyServer 的管理范围。
  _Avoid_: DB Version
- **Direct Connection（直连）** — 面板用数据库驱动（而非容器 CLI）经宿主映射端口访问 Database Instance 的通道。MySQL 与 PostgreSQL 的库/用户/表/表格浏览/SQL 执行走直连，拿到结构化类型与参数化查询；备份/恢复与配置编辑仍走容器 CLI。Redis 目前保持 CLI 通道。
- **Database** — 某个 Database Instance 下的实际数据库（例如 `myapp`）。
- **DB User** — 数据库用户，跨版本共享（同一类型下所有版本共用用户列表）。
- **DB Backup** — 数据库备份记录，关联到特定 Database。

### 定时任务

- **Cron Task** — 一条定时任务定义，承载为 systemd 的一对 `.timer`（OnCalendar 触发）+ `.service`（mise exec 执行）。状态读 `systemctl`，日志走 journald，重试/超时由 systemd 原生处理。任务用 UI 调度表单（预设频率）描述，后端转为 OnCalendar 表达式。
- **Cron Script** — 可被 Cron Task 引用的可复用脚本（sh/bash/python）。内容落盘 `/opt/easyserver/scripts/`，DB `scripts` 表仅存元数据（名称/语言/描述）。
_Avoid_: Cron Log（已弃用，日志由 journald 承载）

### 防火墙

- **Firewall** — 系统防火墙（iptables/nftables）。管理规则、区域、端口转发。

### 运行时

- **Runtime** — 编程语言运行时（PHP、Node.js、Python 等）。一期纳管：node / python / go / java / php。
- **Runtime Version** — 由 `mise` 接管安装的精确版本号（如 `20.11.0`）。版本号一旦落库不漂移；"升级到最新补丁"会产生新记录。
- **Global Default** — 通过 `/etc/mise/config.toml` 写入的系统级默认版本，仅服务 SSH 登录用户与未受面板控制的脚本；面板自身的执行流不依赖。
- **Execution Shim** — 面板主动发起的执行（Process、Cron）强制包裹为 `mise exec <lang>@<exact> -- <cmd>`，彻底隔离 PATH。托管 systemd unit（进程守护、定时任务）的 ExecStart 由面板生成时即带有此包裹。
- **Mirror Profile** — 存于 DB 的镜像 env 表（淘宝 / 华为 / 清华为默认 seed），生效后写入 `/etc/mise/config.toml` 的 `[env]` 段供 mise 读取，UI 可编辑。

### 容器

- **ContainerEngine** — 容器引擎类型：Docker 或 Podman。刻意避开 `Runtime`（该词已指编程语言运行时，见上「运行时」节），也不撞 OCI 的低层 container runtime。面板自动探测已装引擎，前端以双 tab 呈现；同一时刻每个请求只作用于一个引擎。API 全部收在 `/container/*` 前缀下，引擎通过 `?engine=podman` query 参数区分（缺省 `docker`）。
- **Managed Database Container** — 由 EasyServer 创建、标识并全生命周期管理的数据库容器。它是 DB Version 的运行载体，不是通用容器列表中可任意编辑的 Container。
- **Container** — 某个 ContainerEngine 下的容器实例。管理启动、停止、日志等。API 资源路径为 `/container/instances`。
- **Image** — 容器镜像，归属某个 ContainerEngine。
- **Compose Project** — 由 Compose 描述文件定义的一组服务。Docker 用 `docker compose`，Podman 用 `podman-compose`。
- **Volume** / **Network** — 容器卷与容器网络，均归属某个 ContainerEngine。

### 后台任务

- **Background Task（后台任务）** — 由通用任务执行器管理的一次后台执行单元。执行器保证：Task Key 去重（同键同时只跑一个）、任务级超时（可选，到点取消本次尝试的上下文并按失败处理）、失败重试（可选，固定间隔，重试期间状态仍为 running，日志连续追加）、全局并发上限（超出拒绝而非排队）、终态保留（失败/取消保留至同 key 重装，成功即清）。能力由调用方按需开启，非任务必选。
- **Task Key** — 后台任务的去重键，**同时也是查找句柄**（同一键同时只允许一个任务运行，因此键唯一标识一次执行）。数据库安装的 Task Key = 容器名（container ID）。- **Task Log（任务日志）** — 后台任务的可选附件：内存环形缓冲 + 游标回放，订阅者先回放已缓冲行再收实时行；不接收日志则不产生任何流式成本。

### 系统管理

- **Process** — ⚠ 已过期：旧文档称由 `ProcessManager` 管理，代码中已不存在该组件。术语避用，由 Background Task 承接"后台执行单元"的语义。
- **System Process** — 通过 `SystemProcessService` 管理的 systemd 服务。
- **Template** — 预定义的服务模板，一键安装常见软件（如 Nginx、MySQL）。

### 通知

- **Notification** — 发送给管理员的通知消息（Webhook/邮件等）。
- **Alert Rule** — 监控告警规则（CPU > 90% 持续 5 分钟 → 触发通知）。

### 审计与会话

- **Audit Log** — 操作审计记录（谁在什么时间对什么资源做了什么操作）。
- **Session** — 管理员的一次登录会话，有心跳机制和超时清理。

# 0010: 从 systemctl CLI 迁移到 go-systemd D-Bus 客户端

当前 EasyServer 在进程守护（`internal/domain/systemd`）、定时任务（`internal/domain/cron`）、容器引擎管理（`internal/domain/container`）以及 SSH 模块中，大量通过 `exec.CommandContext(ctx, "systemctl", ...)` 派生子进程与解析标准输出文本（如 `systemctl show` 的 `KEY=VAL` 和 `list-units` 列表）。痛点：高频与批量调用时产生显著的进程创建销毁开销与延迟抖动；文本解析脆弱，容易因不同 Linux 发行版及 systemd 版本格式差异引发解析 Bug；缺乏细粒度的 D-Bus 错误码与异步 Job 完成通知。

决定：引入 `github.com/coreos/go-systemd/v22/dbus`（纯 Go 实现，底层依赖 `godbus/dbus`，零 C 依赖），在基础设施层（`internal/infra/systemd`）封装统一的 `SystemdClient` 单例连接；将系统服务、进程守护、定时任务和容器引擎的 unit 控制与属性查询全面迁移至 D-Bus IPC 通信；日志读取与 SSE 流式推流继续保留 `journalctl` CLI 方案，杜绝 CGO 依赖以保障纯静态单二进制构建。

## Considered Options

- **全量迁移至 coreos/go-systemd（含 sdjournal）** -- 拒绝。`go-systemd/sdjournal` 强依赖 CGO（需在宿主或构建机安装 `libsystemd-dev` 动态链接库），破坏 Go 纯静态无依赖编译（`CGO_ENABLED=0`）以及跨平台交叉编译能力；且 C 接口指针生命周期与 SSE 流式读取结合复杂度极高。
- **维持现有 systemctl / journalctl CLI 方案** -- 拒绝。在高并发列表与详情扫描、批量状态读取时，频繁 fork 子进程带来不必要的 CPU 与延迟开销，且文本正则与字符串拆分维护成本高。
- **在各个 Domain 内部独立引入并建立 D-Bus 连接** -- 拒绝。导致多个业务领域重复维护 socket 连接生命周期、重连机制与上下文超时控制，不利于单例复用和测试 Mock。
- **将控制迁移至 D-Bus，日志保留 journalctl CLI（混合架构）** -- **采纳**。D-Bus 控制享受纯 Go、零 C 依赖、高吞吐、毫秒级响应与强类型；日志读取通过成熟的 `journalctl -f -o json` 管道流式传输，兼得二者最大优势。

## Consequences

- **分层架构**：
  - 在 `internal/infra/systemd` 抽象 `SystemdClient` 接口并提供基于 `go-systemd/v22/dbus` 的默认实现与 `MockSystemdClient`。
  - 维护单一复用的 D-Bus Unix Socket 长连接（通过互斥锁与懒加载重连兜底断线场景）。
  - `domain/systemd`、`domain/cron`、`domain/container`、`domain/ssh` 等业务包统一依赖 `infra/systemd` 接口。
- **阻塞与超时语义**：
  - 接口方法默认保持同步阻塞（等待 D-Bus Job 返回 channel 直到 "done" 或 context 超时），并在超时或错误时直接返回强类型结构化错误信息。
  - 涉及复杂/耗时任务（如批量安装与重载）仍可协同 `internal/infra/task` 执行器。
- **类型安全**：
  - 弃用 `systemctl show` 的文本解析逻辑，统一通过 `GetUnitPropertiesContext` / `GetUnitPropertyContext` 直接获取 `dbus.Variant` 强类型字典。
- **环境降级与测试友好**：
  - 在无 systemd 环境（如 macOS 本地开发、无 init 系统的 Docker 容器）中，连接初始化优雅返回 `ErrSystemdUnavailable` 而不导致服务启动崩溃；单元测试可无缝注入 MockClient。
- **构建环境保持纯净**：
  - 保持 `CGO_ENABLED=0`，不需要系统安装 `libsystemd` 开发包，Go 静态二进制构建体验与部署模式完全不变。

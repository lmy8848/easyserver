# 0011: 从 Docker / Podman CLI 迁移到原生 Unix Socket REST 客户端统一纳管

当前 EasyServer 在容器管理（`internal/domain/container`）中，通过 `exec.CommandContext(ctx, "docker"/"podman", ...)` 派生子进程并使用手写的 JSON Shim、正则表达式（如 `portMappingRE`）与字符串拆分来解析命令行输出（如 `docker ps --format json` 与 `podman ps --format json`）。痛点：高频与定时轮询（列表、统计、日志）反复 fork 进程带来显著的 CPU 抖动与延迟；Docker 与 Podman CLI 输出格式不一致（NDJSON vs JSON 数组、大写字段 vs 小写字段）导致大量脆弱的格式抹平代码；字符串解析易受不同引擎版本输出格式微调影响而产生隐蔽 Bug。

决定：在基础设施层（`internal/infra/container`）基于 Go 原生标准库（`net/http` + `net.Dial("unix", ...)`）自研超轻量 `EngineClient` 抽象。利用 Podman 3.0+ 提供的 Docker Engine 100% 兼容 REST API 端点（`/run/podman/podman.sock`），以纯 Go 标准库和 Socket 路由同时驱动 Docker（`/var/run/docker.sock`）与 Podman。**零外部第三方依赖，保持 `CGO_ENABLED=0` 与 `go.mod` 极致纯净**；Compose 部署与镜像源本地文件配置继续保留成熟方案。

## Considered Options

- **维持现有 docker / podman CLI 方案** -- 拒绝。高频调用下 fork 子进程开销显著，需针对 Docker 和 Podman 维护双套 JSON 结果解析结构体（如 `dockerPSRow` 与 `podmanPSRow`）与正则端口提取代码，维护成本高且容易出错。
- **引入 Docker 官方 Go SDK (`github.com/docker/docker/client`)** -- 拒绝。Docker 官方 SDK 依赖树庞大，在 `go.mod` 中引入数十个间接依赖（包括 Windows 命名管道 `go-winio`、OpenTelemetry、gRPC 等），增加不必要的包维护负担。
- **基于 Go 原生标准库（`net/http` + Unix Socket）自研轻量 `EngineClient`** -- **采纳**。Docker Engine API 是标准 REST 协议，利用 Go 标准库通过 Unix Domain Socket 发送 HTTP 请求并反序列化强类型结构体，实现 0 外部依赖、0 二进制体积膨胀、完全自主可控。

## Consequences

- **分层架构**：
  - 在 `internal/infra/container` 抽象 `EngineClient` 接口，提供基于 `net/http` + Unix Socket 的轻量实现与 `MockEngineClient` 测试桩。
  - 维护 Socket 连接池（Docker 连接 `/var/run/docker.sock`，Podman 连接 `/run/podman/podman.sock`），支持长连接复用与优雅关闭。
  - `internal/domain/container` 依赖 `EngineClient` 完成容器、镜像、网络与存储卷的强类型调用与数据模型转换。
- **Podman 兼容性**：
  - Podman 原生支持 Docker 兼容 REST API。
  - Podman 依赖 `podman.socket` 单元激活，EasyServer 已具备 D-Bus 管理 `podman.socket` 的能力。
- **强类型与流式读取**：
  - 容器列表、详情、镜像、存储卷、网络、Stats 统计完全使用强类型结构体。
  - 日志读取基于标准 8 字节多路复用帧头解复用（`DemuxLogs`），原生安全高效。
- **构建环境与测试体验**：
  - 保持 `CGO_ENABLED=0`，零外部依赖，`go.mod` 保持绝对干净。
  - 单元测试可直接通过 `MockEngineClient` 注入模拟数据，无需在测试机运行真实的 Docker/Podman 守护进程。

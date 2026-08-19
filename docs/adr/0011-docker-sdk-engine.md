# 0011: 从 Docker / Podman CLI 迁移到 Docker 官方 Go SDK 统一纳管

当前 EasyServer 在容器管理（`internal/domain/container`）中，通过 `exec.CommandContext(ctx, "docker"/"podman", ...)` 派生子进程并使用手写的 JSON Shim、正则表达式（如 `portMappingRE`）与字符串拆分来解析命令行输出（如 `docker ps --format json` 与 `podman ps --format json`）。痛点：高频与定时轮询（列表、统计、日志）反复 fork 进程带来显著的 CPU 抖动与延迟；Docker 与 Podman CLI 输出格式不一致（NDJSON vs JSON 数组、大写字段 vs 小写字段）导致大量脆弱的格式抹平代码；字符串解析易受不同引擎版本输出格式微调影响而产生隐蔽 Bug。

决定：引入 Docker 官方 Go SDK（`github.com/docker/docker/client`，纯 Go 实现，零 CGO 依赖），在基础设施层（`internal/infra/container`）封装统一的 `EngineClient` 抽象。利用 Podman 3.0+ 提供的 Docker Engine 100% 兼容 REST API 端点（`/run/podman/podman.sock`），以统一的一套 Docker SDK 代码和 Socket 路由同时驱动 Docker 与 Podman。保持 `CGO_ENABLED=0` 静态编译；Compose 部署与镜像源本地文件配置继续保留成熟方案。

## Considered Options

- **维持现有 docker / podman CLI 方案** -- 拒绝。高频调用下 fork 子进程开销显著，需针对 Docker 和 Podman 维护双套 JSON 结果解析结构体（如 `dockerPSRow` 与 `podmanPSRow`）与正则端口提取代码，维护成本高且容易出错。
- **引入 Podman 官方 Go Bindings (`containers/podman/v5/pkg/bindings`)** -- 拒绝。Podman 官方 SDK 包含了庞大的内部存储驱动与底层 cgroup 模块，引入会导致上百个重型间接依赖，并极易破坏 `CGO_ENABLED=0` 的纯净静态编译体验。
- **Docker 官方 Go SDK 统一纳管（对接 Docker Socket 与 Podman Compat Socket）** -- **采纳**。Podman 原生设计了完全兼容 Docker Engine REST API 的 Socket 端点。使用一套官方 Docker Go SDK + API 版本自动协商（`WithAPIVersionNegotiation`），既能彻底消灭胶水代码与子进程开销，又无需引入复杂的双套第三方 SDK。

## Consequences

- **分层架构**：
  - 在 `internal/infra/container` 抽象 `EngineClient` 接口，提供基于 `github.com/docker/docker/client` 的标准实现与 `MockEngineClient` 测试桩。
  - 维护 Socket 连接池（Docker 连接 `unix:///var/run/docker.sock`，Podman 连接 `unix:///run/podman/podman.sock`），支持懒初始化与长连接复用。
  - `internal/domain/container` 依赖 `EngineClient` 完成容器、镜像、网络与存储卷的强类型调用与数据模型转换。
- **Podman 兼容性与 API 版本自动协商**：
  - 客户端构建配置 `client.WithAPIVersionNegotiation()`，SDK 自动握手降级适配 Podman 4.x/5.x 支持的 Docker API 版本（1.41~1.44），消除版本过高报错。
  - Podman 依赖 `podman.socket` 单元激活，EasyServer 已具备 D-Bus 管理 `podman.socket` 的能力。
- **强类型与流式读取**：
  - 容器列表、详情、镜像、存储卷、网络、Stats 统计完全使用 SDK 强类型结构体（`types.Container`、`image.Summary`、`types.StatsJSON` 等）。
  - 日志读取与 Exec 执行基于多路复用流（`stdcopy.StdCopy`），原生安全高效。
- **构建环境与测试体验**：
  - 保持 `CGO_ENABLED=0`，完全不依赖外部 C 库与动态链接，构建体验与一键安装脚本保持纯净。
  - 单元测试可直接通过 `MockEngineClient` 注入模拟数据，无需在测试机运行真实的 Docker/Podman 守护进程。

# 容器管理同时支持 Docker 与 Podman

容器管理当前完全绑定 Docker CLI：`internal/container/service.go` 每个方法硬编码 `docker ...` 子命令，系统级操作（安装、守护进程启停、`daemon.json` 镜像加速）都是 Docker 专属。决定扩为同时支持 Docker 与 Podman。**Service 保持单实例**，API 层全部收拢到 **`/container/*`** 一个前缀下，引擎通过 `?engine=podman` query 参数区分（缺省 `docker`）；handler 从 query 取引擎作为参数传入 Service 方法；Service 内部按引擎选二进制，JSON 解析集中在一处分发。不加方法层 Go 接口。

## 术语

- **ContainerEngine**（容器引擎）：Docker 或 Podman 的抽象。刻意避开 `Runtime` —— 该项目里 `Runtime` 已指编程语言运行时（`internal/runtimeenv`），且 OCI 的 "container runtime" 特指 runc/crun/containerd 这类低层运行时。用 `ContainerEngine` / `?engine=` 与两者都错开。
- **Container**（容器实例）：某个 ContainerEngine 下的容器实例，API 资源路径为 `/container/instances`。

## 关键事实（驱动本轮选择）

- Podman CLI 在容器 CRUD 子命令上与 Docker 几乎兼容（`--format json` 模板语法一致），但：
  - **JSON 输出形态不同**：Docker 的 `ps/images/volume ls --format json` 是逐行 NDJSON + 大写字段；Podman 是单个 JSON 数组 + 小写字段。当前逐行 `Unmarshal` 到大写 key shim 的逻辑对 Podman 整个失效。
  - `podman compose` 委托外部 provider `podman-compose`，且 `compose ls --format json` 不支持。
  - Podman 无守护进程，rootful/rootless 两套 systemd 单元语义不同。
- **路由冲突**：`internal/runtimeenv` 已注册 `GET /runtime`、`POST /runtime/install` 等语言运行时路由。容器系统级路由若沿用 `/runtime/...` 会与 `POST /runtime/install` 硬冲突（Gin 启动 panic）。故容器系统路由与资源路由统一收拢到 `/container/*`。

## Considered Options

- **两 Service 实例 + engine 字段** -- 拒绝。探测时为每个已装引擎各建 `*Service` 持 `engine`/`binary` 字段，handler 按 `?engine=` 选实例。理由：与"单 Service 实例"相悖，且会引入按请求在 handler 层选实例的复杂度。
- **每请求传 engine 参数 + 方法内分支** -- 拒绝。约 30 个方法内部都要 `if engine == "podman"` 分支选二进制与 shim，分支散落。
- **方法层 Runtime 接口 + Docker/Podman 两实现** -- 拒绝。CRUD 代码 95% 相同，接口会让每个方法都变两遍，属过度抽象。
- **两套 API 路由（`/docker/*` 与 `/podman/*`）** -- 拒绝。曾短暂采用，但让同一资源出现两套路径，前端与代理配置都要跟着分叉。
- **沿用 `/runtime/*` 系统路由** -- 拒绝。与 `internal/runtimeenv` 的 `POST /runtime/install` 等硬冲突，且 `Runtime` 术语与语言运行时领域混淆。

## Consequences

- 单 Service 实例，方法签名带 `engine string` 参数；系统级操作（安装/启停/镜像加速）Docker 与 Podman 各一套实现。
- 一套路由全部收在 `/container/*`：系统级 `/container/status|install|start|stop|restart|info|mirror`，资源 `/container/instances|images|compose/*|volumes|networks`，均以 `?engine=` 区分，缺省 `docker` 向后兼容。
- Podman 系统级操作按 **rootful** 语义：`systemctl` 管 `podman.service`/`podman.socket`；安装走 `detectOS` 的发行版包管理器（apt/dnf/zypper）；镜像加速写 `/etc/containers/registries.conf`。
- 前端双 tab「Docker / Podman」，未装侧置灰；每个 tab 的请求带对应 `?engine=`。
- Podman Compose 的 `ls --format json` 不支持 → 项目列表走文本解析或暂空。
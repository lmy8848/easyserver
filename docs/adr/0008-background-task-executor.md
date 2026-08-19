# 统一后台任务执行器

后台任务（数据库安装、脚本执行、运行时安装）分散在五处实现，各自维护任务生命周期与日志流（`installer` 的环形游标、`ScriptRunner` 的订阅广播、`runtimeenv` 的 DB progress 行），订阅模型、日志信封、SSE 样板互不兼容。我们决定抽出一个通用后台任务执行器（`internal/infra/task`）统一任务生命周期：去重、取消、状态机、超时、重试、并发上限按需开启；**流式日志降为可选附件**（`Task Log` 内存环形缓冲 + 游标回放），调用方不接日志则不产生任何流式成本——日志不应反向绑架任务模型。

## Status

proposed

## Considered Options

- **闭包任务 + 可选日志注入（选定）**：`Start`/`StartWithLog` 双入口，无日志任务不感知 log 参数。相对单入口 + 可空 log：nil 判断不会泄漏进每个任务体。
- **进程级任务（`RunProcess`，拒绝/延后）**：内置 stdout/stderr 双泵 + Setpgid 杀进程组，把 cron `runPumps`/runtimeenv `runStreaming`/executor `RunStream` 三套管道泵合一。改动量大，且会放大本轮改造的流式成分，与"执行器为主、日志可选"的判断相悖。留待后续。
- **终态 TTL（拒绝）**：installer 现状按 TTL 保留终态日志。改为**失败/取消保留至同 key 下一次启动**（成功即清），map 天然有界、无需 TTL 清理器，且重装语义（新的成功/失败取代旧记录）与"重新安装=卸载+安装"一致。
- **并发超出排队（拒绝）**：需要 queued 状态 + 有界队列 + 排队取消 + 排队超时，等于把执行器升级成有界调度器。单管理员低频任务撞上限概率极低，超限**拒绝**比排队划算。
- **`byID`/`byKey` 双 map（拒绝）**：曾以 `id` 为查找句柄、`key` 为去重键。审查后确认查找句柄可以用去重键承担（key 同时只运行一个任务 → key 唯一标识执行），**去重键即查找句柄，单 `byKey` map** 即可，无独立 id。
- **去重粒度（per-DBType → per-container）**：installer 用 DBType 做锁（同类型只能装一个）。迁移改为 **key=containerID**——同类型不同版本可并行安装（互不冲突），同版本重复安装仍被 repo 层 `CountInstancesByDBTypeAndVersion` 挡住；SSE/取消按 containerID 查找不需要 DBType 间接。
- **`ActiveInstalls` 端点（删除）**：`GET /db/installs` 与 `ActiveInstall`/`ActiveInstalls()`/`ListActiveInstalls` 是旧 modal 流程残留，前端从未调用（"正在安装"由实例行 `status='installing'` 渲染）。删除，连带 `ActiveInfo` 收敛为 `ActiveKeys() []string`。

## Consequences

- 超时归入 `failed` 并触发重试（每次尝试拿全新 ctx）；用户取消优先，永不重试；状态机 `running → succeeded / failed / canceled`（无 queued）。
- 并发上限由调用方构造时指定，数据库安装迁移用默认 8。
- `succeeded` 完成即清 → 刷新后回看成功日志会显示已丢失；接受（前端在完成时刻已拿到全部日志行）。
- 数据库 installer 迁移为第一个使用者，作为形态验证；cron/runtimeenv 迁移与 SSE 传输样板收敛另立任务。
- 同类型不同版本可并行安装（此前 per-DBType 串行）；同版本重复安装仍被 DB 行唯一约束挡住。

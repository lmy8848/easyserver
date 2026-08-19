# Monitor 采集引入 MetricSource seam

`MonitorService.collect()` 是监控热路径，但 `/proc` 采集硬编码在调用链里、无 seam 注入受控数据，导致告警评估、审计阈值触发与冷却、环形缓冲写入等编排逻辑无法单测。已有 `Repository`/`Evaluator`/`SystemEventLogger` 三个接口 seam，唯独采集层缺失。决定引入第四个接口 `MetricSource`（`Read() *MonitorPoint`），`NewMonitorService` 内部默认创建 Linux 实现，测试用同包构造注入假实现；范围仅限 `collect()` 热路径的 6 个指标，不含进程监控与按需 API。

## Considered Options

- **直接读 `/proc`，不抽 seam** -- 拒绝。`collect()` 编排无法单测，告警/审计阈值边界无验证，每次改锁/协程都盲飞。
- **文件系统 mock `/proc`** -- 拒绝。`readDisk` 走 `syscall.Statfs`（系统调用，非文件读取），无法以假文件替换，6 个指标至少 1 个覆盖不了。
- **functional option 注入 MetricSource** -- 拒绝。`NewMonitorService` 构造时即 `go s.run(ctx)`，option 在 goroutine 启动前应用虽可避免竞态，但同包测试用 struct literal 直接构造更简单，且 wire 与 `NewMonitorService` 签名零改。字段类型声明为接口即可让测试替换。
- **`Read() (*MonitorPoint, error)` 带 error** -- 拒绝。既有 6 个 read 各自容错、部分失败留零值是稳定行为；引入 error 要么整体失败丢数据，要么 `collect()` 增加错误处理分支，属行为变更、超出"补测试 seam"的范围。
- **纳入进程监控 / `readSystemInfo` / `readPartitions`** -- 拒绝。它们是按需 API（用户点开页面才调），不在 `collect()` 热路径，纳入会扩大改动面且不服务于"可测 collect()"这一目标。

## Consequences

- `service.go` 从 ~1030 行降至 ~780 行（非外部架构报告渲染的 ~200 行；该报告未考虑范围限定）。
- `MetricSource` 是**有状态** seam（持有 CPU/网络/磁盘 IO 的差值累计值），与无状态的 `Repository`/`Evaluator` 不同；由 `collect` 所在单一 goroutine 独占，无需加锁。
- 6 个 `/proc`+`syscall` 解析方法迁为 `linuxMetricSource` 私有方法后**仍不可单测**（`syscall.Statfs` 限制）--这是已知缺口，本次不补。
- `lastCleanup`（DB 清理时间戳）留在 `MonitorService`，作为编排状态与采集状态分离的体现。

# 定时任务从系统 crond 迁移到 systemd timers

当前 `internal/cron` 依赖系统 crond 触发：`SyncToSystemCrontab` 写 `/etc/cron.d/easyserver`，面板无进程内调度器，`executeTask`（重试/超时/日志）只在 `RunNow` 手动触发时由面板执行。痛点：`/etc/cron.d` 在精简系统/容器缺失导致同步失败；重试/超时/日志/状态全在 Go 里自研，与 `internal/systemd`（进程守护迁移后）已具备的「unit 生成 + daemon-reload + systemctl 状态 + journald 日志」能力重复。决定：把定时任务承载从 crond 全面迁到 systemd timer——`.timer`（OnCalendar 触发）+ `.service`（mise exec 执行）一对，复用 `CreateManaged`/`RenderUnit`/`writeUnitFile` 全套；状态读 `systemctl show`，日志走 journald，重试/超时交给 systemd 原生（`Restart=on-failure`+`StartLimitBurst`+`TimeoutStartSec`）；删 `cron_logs` 表与 `executeTask` 的 Go 执行逻辑，`cron_tasks` 表删（timer unit 是唯一权威），脚本内容落盘 `/opt/easyserver/scripts/`，`scripts` 表降为元数据（name/lang/description）。

## Considered Options

- **保留 crond，仅补 `MkdirAll(/etc/cron.d)`** -- 拒绝。只消除报错，不解决 crond 缺失时任务根本不触发、以及重试/超时/状态/日志自研与 systemd 能力重复的问题。
- **面板自研进程内调度器** -- 拒绝。与 session 内存化、process→systemd 的既有方向背道而驰；面板重启即丢调度、无 cgroup 限制、日志/状态自研，重造 systemd 已有轮子。
- **cron 表达式完整映射到 OnCalendar** -- 拒绝。crontab 的 `L`（月末）/`W`（最近工作日）/`#`（第 N 个星期几）在 OnCalendar 无对应，完整映射不可行。改为 UI 结构化调度表单（预设频率）→ 转换器生成固定几种 OnCalendar 形态，从源头规避 esoteric 特性。
- **脚本内容内联进 ExecStart** -- 拒绝。脚本更新需重写 unit + daemon-reload；改为落盘 `/opt/easyserver/scripts/<id>.sh`，ExecStart 指向文件，脚本更新只重写文件。
- **保留 cron_tasks 表作配置源，unit 为投影** -- 拒绝。与进程守护「unit 文件即权威」不一致，多一张镜像同步表；改为删表，UI 任务列表直接读 systemd。
- **日志保留 SQLite cron_logs** -- 拒绝。与进程守护统一走 journald，删 `cron_logs` 表，少一层镜像同步。

## Consequences

- **行为变化**：`Persistent` 默认 off（严格到点执行，与 cron 一致），创建时可选 persistent。计划任务创建时可选。
- **cron 表达式弃用**：用户不再填 5 字段 cron，改 UI 预设频率表单；`schedule` 字段语义变为 OnCalendar 表达式（由转换器产出）。
- **命名隔离**：timer/service 用 `easyserver-cron-<name>` 前缀，与进程守护的 `easyserver-<name>` 不冲突。
- **手动迁移**（Q15）：旧 `/etc/cron.d/easyserver` 与 `cron_tasks` 数据由发布时手动清理，代码不自动迁移。
- **状态读取**：`status`/`last_run`/`last_result` 由 `systemctl show` 提供的 ActiveState/Result/ExecMainExitTimestamp 映射，非实时轮询到具体时间戳，放弃 DB 持久化 last_run。
- **校验**：OnCalendar 表达式用 `systemd-analyze calendar` 校验并推导 next_run。
- **时区**：系统时区，不引入每任务时区设置。
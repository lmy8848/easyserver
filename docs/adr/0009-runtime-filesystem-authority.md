# 运行环境权威从 runtime_version 表迁移到 installs/ 文件系统

`runtime_version` 表是运行环境（lang@exact → 状态/路径）的唯一权威：面板安装写入行、列表查行、cron/systemd 绑定查行回填。这张表有三层问题：

1. **双写源会漂移**。mise 实际装到 `$MISE_DATA_DIR/installs/<tool>/<version>/`，表里 `status='installed'` 的行只是面板自记的副本。用户手动 `mise install node@22`、或面板崩溃留下半截目录，表与磁盘就不一致；「权威」名不副实。
2. **行号不是绑定语义**。cron/进程守护把 `RuntimeVersionID`（DB 自增 id）写进 unit 注释，`fillRuntime` 再查表回填 lang/exact。磁盘上真相（`RuntimeLang`/`RuntimeExact`）与索引（行号）并存，多一层「行号→内容」的间接层。
3. **状态列是过程态混入终态**。`installing`/`failed`/进度/日志本质是后台任务的过程态，却按行持久化，与 ADR-0008（后台任务走 `internal/infra/task` 内存执行器）重复。进程态与安装产物（目录）混在一张表里。

决定：运行环境列表从 `installs/` 目录直接扫描生成；面板安装成功后写完成标记文件（`.easyserver-ok`），扫描只认带标记的目录；`runtime_version` 表删除；过程态（安装中/失败/进度/日志）只存在 `internal/infra/task` 内存执行器，面板重启即失。

## Considered Options

- **保留 DB 表，仅把目录扫描作为回填** — 拒绝。目录扫描已是真相，再同步一张表是纯镜像开销；且无法解决"面板崩溃残留半截目录"（表与盘都不准，除非 HealState 猜）。
- **纯目录扫描、无完成标记** — 拒绝。mise 解压直接进 `installs/`，崩溃残留的半截目录会被当成已安装，绑定 cron 后执行必炸。`mise ls --installed --json` 实测把空目录也报 `installed: true`，等价于目录存在，不能作为完成判据。
- **校验 `bin/<tool>` 存在** — 拒绝。五种工具各自特判（`bin/node`、`bin/python3`、`bin/go`、`bin/java`、`bin/php`），脆弱且每个工具不同；完成标记是统一判据，与"文件即权威"自洽（标记本身也是文件）。
- **派生稳定数字 id（hash lang@exact）** — 拒绝。API 形态可不变，但旧 cron 任务 unit 里存的是真 DB id，对不上新 hash，照样全量迁移；且 id 掩盖语义，`node@20.11.0` 字符串本身就是绑定键（unit 文件早以 Lang/Exact 持久化）。

## Consequences

- **绑定键统一为 `lang@exact` 字符串**：cron 任务/进程守护 unit 注释的 `RuntimeLang`/`RuntimeExact` 是绑定真相，`# RuntimeVersionID=` 注释删除（旧文件里残留的忽略，解析向后兼容）；`fillRuntime` 从「按行号查 DB」改为「按目录存在性校验」。
- **前端 Select value 收敛为 `lang@exact`**：`RuntimeVersionSelect` 的 `{id, lang, exact}` 对象改为字符串；cron 表单字段 `runtime_version_id` 改名 `runtime`；ProcessGuardian 三字段 `runtime_version_id/runtime_lang/runtime_exact` 收敛为 `runtime`。
- **过程态只存内存**：安装中/卸载中/失败/进度/日志仅存在于 taskMgr，重启后失败安装无痕（列表显示"未安装"）；`install_error` 不落盘。
- **手动 `mise install` 的版本不带 `.easyserver-ok` 标记**，面板列表不显示（标注已知限制）。
- **`websites.runtime_version_id` 列删除**（无 FK、从未参与执行、仅元数据），Website 前端去掉运行时展示。
- **packages API 从 `?runtime_id=123` 改为 `?runtime=node@20.11.0`**，后端从扫描结果解 path。
- **迁移**：`000009_runtime_filesystem.down.sql` 重建表；`up.sql` 删除 `runtime_version` 表与 `websites.runtime_version_id` 列。旧 cron/进程守护 unit 文件含残留 `RuntimeVersionID=` 注释，解析忽略，无需手动迁移。
- 手动迁移惯例延续 ADR-0004（代码不自动迁移历史数据）。

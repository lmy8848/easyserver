# mise 落地实施细则

补充 [ADR-0001](0001-use-mise-for-runtime-management.md) 的具体执行细节。所有数值与机制在此一次性敲定；实现期偏离必须回头更新本文。

## 1. mise 自身

- 锁定版本 **`v2026.6.14`**，随 EasyServer 发布升级，禁用自动更新。
- 安装路径 `/opt/easyserver/mise/bin/mise`。下载源走 GitHub releases，国内镜像兜底（详见 §6）。
- 数据目录 `MISE_DATA_DIR=/opt/easyserver/mise`，config 根 `MISE_CONFIG_DIR=/opt/easyserver/mise`，在所有调用位点显式声明，不依赖 `$HOME`。
- **全隔离**：二进制、数据、config 全部自包含在 `/opt/easyserver/mise` 一个目录，不写 `/etc/profile.d/mise.sh`、不写 `/etc/mise/config.toml`、不碰用户 shell PATH。卸载面板 = 删该目录，零系统残留。面板进程内 PATH 会前缀注入 shims（仅影响面板自身及其子进程，供包管理用），不侵入用户 shell。

## 2. 一期支持的 Runtime

`node` / `python` / `go` / `java`（jdk）/ `php`。

其余语言（rust / ruby / erlang / dotnet 等）暂不开放面板入口；用户自行在 SSH 中 `mise install` 不阻拦但面板不纳管。

## 3. Runtime Version 解析

- UI 中用户只选主版本（如 "Node 20"）。
- 触发安装的瞬间调用 `mise latest <lang>@<major>` 解析为精确版本（如 `20.11.0`），写入 DB。
- 已落库的版本号不漂移。"升级到最新补丁"是显式动作，产生**新的** Runtime Version 记录，旧版本继续保留直到无引用。

## 4. Execution Shim

仅作用于面板**主动发起**的执行入口：

- **Process**（`internal/process`）：启动子进程时包装为
  ```
  /opt/easyserver/mise/bin/mise exec <lang>@<exact> -- <cmd> <args...>
  ```
- **Cron**：写入 crontab 的命令行形如：
  ```
  MISE_DATA_DIR=/opt/easyserver/mise MISE_CONFIG_DIR=/opt/easyserver/mise /opt/easyserver/mise/bin/mise exec <lang>@<exact> -- <cmd>
  ```
  不走 `bash -lc` / login shell，避免依赖 cron 环境的隐式 PATH。

**不**作用于 Systemd Service：面板对 systemd unit 只做 list / start / stop / enable / logs，从不写 unit 文件，因此 unit 内的命令是否走 mise exec 由用户决定，面板不介入。

## 5. Global Default

- 写入路径 **面板私有 `/opt/easyserver/mise/config.toml`**（由 `MISE_CONFIG_DIR` 指向），**不是** `/etc/mise/config.toml`，也**不**写 `/etc/profile.d/mise.sh`。
- 因 config 归面板私有且不碰用户 shell，**"SSH 用户直接 `node` 用默认版本"的能力被砍**。Process / Cron / Systemd 一律用 `mise exec <tool>@<exact>` 显式指定版本，不依赖默认。DB 的 `global_default` 表与 UI 默认逻辑保留，仅影响 `mise exec` 不带版本时的解析。
- mise 的 config 查找根指向面板目录后，**完全不读用户 `~/.config/mise/config.toml`**——用户自己的 mise 配置不受面板影响，面板也不被用户配置污染。

## 6. 镜像 (Mirror Profile)

- 镜像配置以一张 DB 表 `runtime_mirror` 承载，UI 可编辑/启用/禁用。
- 生效路径：DB 中启用的镜像 env 被写入面板私有 `/opt/easyserver/mise/config.toml` 的 `[env]` 段，mise 自身读取。
- 经官方文档校对（见 [.scratch/mise-refactor/validation.md](../../.scratch/mise-refactor/validation.md)），可用的 native mirror env **只有两条**：
  - **node**：`MISE_NODE_MIRROR_URL`
  - **go**：`MISE_GO_DOWNLOAD_MIRROR`（业务侧另设 `GOPROXY`，写入同一 `[env]` 段）
- 面板首次启动时仅 seed 这两条（淘宝 / 华为 / 清华各候选一份，默认启用其中一条）。
- **python / java / php 没有 native mirror env**：
  - python 默认走 `astral-sh/python-build-standalone` GitHub releases；
  - java 走 Foojay API；
  - php 通过 `vfox-php` 编译安装。
  
  这三类**保持 mise 默认下载行为**，镜像表不为其 seed 任何条目，UI 也不显示镜像编辑入口。
- 第二期可考虑：python 切编译模式（`MISE_PYTHON_COMPILE=1` + 镜像 pyenv）、php-build 自定义镜像、离线 tarball 注入等扩展。**本期不实现。**

## 7. 安装并发

- 同一 `<lang>@<exact>` 串行：进程内 `sync.Map<string, *sync.Mutex>`。
- UI 在已有同 key 任务运行时直接拒绝新请求，并展示进行中任务的实时进度。

## 8. 卸载语义

Runtime Version 在卸载前必须**无引用**。引用来源外键：

- `process.runtime_version_id`
- `cron_job.runtime_version_id`
- `global_default.runtime_version_id`

存在引用时 API 直接拒绝；UI 展示完整引用列表，要求用户先解绑或换绑后重试。

## 9. 数据迁移：硬切换

不保留对旧数据的运行时兼容。重构发布版本附带一次性迁移脚本：

1. 将现存 `process` / `cron_job` 两表导出为本地 JSON 备份（路径写入审计日志）。
2. 清空两表数据。
3. 新 schema 中 `runtime_version_id` 设为 **NOT NULL**，用户须在 UI 重新创建条目。
4. UI 升级后一次性弹窗：「运行环境模块已重构，旧任务备份位于 …，请重新创建」。

后续版本不再提供从旧 schema 自动恢复的路径，备份文件由用户自行处理。

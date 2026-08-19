# 数据库管理全面迁移为受管容器

EasyServer 现有数据库管理以 apt、systemd、宿主机配置文件和客户端命令为运行基础，导致宿主机侵入高且无法可靠发现面板外安装。决定将 MySQL、PostgreSQL 与 Redis 的受管数据库全面迁移为容器：不再提供或兼容宿主机安装模式；每个 Database Instance 均由 EasyServer 创建并管理的数据库容器承载，数据持久化、状态、日志、备份和恢复均以该容器为权威。

## Consequences

- 移除数据库模块对 apt、systemd、`/etc` 数据库配置及宿主机数据库客户端的依赖。
- 宿主机上既有数据库不会继续受 EasyServer 管理，本次不提供数据迁移或容器接管能力。
- 普通 Container 与 Managed Database Container 分属不同资源模型：后者不允许通过通用容器页绕过数据库生命周期约束。

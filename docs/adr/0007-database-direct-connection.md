# MySQL 与 PostgreSQL 逻辑管理迁移为数据库驱动直连

容器化数据库的"逻辑管理"操作（库/用户/表/表格浏览/权限/SQL 执行）当前走 `docker exec <container> mysql/psql -N -B` 文本输出再 `strings.Split` 解析——类型与 NULL 丢失、值含 tab/换行/BLOB 即碎、无参数化、无事务。决定将 MySQL 与 PostgreSQL 的这类操作迁移为数据库驱动（`database/sql` + go-sql-driver/mysql、pgx/v5 stdlib）经容器暴露的宿主端口直连。**备份/恢复、配置编辑、容器生命周期保留 CLI**（驱动层没有 mysqldump/pg_dump 等价物，配置是文件编辑不是查询）。**Redis 暂不迁移**（独立 API，key 浏览的文本解析痛感最低，收益/成本比不足）。

## Status

proposed —— 等待容器迁移 PR（`feat/database-container-migration`）合并进 master 后从 master 新分支实施。

## 关键决策

- **ContainerPort 恒为引擎默认端口**（MySQL 3306 / PostgreSQL 5432 / Redis 6379），修复 `service.go:594` 把用户端口同时用作容器内端口的映射 bug（`--publish X:X` 把宿主端口映射到容器内无服务的端口）。用户选择的端口只作宿主映射端口（HostPort）。
- **老实例兼容**：新增 `container_port` 列。新实例落库引擎默认端口；老实例 `Port == 引擎默认端口` 者回填为可直连，否则留 0、走 CLI 回退并在 UI 提示端口映射异常、建议改端口重建。
- **值序列化**：直连返回原生类型，`QueryTable` 同时返回 `ColumnTypes`；NULL / BLOB / 时间 / 数字的展示由前端按列类型处理。
- **多语句**：MySQL 池开 `multiStatements=true`（ExecuteSQL 用户原文执行本就是管理面板预期；参数化路径的值走 driver 绑定，不拼 SQL 文本）。
- **测试**：sqlmock 模拟 `sql.Rows` 喂直连层，CI 不引入 docker；现有 MockExecutor 测试保留，覆盖 CLI 回退路径。
- **引擎覆盖顺序**：先 MySQL + PostgreSQL（共享一套 `database/sql` 行扫描骨架）；Redis 留 CLI 观察，何时需要更丰富的 key 浏览再评估 go-redis。

## Consequences

- go.mod 新增两个依赖：go-sql-driver/mysql、jackc/pgx/v5。
- 直连依赖宿主端口映射正确性与容器运行态；MySQL 8 `caching_sha2_password` 非 TLS 首次握手需 dsn 加 `allowPublicKeyRetrieval=true`。
- 每实例一个惰性连接池缓存（`map[instanceID]*sql.DB` + 互斥锁），实例卸载时 `Close`；池设 `ConnMaxIdleTime` 防空转。
- 已用坏端口映射创建的老实例部分回退 CLI，提示用户改端口重建以吃直连收益。
- Redis 保持 CLI 是**有意的**，不是未完成迁移。

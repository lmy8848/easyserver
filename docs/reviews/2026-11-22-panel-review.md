# EasyServer 面板系统审查与功能增强建议

日期：2026-11-22
范围：`internal/`（20 个领域包）、`web/src`、插件系统、认证/会话/直连通道、CI 与发布。
方法：静态阅读 + 定向检索（本次为增量审查，不重复 2026-10-29 全量报告）。

## 一、总体评估

面板已经是一个功能面很宽的产品：服务器/容器/数据库/网站/防火墙/SSH/终端/文件/计划任务/环境运行时/安全扫描/插件市场一应俱全，共 20 个领域包、约 300 个 Go 文件、72 个测试文件；认证侧近期完成了 refresh token（不透明、设备绑定、轮换与重放吊销）、移动端直连通道（Ed25519 签名 + 指纹固定）、会话踢出自愈等加固；CI 覆盖 lint、govulncheck、`go test -race`、前端 lint/tsc/test/build、插件与 SDK 独立编译，发布前有启动冒烟。

结论：**功能广度足够，短板集中在"可观测性、运维闭环"两块（面板定位为单用户产品，权限模型不在范围内）**，以及少数性能/工程债。

## 二、确认的缺口（带证据）

| 编号 | 缺口 | 证据 | 影响 |
|---|---|---|---|
| G1 | 没有 Prometheus `/metrics`，也没有结构化健康指标 | 全仓库 `internal/` 无 `prometheus` 引用；仅有 `/health` 返回 `{"status":"ok"}`（`internal/api/router.go`） | 外部监控（Grafana/Uptime）只能做存活探测，看不到面板自身指标（请求量/延迟/错误率/队列） |
| G2 | 通知只有 Webhook 一种渠道 | `internal/domain/notify/service.go:44` 起仅 webhook 实现；无 SMTP 依赖 | 多数团队没有钉钉/飞书机器人，告警无法落地到邮箱/IM |
| G3 | ~~只有单一 `admin` 角色~~ | `internal/domain/auth/model.go:11` 仅 `RoleAdmin` | **不在范围内**：面板定位为单用户产品，不做多角色/多租户 |
| G4 | 没有"面板自身"的备份/迁移 | 备份能力只在 `internal/domain/database/`（业务数据库备份） | 换机/灾难恢复要手工拷 `config.toml`、`data/easyserver.db`、站点目录、插件目录 |
| G5 | 没有可吊销的 API 令牌 | 无 PAT/ApiToken 实现，自动化只能复用登录 JWT | CI/脚本要拿长期凭据，且无法按用途吊销 |
| G6 | SQLite 读也被串行 | `internal/infra/database/database.go:42` `SetMaxOpenConns(1)` | 面板多标签/监控轮询时排队，写锁竞争放大（当前规模可接受，但留了天花板） |
| G7 | 前端 i18n 缺失、`any` 债 | 全中文硬编码；`as any/: any` 上百处 | 无法国际化；类型边界弱 |
| G8 | 发布产物无签名/attestation | `.github/workflows/release.yml` 仅 sha256 | 供应链完整性只能靠人工比对 |

## 三、功能增强建议（按价值/成本排序）

| 优先级 | 增强 | 内容 | 成本 | 风险 |
|---|---|---|---|---|
| P1 | **面板自备份 / 一键迁移** | 导出 `config.toml` + SQLite + 站点目录 + 插件目录为加密归档（可选 AES，密码派生密钥）；导入时校验版本与迁移状态，支持"预览差异"再覆盖；同时提供 cron 定时自备份 | M | 中（写路径多，需原子替换与回滚） |
| P1 | **Prometheus 指标端点** | `/metrics`（Bearer/内网限制），暴露：HTTP 请求数/时延直方图/状态码、监控采集耗时、SQLite 忙等次数、插件运行数、会话数、直连通道请求数与 401 计数 | S | 低 |
| P1 | **SMTP 邮件通知渠道** | 通知配置增加 SMTP（host/port/TLS/账号/密码、发件人、收件人组），告警路由按严重级选择渠道；测试发送按钮 | S–M | 低（依赖外发网络） |
| P2 | **API 访问令牌（PAT）** | `POST /api/auth/tokens` 生成（作用域如 `monitor:read`/`files:read`、过期、可吊销），校验走独立中间件；审计记录 token 名称 | M | 中（权限面扩大，需作用域白名单） |
| — | ~~角色与权限点~~ | 单用户产品，不做多角色；仅在将来开放协作时再评估 | — | — |
| P2 | **审计增强** | 时间范围/操作者/关键字过滤、CSV/JSON 导出、敏感操作记录"变更前/后"摘要 | M | 低 |
| P3 | **升级与回滚闭环** | 现有一键重启（`RestartPanel`）+ 版本检查；补"下载-校验-替换-健康检查-失败自动回滚"的升级流程与面板内进度 | M | 中 |
| P3 | 前端 i18n 骨架 | 抽取中文文案为 locale 文件，默认中文、预留 en | L | 低 |

## 落地记录

- 2026-11-22：`/metrics` 已实现并上线（`internal/infra/metrics`，Prometheus 文本格式）。默认仅 loopback 可访问，配置 `[server] metrics_token` 后改为 `Authorization: Bearer <token>`，可对内外监控开放。已暴露：HTTP 请求计数/耗时直方图（按路由模板、状态码）、活跃会话数。
  验证：本机 `curl -k https://127.0.0.1/metrics` 返回文本指标；外部无 token 返回 403。
- 2026-11-22：SMTP 邮件通知渠道已实现并上线。登录通知与监控告警会同时发邮件（与 Webhook 独立开关）；设置页新增「邮件通知（SMTP）」卡片（主机/端口/用户名/密码/发件人/收件人/启用开关 + 保存 + 测试邮件），密码只回显"是否已配置"，支持 `clear_smtp_password` 显式清除。465 走隐式 TLS，其它端口 STARTTLS。
  验证：未配置时测试接口返回"请先配置 SMTP 主机与收件人"；错误主机返回具体 DNS 失败原因；配置可正常清空。

- 2026-11-22：面板数据导出（备份）已实现并上线：`GET /api/backup/export?include=config,db,sites,plugins`，tar.gz 内含 `manifest.json`、`config.toml`、SQLite 一致性快照（`VACUUM INTO`），可选站点与插件目录。设置 → 服务器页新增「数据备份」卡片（导出基础/完整两种）。验证：基础包 2.8MB（3 项），含站点包 12.3MB。
  待办：导入/恢复（需要版本校验、预恢复快照、原子替换与回滚）、定时自备份。

## 四、建议的首批（本次可落地的三件）

0. 前提：面板是单用户产品，以下增强均围绕"单个管理员 + 自动化"设计，不引入多租户/角色体系。

1. **`/metrics` + 健康详情**：改动集中在 `internal/httpx/middleware`（请求计数/时延）与 `internal/api`（路由），不影响业务语义，能被现有 Grafana 立刻消费。
2. **SMTP 邮件渠道**：`notify` 领域新增 sender，配置沿用 `settings` 的"掩码回显 + 测试发送"模式，风险低。
3. **PAT**：给自动化/脚本一条可吊销的通道，避免长期 JWT 泄漏；作用域先只做只读（`monitor:read`、`files:read`、`audit:read`）。

面板自备份（P1）建议紧随其后：它是"运维闭环"里最有价值的一件，但涉及写盘与恢复流程，适合单独一轮实现 + 演练。

## 五、已核查且扎实的部分（避免重复审查）

- 路径沙箱 `internal/util/path.go:85`、压缩包解压（禁软链/硬链、`O_NOFOLLOW`、体积上限、失败回滚）。
- 认证：Cookie `HttpOnly+SameSite=Strict`、账号锁定、refresh 轮换/重放吊销、直连通道设备签名与指纹固定。
- 插件：zip slip/zip bomb 防护、权限审批、Host API 白名单代理；前端 iframe 同源边界已在注释中明确。
- 中间件顺序与分组、请求日志（不记录 query，避免 token 入日志）、审计覆盖写操作。
- CI：lint + govulncheck + race + 插件/SDK 编译 + 前端全链路 + release 冒烟。

# EasyServer 全项目审查报告

日期：2026-10-29
范围：`internal/`（后端 21 个领域包）、`web/src`（前端）、`cmd/`、`internal/plugin`、CI/发布/部署、文档。
方法：静态阅读 + 定向 grep + 线上运行配置核对（代码以当前工作区为准，含未提交的 `web/src/services/client.ts` 外链修复）。

## 一、结论摘要

整体质量高于同类自研面板：路径沙箱、压缩包解压、认证/Cookie、插件包解压、CI 流水线都做了成体系的防护，且有对应的挑战测试（`internal/domain/filemanager/challenge_test.go`）。本轮审查未发现"未授权即可读任意文件 / 执行任意命令"的洞。

真正需要动的是三类：

1. **插件前端的信任边界是假的**（P1）。`sandbox="allow-scripts allow-same-origin"` 让插件页面与面板同源，可读面板 DOM/localStorage、带 Cookie 调全部 API；Host SDK 的 `postMessage` 代理既不校验 `origin` 也不校验 `source`，任意外部站点只要能让用户打开插件页，就能通过弹窗把面板数据取走。
2. **会话令牌会漏进 URL**（P1）。`access_token` 查询参数回退把完整 JWT 放进 URL，Cloudflare/nginx 访问日志、浏览器历史都会留下长期有效的会话令牌。
3. **运行面权限过宽**（P1）。线上 `config.toml`（含 `jwt_secret`）与 SQLite（含用户、插件密钥密文）都是 `0644`；插件进程以 root 运行且没有降权模式。

其余为 P2/P3：密码随链接进 URL、在线设备列表 5 分钟窗口、SQLite 单连接串行、终端/envconfig 等零测试域、前端 `any` 与定时器治理、Actions 未固定 SHA。

## 修复进展（2026-10-29，已部署 v0.2.3-security）

| 编号 | 问题 | 状态 |
|---|---|---|
| P1-1 | 插件页 postMessage 无 origin/source 校验、任意 GET 代理 | ✅ 已修：校验同源 + 来自本 iframe；request 收紧到后端同款只读白名单 |
| P1-2 | 插件 iframe `allow-same-origin` 导致实质不隔离 | ⚠️ 部分：保留（插件前端需带 Cookie 调自身接口，去掉会破坏现有插件）；残留在报告中记录，中期用独立源解决 |
| P1-3 | `?access_token=` 传递会话 JWT | ✅ 已修：中间件移除该回退 + 回归测试；媒体预览走 Cookie |
| P1-4 | 配置/数据库/日志文件权限过宽 | ✅ 已修：`config.toml`/SQLite 主文件 `600`；`/opt/easyserver/data` 改 `711`（nginx 的 www-data 需遍历静态站点，`700` 会导致站点 500）；systemd `UMask=0077` 实测会破坏站点读取，已回退 `0022` 并在 unit 中注明 |
| P1-5 | 插件进程 root 运行 | ⏸ 设计级，未动（需配套插件运行用户与能力约束） |
| P2-1 | 带密码分享链接把密码拼进 URL | ✅ 已修：改放 `#password`（fragment 不发给服务端），下载页兼容旧 `?password` |
| P2-2 | 在线设备只显示 5 分钟内活跃会话 | ✅ 已修：列表改为所有未过期会话 + 单测；列名改"最近活动" |
| P2-3 | 插件密钥从 JWT 密钥派生 | ⏸ 未动（需配置项与迁移方案） |
| 前端 401 | 全量硬跳登录页 | ✅ 已修（外链白名单）+ 纯函数单测 |
| 测试缺口 | 无 `client.ts` 测试 | ✅ 已补 3 条；后端补 access_token、会话列表 2 条回归 |

## 二、安全发现

### P1-1 插件页 postMessage 代理可被第三方网页利用外泄数据

- 证据：`web/src/pages/Plugins/PluginFrame.tsx:14-31`
- 现状：`message` 监听只判断 `data.__esplugin`，随后按 `type=request` 用面板自己的 `api` 实例执行 `api.get(payload.url)`，并把结果 `postMessage` 回 `e.source`（`targetOrigin: e.origin`）。
- 影响：攻击页面 `window.open('https://panel.0xdead.icu/plugins/<id>')` 后向该窗口发送一条消息，即可拿到任意 GET 接口的响应（会话列表、文件列表、环境配置等）——只要受害者的面板处于登录态。`e.origin`、`e.source` 都未校验，插件页本身被 XSS 时同理。
- 修复：处理前断言 `e.origin === window.location.origin` 且 `e.source === iframeRef.current.contentWindow`；`request` 类型改为按插件已声明权限的**接口白名单**（避免"任意 GET"成为通用数据通道）。

### P1-2 插件 iframe 的 sandbox 实际不隔离

- 证据：`web/src/pages/Plugins/PluginFrame.tsx:44`、`web/src/pages/Dashboard.tsx:287`，两处均为 `sandbox="allow-scripts allow-forms allow-same-origin"`。
- 现状：插件资源由面板同源提供（`/api/plugins/:id/`）。`allow-scripts + allow-same-origin` 同时存在时，iframe 保留真实源，可 `parent.document` 读 DOM、读 `localStorage`（`user` 等）、并以受害者 Cookie 调 `/api/*`。注释"插件前端不持有面板 token"容易误导：Web 端登录态在 Cookie，插件页自动携带。
- 影响：一个被投毒/有 XSS 的插件等于面板账号的完整浏览器权限（不是 root，但可改设置、读审计、导数据）。
- 修复：去掉 `allow-same-origin`（保持 `allow-scripts`），插件资源改由独立路径/子域提供；需要的数据一律走父页面白名单代理；补 `sandbox` 说明到插件手册。

### P1-3 JWT 可通过 `access_token` 查询参数传递

- 证据：`internal/httpx/middleware/jwt.go:56-66`
- 现状：GET/HEAD 请求允许 `?access_token=<JWT>`，用于 `<img>/<video>/<audio>/<iframe>` 无法带头的场景。
- 影响：令牌进 URL 即进 Cloudflare 日志、nginx access log、浏览器历史、Referer（虽已设 `strict-origin` 降风险）。JWT 有效期默认 24h，泄漏窗口很长。
- 修复：媒体类改用短期（如 5 分钟）、绑定 `path+exp` 的 HMAC ticket（分享下载已有同类 `GetTicket` 实现可复用），不再接受会话 JWT。

### P1-4 配置与数据库文件权限过宽（线上实测）

- 证据：`ssh easyserver ls -l`：
  - `/opt/easyserver/config.toml` → `-rw-r--r--`
  - `/opt/easyserver/data/easyserver.db` → `-rw-r--r--`
- 影响：`jwt_secret` 明文、用户表/插件密钥密文对任何本地用户可读；拿到 JWT 密钥即可伪造任意用户令牌（会话校验在同一密钥下仍可通过，若会话表匹配则直接登录）。
- 修复：`chmod 600` 两文件，`deploy/easyserver.service` 增加 `UMask=0077`，文档补一条安装后检查。

### P1-5 插件进程以 root 运行，无降权选项

- 证据：`internal/plugin/supervisor.go:220`（`exec.CommandContext` 未设置 `SysProcAttr.Credential`）；文档 `docs/plugin-manual.md:54` 已说明 `exec.restricted` 等同 root。
- 影响：插件 = 主机 root 代码执行。这不是"实现 bug"，但配合 zip 安装（管理员上传）与第三方插件源，是最大的信任边界，应显式设计。
- 修复：中期提供 `run_as` 插件运行用户（systemd `User=`/`NoNewPrivileges`/`ProtectSystem=strict` 的等价约束），Host API 权限只作为最低门槛。

### P2-1 带密码分享链接把密码拼在 URL

- 证据：`web/src/pages/FileShares/index.tsx:110`（`${origin}/share/${token}?password=${encodeURIComponent(pwd)}`）
- 影响：密码进入浏览器历史、聊天软件链接预览、代理日志；链接长期可重放。
- 修复：改为一次性短票据（服务端签发 5 分钟、绑定 share 与 IP 可选），或仅提示在下载页手输。

### P2-2 "在线设备"只统计最近 5 分钟活跃的会话

- 证据：`internal/domain/auth/session.go:207`（`LoginAt.After(now.Add(-5*time.Minute))`）
- 影响：手机静置 6 分钟后在设置页"登录会话"里消失，但令牌仍有效，用户无法踢出；也造成"设备数跳变"的困惑。
- 修复：列表按 `ExpiresAt` 过滤并单独标注"最近活跃时间"，把 5 分钟窗口只用于"当前活跃"排序/统计。

### P2-3 插件密钥与 JWT 密钥强耦合

- 证据：`internal/plugin/repo_sqlite.go:32`（`sha256("easyserver:plugin-secrets:" + key)`）
- 影响：轮换 `jwt_secret`（安全事件后的标准操作）会导致 `plugin_secrets` 全部无法解密且不报错，插件在后台静默失效。
- 修复：独立 `plugin_secrets_key` 配置项（首次启动生成、持久化），或轮换时双密钥迁移。

### P2-4 限流维度单一

- 证据：`internal/httpx/middleware/ratelimit.go`（按 IP 的固定窗口）
- 影响：分布式来源可绕过；公共接口（`/auth/qr/status`、`/shares/public/:token/ticket`）只有 IP 维度。账号锁定（`MaxLoginAttempts=5`）缓解了登录爆破，但 TOTP/QR 轮询仍可被刷。
- 修复：登录类限流加"IP + 用户名"双键；公共接口补 UA/令牌维度的低阈值。

## 三、后端正确性与健壮性

总体：错误封装（`errx`）、中间件顺序、优雅关闭（`cmd/server/app.go` 的 `gracefulShutdown` + FD 继承热重启）都规范。已核查并认可的实现：

- 路径沙箱 `internal/util/path.go:85` `ResolveInSandbox`：拒 NUL、拒 `..`、绝对路径重映射到沙箱、逐级 `EvalSymlinks` 后再次前缀校验，逻辑正确且有测试。
- 压缩包解压 `internal/domain/filemanager/filemanager.go:444` 起：拒绝对路径/穿越、禁 symlink/hardlink、`O_NOFOLLOW`、体积与数量上限、失败回滚。
- 插件 zip 安装 `internal/plugin/install.go:40` 起：zip slip、zip bomb、权限规范化齐全。
- Cookie：`HttpOnly + SameSite=Strict + 动态 Secure`（`internal/domain/auth/http/handler.go:79`）。
- 账号锁定：`repo_sqlite_user.go:167` SQL 原子自增 + 锁定时间。
- cron unit 生成：名称正则、换行拒绝、`Description` 清洗、命令落盘为脚本而非拼进 `ExecStart`。

需要关注：

- **P2 SQLite 单连接**（`internal/infra/database/database.go:42`）：`SetMaxOpenConns(1)` 让读也串行。当前单管理员场景无感，但监控轮询 + 多标签页会排队；建议读写分离（`SetMaxOpenConns(4)` + WAL 已开启即可，写冲突由 `busy_timeout` 兜底）。
- **P2 会话纯内存**（`session.go` 注释已说明）：进程重启全员登出。对"踢出/审计"是安全收益，但和"重启不断线"的运维期望冲突；若要持久化，需注意 token 明文入库的加密。
- **P2 命令执行面**：`internal/domain/container/service.go:889/941/1134`、`internal/domain/web/http/handler.go:561`（站点构建命令）、`internal/plugin/services.go:75` 均为 `sh -c`。前者输入来自管理员配置，属设计内；建议在审计日志里记录完整命令与来源用户（目前部分只记摘要）。
- **P3** `NewRateLimiter` 的后台清理 goroutine 是 `cleanupOnce` 单例、永不退出（进程生命周期可接受，但测试会残留）。

## 四、前端

- **P2** 全局 401 跳转过于粗暴：`web/src/services/client.ts:28` 对所有 401 硬跳 `/login`。本次外链失效正是它造成（已在工作区修复 `/share/` 白名单）。长期建议：区分"认证接口 401"与"资源接口 401"，后者只提示不改路由；受保护页由 `ProtectedRoute` 决定跳转。
- **P2** 类型债：`any`/`as any` 共 101 处，集中在错误处理与 DTO 边界；建议逐步用生成的 API 类型或无 `any` 的 `unknown` + 守卫。
- **P2** 轮询分散：32 处 `setInterval/setTimeout`，抽查（Website、SystemMonitor、Login QR、Firewall）都有清理；建议收敛为统一的 `usePolling(fn, ms, enabled)` hook，避免新增页面漏清理。
- **P2** 测试面：仅 `LogViewer` 有 4 个测试文件；`ShareDownload`、`client.ts`、`ProtectedRoute`、插件代理都是高风险且易测的纯逻辑，本次 401 重定向问题本可被一个 3 行测试拦住。
- **P3** 包体：构建有 chunk 过大告警；`vite.config.ts` 可按路由手工分包。
- **P3** `dangerouslySetInnerHTML` 仅用于硬编码图标路径（`Layout.tsx:100`），无用户输入，安全。

## 五、工程化与发布

做得好的部分（核查过）：CI 有 `golangci-lint`（固定版本）、`govulncheck`、`go test -race`、插件与 SDK 独立 module 编译测试、前端 lint/tsc/test/build；Release 交叉编译 amd64/arm64 + 启动 smoke test + sha256。

待补：

- **P2** Actions 固定到 tag（`actions/checkout@v7`、`pnpm/action-setup@v6`）而非 SHA，供应链上可被上游改指向；建议改成 commit SHA。
- **P2** 发布产物无签名/attestation（`cosign` 或 GitHub artifact attestation），只有 sha256。
- **P2** 部署脚本没有制度化：本次修复是手工 `scp + systemctl restart`（已备份旧二进制）。建议把 `deploy/` 补一个 `install.sh`：备份、原子替换、健康检查失败自动回滚、`umask` 与权限设置。
- **P3** 文档：`PluginFrame.tsx:6` 的"sandbox 隔离"描述与实际不符；插件手册应补"前端同源"的真实边界与 Host SDK 白名单。

## 六、测试覆盖缺口

按领域统计（tests/files）：`terminal 0/3`、`envconfig 0/6`、`alert 0/2`、`notify 0/1`；`firewall 1/11`、`monitor 1/5`、`container 1/4` 偏低。优先补：

1. `terminal`：会话创建/回收、命令审计、PTY 泄漏。
2. `envconfig`：变量作用域与敏感值掩码。
3. `firewall`：规则参数校验与回滚（误操作成本最高）。
4. `auth` 的 QR 状态机并发用例（confirm 与 status 竞争、过期清理）。

## 七、优先级路线图

**本周（低风险、高收益）**

1. 线上 `chmod 600` 两个文件 + systemd `UMask=0077`。
2. 插件页 `postMessage` 加 `origin/source` 校验，并把 `request` 收敛到白名单。
3. 去掉插件 iframe 的 `allow-same-origin`，或明确记录为"受信插件"并加二次确认。

**下个版本**

4. 媒体令牌化，移除 `access_token` 查询参数的会话 JWT。
5. 在线设备列表按过期时间展示。
6. 插件密钥独立于 JWT 密钥。
7. 前端补 `client.ts`/`ShareDownload` 单测；后端补 terminal/envconfig/firewall 基础用例。

**中期**

8. 插件降权运行（非 root）与资源限制。
9. 部署/升级脚本化（备份-替换-健康检查-回滚）。
10. SQLite 读写并发调整与慢查询观测。

## 八、附：本次已核查且未发现问题的清单

- 中间件装配与路由分组（`internal/api/router.go:370-430`）：全局安全头/CORS/请求 ID/恢复/IP 白名单，`/api` 全量限流与请求体上限，`/ws` 独立鉴权。
- CORS/安全头：`internal/httpx/middleware/security.go`、`cors.go`。
- 分享外链后端：公开路由组、密码 bcrypt、过期/次数上限、下载计数原子自增、ticket HMAC。
- 文件管理：上传大小限制、重命名/删除路径校验、目录分享子路径解析。
- 日志：请求日志只记 path 不含 query（避免 `access_token` 落盘），4xx/5xx 分级。
- 配置热更新：`config.Store` 快照 + 深拷贝，运行时改设置即时生效。
- 构建与版本注入：`Makefile` 与 Release 一致（`internal/infra.Version`）。

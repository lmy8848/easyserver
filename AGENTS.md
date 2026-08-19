# EasyServer

Linux 服务器管理面板 · Go + Gin + SQLite + React + TypeScript。

## Layout

- `cmd/server/` — 后端入口（`main.go`）。
- `internal/` — 后端业务包，按领域拆分（`auth` / `container` / `cron` / `database` / `monitor` / `ssh` / `firewall` / `filemanager` ...）。`internal/api` 是路由汇聚层（Gin），handler 在各领域的 `http` 子包；并 embed 前端产物（`internal/api/web/dist`）。
- `web/` — 前端（React + TS + Vite），源代码在 `web/src`。
- `migrations/` — SQLite 迁移；`config.toml.example` 是配置模板。
- `docs/` — 内部工程文档（`agents/` / `adr/`）；`CONTEXT.md` — 项目上下文。
- `gh-pages` 分支 — GitHub Pages 站点源码（https://lmy8848.github.io/easyserver/）。

## Git

提交规范：

- Conventional Commits：`type(scope): 中文描述`，scope 可选。
- type 小写，如：`feat` / `fix` / `refactor` / `perf` / `test` / `docs` 等。
- scope 业务功能的层面抽象scope，例如feat(auth) / feat(cron), 不允许多领域。
- 标题与正文空一行；正文只写必要补充，不复述标题；正文需写清楚改动。

### Git hooks（.githooks/）

克隆后需执行一次启用：`git config core.hooksPath .githooks`。

- **pre-commit**：提交时静态检查，自动修复并重新暂存。后端 `golangci-lint --fix`
  （未安装时跳过不阻塞；配置见 `.golangci.yml`，已含 goimports / go vet）；前端
  `eslint --fix`（web/ 目录内运行）。
- **pre-push**：推送时完整检查，只对本次推送涉及的文件类型触发。后端
  `go test -tags dev ./cmd/... ./internal/...` + `go build -tags dev ./cmd/server`；
  前端 `pnpm run test` + `pnpm run build`。

提交/推送前无需手动跑这些检查，钩子会兜底；本地快速检查按需执行 `go build` / `tsc --noEmit`。

### 协作流程（PR + Squash）

复杂改动（超过800行）不直接在 master 上写，开分支走 PR；分支名说清干啥即可（英文）。全流程用 `gh` 完成，无需网页：

```bash
git checkout -b fix/auth-ttl
# 改代码 → git add -A && git commit -m "fix(auth): ..." （commit 信息即 PR 标题）
git push -u origin fix/auth-ttl
gh pr create --fill                          # 开 PR
git fetch origin && git rebase origin/master # 合并前对齐，冲突在这解决；解完 git add + git rebase --continue，再 git push -f（仅限自己分支，master 禁用 -f）
gh pr merge --squash --delete-branch         # 合并：squash 成一条，提交信息用 PR 标题
git checkout master && git pull origin master
```

默认半自动：改完开 PR、rebase 对齐后**停在合并前等 review**，队友看完再 `gh pr merge`；不要未经允许直接合并。冲突能解则解，拿不准停下问用户。

## Agent skills

### Issue tracker

Issues 与 PRD 存于 GitHub Issues，用 `gh` CLI 操作。See `docs/agents/issue-tracker.md`.

### Triage labels

使用默认五个角色字符串（`needs-triage` / `needs-info` / `ready-for-agent` / `ready-for-human` / `wontfix`）。See `docs/agents/triage-labels.md`.

### Domain docs

单上下文：根目录一份 `CONTEXT.md` + `docs/adr/`。See `docs/agents/domain.md`.

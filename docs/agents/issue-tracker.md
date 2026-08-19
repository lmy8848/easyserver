# Issue tracker: GitHub

Issues 与 PRD 存于 GitHub Issues，所有操作走 `gh` CLI。

## 约定

- **创建 issue**：`gh issue create --title "..." --body "..."`，多行正文用 heredoc。
- **读取 issue**：`gh issue view <number> --comments`，用 `jq` 过滤评论并取 labels。
- **列出 issues**：`gh issue list --state open --json number,title,body,labels,comments --jq '[.[] | {number, title, body, labels: [.labels[].name], comments: [.comments[].body]}]'`，配合 `--label` / `--state` 过滤。
- **评论**：`gh issue comment <number> --body "..."`
- **加/删标签**：`gh issue edit <number> --add-label "..."` / `--remove-label "..."`
- **关闭**：`gh issue close <number> --comment "..."`

仓库从 `git remote -v` 推断——在 clone 内运行 `gh` 会自动识别。

## 将 PR 视为请求入口

**PR 作为请求入口: no.**（若本仓库把外部 PR 当功能请求处理，改为 `yes`；`/triage` 读此标志。）

设为 `yes` 时，PR 与 issue 走同一套标签与状态，用 `gh pr` 等价命令：

- **读取 PR**：`gh pr view <number> --comments`；diff 用 `gh pr diff <number>`。
- **列出待 triage 的外部 PR**：`gh pr list --state open --json number,title,body,labels,author,authorAssociation,comments`，只保留 `authorAssociation` 为 `CONTRIBUTOR` / `FIRST_TIME_CONTRIBUTOR` / `NONE` 的（排除 `OWNER` / `MEMBER` / `COLLABORATOR`）。
- **评论/打标签/关闭**：`gh pr comment`、`gh pr edit --add-label`/`--remove-label`、`gh pr close`。

GitHub 的 issue 与 PR 共用同一编号空间，裸 `#42` 可能是任一——用 `gh pr view 42` 解析，失败再退回 `gh issue view 42`。

## 当技能说"发布到 issue tracker"

创建一条 GitHub issue。

## 当技能说"获取相关 ticket"

运行 `gh issue view <number> --comments`。

## Wayfinding 操作

供 `/wayfinder` 使用。**Map** 是一条带子 issue 作为 ticket 的 issue。

- **Map**：一条打 `wayfinder:map` 标签的 issue，承载 Notes / Decisions-so-far / Fog 正文。`gh issue create --label wayfinder:map`。
- **子 ticket**：以 GitHub sub-issue 关联到 map（`gh api` 操作 sub-issues 端点）。未启用 sub-issues 时，把子项写进 map 正文的 task list，并在子项正文顶部加 `Part of #<map>`。标签：`wayfinder:<type>`（`research`/`prototype`/`grilling`/`task`）。被认领后 assign 给负责的开发者。
- **阻塞**：GitHub 原生 issue 依赖——UI 可见的标准形式。用 `gh api --method POST repos/<owner>/<repo>/issues/<child>/dependencies/blocked_by -F issue_id=<blocker-db-id>` 加边，其中 `<blocker-db-id>` 是阻塞者的数字 **database id**（`gh api repos/<owner>/<repo>/issues/<n> --jq .id`，不是 `#number` 或 `node_id`）。GitHub 通过 `issue_dependencies_summary.blocked_by` 报告（只含未关闭的阻塞者——实时门控）。依赖不可用时退回在子项正文顶部写 `Blocked by: #<n>, #<n>`。所有阻塞者关闭后 ticket 才算解阻。
- **Frontier 查询**：列出 map 的未关闭子项（`gh issue list --state open`，限定在 map 的 sub-issues / task list 内），去掉有未关闭阻塞者（`issue_dependencies_summary.blocked_by > 0`，或 `Blocked by` 行中有未关闭 issue）或已有 assignee 的；按 map 顺序取第一个。
- **认领**：`gh issue edit <n> --add-assignee @me`——本次会话的首次写入。
- **解决**：`gh issue comment <n> --body "<answer>"`，然后 `gh issue close <n>`，再把上下文指针（gist + 链接）追加到 map 的 Decisions-so-far。

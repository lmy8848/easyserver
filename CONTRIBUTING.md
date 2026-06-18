# 贡献指南

欢迎贡献 EasyServer！请遵循以下规范。

## 行为准则

- 尊重所有参与者
- 鼓励建设性反馈
- 禁止人身攻击或侮辱性言论

## 分支策略

```
main          ← 稳定发布
develop       ← 开发主分支
feat/xxx      ← 功能分支
fix/xxx       ← 修复分支
docs/xxx      ← 文档分支
```

- 功能开发从 `develop` 切出 `feat/` 分支，完成后提 PR → `develop`
- 修复从 `develop` 切出 `fix/` 分支
- 发布时 `develop` → `main`，打版本 tag

## 提交规范

遵循 [Conventional Commits](https://www.conventionalcommits.org/)：

```
<type>(<scope>): <description>

[optional body]

[optional footer]
```

### Type 一览

| Type | 场景 |
|------|------|
| `feat` | 新功能 |
| `fix` | Bug 修复 |
| `docs` | 文档变更 |
| `refactor` | 重构（不改变功能） |
| `perf` | 性能优化 |
| `test` | 测试相关 |
| `style` | 代码格式（非功能变更） |
| `chore` | 构建/CI/工具变更 |
| `revert` | 回滚 |

### Scope 示例

```
feat(monitor): add disk I/O monitoring
fix(auth): login lockout not clearing after successful login
docs(api): update websocket heartbeat section
```

## PR 流程

1. Fork 仓库并创建功能分支
2. 确保代码通过 lint 和测试
3. 提交 PR 到 `develop` 分支
4. 至少 1 名维护者 Review 后合并

### PR 检查清单

- [ ] 代码遵循项目编码规范
- [ ] 新功能包含必要的测试
- [ ] API 变更已更新文档
- [ ] 所有测试通过
- [ ] 没有引入新的安全警告

## 开发环境

参考 [docs/development.md](docs/development.md) 搭建本地开发环境。

### 运行测试

```bash
# 后端测试
go test ./... -v

# 后端测试（含覆盖率）
go test ./... -coverprofile=coverage.out
go tool cover -html=coverage.out

# 前端类型检查
cd web && npx tsc --noEmit

# 前端 lint
cd web && npx eslint src/
```

## 代码 Review 原则

- 保持 PR 小而集中（建议 ≤400 行变更）
- 每个 PR 只做一件事
- Review 关注：正确性、安全性、性能、可维护性
- 使用 GitHub 的 "Request Changes" 或 "Approve" 功能

## 议题报告

### Bug 报告

请包含：

- 环境信息（OS、浏览器、版本）
- 复现步骤
- 预期行为 vs 实际行为
- 日志或截图（如有）

### 功能请求

请说明：

- 使用场景和动机
- 期望的解决方案
- 替代方案（如有）

## 安全漏洞

请直接联系维护者，**不要**公开提交 Issue。安全相关问题会优先处理。

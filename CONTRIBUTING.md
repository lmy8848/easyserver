# Contributing to EasyServer

Welcome! EasyServer is [MIT-licensed](LICENSE) and contributions are very welcome — bug reports, feature requests, and pull requests.

## Code of Conduct

- Respect all participants
- Encourage constructive feedback
- No harassment or abusive language

## Branching

The repository uses `master` as the default (stable) branch. Feature work is done on short-lived branches and merged via Pull Request:

```
master           ← default branch, stable
feat/xxx         ← feature branches
fix/xxx          ← bug-fix branches
docs/xxx         ← documentation branches
chore/xxx        ← build / CI / tooling branches
```

## Commit Convention

We follow [Conventional Commits](https://www.conventionalcommits.org/):

```
<type>(<scope>): <description>

[optional body]

[optional footer]
```

### Types

| Type      | When to use                              |
|-----------|------------------------------------------|
| `feat`    | New feature                              |
| `fix`     | Bug fix                                  |
| `docs`    | Documentation only                       |
| `refactor`| Code change that neither fixes a bug nor adds a feature |
| `perf`    | Performance improvement                  |
| `test`    | Adding/updating tests                    |
| `style`   | Formatting, no functional change         |
| `chore`   | Build / CI / tooling                     |
| `revert`  | Reverting a previous change              |

### Examples

```
feat(monitor): add disk I/O monitoring
fix(auth): login lockout not clearing after successful login
docs(api): document websocket heartbeat section
```

- Use one scope per commit
- Put a blank line between subject and body; the body should explain the *why*, not repeat the subject

## Pull Request Workflow

1. Fork the repository and create a feature branch
2. Ensure your change passes local checks (see below)
3. Submit a PR against `master`
4. At least one maintainer reviews and merges

### PR Checklist

- [ ] Code follows the project's coding standards (no hard-coded values, use the layered architecture for new features)
- [ ] New functionality includes the necessary tests
- [ ] API changes are reflected in the docs
- [ ] All tests pass
- [ ] No new security warnings

## Local Checks

Static checks run automatically via git hooks — enable once after clone:

```bash
git config core.hooksPath .githooks
```

- **pre-commit** — backend `golangci-lint --fix` (auto-fixes and re-stages; skipped if golangci-lint isn't installed) + frontend `eslint --fix`
- **pre-push** — backend `go test -tags dev ./cmd/... ./internal/...` + `go build -tags dev ./cmd/server`; frontend `pnpm run test` + `pnpm run build` (triggered only for the file types in the pushed range)

Manual equivalents:

```bash
# Backend tests
go test -tags dev ./cmd/... ./internal/...

# Backend cross-compile for Linux (the panel targets Linux)
make build-linux

# Frontend type check + lint + tests
cd web && pnpm run build   # includes tsc --noEmit
cd web && pnpm run lint
cd web && pnpm run test
```

## Development Environment

| Tool   | Version |
|--------|---------|
| Go     | 1.25+   |
| Node.js| 22+    |
| pnpm   | 11+     |

## Code Review Principles

- One PR does one thing
- Review for: correctness, security, performance, maintainability
- Use GitHub's "Request Changes" or "Approve" as appropriate

## Issue Reporting

### Bug Reports

Please include:

- Environment info (OS, browser, backend version)
- Steps to reproduce
- Expected vs actual behavior
- Relevant log lines (from `/opt/easyserver/easyserver.log`) or screenshots

### Feature Requests

Please include:

- Use case and motivation
- Proposed solution
- Alternatives you considered (if any)

## Security Vulnerabilities

Contact the maintainers directly — **do not** open a public issue. Security issues are handled with priority.

## Reporting Use

EasyServer is released under the MIT License. When you report an issue, you grant permission for the maintainers to use the information to improve the project.

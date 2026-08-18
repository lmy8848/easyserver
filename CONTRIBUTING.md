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

> Note: the panel's own remote-deployment feature name-collides with the `deploy/` directory — keep them conceptually separate.

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

- Keep the subject under 72 characters
- Use one scope per commit (no comma-separated scopes)
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

Both frontend and backend must compile and pass checks before submitting:

```bash
# Both frontend + backend
make check

# Backend only (gofmt / vet / test / build)
make check-go

# Frontend only (tsc / eslint / vite build)
make check-web
```

> The backend targets Linux (uses Linux syscalls); a full backend build runs on the server or with `GOOS=linux` for cross-compilation checks. Frontend checks run locally.

Manual equivalents:

```bash
# Backend tests
go test ./... -v

# Backend tests with coverage
go test ./... -coverprofile=coverage.out
go tool cover -html=coverage.out

# Frontend type check + lint + build
cd web && pnpm build
cd web && pnpm lint
```

## Development Environment

See [docs/development.md](docs/development.md) for the full local setup guide.

| Tool   | Version |
|--------|---------|
| Go     | 1.25+   |
| Node.js| 22+    |
| pnpm   | 11+     |

## Code Review Principles

- Keep PRs small and focused (aim for ≤400 changed lines)
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
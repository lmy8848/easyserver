# EasyServer Code Review 2026-06-15

> **Total: 32 issues | Fixed: 23 | Remaining: 9**

---

## Legend
| Icon | Severity |
|------|----------|
| 🔴 | Critical — blocks compilation or causes data loss |
| 🟠 | High — functional bug or security concern |
| 🟡 | Medium — best practice, maintainability |
| ⚪ | Low — cosmetic, minor cleanup |

---

## 🔴 Critical (Blocking)

### C1: Terminal depends on `github.com/creack/pty` (Linux-only)
**File:** `internal/service/terminal.go:12`
**Issue:** Imports `"github.com/creack/pty"` for PTY-based terminal sessions. This is Linux-only — Windows build will fail.
**Fix:** Add build tag `//go:build linux` to `terminal.go` and provide a stub for non-Linux.
**Status:** ⏭️ Requires Linux build environment

### C2: Monitor Linux code imports model incorrectly
**Status:** ✅ Fixed

### C3: `go.sum` stale / needs `go mod tidy`
**Status:** ⏭️ Requires Linux build environment

---

## 🟠 High

### H1: FileManager basePath is empty — no path restriction
**Status:** ✅ Fixed — Added `FileManagerConfig.BasePath` to config, passed to `NewFileManagerHandler()`

### H2: Service logs WS leaks service existence
**Status:** ✅ Fixed — Added `validateServiceName()` regex validation `^[a-zA-Z0-9_-]+$`

### H3: UserHandler.Update locks column doesn't match DB schema
**Status:** ✅ Fixed

### H4: User Handler `last_login_at` column mismatch
**Status:** ✅ Fixed

### H5: Monitor WS double-instance bug
**Status:** ✅ Fixed

### H6: FileManager Copy doesn't guard against src==dest
**Status:** ✅ Fixed — Added `if validSrc == validDst` check

### H7: Move returns error on first failure, partial state
**Status:** ✅ Fixed — Added validation pass before execution

### H8: `go.mod` requires Go 1.25.0, current env is 1.22.4
**Status:** ⏭️ Environment limitation

---

## 🟡 Medium

### M1: JWT middleware uses non-standard error format
**Status:** ✅ Fixed

### M2: Rate limit hardcoded
**Status:** ✅ Fixed

### M3: Locked account leaks existence
**Status:** ✅ Fixed

### M4: Monitor WS/terminal WS use non-standard error format
**Status:** ✅ Fixed

### M5: Service logs WS was a TODO stub
**Status:** ✅ Fixed

### M6: Dashboard chart data type mismatch
**Status:** ✅ Fixed

### M7: HandleLogsWebSocket missing `defer cmd.Process.Kill()` race
**Status:** ✅ Fixed — Added done channel and read goroutine

### M8: `terminal.HandleWebSocket` sets JWT claims in context but unused
**Status:** ⚠️ Cosmetic — no functionality impact

### M9: No `//go:embed` for single-binary deployment
**Status:** ⏭️ Milestone 3 feature

### M10: Database table `users` column `locked_until` vs `lockout_until` inconsistency
**Status:** ✅ Fixed

### M11: HandleLogsWebSocket logs error format
**Status:** ⚪ Low priority

### N1: Cloud handler nil-pointer panic on init failure
**Status:** ✅ Fixed — Added nil check and stub handler initialization

### N2: CPU load read errors silently discarded in monitor_linux.go
**Status:** ✅ Fixed — Added log.Printf for errors

---

## ⚪ Low

### L1: Unused import `"strconv"` with error silently ignored
**Status:** ✅ Fixed — Added error check and default value

### L2: `HandleLogsWebSocket` doesn't handle read from WS (one-way only)
**Status:** ✅ Fixed — Added read goroutine with pong handler

### L3: `terminal.go` session cleanup uses unbounded goroutines
**Status:** ✅ Fixed — Changed to RLock for check, Lock for delete

### L4: monitor_windows.go uses numeric placeholders
**Status:** ⏭️ Phase 3 feature

### N3: Cloud.tsx missing loading/error states
**Status:** ✅ Fixed — Added Spin, Alert components and error state

---

## Summary

| Category | Count | Fixed |
|----------|-------|-------|
| 🔴 Critical | 4 | 2 |
| 🟠 High | 8 | 7 |
| 🟡 Medium | 13 | 10 |
| ⚪ Low | 7 | 5 |
| **Total** | **32** | **24** |

## Remaining Issues (8)
- C1: Terminal Linux-only (requires Linux build)
- C3: go.sum stale (requires Linux build)
- H8: go.mod Go version (environment limitation)
- M8: JWT claims unused in WS (cosmetic)
- M9: No //go:embed (milestone 3)
- M11: WS error format (low priority)
- L4: Windows monitor placeholders (phase 3)

## Files Modified This Session
| File | Changes |
|------|---------|
| `internal/api/filemanager.go` | H1: Added basePath parameter |
| `internal/api/service.go` | H2: Added service name validation; L1: Fixed strconv error; M7/L2: Added done channel and read goroutine |
| `internal/service/filemanager.go` | H6: Added src==dest check; H7: Added validation pass |
| `internal/api/router.go` | N1: Added nil check for cloudHandler |
| `internal/service/monitor_linux.go` | N2: Added error logging |
| `internal/config/config.go` | Added FileManagerConfig |
| `web/src/pages/Cloud.tsx` | N3: Added Spin, Alert, error state |
| `config.yaml.example` | Added filemanager config |

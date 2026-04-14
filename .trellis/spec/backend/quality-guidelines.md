# Quality Guidelines

> Code quality standards for backend development.

---

## Overview

Backend changes must preserve the repo's existing runtime model:

- startup order in `main.go` is significant
- core logging is context-aware
- platform differences are handled with split files
- settings, database, and UI bridge contracts are already shared across layers

Comments should stay concise and in English.

---

## Forbidden Patterns

- Do not use `fmt.Println`, `println`, or the standard library `log` package for application logging.
- Do not spawn unmanaged goroutines for long-lived work when `util.Go(...)` or `util.GoRecover(...)` should own recovery and context propagation.
- Do not panic for normal runtime failures such as IO, DB, or user-input errors.
- Do not mix data compatibility logic into `AutoMigrate`; use `migration/`.
- Do not add speculative unit tests by default when the user did not ask for them.

---

## Required Patterns

- Wrap operation failures with context using `%w` unless the caller must branch on a specific sentinel.
- Preserve or create trace-aware contexts before logging or crossing transport boundaries.
- Keep feature ownership coherent: UI bridge code in `ui/`, settings persistence in `setting/`, shared DB bootstrap in `database/`, plugin behavior in `plugin/system/...`.
- Follow repo build conventions. For backend work, verify with `make build` in `wox.core`; broader cross-layer work may need repo-level verification.
- For major fixes or feature additions, add corresponding smoke coverage instead of relying only on local reasoning.

---

## Testing Requirements

- Backend runtime tests live in `wox.core/test/` and are integration-style rather than tiny isolated unit tests.
- The standard repo entry point is `make test` at the repo root, which delegates to `go test ./test -v` in `wox.core` with isolated test directories.
- Use the existing test harness instead of inventing ad-hoc bootstrap code. Examples: `wox.core/test/test_base.go`, `wox.core/test/test_runner.go`, `wox.core/test/test_environment_test.go`.
- If a change affects shipped desktop behavior or UI-visible flows, update or add a matching Flutter smoke test under `wox.ui.flutter/wox/integration_test/`.

---

## Code Review Checklist

- Does the change preserve startup/init ordering and package boundaries?
- Are logs trace-aware and routed through `util.GetLogger()`?
- Are errors wrapped or handled at the correct boundary?
- If persistence changed, is the right migration path used?
- If the change crosses layers, were transport payloads and smoke tests updated together?

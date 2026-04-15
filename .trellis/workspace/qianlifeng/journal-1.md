# Journal - qianlifeng (Part 1)

> AI development session journal
> Started: 2026-04-14

---



## Session 1: Bootstrap Wox Development Guidelines

**Date**: 2026-04-14
**Task**: Bootstrap Wox Development Guidelines

### Summary

Documented Wox-specific backend and frontend development conventions from the existing codebase and archived the bootstrap guideline task.

### Main Changes

| Area | Description |
|------|-------------|
| Backend guidelines | Documented Wox backend structure, SQLite/Gorm patterns, migration flow, error handling, logging, and quality expectations under `.trellis/spec/backend/`. |
| Frontend guidelines | Documented Flutter/GetX UI structure, component conventions, state ownership, type-safety patterns, and smoke-test expectations under `.trellis/spec/frontend/`. |
| Task workflow | Initialized task context files, captured implementation/check references, and archived `00-bootstrap-guidelines` after completion. |

**Updated Files**:
- `.trellis/spec/backend/database-guidelines.md`
- `.trellis/spec/backend/directory-structure.md`
- `.trellis/spec/backend/error-handling.md`
- `.trellis/spec/backend/index.md`
- `.trellis/spec/backend/logging-guidelines.md`
- `.trellis/spec/backend/quality-guidelines.md`
- `.trellis/spec/frontend/component-guidelines.md`
- `.trellis/spec/frontend/directory-structure.md`
- `.trellis/spec/frontend/hook-guidelines.md`
- `.trellis/spec/frontend/index.md`
- `.trellis/spec/frontend/quality-guidelines.md`
- `.trellis/spec/frontend/state-management.md`
- `.trellis/spec/frontend/type-safety.md`
- `.trellis/tasks/archive/2026-04/00-bootstrap-guidelines/`


### Git Commits

| Hash | Message |
|------|---------|
| `8cedcc99` | (see git log) |
| `2fbf8983` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 2: Fix Everything provider fallback

**Date**: 2026-04-15
**Task**: Fix Everything provider fallback

### Summary

(Add summary)

### Main Changes

| Feature | Description |
|---------|-------------|
| Root cause | Traced the file search failure to `WalkEverything()` returning early when SDK3 was unavailable, which prevented the SDK2 fallback from ever running on Everything 1.4 installations. |
| Backend fix | Split the SDK3 path into a primary search flow, added explicit fallback routing to the legacy SDK, and improved legacy query error classification so non-IPC failures report the real `last_error` code. |
| Regression coverage | Added Windows-specific tests covering fallback routing and legacy error mapping for the Everything integration. |
| Verification | Ran `go test ./util/filesearch -run Everything -count=1` and `go test ./util/filesearch -count=1` with `go.exe` under `wox.core`. |

**Updated Files**:
- `wox.core/util/filesearch/everything_sdk_windows.go`
- `wox.core/util/filesearch/everything_sdk2_windows.go`
- `wox.core/util/filesearch/everything_sdk_windows_test.go`


### Git Commits

| Hash | Message |
|------|---------|
| `81c0e606` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete

# Error Handling

> How errors are handled in this project.

---

## Overview

This codebase mostly uses plain Go `error` values with contextual wrapping instead of a large custom error hierarchy.

- Lower layers return `error`.
- Callers add context with `fmt.Errorf(... %w)`.
- Boundary layers log, translate, or surface the error to the UI.
- Panics are reserved for impossible bootstrap or registration failures, not normal runtime problems.

---

## Error Types

- Standard Go `error` values are the default.
- Sentinel errors from dependencies are handled explicitly when needed. Example: `gorm.ErrRecordNotFound` in `wox.core/setting/mru.go`.
- Panic is acceptable only for invariant violations that indicate programmer error or corrupt registration state. Examples:
  - `migration.Register(nil)` or duplicate migration IDs in `wox.core/migration/migrator.go`
  - constructing `setting.Manager` before the database is initialized in `wox.core/setting/manager.go`

There is no project-wide custom `AppError` type to standardize around today.

---

## Error Handling Patterns

- Validate request inputs early and return immediately. Example: `handlePreview` in `wox.core/ui/router.go` checks `id`, `sessionId`, and `queryId` before calling deeper code.
- Wrap lower-level failures with useful operation context. Example: `failed to query MRU record: %w`, `migration: %s failed: %w`, `failed to check autostart status: %w`.
- Log at the boundary that can decide whether the process should continue, return an HTTP error, or degrade gracefully.
- For background maintenance work, prefer logging and continuing instead of crashing the app. Examples: migration after-commit warnings, periodic cleanup errors, analytics init failure.
- Use `util.GoRecover(...)` around goroutines or top-level request handlers that must not crash the process.

Examples:

- `wox.core/main.go`: startup failures are logged once at the application boundary and then return early.
- `wox.core/migration/migrator.go`: wraps DB and migration failures with migration IDs for diagnosis.
- `wox.core/setting/manager.go`: logs recovery attempts for autostart mismatch and reverts settings when corrective actions fail.

---

## API Error Responses

REST endpoints return a simple envelope defined in `wox.core/ui/http.go`:

- Success: `{"Success": true, "Message": "", "Data": ...}`
- Error: `{"Success": false, "Message": "<message>", "Data": ""}`

Guidelines:

- Use `writeErrorResponse(w, "...")` for request validation and operation failures.
- Return user-readable messages; the UI treats the response as an error string, not a typed error object.
- Keep transport conversion at the handler boundary. Example: `handlePluginStore` copies store manifests into DTOs and returns a response or a single error string.
- WebSocket flows use `responseUIError(...)` with the same boundary responsibility.

---

## Common Mistakes

- Do not log the same error at every stack frame; log it at the boundary that owns the fallback or return path.
- Do not panic for recoverable IO, DB, network, or user-input errors.
- Do not return bare library errors when one extra sentence would identify the failed operation.
- Do not expose sensitive values such as API keys or raw secrets in error strings.

# Logging Guidelines

> How logging is done in this project.

---

## Overview

Backend logging is centralized in `wox.core/util/log.go`.

- Use `util.GetLogger()` to access the shared logger.
- All log calls should carry a `context.Context`.
- Trace IDs, session IDs, and component names are attached through context helpers in `wox.core/util/context.go`.
- Log output is written through zap-backed file logging with rotation support.

The backend logger is the source of truth for process diagnostics. Do not introduce parallel logging styles.

---

## Log Levels

- `Debug`: high-volume diagnostics, timing details, ignored events, and dev-oriented traces.
  Examples: query debounce details in `plugin/manager.go`, UI bridge noise in `ui/http.go`.
- `Info`: lifecycle milestones and expected state transitions.
  Examples: startup banner in `main.go`, migration apply/skip events in `migration/migrator.go`, cleanup counts in `setting/mru.go`.
- `Warn`: recoverable problems or degraded behavior.
  Examples: failed PRAGMA execution, autostart mismatch repair issues, skipped UI responses when the socket is not ready.
- `Error`: failed operations that the app or current request could not complete successfully.
  Examples: database init failure, failed backup/restore actions, failed JSON marshal/unmarshal at transport boundaries.

---

## Structured Logging

The logger formats each message with:

- timestamp with milliseconds
- goroutine ID
- trace ID when present
- short level marker (`DBG`, `INF`, `WRN`, `ERR`)
- component name when present
- message text

Required patterns:

- Create a trace context with `util.NewTraceContext()` or `util.NewTraceContextWith(...)` for new flows.
- Preserve trace and session context when bridging HTTP or WebSocket requests. Example: `getTraceContext` in `wox.core/ui/router.go`.
- Use `fmt.Sprintf(...)` only to build the message body; the trace and component fields come from context, not from manual string prefixes.

Examples:

- `wox.core/util/log.go`: canonical formatting and rotation behavior.
- `wox.core/ui/http.go`: attaches trace and session context while handling UI messages.
- `wox.core/main.go`: records startup environment, version, paths, and initialization failures with one trace context.

---

## What to Log

- Application startup, shutdown, migrations, and periodic background jobs.
- External process or external service failures, including stderr samples when they materially help diagnosis. Example: SQLite recovery in `database/database.go`.
- State repairs or automatic fallback decisions. Examples: autostart correction, migration skip/apply, MRU cleanup.
- User-visible failures returned across the UI bridge.

---

## What NOT to Log

- Secrets such as AI provider API keys, proxy credentials, or raw tokens.
- Full user content when a summary is enough. Avoid dumping clipboard content, chat payloads, or large plugin results unless explicitly debugging a safe local scenario.
- Duplicate messages from multiple stack frames for the same failure.
- Ad-hoc `fmt.Println`, `log.Println`, or other side-channel logging that bypasses `util.GetLogger()`.

# Directory Structure

> How backend code is organized in this project.

---

## Overview

Backend code lives under `wox.core/`. The codebase is organized by runtime responsibility instead of a strict controller/service/repository split.

- Startup composition lives in `main.go`.
- Long-lived application capabilities sit in top-level packages such as `ui`, `setting`, `database`, `plugin`, and `telemetry`.
- Reusable OS and infrastructure helpers live under `util/`.
- Platform-specific implementations use Go build suffixes like `_windows.go`, `_linux.go`, and `_darwin.go`.

This structure matters more than an abstract layering rule: new code should land beside the capability it belongs to.

---

## Directory Layout

```text
wox.core/
├── main.go
├── ai/                 # AI providers and MCP integration
├── analytics/          # Analytics models and tracker bootstrap
├── common/             # Shared cross-layer models
├── database/           # Core database bootstrap and shared DB models
├── i18n/               # Language loading and translation manager
├── migration/          # App-level compatibility migrations
├── plugin/             # Plugin manager, SDK bridge, built-in plugins
│   ├── host/
│   └── system/
├── resource/           # Embedded assets shipped with the app
├── setting/            # Wox settings, stores, validators, definitions
├── telemetry/          # Presence and heartbeat reporting
├── ui/                 # HTTP/WebSocket bridge to Flutter UI
│   └── dto/            # Transport-only DTOs for UI responses
├── util/               # OS helpers, logging, windowing, filesearch, hotkeys
└── test/               # Integration-style backend runtime tests
```

---

## Module Organization

- Put app-wide capabilities in dedicated top-level packages. Examples: `setting`, `ui`, `database`, `telemetry`.
- Put transport-only response shapes in `ui/dto/`; do not reuse DTOs as domain models in other packages.
- Keep plugin-specific code under `plugin/system/<plugin>` unless the code is clearly shared across multiple features.
- Keep feature-local persistence types close to the feature when the data is not part of the shared app database contract. Example: `plugin/system/shell/shell_history.go`.
- Use `util/<capability>` for reusable helpers that are not business features. Platform branches belong in split files, not in runtime `if runtime.GOOS` checks inside shared files.

---

## Naming Conventions

- Package names are lowercase and short: `setting`, `plugin`, `ui`, `util`.
- File names use snake_case and often describe the owned capability: `wox_setting.go`, `provider_local.go`, `startup_restore.go`.
- Platform-specific files must use Go suffixes, for example `window_windows.go`, `window_linux.go`, `window_darwin.go`.
- Keep new files close to the package concept they extend instead of introducing generic folders like `helpers/` or `services/`.

---

## Examples

- `wox.core/main.go`: the root composition file wires location, logger, database, migration, settings, UI, plugins, telemetry, and hotkeys in a strict startup order.
- `wox.core/ui/router.go` and `wox.core/ui/dto/`: transport handlers stay in `ui`, while DTOs are separated from domain packages.
- `wox.core/plugin/system/clipboard/`: a plugin-specific subsystem keeps its runtime logic and its dedicated SQLite wrapper together instead of leaking into shared packages.
- `wox.core/util/filesearch/`: a large shared subsystem still stays under one capability-focused package, with platform files and tests alongside implementation.

---

## Avoid

- Do not add new HTTP handlers in unrelated packages; route registration belongs in `ui/router.go`.
- Do not hide business code in `util/`; `util/` is for reusable infrastructure and OS integration, not feature ownership.
- Do not use runtime platform branching when a split `*_windows.go` or `*_darwin.go` file matches the existing pattern.

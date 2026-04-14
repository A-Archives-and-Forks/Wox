# Directory Structure

> How frontend code is organized in this project.

---

## Overview

The desktop UI lives in `wox.ui.flutter/wox/lib`. The codebase is organized by responsibility:

- `modules/` contains screen-level feature views.
- `components/` contains reusable widgets and shared UI primitives.
- `controllers/` contains GetX controllers and editing controllers.
- `entity/`, `models/`, and `enums/` define transport and UI types.
- `utils/` contains infrastructure helpers, theme loaders, logging, platform adapters, and webview support.
- `api/` is the narrow HTTP client layer to the Go backend.

This is a Flutter desktop app, not a React app. Organize around widgets, controllers, and platform adapters instead of pages/hooks/services vocabulary from web stacks.

---

## Directory Layout

```text
wox.ui.flutter/wox/lib/
├── api/                # HTTP wrapper around backend endpoints
├── components/         # Shared widgets and plugin setting widgets
│   └── plugin/
├── controllers/        # GetX controllers and editing controllers
├── entity/             # Transport/domain model classes
│   ├── setting/
│   └── validator/
├── enums/              # String-backed enums used across UI and API calls
├── models/             # UI-only models
├── modules/
│   ├── launcher/views/
│   └── setting/views/
├── utils/              # Theme, logging, HTTP, platform, webview helpers
│   ├── test/
│   ├── webview/
│   └── windows/
└── main.dart           # App bootstrap and dependency registration
```

---

## Module Organization

- Put screen-specific layout under `modules/<feature>/views/`.
- Put shared reusable widgets under `components/`; keep plugin-setting-specific widgets under `components/plugin/`.
- Put shared state and orchestration in `controllers/`, not inside large view widgets.
- Put all backend calls behind `WoxApi` and `WoxHttpUtil`; widgets and controllers should not construct raw endpoints themselves.
- Put platform-specific desktop integrations under `utils/windows/` or `utils/webview/<platform>/`.

---

## Naming Conventions

- File names use snake_case and almost always start with `wox_`.
- Shared widgets usually end with `View`, `Button`, `Field`, or another UI noun. Examples: `wox_setting_view.dart`, `wox_button.dart`, `wox_textfield.dart`.
- Controllers end with `Controller`. Examples: `wox_launcher_controller.dart`, `wox_setting_controller.dart`.
- Utility singletons end with `Util`. Examples: `wox_setting_util.dart`, `wox_theme_util.dart`.
- Keep the established `Wox...` class prefix even when it feels verbose; consistency is more important than shortening names in just one area.

---

## Examples

- `wox.ui.flutter/wox/lib/main.dart`: bootstraps services, registers controllers with GetX, and chooses between launcher and settings shells.
- `wox.ui.flutter/wox/lib/modules/setting/views/`: groups screen-level settings views by feature while reusing shared components.
- `wox.ui.flutter/wox/lib/components/plugin/`: isolates the plugin-setting renderer widgets from the rest of the shared component library.
- `wox.ui.flutter/wox/lib/utils/windows/`: keeps desktop-platform window behavior out of general-purpose widgets.

---

## Avoid

- Do not put shared controller logic inside `modules/.../views/` just because one screen uses it first.
- Do not bypass `api/` by making raw `Dio` or HTTP calls inside widgets.
- Do not create generic folders like `helpers/` when an existing capability folder already matches the code.

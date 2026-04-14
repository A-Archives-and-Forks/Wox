# Hook Guidelines

> How hooks are used in this project.

---

## Overview

This repository does not use React-style hooks. The file exists because the Trellis template expects it, but the actual Flutter patterns are:

- GetX controllers for shared reactive state
- utility singletons for cached app configuration
- `StatefulWidget` lifecycle methods for widget-local setup and teardown
- small mixins for repeated widget behavior
- `ValueNotifier` only for narrow platform integrations

If you find yourself trying to invent `useSomething`, you are probably importing the wrong mental model.

---

## Custom Hook Patterns

Use these replacements instead of custom hooks:

- Shared cross-screen state or orchestration:
  create or extend a GetX controller in `controllers/`.
- Cached configuration or singleton service:
  use a utility singleton such as `WoxSettingUtil` or `WoxThemeUtil`.
- Repeated widget behavior with local state:
  use a mixin or shared base widget. Example: `WoxSettingPluginItemMixin`.
- Widget-scoped async state:
  keep it in a `StatefulWidget` and load it from `initState`.

Examples:

- `wox.ui.flutter/wox/lib/main.dart`: registers long-lived controllers with `Get.put(...)`.
- `wox.ui.flutter/wox/lib/controllers/wox_setting_controller.dart`: owns shared settings state and refresh methods.
- `wox.ui.flutter/wox/lib/components/plugin/wox_setting_plugin_item_view.dart`: uses a mixin to reuse widget-side layout and validation behavior.

---

## Data Fetching

- Default path: controller -> `WoxApi` -> `WoxHttpUtil`.
- Use a generated trace ID for each request before calling the backend.
- Keep long-lived fetched state in controllers with `.obs`, `RxList`, `Rxn`, or model wrappers.
- Use `FutureBuilder` only for localized one-shot reads that do not need controller ownership. Examples: parts of `wox_setting_ai_view.dart`, `wox_webview_preview.dart`.
- Do not call backend endpoints directly from low-level shared widgets unless the widget is the clear owner of that interaction.

---

## Naming Conventions

- Controllers end with `Controller`, not `Hook`.
- Utility singletons end with `Util`.
- Mixins should use a descriptive `...Mixin` name tied to the reused behavior.
- Do not add `use*` functions; that naming does not match the codebase.

---

## Common Mistakes

- Do not fetch data from `build()` and hope rebuild timing will behave.
- Do not mix controller-owned state and widget-local `setState` for the same data source.
- Do not bypass `WoxApi`/`WoxHttpUtil` with ad-hoc networking code.
- Do not add React-style abstractions to a GetX + Flutter desktop codebase.

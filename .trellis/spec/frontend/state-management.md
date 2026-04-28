# State Management

> How state is managed in this project.

---

## Overview

Wox uses GetX for application-level reactive state, plain widget state for local interaction state, and a few utility singletons for cached configuration.

- Controllers own shared launcher, settings, and AI chat state.
- Widgets use `setState` for ephemeral local UI state only.
- Backend data is fetched manually through `WoxApi`; there is no React Query or Riverpod cache layer.
- WebSocket traffic is also funneled through controller logic rather than spread across arbitrary widgets.

---

## State Categories

- Global app/session state:
  GetX controllers registered in `main.dart`, such as `WoxLauncherController`, `WoxSettingController`, and `WoxAIChatController`.
- Cached configuration:
  utility singletons like `WoxSettingUtil` and `WoxThemeUtil`.
- Local widget state:
  `StatefulWidget` fields for visibility toggles, selection, local loading flags, or platform handles.
- Narrow reactive platform state:
  `ValueNotifier` or controller-like wrappers in specialized platform integrations. Example: Windows webview controller types.

Examples:

- `wox.ui.flutter/wox/lib/main.dart`: registers controllers once at app startup.
- `wox.ui.flutter/wox/lib/controllers/wox_setting_controller.dart`: exposes settings, plugin lists, runtime status, and loading/error flags as GetX observables.
- `wox.ui.flutter/wox/lib/controllers/wox_launcher_controller.dart`: owns launcher-wide query, preview, toolbar, loading, and window behavior state.

---

## When to Use Global State

Promote state into a controller when any of these are true:

- multiple screens or panels need the same data
- the data must survive window show/hide cycles
- backend events or WebSocket messages update it
- keyboard or window-management flows depend on it

Keep state local when it only affects one widget subtree and does not need to be coordinated elsewhere.

---

## Server State

- All backend calls go through `WoxApi`, which delegates transport details to `WoxHttpUtil`.
- Controllers usually fetch, normalize, and then write into `.value`, `.assignAll(...)`, or other GetX observables.
- After mutations, controllers typically reload authoritative data instead of trying to patch every dependent field manually. Example: `updateConfig` reloads settings after calling the backend.
- One-off reads can use `FutureBuilder`, but avoid building whole feature flows around it when the result needs reuse or refresh behavior.

Examples:

- `wox.ui.flutter/wox/lib/api/wox_api.dart`: the single endpoint layer for settings, plugins, themes, AI, and diagnostics.
- `wox.ui.flutter/wox/lib/utils/wox_http_util.dart`: adds `TraceId` and `SessionId` headers and converts the backend response envelope into typed objects.
- `wox.ui.flutter/wox/lib/utils/wox_setting_util.dart`: caches the current settings model outside widget trees.

---

## Scenario: Launcher Temporary Query Sources

### 1. Scope / Trigger

- Trigger: a backend caller opens Wox and will inject or has already injected a temporary query, such as query hotkey, tray query, selection query, or explorer type-to-search.

### 2. Signatures

- Backend payload: `wox.core/common.ShowContext.ShowSource`
- Frontend enum: `wox.ui.flutter/wox/lib/enums/wox_show_source_enum.dart`
- Frontend state gates: `WoxLauncherController.showApp()` and `WoxLauncherController.shouldRestoreQueryAfterHide()`

### 3. Contracts

- `ShowSource=default`: normal launcher open; continue mode may reuse the current query and visible results when calculating initial height.
- Temporary query sources: must be listed in both `shouldPreserveIncomingQuery` and `shouldRestoreQueryAfterHide`.
- Delayed-query sources, such as explorer type-to-search, must still use a non-default source before calling `ShowApp`; otherwise stale continue-mode results can affect initial bounds before `ChangeQuery` arrives.

### 4. Validation & Error Matrix

| Case                                          | Expected Behavior                                                                   |
| --------------------------------------------- | ----------------------------------------------------------------------------------- |
| Temporary source is known                     | Initial height ignores stale results, then the injected query drives result height. |
| Temporary source is missing from restore gate | Hiding Wox leaves the temporary query as the main session.                          |
| Delayed-query source uses `default`           | Continue-mode stale results can make anchored windows open at the wrong position.   |

### 5. Good/Base/Bad Cases

- Good: `ShowSource=explorer` for type-to-search before the later `ChangeQuery("explorer ...")`.
- Base: `ShowSource=default` for a normal launcher toggle with no injected query.
- Bad: a new temporary source only added to the backend enum without adding the matching Dart enum and controller gates.

### 6. Tests Required

- For new temporary launcher sources, add or update smoke coverage that verifies initial window height, injected query display, and previous-query restoration after hide.
- If the source depends on native OS focus hooks and cannot be fully automated, verify with the closest backend/frontend build checks and document the manual scenario.

### 7. Wrong vs Correct

#### Wrong

```go
ShowContext{ShowSource: common.ShowSourceDefault}
```

#### Correct

```go
ShowContext{ShowSource: common.ShowSourceExplorer}
```

---

## Common Mistakes

- Do not use `setState` for data that already belongs to a controller.
- Do not duplicate the same server state in both a utility singleton and a controller without an explicit sync path.
- Do not make API requests from deeply nested presentational widgets unless the widget clearly owns the data lifecycle.
- Do not mutate shared collections in a way that bypasses GetX notifications.

# Quality Guidelines

> Code quality standards for frontend development.

---

## Overview

Frontend work must preserve three things at the same time:

- the shared visual language from Wox theme helpers and shared widgets
- the GetX-based ownership model for app state
- the desktop interaction model around focus, hotkeys, window visibility, and smoke-tested launcher behavior

Comments should stay concise and in English.

---

## Forbidden Patterns

- Do not issue raw HTTP or `Dio` requests from random widgets; go through `WoxApi` and `WoxHttpUtil`.
- Do not introduce React-style hooks or a second state-management library into the existing GetX architecture.
- Do not fetch server data directly from `build()` for reusable or long-lived state.
- Do not restyle common controls from scratch when a shared Wox component already exists.
- Do not add speculative unit tests by default when the user did not ask for them.

---

## Required Patterns

- Generate a trace ID for backend requests and keep transport calls behind `WoxApi`.
- Reuse shared components, theme helpers, and layout wrappers before introducing new UI primitives.
- Keep screen-wide and cross-screen state in controllers; keep `setState` local to widget-owned state.
- For major fixes or feature additions, add or update a matching smoke test under `wox.ui.flutter/wox/integration_test/`.
- Run at least the relevant frontend verification for the touched area before handoff. Common checks are `flutter analyze`, targeted smoke tests, and build verification when packaging behavior changes.

---

## Testing Requirements

- The existing high-value UI safety net is the integration smoke suite under `wox.ui.flutter/wox/integration_test/`.
- `launcher_smoke_test.dart` is the aggregator entry point; individual flows are registered from dedicated smoke test files.
- Major launcher, settings, hotkey, or startup behavior changes should extend these smoke tests.
- If the user did not request unit tests, prefer strengthening smoke/integration coverage over inventing narrow widget tests.

Examples:

- `wox.ui.flutter/wox/integration_test/launcher_smoke_test.dart`: central registration point for launcher smoke coverage.
- `wox.ui.flutter/wox/integration_test/smoke_test_helper.dart`: shared app bootstrap, teardown, and backend-trigger helpers used by smoke tests.
- `wox.ui.flutter/wox/analysis_options.yaml`: enables the standard Flutter lint set used by the project.

---

## Code Review Checklist

- Does the change preserve shared theme/component usage instead of introducing a one-off visual style?
- Is controller ownership still clear, or did widget-local state leak into feature-wide behavior?
- Are backend calls typed, traced, and routed through `WoxApi`?
- If the change affects desktop behavior, keyboard handling, or startup flow, was smoke coverage updated?
- Does the change fit the existing file naming and module structure conventions?

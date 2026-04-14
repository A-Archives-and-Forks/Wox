# Component Guidelines

> How components are built in this project.

---

## Overview

Wox components are Flutter widgets built on top of Material widgets and repo-specific theme helpers.

- Shared visual consistency comes from `WoxThemeUtil`, `colors.dart`, and small wrapper widgets.
- Stateless widgets are preferred when the widget only renders data and delegates behavior outward.
- Stateful widgets are used for local interaction state, platform handles, or async resources that are truly view-local.
- Reusable layout primitives should be extracted once and reused instead of restyling every screen by hand.

---

## Component Structure

- Prefer `StatelessWidget` for pure wrappers. Examples: `wox_button.dart`, `wox_setting_form_field.dart`, `wox_checkbox_tile.dart`.
- Use `StatefulWidget` when the widget owns transient UI state, timers, or async setup. Examples: `wox_setting_view.dart`, `wox_ai_model_selector_view.dart`, `wox_terminal_preview_view.dart`.
- Keep constructor parameters typed and named; provide defaults where a shared component needs a stable style contract.
- Extract repeated layout helpers inside the widget or in a shared base widget/mixin instead of copying layout trees. Example: `WoxSettingPluginItem` and `WoxSettingPluginItemMixin`.

---

## Props Conventions

- Use named parameters and `required` for essential inputs.
- Keep fields strongly typed: `VoidCallback?`, `Widget?`, `double`, `String`, or generics like `WoxDropdownButton<T>`.
- Prefer semantic constructor variants when the component has a fixed design vocabulary. Example: `WoxButton.primary`, `WoxButton.secondary`, `WoxButton.text`.
- Use generic widgets only when they genuinely reuse behavior across types. Examples: `WoxListView<T>`, `WoxDropdownButton<T>`.

---

## Styling Patterns

- Derive colors from theme helpers such as `getThemeTextColor()`, `getThemeActiveBackgroundColor()`, and `WoxThemeUtil.instance.currentTheme`.
- Keep common font sizes and spacing consistent with existing widgets; do not invent a new visual scale per screen.
- Reuse shared layout wrappers for form rows and labels. Example: `WoxSettingFormField`.
- Prefer composing Material widgets with custom styles over creating a fully custom render path unless platform behavior requires it.

Examples:

- `wox.ui.flutter/wox/lib/components/wox_button.dart`: centralizes button variants and theme-aware colors.
- `wox.ui.flutter/wox/lib/components/wox_setting_form_field.dart`: standardizes label/content/tips layout for settings pages.
- `wox.ui.flutter/wox/lib/modules/setting/views/wox_setting_view.dart`: composes navigation and content areas while still reusing lower-level widgets.

---

## Accessibility

- Preserve keyboard behavior for desktop interactions. Example: `WoxSettingView` handles `Escape` through `WoxPlatformFocus`.
- Keep focus management explicit when a widget changes the active panel or input target.
- Prefer descriptive labels and visible affordances over icon-only controls where the action is not obvious.
- Do not break launcher or settings keyboard navigation when introducing new widgets.

---

## Common Mistakes

- Do not hardcode one-off colors or spacing when an existing theme helper or shared component already covers the case.
- Do not put shared async/business state into widget-local `setState` if other screens or controllers also need it.
- Do not start network work from `build()`; trigger it from controllers, lifecycle methods, or one-shot builders where appropriate.
- Do not duplicate plugin-setting layout logic; extend the existing `components/plugin/` patterns instead.

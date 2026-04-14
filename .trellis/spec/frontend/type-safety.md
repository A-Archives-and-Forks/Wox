# Type Safety

> Type safety patterns in this project.

---

## Overview

The UI is written in Dart with strong typing, explicit model classes, and manual JSON mapping.

- Transport models live in `entity/`.
- enums live in `enums/`.
- UI-only supporting models live in `models/`.
- Generic decoding is centralized in `EntityFactory`.

The codebase does not use a schema-validation package such as Zod. Runtime validation is mostly manual at API and model boundaries.

---

## Type Organization

- Define backend-facing payload models in `entity/` with `fromJson` and `toJson`.
- Keep nested setting and validator types in dedicated subfolders under `entity/` when the family is large enough.
- Use enums for stable string codes that cross layers. Examples: `WoxLaunchModeEnum`, `WoxPositionTypeEnum`, `WoxMsgMethodEnum`.
- Keep controller state strongly typed, including generic controller instances. Examples: `WoxListController<WoxQueryResult>`, `Rxn<WoxResultAction>`.

Examples:

- `wox.ui.flutter/wox/lib/entity/wox_setting.dart`: a large transport model with explicit defaults and nested typed lists.
- `wox.ui.flutter/wox/lib/utils/entity_factory.dart`: centralizes generic object creation and list decoding.
- `wox.ui.flutter/wox/lib/controllers/wox_launcher_controller.dart`: uses typed generics and observables instead of dynamic state bags.

---

## Validation

- `WoxHttpUtil` checks the backend response envelope and throws when `success == false`.
- `EntityFactory` logs parse failures and returns safe defaults instead of crashing the whole UI on one malformed item.
- Individual models supply sensible defaults in `fromJson` for missing or nullable fields.
- Plugin-setting validation rules are modeled explicitly under `entity/validator/`.

This is a tolerant desktop client: preserve graceful fallback behavior when adding new fields or decoding new backend responses.

---

## Common Patterns

- Use `.obs`, `Rxn<T>`, and typed model classes together instead of untyped maps.
- Keep JSON field access inside model factories rather than spreading raw `Map<String, dynamic>` lookups through controllers and widgets.
- Prefer enums or typed constants when a string code already exists in `enums/`.
- Use helper factories rather than duplicating type-switch code in every network caller.

---

## Forbidden Patterns

- Do not use raw `dynamic` or untyped `List`/`Map` as long-lived controller state when a model class already exists.
- Do not parse backend JSON directly inside presentation widgets.
- Do not replace existing enums with new hard-coded string literals.
- Do not rely on unchecked casts when `EntityFactory` or a dedicated model constructor already provides the typed path.

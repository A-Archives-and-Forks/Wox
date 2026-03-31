# Plugin Status In Toolbar Design

## Summary

Wox needs a first-class plugin status channel for long-running work such as indexing, downloading, syncing, and background preparation. Today plugins often abuse query results to show temporary progress, which mixes task state with search results and produces poor UI.

This design introduces plugin status as a dedicated launcher concept shown in the bottom toolbar. It supports both plugin-scoped and global-scoped status, integrates with existing toolbar actions, and defines explicit priority rules against notify, doctor information, and result actions.

## Goals

- Give plugins a clean API to show long-running status without faking query results.
- Reuse the existing toolbar as the primary status surface.
- Allow toolbar status to appear even when there are no results.
- Support plugin-scoped status that follows plugin query context.
- Support global-scoped status for background work.
- Support execute actions on status entries.
- Define deterministic competition rules between status, notify, doctor info, and result actions.
- Add smoke tests for all major competition and recovery paths.

## Non-Goals

- No status history panel.
- No multiple visible stacked statuses in the launcher.
- No plugin setting to disable global status in the first version.
- No form actions for status in the first version.
- No unified toolbar content model refactor in the first version.

## Why Toolbar, Not Query Icon

The query icon is too small and too query-centric to carry rich status. It is suitable only for lightweight visual hints such as a spinner or warning badge.

The toolbar already behaves like a launcher footer with left-side message content and right-side actions. It is a better fit for task-oriented status because it can present:

- icon and text
- persistent content
- multiple hotkey-driven actions
- operation-specific priority over result actions

## UX Rules

### Primary Surface

- The bottom toolbar is the primary status surface.
- Toolbar may be shown even when there are no results.
- The query icon may later mirror status with lightweight visual hints, but it is not part of the first implementation contract.

### Status Scope

- `plugin` scope: visible only while the user is in that plugin's query context.
- `global` scope: may be visible outside plugin query context.

### Visibility and Priority

Toolbar content priority is:

1. Active plugin `plugin` scope status
2. Latest visible `global` scope status
3. Doctor info
4. Result actions
5. Hidden

When the user enters a plugin query context, that plugin's visible `plugin` scope status overrides any visible `global` scope status.

### Notify Interaction

- If no visible status occupies the toolbar, `notify` keeps its current behavior and may render in toolbar when Wox is visible.
- If a visible status occupies the toolbar, `notify` must not replace it.
- When status occupies the toolbar, visible-session `notify` falls back to system toast.
- When Wox is hidden, `notify` still goes directly to system toast.

### Global Status Competition

- If multiple plugins publish `global` scope statuses, the latest successful `ShowStatus(global)` wins.
- First version does not maintain a queue or history.

### Plugin Status Lifecycle

- `plugin` scope status is automatically hidden by Wox when the plugin query context is left.
- Re-entering plugin query context does not automatically restore status from UI cache.
- Plugins are expected to decide whether to call `ShowStatus` again after re-entering.

### Status Actions

- Status supports execute actions only.
- Status actions are shown in the existing toolbar actions area.
- Status actions take visual priority over result actions.
- If the visible area is insufficient, lower-priority actions are moved into `More Actions`.
- If hotkeys conflict, status actions win over result actions.
- If default action conflicts, status default action wins over result default action.

## API Design

### Public API Methods

Add the following plugin API methods:

- `ShowStatus`
- `ClearStatus`
- `OnEnterPluginQuery`
- `OnLeavePluginQuery`

`ShowStatus` is an upsert API:

- if `status.id` does not exist for that plugin and scope, create it
- if `status.id` already exists, replace current values and re-render

`ClearStatus` removes the status by id for the calling plugin.

### Status Shape

The first version should keep the payload small and explicit.

```ts
type PluginStatusScope = "plugin" | "global";

interface PluginStatus {
  id: string;
  scope: PluginStatusScope;
  title: string;
  subTitle?: string;
  icon?: WoxImage;
  progress?: number;
  indeterminate?: boolean;
  actions?: StatusAction[];
}
```

### Status Action Shape

Status actions should follow `QueryResultAction` semantics closely, but remain status-specific types in implementation. The first version supports execute actions only.

```ts
interface StatusAction {
  id: string;
  name: string;
  icon?: WoxImage;
  hotkey?: string;
  isDefault?: boolean;
  preventHideAfterAction?: boolean;
  contextData?: Record<string, string>;
  action: (ctx, actionContext) => void | Promise<void>;
}
```

Action context should identify the status source rather than a result:

```ts
interface StatusActionContext {
  statusId: string;
  statusActionId: string;
  contextData: Record<string, string>;
}
```

### Query Lifecycle Events

Add query-context lifecycle callbacks:

- `OnEnterPluginQuery(ctx, callback)`
- `OnLeavePluginQuery(ctx, callback)`

Rules:

- `OnEnterPluginQuery` fires once when crossing from outside the plugin query context into it.
- Continued typing inside the same plugin query context must not re-fire `OnEnterPluginQuery`.
- `OnLeavePluginQuery` fires once when leaving the plugin query context.
- `OnLeavePluginQuery` does not carry a reason in the first version.

These callbacks exist to help plugins decide when to show, refresh, pause, or stop publishing plugin-scoped status. UI correctness must not depend on plugins handling them perfectly.

## Behavioral Examples

### Example 1: Plugin-Scoped Indexing

1. User enters `files ...`
2. Wox fires `OnEnterPluginQuery`
3. Plugin calls `ShowStatus` with scope `plugin`
4. Toolbar shows indexing status even if the result list is empty
5. User leaves `files ...`
6. Wox fires `OnLeavePluginQuery`
7. Wox automatically hides the plugin-scoped status

### Example 2: Global Download

1. Plugin starts a background download
2. Plugin calls `ShowStatus` with scope `global`
3. If the user is not inside another plugin with visible plugin-scoped status, toolbar shows this global status
4. Another plugin later calls `ShowStatus(global)`
5. The newer global status replaces the previous one

### Example 3: Notify While Status Is Visible

1. Toolbar currently shows a visible status
2. A plugin calls `Notify`
3. Toolbar remains unchanged
4. The notify message falls back to system toast

## Architecture Impact

### wox.core

- Add status domain models in the plugin/core boundary.
- Extend plugin API surface with `ShowStatus`, `ClearStatus`, `OnEnterPluginQuery`, `OnLeavePluginQuery`.
- Track visible status state per plugin and scope.
- Track latest active global status.
- Emit enter/leave query lifecycle callbacks when query context crosses plugin boundaries.
- Route notify to system toast when toolbar status is currently active.
- Keep plugin-scoped status automatically hidden when its plugin query context is not active.

### Plugin Hosts

- Add websocket/json-rpc methods for new status APIs and lifecycle callbacks.
- Support execute callbacks for status actions similarly to result actions.
- Preserve context data and hotkey behavior.

### Plugin SDKs

- Add public API methods and type definitions for Node.js and Python.
- Document that `ShowStatus` is upsert and `plugin` scope is only visible in plugin query context.
- Document that status actions support execute only in the first version.

### Flutter UI

- Allow toolbar visibility when status is present even if there are no results.
- Render left-side status content from dedicated status state without forcing a toolbar-source unification refactor yet.
- Merge status actions and result actions for right-side toolbar display, with status actions taking priority.
- Push overflowed lower-priority actions into `More Actions`.
- Preserve existing doctor and result action behavior when no status wins.

## Testing Strategy

This feature requires smoke tests because most failures will be lifecycle or competition bugs rather than isolated unit logic bugs.

The smoke suite should cover at least:

- toolbar shows status with zero results
- plugin-scoped status appears on plugin enter
- plugin-scoped status hides on plugin leave
- continued typing inside the same plugin context does not re-trigger enter
- notify falls back to toast when status occupies toolbar
- notify still uses existing visible/hidden routing when no status is visible
- plugin-scoped status overrides visible global status while inside plugin context
- latest global status wins when multiple global statuses compete
- repeated `ShowStatus` with same id updates instead of duplicating
- `ClearStatus` restores the next visible toolbar source
- status actions render before result actions
- overflowed result actions move into `More Actions`
- status default action wins over result default action
- status hotkey wins over conflicting result action hotkey
- clearing status restores result toolbar actions correctly
- window hide/show and temporary query transitions do not leave stale plugin-scoped status visible

Tests should favor high-signal launcher smoke coverage over large numbers of narrow unit tests.

## Implementation Notes

- Keep the first version incremental. Do not start by introducing a unified toolbar content abstraction.
- Add concise English comments only where intent is not obvious, especially around lifecycle boundaries, status competition, and toolbar fallback behavior.
- Avoid coupling status storage to result storage.
- Reuse existing toolbar action presentation code where it helps, but do not force result-only types directly into status ownership if that makes the model confusing.

## Open Decisions Resolved

- Toolbar is the primary status surface.
- Query icon is not the primary surface.
- API names are `ShowStatus` and `ClearStatus`.
- Status actions support execute only.
- Global status is allowed by default for all plugins in the first version.
- Multiple global statuses compete by latest-write-wins.
- Query lifecycle callbacks are `OnEnterPluginQuery` and `OnLeavePluginQuery`.


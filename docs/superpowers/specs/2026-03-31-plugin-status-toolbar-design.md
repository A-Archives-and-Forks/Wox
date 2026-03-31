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

<<<<<<< HEAD
interface ToolbarMsg {
  id: string;
  source: "notify" | "status" | "doctor";
  scope: PluginStatusScope;
  title: string;
  progress?: number; // 0-100, 如果用户设置了progress, 那么wox会默认在status message旁边展示一个圆形的进度条，表示当前的进度百分比。如果用户没有设置progress, 进度条不会显示。
  indeterminate?: boolean;
  actions?: PluginStatusAction[];
  displaySeconds?: number;
}
```

### Status Action 数据结构

status action 的语义尽量贴近 `QueryResultAction`，但实现上仍然保持为 status 自己的类型。第一版只支持 execute action。

```ts
interface PluginStatusAction {
=======
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
>>>>>>> b4649be9 (docs: add plugin status toolbar design spec)
  id: string;
  name: string;
  icon?: WoxImage;
  hotkey?: string;
  isDefault?: boolean;
  preventHideAfterAction?: boolean;
  contextData?: Record<string, string>;
<<<<<<< HEAD
  action: (ctx, actionContext: PluginStatusActionContext) => void | Promise<void>;
}
```

action context 需要表达 status 来源，而不是 result 来源：

```ts
interface PluginStatusActionContext {
=======
  action: (ctx, actionContext) => void | Promise<void>;
}
```

Action context should identify the status source rather than a result:

```ts
interface StatusActionContext {
>>>>>>> b4649be9 (docs: add plugin status toolbar design spec)
  statusId: string;
  statusActionId: string;
  contextData: Record<string, string>;
}
```

<<<<<<< HEAD

### Query 生命周期事件

# 新增 query-context 生命周期回调：

### Query Lifecycle Events

Add query-context lifecycle callbacks:

> > > > > > > b4649be9 (docs: add plugin status toolbar design spec)

- `OnEnterPluginQuery(ctx, callback)`
- `OnLeavePluginQuery(ctx, callback)`

<<<<<<< HEAD
规则如下：

- `OnEnterPluginQuery` 只在从“插件 query 上下文外”跨到“插件 query 上下文内”时触发一次。
- 在同一个插件 query 上下文中继续输入，不会重复触发 `OnEnterPluginQuery`。
- `OnLeavePluginQuery` 在离开该插件 query 上下文时触发一次。
- 第一版 `OnLeavePluginQuery` 不带 reason。

这些事件的作用是帮助插件决定何时展示、刷新、暂停或停止发布 `plugin scope` status。UI 的正确性不能依赖插件是否完美处理这些事件。

## 行为示例

### 示例 1：插件内索引状态

1. 用户输入 `files ...`
2. Wox 触发 `OnEnterPluginQuery`
3. 插件调用 `ShowToolbarMsg(scope = plugin)`
4. 即使没有 result，toolbar 也显示 indexing status
5. 用户离开 `files ...`
6. Wox 触发 `OnLeavePluginQuery`
7. Wox 自动隐藏该插件的 `plugin scope` status

### 示例 2：全局下载状态

1. 某插件启动后台下载任务
2. 插件调用 `ShowToolbarMsg(scope = global)`
3. 如果用户当前不在其他插件的可见 `plugin scope` status 中，则 toolbar 显示该 global status
4. 另一个插件之后再次调用 `ShowToolbarMsg(global)`
5. 更新的 global status 覆盖前一个 global status

### 示例 3：Status 可见时收到 Notify

1. 当前 toolbar 正在显示某个可见 status
2. 某插件调用 `Notify`
3. toolbar 保持原 status 不变
4. notify 回退为系统 toast

## 架构影响

### wox.core

- 在 plugin/core 边界新增 status 领域模型。
- 扩展插件 API，支持 `ShowToolbarMsg`、`ClearToolbarMsg`、`OnEnterPluginQuery`、`OnLeavePluginQuery`。
- 维护按插件和 scope 存储的 status 状态。
- 维护当前最新的 active global status。
- 当 query 上下文跨插件边界切换时，发出 enter/leave 生命周期回调。
- 当 toolbar 当前被 status 占用时，notify 改走系统 toast。
- 保证不在插件 query 上下文中时，`plugin scope` status 不可见。

### 插件 Host

- 为新增 toolbar msg API 和生命周期回调补 websocket/json-rpc 方法。
- status action 的 execute callback 走与 result action 类似的回调链路。
- 保持 context data 和 hotkey 语义一致。

### 插件 SDK

- 为 Node.js 和 Python SDK 增加新的 public API 与类型定义。
- 文档中明确 `ShowToolbarMsg` 是 upsert 语义。
- 文档中明确 `plugin scope` 只有在插件 query 上下文中可见。
- 文档中明确第一版 status action 只支持 execute。

### Flutter UI

- 当存在 status 时，即使没有 result，也允许 toolbar 显示。
- Flutter 侧统一使用 `ToolbarMsg` 作为渲染模型，notify 与 status 最终都落到这一种对象上。
- 右侧 toolbar action 区需要合并 status actions 与 result actions，并以 status actions 为高优先级。
- 被挤压的低优先级 actions 自动进入 `More Actions`。
- 当没有 status 获胜时，保持 doctor 与 result action 的现有行为。

## 测试策略

这套功能必须补 smoke test，因为大多数 bug 都会出现在生命周期和竞争链路上，而不是单点的纯逻辑函数上。

smoke test 至少覆盖：

- 没有 result 时 toolbar 仍能显示 status
- 进入插件时显示 `plugin scope` status
- 离开插件时自动隐藏 `plugin scope` status
- 在同一个插件上下文中继续输入不会重复触发 enter
- status 占用 toolbar 时，notify 回退为 toast
- 没有 status 时，notify 继续保持现有 visible/hidden 路由
- 在插件上下文中，`plugin scope` status 覆盖可见的 `global scope` status
- 多个 global status 竞争时，最后一次 `ShowToolbarMsg(global)` 获胜
- 对同一个 `status.id` 重复 `ShowToolbarMsg` 会更新而不是重复创建
- `ClearToolbarMsg` 后 toolbar 能恢复到下一层可见来源
- status actions 优先显示在 result actions 之前
- 被挤压的 result actions 会进入 `More Actions`
- status 默认动作优先于 result 默认动作
- status hotkey 优先于冲突的 result action hotkey
- 清理 status 后 result toolbar actions 能正确恢复
- window hide/show、temporary query 等边界切换时，不会残留过期的 `plugin scope` status

测试上优先追求高信号的 launcher smoke coverage，而不是堆大量窄粒度单元测试。

## 实现说明

- 第一版保持增量实现，不要一开始就做超出需要范围的 toolbar 大重构。
- 写代码时补充必要的英文注释，重点放在生命周期边界、status 竞争、toolbar fallback 等不够直观的逻辑上。
- 不要把 status 存储与 result 存储强耦合。
- 可以复用现有 toolbar action 展示链路，但不要为了复用而强行让 result-only 类型直接拥有 status 语义。
- Flutter 渲染层统一只保留一个 `ToolbarMsg` 模型。

## 已确认决策

- toolbar 是主状态展示面
- query icon 不是主展示面
- API 名字使用 `ShowToolbarMsg` 和 `ClearToolbarMsg`
- status action 第一版只支持 execute
- 第一版默认允许所有插件显示 global status
- 多个 global status 使用 latest-write-wins
- # query 生命周期事件使用 `OnEnterPluginQuery` 和 `OnLeavePluginQuery`

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

> > > > > > > b4649be9 (docs: add plugin status toolbar design spec)

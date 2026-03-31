# 插件 Toolbar 状态设计

## 概述

Wox 需要一套一等公民的插件状态通道，用于展示 indexing、下载、同步、后台准备等耗时任务。当前插件通常只能通过伪造 query result 来过渡性展示进度，这会把任务状态和搜索结果混在一起，UI 也比较难看。

本设计引入独立的插件状态能力，并将 bottom toolbar 作为主展示面。状态支持 `plugin` 和 `global` 两种 scope，能够与现有 toolbar action 集成，并且定义清楚它和 notify、doctor 信息、result actions 之间的优先级规则。

## 目标

- 给插件提供一套干净的 API，用于展示长任务状态，而不是伪造 query result。
- 复用现有 toolbar 作为主状态展示面。
- 支持在没有 result 的情况下单独显示 toolbar status。
- 支持跟随插件 query 上下文的 `plugin scope` status。
- 支持后台任务使用的 `global scope` status。
- 支持 status 自带 execute actions。
- 明确定义 status、notify、doctor info、result actions 之间的竞争规则。
- 为主要竞争与恢复链路补齐 smoke test。

## 非目标

- 第一版不做状态历史面板。
- 第一版不做多个可见 status 叠加展示。
- 第一版不做插件级的 global status 开关设置。
- 第一版不支持 status form action。
- 第一版不先做统一的 toolbar content model 重构。

## 为什么选 Toolbar 而不是 Query Icon

query icon 太小，而且语义上更偏向 query context，不适合承载完整状态信息。它最多只适合表达一个轻量视觉信号，比如 spinner 或 warning badge。

toolbar 现在本身就很像 launcher footer，天然分成左侧消息和右侧动作，更适合承载任务状态，因为它能展示：

- icon 和文本
- 持续存在的内容
- 多个带 hotkey 的动作
- 与 result actions 的明确优先级关系

## 交互规则

### 主展示面

- bottom toolbar 是主状态展示面。
- 即使没有 result，也允许显示 toolbar。

### Status Scope

- `plugin` scope：只在用户处于该插件 query 上下文时可见。
- `global` scope：可以在插件 query 上下文之外可见。

### 可见性与优先级

toolbar 内容优先级如下：

1. 当前激活插件的 `plugin` scope status
2. 当前最新生效的 `global` scope status
3. doctor info
4. result actions
5. 隐藏 toolbar

当用户进入某个插件 query 上下文后，该插件的可见 `plugin` scope status 必须覆盖当前可见的 `global` scope status。

### Notify 交互

- 当 toolbar 上没有可见 status 占用时，`notify` 保持现有行为，Wox 可见时仍可显示在 toolbar。
- 当 toolbar 已经被 status 占用时，`notify` 不能覆盖它。
- 当 status 占用 toolbar 时，可见窗口下的 `notify` 回退为系统 toast。
- 当 Wox 不可见时，`notify` 继续直接走系统 toast。

### Global Status 竞争

- 当多个插件同时发布 `global` scope status 时，最后一次成功 `ShowStatus(global)` 的状态获胜。

### Plugin Status 生命周期

- `plugin` scope status 在离开插件 query 上下文时由 Wox 自动隐藏。
- 重新进入插件 query 上下文时，UI 不会自动从缓存恢复旧 status。
- 是否重新 `ShowStatus` 由插件自己决定。

### Status Actions

- status 只支持 execute actions。
- status actions 复用现有 toolbar actions 展示区域。
- status actions 在视觉和交互上优先于 result actions。
- 可见区域不足时，低优先级 action 自动进入 `More Actions`。
- hotkey 冲突时，status actions 优先于 result actions。
- 默认动作冲突时，status 默认动作优先于 result 默认动作。

## API 设计

### Public API

新增以下插件 API：

- `ShowStatus`
- `ClearStatus`
- `OnEnterPluginQuery`
- `OnLeavePluginQuery`

`ShowStatus` 是 upsert 语义：

- 如果该插件该 scope 下不存在同 id status，则创建并显示
- 如果已存在同 id status，则替换当前内容并重新渲染

`ClearStatus` 按 id 清理当前插件名下的 status。

### Status 数据结构

第一版先保持 payload 简洁明确：

```ts
type PluginStatusScope = "plugin" | "global";

interface PluginStatus {
  id: string;
  scope: PluginStatusScope;
  title: string;
  progress?: number; // 0-100, 如果用户设置了progress, 那么wox会默认在status message旁边展示一个圆形的进度条，表示当前的进度百分比。如果用户没有设置progress, 进度条不会显示。
  indeterminate?: boolean;
  actions?: PluginStatusAction[];
}
```

### Status Action 数据结构

status action 的语义尽量贴近 `QueryResultAction`，但实现上仍然保持为 status 自己的类型。第一版只支持 execute action。

```ts
interface PluginStatusAction {
  id: string;
  name: string;
  icon?: WoxImage;
  hotkey?: string;
  isDefault?: boolean;
  preventHideAfterAction?: boolean;
  contextData?: Record<string, string>;
  action: (ctx, actionContext: PluginStatusActionContext) => void | Promise<void>;
}
```

action context 需要表达 status 来源，而不是 result 来源：

```ts
interface PluginStatusActionContext {
  statusId: string;
  statusActionId: string;
  contextData: Record<string, string>;
}
```

### Query 生命周期事件

新增 query-context 生命周期回调：

- `OnEnterPluginQuery(ctx, callback)`
- `OnLeavePluginQuery(ctx, callback)`

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
3. 插件调用 `ShowStatus(scope = plugin)`
4. 即使没有 result，toolbar 也显示 indexing status
5. 用户离开 `files ...`
6. Wox 触发 `OnLeavePluginQuery`
7. Wox 自动隐藏该插件的 `plugin scope` status

### 示例 2：全局下载状态

1. 某插件启动后台下载任务
2. 插件调用 `ShowStatus(scope = global)`
3. 如果用户当前不在其他插件的可见 `plugin scope` status 中，则 toolbar 显示该 global status
4. 另一个插件之后再次调用 `ShowStatus(global)`
5. 更新的 global status 覆盖前一个 global status

### 示例 3：Status 可见时收到 Notify

1. 当前 toolbar 正在显示某个可见 status
2. 某插件调用 `Notify`
3. toolbar 保持原 status 不变
4. notify 回退为系统 toast

## 架构影响

### wox.core

- 在 plugin/core 边界新增 status 领域模型。
- 扩展插件 API，支持 `ShowStatus`、`ClearStatus`、`OnEnterPluginQuery`、`OnLeavePluginQuery`。
- 维护按插件和 scope 存储的 status 状态。
- 维护当前最新的 active global status。
- 当 query 上下文跨插件边界切换时，发出 enter/leave 生命周期回调。
- 当 toolbar 当前被 status 占用时，notify 改走系统 toast。
- 保证不在插件 query 上下文中时，`plugin scope` status 不可见。

### 插件 Host

- 为新增 status API 和生命周期回调补 websocket/json-rpc 方法。
- status action 的 execute callback 走与 result action 类似的回调链路。
- 保持 context data 和 hotkey 语义一致。

### 插件 SDK

- 为 Node.js 和 Python SDK 增加新的 public API 与类型定义。
- 文档中明确 `ShowStatus` 是 upsert 语义。
- 文档中明确 `plugin scope` 只有在插件 query 上下文中可见。
- 文档中明确第一版 status action 只支持 execute。

### Flutter UI

- 当存在 status 时，即使没有 result，也允许 toolbar 显示。
- 左侧 status 内容先独立接入 dedicated status state，不要求第一版先统一 toolbar source model。
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
- 多个 global status 竞争时，最后一次 `ShowStatus(global)` 获胜
- 对同一个 `status.id` 重复 `ShowStatus` 会更新而不是重复创建
- `ClearStatus` 后 toolbar 能恢复到下一层可见来源
- status actions 优先显示在 result actions 之前
- 被挤压的 result actions 会进入 `More Actions`
- status 默认动作优先于 result 默认动作
- status hotkey 优先于冲突的 result action hotkey
- 清理 status 后 result toolbar actions 能正确恢复
- window hide/show、temporary query 等边界切换时，不会残留过期的 `plugin scope` status

测试上优先追求高信号的 launcher smoke coverage，而不是堆大量窄粒度单元测试。

## 实现说明

- 第一版保持增量实现，不要一开始就做统一 toolbar content abstraction。
- 写代码时补充必要的英文注释，重点放在生命周期边界、status 竞争、toolbar fallback 等不够直观的逻辑上。
- 不要把 status 存储与 result 存储强耦合。
- 可以复用现有 toolbar action 展示链路，但不要为了复用而强行让 result-only 类型直接拥有 status 语义。

## 已确认决策

- toolbar 是主状态展示面
- query icon 不是主展示面
- API 名字使用 `ShowStatus` 和 `ClearStatus`
- status action 第一版只支持 execute
- 第一版默认允许所有插件显示 global status
- 多个 global status 使用 latest-write-wins
- query 生命周期事件使用 `OnEnterPluginQuery` 和 `OnLeavePluginQuery`

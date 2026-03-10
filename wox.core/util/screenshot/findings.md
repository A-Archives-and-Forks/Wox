# Findings & Decisions

## Requirements
- New screenshot capability should live under `util`
- Implementation should use CGO
- Must support Windows and macOS
- Shared logic between operating systems should be maximized
- Must support multi-display environments
- User must be able to drag-select a rectangular capture region
- Captured image must support annotation after selection
- Screenshot interaction flow must not use Flutter
- User prefers maximal Go/CGO reuse even for annotation and interaction logic
- User expects a detailed plan first because this is a large change

## Research Findings
- Repository architecture separates Go core (`wox.core`) from Flutter UI (`wox.ui.flutter/wox`)
- `wox.core` is the bridge layer for HTTP/WebSocket communication with the UI
- Workspace launch config already enables `CGO_ENABLED=1` for Go runs
- Project guidance prioritizes clarity, consistency, module boundaries, and preserving semantics
- Existing native utility packages already cover adjacent problems:
- `wox.core/util/screen` handles screen enumeration on Windows, macOS, and Linux
- `wox.core/util/window` already uses CGO / Objective-C for native window control on Windows and macOS
- `wox.core/util/overlay` already contains a Windows native overlay implementation
- `wox.core/util/clipboard` shows the current cross-platform pattern of shared Go API plus OS-specific files
- There is no existing screenshot capture module or annotation pipeline under `util`
- `util/screen` currently exposes a very small shared `Size` model and per-OS `GetMouseScreen` / `GetActiveScreen` APIs
- Windows screen implementation already documents the repo's physical-vs-logical coordinate strategy for DPI-scaled multi-monitor setups
- `util/window` follows the repo convention: thin Go wrapper, OS build tags, native symbols declared in Go, native work implemented in `.c` or `.m`
- `util/overlay` on Windows shows a production pattern for a long-lived native UI surface controlled from Go with callbacks back into Go
- `util/screen/screen_darwin.m` already normalizes macOS coordinates from AppKit's bottom-left system into the top-left model used by Go/UI code
- There is an existing global selection feature wired through `main.go` and `ui/manager.go`
- Current architecture already supports a global hotkey triggering native selection logic and then routing results back into the main Wox UI flow
- This existing trigger path is a strong candidate for a future screenshot hotkey and session lifecycle entry point
- `main.go` registers global hotkeys on the main thread and already initializes selection before UI startup is complete
- `ui.Manager.RegisterSelectionHotkey` shows the expected orchestration point for a future screenshot hotkey
- `ui.Manager.QuerySelection` refreshes active-window state and then drives a UI flow from backend code
- The existing core/UI channel is a generic websocket RPC, not a query-only channel
- `uiImpl.invokeWebsocketMethod` already allows backend-initiated UI commands with request/response semantics
- Current Flutter method enum only defines launcher/terminal related methods, so a screenshot UI flow would need new message types and handlers if annotation is done in Flutter
- `util/clipboard` already supports cross-platform image read/write, including explicit image-byte writes
- `common.WoxImage` already provides in-memory image to PNG/base64/path conversions used elsewhere in the app
- Existing clipboard plugin persists captured images to disk and logs dimensions/hash, which is a useful precedent for screenshot result persistence and deduplication
- The initial screenshot package skeleton can be added without touching current hotkey or UI manager flows yet
- The repository still builds successfully after adding the screenshot package skeleton
- `util/screen` now has a shared `Display` model and `ListDisplays()` API
- Screenshot providers now resolve real display metadata through `util/screen`, instead of returning placeholder data
- Session startup now initializes display list and virtual bounds before later shell/capture steps
- `CaptureProvider` now returns concrete `*image.RGBA` results
- macOS capture is implemented by capturing each display separately and composing/cropping in Go
- Windows capture is implemented with a first-pass GDI `BitBlt` plus 32bpp DIB path
- Shared session state now consumes shell events directly and emits ViewModel snapshots
- The Go-side interaction core is now testable without a native overlay window implementation
- `StartSession` now drives a real minimal lifecycle instead of returning `ErrNotImplemented`
- The current export path captures the selected logical region, renders through a shared renderer, and optionally copies/saves through a shared exporter
- The current macOS shell is now sufficient for drag-select, confirm, cancel, and shell-to-session event flow
- There is now a minimal `plugin/system` entry point for screenshot, so the feature can be exercised from normal Wox query flow instead of only a manual harness
- The first macOS shell implementation used one virtual-desktop window, but that did not provide reliable interaction on non-primary displays in real multi-monitor setups
- `Esc` handling on macOS needed a stronger event path than `keyDown:` on the borderless overlay view alone
- Selection editing now lives in shared Go session logic, including hover hit-test, moving, edge/corner resize, and cursor choice
- The macOS shell now consumes `ActiveHandle` and `Cursor` from the shared `ViewModel` and renders visible resize handles
- The macOS shell now includes separate native toolbar and properties windows that are positioned from shared Go anchors
- Shared `ToolState` now drives native tool selection, undo/redo/confirm enablement, and property controls for color, stroke width, and text size
- Default AppKit button chrome produced incorrect toolbar text colors on the dark overlay, so the toolbar now applies explicit attributed-title colors and custom backgrounds

## Technical Decisions
| Decision | Rationale |
|----------|-----------|
| Start with architecture and rollout plan instead of coding | User explicitly asked for a detailed plan for a large modification |
| Use existing `screen`, `window`, `overlay`, and `clipboard` packages as design references | They define established cross-platform and CGO integration patterns in this repo |
| Treat coordinate normalization as a first-class architecture concern | Existing `screen` code already proves DPI and monitor coordinates are non-trivial on Windows |
| Reuse the existing hotkey and manager orchestration pattern for screenshot entry | The repo already uses this approach for global selection workflows |
| Keep screenshot session orchestration, annotation state, and rendering in Go where possible | User explicitly prefers more shared Go/CGO reuse |
| Standardize screenshot outputs around `image.Image` plus explicit metadata structs | Clipboard and `common.WoxImage` already operate naturally on that representation |
| Keep the screenshot interaction surface out of Flutter, with native shells delegating to shared Go logic | User rejected Flutter for this workflow but allows heavy Go reuse |
| Model the native layer as an event source plus paint target | This keeps OS-specific code focused on windowing/input while Go owns behavior |
| Land the package as a compile-safe skeleton before wiring any existing flows | It fixes interfaces and boundaries early while keeping risk low |
| Centralize monitor topology in `util/screen` before implementing capture and shells | It prevents screenshot-specific code from inventing a second coordinate/displays abstraction |
| Use the same `CaptureRect` and `CaptureDisplays` contract across platforms even if native implementation details differ | Shared session and export logic should not care whether capture is global, per-display, or stitched |
| Build the session state machine before the native overlay shell | It reduces platform-specific debugging by proving interaction rules in Go first |
| Make `StartSession` block on a shared session outcome channel | It keeps the API synchronous while still allowing the native shell to drive the lifecycle asynchronously |
| Keep renderer/exporter shared even before annotations are implemented | It stabilizes the end-to-end pipeline early and avoids a later refactor when native chrome becomes richer |
| Add a system plugin before richer native chrome is finished | It gives the feature a real product entry point early and makes manual verification easier from normal Wox usage |
| Use one overlay window per display on macOS instead of one oversized virtual-desktop window | It matches AppKit's multi-screen behavior better and makes interaction reliable on secondary displays |
| Add a local key monitor for `Esc`/`Return` on macOS | Borderless overlay windows are not a reliable sole source of keyboard events during screenshot sessions |
| Keep move/resize semantics in shared Go geometry/session code | It avoids re-implementing selection behavior per platform and makes the behavior unit-testable |
| Keep toolbar visibility, property visibility, and tool/property values in the shared `ViewModel` | It keeps native chrome thin and prevents the macOS and Windows shells from diverging on control state |
| Style toolbar buttons explicitly instead of relying on default AppKit tinting | The dark screenshot chrome needs stable white, green, and red foreground colors across states |

## Issues Encountered
| Issue | Resolution |
|-------|------------|
| Previous session catchup reported unsynced exploration with no planning files | Reconstructed state in planning artifacts and continue from current repo state |
| Search output for websocket-related symbols was too broad to inspect directly | Switched to targeted reads of `ui/http.go`, `ui_impl.go`, and Flutter websocket enums |
| Original revision pushed too much annotation/UI responsibility into native code | Revised architecture so native code can stay thin while shared Go owns state, rendering, and most behavior |
| `make build` surfaced existing warnings in unrelated native modules | Confirmed the build still completed successfully and treated them as pre-existing noise, not regressions from the screenshot package |
| Pixel work-area normalization on macOS still uses scale-adjusted logical coordinates as an interim approximation | Acceptable for current session/bootstrap work; revisit when implementing actual pixel capture composition |
| macOS 15 SDK marks old CoreGraphics screenshot entry points unavailable at compile time | Switched to dynamic symbol lookup and per-display composition so the package still compiles under the current SDK |
| Current session interaction now supports selection create/move/resize, confirm/cancel, and basic native tool/property controls | Annotation object editing and Windows shell parity remain to be implemented |
| Current multi-display export composition uses per-display logical-to-pixel mapping and global pixel unioning | Good enough for the current MVP path, but mixed-DPI edge cases still need validation on real Windows/macOS multi-monitor hardware |
| Native macOS chrome now exists, but annotation object creation and editing are still not implemented | The next slice should wire tool drags into document mutations and preview rendering |

## Resources
- `AGENTS.md`
- `README.md`
- `Wox.code-workspace`
- `/Users/qianlifeng/.codex/skills/planning-with-file/SKILL.md`
- `wox.core/util/screen`
- `wox.core/util/window`
- `wox.core/util/overlay`
- `wox.core/util/clipboard`
- `wox.core/util/screen/screen_windows.go`
- `wox.core/util/window/window_windows.go`
- `wox.core/util/window/window_darwin.go`
- `wox.core/util/overlay/overlay_windows.go`
- `wox.core/util/screen/screen_darwin.m`
- `wox.core/main.go`
- `wox.core/ui/manager.go`
- `wox.core/ui/http.go`
- `wox.core/ui/ui_impl.go`
- `wox.ui.flutter/wox/lib/enums/wox_msg_method_enum.dart`
- `wox.ui.flutter/wox/lib/utils/wox_websocket_msg_util.dart`
- `wox.core/util/clipboard/clipboard.go`
- `wox.core/util/clipboard/clipboard_windows.go`
- `wox.core/util/clipboard/clipboard_darwin.go`
- `wox.core/common/image.go`

## Visual/Browser Findings
- Reference image shows a full-screen dimmed overlay over the desktop
- Selection rectangle includes visible border, resize handles, and live width/height label
- Annotation toolbar appears near the selected area after region selection
- Toolbar includes primitives similar to rectangle, ellipse, arrow, pen, mosaic/blur, text, undo, save/share, cancel, and confirm

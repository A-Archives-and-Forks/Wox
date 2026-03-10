# Task Plan: Cross-platform Screenshot Module

## Goal
Design a detailed implementation plan for a CGO-based screenshot module under `wox.core/util` that supports Windows and macOS, maximizes cross-platform reuse, handles multi-monitor capture, interactive rectangle selection, and post-capture annotation.

## Current Phase
Phase 8

## Phases
### Phase 1: Requirements & Discovery
- [x] Understand user intent
- [x] Identify constraints and requirements
- [x] Document findings in findings.md
- **Status:** complete

### Phase 2: Existing Architecture Mapping
- [x] Inspect current `util`, CGO, windowing, event, and UI bridge code
- [x] Identify reuse points and integration seams
- [x] Record constraints and risks
- **Status:** complete

### Phase 3: Module Architecture Design
- [x] Define shared abstractions and package layout
- [x] Split shared logic vs OS adapters
- [x] Define lifecycle, threading, and data models
- **Status:** complete

### Phase 4: Delivery Plan
- [x] Break work into implementation milestones
- [x] Define testing and verification strategy
- [x] Deliver detailed plan and risk list
- **Status:** complete

### Phase 5: Package Skeleton
- [x] Create compile-safe `util/screenshot` package boundaries
- [x] Add shared types, bridge protocol, and session scaffolding
- [x] Add OS-specific shell/capture placeholders
- **Status:** complete

### Phase 6: Next Implementation Slice
- [x] Expand `screen` into full display enumeration
- [x] Implement capture provider on macOS and Windows
- [x] Wire shell event loop to shared session state
- [x] Wire `StartSession` into a minimal capture/export lifecycle
- **Status:** complete

### Phase 7: Native Shell Expansion
- [x] Land a minimal macOS overlay shell that can emit mouse and keyboard events
- [x] Keep shell rendering driven by shared `ViewModel`
- [x] Add a minimal Wox system-plugin entry for launching screenshot sessions
- [x] Add native resize handles and move/resize hit-testing
- [x] Add native toolbar and editor panel windows
- [x] Mirror shared tool/property state into native controls
- **Status:** in_progress

## Key Questions
1. Which responsibilities should stay in native CGO code, and which should stay in Go shared logic?
2. How should the screenshot overlay and annotation surface integrate with Wox's existing UI/runtime boundaries?
3. What coordinate model is required to make multi-display and HiDPI behavior consistent across Windows and macOS?

## Decisions Made
| Decision | Rationale |
|----------|-----------|
| Use file-based planning artifacts for this task | The request is a large architecture change and needs persistent working state |
| Keep screenshot orchestration inside `wox.core` | Global hotkeys, native access, and session control already live there |
| Maximize shared Go and CGO code, and keep only unavoidable shell/UI differences in OS-specific files | User explicitly prefers more Go/CGO reuse as long as Flutter is not involved |
| Separate capture, interaction session, annotation document, and export as distinct layers | It reduces OS-specific duplication and makes later Linux support possible |
| Use a unified top-left virtual desktop coordinate model in shared code | Existing `screen` code already normalizes coordinates differently per OS |
| Keep the screenshot interaction surface out of Flutter; native windows can be thin shells over shared Go state and rendering | User allows more Go reuse, but still requires the screenshot workflow to avoid Flutter |
| Use a native-shell event bridge instead of duplicating interaction logic per OS | Shared Go session and document logic will reduce cross-platform divergence |
| Land a compile-safe package skeleton before wiring existing flows | It fixes interfaces and boundaries early while keeping integration risk low |
| Build screenshot display discovery on top of a shared `util/screen` display model | Capture, session setup, and later overlay placement all need the same monitor topology data |
| On macOS, compose rect captures in Go from per-display captures instead of relying on deprecated global CoreGraphics screenshot entry points | This keeps the package compilable on the macOS 15 SDK while preserving a shared capture interface |
| Treat native shells as event producers and ViewModel consumers from day one | This keeps shared interaction logic testable before native window work is complete |

## Errors Encountered
| Error | Attempt | Resolution |
|-------|---------|------------|
| Previous session timed out before planning files were created | 1 | Started fresh planning files and will reconcile with current repo state |

## Notes
- Follow repository rules from `AGENTS.md`
- Comments must stay concise and in English if code is added later
- If implementation happens later, verify with `make build` in `wox.core`
- Recommended implementation order:
- 1. Shared geometry/session model
- 2. Shared renderer and event protocol
- 3. Screen snapshot capture adapters
- 4. Native overlay selection shells
- 5. Native annotation chrome shells
- 6. Export/copy/save pipeline
- 7. Hotkey and UI integration

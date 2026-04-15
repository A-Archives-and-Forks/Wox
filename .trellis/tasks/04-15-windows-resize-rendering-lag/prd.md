# Fix Windows Resize Rendering Lag

## Goal
Investigate and fix the Windows launcher rendering issue where automatic height changes after query result updates leave the result list visually stale, clipped, or misaligned until another query-driven resize occurs.

## Known Behavior
- The issue is most visible on Windows when query results change and Wox automatically adjusts its height.
- The visible failure is in the result content area rather than only the transparent background or acrylic material.
- A subsequent query height adjustment usually restores the correct rendering.

## Requirements
- Narrow the root cause to the resize path between Flutter result updates and the Windows native window resize handling.
- Preserve the existing Windows transparent/acrylic window behavior.
- Preserve current launcher auto-resize behavior for query results, preview, and toolbar visibility.
- Avoid introducing visible flicker or extra resize jitter while fixing the stale render.

## Open Questions
- Whether the issue happens only when the window grows taller, or also when it shrinks.
- Whether the problem is specific to list layout, grid layout, or both.
- Whether the failure depends on DPI scale, toolbar visibility, or preview state.

## Acceptance Criteria
- [ ] Query-result-driven auto-resize on Windows no longer leaves the result list clipped, stale, or visually out of sync.
- [ ] The fix targets the root cause in the resize/render timing chain instead of relying on repeated user-triggered resize recovery.
- [ ] Existing launcher resize flows still behave correctly on Windows.

## Technical Notes
- Relevant paths already identified:
  - `wox.ui.flutter/wox/lib/controllers/wox_launcher_controller.dart`
  - `wox.ui.flutter/wox/lib/modules/launcher/views/wox_launcher_view.dart`
  - `wox.ui.flutter/wox/lib/modules/launcher/views/wox_query_result_view.dart`
  - `wox.ui.flutter/wox/lib/utils/windows/windows_window_manager.dart`
  - `wox.ui.flutter/wox/windows/runner/flutter_window.cpp`
  - `wox.ui.flutter/wox/windows/runner/win32_window.cpp`

# Fix Tray Query Anchor Positioning

## Goal
Fix Windows tray query popup positioning so tray-triggered windows stay anchored near the tray icon even when the real initial window height differs from the backend's estimate, especially when the query box is hidden and preview-only layout is used.

## Requirements
- Move tray query positioning responsibility from backend top-left estimation to UI final-height calculation.
- Preserve current tray icon screen detection and anchor selection in backend.
- Keep existing non-tray window positioning behavior unchanged.
- Preserve Windows upward growth behavior after the window is shown.
- Keep the cross-layer payload explicit and typed.

## Acceptance Criteria
- [ ] Tray query on Windows can open near the tray icon when `HideQueryBox=true`.
- [ ] Preview-only tray queries with `resultPreviewRatio=0` use the same tray anchor instead of drifting away because of height mismatches.
- [ ] Existing non-tray show flows still use their current position handling.
- [ ] Build verification for touched backend code succeeds.

## Technical Notes
- Backend should send tray anchor metadata instead of a precomputed tray query top-left position.
- Flutter should compute the actual top-left using `targetHeight` from its own layout calculation before calling `setBounds`.
- The fix should stay scoped to tray query flows and avoid broad changes to shared positioning behavior.

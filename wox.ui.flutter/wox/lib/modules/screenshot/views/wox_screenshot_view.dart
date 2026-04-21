import 'dart:math' as math;

import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:get/get.dart';
import 'package:uuid/v4.dart';
import 'package:wox/controllers/wox_screenshot_controller.dart';
import 'package:wox/entity/screenshot_session.dart';

const Key screenshotCanvasKey = Key('screenshot-canvas');
const Key screenshotToolbarKey = Key('screenshot-toolbar');
const Key screenshotEditBarKey = Key('screenshot-edit-bar');
const Key screenshotConfirmKey = Key('screenshot-confirm');
const Key screenshotCancelKey = Key('screenshot-cancel');
const Key screenshotUndoKey = Key('screenshot-undo');
const Key screenshotToolSelectKey = Key('screenshot-tool-select');
const Key screenshotToolRectKey = Key('screenshot-tool-rect');
const Key screenshotToolEllipseKey = Key('screenshot-tool-ellipse');
const Key screenshotToolArrowKey = Key('screenshot-tool-arrow');
const Key screenshotToolTextKey = Key('screenshot-tool-text');

const List<Color> _annotationPalette = <Color>[Color(0xFFFF5B36), Color(0xFFF9C74F), Color(0xFF29FF72), Color(0xFF4DA3FF), Color(0xFFC77DFF), Color(0xFFFFFFFF)];
const double _selectionHandleSize = 12;
const double _annotationHandleSize = 12;
const double _selectionEdgeTolerance = 7;
const double _textDraftMaxWidth = 480;

class WoxScreenshotView extends StatefulWidget {
  const WoxScreenshotView({super.key});

  @override
  State<WoxScreenshotView> createState() => _WoxScreenshotViewState();
}

enum _InteractionMode { createSelection, moveSelection, resizeSelection, createAnnotation, moveAnnotation, resizeShapeAnnotation, moveArrowStart, moveArrowEnd, moveText }

enum _ResizeHandle { topLeft, top, topRight, right, bottomRight, bottom, bottomLeft, left }

enum _AnnotationHandle { topLeft, top, topRight, right, bottomRight, bottom, bottomLeft, left, arrowStart, arrowEnd }

class _AnnotationHitTarget {
  const _AnnotationHitTarget({required this.annotation, this.handle, required this.cursor});

  final ScreenshotAnnotation annotation;
  final _AnnotationHandle? handle;
  final MouseCursor cursor;
}

class _WoxScreenshotViewState extends State<WoxScreenshotView> {
  final controller = Get.find<WoxScreenshotController>();
  final focusNode = FocusNode(debugLabel: 'screenshot-workspace');
  bool _isCancellingSession = false;

  _InteractionMode? _interactionMode;
  _ResizeHandle? _resizeHandle;
  _AnnotationHandle? _annotationHandle;
  Offset? _dragStartGlobal;
  Rect? _selectionAtDragStart;
  Rect? _annotationDraftRect;
  Offset? _annotationStart;
  Offset? _annotationEnd;
  String? _dragAnnotationId;
  ScreenshotAnnotation? _annotationAtDragStart;
  MouseCursor _hoverCursor = SystemMouseCursors.basic;

  @override
  void initState() {
    super.initState();
    HardwareKeyboard.instance.addHandler(_handleGlobalScreenshotKeyEvent);
    WidgetsBinding.instance.addPostFrameCallback((_) {
      if (mounted) {
        focusNode.requestFocus();
      }
    });
  }

  @override
  void dispose() {
    HardwareKeyboard.instance.removeHandler(_handleGlobalScreenshotKeyEvent);
    focusNode.dispose();
    super.dispose();
  }

  bool _handleGlobalScreenshotKeyEvent(KeyEvent event) {
    if ((event is! KeyDownEvent && event is! KeyRepeatEvent) || event.logicalKey != LogicalKeyboardKey.escape) {
      return false;
    }

    // The screenshot workflow used to rely on the workspace Focus node receiving Escape. That was
    // not reliable once annotation text fields took focus or a toolbar click left the page without a
    // stable primary focus, so Escape stopped dismissing the active screenshot session. A dedicated
    // screenshot-level HardwareKeyboard handler keeps the cancel shortcut working anywhere inside the
    // annotation UI without changing the rest of the keyboard flow.
    _cancelSessionFromKeyboard();
    return true;
  }

  void _cancelSessionFromKeyboard() {
    if (_isCancellingSession || !controller.isSessionActive.value) {
      return;
    }

    _isCancellingSession = true;
    controller.cancelSession(const UuidV4().generate(), reason: 'keyboard_escape').whenComplete(() {
      if (mounted) {
        _isCancellingSession = false;
      }
    });
  }

  @override
  Widget build(BuildContext context) {
    return Obx(() {
      final stage = controller.stage.value;
      final virtualBounds = controller.virtualBoundsRect;

      return Focus(
        focusNode: focusNode,
        autofocus: true,
        onKeyEvent: _handleWorkspaceKeyEvent,
        child: Material(
          color: Colors.transparent,
          child: stage == ScreenshotSessionStage.loading ? _LoadingView(label: controller.tr('plugin_screenshot_capture_title')) : _buildWorkspace(context, virtualBounds),
        ),
      );
    });
  }

  KeyEventResult _handleWorkspaceKeyEvent(FocusNode node, KeyEvent event) {
    if (event is! KeyDownEvent) {
      return KeyEventResult.ignored;
    }

    if (event.logicalKey == LogicalKeyboardKey.escape) {
      _cancelSessionFromKeyboard();
      return KeyEventResult.handled;
    }

    if (controller.textDraftPosition.value != null) {
      return KeyEventResult.ignored;
    }

    if (event.logicalKey == LogicalKeyboardKey.enter) {
      if (controller.selectionRect != null) {
        controller.confirmSelection(const UuidV4().generate());
      }
      return KeyEventResult.handled;
    }

    if (event.logicalKey == LogicalKeyboardKey.delete || event.logicalKey == LogicalKeyboardKey.backspace) {
      controller.deleteSelectedAnnotation();
      return KeyEventResult.handled;
    }

    if ((HardwareKeyboard.instance.isControlPressed || HardwareKeyboard.instance.isMetaPressed) && event.logicalKey == LogicalKeyboardKey.keyZ) {
      controller.undoAnnotation();
      return KeyEventResult.handled;
    }

    return KeyEventResult.ignored;
  }

  Widget _buildWorkspace(BuildContext context, Rect virtualBounds) {
    return Stack(
      children: [
        MouseRegion(
          cursor: _hoverCursor,
          onHover: (event) => _handleHover(event.localPosition),
          onExit: (_) => _setHoverCursor(SystemMouseCursors.basic),
          child: GestureDetector(
            key: screenshotCanvasKey,
            behavior: HitTestBehavior.translucent,
            onPanStart: (details) => _handlePanStart(details.localPosition),
            onPanUpdate: (details) => _handlePanUpdate(details.localPosition),
            onPanEnd: (_) => _handlePanEnd(),
            onTapDown: (details) => _handleTap(details.localPosition),
            onDoubleTapDown: (details) => _handleDoubleTap(details.localPosition),
            child: Stack(
              children: [
                RepaintBoundary(
                  // Dragging annotations used to visually disturb the captured background because the
                  // entire workspace repainted on every pointer update. Keeping the snapshots isolated in
                  // their own repaint boundary limits redraws to overlays that actually changed.
                  child: _WorkspaceBackground(snapshots: controller.displaySnapshots.toList(), virtualBounds: virtualBounds),
                ),
                Obx(() {
                  final selectionRect = controller.selectionRect;
                  final selectionLocalRect = selectionRect?.shift(-virtualBounds.topLeft);
                  final textDraftPosition = controller.textDraftPosition.value;
                  final selectedAnnotationId = controller.selectedAnnotationId.value;

                  return Stack(
                    children: [
                      Positioned.fill(
                        child: CustomPaint(
                          painter: _WorkspaceShadePainter(
                            selectionRect: selectionLocalRect,
                            selectionSizeLabel: selectionRect == null ? null : '${selectionRect.width.round()} x ${selectionRect.height.round()}',
                          ),
                        ),
                      ),
                      if (selectionRect != null)
                        Positioned.fill(
                          child: RepaintBoundary(
                            child: CustomPaint(
                              painter: _AnnotationPainter(
                                annotations: controller.annotations.toList(),
                                canvasOrigin: virtualBounds.topLeft,
                                selectionClipRect: selectionLocalRect,
                                draftRect: _annotationDraftRect,
                                draftStart: _annotationStart,
                                draftEnd: _annotationEnd,
                                draftType: _currentDraftType(),
                                previewColor: controller.annotationCreationColor.value,
                                selectedAnnotationId: selectedAnnotationId,
                                editingTextAnnotationId: controller.editingTextAnnotationId.value,
                              ),
                            ),
                          ),
                        ),
                      if (selectionLocalRect != null) _buildSelectionFrame(selectionLocalRect),
                      if (textDraftPosition != null && selectionRect != null) _buildTextDraftField(textDraftPosition - virtualBounds.topLeft),
                    ],
                  );
                }),
              ],
            ),
          ),
        ),
        // The toolbar and edit bar used to live inside the canvas GestureDetector, so clicking a
        // swatch or action button also triggered the workspace tap handler and cleared the current
        // annotation selection first. Lifting those overlays above the gesture layer keeps their
        // controls interactive without letting canvas hit-testing cancel the active edit target.
        Obx(() => _buildToolbar(context, controller.selectionRect, virtualBounds)),
        Obx(() => _buildEditBar(virtualBounds)),
      ],
    );
  }

  Widget _buildToolbar(BuildContext context, Rect? selectionRect, Rect virtualBounds) {
    final currentTool = controller.currentTool.value;
    final canConfirm = selectionRect != null && selectionRect.width >= 1 && selectionRect.height >= 1;
    final selectionLocalRect = selectionRect?.shift(-virtualBounds.topLeft);
    final creationColor = controller.annotationCreationColor.value;

    return Positioned.fill(
      child: CustomSingleChildLayout(
        delegate: _SelectionToolbarLayoutDelegate(selectionRect: selectionLocalRect),
        // The creation toolbar stays attached to the active capture rect so tool switching and new
        // annotation placement happen near the selected region instead of forcing long pointer
        // travel to the edge of the screen.
        child: SingleChildScrollView(
          scrollDirection: Axis.horizontal,
          child: Container(
            key: screenshotToolbarKey,
            padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 10),
            decoration: BoxDecoration(
              color: const Color(0xCC1E1A18),
              borderRadius: BorderRadius.circular(18),
              boxShadow: const [BoxShadow(color: Color(0x55000000), blurRadius: 24, offset: Offset(0, 12))],
            ),
            child: Row(
              mainAxisSize: MainAxisSize.min,
              children: [
                _ToolButton(
                  key: screenshotToolSelectKey,
                  icon: Icons.select_all,
                  selected: currentTool == ScreenshotTool.select,
                  activateOnTapDown: true,
                  onPressed: () => controller.setTool(ScreenshotTool.select),
                ),
                _ToolButton(
                  key: screenshotToolRectKey,
                  icon: Icons.crop_square,
                  selected: currentTool == ScreenshotTool.rect,
                  activateOnTapDown: true,
                  onPressed: () => controller.setTool(ScreenshotTool.rect),
                ),
                _ToolButton(
                  key: screenshotToolEllipseKey,
                  icon: Icons.circle_outlined,
                  selected: currentTool == ScreenshotTool.ellipse,
                  activateOnTapDown: true,
                  onPressed: () => controller.setTool(ScreenshotTool.ellipse),
                ),
                _ToolButton(
                  key: screenshotToolTextKey,
                  icon: Icons.text_fields,
                  selected: currentTool == ScreenshotTool.text,
                  activateOnTapDown: true,
                  onPressed: () => controller.setTool(ScreenshotTool.text),
                ),
                _ToolButton(
                  key: screenshotToolArrowKey,
                  icon: Icons.north_east,
                  selected: currentTool == ScreenshotTool.arrow,
                  activateOnTapDown: true,
                  onPressed: () => controller.setTool(ScreenshotTool.arrow),
                ),
                const SizedBox(width: 10),
                _buildColorPalette(selectedColor: creationColor, onColorSelected: controller.setAnnotationCreationColor, compact: true),
                const SizedBox(width: 6),
                _ToolButton(key: screenshotUndoKey, icon: Icons.undo, enabled: controller.annotations.isNotEmpty, onPressed: controller.undoAnnotation),
                const SizedBox(width: 6),
                _ToolButton(
                  key: screenshotCancelKey,
                  icon: Icons.close,
                  color: const Color(0xFFFF6B6B),
                  onPressed: () => controller.cancelSession(const UuidV4().generate(), reason: 'toolbar_cancel_button'),
                ),
                _ToolButton(
                  key: screenshotConfirmKey,
                  icon: Icons.check,
                  color: const Color(0xFF30E37A),
                  enabled: canConfirm,
                  onPressed:
                      canConfirm
                          ? () {
                            if (controller.textDraftPosition.value != null) {
                              controller.commitTextDraft();
                            }
                            controller.confirmSelection(const UuidV4().generate());
                          }
                          : null,
                ),
              ],
            ),
          ),
        ),
      ),
    );
  }

  Widget _buildEditBar(Rect virtualBounds) {
    final selectionRect = controller.selectionRect;
    final selectedAnnotation = controller.selectedAnnotation;
    if (selectionRect == null || selectedAnnotation == null) {
      return const SizedBox.shrink();
    }

    final selectionLocalRect = selectionRect.shift(-virtualBounds.topLeft);
    final annotationLocalRect = screenshotAnnotationBounds(selectedAnnotation)?.shift(-virtualBounds.topLeft);

    return Positioned.fill(
      child: CustomSingleChildLayout(
        delegate: _SelectionEditBarLayoutDelegate(selectionRect: selectionLocalRect, anchorRect: annotationLocalRect),
        // Existing annotations now use a dedicated edit bar anchored beside the selection frame.
        // Keeping edit controls outside the captured content avoids covering the annotation while
        // still keeping color/delete/font actions spatially tied to the selected element.
        child: Container(
          key: screenshotEditBarKey,
          width: 92,
          padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 12),
          decoration: BoxDecoration(
            color: const Color(0xD91B1715),
            borderRadius: BorderRadius.circular(18),
            boxShadow: const [BoxShadow(color: Color(0x55000000), blurRadius: 20, offset: Offset(0, 10))],
          ),
          child: Column(
            mainAxisSize: MainAxisSize.min,
            children: [
              _buildColorPalette(selectedColor: selectedAnnotation.color, onColorSelected: controller.updateSelectedAnnotationColor),
              if (selectedAnnotation.type == ScreenshotAnnotationType.text) ...[
                const SizedBox(height: 10),
                _EditActionButton(icon: Icons.remove, onPressed: () => controller.updateSelectedTextFontSize(-2)),
                Padding(
                  padding: const EdgeInsets.symmetric(vertical: 4),
                  child: Text('${selectedAnnotation.fontSize.round()}', style: const TextStyle(color: Colors.white, fontSize: 12, fontWeight: FontWeight.w700)),
                ),
                _EditActionButton(icon: Icons.add, onPressed: () => controller.updateSelectedTextFontSize(2)),
              ],
              const SizedBox(height: 10),
              _EditActionButton(icon: Icons.delete_outline, color: const Color(0xFFFF6B6B), onPressed: controller.deleteSelectedAnnotation),
            ],
          ),
        ),
      ),
    );
  }

  Widget _buildColorPalette({required Color selectedColor, required ValueChanged<Color> onColorSelected, bool compact = false}) {
    final paletteChildren =
        _annotationPalette
            .map((color) => _ColorSwatchButton(color: color, selected: selectedColor.toARGB32() == color.toARGB32(), onPressed: () => onColorSelected(color), compact: compact))
            .toList();

    return SizedBox(width: compact ? 56 : 72, child: Wrap(spacing: compact ? 4 : 8, runSpacing: compact ? 4 : 8, alignment: WrapAlignment.center, children: paletteChildren));
  }

  Widget _buildSelectionFrame(Rect selectionLocalRect) {
    const borderColor = Color(0xFF29FF72);

    final handles = _ResizeHandle.values.map((handle) {
      final position = _handleOffsetForRect(selectionLocalRect, handle);
      return Positioned(
        left: position.dx - _selectionHandleSize / 2,
        top: position.dy - _selectionHandleSize / 2,
        child: Container(
          width: _selectionHandleSize,
          height: _selectionHandleSize,
          decoration: BoxDecoration(color: borderColor, border: Border.all(color: Colors.black.withValues(alpha: 0.45), width: 1), borderRadius: BorderRadius.circular(4)),
        ),
      );
    });

    return Stack(
      children: [
        Positioned.fromRect(
          rect: selectionLocalRect,
          child: IgnorePointer(
            // The previous frame decoration used a full-rect box shadow, which visually bled into
            // the selected area and made the "transparent" capture region look slightly grey. Keep
            // the frame to a pure stroke so the user sees the raw screenshot pixels inside the crop.
            child: Container(decoration: BoxDecoration(border: Border.all(color: borderColor, width: 2))),
          ),
        ),
        ...handles,
      ],
    );
  }

  Widget _buildTextDraftField(Offset localPosition) {
    return Positioned(
      left: localPosition.dx,
      top: localPosition.dy,
      child: MouseRegion(
        cursor: SystemMouseCursors.text,
        child: Material(
          type: MaterialType.transparency,
          child: ConstrainedBox(
            // The draft editor now sits directly on top of the rendered text instead of inside a
            // decorated popup. Matching the painter width cap keeps wrapping identical while the
            // transparent field makes the edit state look like the same annotation gaining a caret.
            constraints: const BoxConstraints(minWidth: 24, maxWidth: _textDraftMaxWidth),
            child: TextField(
              controller: controller.textDraftController,
              autofocus: true,
              maxLines: null,
              minLines: 1,
              cursorColor: controller.textDraftColor.value,
              style: buildScreenshotTextStyle(color: controller.textDraftColor.value, fontSize: controller.textDraftFontSize.value),
              decoration: const InputDecoration.collapsed(hintText: ''),
              onSubmitted: (_) => controller.commitTextDraft(),
              onTapOutside: (_) => controller.commitTextDraft(),
            ),
          ),
        ),
      ),
    );
  }

  void _handleTap(Offset localPosition) {
    if (controller.textDraftPosition.value != null) {
      return;
    }

    final globalPosition = _toGlobalPosition(localPosition);
    final annotation = _hitTestAnnotationBody(globalPosition);
    if (annotation != null) {
      // Text annotations should feel editable in place. A single click now both selects the label
      // and opens the inline editor so the cursor changes to text mode immediately instead of
      // forcing the user through a separate double-click gesture.
      if (annotation.type == ScreenshotAnnotationType.text && annotation.start != null) {
        controller.selectAnnotation(annotation.id);
        controller.startTextDraft(annotation.start!, annotationId: annotation.id, initialText: annotation.text ?? '', fontSize: annotation.fontSize, color: annotation.color);
        return;
      }

      // Non-text annotations still use single-click selection without entering a secondary mode.
      controller.selectAnnotation(annotation.id);
      return;
    }

    switch (controller.currentTool.value) {
      case ScreenshotTool.select:
        controller.selectAnnotation(null);
        return;
      case ScreenshotTool.rect:
      case ScreenshotTool.ellipse:
      case ScreenshotTool.arrow:
        controller.selectAnnotation(null);
        return;
      case ScreenshotTool.text:
        final selectionRect = controller.selectionRect;
        if (selectionRect == null || !selectionRect.contains(globalPosition)) {
          controller.selectAnnotation(null);
          return;
        }

        controller.selectAnnotation(null);
        controller.startTextDraft(globalPosition, fontSize: controller.textDraftFontSize.value, color: controller.annotationCreationColor.value);
        return;
    }
  }

  void _handleDoubleTap(Offset localPosition) {
    final globalPosition = _toGlobalPosition(localPosition);
    final annotation = _hitTestAnnotationBody(globalPosition, allowOnlyText: true);
    if (annotation == null || annotation.type != ScreenshotAnnotationType.text || annotation.start == null) {
      return;
    }

    controller.selectAnnotation(annotation.id);
    controller.startTextDraft(annotation.start!, annotationId: annotation.id, initialText: annotation.text ?? '', fontSize: annotation.fontSize, color: annotation.color);
  }

  void _handlePanStart(Offset localPosition) {
    if (controller.textDraftPosition.value != null) {
      controller.commitTextDraft();
    }

    final selectionRect = controller.selectionRect;
    final globalPosition = _toGlobalPosition(localPosition);
    _dragStartGlobal = globalPosition;

    // Annotation editing now depends on an explicit tap-based selection instead of whichever tool
    // drew the mark. Once something is selected, keep its handles interactive even if the toolbar
    // still shows a creation tool so follow-up edits do not require a separate mode switch.
    final selectedHandleHit = _hitTestSelectedAnnotationHandle(globalPosition);
    if (selectedHandleHit != null) {
      _dragAnnotationId = selectedHandleHit.annotation.id;
      _annotationAtDragStart = selectedHandleHit.annotation;
      _annotationHandle = selectedHandleHit.handle;
      _interactionMode =
          selectedHandleHit.handle == _AnnotationHandle.arrowStart
              ? _InteractionMode.moveArrowStart
              : selectedHandleHit.handle == _AnnotationHandle.arrowEnd
              ? _InteractionMode.moveArrowEnd
              : _InteractionMode.resizeShapeAnnotation;
      return;
    }

    final selectedAnnotation = controller.selectedAnnotation;
    if (selectedAnnotation != null && _annotationContainsPoint(selectedAnnotation, globalPosition)) {
      _dragAnnotationId = selectedAnnotation.id;
      _annotationAtDragStart = selectedAnnotation;
      _interactionMode = selectedAnnotation.type == ScreenshotAnnotationType.text ? _InteractionMode.moveText : _InteractionMode.moveAnnotation;
      return;
    }

    switch (controller.currentTool.value) {
      case ScreenshotTool.select:
        final annotationBodyHit = _hitTestAnnotationBody(globalPosition);
        if (annotationBodyHit != null) {
          controller.selectAnnotation(annotationBodyHit.id);
          _dragAnnotationId = annotationBodyHit.id;
          _annotationAtDragStart = annotationBodyHit;
          _interactionMode = annotationBodyHit.type == ScreenshotAnnotationType.text ? _InteractionMode.moveText : _InteractionMode.moveAnnotation;
          return;
        }

        controller.selectAnnotation(null);
        if (selectionRect != null) {
          final handle = _hitTestSelectionHandle(selectionRect, globalPosition);
          if (handle != null) {
            _interactionMode = _InteractionMode.resizeSelection;
            _resizeHandle = handle;
            _selectionAtDragStart = selectionRect;
            return;
          }
          if (selectionRect.contains(globalPosition)) {
            _interactionMode = _InteractionMode.moveSelection;
            _selectionAtDragStart = selectionRect;
            return;
          }
        }

        _interactionMode = _InteractionMode.createSelection;
        controller.updateSelection(Rect.fromPoints(globalPosition, globalPosition));
        break;
      case ScreenshotTool.rect:
      case ScreenshotTool.ellipse:
      case ScreenshotTool.arrow:
        controller.selectAnnotation(null);
        if (selectionRect == null || !selectionRect.contains(globalPosition)) {
          return;
        }
        _interactionMode = _InteractionMode.createAnnotation;
        _annotationStart = globalPosition;
        _annotationEnd = globalPosition;
        _annotationDraftRect = Rect.fromPoints(globalPosition, globalPosition);
        break;
      case ScreenshotTool.text:
        break;
    }
  }

  void _handlePanUpdate(Offset localPosition) {
    final interactionMode = _interactionMode;
    final dragStart = _dragStartGlobal;
    if (interactionMode == null || dragStart == null) {
      return;
    }

    final globalPosition = _toGlobalPosition(localPosition);
    switch (interactionMode) {
      case _InteractionMode.createSelection:
        controller.updateSelection(Rect.fromPoints(dragStart, globalPosition));
        break;
      case _InteractionMode.moveSelection:
        final original = _selectionAtDragStart;
        if (original == null) {
          break;
        }
        controller.updateSelection(_shiftRectWithinBounds(original, globalPosition - dragStart, controller.virtualBoundsRect));
        break;
      case _InteractionMode.resizeSelection:
        final original = _selectionAtDragStart;
        final handle = _resizeHandle;
        if (original == null || handle == null) {
          break;
        }
        controller.updateSelection(_resizeRect(original, handle, globalPosition));
        break;
      case _InteractionMode.createAnnotation:
        final currentSelection = controller.selectionRect;
        if (currentSelection == null) {
          break;
        }
        final clamped = _clampOffsetToRect(globalPosition, currentSelection);
        _annotationEnd = clamped;
        _annotationDraftRect = Rect.fromPoints(dragStart, clamped);
        setState(() {});
        break;
      case _InteractionMode.moveAnnotation:
        _updateDraggedAnnotation(globalPosition, dragStart);
        break;
      case _InteractionMode.resizeShapeAnnotation:
        _resizeSelectedShape(globalPosition);
        break;
      case _InteractionMode.moveArrowStart:
        _moveArrowEndpoint(globalPosition, updateStart: true);
        break;
      case _InteractionMode.moveArrowEnd:
        _moveArrowEndpoint(globalPosition, updateStart: false);
        break;
      case _InteractionMode.moveText:
        _updateDraggedAnnotation(globalPosition, dragStart);
        break;
    }
  }

  void _handlePanEnd() {
    final interactionMode = _interactionMode;
    final needsOverlayRefresh = interactionMode == _InteractionMode.createAnnotation;

    if (interactionMode == _InteractionMode.createAnnotation && _annotationStart != null && _annotationEnd != null) {
      // Freshly drawn annotations now stay unselected by default. Auto-selecting every new mark
      // made the next pointer gesture look like an unwanted edit, so selection is left to explicit
      // taps on existing annotations and blank taps already clear the current selection.
      switch (controller.currentTool.value) {
        case ScreenshotTool.rect:
          controller.addShapeAnnotation(ScreenshotAnnotationType.rect, _annotationDraftRect!);
          break;
        case ScreenshotTool.ellipse:
          controller.addShapeAnnotation(ScreenshotAnnotationType.ellipse, _annotationDraftRect!);
          break;
        case ScreenshotTool.arrow:
          controller.addArrowAnnotation(_annotationStart!, _annotationEnd!);
          break;
        case ScreenshotTool.select:
        case ScreenshotTool.text:
          break;
      }
    }

    _interactionMode = null;
    _resizeHandle = null;
    _annotationHandle = null;
    _dragStartGlobal = null;
    _selectionAtDragStart = null;
    _annotationDraftRect = null;
    _annotationStart = null;
    _annotationEnd = null;
    _dragAnnotationId = null;
    _annotationAtDragStart = null;
    if (needsOverlayRefresh) {
      setState(() {});
    }
  }

  void _handleHover(Offset localPosition) {
    if (_interactionMode != null || controller.currentTool.value != ScreenshotTool.select) {
      _setHoverCursor(SystemMouseCursors.basic);
      return;
    }

    final globalPosition = _toGlobalPosition(localPosition);
    final selectedHandleHit = _hitTestSelectedAnnotationHandle(globalPosition);
    if (selectedHandleHit != null) {
      _setHoverCursor(selectedHandleHit.cursor);
      return;
    }

    final annotationBodyHit = _hitTestAnnotationBody(globalPosition);
    if (annotationBodyHit != null) {
      if (annotationBodyHit.type == ScreenshotAnnotationType.text) {
        _setHoverCursor(SystemMouseCursors.text);
        return;
      }
      _setHoverCursor(SystemMouseCursors.move);
      return;
    }

    final selectionRect = controller.selectionRect;
    if (selectionRect != null) {
      final handle = _hitTestSelectionHandle(selectionRect, globalPosition);
      if (handle != null) {
        _setHoverCursor(_cursorForResizeHandle(handle));
        return;
      }
      if (selectionRect.contains(globalPosition)) {
        _setHoverCursor(SystemMouseCursors.move);
        return;
      }
    }

    _setHoverCursor(SystemMouseCursors.basic);
  }

  void _setHoverCursor(MouseCursor cursor) {
    if (_hoverCursor == cursor) {
      return;
    }
    setState(() {
      _hoverCursor = cursor;
    });
  }

  void _updateDraggedAnnotation(Offset globalPosition, Offset dragStart) {
    final annotation = _annotationAtDragStart;
    final annotationId = _dragAnnotationId;
    final currentSelection = controller.selectionRect;
    if (annotation == null || annotationId == null || currentSelection == null) {
      return;
    }

    final delta = globalPosition - dragStart;
    switch (annotation.type) {
      case ScreenshotAnnotationType.rect:
      case ScreenshotAnnotationType.ellipse:
        final rect = annotation.rect;
        if (rect == null) {
          return;
        }
        controller.updateAnnotationRect(annotationId, _shiftRectWithinBounds(rect, delta, currentSelection));
        break;
      case ScreenshotAnnotationType.arrow:
        final start = annotation.start;
        final end = annotation.end;
        if (start == null || end == null) {
          return;
        }
        final originalBounds = Rect.fromPoints(start, end).inflate(1);
        final shiftedBounds = _shiftRectWithinBounds(originalBounds, delta, currentSelection);
        final clampedDelta = shiftedBounds.topLeft - originalBounds.topLeft;
        controller.updateArrowPoints(annotationId, start: start + clampedDelta, end: end + clampedDelta);
        break;
      case ScreenshotAnnotationType.text:
        final start = annotation.start;
        final textBounds = screenshotAnnotationBounds(annotation);
        if (start == null || textBounds == null) {
          return;
        }
        final shiftedBounds = _shiftRectWithinBounds(textBounds, delta, currentSelection);
        controller.updateTextPosition(annotationId, start + (shiftedBounds.topLeft - textBounds.topLeft));
        break;
    }
  }

  void _resizeSelectedShape(Offset globalPosition) {
    final annotation = _annotationAtDragStart;
    final annotationId = _dragAnnotationId;
    final handle = _annotationHandle;
    final currentSelection = controller.selectionRect;
    if (annotation == null || annotation.rect == null || annotationId == null || handle == null || currentSelection == null) {
      return;
    }

    controller.updateAnnotationRect(annotationId, _resizeShapeRect(annotation.rect!, handle, _clampOffsetToRect(globalPosition, currentSelection), currentSelection));
  }

  void _moveArrowEndpoint(Offset globalPosition, {required bool updateStart}) {
    final annotation = _annotationAtDragStart;
    final annotationId = _dragAnnotationId;
    final currentSelection = controller.selectionRect;
    if (annotation == null || annotationId == null || currentSelection == null) {
      return;
    }

    final nextPoint = _clampOffsetToRect(globalPosition, currentSelection);
    controller.updateArrowPoints(annotationId, start: updateStart ? nextPoint : annotation.start, end: updateStart ? annotation.end : nextPoint);
  }

  Offset _toGlobalPosition(Offset localPosition) {
    return localPosition + controller.virtualBoundsRect.topLeft;
  }

  _ResizeHandle? _hitTestSelectionHandle(Rect rect, Offset point) {
    for (final handle in _ResizeHandle.values) {
      final handleOffset = _handleOffsetForRect(rect, handle);
      if ((handleOffset - point).distance <= 12) {
        return handle;
      }
    }

    if (point.dx >= rect.left && point.dx <= rect.right) {
      if ((point.dy - rect.top).abs() <= _selectionEdgeTolerance) {
        return _ResizeHandle.top;
      }
      if ((point.dy - rect.bottom).abs() <= _selectionEdgeTolerance) {
        return _ResizeHandle.bottom;
      }
    }

    if (point.dy >= rect.top && point.dy <= rect.bottom) {
      if ((point.dx - rect.left).abs() <= _selectionEdgeTolerance) {
        return _ResizeHandle.left;
      }
      if ((point.dx - rect.right).abs() <= _selectionEdgeTolerance) {
        return _ResizeHandle.right;
      }
    }

    return null;
  }

  _AnnotationHitTarget? _hitTestSelectedAnnotationHandle(Offset point) {
    final annotation = controller.selectedAnnotation;
    if (annotation == null) {
      return null;
    }

    switch (annotation.type) {
      case ScreenshotAnnotationType.rect:
      case ScreenshotAnnotationType.ellipse:
        final rect = annotation.rect;
        if (rect == null) {
          return null;
        }
        for (final handle in _shapeAnnotationHandles) {
          if ((_handleOffsetForAnnotationRect(rect, handle) - point).distance <= 12) {
            return _AnnotationHitTarget(annotation: annotation, handle: handle, cursor: _cursorForAnnotationHandle(handle));
          }
        }
        return null;
      case ScreenshotAnnotationType.arrow:
        final start = annotation.start;
        final end = annotation.end;
        if (start == null || end == null) {
          return null;
        }
        if ((start - point).distance <= 12) {
          return _AnnotationHitTarget(annotation: annotation, handle: _AnnotationHandle.arrowStart, cursor: SystemMouseCursors.precise);
        }
        if ((end - point).distance <= 12) {
          return _AnnotationHitTarget(annotation: annotation, handle: _AnnotationHandle.arrowEnd, cursor: SystemMouseCursors.precise);
        }
        return null;
      case ScreenshotAnnotationType.text:
        return null;
    }
  }

  ScreenshotAnnotation? _hitTestAnnotationBody(Offset point, {bool allowOnlyText = false}) {
    for (final annotation in controller.annotations.reversed) {
      if (allowOnlyText && annotation.type != ScreenshotAnnotationType.text) {
        continue;
      }
      if (_annotationContainsPoint(annotation, point)) {
        return annotation;
      }
    }
    return null;
  }

  bool _annotationContainsPoint(ScreenshotAnnotation annotation, Offset point) {
    switch (annotation.type) {
      case ScreenshotAnnotationType.rect:
        final rect = annotation.rect;
        return rect != null && rect.inflate(8).contains(point);
      case ScreenshotAnnotationType.ellipse:
        final rect = annotation.rect;
        if (rect == null) {
          return false;
        }
        final inflated = rect.inflate(8);
        final radiusX = inflated.width / 2;
        final radiusY = inflated.height / 2;
        final center = inflated.center;
        final dx = (point.dx - center.dx) / radiusX;
        final dy = (point.dy - center.dy) / radiusY;
        return dx * dx + dy * dy <= 1;
      case ScreenshotAnnotationType.arrow:
        final start = annotation.start;
        final end = annotation.end;
        if (start == null || end == null) {
          return false;
        }
        return _distanceToSegment(point, start, end) <= 10;
      case ScreenshotAnnotationType.text:
        final bounds = screenshotAnnotationBounds(annotation);
        return bounds != null && bounds.inflate(8).contains(point);
    }
  }

  Offset _handleOffsetForRect(Rect rect, _ResizeHandle handle) {
    switch (handle) {
      case _ResizeHandle.topLeft:
        return rect.topLeft;
      case _ResizeHandle.top:
        return Offset(rect.center.dx, rect.top);
      case _ResizeHandle.topRight:
        return rect.topRight;
      case _ResizeHandle.right:
        return Offset(rect.right, rect.center.dy);
      case _ResizeHandle.bottomRight:
        return rect.bottomRight;
      case _ResizeHandle.bottom:
        return Offset(rect.center.dx, rect.bottom);
      case _ResizeHandle.bottomLeft:
        return rect.bottomLeft;
      case _ResizeHandle.left:
        return Offset(rect.left, rect.center.dy);
    }
  }

  Offset _handleOffsetForAnnotationRect(Rect rect, _AnnotationHandle handle) {
    switch (handle) {
      case _AnnotationHandle.topLeft:
        return rect.topLeft;
      case _AnnotationHandle.top:
        return Offset(rect.center.dx, rect.top);
      case _AnnotationHandle.topRight:
        return rect.topRight;
      case _AnnotationHandle.right:
        return Offset(rect.right, rect.center.dy);
      case _AnnotationHandle.bottomRight:
        return rect.bottomRight;
      case _AnnotationHandle.bottom:
        return Offset(rect.center.dx, rect.bottom);
      case _AnnotationHandle.bottomLeft:
        return rect.bottomLeft;
      case _AnnotationHandle.left:
        return Offset(rect.left, rect.center.dy);
      case _AnnotationHandle.arrowStart:
      case _AnnotationHandle.arrowEnd:
        return rect.center;
    }
  }

  Rect _resizeRect(Rect original, _ResizeHandle handle, Offset point) {
    final rect = _rectForResizeHandle(original, handle, point);
    return rect;
  }

  Rect _resizeShapeRect(Rect original, _AnnotationHandle handle, Offset point, Rect bounds) {
    final rect = _rectForAnnotationHandle(original, handle, point);
    return _clampRectToBounds(rect, bounds);
  }

  Rect _rectForResizeHandle(Rect original, _ResizeHandle handle, Offset point) {
    switch (handle) {
      case _ResizeHandle.topLeft:
        return Rect.fromPoints(point, original.bottomRight);
      case _ResizeHandle.top:
        return _normalizedRectFromLTRB(original.left, point.dy, original.right, original.bottom);
      case _ResizeHandle.topRight:
        return _normalizedRectFromLTRB(original.left, point.dy, point.dx, original.bottom);
      case _ResizeHandle.right:
        return _normalizedRectFromLTRB(original.left, original.top, point.dx, original.bottom);
      case _ResizeHandle.bottomRight:
        return Rect.fromPoints(original.topLeft, point);
      case _ResizeHandle.bottom:
        return _normalizedRectFromLTRB(original.left, original.top, original.right, point.dy);
      case _ResizeHandle.bottomLeft:
        return _normalizedRectFromLTRB(point.dx, original.top, original.right, point.dy);
      case _ResizeHandle.left:
        return _normalizedRectFromLTRB(point.dx, original.top, original.right, original.bottom);
    }
  }

  Rect _rectForAnnotationHandle(Rect original, _AnnotationHandle handle, Offset point) {
    switch (handle) {
      case _AnnotationHandle.topLeft:
        return Rect.fromPoints(point, original.bottomRight);
      case _AnnotationHandle.top:
        return _normalizedRectFromLTRB(original.left, point.dy, original.right, original.bottom);
      case _AnnotationHandle.topRight:
        return _normalizedRectFromLTRB(original.left, point.dy, point.dx, original.bottom);
      case _AnnotationHandle.right:
        return _normalizedRectFromLTRB(original.left, original.top, point.dx, original.bottom);
      case _AnnotationHandle.bottomRight:
        return Rect.fromPoints(original.topLeft, point);
      case _AnnotationHandle.bottom:
        return _normalizedRectFromLTRB(original.left, original.top, original.right, point.dy);
      case _AnnotationHandle.bottomLeft:
        return _normalizedRectFromLTRB(point.dx, original.top, original.right, point.dy);
      case _AnnotationHandle.left:
        return _normalizedRectFromLTRB(point.dx, original.top, original.right, original.bottom);
      case _AnnotationHandle.arrowStart:
      case _AnnotationHandle.arrowEnd:
        return original;
    }
  }

  Rect _shiftRectWithinBounds(Rect rect, Offset delta, Rect bounds) {
    var shifted = rect.shift(delta);
    if (shifted.left < bounds.left) {
      shifted = shifted.shift(Offset(bounds.left - shifted.left, 0));
    }
    if (shifted.top < bounds.top) {
      shifted = shifted.shift(Offset(0, bounds.top - shifted.top));
    }
    if (shifted.right > bounds.right) {
      shifted = shifted.shift(Offset(bounds.right - shifted.right, 0));
    }
    if (shifted.bottom > bounds.bottom) {
      shifted = shifted.shift(Offset(0, bounds.bottom - shifted.bottom));
    }
    return shifted;
  }

  Offset _clampOffsetToRect(Offset point, Rect bounds) {
    return Offset(point.dx.clamp(bounds.left, bounds.right).toDouble(), point.dy.clamp(bounds.top, bounds.bottom).toDouble());
  }

  Rect _clampRectToBounds(Rect rect, Rect bounds) {
    final normalized = _normalizedRectFromLTRB(rect.left, rect.top, rect.right, rect.bottom);
    final left = normalized.left.clamp(bounds.left, bounds.right).toDouble();
    final top = normalized.top.clamp(bounds.top, bounds.bottom).toDouble();
    final right = normalized.right.clamp(bounds.left, bounds.right).toDouble();
    final bottom = normalized.bottom.clamp(bounds.top, bounds.bottom).toDouble();
    return _normalizedRectFromLTRB(left, top, right, bottom);
  }

  Rect _normalizedRectFromLTRB(double left, double top, double right, double bottom) {
    return Rect.fromLTRB(math.min(left, right), math.min(top, bottom), math.max(left, right), math.max(top, bottom));
  }

  MouseCursor _cursorForResizeHandle(_ResizeHandle handle) {
    switch (handle) {
      case _ResizeHandle.topLeft:
      case _ResizeHandle.bottomRight:
        return SystemMouseCursors.resizeUpLeftDownRight;
      case _ResizeHandle.topRight:
      case _ResizeHandle.bottomLeft:
        return SystemMouseCursors.resizeUpRightDownLeft;
      case _ResizeHandle.top:
      case _ResizeHandle.bottom:
        return SystemMouseCursors.resizeUpDown;
      case _ResizeHandle.left:
      case _ResizeHandle.right:
        return SystemMouseCursors.resizeLeftRight;
    }
  }

  MouseCursor _cursorForAnnotationHandle(_AnnotationHandle handle) {
    switch (handle) {
      case _AnnotationHandle.topLeft:
      case _AnnotationHandle.bottomRight:
        return SystemMouseCursors.resizeUpLeftDownRight;
      case _AnnotationHandle.topRight:
      case _AnnotationHandle.bottomLeft:
        return SystemMouseCursors.resizeUpRightDownLeft;
      case _AnnotationHandle.top:
      case _AnnotationHandle.bottom:
        return SystemMouseCursors.resizeUpDown;
      case _AnnotationHandle.left:
      case _AnnotationHandle.right:
        return SystemMouseCursors.resizeLeftRight;
      case _AnnotationHandle.arrowStart:
      case _AnnotationHandle.arrowEnd:
        return SystemMouseCursors.precise;
    }
  }

  double _distanceToSegment(Offset point, Offset start, Offset end) {
    final dx = end.dx - start.dx;
    final dy = end.dy - start.dy;
    if (dx == 0 && dy == 0) {
      return (point - start).distance;
    }

    final projection = ((point.dx - start.dx) * dx + (point.dy - start.dy) * dy) / (dx * dx + dy * dy);
    final clampedProjection = projection.clamp(0.0, 1.0).toDouble();
    final closest = Offset(start.dx + dx * clampedProjection, start.dy + dy * clampedProjection);
    return (point - closest).distance;
  }

  ScreenshotAnnotationType? _currentDraftType() {
    switch (controller.currentTool.value) {
      case ScreenshotTool.rect:
        return ScreenshotAnnotationType.rect;
      case ScreenshotTool.ellipse:
        return ScreenshotAnnotationType.ellipse;
      case ScreenshotTool.arrow:
        return ScreenshotAnnotationType.arrow;
      case ScreenshotTool.select:
      case ScreenshotTool.text:
        return null;
    }
  }
}

class _WorkspaceBackground extends StatelessWidget {
  const _WorkspaceBackground({required this.snapshots, required this.virtualBounds});

  final List<DisplaySnapshot> snapshots;
  final Rect virtualBounds;

  @override
  Widget build(BuildContext context) {
    return Stack(
      children: [
        for (final snapshot in snapshots)
          Positioned.fromRect(
            rect: snapshot.logicalBounds.toRect().shift(-virtualBounds.topLeft),
            child: Image(image: snapshot.imageProvider, fit: BoxFit.fill, gaplessPlayback: true),
          ),
      ],
    );
  }
}

class _SelectionToolbarLayoutDelegate extends SingleChildLayoutDelegate {
  const _SelectionToolbarLayoutDelegate({required this.selectionRect});

  final Rect? selectionRect;
  static const EdgeInsets _viewportPadding = EdgeInsets.all(24);
  static const double _selectionGap = 16;

  @override
  BoxConstraints getConstraintsForChild(BoxConstraints constraints) {
    return BoxConstraints(
      maxWidth: (constraints.maxWidth - _viewportPadding.horizontal).clamp(0, double.infinity),
      maxHeight: (constraints.maxHeight - _viewportPadding.vertical).clamp(0, double.infinity),
    );
  }

  @override
  Offset getPositionForChild(Size size, Size childSize) {
    if (selectionRect == null) {
      return Offset(
        ((size.width - childSize.width) / 2).clamp(_viewportPadding.left, size.width - childSize.width - _viewportPadding.right),
        (size.height - childSize.height - _viewportPadding.bottom).clamp(_viewportPadding.top, size.height - childSize.height - _viewportPadding.bottom),
      );
    }

    final rightAlignedLeft = selectionRect!.right - childSize.width;
    final left = rightAlignedLeft.clamp(_viewportPadding.left, size.width - childSize.width - _viewportPadding.right);

    final preferredBelowTop = selectionRect!.bottom + _selectionGap;
    final belowFits = preferredBelowTop + childSize.height <= size.height - _viewportPadding.bottom;
    final top =
        belowFits
            ? preferredBelowTop
            : (selectionRect!.top - childSize.height - _selectionGap).clamp(_viewportPadding.top, size.height - childSize.height - _viewportPadding.bottom);

    return Offset(left, top);
  }

  @override
  bool shouldRelayout(covariant _SelectionToolbarLayoutDelegate oldDelegate) {
    return oldDelegate.selectionRect != selectionRect;
  }
}

class _SelectionEditBarLayoutDelegate extends SingleChildLayoutDelegate {
  const _SelectionEditBarLayoutDelegate({required this.selectionRect, required this.anchorRect});

  final Rect selectionRect;
  final Rect? anchorRect;
  static const EdgeInsets _viewportPadding = EdgeInsets.all(24);
  static const double _selectionGap = 16;

  @override
  BoxConstraints getConstraintsForChild(BoxConstraints constraints) {
    return BoxConstraints(
      maxWidth: (constraints.maxWidth - _viewportPadding.horizontal).clamp(0, double.infinity),
      maxHeight: (constraints.maxHeight - _viewportPadding.vertical).clamp(0, double.infinity),
    );
  }

  @override
  Offset getPositionForChild(Size size, Size childSize) {
    final hasRightSpace = selectionRect.right + _selectionGap + childSize.width <= size.width - _viewportPadding.right;
    final left =
        hasRightSpace
            ? selectionRect.right + _selectionGap
            : (selectionRect.left - childSize.width - _selectionGap).clamp(_viewportPadding.left, size.width - childSize.width - _viewportPadding.right);

    final targetCenterY = (anchorRect ?? selectionRect).center.dy;
    final top = (targetCenterY - childSize.height / 2).clamp(_viewportPadding.top, size.height - childSize.height - _viewportPadding.bottom);
    return Offset(left, top);
  }

  @override
  bool shouldRelayout(covariant _SelectionEditBarLayoutDelegate oldDelegate) {
    return oldDelegate.selectionRect != selectionRect || oldDelegate.anchorRect != anchorRect;
  }
}

class _LoadingView extends StatelessWidget {
  const _LoadingView({required this.label});

  final String label;

  @override
  Widget build(BuildContext context) {
    return ColoredBox(
      color: const Color(0xFF090909),
      child: Center(
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            const SizedBox(width: 28, height: 28, child: CircularProgressIndicator(strokeWidth: 2.4, color: Color(0xFF29FF72))),
            const SizedBox(height: 14),
            Text(label, style: const TextStyle(color: Colors.white, fontSize: 16, fontWeight: FontWeight.w600)),
          ],
        ),
      ),
    );
  }
}

class _ToolButton extends StatelessWidget {
  const _ToolButton({
    super.key,
    required this.icon,
    required this.onPressed,
    this.selected = false,
    this.enabled = true,
    this.color = Colors.white,
    this.activateOnTapDown = false,
  });

  final IconData icon;
  final VoidCallback? onPressed;
  final bool selected;
  final bool enabled;
  final Color color;
  final bool activateOnTapDown;

  @override
  Widget build(BuildContext context) {
    final foreground = enabled ? (selected ? const Color(0xFF29FF72) : color) : Colors.white38;
    final enabledAction = enabled ? onPressed : null;

    return Padding(
      padding: const EdgeInsets.symmetric(horizontal: 4),
      child: InkWell(
        // Tool switching on desktop felt delayed because InkWell.onTap waits for pointer-up and also
        // competes with the toolbar scroll view's gesture arena. Triggering the low-risk tool-change
        // actions on tap-down makes the active icon respond immediately while leaving confirm/cancel
        // style actions on the safer pointer-up path.
        onTapDown: activateOnTapDown && enabledAction != null ? (_) => enabledAction() : null,
        onTap: activateOnTapDown ? null : enabledAction,
        borderRadius: BorderRadius.circular(10),
        child: Container(
          width: 40,
          height: 40,
          decoration: BoxDecoration(color: selected ? const Color(0x3329FF72) : Colors.transparent, borderRadius: BorderRadius.circular(10)),
          child: Icon(icon, color: foreground, size: 24),
        ),
      ),
    );
  }
}

class _ColorSwatchButton extends StatelessWidget {
  const _ColorSwatchButton({required this.color, required this.selected, required this.onPressed, required this.compact});

  final Color color;
  final bool selected;
  final VoidCallback onPressed;
  final bool compact;

  @override
  Widget build(BuildContext context) {
    final size = compact ? 16.0 : 20.0;
    return GestureDetector(
      onTap: onPressed,
      child: Container(
        width: size,
        height: size,
        decoration: BoxDecoration(color: color, shape: BoxShape.circle, border: Border.all(color: selected ? const Color(0xFF29FF72) : Colors.white24, width: selected ? 2 : 1)),
      ),
    );
  }
}

class _EditActionButton extends StatelessWidget {
  const _EditActionButton({required this.icon, required this.onPressed, this.color = Colors.white});

  final IconData icon;
  final VoidCallback onPressed;
  final Color color;

  @override
  Widget build(BuildContext context) {
    return InkWell(
      onTap: onPressed,
      borderRadius: BorderRadius.circular(10),
      child: Container(
        width: 42,
        height: 42,
        decoration: BoxDecoration(color: const Color(0x22FFFFFF), borderRadius: BorderRadius.circular(10)),
        child: Icon(icon, color: color, size: 22),
      ),
    );
  }
}

class _WorkspaceShadePainter extends CustomPainter {
  _WorkspaceShadePainter({required this.selectionRect, required this.selectionSizeLabel});

  final Rect? selectionRect;
  final String? selectionSizeLabel;

  @override
  void paint(Canvas canvas, Size size) {
    final overlayPaint = Paint()..color = const Color(0x77000000);
    if (selectionRect != null) {
      final clampedSelection = selectionRect!.intersect(Offset.zero & size);
      // The even-odd path version was logically correct, but anti-aliased path filling plus the
      // selection-frame shadow still made the crop interior look slightly tinted. Painting the four
      // outside bands directly guarantees the selected pixels remain fully untouched.
      if (clampedSelection.top > 0) {
        canvas.drawRect(Rect.fromLTWH(0, 0, size.width, clampedSelection.top), overlayPaint);
      }
      if (clampedSelection.bottom < size.height) {
        canvas.drawRect(Rect.fromLTWH(0, clampedSelection.bottom, size.width, size.height - clampedSelection.bottom), overlayPaint);
      }
      if (clampedSelection.left > 0) {
        canvas.drawRect(Rect.fromLTWH(0, clampedSelection.top, clampedSelection.left, clampedSelection.height), overlayPaint);
      }
      if (clampedSelection.right < size.width) {
        canvas.drawRect(Rect.fromLTWH(clampedSelection.right, clampedSelection.top, size.width - clampedSelection.right, clampedSelection.height), overlayPaint);
      }
    } else {
      canvas.drawRect(Offset.zero & size, overlayPaint);
    }

    if (selectionRect != null && selectionSizeLabel != null) {
      final backgroundPaint = Paint()..color = const Color(0xE6171717);
      final labelPainter = TextPainter(
        text: TextSpan(text: selectionSizeLabel, style: const TextStyle(color: Colors.white, fontSize: 14, fontWeight: FontWeight.w700)),
        textDirection: TextDirection.ltr,
      )..layout();

      final labelOffset = Offset(selectionRect!.left + 12, selectionRect!.top - 28);
      final labelRect = RRect.fromRectAndRadius(Rect.fromLTWH(labelOffset.dx - 8, labelOffset.dy - 4, labelPainter.width + 16, labelPainter.height + 8), const Radius.circular(10));
      canvas.drawRRect(labelRect, backgroundPaint);
      labelPainter.paint(canvas, labelOffset);
    }
  }

  @override
  bool shouldRepaint(covariant _WorkspaceShadePainter oldDelegate) {
    return oldDelegate.selectionRect != selectionRect || oldDelegate.selectionSizeLabel != selectionSizeLabel;
  }
}

class _AnnotationPainter extends CustomPainter {
  _AnnotationPainter({
    required this.annotations,
    required this.canvasOrigin,
    required this.selectionClipRect,
    required this.draftRect,
    required this.draftStart,
    required this.draftEnd,
    required this.draftType,
    required this.previewColor,
    required this.selectedAnnotationId,
    required this.editingTextAnnotationId,
  });

  final List<ScreenshotAnnotation> annotations;
  final Offset canvasOrigin;
  final Rect? selectionClipRect;
  final Rect? draftRect;
  final Offset? draftStart;
  final Offset? draftEnd;
  final ScreenshotAnnotationType? draftType;
  final Color previewColor;
  final String? selectedAnnotationId;
  final String? editingTextAnnotationId;

  @override
  void paint(Canvas canvas, Size size) {
    final visibleAnnotations = editingTextAnnotationId == null ? annotations : annotations.where((annotation) => annotation.id != editingTextAnnotationId).toList(growable: false);

    // Inline text editing paints the caret field directly above the annotation. Hiding the source
    // text while that editor is active prevents the stale rendered label from peeking through and
    // keeps editing visually identical to the non-editing state apart from the caret itself.
    paintWorkspaceAnnotations(canvas, annotations: visibleAnnotations, canvasOrigin: canvasOrigin, selectionClipRect: selectionClipRect);
    if (draftType != null) {
      final previewAnnotations = <ScreenshotAnnotation>[];
      if (draftType == ScreenshotAnnotationType.arrow && draftStart != null && draftEnd != null) {
        previewAnnotations.add(ScreenshotAnnotation(id: 'draft-arrow', type: ScreenshotAnnotationType.arrow, start: draftStart, end: draftEnd, color: previewColor));
      } else if (draftRect != null) {
        previewAnnotations.add(ScreenshotAnnotation(id: 'draft-shape', type: draftType!, rect: draftRect, color: previewColor));
      }
      paintWorkspaceAnnotations(canvas, annotations: previewAnnotations, canvasOrigin: canvasOrigin, selectionClipRect: selectionClipRect);
    }

    ScreenshotAnnotation? selectedAnnotation;
    if (selectedAnnotationId != null) {
      for (final annotation in annotations) {
        if (annotation.id == selectedAnnotationId) {
          selectedAnnotation = annotation;
          break;
        }
      }
    }
    if (selectedAnnotation != null) {
      _paintSelectedAnnotationControls(canvas, selectedAnnotation, canvasOrigin);
    }
  }

  void _paintSelectedAnnotationControls(Canvas canvas, ScreenshotAnnotation annotation, Offset origin) {
    final handleFill = Paint()..color = Colors.white;
    final handleStroke =
        Paint()
          ..color = const Color(0xCC111111)
          ..strokeWidth = 1
          ..style = PaintingStyle.stroke;

    switch (annotation.type) {
      case ScreenshotAnnotationType.rect:
      case ScreenshotAnnotationType.ellipse:
        final rect = annotation.rect?.shift(-origin);
        if (rect == null) {
          return;
        }
        // The previous selected-state outline wrapped the whole annotation in a bright white frame.
        // That made edits feel noisy and partially obscured the mark itself even though the drag
        // handles already communicate editability. Keep only the handles so selection stays clear
        // without repainting an extra border over the user's annotation.
        for (final handle in _shapeAnnotationHandles) {
          final handleCenter = _annotationHandleOffsetForSelection(rect, handle);
          final handleRect = Rect.fromCenter(center: handleCenter, width: _annotationHandleSize, height: _annotationHandleSize);
          canvas.drawRRect(RRect.fromRectAndRadius(handleRect, const Radius.circular(4)), handleFill);
          canvas.drawRRect(RRect.fromRectAndRadius(handleRect, const Radius.circular(4)), handleStroke);
        }
        break;
      case ScreenshotAnnotationType.arrow:
        final start = annotation.start;
        final end = annotation.end;
        if (start == null || end == null) {
          return;
        }
        for (final handleCenter in <Offset>[start - origin, end - origin]) {
          canvas.drawCircle(handleCenter, _annotationHandleSize / 2, handleFill);
          canvas.drawCircle(handleCenter, _annotationHandleSize / 2, handleStroke);
        }
        break;
      case ScreenshotAnnotationType.text:
        break;
    }
  }

  Offset _annotationHandleOffsetForSelection(Rect rect, _AnnotationHandle handle) {
    switch (handle) {
      case _AnnotationHandle.topLeft:
        return rect.topLeft;
      case _AnnotationHandle.top:
        return Offset(rect.center.dx, rect.top);
      case _AnnotationHandle.topRight:
        return rect.topRight;
      case _AnnotationHandle.right:
        return Offset(rect.right, rect.center.dy);
      case _AnnotationHandle.bottomRight:
        return rect.bottomRight;
      case _AnnotationHandle.bottom:
        return Offset(rect.center.dx, rect.bottom);
      case _AnnotationHandle.bottomLeft:
        return rect.bottomLeft;
      case _AnnotationHandle.left:
        return Offset(rect.left, rect.center.dy);
      case _AnnotationHandle.arrowStart:
      case _AnnotationHandle.arrowEnd:
        return rect.center;
    }
  }

  @override
  bool shouldRepaint(covariant _AnnotationPainter oldDelegate) {
    return oldDelegate.annotations != annotations ||
        oldDelegate.canvasOrigin != canvasOrigin ||
        oldDelegate.selectionClipRect != selectionClipRect ||
        oldDelegate.draftRect != draftRect ||
        oldDelegate.draftStart != draftStart ||
        oldDelegate.draftEnd != draftEnd ||
        oldDelegate.draftType != draftType ||
        oldDelegate.previewColor != previewColor ||
        oldDelegate.selectedAnnotationId != selectedAnnotationId ||
        oldDelegate.editingTextAnnotationId != editingTextAnnotationId;
  }
}

void paintWorkspaceAnnotations(Canvas canvas, {required List<ScreenshotAnnotation> annotations, required Offset canvasOrigin, required Rect? selectionClipRect}) {
  if (selectionClipRect != null) {
    // Annotation tools must stay visually inside the captured selection. Clipping the workspace
    // paint to the selection rect keeps shapes aligned with the user's drag origin and prevents
    // any stroke from leaking outside the active capture region.
    canvas.save();
    canvas.clipRect(selectionClipRect);
  }

  paintScreenshotAnnotations(canvas, annotations, canvasOrigin);

  if (selectionClipRect != null) {
    canvas.restore();
  }
}

const List<_AnnotationHandle> _shapeAnnotationHandles = <_AnnotationHandle>[
  _AnnotationHandle.topLeft,
  _AnnotationHandle.top,
  _AnnotationHandle.topRight,
  _AnnotationHandle.right,
  _AnnotationHandle.bottomRight,
  _AnnotationHandle.bottom,
  _AnnotationHandle.bottomLeft,
  _AnnotationHandle.left,
];

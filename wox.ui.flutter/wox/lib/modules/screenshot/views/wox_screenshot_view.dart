import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:get/get.dart';
import 'package:uuid/v4.dart';
import 'package:wox/controllers/wox_screenshot_controller.dart';
import 'package:wox/entity/screenshot_session.dart';

const Key screenshotCanvasKey = Key('screenshot-canvas');
const Key screenshotToolbarKey = Key('screenshot-toolbar');
const Key screenshotConfirmKey = Key('screenshot-confirm');
const Key screenshotCancelKey = Key('screenshot-cancel');
const Key screenshotUndoKey = Key('screenshot-undo');
const Key screenshotToolSelectKey = Key('screenshot-tool-select');
const Key screenshotToolRectKey = Key('screenshot-tool-rect');
const Key screenshotToolEllipseKey = Key('screenshot-tool-ellipse');
const Key screenshotToolArrowKey = Key('screenshot-tool-arrow');
const Key screenshotToolTextKey = Key('screenshot-tool-text');

class WoxScreenshotView extends StatefulWidget {
  const WoxScreenshotView({super.key});

  @override
  State<WoxScreenshotView> createState() => _WoxScreenshotViewState();
}

enum _InteractionMode { createSelection, moveSelection, resizeSelection, createAnnotation }

enum _ResizeHandle { topLeft, top, topRight, right, bottomRight, bottom, bottomLeft, left }

class _WoxScreenshotViewState extends State<WoxScreenshotView> {
  final controller = Get.find<WoxScreenshotController>();
  final focusNode = FocusNode(debugLabel: 'screenshot-workspace');

  _InteractionMode? _interactionMode;
  _ResizeHandle? _resizeHandle;
  Offset? _dragStartGlobal;
  Rect? _selectionAtDragStart;
  Rect? _annotationDraftRect;
  Offset? _annotationStart;
  Offset? _annotationEnd;

  @override
  void initState() {
    super.initState();
    WidgetsBinding.instance.addPostFrameCallback((_) {
      if (mounted) {
        focusNode.requestFocus();
      }
    });
  }

  @override
  void dispose() {
    focusNode.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    return Obx(() {
      final stage = controller.stage.value;
      final virtualBounds = controller.virtualBoundsRect;

      return KeyboardListener(
        focusNode: focusNode,
        autofocus: true,
        onKeyEvent: (event) {
          if (event is! KeyDownEvent) {
            return;
          }
          if (event.logicalKey == LogicalKeyboardKey.escape) {
            controller.cancelTextDraft();
            controller.cancelSession(const UuidV4().generate());
          }
          if (event.logicalKey == LogicalKeyboardKey.enter && controller.selectionRect != null && controller.textDraftPosition.value == null) {
            controller.confirmSelection(const UuidV4().generate());
          }
          if ((HardwareKeyboard.instance.isControlPressed || HardwareKeyboard.instance.isMetaPressed) && event.logicalKey == LogicalKeyboardKey.keyZ) {
            controller.undoAnnotation();
          }
        },
        child: Material(
          color: Colors.transparent,
          child: stage == ScreenshotSessionStage.loading ? _LoadingView(label: controller.tr('plugin_screenshot_capture_title')) : _buildWorkspace(context, virtualBounds),
        ),
      );
    });
  }

  Widget _buildWorkspace(BuildContext context, Rect virtualBounds) {
    return GestureDetector(
      key: screenshotCanvasKey,
      behavior: HitTestBehavior.translucent,
      onPanStart: (details) => _handlePanStart(details.localPosition),
      onPanUpdate: (details) => _handlePanUpdate(details.localPosition),
      onPanEnd: (_) => _handlePanEnd(),
      onTapDown: (details) => _handleTap(details.localPosition),
      child: Stack(
        children: [
          // Dragging the selection used to rebuild the entire screenshot scene, which caused the
          // captured background to flicker on Windows. Keeping the background in its own repaint
          // boundary makes selection updates repaint only the overlay that actually changes.
          RepaintBoundary(child: _WorkspaceBackground(snapshots: controller.displaySnapshots.toList(), virtualBounds: virtualBounds)),
          Obx(() {
            final selectionRect = controller.selectionRect;
            final selectionLocalRect = selectionRect?.shift(-virtualBounds.topLeft);
            final textDraftPosition = controller.textDraftPosition.value;

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
                    child: CustomPaint(
                      painter: _AnnotationPainter(
                        annotations: controller.annotations.toList(),
                        canvasOrigin: virtualBounds.topLeft,
                        selectionClipRect: selectionLocalRect,
                        draftRect: _annotationDraftRect,
                        draftStart: _annotationStart,
                        draftEnd: _annotationEnd,
                        draftType: _currentDraftType(),
                        previewColor: const Color(0xFFFF5B36),
                      ),
                    ),
                  ),
                if (selectionLocalRect != null) _buildSelectionFrame(selectionLocalRect),
                if (textDraftPosition != null && selectionRect != null) _buildTextDraftField(textDraftPosition - virtualBounds.topLeft),
              ],
            );
          }),
          Obx(() => _buildToolbar(context, controller.selectionRect, virtualBounds)),
        ],
      ),
    );
  }

  Widget _buildToolbar(BuildContext context, Rect? selectionRect, Rect virtualBounds) {
    final currentTool = controller.currentTool.value;
    final canConfirm = selectionRect != null && selectionRect.width >= 1 && selectionRect.height >= 1;

    final selectionLocalRect = selectionRect?.shift(-virtualBounds.topLeft);

    return Positioned.fill(
      child: CustomSingleChildLayout(
        delegate: _SelectionToolbarLayoutDelegate(selectionRect: selectionLocalRect),
        // The toolbar now follows the active selection because the previous fixed screen-bottom
        // placement forced large pointer travel and felt detached from the annotation target.
        // We still keep horizontal scrolling so the full toolset remains usable on narrow layouts.
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
                  onPressed: () => controller.setTool(ScreenshotTool.select),
                ),
                _ToolButton(
                  key: screenshotToolRectKey,
                  icon: Icons.crop_square,
                  selected: currentTool == ScreenshotTool.rect,
                  onPressed: () => controller.setTool(ScreenshotTool.rect),
                ),
                _ToolButton(
                  key: screenshotToolEllipseKey,
                  icon: Icons.circle_outlined,
                  selected: currentTool == ScreenshotTool.ellipse,
                  onPressed: () => controller.setTool(ScreenshotTool.ellipse),
                ),
                _ToolButton(
                  key: screenshotToolTextKey,
                  icon: Icons.text_fields,
                  selected: currentTool == ScreenshotTool.text,
                  onPressed: () => controller.setTool(ScreenshotTool.text),
                ),
                _ToolButton(
                  key: screenshotToolArrowKey,
                  icon: Icons.north_east,
                  selected: currentTool == ScreenshotTool.arrow,
                  onPressed: () => controller.setTool(ScreenshotTool.arrow),
                ),
                const SizedBox(width: 6),
                _ToolButton(key: screenshotUndoKey, icon: Icons.undo, enabled: controller.annotations.isNotEmpty, onPressed: controller.undoAnnotation),
                const SizedBox(width: 6),
                _ToolButton(key: screenshotCancelKey, icon: Icons.close, color: const Color(0xFFFF6B6B), onPressed: () => controller.cancelSession(const UuidV4().generate())),
                _ToolButton(
                  key: screenshotConfirmKey,
                  icon: Icons.check,
                  color: const Color(0xFF30E37A),
                  enabled: canConfirm,
                  onPressed: canConfirm ? () => controller.confirmSelection(const UuidV4().generate()) : null,
                ),
              ],
            ),
          ),
        ),
      ),
    );
  }

  Widget _buildSelectionFrame(Rect selectionLocalRect) {
    const borderColor = Color(0xFF29FF72);
    const handleSize = 12.0;

    final handles = _ResizeHandle.values.map((handle) {
      final position = _handleOffsetForRect(selectionLocalRect, handle);
      return Positioned(
        left: position.dx - handleSize / 2,
        top: position.dy - handleSize / 2,
        child: Container(
          width: handleSize,
          height: handleSize,
          decoration: BoxDecoration(color: borderColor, border: Border.all(color: Colors.black.withValues(alpha: 0.45), width: 1), borderRadius: BorderRadius.circular(4)),
        ),
      );
    });

    return Stack(
      children: [
        Positioned.fromRect(
          rect: selectionLocalRect,
          child: IgnorePointer(
            child: Container(decoration: BoxDecoration(border: Border.all(color: borderColor, width: 2), boxShadow: const [BoxShadow(color: Color(0x33000000), blurRadius: 18)])),
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
      child: Material(
        color: Colors.transparent,
        child: Container(
          width: 240,
          padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 6),
          decoration: BoxDecoration(color: const Color(0xCC161311), borderRadius: BorderRadius.circular(12), border: Border.all(color: const Color(0xFF29FF72), width: 1.5)),
          child: TextField(
            controller: controller.textDraftController,
            autofocus: true,
            style: const TextStyle(color: Colors.white, fontSize: 18, fontWeight: FontWeight.w600),
            decoration: const InputDecoration(isDense: true, border: InputBorder.none, hintText: 'Text', hintStyle: TextStyle(color: Colors.white54)),
            onSubmitted: (_) => controller.commitTextDraft(),
          ),
        ),
      ),
    );
  }

  void _handleTap(Offset localPosition) {
    if (controller.currentTool.value != ScreenshotTool.text || controller.selectionRect == null) {
      return;
    }

    final globalPosition = _toGlobalPosition(localPosition);
    if (!controller.selectionRect!.contains(globalPosition)) {
      return;
    }

    controller.startTextDraft(globalPosition);
  }

  void _handlePanStart(Offset localPosition) {
    controller.cancelTextDraft();
    final selectionRect = controller.selectionRect;
    final globalPosition = _toGlobalPosition(localPosition);
    _dragStartGlobal = globalPosition;

    switch (controller.currentTool.value) {
      case ScreenshotTool.select:
        if (selectionRect != null) {
          final handle = _hitTestHandle(selectionRect, globalPosition);
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
        final selectionRect = controller.selectionRect;
        if (selectionRect == null) {
          break;
        }
        final clamped = Offset(globalPosition.dx.clamp(selectionRect.left, selectionRect.right), globalPosition.dy.clamp(selectionRect.top, selectionRect.bottom));
        _annotationEnd = clamped;
        _annotationDraftRect = Rect.fromPoints(dragStart, clamped);
        setState(() {});
        break;
    }
  }

  void _handlePanEnd() {
    final interactionMode = _interactionMode;
    final needsOverlayRefresh = interactionMode == _InteractionMode.createAnnotation || _annotationDraftRect != null || _annotationStart != null || _annotationEnd != null;

    if (interactionMode == _InteractionMode.createAnnotation && _annotationStart != null && _annotationEnd != null) {
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
    _dragStartGlobal = null;
    _selectionAtDragStart = null;
    _annotationDraftRect = null;
    _annotationStart = null;
    _annotationEnd = null;
    // Dragging the selection itself does not change any local widget state that must repaint after
    // the pointer is released. Avoiding this unconditional setState removes the end-of-drag flash
    // that happened after every selection move/resize on Windows.
    if (needsOverlayRefresh) {
      setState(() {});
    }
  }

  Offset _toGlobalPosition(Offset localPosition) {
    return localPosition + controller.virtualBoundsRect.topLeft;
  }

  _ResizeHandle? _hitTestHandle(Rect rect, Offset point) {
    const radius = 12.0;
    for (final handle in _ResizeHandle.values) {
      if ((_handleOffsetForRect(rect, handle) - point).distance <= radius) {
        return handle;
      }
    }
    return null;
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

  Rect _resizeRect(Rect original, _ResizeHandle handle, Offset point) {
    double left = original.left;
    double top = original.top;
    double right = original.right;
    double bottom = original.bottom;

    switch (handle) {
      case _ResizeHandle.topLeft:
        left = point.dx;
        top = point.dy;
        break;
      case _ResizeHandle.top:
        top = point.dy;
        break;
      case _ResizeHandle.topRight:
        right = point.dx;
        top = point.dy;
        break;
      case _ResizeHandle.right:
        right = point.dx;
        break;
      case _ResizeHandle.bottomRight:
        right = point.dx;
        bottom = point.dy;
        break;
      case _ResizeHandle.bottom:
        bottom = point.dy;
        break;
      case _ResizeHandle.bottomLeft:
        left = point.dx;
        bottom = point.dy;
        break;
      case _ResizeHandle.left:
        left = point.dx;
        break;
    }

    return Rect.fromLTRB(left, top, right, bottom);
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
          Positioned.fromRect(rect: snapshot.logicalBounds.toRect().shift(-virtualBounds.topLeft), child: Image.memory(snapshot.imageBytes, fit: BoxFit.fill)),
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
  const _ToolButton({super.key, required this.icon, required this.onPressed, this.selected = false, this.enabled = true, this.color = Colors.white});

  final IconData icon;
  final VoidCallback? onPressed;
  final bool selected;
  final bool enabled;
  final Color color;

  @override
  Widget build(BuildContext context) {
    final foreground = enabled ? (selected ? const Color(0xFF29FF72) : color) : Colors.white38;

    return Padding(
      padding: const EdgeInsets.symmetric(horizontal: 4),
      child: InkWell(
        onTap: enabled ? onPressed : null,
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

class _WorkspaceShadePainter extends CustomPainter {
  _WorkspaceShadePainter({required this.selectionRect, required this.selectionSizeLabel});

  final Rect? selectionRect;
  final String? selectionSizeLabel;

  @override
  void paint(Canvas canvas, Size size) {
    final overlayPaint = Paint()..color = const Color(0x77000000);
    final path = Path()..addRect(Offset.zero & size);
    if (selectionRect != null) {
      path.addRect(selectionRect!);
      path.fillType = PathFillType.evenOdd;
    }
    canvas.drawPath(path, overlayPaint);

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
  });

  final List<ScreenshotAnnotation> annotations;
  final Offset canvasOrigin;
  final Rect? selectionClipRect;
  final Rect? draftRect;
  final Offset? draftStart;
  final Offset? draftEnd;
  final ScreenshotAnnotationType? draftType;
  final Color previewColor;

  @override
  void paint(Canvas canvas, Size size) {
    paintWorkspaceAnnotations(canvas, annotations: annotations, canvasOrigin: canvasOrigin, selectionClipRect: selectionClipRect);
    if (draftType == null) {
      return;
    }

    final previewAnnotations = <ScreenshotAnnotation>[];
    if (draftType == ScreenshotAnnotationType.arrow && draftStart != null && draftEnd != null) {
      previewAnnotations.add(ScreenshotAnnotation(id: 'draft-arrow', type: ScreenshotAnnotationType.arrow, start: draftStart, end: draftEnd, color: previewColor));
    } else if (draftRect != null) {
      previewAnnotations.add(ScreenshotAnnotation(id: 'draft-shape', type: draftType!, rect: draftRect, color: previewColor));
    }

    paintWorkspaceAnnotations(canvas, annotations: previewAnnotations, canvasOrigin: canvasOrigin, selectionClipRect: selectionClipRect);
  }

  @override
  bool shouldRepaint(covariant _AnnotationPainter oldDelegate) {
    return oldDelegate.annotations != annotations ||
        oldDelegate.canvasOrigin != canvasOrigin ||
        oldDelegate.selectionClipRect != selectionClipRect ||
        oldDelegate.draftRect != draftRect ||
        oldDelegate.draftStart != draftStart ||
        oldDelegate.draftEnd != draftEnd ||
        oldDelegate.draftType != draftType;
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

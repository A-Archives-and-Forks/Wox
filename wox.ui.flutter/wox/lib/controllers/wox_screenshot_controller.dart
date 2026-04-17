import 'dart:async';
import 'dart:ui' as ui;
import 'dart:typed_data';

import 'package:flutter/material.dart';
import 'package:get/get.dart';
import 'package:uuid/v4.dart';
import 'package:wox/api/wox_api.dart';
import 'package:wox/controllers/wox_launcher_controller.dart';
import 'package:wox/controllers/wox_setting_controller.dart';
import 'package:wox/entity/screenshot_session.dart';
import 'package:wox/utils/log.dart';
import 'package:wox/utils/screenshot/screenshot_platform_bridge.dart';
import 'package:wox/utils/windows/window_manager.dart';

class WoxScreenshotController extends GetxController {
  final isSessionActive = false.obs;
  final stage = ScreenshotSessionStage.idle.obs;
  final currentTool = ScreenshotTool.select.obs;
  final displaySnapshots = <DisplaySnapshot>[].obs;
  final annotations = <ScreenshotAnnotation>[].obs;
  final selection = Rxn<ScreenshotRect>();
  final virtualBounds = Rxn<ScreenshotRect>();
  final textDraftPosition = Rxn<Offset>();
  final textDraftController = TextEditingController();

  final Map<String, ui.Image> _decodedImages = <String, ui.Image>{};
  Completer<CaptureScreenshotResult>? _sessionCompleter;
  _SavedScreenshotWindowState? _savedWindowState;

  String tr(String key) => Get.find<WoxSettingController>().tr(key);

  Rect get virtualBoundsRect => virtualBounds.value?.toRect() ?? Rect.zero;

  Rect? get selectionRect => selection.value?.toRect();

  Future<CaptureScreenshotResult> startCaptureSession(
    String traceId,
    CaptureScreenshotRequest request,
  ) async {
    if (_sessionCompleter != null && !_sessionCompleter!.isCompleted) {
      return CaptureScreenshotResult.failed(
        errorCode: 'busy',
        errorMessage: 'Screenshot session is already running',
      );
    }

    _sessionCompleter = Completer<CaptureScreenshotResult>();
    await _prepareNewSession(traceId);

    try {
      final snapshots =
          await ScreenshotPlatformBridge.instance.captureAllDisplays();
      if (snapshots.isEmpty) {
        throw StateError('No display snapshots returned');
      }

      await _decodeDisplayImages(snapshots);
      displaySnapshots.assignAll(snapshots);
      virtualBounds.value = ScreenshotRect.fromRect(
        _calculateUnionRect(
          snapshots.map((item) => item.logicalBounds.toRect()).toList(),
        ),
      );

      final bounds = virtualBoundsRect;
      // We resize the single Wox window to the virtual desktop after the capture completes so the
      // Flutter workspace can own region selection across monitors without relying on multi-window support.
      await windowManager.setBounds(bounds.topLeft, bounds.size);
      await windowManager.setAlwaysOnTop(true);
      await windowManager.show();
      await windowManager.focus();
      await WoxApi.instance.onShow(traceId);
      stage.value = ScreenshotSessionStage.selecting;
    } catch (e) {
      Logger.instance.error(traceId, 'Failed to start screenshot session: $e');
      final failed = CaptureScreenshotResult.failed(
        errorCode: 'capture_failed',
        errorMessage: e.toString(),
      );
      await _restoreWindowState(traceId);
      _resetSessionState();
      _sessionCompleter = null;
      return failed;
    }

    return _sessionCompleter!.future;
  }

  Future<void> _prepareNewSession(String traceId) async {
    _disposeDecodedImages();
    displaySnapshots.clear();
    annotations.clear();
    selection.value = null;
    virtualBounds.value = null;
    currentTool.value = ScreenshotTool.select;
    textDraftController.clear();
    textDraftPosition.value = null;
    stage.value = ScreenshotSessionStage.loading;
    isSessionActive.value = true;

    final launcherController = Get.find<WoxLauncherController>();
    final isVisible = await windowManager.isVisible();
    final position = await windowManager.getPosition();
    final size = await windowManager.getSize();
    _savedWindowState = _SavedScreenshotWindowState(
      wasVisible: isVisible,
      wasInSettingView: launcherController.isInSettingView.value,
      position: position,
      size: size,
      forceHideOnBlur: launcherController.forceHideOnBlur,
    );

    launcherController.forceHideOnBlur = false;
    if (isVisible) {
      await WoxApi.instance.onHide(traceId);
    }

    // Hiding the current window before native capture prevents the launcher itself from ending up in
    // the captured background, which is a hard requirement for the single-window screenshot workflow.
    await windowManager.hide();
    await Future.delayed(const Duration(milliseconds: 120));
  }

  Future<void> cancelSession(String traceId) async {
    await _finishSession(
      traceId,
      CaptureScreenshotResult.cancelled(),
      ScreenshotSessionStage.cancelled,
    );
  }

  Future<void> failSession(
    String traceId, {
    required String errorCode,
    required String errorMessage,
  }) async {
    await _finishSession(
      traceId,
      CaptureScreenshotResult.failed(
        errorCode: errorCode,
        errorMessage: errorMessage,
      ),
      ScreenshotSessionStage.failed,
    );
  }

  Future<void> confirmSelection(String traceId) async {
    final currentSelection = selectionRect;
    if (currentSelection == null ||
        currentSelection.width < 1 ||
        currentSelection.height < 1) {
      return;
    }

    stage.value = ScreenshotSessionStage.exporting;
    try {
      final pngBytes = await exportSelectionPng();
      await _finishSession(
        traceId,
        CaptureScreenshotResult.completed(pngBytes, currentSelection),
        ScreenshotSessionStage.done,
      );
    } catch (e) {
      Logger.instance.error(traceId, 'Failed to export screenshot: $e');
      await failSession(
        traceId,
        errorCode: 'export_failed',
        errorMessage: e.toString(),
      );
    }
  }

  Future<void> _finishSession(
    String traceId,
    CaptureScreenshotResult result,
    ScreenshotSessionStage finalStage,
  ) async {
    final completer = _sessionCompleter;
    if (completer == null || completer.isCompleted) {
      return;
    }

    stage.value = finalStage;
    await _restoreWindowState(traceId);
    _resetSessionState();
    completer.complete(result);
    _sessionCompleter = null;
  }

  Future<void> _restoreWindowState(String traceId) async {
    final savedState = _savedWindowState;
    if (savedState == null) {
      return;
    }

    final launcherController = Get.find<WoxLauncherController>();
    launcherController.forceHideOnBlur = savedState.forceHideOnBlur;

    await windowManager.setAlwaysOnTop(!savedState.wasInSettingView);
    await windowManager.setBounds(savedState.position, savedState.size);

    if (savedState.wasVisible) {
      await windowManager.show();
      await windowManager.focus();
      await WoxApi.instance.onShow(traceId);
      if (savedState.wasInSettingView) {
        Get.find<WoxSettingController>().settingFocusNode.requestFocus();
      } else {
        launcherController.focusQueryBox(selectAll: true);
      }
    } else {
      await windowManager.hide();
      await WoxApi.instance.onHide(traceId);
    }
  }

  void _resetSessionState() {
    _savedWindowState = null;
    textDraftPosition.value = null;
    textDraftController.clear();
    currentTool.value = ScreenshotTool.select;
    selection.value = null;
    displaySnapshots.clear();
    annotations.clear();
    virtualBounds.value = null;
    stage.value = ScreenshotSessionStage.idle;
    isSessionActive.value = false;
    _disposeDecodedImages();
  }

  void updateSelection(Rect rect) {
    final clampedRect = _clampRectToBounds(rect, virtualBoundsRect);
    selection.value = ScreenshotRect.fromRect(clampedRect);
    if (stage.value == ScreenshotSessionStage.selecting ||
        stage.value == ScreenshotSessionStage.annotating) {
      stage.value = ScreenshotSessionStage.annotating;
    }
  }

  void setTool(ScreenshotTool tool) {
    currentTool.value = tool;
    if (tool != ScreenshotTool.text) {
      textDraftPosition.value = null;
      textDraftController.clear();
    }
  }

  void startTextDraft(Offset position) {
    textDraftPosition.value = position;
    textDraftController.clear();
  }

  void cancelTextDraft() {
    textDraftPosition.value = null;
    textDraftController.clear();
  }

  void commitTextDraft() {
    final position = textDraftPosition.value;
    final text = textDraftController.text.trim();
    if (position == null || text.isEmpty) {
      cancelTextDraft();
      return;
    }

    annotations.add(
      ScreenshotAnnotation(
        id: const UuidV4().generate(),
        type: ScreenshotAnnotationType.text,
        start: position,
        text: text,
      ),
    );
    cancelTextDraft();
  }

  void addShapeAnnotation(ScreenshotAnnotationType type, Rect rect) {
    if (rect.width < 2 || rect.height < 2) {
      return;
    }

    annotations.add(
      ScreenshotAnnotation(
        id: const UuidV4().generate(),
        type: type,
        rect: rect,
      ),
    );
  }

  void addArrowAnnotation(Offset start, Offset end) {
    if ((start - end).distance < 2) {
      return;
    }

    annotations.add(
      ScreenshotAnnotation(
        id: const UuidV4().generate(),
        type: ScreenshotAnnotationType.arrow,
        start: start,
        end: end,
      ),
    );
  }

  void undoAnnotation() {
    if (annotations.isEmpty) {
      return;
    }
    annotations.removeLast();
  }

  Future<Uint8List> exportSelectionPng() async {
    final currentSelection = selectionRect;
    if (currentSelection == null) {
      throw StateError('Selection is empty');
    }

    final exportSlices = <_DisplayExportSlice>[];
    for (final snapshot in displaySnapshots) {
      final logicalRect = snapshot.logicalBounds.toRect();
      final intersection = logicalRect.intersect(currentSelection);
      if (intersection.isEmpty) {
        continue;
      }

      final decodedImage = _decodedImages[snapshot.displayId];
      if (decodedImage == null) {
        continue;
      }

      final sourceScaleX = decodedImage.width / logicalRect.width;
      final sourceScaleY = decodedImage.height / logicalRect.height;
      final pixelScaleX = snapshot.pixelBounds.width / logicalRect.width;
      final pixelScaleY = snapshot.pixelBounds.height / logicalRect.height;
      final sourceRect = Rect.fromLTWH(
        (intersection.left - logicalRect.left) * sourceScaleX,
        (intersection.top - logicalRect.top) * sourceScaleY,
        intersection.width * sourceScaleX,
        intersection.height * sourceScaleY,
      );
      final destRect = Rect.fromLTWH(
        snapshot.pixelBounds.x +
            (intersection.left - logicalRect.left) * pixelScaleX,
        snapshot.pixelBounds.y +
            (intersection.top - logicalRect.top) * pixelScaleY,
        intersection.width * pixelScaleX,
        intersection.height * pixelScaleY,
      );

      exportSlices.add(
        _DisplayExportSlice(
          image: decodedImage,
          logicalRect: logicalRect,
          sourceRect: sourceRect,
          destRect: destRect,
          pixelScaleX: pixelScaleX,
          pixelScaleY: pixelScaleY,
        ),
      );
    }

    if (exportSlices.isEmpty) {
      throw StateError('Selection does not intersect any captured display');
    }

    final pixelUnion = _calculateUnionRect(
      exportSlices.map((item) => item.destRect).toList(),
    );
    final recorder = ui.PictureRecorder();
    final canvas = Canvas(recorder);
    final paint = Paint();

    for (final slice in exportSlices) {
      canvas.drawImageRect(
        slice.image,
        slice.sourceRect,
        slice.destRect.shift(-pixelUnion.topLeft),
        paint,
      );
    }

    for (final slice in exportSlices) {
      canvas.save();
      final localDestRect = slice.destRect.shift(-pixelUnion.topLeft);
      canvas.clipRect(localDestRect);
      canvas.translate(
        localDestRect.left -
            (slice.logicalRect.left - currentSelection.left) *
                slice.pixelScaleX,
        localDestRect.top -
            (slice.logicalRect.top - currentSelection.top) * slice.pixelScaleY,
      );
      canvas.scale(slice.pixelScaleX, slice.pixelScaleY);
      paintScreenshotAnnotations(canvas, annotations, currentSelection.topLeft);
      canvas.restore();
    }

    final picture = recorder.endRecording();
    final image = await picture.toImage(
      pixelUnion.width.ceil(),
      pixelUnion.height.ceil(),
    );
    final byteData = await image.toByteData(format: ui.ImageByteFormat.png);
    if (byteData == null) {
      throw StateError('Failed to encode exported screenshot');
    }

    return byteData.buffer.asUint8List();
  }

  Future<void> _decodeDisplayImages(List<DisplaySnapshot> snapshots) async {
    _disposeDecodedImages();
    for (final snapshot in snapshots) {
      final codec = await ui.instantiateImageCodec(snapshot.imageBytes);
      final frame = await codec.getNextFrame();
      _decodedImages[snapshot.displayId] = frame.image;
    }
  }

  void _disposeDecodedImages() {
    for (final image in _decodedImages.values) {
      image.dispose();
    }
    _decodedImages.clear();
  }

  Rect _calculateUnionRect(List<Rect> rects) {
    var left = rects.first.left;
    var top = rects.first.top;
    var right = rects.first.right;
    var bottom = rects.first.bottom;

    for (final rect in rects.skip(1)) {
      left = left < rect.left ? left : rect.left;
      top = top < rect.top ? top : rect.top;
      right = right > rect.right ? right : rect.right;
      bottom = bottom > rect.bottom ? bottom : rect.bottom;
    }

    return Rect.fromLTRB(left, top, right, bottom);
  }

  Rect _clampRectToBounds(Rect rect, Rect bounds) {
    final normalized = Rect.fromPoints(rect.topLeft, rect.bottomRight);
    final left = normalized.left.clamp(bounds.left, bounds.right);
    final top = normalized.top.clamp(bounds.top, bounds.bottom);
    final right = normalized.right.clamp(bounds.left, bounds.right);
    final bottom = normalized.bottom.clamp(bounds.top, bounds.bottom);
    return Rect.fromLTRB(left, top, right, bottom);
  }

  @override
  void onClose() {
    _disposeDecodedImages();
    textDraftController.dispose();
    super.onClose();
  }

  Future<void> resetForIntegrationTest() async {
    if (_sessionCompleter != null && !_sessionCompleter!.isCompleted) {
      _sessionCompleter!.complete(CaptureScreenshotResult.cancelled());
    }
    _sessionCompleter = null;
    _savedWindowState = null;
    _resetSessionState();
    ScreenshotPlatformBridge.resetInstance();
  }
}

class _SavedScreenshotWindowState {
  const _SavedScreenshotWindowState({
    required this.wasVisible,
    required this.wasInSettingView,
    required this.position,
    required this.size,
    required this.forceHideOnBlur,
  });

  final bool wasVisible;
  final bool wasInSettingView;
  final Offset position;
  final Size size;
  final bool forceHideOnBlur;
}

class _DisplayExportSlice {
  const _DisplayExportSlice({
    required this.image,
    required this.logicalRect,
    required this.sourceRect,
    required this.destRect,
    required this.pixelScaleX,
    required this.pixelScaleY,
  });

  final ui.Image image;
  final Rect logicalRect;
  final Rect sourceRect;
  final Rect destRect;
  final double pixelScaleX;
  final double pixelScaleY;
}

void paintScreenshotAnnotations(
  Canvas canvas,
  List<ScreenshotAnnotation> annotations,
  Offset selectionOrigin,
) {
  for (final annotation in annotations) {
    final paint =
        Paint()
          ..color = annotation.color
          ..strokeWidth = annotation.strokeWidth
          ..style =
              annotation.type == ScreenshotAnnotationType.text
                  ? PaintingStyle.fill
                  : PaintingStyle.stroke
          ..strokeCap = StrokeCap.round
          ..strokeJoin = StrokeJoin.round;

    switch (annotation.type) {
      case ScreenshotAnnotationType.rect:
        if (annotation.rect != null) {
          canvas.drawRect(annotation.rect!.shift(-selectionOrigin), paint);
        }
        break;
      case ScreenshotAnnotationType.ellipse:
        if (annotation.rect != null) {
          canvas.drawOval(annotation.rect!.shift(-selectionOrigin), paint);
        }
        break;
      case ScreenshotAnnotationType.arrow:
        final start = annotation.start;
        final end = annotation.end;
        if (start == null || end == null) {
          break;
        }

        final localStart = start - selectionOrigin;
        final localEnd = end - selectionOrigin;
        canvas.drawLine(localStart, localEnd, paint);
        final angle = (localEnd - localStart).direction;
        const arrowLength = 16.0;
        final arrowLeft =
            localEnd - Offset.fromDirection(angle - 0.5, arrowLength);
        final arrowRight =
            localEnd - Offset.fromDirection(angle + 0.5, arrowLength);
        canvas.drawLine(localEnd, arrowLeft, paint);
        canvas.drawLine(localEnd, arrowRight, paint);
        break;
      case ScreenshotAnnotationType.text:
        final start = annotation.start;
        final text = annotation.text;
        if (start == null || text == null || text.isEmpty) {
          break;
        }

        final textPainter = TextPainter(
          text: TextSpan(
            text: text,
            style: TextStyle(
              color: annotation.color,
              fontSize: annotation.fontSize,
              fontWeight: FontWeight.w600,
              shadows: const [Shadow(color: Color(0xAA000000), blurRadius: 4)],
            ),
          ),
          textDirection: TextDirection.ltr,
        )..layout(maxWidth: 480);
        textPainter.paint(canvas, start - selectionOrigin);
        break;
    }
  }
}

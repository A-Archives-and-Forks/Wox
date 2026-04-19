import 'dart:async';
import 'dart:io';
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
  static const Color defaultAnnotationColor = Color(0xFFFF5B36);
  static const double minTextFontSize = 12;
  static const double maxTextFontSize = 48;

  final isSessionActive = false.obs;
  final stage = ScreenshotSessionStage.idle.obs;
  final currentTool = ScreenshotTool.select.obs;
  final displaySnapshots = <DisplaySnapshot>[].obs;
  final annotations = <ScreenshotAnnotation>[].obs;
  final selection = Rxn<ScreenshotRect>();
  final virtualBounds = Rxn<ScreenshotRect>();
  final workspaceScale = 1.0.obs;
  final selectedAnnotationId = RxnString();
  final editingTextAnnotationId = RxnString();
  final annotationCreationColor = defaultAnnotationColor.obs;
  final textDraftPosition = Rxn<Offset>();
  final textDraftFontSize = 20.0.obs;
  final textDraftColor = defaultAnnotationColor.obs;
  final textDraftController = TextEditingController();

  final Map<String, ui.Image> _decodedImages = <String, ui.Image>{};
  Completer<CaptureScreenshotResult>? _sessionCompleter;
  _SavedScreenshotWindowState? _savedWindowState;

  String tr(String key) => Get.find<WoxSettingController>().tr(key);

  Rect get virtualBoundsRect => virtualBounds.value?.toRect() ?? Rect.zero;

  Rect? get selectionRect => selection.value?.toRect();

  ScreenshotAnnotation? get selectedAnnotation => annotationById(selectedAnnotationId.value);

  ScreenshotAnnotation? annotationById(String? annotationId) {
    if (annotationId == null) {
      return null;
    }

    for (final annotation in annotations) {
      if (annotation.id == annotationId) {
        return annotation;
      }
    }

    return null;
  }

  Future<CaptureScreenshotResult> startCaptureSession(String traceId, CaptureScreenshotRequest request) async {
    if (_sessionCompleter != null && !_sessionCompleter!.isCompleted) {
      return CaptureScreenshotResult.failed(errorCode: 'busy', errorMessage: 'Screenshot session is already running');
    }

    _sessionCompleter = Completer<CaptureScreenshotResult>();
    await _prepareNewSession(traceId);

    try {
      final rawSnapshots = await ScreenshotPlatformBridge.instance.captureAllDisplays();
      if (rawSnapshots.isEmpty) {
        throw StateError('No display snapshots returned');
      }

      final nativeWorkspaceBounds = _calculateUnionRect(rawSnapshots.map((item) => item.logicalBounds.toRect()).toList());
      final nativeSelectionResult = await _tryStartMacOSNativeSelectionEditor(traceId, rawSnapshots, nativeWorkspaceBounds);
      if (nativeSelectionResult != null) {
        return nativeSelectionResult;
      }

      final presentation = await ScreenshotPlatformBridge.instance.presentCaptureWorkspace(ScreenshotRect.fromRect(nativeWorkspaceBounds));
      final normalizedSnapshots = _normalizeSnapshotsForWorkspace(
        rawSnapshots,
        nativeWorkspaceBounds: nativeWorkspaceBounds,
        workspaceBounds: presentation.workspaceBounds.toRect(),
        workspaceScale: presentation.workspaceScale,
      );

      await _decodeDisplayImages(normalizedSnapshots);
      displaySnapshots.assignAll(normalizedSnapshots);
      virtualBounds.value = ScreenshotRect.fromRect(presentation.workspaceBounds.toRect());
      workspaceScale.value = presentation.workspaceScale;

      if (!presentation.presentedByPlatform) {
        final bounds = virtualBoundsRect;
        // The fallback path still uses one Flutter window, but only platforms without screenshot-
        // specific native presentation should reach it. macOS and Windows install their own
        // capture overlay handling so multi-display selection does not inherit launcher assumptions.
        await windowManager.setBounds(bounds.topLeft, bounds.size);
        await windowManager.setAlwaysOnTop(true);
        await windowManager.show();
        await windowManager.focus();
      }

      await WoxApi.instance.onShow(traceId);
      stage.value = ScreenshotSessionStage.selecting;
    } catch (e) {
      Logger.instance.error(traceId, 'Failed to start screenshot session: $e');
      final failed = CaptureScreenshotResult.failed(errorCode: 'capture_failed', errorMessage: e.toString());
      await _restoreWindowState(traceId);
      _resetSessionState();
      _sessionCompleter = null;
      return failed;
    }

    return _sessionCompleter!.future;
  }

  Future<CaptureScreenshotResult?> _tryStartMacOSNativeSelectionEditor(String traceId, List<DisplaySnapshot> rawSnapshots, Rect nativeWorkspaceBounds) async {
    if (!Platform.isMacOS || rawSnapshots.length < 2) {
      return null;
    }

    final nativeSelection = await ScreenshotPlatformBridge.instance.selectCaptureRegion(ScreenshotRect.fromRect(nativeWorkspaceBounds));
    if (!nativeSelection.wasHandled) {
      return null;
    }

    if (nativeSelection.selection == null) {
      final cancelled = CaptureScreenshotResult.cancelled();
      await _restoreWindowState(traceId);
      _resetSessionState();
      _sessionCompleter = null;
      return cancelled;
    }

    // A single Flutter window cannot reliably render across multiple macOS displays, so the
    // annotation editor is confined to the monitor where the user drew the selection. This avoids
    // cross-display rendering artifacts while keeping the transition seamless on that monitor.
    final selectedDisplayBounds = _findDisplayBoundsForSelection(nativeSelection.selection!.toRect(), rawSnapshots);

    final presentation = await ScreenshotPlatformBridge.instance.presentCaptureWorkspace(ScreenshotRect.fromRect(selectedDisplayBounds));
    final normalizedSnapshots = _normalizeSnapshotsForWorkspace(
      rawSnapshots,
      nativeWorkspaceBounds: selectedDisplayBounds,
      workspaceBounds: presentation.workspaceBounds.toRect(),
      workspaceScale: presentation.workspaceScale,
    );
    final normalizedSelection = _normalizeNativeRectForWorkspace(
      nativeSelection.selection!.toRect(),
      nativeWorkspaceBounds: selectedDisplayBounds,
      workspaceBounds: presentation.workspaceBounds.toRect(),
      workspaceScale: presentation.workspaceScale,
    );

    await _decodeDisplayImages(normalizedSnapshots);
    displaySnapshots.assignAll(normalizedSnapshots);
    virtualBounds.value = ScreenshotRect.fromRect(presentation.workspaceBounds.toRect());
    // Native selection should only replace the drag phase. Annotation still happens on the full
    // Flutter screenshot workspace so the user keeps the original all-screen shade and toolbar flow.
    selection.value = ScreenshotRect.fromRect(normalizedSelection);
    workspaceScale.value = presentation.workspaceScale;

    if (!presentation.presentedByPlatform) {
      final bounds = virtualBoundsRect;
      await windowManager.setBounds(bounds.topLeft, bounds.size);
      await windowManager.setAlwaysOnTop(true);
      await windowManager.show();
      await windowManager.focus();
    }

    await WoxApi.instance.onShow(traceId);
    stage.value = ScreenshotSessionStage.annotating;
    // Flutter now renders its own background and shade identically to the native overlay. Wait for
    // the first frame so the two surfaces overlap perfectly, then dismiss the native windows. The
    // user sees one continuous surface instead of a flash to the desktop between the two layers.
    await WidgetsBinding.instance.endOfFrame;
    await Future<void>.delayed(const Duration(milliseconds: 16));
    await WidgetsBinding.instance.endOfFrame;
    await ScreenshotPlatformBridge.instance.dismissNativeSelectionOverlays();
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
    // ScreenCaptureKit can still sample the desktop for a short moment after Wox hides, especially
    // when another fullscreen space becomes active. Waiting a bit longer gives the compositor time
    // to promote the previous app before the native multi-display snapshots are captured.
    await Future.delayed(const Duration(milliseconds: 220));
  }

  Future<void> cancelSession(String traceId) async {
    await _finishSession(traceId, CaptureScreenshotResult.cancelled(), ScreenshotSessionStage.cancelled);
  }

  Future<void> failSession(String traceId, {required String errorCode, required String errorMessage}) async {
    await _finishSession(traceId, CaptureScreenshotResult.failed(errorCode: errorCode, errorMessage: errorMessage), ScreenshotSessionStage.failed);
  }

  Future<void> confirmSelection(String traceId) async {
    final currentSelection = selectionRect;
    if (currentSelection == null || currentSelection.width < 1 || currentSelection.height < 1) {
      return;
    }

    stage.value = ScreenshotSessionStage.exporting;
    try {
      final pngBytes = await exportSelectionPng();
      await _finishSession(traceId, CaptureScreenshotResult.completed(pngBytes, currentSelection), ScreenshotSessionStage.done);
    } catch (e) {
      Logger.instance.error(traceId, 'Failed to export screenshot: $e');
      await failSession(traceId, errorCode: 'export_failed', errorMessage: e.toString());
    }
  }

  Future<void> _finishSession(String traceId, CaptureScreenshotResult result, ScreenshotSessionStage finalStage) async {
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

    // The native multi-display selector can stay alive until Flutter confirms its workspace is
    // visible. Closing it here as part of the generic restore path prevents a stuck topmost shade
    // when the screenshot session aborts before that handoff completes.
    await ScreenshotPlatformBridge.instance.dismissNativeSelectionOverlays();
    await ScreenshotPlatformBridge.instance.dismissCaptureWorkspacePresentation();
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
    selectedAnnotationId.value = null;
    editingTextAnnotationId.value = null;
    textDraftPosition.value = null;
    textDraftFontSize.value = 20;
    textDraftColor.value = annotationCreationColor.value;
    textDraftController.clear();
    currentTool.value = ScreenshotTool.select;
    selection.value = null;
    displaySnapshots.clear();
    annotations.clear();
    virtualBounds.value = null;
    workspaceScale.value = 1;
    stage.value = ScreenshotSessionStage.idle;
    isSessionActive.value = false;
    _disposeDecodedImages();
  }

  List<DisplaySnapshot> _normalizeSnapshotsForWorkspace(
    List<DisplaySnapshot> snapshots, {
    required Rect nativeWorkspaceBounds,
    required Rect workspaceBounds,
    required double workspaceScale,
  }) {
    final safeWorkspaceScale = workspaceScale <= 0 ? 1.0 : workspaceScale;

    // Windows capture now reports native virtual-desktop coordinates for every monitor snapshot so
    // one screenshot overlay can span mixed-DPI displays. Normalizing those native coordinates here
    // keeps the widget tree and export logic on one stable workspace contract regardless of the
    // platform-specific capture source.
    return snapshots.map((snapshot) {
      final nativeBounds = snapshot.logicalBounds.toRect();
      final normalizedBounds = Rect.fromLTWH(
        workspaceBounds.left + (nativeBounds.left - nativeWorkspaceBounds.left) / safeWorkspaceScale,
        workspaceBounds.top + (nativeBounds.top - nativeWorkspaceBounds.top) / safeWorkspaceScale,
        nativeBounds.width / safeWorkspaceScale,
        nativeBounds.height / safeWorkspaceScale,
      );

      return snapshot.copyWith(logicalBounds: ScreenshotRect.fromRect(normalizedBounds));
    }).toList();
  }

  void updateSelection(Rect rect) {
    final clampedRect = _clampRectToBounds(rect, virtualBoundsRect);
    selection.value = ScreenshotRect.fromRect(clampedRect);
    if (stage.value == ScreenshotSessionStage.selecting || stage.value == ScreenshotSessionStage.annotating) {
      stage.value = ScreenshotSessionStage.annotating;
    }
  }

  void setTool(ScreenshotTool tool) {
    currentTool.value = tool;
    if (tool != ScreenshotTool.text) {
      cancelTextDraft();
    }
  }

  void selectAnnotation(String? annotationId) {
    if (annotationId != null && annotationById(annotationId) == null) {
      return;
    }

    selectedAnnotationId.value = annotationId;
    if (annotationId == null || editingTextAnnotationId.value != annotationId) {
      editingTextAnnotationId.value = null;
    }
  }

  void startTextDraft(Offset position, {String? annotationId, String initialText = '', double fontSize = 20, Color? color}) {
    textDraftPosition.value = position;
    editingTextAnnotationId.value = annotationId;
    textDraftFontSize.value = fontSize.clamp(minTextFontSize, maxTextFontSize).toDouble();
    textDraftColor.value = color ?? annotationCreationColor.value;
    textDraftController.text = initialText;
    textDraftController.selection = TextSelection(baseOffset: 0, extentOffset: initialText.length);
  }

  void cancelTextDraft() {
    textDraftPosition.value = null;
    textDraftController.clear();
    editingTextAnnotationId.value = null;
    textDraftFontSize.value = 20;
    textDraftColor.value = annotationCreationColor.value;
  }

  void commitTextDraft() {
    final position = textDraftPosition.value;
    final text = textDraftController.text.trim();
    if (position == null || text.isEmpty) {
      cancelTextDraft();
      return;
    }

    final editingAnnotationId = editingTextAnnotationId.value;
    if (editingAnnotationId != null) {
      _replaceAnnotationById(editingAnnotationId, (annotation) => annotation.copyWith(text: text, start: position, fontSize: textDraftFontSize.value, color: textDraftColor.value));
    } else {
      annotations.add(
        ScreenshotAnnotation(
          id: const UuidV4().generate(),
          type: ScreenshotAnnotationType.text,
          start: position,
          text: text,
          color: textDraftColor.value,
          fontSize: textDraftFontSize.value,
        ),
      );
    }
    cancelTextDraft();
  }

  void addShapeAnnotation(ScreenshotAnnotationType type, Rect rect) {
    if (rect.width < 2 || rect.height < 2) {
      return;
    }

    annotations.add(ScreenshotAnnotation(id: const UuidV4().generate(), type: type, rect: rect, color: annotationCreationColor.value));
  }

  void addArrowAnnotation(Offset start, Offset end) {
    if ((start - end).distance < 2) {
      return;
    }

    annotations.add(ScreenshotAnnotation(id: const UuidV4().generate(), type: ScreenshotAnnotationType.arrow, start: start, end: end, color: annotationCreationColor.value));
  }

  // Existing annotations now support editing in place, so controller-level update helpers keep
  // geometry and color mutations out of the widget tree and make selection-aware edits reusable.
  void updateSelectedAnnotationColor(Color color) {
    final annotationId = selectedAnnotationId.value;
    if (annotationId == null) {
      annotationCreationColor.value = color;
      return;
    }

    _replaceAnnotationById(annotationId, (annotation) => annotation.copyWith(color: color));
    if (editingTextAnnotationId.value == annotationId) {
      textDraftColor.value = color;
    }
  }

  void updateSelectedTextFontSize(double delta) {
    final annotation = selectedAnnotation;
    if (annotation == null || annotation.type != ScreenshotAnnotationType.text) {
      return;
    }

    final nextSize = (annotation.fontSize + delta).clamp(minTextFontSize, maxTextFontSize).toDouble();
    _replaceAnnotationById(annotation.id, (current) => current.copyWith(fontSize: nextSize));
    if (editingTextAnnotationId.value == annotation.id) {
      textDraftFontSize.value = nextSize;
    }
  }

  void setAnnotationCreationColor(Color color) {
    annotationCreationColor.value = color;
  }

  void updateAnnotationRect(String annotationId, Rect rect) {
    _replaceAnnotationById(annotationId, (annotation) => annotation.copyWith(rect: rect));
  }

  void updateArrowPoints(String annotationId, {Offset? start, Offset? end}) {
    _replaceAnnotationById(annotationId, (annotation) => annotation.copyWith(start: start ?? annotation.start, end: end ?? annotation.end));
  }

  void updateTextPosition(String annotationId, Offset position) {
    _replaceAnnotationById(annotationId, (annotation) => annotation.copyWith(start: position));
    if (editingTextAnnotationId.value == annotationId) {
      textDraftPosition.value = position;
    }
  }

  void deleteSelectedAnnotation() {
    final annotationId = selectedAnnotationId.value;
    if (annotationId == null) {
      return;
    }

    final removedEditingAnnotation = editingTextAnnotationId.value == annotationId;
    annotations.removeWhere((annotation) => annotation.id == annotationId);
    selectedAnnotationId.value = null;
    if (removedEditingAnnotation) {
      cancelTextDraft();
    }
  }

  void undoAnnotation() {
    if (annotations.isEmpty) {
      return;
    }
    final removed = annotations.removeLast();
    if (removed.id == selectedAnnotationId.value) {
      selectedAnnotationId.value = null;
    }
    if (removed.id == editingTextAnnotationId.value) {
      cancelTextDraft();
    }
  }

  Rect _normalizeNativeRectForWorkspace(Rect nativeRect, {required Rect nativeWorkspaceBounds, required Rect workspaceBounds, required double workspaceScale}) {
    final safeWorkspaceScale = workspaceScale <= 0 ? 1.0 : workspaceScale;
    return Rect.fromLTWH(
      workspaceBounds.left + (nativeRect.left - nativeWorkspaceBounds.left) / safeWorkspaceScale,
      workspaceBounds.top + (nativeRect.top - nativeWorkspaceBounds.top) / safeWorkspaceScale,
      nativeRect.width / safeWorkspaceScale,
      nativeRect.height / safeWorkspaceScale,
    );
  }

  /// Finds the display whose bounds best contain the given selection rect. Used to confine the
  /// Flutter annotation editor to a single monitor after the native multi-display selector finishes,
  /// since a single Flutter window cannot reliably render across multiple macOS displays.
  Rect _findDisplayBoundsForSelection(Rect selection, List<DisplaySnapshot> snapshots) {
    final center = selection.center;
    for (final snapshot in snapshots) {
      if (snapshot.logicalBounds.toRect().contains(center)) {
        return snapshot.logicalBounds.toRect();
      }
    }

    DisplaySnapshot? best;
    double bestArea = 0;
    for (final snapshot in snapshots) {
      final intersection = snapshot.logicalBounds.toRect().intersect(selection);
      if (!intersection.isEmpty) {
        final area = intersection.width * intersection.height;
        if (area > bestArea) {
          bestArea = area;
          best = snapshot;
        }
      }
    }
    return best?.logicalBounds.toRect() ?? snapshots.first.logicalBounds.toRect();
  }

  Future<Uint8List> exportSelectionPng() async {
    final currentSelection = selectionRect;
    if (currentSelection == null) {
      throw StateError('Selection is empty');
    }

    final rendered = await _renderSelectionImage(selection: currentSelection, snapshots: displaySnapshots.toList(), annotationsToPaint: annotations.toList());
    return rendered.pngBytes;
  }

  Future<_RenderedSelectionImage> _renderSelectionImage({
    required Rect selection,
    required List<DisplaySnapshot> snapshots,
    required List<ScreenshotAnnotation> annotationsToPaint,
  }) async {
    final exportSlices = <_DisplayExportSlice>[];
    for (final snapshot in snapshots) {
      final logicalRect = snapshot.logicalBounds.toRect();
      final intersection = logicalRect.intersect(selection);
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
        snapshot.pixelBounds.x + (intersection.left - logicalRect.left) * pixelScaleX,
        snapshot.pixelBounds.y + (intersection.top - logicalRect.top) * pixelScaleY,
        intersection.width * pixelScaleX,
        intersection.height * pixelScaleY,
      );

      exportSlices.add(
        _DisplayExportSlice(
          image: decodedImage,
          logicalRect: logicalRect,
          intersectionRect: intersection,
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

    final pixelUnion = _calculateUnionRect(exportSlices.map((item) => item.destRect).toList());
    final recorder = ui.PictureRecorder();
    final canvas = Canvas(recorder);
    final paint = Paint();

    for (final slice in exportSlices) {
      canvas.drawImageRect(slice.image, slice.sourceRect, slice.destRect.shift(-pixelUnion.topLeft), paint);
    }

    for (final slice in exportSlices) {
      canvas.save();
      final localDestRect = slice.destRect.shift(-pixelUnion.topLeft);
      canvas.clipRect(localDestRect);
      // Exported annotations are painted in selection-local logical coordinates. The previous
      // translation anchored them to the full display origin, which pushed shapes/text outside the
      // exported crop whenever the selection started away from the monitor's top-left corner.
      // Align each slice to the slice/selection intersection instead so mixed-DPI exports keep the
      // same annotation positions the user saw in the editor and clipboard output.
      canvas.translate(
        localDestRect.left - (slice.intersectionRect.left - selection.left) * slice.pixelScaleX,
        localDestRect.top - (slice.intersectionRect.top - selection.top) * slice.pixelScaleY,
      );
      canvas.scale(slice.pixelScaleX, slice.pixelScaleY);
      paintScreenshotAnnotations(canvas, annotationsToPaint, selection.topLeft);
      canvas.restore();
    }

    final picture = recorder.endRecording();
    final image = await picture.toImage(pixelUnion.width.ceil(), pixelUnion.height.ceil());
    final byteData = await image.toByteData(format: ui.ImageByteFormat.png);
    image.dispose();
    if (byteData == null) {
      throw StateError('Failed to encode exported screenshot');
    }

    return _RenderedSelectionImage(pngBytes: byteData.buffer.asUint8List(), pixelWidth: pixelUnion.width.ceil(), pixelHeight: pixelUnion.height.ceil());
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

  void _replaceAnnotationById(String annotationId, ScreenshotAnnotation Function(ScreenshotAnnotation annotation) replace) {
    final index = annotations.indexWhere((annotation) => annotation.id == annotationId);
    if (index < 0) {
      return;
    }

    annotations[index] = replace(annotations[index]);
    annotations.refresh();
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
  const _SavedScreenshotWindowState({required this.wasVisible, required this.wasInSettingView, required this.position, required this.size, required this.forceHideOnBlur});

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
    required this.intersectionRect,
    required this.sourceRect,
    required this.destRect,
    required this.pixelScaleX,
    required this.pixelScaleY,
  });

  final ui.Image image;
  final Rect logicalRect;
  final Rect intersectionRect;
  final Rect sourceRect;
  final Rect destRect;
  final double pixelScaleX;
  final double pixelScaleY;
}

class _RenderedSelectionImage {
  const _RenderedSelectionImage({required this.pngBytes, required this.pixelWidth, required this.pixelHeight});

  final Uint8List pngBytes;
  final int pixelWidth;
  final int pixelHeight;
}

void paintScreenshotAnnotations(Canvas canvas, List<ScreenshotAnnotation> annotations, Offset selectionOrigin) {
  for (final annotation in annotations) {
    final paint =
        Paint()
          ..color = annotation.color
          ..strokeWidth = annotation.strokeWidth
          ..style = annotation.type == ScreenshotAnnotationType.text ? PaintingStyle.fill : PaintingStyle.stroke
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
        final arrowLeft = localEnd - Offset.fromDirection(angle - 0.5, arrowLength);
        final arrowRight = localEnd - Offset.fromDirection(angle + 0.5, arrowLength);
        canvas.drawLine(localEnd, arrowLeft, paint);
        canvas.drawLine(localEnd, arrowRight, paint);
        break;
      case ScreenshotAnnotationType.text:
        final start = annotation.start;
        final textPainter = buildScreenshotTextPainter(annotation);
        if (start == null || textPainter == null) {
          break;
        }

        textPainter.paint(canvas, start - selectionOrigin);
        break;
    }
  }
}

// Text annotations now support selection, drag, and inline editing. Centralizing the text layout
// logic keeps hit testing and painting aligned so editing overlays appear exactly where the text
// was rendered instead of drifting due to duplicated style calculations.
TextPainter? buildScreenshotTextPainter(ScreenshotAnnotation annotation) {
  final start = annotation.start;
  final text = annotation.text;
  if (start == null || text == null || text.isEmpty) {
    return null;
  }

  return TextPainter(
    text: TextSpan(
      text: text,
      style: TextStyle(color: annotation.color, fontSize: annotation.fontSize, fontWeight: FontWeight.w600, shadows: const [Shadow(color: Color(0xAA000000), blurRadius: 4)]),
    ),
    textDirection: TextDirection.ltr,
  )..layout(maxWidth: 480);
}

Rect? screenshotAnnotationBounds(ScreenshotAnnotation annotation) {
  switch (annotation.type) {
    case ScreenshotAnnotationType.rect:
    case ScreenshotAnnotationType.ellipse:
      return annotation.rect;
    case ScreenshotAnnotationType.arrow:
      final start = annotation.start;
      final end = annotation.end;
      if (start == null || end == null) {
        return null;
      }
      return Rect.fromPoints(start, end);
    case ScreenshotAnnotationType.text:
      final start = annotation.start;
      final textPainter = buildScreenshotTextPainter(annotation);
      if (start == null || textPainter == null) {
        return null;
      }
      return start & textPainter.size;
  }
}

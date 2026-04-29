import 'dart:async';
import 'dart:io';
import 'dart:math' as math;
import 'dart:typed_data';
import 'dart:ui' as ui;

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
  static const int _scrollingCaptureMaxFrames = 16;
  static const double _scrollingCaptureWheelSteps = 7;
  static const Duration _scrollingCaptureSettleDelay = Duration(milliseconds: 120);
  static const double _scrollingCaptureOverlapThreshold = 20;
  static const double _scrollingCaptureDuplicateThreshold = 6;
  static const int _scrollingCaptureSeamFeatherRows = 10;
  static const double _scrollingCaptureToolbarMinWidth = 168;

  final isSessionActive = false.obs;
  final stage = ScreenshotSessionStage.idle.obs;
  final currentTool = ScreenshotTool.select.obs;
  final displaySnapshots = <DisplaySnapshot>[].obs;
  final annotations = <ScreenshotAnnotation>[].obs;
  final scrollingCaptureFrames = <ScrollingCapturePreviewFrame>[].obs;
  final isScrollingCaptureUpdating = false.obs;
  final isNativeScrollingCaptureOverlay = false.obs;
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
  final Map<String, Future<void>> _displayDecodeTasks = <String, Future<void>>{};
  List<DisplaySnapshot> _pendingRawSnapshots = const <DisplaySnapshot>[];
  final Map<String, DisplaySnapshot> _hydratedRawSnapshots = <String, DisplaySnapshot>{};
  final Map<String, Future<DisplaySnapshot>> _rawSnapshotHydrationTasks = <String, Future<DisplaySnapshot>>{};
  Rect? _nativeWorkspaceBounds;
  Rect? _activeNativeWorkspaceBounds;
  String? _preparedDisplayId;
  Rect? _preparedDisplayBounds;
  ScreenshotWorkspacePresentation? _preparedPresentation;
  List<DisplaySnapshot>? _preparedSnapshots;
  Timer? _scrollingCaptureFrameDebounce;
  StreamSubscription<void>? _scrollingCaptureWheelSubscription;
  Rect? _scrollingCaptureControlsBounds;
  Rect? _pendingScrollingCaptureSelection;
  StreamSubscription<ScreenshotSelectionDisplayHint>? _selectionDisplayHintSubscription;
  bool _acceptSelectionDisplayHints = false;
  int _preparedDisplayRevision = 0;
  int _captureSessionRevision = 0;
  Completer<CaptureScreenshotResult>? _sessionCompleter;
  _SavedScreenshotWindowState? _savedWindowState;
  CaptureScreenshotRequest? _activeRequest;

  String tr(String key) => Get.find<WoxSettingController>().tr(key);

  // The screenshot view needs read-only access to caller metadata such as the plugin icon. Keeping
  // mutation inside the controller preserves the existing session lifecycle while allowing the
  // toolbox to render request-scoped identity details.
  CaptureScreenshotRequest? get activeRequest => _activeRequest;

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

    _activeRequest = request;
    _sessionCompleter = Completer<CaptureScreenshotResult>();
    await _prepareNewSession(traceId);

    try {
      final metadataSnapshots = await ScreenshotPlatformBridge.instance.captureDisplayMetadata();
      if (metadataSnapshots.isEmpty) {
        throw StateError('No display snapshots returned');
      }

      final nativeWorkspaceBounds = _calculateUnionRect(metadataSnapshots.map((item) => item.logicalBounds.toRect()).toList());
      if (Platform.isMacOS) {
        // macOS native selection now starts from cached display metadata so the topmost overlay can
        // appear before Flutter receives PNG/base64 payloads for every monitor. That keeps the
        // screenshot startup path focused on the native selector, then hydrates pixels only when
        // the annotation/export pipeline truly needs them.
        final nativeSelectionResult = await _tryStartMacOSNativeSelectionEditor(traceId, metadataSnapshots, nativeWorkspaceBounds);
        if (nativeSelectionResult != null) {
          return nativeSelectionResult;
        }

        await _presentPreparedCaptureWorkspace(traceId, metadataSnapshots, nativeWorkspaceBounds);
        return _sessionFutureOrCancelled();
      }

      if (Platform.isWindows) {
        await _presentPreparedCaptureWorkspace(traceId, metadataSnapshots, nativeWorkspaceBounds);
      } else {
        final rawSnapshots = await _hydrateRawSnapshots(metadataSnapshots);
        await _presentFlutterCaptureWorkspace(traceId, rawSnapshots, nativeWorkspaceBounds);
      }
    } catch (e) {
      Logger.instance.error(traceId, 'Failed to start screenshot session: $e');
      final failed = CaptureScreenshotResult.failed(errorCode: 'capture_failed', errorMessage: e.toString());
      await _restoreWindowState(traceId);
      _resetSessionState();
      _sessionCompleter = null;
      return failed;
    }

    return _sessionFutureOrCancelled();
  }

  Future<CaptureScreenshotResult> _sessionFutureOrCancelled() {
    final completer = _sessionCompleter;
    if (completer == null || completer.isCompleted) {
      // Window show/focus recovery can cancel a screenshot session while the startup coroutine is
      // still unwinding. Returning a cancelled result here keeps that race user-visible but avoids
      // turning a legitimate cancellation into a null-completer crash.
      return Future<CaptureScreenshotResult>.value(CaptureScreenshotResult.cancelled());
    }

    return completer.future;
  }

  Future<void> _presentFlutterCaptureWorkspace(String traceId, List<DisplaySnapshot> rawSnapshots, Rect nativeWorkspaceBounds) async {
    _activeNativeWorkspaceBounds = nativeWorkspaceBounds;
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
  }

  Future<void> _presentPreparedCaptureWorkspace(String traceId, List<DisplaySnapshot> metadataSnapshots, Rect nativeWorkspaceBounds) async {
    _activeNativeWorkspaceBounds = nativeWorkspaceBounds;
    final presentation = await ScreenshotPlatformBridge.instance.prepareCaptureWorkspace(ScreenshotRect.fromRect(nativeWorkspaceBounds));
    final rawSnapshots = await _hydrateRawSnapshots(metadataSnapshots);
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

    await WoxApi.instance.onShow(traceId);
    if (presentation.presentedByPlatform) {
      // macOS and Windows now share the same handoff: resize and prime the native screenshot shell
      // before Flutter decodes monitor PNGs, then reveal only after the first annotation frame is
      // ready. The previous all-in-one path made the user wait for capture, PNG encoding, layout,
      // and show on one visible transition.
      await ScreenshotPlatformBridge.instance.revealPreparedCaptureWorkspace();
    } else {
      final bounds = virtualBoundsRect;
      await windowManager.setBounds(bounds.topLeft, bounds.size);
      await windowManager.setAlwaysOnTop(true);
      await windowManager.show();
      await windowManager.focus();
    }

    stage.value = ScreenshotSessionStage.selecting;
  }

  Future<CaptureScreenshotResult?> _tryStartMacOSNativeSelectionEditor(String traceId, List<DisplaySnapshot> rawSnapshots, Rect nativeWorkspaceBounds) async {
    if (!Platform.isMacOS || rawSnapshots.length < 2) {
      return null;
    }

    _pendingRawSnapshots = rawSnapshots;
    _nativeWorkspaceBounds = nativeWorkspaceBounds;
    _acceptSelectionDisplayHints = true;
    final previousSelectionDisplayHintSubscription = _selectionDisplayHintSubscription;
    if (previousSelectionDisplayHintSubscription != null) {
      await previousSelectionDisplayHintSubscription.cancel();
    }
    _selectionDisplayHintSubscription = ScreenshotPlatformBridge.instance.selectionDisplayHints().listen((hint) {
      unawaited(_handleMacOSSelectionDisplayHint(traceId, hint));
    });

    final nativeSelection = await ScreenshotPlatformBridge.instance.selectCaptureRegion(ScreenshotRect.fromRect(nativeWorkspaceBounds));
    _acceptSelectionDisplayHints = false;
    final activeSelectionDisplayHintSubscription = _selectionDisplayHintSubscription;
    if (activeSelectionDisplayHintSubscription != null) {
      await activeSelectionDisplayHintSubscription.cancel();
    }
    _selectionDisplayHintSubscription = null;
    if (!nativeSelection.wasHandled) {
      _clearMacOSPreparationState();
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
    final selectedDisplay = _findDisplaySnapshotForSelection(nativeSelection.selection!.toRect(), rawSnapshots);
    _activeNativeWorkspaceBounds = selectedDisplay.logicalBounds.toRect();
    await _prepareMacOSDisplayForAnnotation(selectedDisplay);
    final presentation = _preparedPresentation;
    final normalizedSnapshots = _preparedSnapshots;
    if (presentation == null || normalizedSnapshots == null) {
      throw StateError('macOS screenshot handoff did not prepare a Flutter workspace');
    }
    final normalizedSelection = _normalizeNativeRectForWorkspace(
      nativeSelection.selection!.toRect(),
      nativeWorkspaceBounds: selectedDisplay.logicalBounds.toRect(),
      workspaceBounds: presentation.workspaceBounds.toRect(),
      workspaceScale: presentation.workspaceScale,
    );

    displaySnapshots.assignAll(normalizedSnapshots);
    virtualBounds.value = ScreenshotRect.fromRect(presentation.workspaceBounds.toRect());
    selection.value = ScreenshotRect.fromRect(normalizedSelection);
    workspaceScale.value = presentation.workspaceScale;
    stage.value = ScreenshotSessionStage.annotating;
    // Native selection now hands Flutter one prepared display immediately, then hydrates any other
    // displays intersecting the chosen rect in the background. That keeps the reveal path fast
    // without regressing multi-display exports that still need the remaining pixels later.
    unawaited(_ensureSelectionSnapshotsReady(normalizedSelection));
    await WidgetsBinding.instance.endOfFrame;
    await WoxApi.instance.onShow(traceId);

    if (presentation.presentedByPlatform) {
      await ScreenshotPlatformBridge.instance.revealPreparedCaptureWorkspace();
    } else {
      final bounds = virtualBoundsRect;
      await windowManager.setBounds(bounds.topLeft, bounds.size);
      await windowManager.setAlwaysOnTop(true);
      await windowManager.show();
      await windowManager.focus();
    }

    // Native selection now stays on-screen until Flutter already holds the final annotation frame.
    // That removes the visible "loading / resize / repaint" gap that used to appear after mouse-up.
    await WidgetsBinding.instance.endOfFrame;
    await ScreenshotPlatformBridge.instance.dismissNativeSelectionOverlays();
    return _sessionFutureOrCancelled();
  }

  Future<void> _prepareNewSession(String traceId) async {
    _clearMacOSPreparationState();
    _disposeDecodedImages();
    _captureSessionRevision += 1;
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
    // The launcher query box can still hold primary focus from the action that started screenshot
    // capture. If that stale focus survives into the screenshot workspace, launcher-side IME and
    // focus listeners wake back up behind the overlay and can cancel the session before annotation
    // begins. Clear the launcher focus up front so the screenshot view becomes the only focus owner.
    FocusManager.instance.primaryFocus?.unfocus();
    launcherController.queryBoxFocusNode.unfocus();
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
  }

  Future<void> cancelSession(String traceId, {String reason = 'unspecified'}) async {
    await _hideScreenshotWindowBeforeFinish(traceId);
    await _finishSession(traceId, CaptureScreenshotResult.cancelled(), ScreenshotSessionStage.cancelled, windowAlreadyHidden: true, reason: reason);
  }

  Future<void> failSession(String traceId, {required String errorCode, required String errorMessage}) async {
    await _hideScreenshotWindowBeforeFinish(traceId);
    await _finishSession(
      traceId,
      CaptureScreenshotResult.failed(errorCode: errorCode, errorMessage: errorMessage),
      ScreenshotSessionStage.failed,
      windowAlreadyHidden: true,
      reason: 'failure:$errorCode',
    );
  }

  Future<void> confirmSelection(String traceId) async {
    final currentSelection = selectionRect;
    if (currentSelection == null || currentSelection.width < 1 || currentSelection.height < 1) {
      return;
    }

    stage.value = ScreenshotSessionStage.exporting;
    try {
      await _hideScreenshotWindowBeforeFinish(traceId);
      await _ensureSelectionSnapshotsReady(currentSelection);

      // Screenshot completion used to push full PNG/base64 payloads back through the websocket
      // bridge. The backend now preallocates the export path inside woxDataDirectory so Flutter can
      // write the final PNG there and immediately hand the same file to the platform clipboard code.
      final activeRequest = _activeRequest;
      if (activeRequest == null || activeRequest.exportFilePath.isEmpty) {
        throw StateError('Screenshot export file path is missing');
      }

      final screenshotPath = await _writeSelectionPngFile(
        exportFilePath: activeRequest.exportFilePath,
        selection: currentSelection,
        snapshots: displaySnapshots.toList(),
        annotationsToPaint: annotations.toList(),
      );

      var clipboardWriteSucceeded = true;
      String? clipboardWarningMessage;
      if (activeRequest.output == 'clipboard') {
        try {
          await ScreenshotPlatformBridge.instance.writeClipboardImageFile(filePath: screenshotPath);
        } catch (e) {
          // Clipboard rejection should not discard a screenshot file that was already exported.
          // Returning a completed session with warning fields lets Go notify the user about the
          // degraded clipboard path while keeping the saved PNG available.
          clipboardWriteSucceeded = false;
          clipboardWarningMessage = e.toString();
          Logger.instance.warn(traceId, 'Screenshot exported but clipboard write failed: $clipboardWarningMessage');
        }
      }

      final result = CaptureScreenshotResult.completed(
        selectionRect: currentSelection,
        screenshotPath: screenshotPath,
        clipboardWriteSucceeded: clipboardWriteSucceeded,
        clipboardWarningMessage: clipboardWarningMessage,
      );
      await _finishSession(traceId, result, ScreenshotSessionStage.done, restoreVisibility: false, windowAlreadyHidden: true);
    } catch (e) {
      Logger.instance.error(traceId, 'Failed to export screenshot: $e');
      await failSession(traceId, errorCode: 'export_failed', errorMessage: e.toString());
    }
  }

  Future<void> startScrollingCapture(String traceId) async {
    final currentSelection = selectionRect;
    if (currentSelection == null || currentSelection.width < 1 || currentSelection.height < 1) {
      return;
    }

    stage.value = ScreenshotSessionStage.scrolling;
    _disposeScrollingCaptureFrames();
    try {
      await WidgetsBinding.instance.endOfFrame;
      await _appendScrollingCaptureFrame(traceId, currentSelection);
      if (Platform.isMacOS && scrollingCaptureFrames.isNotEmpty) {
        final controlsBounds = _calculateScrollingControlsBounds(currentSelection);
        _scrollingCaptureControlsBounds = controlsBounds;
        await ScreenshotPlatformBridge.instance.beginScrollingCaptureOverlay(
          workspaceBounds: ScreenshotRect.fromRect(virtualBoundsRect),
          selection: ScreenshotRect.fromRect(currentSelection),
          controlsBounds: ScreenshotRect.fromRect(controlsBounds),
        );
        isNativeScrollingCaptureOverlay.value = true;
        _scrollingCaptureWheelSubscription?.cancel();
        _scrollingCaptureWheelSubscription = ScreenshotPlatformBridge.instance.scrollingCaptureWheelEvents().listen((_) {
          final selectionForRefresh = selectionRect;
          if (selectionForRefresh != null) {
            _scheduleScrollingCaptureFrame(traceId, selectionForRefresh);
          }
        });
        // Do not poll screenshots while idle. Polling appended frames before the user scrolled and
        // made the preview grow on its own; scrolling capture should advance only after real wheel
        // input because that is the only signal that the underlying page may have moved.
      }
    } catch (e) {
      Logger.instance.error(traceId, 'Failed to start scrolling screenshot: $e');
      await failSession(traceId, errorCode: 'scrolling_start_failed', errorMessage: e.toString());
    }
  }

  Future<void> handleScrollingCaptureWheel(String traceId, double scrollDeltaY) async {
    if (stage.value != ScreenshotSessionStage.scrolling) {
      return;
    }

    final currentSelection = selectionRect;
    if (currentSelection == null || currentSelection.width < 1 || currentSelection.height < 1) {
      return;
    }

    try {
      // Scrolling mode must not warp the cursor. Forward only the user's wheel delta at the current
      // pointer location, then refresh the stitched preview after a short settle window so rapid
      // wheel gestures do not trigger full-desktop capture on every native scroll tick.
      await ScreenshotPlatformBridge.instance.scrollMouse(deltaY: _scrollDeltaToWheelSteps(scrollDeltaY));
      _scheduleScrollingCaptureFrame(traceId, currentSelection);
    } catch (e) {
      Logger.instance.error(traceId, 'Failed to update scrolling screenshot: $e');
    }
  }

  void _scheduleScrollingCaptureFrame(String traceId, Rect selection) {
    _pendingScrollingCaptureSelection = selection;
    if (_scrollingCaptureFrameDebounce != null) {
      return;
    }

    _scrollingCaptureFrameDebounce = Timer(_scrollingCaptureSettleDelay, () {
      _scrollingCaptureFrameDebounce = null;
      if (stage.value != ScreenshotSessionStage.scrolling) {
        _pendingScrollingCaptureSelection = null;
        return;
      }
      if (isScrollingCaptureUpdating.value) {
        final queuedSelection = _pendingScrollingCaptureSelection;
        if (queuedSelection != null) {
          _scheduleScrollingCaptureFrame(traceId, queuedSelection);
        }
        return;
      }

      final selectionForCapture = _pendingScrollingCaptureSelection;
      _pendingScrollingCaptureSelection = null;
      if (selectionForCapture == null) {
        return;
      }

      // Use throttling instead of debounce for wheel-driven capture. Debounce waited until scrolling
      // stopped, which allowed a single captured pair to be separated by more than one viewport and
      // caused poor overlap matches; throttling records intermediate frames while staying bounded.
      unawaited(
        _appendScrollingCaptureFrame(traceId, selectionForCapture).whenComplete(() {
          final queuedSelection = _pendingScrollingCaptureSelection;
          if (queuedSelection != null && stage.value == ScreenshotSessionStage.scrolling) {
            _scheduleScrollingCaptureFrame(traceId, queuedSelection);
          }
        }),
      );
    });
  }

  Rect _calculateScrollingControlsBounds(Rect selection) {
    final bounds = virtualBoundsRect;
    final rightAvailableWidth = math.max(0.0, bounds.right - selection.right - 44);
    final leftAvailableWidth = math.max(0.0, selection.left - bounds.left - 44);
    final maxAvailableWidth = math.max(rightAvailableWidth, leftAvailableWidth);
    final previewSize = _calculateScrollingPreviewRenderSize(selection: selection, maxWidth: maxAvailableWidth, maxHeight: math.min(selection.height, 520.0));
    final controlsWidth = math.max(previewSize.width, _scrollingCaptureToolbarMinWidth);
    final controlsHeight = previewSize.height + 72;
    final useRightSide = selection.right + 20 + controlsWidth <= bounds.right - 24 || rightAvailableWidth >= leftAvailableWidth;
    final left = useRightSide ? selection.right + 20 : math.max(bounds.left + 24, selection.left - controlsWidth - 20);
    final top = selection.top.clamp(bounds.top + 24, math.max(bounds.top + 24, bounds.bottom - controlsHeight - 24)).toDouble();

    // The macOS scrolling overlay moves Flutter into a compact preview/toolbox panel while a native
    // mouse-transparent overlay dims only the outside of the selected region. Keeping this geometry
    // in the controller lets the preview width follow the stitched image aspect ratio instead of a
    // fixed hard-coded side panel.
    return Rect.fromLTWH(left, top, controlsWidth, controlsHeight);
  }

  Size _calculateScrollingPreviewRenderSize({required Rect selection, required double maxWidth, required double maxHeight}) {
    final totalHeight = scrollingCaptureFrames.fold<int>(0, (total, frame) => total + frame.visibleHeight);
    final contentWidth = scrollingCaptureFrames.isEmpty ? selection.width : scrollingCaptureFrames.first.pixelWidth.toDouble();
    final contentHeight = scrollingCaptureFrames.isEmpty || totalHeight <= 0 ? selection.height : totalHeight.toDouble();
    final safeMaxWidth = math.max(1.0, maxWidth);
    final safeMaxHeight = math.max(1.0, maxHeight);
    final scale = math.min(safeMaxWidth / math.max(1.0, contentWidth), safeMaxHeight / math.max(1.0, contentHeight));
    return Size(math.max(1.0, contentWidth * scale), math.max(1.0, contentHeight * scale));
  }

  Future<void> _syncNativeScrollingControlsBounds(String traceId, Rect selection) async {
    if (!Platform.isMacOS || !isNativeScrollingCaptureOverlay.value) {
      return;
    }

    final nextBounds = _calculateScrollingControlsBounds(selection);
    final previousBounds = _scrollingCaptureControlsBounds;
    if (previousBounds != null &&
        (previousBounds.left - nextBounds.left).abs() < 1 &&
        (previousBounds.top - nextBounds.top).abs() < 1 &&
        (previousBounds.width - nextBounds.width).abs() < 1 &&
        (previousBounds.height - nextBounds.height).abs() < 1) {
      return;
    }

    _scrollingCaptureControlsBounds = nextBounds;
    try {
      // The stitched image becomes narrower as more vertical content is added. Resizing the compact
      // Flutter preview window after each accepted frame keeps the native side panel wrapped to the
      // image instead of leaving the old first-frame panel width visible as a gray gutter.
      await windowManager.setBounds(nextBounds.topLeft, nextBounds.size);
    } catch (e) {
      Logger.instance.warn(traceId, 'Failed to resize scrolling screenshot preview window: $e');
    }
  }

  Future<void> confirmScrollingSelection(String traceId) async {
    final currentSelection = selectionRect;
    if (currentSelection == null || currentSelection.width < 1 || currentSelection.height < 1) {
      return;
    }

    if (scrollingCaptureFrames.isEmpty) {
      await _appendScrollingCaptureFrame(traceId, currentSelection);
    }

    stage.value = ScreenshotSessionStage.exporting;
    try {
      await _hideScreenshotWindowBeforeFinish(traceId);

      final activeRequest = _activeRequest;
      if (activeRequest == null || activeRequest.exportFilePath.isEmpty) {
        throw StateError('Screenshot export file path is missing');
      }

      final screenshotPath = await _writeScrollingSelectionPngFile(exportFilePath: activeRequest.exportFilePath, frames: scrollingCaptureFrames.toList());

      var clipboardWriteSucceeded = true;
      String? clipboardWarningMessage;
      if (activeRequest.output == 'clipboard') {
        try {
          await ScreenshotPlatformBridge.instance.writeClipboardImageFile(filePath: screenshotPath);
        } catch (e) {
          clipboardWriteSucceeded = false;
          clipboardWarningMessage = e.toString();
          Logger.instance.warn(traceId, 'Scrolling screenshot exported but clipboard write failed: $clipboardWarningMessage');
        }
      }

      final result = CaptureScreenshotResult.completed(
        selectionRect: currentSelection,
        screenshotPath: screenshotPath,
        clipboardWriteSucceeded: clipboardWriteSucceeded,
        clipboardWarningMessage: clipboardWarningMessage,
      );
      await _finishSession(traceId, result, ScreenshotSessionStage.done, restoreVisibility: false, windowAlreadyHidden: true);
    } catch (e) {
      Logger.instance.error(traceId, 'Failed to export scrolling screenshot: $e');
      await failSession(traceId, errorCode: 'scrolling_export_failed', errorMessage: e.toString());
    }
  }

  Future<String> _writeSelectionPngFile({
    required String exportFilePath,
    required Rect selection,
    required List<DisplaySnapshot> snapshots,
    required List<ScreenshotAnnotation> annotationsToPaint,
  }) async {
    final rendered = await _renderSelectionImage(selection: selection, snapshots: snapshots, annotationsToPaint: annotationsToPaint);
    final exportFile = File(exportFilePath);
    await exportFile.parent.create(recursive: true);
    await exportFile.writeAsBytes(rendered.pngBytes, flush: true);
    return exportFile.path;
  }

  Future<void> _appendScrollingCaptureFrame(String traceId, Rect selection) async {
    if (scrollingCaptureFrames.length >= _scrollingCaptureMaxFrames) {
      return;
    }
    if (isScrollingCaptureUpdating.value) {
      return;
    }

    isScrollingCaptureUpdating.value = true;
    try {
      final nextFrame = await _captureScrollingSelectionFrame(selection);
      if (scrollingCaptureFrames.isNotEmpty) {
        final overlap = _findScrollingOverlap(scrollingCaptureFrames.last, nextFrame);
        if (overlap.isDuplicate) {
          nextFrame.dispose();
          return;
        }

        if (!overlap.isReliable) {
          // Unreliable overlap used to append the whole frame, which made live previews repeat the
          // same viewport whenever the page had dynamic content or a wheel refresh captured before
          // scrolling had settled. Dropping the uncertain frame keeps the long image monotonic; the
          // next wheel capture can still append once a stable overlap is visible.
          nextFrame.dispose();
          Logger.instance.warn(traceId, 'Scrolling screenshot overlap was not reliable; dropped frame to avoid repeated stitched content');
          return;
        }

        nextFrame.cropTop = overlap.overlapRows;
        nextFrame.seamFeatherRows = math.min(_scrollingCaptureSeamFeatherRows, math.min(nextFrame.cropTop, nextFrame.visibleHeight)).toInt();
        if (nextFrame.visibleHeight <= 0) {
          nextFrame.dispose();
          return;
        }
      }

      scrollingCaptureFrames.add(nextFrame);
      await _syncNativeScrollingControlsBounds(traceId, selection);
    } finally {
      isScrollingCaptureUpdating.value = false;
    }
  }

  double _scrollDeltaToWheelSteps(double scrollDeltaY) {
    final direction = scrollDeltaY >= 0 ? 1.0 : -1.0;
    final magnitude = (scrollDeltaY.abs() / 60).clamp(1.0, _scrollingCaptureWheelSteps);
    return direction * magnitude;
  }

  Future<String> _writeScrollingSelectionPngFile({required String exportFilePath, required List<ScrollingCapturePreviewFrame> frames}) async {
    final pngBytes = await _encodeScrollingFrames(frames);
    final exportFile = File(exportFilePath);
    await exportFile.parent.create(recursive: true);
    await exportFile.writeAsBytes(pngBytes, flush: true);
    return exportFile.path;
  }

  Future<ScrollingCapturePreviewFrame> _captureScrollingSelectionFrame(Rect selection) async {
    final rawSnapshots = await ScreenshotPlatformBridge.instance.captureAllDisplays();
    if (rawSnapshots.isEmpty) {
      throw StateError('No display snapshots returned while scrolling');
    }

    final nativeWorkspaceBounds = _activeNativeWorkspaceBounds ?? _calculateUnionRect(rawSnapshots.map((snapshot) => snapshot.logicalBounds.toRect()).toList());
    final normalizedSnapshots = _normalizeSnapshotsForWorkspace(
      rawSnapshots,
      nativeWorkspaceBounds: nativeWorkspaceBounds,
      workspaceBounds: virtualBoundsRect,
      workspaceScale: workspaceScale.value,
    );
    final decodedImages = await _decodeSnapshotImages(normalizedSnapshots.where((snapshot) => !snapshot.logicalBounds.toRect().intersect(selection).isEmpty).toList());

    try {
      final composed = await _composeSelectionImage(
        selection: selection,
        snapshots: normalizedSnapshots,
        annotationsToPaint: const <ScreenshotAnnotation>[],
        decodedImages: decodedImages,
      );
      final byteData = await composed.image.toByteData(format: ui.ImageByteFormat.rawRgba);
      if (byteData == null) {
        composed.image.dispose();
        throw StateError('Failed to inspect scrolling screenshot frame');
      }

      return ScrollingCapturePreviewFrame(
        image: composed.image,
        rgbaBytes: byteData.buffer.asUint8List(byteData.offsetInBytes, byteData.lengthInBytes),
        pixelWidth: composed.pixelWidth,
        pixelHeight: composed.pixelHeight,
      );
    } finally {
      for (final image in decodedImages.values) {
        image.dispose();
      }
    }
  }

  Future<Map<String, ui.Image>> _decodeSnapshotImages(List<DisplaySnapshot> snapshots) async {
    final decodedImages = <String, ui.Image>{};
    try {
      for (final snapshot in snapshots) {
        if (!snapshot.hasImageBytes) {
          continue;
        }

        final codec = await ui.instantiateImageCodec(snapshot.imageBytes);
        final frame = await codec.getNextFrame();
        decodedImages[snapshot.displayId] = frame.image;
      }
      return decodedImages;
    } catch (_) {
      for (final image in decodedImages.values) {
        image.dispose();
      }
      rethrow;
    }
  }

  _ScrollingCaptureOverlap _findScrollingOverlap(ScrollingCapturePreviewFrame previous, ScrollingCapturePreviewFrame next) {
    if (previous.pixelWidth != next.pixelWidth || previous.pixelHeight != next.pixelHeight) {
      return const _ScrollingCaptureOverlap(overlapRows: 0, averageDifference: double.infinity, isReliable: false, isDuplicate: false);
    }

    final columns = _scrollingOverlapComparisonColumns(previous, next);
    final sameViewportDifference = _averageSameViewportDifference(previous, next, columns);
    if (sameViewportDifference <= _scrollingCaptureDuplicateThreshold) {
      return _ScrollingCaptureOverlap(overlapRows: next.pixelHeight, averageDifference: sameViewportDifference, isReliable: true, isDuplicate: true);
    }

    final maxOverlap = math.max(1, (next.pixelHeight * 0.97).floor());
    final minOverlap = math.min(maxOverlap, math.max(24, (next.pixelHeight * 0.08).floor()));
    final coarseStep = math.max(2, (next.pixelHeight / 100).round());
    var best = _findBestScrollingOverlapInRange(previous, next, columns, minOverlap, maxOverlap, coarseStep);
    final refineMin = math.max(minOverlap, best.overlapRows - coarseStep);
    final refineMax = math.min(maxOverlap, best.overlapRows + coarseStep);
    best = _findBestScrollingOverlapInRange(previous, next, columns, refineMin, refineMax, 1);

    final isReliable = best.averageDifference <= _scrollingCaptureOverlapThreshold;
    final isDuplicate = isReliable && best.overlapRows >= next.pixelHeight * 0.94;
    return _ScrollingCaptureOverlap(overlapRows: isReliable ? best.overlapRows : 0, averageDifference: best.averageDifference, isReliable: isReliable, isDuplicate: isDuplicate);
  }

  _ScrollingCaptureOverlapCandidate _findBestScrollingOverlapInRange(
    ScrollingCapturePreviewFrame previous,
    ScrollingCapturePreviewFrame next,
    List<int> columns,
    int minOverlap,
    int maxOverlap,
    int step,
  ) {
    var bestOverlap = minOverlap;
    var bestDifference = double.infinity;

    for (var overlapRows = maxOverlap; overlapRows >= minOverlap; overlapRows -= step) {
      final difference = _averageOverlapDifference(previous, next, overlapRows, columns);
      if (difference < bestDifference) {
        bestDifference = difference;
        bestOverlap = overlapRows;
      }
    }

    return _ScrollingCaptureOverlapCandidate(overlapRows: bestOverlap, averageDifference: bestDifference);
  }

  List<int> _scrollingOverlapComparisonColumns(ScrollingCapturePreviewFrame previous, ScrollingCapturePreviewFrame next) {
    final startX = (next.pixelWidth * 0.06).floor();
    final endX = math.max(startX + 1, (next.pixelWidth * 0.94).ceil());
    final candidateStepX = math.max(1, ((endX - startX) / 72).ceil());
    final rowStep = math.max(1, (next.pixelHeight / 24).round());
    final movingColumns = <int>[];

    for (var x = startX; x < endX; x += candidateStepX) {
      var motion = 0.0;
      var texture = 0.0;
      var sampleCount = 0;
      for (var y = 0; y < next.pixelHeight; y += rowStep) {
        final previousLuma = _lumaAt(previous, x, y);
        final nextLuma = _lumaAt(next, x, y);
        motion += (previousLuma - nextLuma).abs();
        if (y >= rowStep) {
          texture += (previousLuma - _lumaAt(previous, x, y - rowStep)).abs();
          texture += (nextLuma - _lumaAt(next, x, y - rowStep)).abs();
        }
        sampleCount += 1;
      }

      final averageMotion = sampleCount == 0 ? 0.0 : motion / sampleCount;
      final averageTexture = sampleCount <= 1 ? 0.0 : texture / ((sampleCount - 1) * 2);
      if (averageMotion >= 9 && averageTexture >= 2.5) {
        movingColumns.add(x);
      }
    }

    if (movingColumns.length >= 12) {
      return _limitScrollingComparisonColumns(movingColumns, 48);
    }

    // Fixed sidebars and blank gutters are common in long screenshots. When motion detection cannot
    // find enough useful columns, fall back to a centered textured sample instead of the full width so
    // static app chrome does not dominate the vertical registration score.
    final fallbackColumns = <int>[];
    final fallbackStart = (next.pixelWidth * 0.18).floor();
    final fallbackEnd = math.max(fallbackStart + 1, (next.pixelWidth * 0.82).ceil());
    final fallbackStep = math.max(1, ((fallbackEnd - fallbackStart) / 48).ceil());
    for (var x = fallbackStart; x < fallbackEnd; x += fallbackStep) {
      fallbackColumns.add(x);
    }
    return fallbackColumns;
  }

  List<int> _limitScrollingComparisonColumns(List<int> columns, int maxColumns) {
    if (columns.length <= maxColumns) {
      return columns;
    }

    final step = columns.length / maxColumns;
    return List<int>.generate(maxColumns, (index) => columns[(index * step).floor().clamp(0, columns.length - 1).toInt()]);
  }

  double _averageSameViewportDifference(ScrollingCapturePreviewFrame previous, ScrollingCapturePreviewFrame next, List<int> columns) {
    final sampleStepY = math.max(1, (next.pixelHeight / 72).round());
    var totalDifference = 0.0;
    var sampleCount = 0;

    for (var y = 0; y < next.pixelHeight; y += sampleStepY) {
      for (final x in columns) {
        totalDifference += math.min((_lumaAt(previous, x, y) - _lumaAt(next, x, y)).abs(), 72.0);
        sampleCount += 1;
      }
    }

    if (sampleCount == 0) {
      return double.infinity;
    }
    return totalDifference / sampleCount;
  }

  double _averageOverlapDifference(ScrollingCapturePreviewFrame previous, ScrollingCapturePreviewFrame next, int overlapRows, List<int> columns) {
    final sampleStepY = math.max(1, (overlapRows / 64).round());
    var totalDifference = 0.0;
    var sampleCount = 0;

    // Compare only columns that look like moving content, and use luma/edge differences with capped
    // outliers. That makes the matcher less sensitive to fixed sidebars, large blank areas, lazy-load
    // placeholders, and live counters while keeping the search cheap enough for preview updates.
    for (var y = 0; y < overlapRows; y += sampleStepY) {
      final previousY = previous.pixelHeight - overlapRows + y;
      for (final x in columns) {
        final previousLuma = _lumaAt(previous, x, previousY);
        final nextLuma = _lumaAt(next, x, y);
        final previousEdge = previousY > 0 ? (previousLuma - _lumaAt(previous, x, previousY - 1)).abs() : 0.0;
        final nextEdge = y > 0 ? (nextLuma - _lumaAt(next, x, y - 1)).abs() : 0.0;
        totalDifference += math.min((previousLuma - nextLuma).abs(), 72.0);
        totalDifference += math.min((previousEdge - nextEdge).abs(), 48.0) * 0.35;
        sampleCount += 1;
      }
    }

    if (sampleCount == 0) {
      return double.infinity;
    }
    return totalDifference / sampleCount;
  }

  double _lumaAt(ScrollingCapturePreviewFrame frame, int x, int y) {
    final safeX = x.clamp(0, frame.pixelWidth - 1).toInt();
    final safeY = y.clamp(0, frame.pixelHeight - 1).toInt();
    final offset = _rgbaOffset(frame.pixelWidth, safeX, safeY);
    return frame.rgbaBytes[offset] * 0.299 + frame.rgbaBytes[offset + 1] * 0.587 + frame.rgbaBytes[offset + 2] * 0.114;
  }

  int _rgbaOffset(int width, int x, int y) {
    return (y * width + x) * 4;
  }

  Future<Uint8List> _encodeScrollingFrames(List<ScrollingCapturePreviewFrame> frames) async {
    if (frames.isEmpty) {
      throw StateError('No scrolling screenshot frames were captured');
    }

    final width = frames.first.pixelWidth;
    final height = frames.fold<int>(0, (total, frame) => total + frame.visibleHeight);
    final recorder = ui.PictureRecorder();
    final canvas = Canvas(recorder);
    final paint = Paint();
    var y = 0.0;

    for (final frame in frames) {
      final visibleHeight = frame.visibleHeight;
      if (visibleHeight <= 0) {
        continue;
      }

      _paintScrollingFrame(canvas: canvas, frame: frame, destinationY: y, destinationWidth: frame.pixelWidth.toDouble(), destinationHeight: visibleHeight.toDouble(), paint: paint);
      y += visibleHeight;
    }

    final picture = recorder.endRecording();
    final image = await picture.toImage(width, height);
    try {
      final byteData = await image.toByteData(format: ui.ImageByteFormat.png);
      if (byteData == null) {
        throw StateError('Failed to encode scrolling screenshot');
      }
      return byteData.buffer.asUint8List(byteData.offsetInBytes, byteData.lengthInBytes);
    } finally {
      image.dispose();
    }
  }

  void _paintScrollingFrame({
    required Canvas canvas,
    required ScrollingCapturePreviewFrame frame,
    required double destinationY,
    required double destinationWidth,
    required double destinationHeight,
    required Paint paint,
  }) {
    canvas.drawImageRect(
      frame.image,
      Rect.fromLTWH(0, frame.cropTop.toDouble(), frame.pixelWidth.toDouble(), frame.visibleHeight.toDouble()),
      Rect.fromLTWH(0, destinationY, destinationWidth, destinationHeight),
      paint,
    );

    final featherRows = math.min(frame.seamFeatherRows, math.min(frame.cropTop, frame.visibleHeight)).toInt();
    if (destinationY <= 0 || featherRows <= 0) {
      return;
    }

    final featherHeight = destinationHeight * featherRows / frame.visibleHeight;
    if (featherHeight <= 0 || destinationY < featherHeight) {
      return;
    }

    // Hard cuts expose small overlap errors as horizontal lines. Paint the last overlapped rows of
    // the new frame back over the previous frame with a vertical alpha ramp so the seam transitions
    // through real shared pixels instead of a single hard boundary.
    final destinationRect = Rect.fromLTWH(0, destinationY - featherHeight, destinationWidth, featherHeight);
    canvas.saveLayer(destinationRect, Paint());
    canvas.drawImageRect(frame.image, Rect.fromLTWH(0, (frame.cropTop - featherRows).toDouble(), frame.pixelWidth.toDouble(), featherRows.toDouble()), destinationRect, paint);
    canvas.drawRect(
      destinationRect,
      Paint()
        ..blendMode = BlendMode.dstIn
        ..shader = ui.Gradient.linear(destinationRect.topLeft, destinationRect.bottomLeft, const [Color(0x00000000), Color(0xFF000000)]),
    );
    canvas.restore();
  }

  Future<void> _finishSession(
    String traceId,
    CaptureScreenshotResult result,
    ScreenshotSessionStage finalStage, {
    bool restoreVisibility = true,
    bool windowAlreadyHidden = false,
    String reason = 'unspecified',
  }) async {
    final completer = _sessionCompleter;
    if (completer == null || completer.isCompleted) {
      return;
    }

    stage.value = finalStage;
    await _restoreWindowState(traceId, restoreVisibility: restoreVisibility, windowAlreadyHidden: windowAlreadyHidden);
    _resetSessionState();
    completer.complete(result);
    _sessionCompleter = null;
  }

  Future<void> _restoreWindowState(String traceId, {bool restoreVisibility = true, bool windowAlreadyHidden = false}) async {
    final savedState = _savedWindowState;
    if (savedState == null) {
      Logger.instance.warn(traceId, 'Screenshot restore skipped because no saved window state is available');
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

    if (savedState.wasVisible && restoreVisibility) {
      await windowManager.show();
      await windowManager.focus();
      await WoxApi.instance.onShow(traceId);
      if (savedState.wasInSettingView) {
        Get.find<WoxSettingController>().settingFocusNode.requestFocus();
      } else {
        launcherController.focusQueryBox(selectAll: true);
      }
    } else {
      if (!windowAlreadyHidden) {
        // Screenshot completion should leave Wox hidden. The previous restore path always tried to
        // show the launcher again before the session reset, which made the finished capture linger
        // on-screen and briefly re-opened Wox after the user had already confirmed the export.
        await windowManager.hide();
        await WoxApi.instance.onHide(traceId);
      }
    }
  }

  Future<void> _hideScreenshotWindowBeforeFinish(String traceId) async {
    // Finishing a screenshot changes the reused Wox window back from fullscreen capture bounds to
    // the saved launcher bounds. Hide first so cancel/failure/confirm do not visibly shrink the
    // capture surface before the normal restore path decides whether to show the launcher again.
    final isVisible = await windowManager.isVisible();
    if (!isVisible) {
      return;
    }

    await windowManager.hide();
    await WoxApi.instance.onHide(traceId);
  }

  void _resetSessionState() {
    _savedWindowState = null;
    _activeRequest = null;
    _clearMacOSPreparationState();
    _disposeScrollingCaptureFrames();
    isScrollingCaptureUpdating.value = false;
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

  void _disposeScrollingCaptureFrames() {
    _scrollingCaptureFrameDebounce?.cancel();
    _scrollingCaptureFrameDebounce = null;
    _pendingScrollingCaptureSelection = null;
    _scrollingCaptureWheelSubscription?.cancel();
    _scrollingCaptureWheelSubscription = null;
    _scrollingCaptureControlsBounds = null;
    isNativeScrollingCaptureOverlay.value = false;
    for (final frame in scrollingCaptureFrames) {
      frame.dispose();
    }
    scrollingCaptureFrames.clear();
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

  List<DisplaySnapshot> _mergeHydratedSnapshotBytes(List<DisplaySnapshot> snapshots) {
    return snapshots.map((snapshot) {
      final hydrated = _hydratedRawSnapshots[snapshot.displayId];
      if (hydrated == null || !hydrated.hasImageBytes || snapshot.imageBytesBase64 == hydrated.imageBytesBase64) {
        return snapshot;
      }

      // macOS native selection now prewarms only the displays that are likely to be shown next.
      // Merge hydrated bytes back into the normalized snapshot list by display id so the visible
      // workspace and later export both reuse the deferred payloads without rebuilding geometry.
      return snapshot.copyWith(imageBytesBase64: hydrated.imageBytesBase64);
    }).toList();
  }

  DisplaySnapshot _rawSnapshotForDisplayId(String displayId) {
    for (final snapshot in _pendingRawSnapshots) {
      if (snapshot.displayId == displayId) {
        return snapshot;
      }
    }

    throw StateError('Display snapshot $displayId is not available');
  }

  Future<DisplaySnapshot> _ensureRawSnapshotHydrated(String displayId) {
    final hydrated = _hydratedRawSnapshots[displayId];
    if (hydrated != null && hydrated.hasImageBytes) {
      return Future<DisplaySnapshot>.value(hydrated);
    }

    final existingTask = _rawSnapshotHydrationTasks[displayId];
    if (existingTask != null) {
      return existingTask;
    }

    final sessionRevision = _captureSessionRevision;
    final rawSnapshot = _rawSnapshotForDisplayId(displayId);
    late final Future<DisplaySnapshot> hydrationTask;
    hydrationTask = () async {
      final loadedSnapshots = await ScreenshotPlatformBridge.instance.loadDisplaySnapshots([displayId]);
      if (loadedSnapshots.isEmpty) {
        throw StateError('Display snapshot $displayId could not be hydrated');
      }

      final loadedSnapshot = loadedSnapshots.first;
      final hydratedSnapshot = rawSnapshot.copyWith(
        logicalBounds: loadedSnapshot.logicalBounds,
        pixelBounds: loadedSnapshot.pixelBounds,
        scale: loadedSnapshot.scale,
        rotation: loadedSnapshot.rotation,
        imageBytesBase64: loadedSnapshot.imageBytesBase64,
      );

      if (sessionRevision == _captureSessionRevision && _sessionCompleter != null && !_sessionCompleter!.isCompleted) {
        _hydratedRawSnapshots[displayId] = hydratedSnapshot;
      }
      return hydratedSnapshot;
    }().whenComplete(() {
      if (_rawSnapshotHydrationTasks[displayId] == hydrationTask) {
        _rawSnapshotHydrationTasks.remove(displayId);
      }
    });

    _rawSnapshotHydrationTasks[displayId] = hydrationTask;
    return hydrationTask;
  }

  Future<List<DisplaySnapshot>> _hydrateRawSnapshotBatch(List<String> displayIds) async {
    if (displayIds.isEmpty) {
      return const <DisplaySnapshot>[];
    }

    final requestedDisplayIds = displayIds.toSet().toList();
    final pendingDisplayIds = <String>[];
    final resolvedSnapshots = <String, DisplaySnapshot>{};
    for (final displayId in requestedDisplayIds) {
      final hydratedSnapshot = _hydratedRawSnapshots[displayId];
      if (hydratedSnapshot != null && hydratedSnapshot.hasImageBytes) {
        resolvedSnapshots[displayId] = hydratedSnapshot;
        continue;
      }

      pendingDisplayIds.add(displayId);
    }

    if (pendingDisplayIds.isNotEmpty) {
      final sessionRevision = _captureSessionRevision;
      final loadedSnapshots = await ScreenshotPlatformBridge.instance.loadDisplaySnapshots(pendingDisplayIds);
      final loadedSnapshotMap = <String, DisplaySnapshot>{};
      for (final loadedSnapshot in loadedSnapshots) {
        loadedSnapshotMap[loadedSnapshot.displayId] = loadedSnapshot;
      }

      // The original hydration path called the native bridge once per monitor. Batch-loading keeps
      // the metadata-first startup useful on Windows and Linux by collapsing those repeated method
      // channel round-trips into one payload fetch while still updating the per-display cache.
      for (final displayId in pendingDisplayIds) {
        final loadedSnapshot = loadedSnapshotMap[displayId];
        if (loadedSnapshot == null) {
          throw StateError('Display snapshot $displayId could not be hydrated');
        }

        final rawSnapshot = _rawSnapshotForDisplayId(displayId);
        final hydratedSnapshot = rawSnapshot.copyWith(
          logicalBounds: loadedSnapshot.logicalBounds,
          pixelBounds: loadedSnapshot.pixelBounds,
          scale: loadedSnapshot.scale,
          rotation: loadedSnapshot.rotation,
          imageBytesBase64: loadedSnapshot.imageBytesBase64,
        );

        resolvedSnapshots[displayId] = hydratedSnapshot;
        if (sessionRevision == _captureSessionRevision && _sessionCompleter != null && !_sessionCompleter!.isCompleted) {
          _hydratedRawSnapshots[displayId] = hydratedSnapshot;
        }
      }
    }

    return displayIds.map((displayId) {
      final hydratedSnapshot = _hydratedRawSnapshots[displayId] ?? resolvedSnapshots[displayId];
      if (hydratedSnapshot == null) {
        throw StateError('Display snapshot $displayId could not be resolved');
      }
      return hydratedSnapshot;
    }).toList();
  }

  Future<List<DisplaySnapshot>> _hydrateRawSnapshots(List<DisplaySnapshot> rawSnapshots) async {
    if (rawSnapshots.isEmpty) {
      return rawSnapshots;
    }

    _pendingRawSnapshots = rawSnapshots;
    return _hydrateRawSnapshotBatch(rawSnapshots.map((snapshot) => snapshot.displayId).toList());
  }

  Future<void> _ensureSelectionSnapshotsReady(Rect selection) async {
    final snapshotsNeedingHydration =
        displaySnapshots.where((snapshot) {
          return !snapshot.hasImageBytes && !snapshot.logicalBounds.toRect().intersect(selection).isEmpty;
        }).toList();
    if (snapshotsNeedingHydration.isEmpty) {
      return;
    }

    final hydratedSnapshots = await _hydrateRawSnapshotBatch(snapshotsNeedingHydration.map((snapshot) => snapshot.displayId).toList());
    for (final hydratedSnapshot in hydratedSnapshots) {
      DisplaySnapshot? currentSnapshot;
      for (final snapshot in displaySnapshots) {
        if (snapshot.displayId == hydratedSnapshot.displayId) {
          currentSnapshot = snapshot;
          break;
        }
      }
      if (currentSnapshot == null) {
        continue;
      }

      await _ensureDisplayDecoded(currentSnapshot.copyWith(imageBytesBase64: hydratedSnapshot.imageBytesBase64));
    }

    if (_sessionCompleter == null || _sessionCompleter!.isCompleted) {
      return;
    }

    final mergedSnapshots = _mergeHydratedSnapshotBytes(displaySnapshots.toList());
    displaySnapshots.assignAll(mergedSnapshots);
    if (_preparedSnapshots != null) {
      _preparedSnapshots = _mergeHydratedSnapshotBytes(_preparedSnapshots!);
    }
  }

  Future<void> _handleMacOSSelectionDisplayHint(String traceId, ScreenshotSelectionDisplayHint hint) async {
    if (!_acceptSelectionDisplayHints || _pendingRawSnapshots.isEmpty) {
      return;
    }

    final nativeWorkspaceBounds = _nativeWorkspaceBounds;
    final displayBounds = hint.displayBounds.toRect();
    if (nativeWorkspaceBounds != null && nativeWorkspaceBounds.intersect(displayBounds).isEmpty) {
      return;
    }

    DisplaySnapshot? targetDisplay;
    for (final snapshot in _pendingRawSnapshots) {
      if (snapshot.displayId == hint.displayId) {
        targetDisplay = snapshot;
        break;
      }
    }
    if (targetDisplay == null) {
      return;
    }

    if (_preparedDisplayId == targetDisplay.displayId && _preparedPresentation != null && _preparedSnapshots != null) {
      return;
    }

    try {
      await _prepareMacOSDisplayForAnnotation(targetDisplay);
    } catch (error) {
      Logger.instance.error(traceId, 'Failed to prewarm macOS screenshot workspace for ${targetDisplay.displayId}: $error');
    }
  }

  Future<void> _prepareMacOSDisplayForAnnotation(DisplaySnapshot targetDisplay) async {
    final targetBounds = targetDisplay.logicalBounds.toRect();
    if (_preparedDisplayId == targetDisplay.displayId && _preparedDisplayBounds == targetBounds && _preparedPresentation != null && _preparedSnapshots != null) {
      return;
    }

    final revision = ++_preparedDisplayRevision;
    final presentation = await ScreenshotPlatformBridge.instance.prepareCaptureWorkspace(ScreenshotRect.fromRect(targetBounds));
    await _ensureRawSnapshotHydrated(targetDisplay.displayId);
    var normalizedSnapshots = _normalizeSnapshotsForWorkspace(
      _pendingRawSnapshots,
      nativeWorkspaceBounds: targetBounds,
      workspaceBounds: presentation.workspaceBounds.toRect(),
      workspaceScale: presentation.workspaceScale,
    );
    normalizedSnapshots = _mergeHydratedSnapshotBytes(normalizedSnapshots);

    DisplaySnapshot? preparedTargetSnapshot;
    for (final snapshot in normalizedSnapshots) {
      if (snapshot.displayId == targetDisplay.displayId) {
        preparedTargetSnapshot = snapshot;
        break;
      }
    }
    if (preparedTargetSnapshot == null) {
      throw StateError('Prepared macOS display snapshot is missing for ${targetDisplay.displayId}');
    }

    await _ensureDisplayDecoded(preparedTargetSnapshot);

    if (revision != _preparedDisplayRevision || _sessionCompleter == null || _sessionCompleter!.isCompleted) {
      return;
    }

    // Mouse-up used to be the point where Flutter first learned which display would host the
    // annotation editor, so the first visible frame still had to decode and lay out the new
    // backdrop. Warming the hidden workspace here makes the reveal path effectively frame-only.
    _preparedDisplayId = targetDisplay.displayId;
    _preparedDisplayBounds = targetBounds;
    _preparedPresentation = presentation;
    _preparedSnapshots = normalizedSnapshots;
    displaySnapshots.assignAll(normalizedSnapshots);
    virtualBounds.value = ScreenshotRect.fromRect(presentation.workspaceBounds.toRect());
    workspaceScale.value = presentation.workspaceScale;
    stage.value = ScreenshotSessionStage.selecting;
  }

  void _clearMacOSPreparationState() {
    _acceptSelectionDisplayHints = false;
    _selectionDisplayHintSubscription?.cancel();
    _selectionDisplayHintSubscription = null;
    _pendingRawSnapshots = const <DisplaySnapshot>[];
    _hydratedRawSnapshots.clear();
    _rawSnapshotHydrationTasks.clear();
    _nativeWorkspaceBounds = null;
    _activeNativeWorkspaceBounds = null;
    _preparedDisplayId = null;
    _preparedDisplayBounds = null;
    _preparedPresentation = null;
    _preparedSnapshots = null;
    _preparedDisplayRevision = 0;
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
    // Existing text annotations now enter inline editing on a single click. Selecting all text made
    // the visual jump obvious and did not feel like editing the same rendered label, so the caret is
    // placed at the end to keep the editing state visually continuous with the painted text.
    textDraftController.selection = TextSelection.collapsed(offset: initialText.length);
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

  /// Finds the display whose bounds best contain the given selection rect. The macOS native drag
  /// overlay can span multiple monitors, but the Flutter annotation editor still has to collapse to
  /// one target display so the handoff can prepare and reveal a single stable workspace.
  DisplaySnapshot _findDisplaySnapshotForSelection(Rect selection, List<DisplaySnapshot> snapshots) {
    final center = selection.center;
    for (final snapshot in snapshots) {
      if (snapshot.logicalBounds.toRect().contains(center)) {
        return snapshot;
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
    return best ?? snapshots.first;
  }

  Future<_RenderedSelectionImage> _renderSelectionImage({
    required Rect selection,
    required List<DisplaySnapshot> snapshots,
    required List<ScreenshotAnnotation> annotationsToPaint,
  }) async {
    final composed = await _composeSelectionImage(selection: selection, snapshots: snapshots, annotationsToPaint: annotationsToPaint);
    try {
      final byteData = await composed.image.toByteData(format: ui.ImageByteFormat.png);
      if (byteData == null) {
        throw StateError('Failed to encode exported screenshot');
      }

      final pngBytes = byteData.buffer.asUint8List();
      return _RenderedSelectionImage(pngBytes: pngBytes);
    } finally {
      composed.image.dispose();
    }
  }

  Future<_ComposedSelectionImage> _composeSelectionImage({
    required Rect selection,
    required List<DisplaySnapshot> snapshots,
    required List<ScreenshotAnnotation> annotationsToPaint,
    Map<String, ui.Image>? decodedImages,
  }) async {
    final imageLookup = decodedImages ?? _decodedImages;
    final exportSlices = <_DisplayExportSlice>[];
    for (final snapshot in snapshots) {
      final logicalRect = snapshot.logicalBounds.toRect();
      final intersection = logicalRect.intersect(selection);
      if (intersection.isEmpty) {
        continue;
      }

      final decodedImage = imageLookup[snapshot.displayId];
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
    return _ComposedSelectionImage(image: image, pixelWidth: pixelUnion.width.ceil(), pixelHeight: pixelUnion.height.ceil());
  }

  Future<void> _decodeDisplayImages(List<DisplaySnapshot> snapshots) async {
    _disposeDecodedImages();
    for (final snapshot in snapshots) {
      if (!snapshot.hasImageBytes) {
        continue;
      }
      await _ensureDisplayDecoded(snapshot);
    }
  }

  Future<void> _ensureDisplayDecoded(DisplaySnapshot snapshot) {
    if (!snapshot.hasImageBytes) {
      return Future<void>.value();
    }

    final decodedImage = _decodedImages[snapshot.displayId];
    if (decodedImage != null) {
      return Future<void>.value();
    }

    final existingTask = _displayDecodeTasks[snapshot.displayId];
    if (existingTask != null) {
      return existingTask;
    }

    final decodeTask = _decodeDisplayImage(snapshot).whenComplete(() {
      _displayDecodeTasks.remove(snapshot.displayId);
    });
    _displayDecodeTasks[snapshot.displayId] = decodeTask;
    return decodeTask;
  }

  Future<void> _decodeDisplayImage(DisplaySnapshot snapshot) async {
    final codec = await ui.instantiateImageCodec(snapshot.imageBytes);
    final frame = await codec.getNextFrame();
    final previousImage = _decodedImages[snapshot.displayId];
    if (previousImage != null) {
      previousImage.dispose();
    }
    _decodedImages[snapshot.displayId] = frame.image;
  }

  void _disposeDecodedImages() {
    for (final image in _decodedImages.values) {
      image.dispose();
    }
    _decodedImages.clear();
    _displayDecodeTasks.clear();
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
    _clearMacOSPreparationState();
    _disposeScrollingCaptureFrames();
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
  const _RenderedSelectionImage({required this.pngBytes});

  final Uint8List pngBytes;
}

class _ComposedSelectionImage {
  const _ComposedSelectionImage({required this.image, required this.pixelWidth, required this.pixelHeight});

  final ui.Image image;
  final int pixelWidth;
  final int pixelHeight;
}

class ScrollingCapturePreviewFrame {
  ScrollingCapturePreviewFrame({required this.image, required this.rgbaBytes, required this.pixelWidth, required this.pixelHeight});

  final ui.Image image;
  final Uint8List rgbaBytes;
  final int pixelWidth;
  final int pixelHeight;
  int cropTop = 0;
  int seamFeatherRows = 0;

  int get visibleHeight => math.max(0, pixelHeight - cropTop);

  void dispose() {
    image.dispose();
  }
}

class _ScrollingCaptureOverlap {
  const _ScrollingCaptureOverlap({required this.overlapRows, required this.averageDifference, required this.isReliable, required this.isDuplicate});

  final int overlapRows;
  final double averageDifference;
  final bool isReliable;
  final bool isDuplicate;
}

class _ScrollingCaptureOverlapCandidate {
  const _ScrollingCaptureOverlapCandidate({required this.overlapRows, required this.averageDifference});

  final int overlapRows;
  final double averageDifference;
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

// Text annotations now support selection, drag, and inline editing. Sharing the exact same text
// style between painter and editor keeps the caret overlay visually merged with the rendered label
// instead of swapping between two slightly different text appearances.
TextStyle buildScreenshotTextStyle({required Color color, required double fontSize}) {
  return TextStyle(color: color, fontSize: fontSize, fontWeight: FontWeight.w600, shadows: const [Shadow(color: Color(0xAA000000), blurRadius: 4)]);
}

TextPainter? buildScreenshotTextPainter(ScreenshotAnnotation annotation) {
  final start = annotation.start;
  final text = annotation.text;
  if (start == null || text == null || text.isEmpty) {
    return null;
  }

  return TextPainter(text: TextSpan(text: text, style: buildScreenshotTextStyle(color: annotation.color, fontSize: annotation.fontSize)), textDirection: TextDirection.ltr)
    ..layout(maxWidth: 480);
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

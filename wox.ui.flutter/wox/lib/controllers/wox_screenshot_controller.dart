import 'dart:async';
import 'dart:io';
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
  final Map<String, Future<void>> _displayDecodeTasks = <String, Future<void>>{};
  List<DisplaySnapshot> _pendingRawSnapshots = const <DisplaySnapshot>[];
  final Map<String, DisplaySnapshot> _hydratedRawSnapshots = <String, DisplaySnapshot>{};
  final Map<String, Future<DisplaySnapshot>> _rawSnapshotHydrationTasks = <String, Future<DisplaySnapshot>>{};
  Rect? _nativeWorkspaceBounds;
  String? _preparedDisplayId;
  Rect? _preparedDisplayBounds;
  ScreenshotWorkspacePresentation? _preparedPresentation;
  List<DisplaySnapshot>? _preparedSnapshots;
  StreamSubscription<ScreenshotSelectionDisplayHint>? _selectionDisplayHintSubscription;
  bool _acceptSelectionDisplayHints = false;
  int _preparedDisplayRevision = 0;
  int _captureSessionRevision = 0;
  Completer<CaptureScreenshotResult>? _sessionCompleter;
  _SavedScreenshotWindowState? _savedWindowState;
  CaptureScreenshotRequest? _activeRequest;

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
    await _finishSession(traceId, CaptureScreenshotResult.cancelled(), ScreenshotSessionStage.cancelled, reason: reason);
  }

  Future<void> failSession(String traceId, {required String errorCode, required String errorMessage}) async {
    await _finishSession(traceId, CaptureScreenshotResult.failed(errorCode: errorCode, errorMessage: errorMessage), ScreenshotSessionStage.failed, reason: 'failure:$errorCode');
  }

  Future<void> confirmSelection(String traceId) async {
    final currentSelection = selectionRect;
    if (currentSelection == null || currentSelection.width < 1 || currentSelection.height < 1) {
      return;
    }

    stage.value = ScreenshotSessionStage.exporting;
    try {
      await _hideCompletedScreenshotWindow(traceId);
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

  Future<void> _hideCompletedScreenshotWindow(String traceId) async {
    // Confirmed captures no longer need to keep the editor visible while PNG export and restore
    // bookkeeping finish. Hiding the window up front removes the perceived lag between clicking
    // confirm and the screenshot UI disappearing, while the export still runs against in-memory data.
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

import 'dart:convert';
import 'dart:io';
import 'dart:ui' as ui;

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:get/get.dart';
import 'package:integration_test/integration_test.dart';
import 'package:wox/controllers/wox_screenshot_controller.dart';
import 'package:wox/entity/screenshot_session.dart';
import 'package:wox/modules/launcher/views/wox_launcher_view.dart';
import 'package:wox/modules/screenshot/views/wox_screenshot_view.dart';
import 'package:wox/utils/screenshot/screenshot_platform_bridge.dart';

import 'smoke_test_helper.dart';

class _FakeScreenshotBridge implements ScreenshotPlatformBridge {
  _FakeScreenshotBridge(this._capture, {this.nativeSelection, this.presentation, this.debugState, this.delegateNativePresentation = false});

  final Future<List<DisplaySnapshot>> Function() _capture;
  final Future<ScreenshotNativeSelectionResult> Function(ScreenshotRect nativeWorkspaceBounds)? nativeSelection;
  final Future<ScreenshotWorkspacePresentation> Function(ScreenshotRect nativeWorkspaceBounds)? presentation;
  final Future<Map<String, dynamic>> Function()? debugState;
  final bool delegateNativePresentation;
  final ScreenshotPlatformBridge _delegate = MethodChannelScreenshotPlatformBridge();

  @override
  Future<List<DisplaySnapshot>> captureAllDisplays() => _capture();

  @override
  Future<ScreenshotNativeSelectionResult> selectCaptureRegion(ScreenshotRect nativeWorkspaceBounds) async {
    if (nativeSelection != null) {
      return nativeSelection!(nativeWorkspaceBounds);
    }
    return const ScreenshotNativeSelectionResult(wasHandled: false);
  }

  @override
  Future<ScreenshotWorkspacePresentation> presentCaptureWorkspace(ScreenshotRect nativeWorkspaceBounds) async {
    if (presentation != null) {
      return presentation!(nativeWorkspaceBounds);
    }
    if (delegateNativePresentation) {
      return _delegate.presentCaptureWorkspace(nativeWorkspaceBounds);
    }
    return ScreenshotWorkspacePresentation(workspaceBounds: nativeWorkspaceBounds, workspaceScale: 1, presentedByPlatform: false);
  }

  @override
  Future<void> dismissCaptureWorkspacePresentation() async {
    if (delegateNativePresentation) {
      await _delegate.dismissCaptureWorkspacePresentation();
    }
  }

  @override
  Future<Map<String, dynamic>> debugCaptureWorkspaceState() async {
    if (debugState != null) {
      return debugState!();
    }
    if (delegateNativePresentation) {
      return _delegate.debugCaptureWorkspaceState();
    }
    return <String, dynamic>{};
  }
}

void main() {
  IntegrationTestWidgetsFlutterBinding.ensureInitialized();
  registerLauncherScreenshotSmokeTests();
}

void registerLauncherScreenshotSmokeTests() {
  group('T11: Screenshot Smoke Tests', () {
    testWidgets('T11-01: Screenshot flow exports a non-empty PNG after multi-display selection', (tester) async {
      await launchAndShowLauncher(tester, windowSize: smokeLargeWindowSize);
      final screenshotController = Get.find<WoxScreenshotController>();
      ScreenshotPlatformBridge.setInstanceForTest(
        _FakeScreenshotBridge(() async {
          return [
            await _buildSnapshot('display-a', const Color(0xFF273469), const ScreenshotRect(x: 0, y: 0, width: 400, height: 300)),
            await _buildSnapshot('display-b', const Color(0xFF7A306C), const ScreenshotRect(x: 400, y: 0, width: 400, height: 300)),
          ];
        }),
      );

      Map<String, dynamic>? sessionResult;
      Object? sessionError;
      final sessionFuture = screenshotController
          .startCaptureSession('smoke-success', _defaultRequest())
          .then((value) {
            final json = value.toJson();
            sessionResult = json;
            return json;
          })
          .catchError((error) {
            sessionError = error;
            throw error;
          });
      await pumpUntil(tester, () => find.byKey(screenshotCanvasKey).evaluate().isNotEmpty || sessionResult != null || sessionError != null, timeout: const Duration(seconds: 15));

      expect(sessionError, isNull);
      expect(sessionResult, isNull, reason: 'Screenshot session completed before the workspace became interactive.');
      expect(screenshotController.isSessionActive.value, isTrue);
      expect(screenshotController.virtualBoundsRect.width, equals(800));
      expect(screenshotController.virtualBoundsRect.height, equals(300));
      expect(find.byType(WoxScreenshotView), findsOneWidget);

      final canvasFinder = find.byKey(screenshotCanvasKey);
      expect(canvasFinder, findsOneWidget);
      final canvasOrigin = tester.getTopLeft(canvasFinder);
      await tester.dragFrom(canvasOrigin + const Offset(80, 50), const Offset(280, 150));
      await tester.pump(const Duration(milliseconds: 250));

      final selectionRect = screenshotController.selectionRect;
      expect(selectionRect, isNotNull);
      expect(selectionRect!.width, greaterThan(200));
      expect(selectionRect.height, greaterThan(120));

      // The integration-test host can render the floating toolbar partially outside the
      // hit-testable root once the selection moves near the edge. Drive the controller directly
      // so the smoke test keeps validating the screenshot workflow instead of toolbar hit tests.
      screenshotController.setTool(ScreenshotTool.rect);
      await tester.pump();
      expect(screenshotController.currentTool.value, ScreenshotTool.rect);

      screenshotController.addShapeAnnotation(ScreenshotAnnotationType.rect, Rect.fromLTWH(selectionRect.left + 18, selectionRect.top + 16, 90, 44));
      screenshotController.addShapeAnnotation(ScreenshotAnnotationType.ellipse, Rect.fromLTWH(selectionRect.left + 130, selectionRect.top + 40, 76, 50));
      screenshotController.addArrowAnnotation(selectionRect.topLeft + const Offset(24, 96), selectionRect.topLeft + const Offset(160, 108));
      screenshotController.startTextDraft(selectionRect.topLeft + const Offset(32, 20));
      screenshotController.textDraftController.text = 'Smoke';
      screenshotController.commitTextDraft();
      expect(screenshotController.annotations.length, equals(4));

      screenshotController.undoAnnotation();
      expect(screenshotController.annotations.length, equals(3));
      screenshotController.startTextDraft(selectionRect.topLeft + const Offset(32, 20));
      screenshotController.textDraftController.text = 'Smoke';
      screenshotController.commitTextDraft();
      expect(screenshotController.annotations.length, equals(4));

      await tester.tap(find.byKey(screenshotConfirmKey));
      await tester.pump(const Duration(milliseconds: 250));
      final result = await sessionFuture;

      expect(result['status'], equals('completed'));
      final pngBase64 = result['pngBase64'] as String? ?? '';
      expect(pngBase64, isNotEmpty);
      expect(base64Decode(pngBase64).length, greaterThan(2048));

      await pumpUntil(tester, () => find.byType(WoxLauncherView).evaluate().isNotEmpty, timeout: const Duration(seconds: 15));
      expect(screenshotController.isSessionActive.value, isFalse);
    });

    testWidgets('T11-02: Screenshot cancel restores launcher without exporting', (tester) async {
      await launchAndShowLauncher(tester, windowSize: smokeLargeWindowSize);
      ScreenshotPlatformBridge.setInstanceForTest(
        _FakeScreenshotBridge(() async => [await _buildSnapshot('display-c', const Color(0xFF144552), const ScreenshotRect(x: 0, y: 0, width: 320, height: 240))]),
      );

      Map<String, dynamic>? sessionResult;
      Object? sessionError;
      final sessionFuture = Get.find<WoxScreenshotController>()
          .startCaptureSession('smoke-cancel', _defaultRequest())
          .then((value) {
            final json = value.toJson();
            sessionResult = json;
            return json;
          })
          .catchError((error) {
            sessionError = error;
            throw error;
          });
      await pumpUntil(tester, () => find.byKey(screenshotCancelKey).evaluate().isNotEmpty || sessionResult != null || sessionError != null, timeout: const Duration(seconds: 15));

      expect(sessionError, isNull);
      expect(sessionResult, isNull, reason: 'Screenshot session completed before the cancel path became interactive.');
      expect(find.byType(WoxScreenshotView), findsOneWidget);
      // The integration-test render surface can stay smaller than the virtual desktop window, so
      // the toolbar may render partially outside the hit-testable root even when the cancel action
      // is visible. Trigger the controller directly so the smoke test verifies the cancel path
      // instead of the test harness hit-testing limits.
      await Get.find<WoxScreenshotController>().cancelSession('smoke-cancel-complete');
      await tester.pump(const Duration(milliseconds: 250));
      final result = await sessionFuture;

      expect(result['status'], equals('cancelled'));
      await pumpUntil(tester, () => find.byType(WoxLauncherView).evaluate().isNotEmpty, timeout: const Duration(seconds: 15));
      expect(Get.find<WoxScreenshotController>().isSessionActive.value, isFalse);
    });

    testWidgets('T11-03: Screenshot bridge failure restores launcher and returns failed status', (tester) async {
      await launchAndShowLauncher(tester, windowSize: smokeLargeWindowSize);
      ScreenshotPlatformBridge.setInstanceForTest(
        _FakeScreenshotBridge(() async {
          throw StateError('permission denied');
        }),
      );

      final result = (await Get.find<WoxScreenshotController>().startCaptureSession('smoke-failed', _defaultRequest())).toJson();
      await tester.pump(const Duration(milliseconds: 250));

      expect(result['status'], equals('failed'));
      expect((result['errorMessage'] as String?) ?? '', contains('permission denied'));
      expect(Get.find<WoxScreenshotController>().isSessionActive.value, isFalse);
      expect(find.byType(WoxLauncherView), findsOneWidget);
    });

    testWidgets('T11-04: Screenshot annotation editing updates existing text and keeps the edit bar outside the selection', (tester) async {
      await launchAndShowLauncher(tester, windowSize: smokeLargeWindowSize);
      final screenshotController = Get.find<WoxScreenshotController>();
      ScreenshotPlatformBridge.setInstanceForTest(
        _FakeScreenshotBridge(() async => [await _buildSnapshot('display-d', const Color(0xFF23395B), const ScreenshotRect(x: 0, y: 0, width: 800, height: 600))]),
      );

      final sessionFuture = screenshotController.startCaptureSession('smoke-edit', _defaultRequest());
      await pumpUntil(tester, () => find.byKey(screenshotCanvasKey).evaluate().isNotEmpty, timeout: const Duration(seconds: 15));

      screenshotController.updateSelection(const Rect.fromLTWH(560, 120, 200, 220));
      screenshotController.annotations.add(
        ScreenshotAnnotation(id: 'text-a', type: ScreenshotAnnotationType.text, start: const Offset(590, 180), text: 'Before', color: Color(0xFFFF5B36), fontSize: 20),
      );
      screenshotController.selectAnnotation('text-a');
      await tester.pumpAndSettle();

      final editBarRect = tester.getRect(find.byKey(screenshotEditBarKey));
      expect(editBarRect.right, lessThan(560));

      screenshotController.updateSelectedAnnotationColor(const Color(0xFF4DA3FF));
      screenshotController.updateSelectedTextFontSize(6);
      screenshotController.startTextDraft(const Offset(590, 180), annotationId: 'text-a', initialText: 'Before', fontSize: 26, color: const Color(0xFF4DA3FF));
      screenshotController.textDraftController.text = 'After';
      screenshotController.commitTextDraft();
      await tester.pumpAndSettle();

      final editedAnnotation = screenshotController.annotations.single;
      expect(editedAnnotation.text, equals('After'));
      expect(editedAnnotation.fontSize, equals(26));
      expect(editedAnnotation.color, equals(const Color(0xFF4DA3FF)));

      await tester.tap(find.byKey(screenshotConfirmKey));
      await tester.pump(const Duration(milliseconds: 250));
      final result = (await sessionFuture).toJson();

      expect(result['status'], equals('completed'));
      await pumpUntil(tester, () => find.byType(WoxLauncherView).evaluate().isNotEmpty, timeout: const Duration(seconds: 15));
    });

    testWidgets('T11-05: Screenshot export uses workspaceScale to map the selected area back to native pixels', (tester) async {
      await launchAndShowLauncher(tester, windowSize: smokeLargeWindowSize);
      final screenshotController = Get.find<WoxScreenshotController>();
      ScreenshotPlatformBridge.setInstanceForTest(
        _FakeScreenshotBridge(
          () async => [await _buildSnapshot('display-scaled', const Color(0xFF1F6FEB), const ScreenshotRect(x: 0, y: 0, width: 400, height: 200))],
          presentation: (nativeWorkspaceBounds) async {
            expect(nativeWorkspaceBounds, const ScreenshotRect(x: 0, y: 0, width: 400, height: 200));
            return const ScreenshotWorkspacePresentation(workspaceBounds: ScreenshotRect(x: 0, y: 0, width: 200, height: 100), workspaceScale: 2, presentedByPlatform: false);
          },
        ),
      );

      final sessionFuture = screenshotController.startCaptureSession('smoke-scaled-export', _defaultRequest());
      await pumpUntil(tester, () => find.byKey(screenshotCanvasKey).evaluate().isNotEmpty, timeout: const Duration(seconds: 15));

      expect(screenshotController.virtualBoundsRect, const Rect.fromLTWH(0, 0, 200, 100));

      screenshotController.updateSelection(const Rect.fromLTWH(20, 10, 150, 80));
      await tester.pumpAndSettle();
      await tester.tap(find.byKey(screenshotConfirmKey));
      await tester.pump(const Duration(milliseconds: 250));

      final result = (await sessionFuture).toJson();
      final pngBytes = base64Decode(result['pngBase64'] as String);
      final codec = await ui.instantiateImageCodec(pngBytes);
      final frame = await codec.getNextFrame();

      expect(result['status'], equals('completed'));
      expect(frame.image.width, equals(300));
      expect(frame.image.height, equals(160));

      frame.image.dispose();
      await pumpUntil(tester, () => find.byType(WoxLauncherView).evaluate().isNotEmpty, timeout: const Duration(seconds: 15));
    });

    testWidgets('T11-06: Native screenshot presentation debug state toggles during the session', (tester) async {
      if (!Platform.isMacOS && !Platform.isWindows) {
        return;
      }

      await launchAndShowLauncher(tester, windowSize: smokeLargeWindowSize);
      final screenshotController = Get.find<WoxScreenshotController>();
      final bridge = _FakeScreenshotBridge(
        () async => [await _buildSnapshot('display-native-debug', const Color(0xFF23395B), const ScreenshotRect(x: 0, y: 0, width: 800, height: 600))],
        delegateNativePresentation: true,
      );
      ScreenshotPlatformBridge.setInstanceForTest(bridge);

      final sessionFuture = screenshotController.startCaptureSession('smoke-native-debug', _defaultRequest());
      await pumpUntil(tester, () => find.byKey(screenshotCanvasKey).evaluate().isNotEmpty, timeout: const Duration(seconds: 15));

      final activeDebugState = await bridge.debugCaptureWorkspaceState();
      expect(activeDebugState['isCapturePresentationActive'], isTrue);
      expect((activeDebugState['workspaceScale'] as num?)?.toDouble() ?? 0, greaterThan(0));

      await screenshotController.cancelSession('smoke-native-debug-cancel');
      await tester.pump(const Duration(milliseconds: 250));
      final result = (await sessionFuture).toJson();
      final restoredDebugState = await bridge.debugCaptureWorkspaceState();

      expect(result['status'], equals('cancelled'));
      expect(restoredDebugState['isCapturePresentationActive'], isFalse);
      await pumpUntil(tester, () => find.byType(WoxLauncherView).evaluate().isNotEmpty, timeout: const Duration(seconds: 15));
    });
  });
}

CaptureScreenshotRequest _defaultRequest() {
  return const CaptureScreenshotRequest(sessionId: 'smoke-session', trigger: 'plugin', scope: 'all_displays', output: 'clipboard', tools: ['rect', 'ellipse', 'arrow', 'text']);
}

Future<DisplaySnapshot> _buildSnapshot(String id, Color color, ScreenshotRect logicalBounds) async {
  final recorder = ui.PictureRecorder();
  final canvas = Canvas(recorder);
  final rect = Rect.fromLTWH(0, 0, logicalBounds.width, logicalBounds.height);
  canvas.drawRect(rect, Paint()..color = color);
  canvas.drawRect(rect.deflate(18), Paint()..color = color.withValues(alpha: 0.78));

  final picture = recorder.endRecording();
  final image = await picture.toImage(logicalBounds.width.toInt(), logicalBounds.height.toInt());
  final bytes = await image.toByteData(format: ui.ImageByteFormat.png);
  return DisplaySnapshot(
    displayId: id,
    logicalBounds: logicalBounds,
    pixelBounds: logicalBounds,
    scale: 1,
    rotation: 0,
    imageBytesBase64: base64Encode(bytes!.buffer.asUint8List()),
  );
}

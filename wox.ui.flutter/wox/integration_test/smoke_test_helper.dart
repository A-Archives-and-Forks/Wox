import 'dart:convert';
import 'dart:io';
import 'dart:ui';

import 'package:extended_text_field/extended_text_field.dart';
import 'package:flutter/foundation.dart';
import 'package:flutter/rendering.dart';
import 'package:flutter/services.dart';
import 'package:flutter/widgets.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:get/get.dart';
import 'package:uuid/v4.dart';
import 'package:wox/api/wox_api.dart';
import 'package:wox/controllers/wox_launcher_controller.dart';
import 'package:wox/controllers/wox_setting_controller.dart';
import 'package:wox/entity/wox_query.dart';
import 'package:wox/entity/wox_setting.dart';
import 'package:wox/enums/wox_launch_mode_enum.dart';
import 'package:wox/enums/wox_position_type_enum.dart';
import 'package:wox/enums/wox_start_page_enum.dart';
import 'package:wox/main.dart' as app;
import 'package:wox/modules/launcher/views/wox_launcher_view.dart';
import 'package:wox/modules/setting/views/wox_setting_view.dart';
import 'package:wox/utils/wox_http_util.dart';
import 'package:wox/utils/wox_setting_util.dart';
import 'package:wox/utils/wox_theme_util.dart';
import 'package:wox/utils/test/wox_test_config.dart';
import 'package:wox/utils/windows/system_input.dart';
import 'package:wox/utils/windows/system_input_interface.dart';
import 'package:wox/utils/windows/window_manager.dart';

const Size smokeLargeWindowSize = Size(1200, 900);
const double smokeWindowPositionTolerance = 1;
const int _windowsAltVirtualKey = 18;
const int _windowsAltScanCode = 56;

class ScreenWorkArea {
  const ScreenWorkArea({required this.x, required this.y, required this.width, required this.height});

  final int x;
  final int y;
  final int width;
  final int height;

  factory ScreenWorkArea.fromJson(Map<String, dynamic> json) {
    return ScreenWorkArea(x: json['x'] as int, y: json['y'] as int, width: json['width'] as int, height: json['height'] as int);
  }
}

void resetSmokeAppState() {
  Get.reset();
}

void registerLauncherTestCleanup(WidgetTester tester, WoxLauncherController controller) {
  addTearDown(() async {
    controller.resetForIntegrationTest();

    // Directly hide the window instead of using controller.hideApp(), which
    // involves async API calls (onSetting, onHide) that may hang during
    // tearDown. The next test calls Get.reset() so full cleanup isn't needed.
    if (await windowManager.isVisible()) {
      await windowManager.hide();
    }
  });
}

Future<WoxLauncherController> launchLauncherApp(WidgetTester tester) async {
  resetSmokeAppState();
  app.main([WoxTestConfig.serverPort.toString(), '-1', 'true']);

  final launcherFinder = find.byType(WoxLauncherView);
  await pumpUntil(tester, () => launcherFinder.evaluate().isNotEmpty, timeout: const Duration(seconds: 30));
  expect(launcherFinder, findsOneWidget);

  final controller = Get.find<WoxLauncherController>();
  registerLauncherTestCleanup(tester, controller);
  return controller;
}

Future<WoxLauncherController> launchAndShowLauncher(WidgetTester tester, {Size? windowSize}) async {
  final controller = await launchLauncherApp(tester);

  await updateSettingDirect('LaunchMode', WoxLaunchModeEnum.WOX_LAUNCH_MODE_FRESH.code);
  await updateSettingDirect('StartPage', WoxStartPageEnum.WOX_START_PAGE_BLANK.code);
  await triggerBackendShowApp(tester);
  // pumpAndSettle is safe here because this runs during launcher setup,
  // before any text input that would start cursor blink timers.
  await tester.pumpAndSettle();

  if (windowSize != null) {
    await ensureWindowSize(tester, windowSize);
  }

  expect(await windowManager.isVisible(), isTrue);
  return controller;
}

Future<void> hideLauncherIfVisible(WidgetTester tester, WoxLauncherController controller) async {
  if (!await windowManager.isVisible()) {
    return;
  }

  await hideLauncherByEscape(tester, controller);
}

Future<void> waitForWindowVisibility(WidgetTester tester, bool visible, {Duration timeout = const Duration(seconds: 30)}) async {
  final deadline = DateTime.now().add(timeout);
  while (DateTime.now().isBefore(deadline)) {
    if (await windowManager.isVisible() == visible) {
      return;
    }
    // When waiting for the window to become hidden, use Future.delayed instead
    // of tester.pump() because macOS stops delivering vsync signals for hidden
    // windows, causing pump() to block indefinitely.
    if (!visible) {
      await Future.delayed(const Duration(milliseconds: 200));
    } else {
      await tester.pump(const Duration(milliseconds: 200));
    }
  }

  fail('Window visibility did not become $visible within $timeout.');
}

Future<void> updateSettingDirect(String key, String value) async {
  final traceId = const UuidV4().generate();
  await WoxApi.instance.updateSetting(traceId, key, value);
  await WoxSettingUtil.instance.loadSetting(traceId);
}

Future<void> saveLastWindowPosition(int x, int y) async {
  await WoxApi.instance.saveWindowPosition(const UuidV4().generate(), x, y);
}

Future<void> triggerBackendShowApp(WidgetTester tester) async {
  await WoxHttpUtil.instance.postData<String>(const UuidV4().generate(), '/show', null);
  await waitForWindowVisibility(tester, true);
}

Future<void> triggerTestQueryHotkey(WidgetTester tester, String query, {bool isSilentExecution = false}) async {
  await WoxHttpUtil.instance.postData<String>(const UuidV4().generate(), '/test/trigger/query_hotkey', {'Query': query, 'IsSilentExecution': isSilentExecution});
  await waitForWindowVisibility(tester, true);
}

Future<void> triggerTestOpenSetting(WidgetTester tester, {String path = '', String param = ''}) async {
  await WoxHttpUtil.instance.postData<String>(const UuidV4().generate(), '/test/trigger/open_setting', {'Path': path, 'Param': param});
  await tester.pump(const Duration(milliseconds: 500));
}

Future<void> triggerTestSelectionHotkey(WidgetTester tester, {required String type, String text = '', List<String> filePaths = const []}) async {
  await WoxHttpUtil.instance.postData<String>(const UuidV4().generate(), '/test/trigger/selection_hotkey', {'Type': type, 'Text': text, 'FilePaths': filePaths});
  await waitForWindowVisibility(tester, true);
}

Future<void> triggerTestTrayQuery(
  WidgetTester tester, {
  required String query,
  bool showQueryBox = true,
  int width = 0,
  int x = 200,
  int y = 40,
  int rectWidth = 40,
  int rectHeight = 40,
}) async {
  await WoxHttpUtil.instance.postData<String>(const UuidV4().generate(), '/test/trigger/tray_query', {
    'Query': query,
    'Width': width,
    'ShowQueryBox': showQueryBox,
    'Rect': {'X': x, 'Y': y, 'Width': rectWidth, 'Height': rectHeight},
  });
  await waitForWindowVisibility(tester, true);
}

Future<Offset> waitForWindowPosition(
  WidgetTester tester,
  Offset expected, {
  double tolerance = smokeWindowPositionTolerance,
  Duration timeout = const Duration(seconds: 30),
}) async {
  final deadline = DateTime.now().add(timeout);
  while (DateTime.now().isBefore(deadline)) {
    await tester.pump(const Duration(milliseconds: 200));
    final actual = await windowManager.getPosition();
    if (isOffsetClose(actual, expected, tolerance: tolerance)) {
      return actual;
    }
  }

  final actual = await windowManager.getPosition();
  fail('Window position did not reach expected $expected within $timeout. Actual: $actual');
}

bool isOffsetClose(Offset actual, Offset expected, {double tolerance = smokeWindowPositionTolerance}) {
  return (actual.dx - expected.dx).abs() <= tolerance && (actual.dy - expected.dy).abs() <= tolerance;
}

Future<ScreenWorkArea> getMouseScreenWorkArea() async {
  if (Platform.isMacOS) {
    const script = '''
ObjC.import("Cocoa");
var mouseLoc = \$.NSEvent.mouseLocation;
var screens = \$.NSScreen.screens;
var result = "{}";
for (var i = 0; i < screens.count; i++) {
    var screen = screens.objectAtIndex(i);
    var frame = screen.frame;
    var mx = mouseLoc.x;
    var my = mouseLoc.y;
    if (mx >= frame.origin.x && mx <= frame.origin.x + frame.size.width &&
        my >= frame.origin.y && my <= frame.origin.y + frame.size.height) {
        var vf = screen.visibleFrame;
        result = JSON.stringify({
            x: Math.round(vf.origin.x),
            y: Math.round(frame.size.height - vf.size.height),
            width: Math.round(vf.size.width),
            height: Math.round(vf.size.height)
        });
        break;
    }
}
result;
''';

    final result = await Process.run('osascript', ['-l', 'JavaScript', '-e', script]);
    if (result.exitCode != 0) {
      throw StateError('Failed to query mouse screen work area on macOS: ${result.stderr}');
    }
    return ScreenWorkArea.fromJson(jsonDecode((result.stdout as String).trim()) as Map<String, dynamic>);
  }

  if (Platform.isWindows) {
    const script = r'''
Add-Type -TypeDefinition @"
using System;
using System.Runtime.InteropServices;

public static class NativeMethods {
    [StructLayout(LayoutKind.Sequential)]
    public struct POINT {
        public int X;
        public int Y;
    }

    [StructLayout(LayoutKind.Sequential)]
    public struct RECT {
        public int Left;
        public int Top;
        public int Right;
        public int Bottom;
    }

    [StructLayout(LayoutKind.Sequential, CharSet = CharSet.Auto)]
    public struct MONITORINFO {
        public int cbSize;
        public RECT rcMonitor;
        public RECT rcWork;
        public uint dwFlags;
    }

    [DllImport("user32.dll")]
    public static extern bool GetCursorPos(out POINT lpPoint);

    [DllImport("user32.dll")]
    public static extern IntPtr MonitorFromPoint(POINT pt, uint dwFlags);

    [DllImport("user32.dll", CharSet = CharSet.Auto)]
    public static extern bool GetMonitorInfo(IntPtr hMonitor, ref MONITORINFO lpmi);

    [DllImport("Shcore.dll")]
    public static extern int GetDpiForMonitor(IntPtr hmonitor, int dpiType, out uint dpiX, out uint dpiY);
}
"@

$cursor = New-Object NativeMethods+POINT
[NativeMethods]::GetCursorPos([ref]$cursor) | Out-Null

$monitor = [NativeMethods]::MonitorFromPoint($cursor, 2)
$monitorInfo = New-Object NativeMethods+MONITORINFO
$monitorInfo.cbSize = [Runtime.InteropServices.Marshal]::SizeOf([type][NativeMethods+MONITORINFO])
[NativeMethods]::GetMonitorInfo($monitor, [ref]$monitorInfo) | Out-Null

$dpiX = [uint32]96
$dpiY = [uint32]96
try {
    [NativeMethods]::GetDpiForMonitor($monitor, 0, [ref]$dpiX, [ref]$dpiY) | Out-Null
} catch {
    $dpiX = [uint32]96
    $dpiY = [uint32]96
}

$scale = $dpiX / 96.0
[pscustomobject]@{
    x = [int]($monitorInfo.rcWork.Left / $scale)
    y = [int]($monitorInfo.rcWork.Top / $scale)
    width = [int](($monitorInfo.rcWork.Right - $monitorInfo.rcWork.Left) / $scale)
    height = [int](($monitorInfo.rcWork.Bottom - $monitorInfo.rcWork.Top) / $scale)
} | ConvertTo-Json -Compress
''';

    final result = await Process.run('powershell.exe', ['-NoLogo', '-NoProfile', '-NonInteractive', '-Command', script]);
    if (result.exitCode != 0) {
      throw StateError('Failed to query mouse screen work area: ${result.stderr}');
    }

    return ScreenWorkArea.fromJson(jsonDecode((result.stdout as String).trim()) as Map<String, dynamic>);
  }

  throw UnsupportedError('Unsupported platform: ${Platform.operatingSystem}');
}

Future<Offset> getExpectedMouseScreenCenterTopLeft() async {
  final screen = await getMouseScreenWorkArea();
  final setting = WoxSettingUtil.instance.currentSetting;
  final theme = WoxThemeUtil.instance.currentTheme.value;

  final queryBoxHeight = 55 + theme.appPaddingTop + theme.appPaddingBottom;
  final resultItemHeight = 50 + theme.resultItemPaddingTop + theme.resultItemPaddingBottom;
  final resultListViewHeight = resultItemHeight * (setting.maxResultCount == 0 ? 10 : setting.maxResultCount);
  final resultContainerHeight = resultListViewHeight + theme.resultContainerPaddingTop + theme.resultContainerPaddingBottom;
  final maxWindowHeight = queryBoxHeight + resultContainerHeight + 40;

  final expectedX = screen.x + (screen.width - setting.appWidth) ~/ 2;
  final expectedY = screen.y + (screen.height - maxWindowHeight) ~/ 2;
  return Offset(expectedX.toDouble(), expectedY.toDouble());
}

Future<void> ensureWindowSize(WidgetTester tester, Size size) async {
  await windowManager.setSize(size);
  // pumpAndSettle is safe here because this is called during launcher setup,
  // before any text input that would start cursor blink timers.
  await tester.pumpAndSettle();
}

Future<void> hideLauncherByEscape(WidgetTester tester, WoxLauncherController controller, {Duration timeout = const Duration(seconds: 30)}) async {
  await systemInput.keyPress(SystemInputKeys.escape);

  final escapeDeliveryDeadline = DateTime.now().add(const Duration(seconds: 2));
  while (DateTime.now().isBefore(escapeDeliveryDeadline)) {
    await tester.pump(const Duration(milliseconds: 100));
    if (!await windowManager.isVisible()) {
      return;
    }
  }

  // Windows desktop smoke does not deliver Escape reliably through tester key injection.
  await controller.hideApp(const UuidV4().generate());
  await waitForWindowVisibility(tester, false, timeout: timeout);
}

Future<void> enterQueryAndWaitForResults(WidgetTester tester, WoxLauncherController controller, String query, {Duration timeout = const Duration(seconds: 30)}) async {
  final extendedTextFieldFinder = find.byType(ExtendedTextField);
  expect(extendedTextFieldFinder, findsOneWidget);

  await tester.tap(extendedTextFieldFinder);
  await tester.pump(const Duration(milliseconds: 200));

  tester.testTextInput.enterText(query);
  await tester.pump();

  // Suppress transient overflow errors that occur during the window resize
  // transition when results first appear and the layout hasn't settled yet.
  final oldHandler = FlutterError.onError;
  FlutterError.onError = (details) {
    if (details.exception is FlutterError && details.exception.toString().contains('overflowed')) {
      return;
    }
    oldHandler?.call(details);
  };

  await pumpUntil(tester, () => controller.resultListViewController.items.isNotEmpty || controller.resultGridViewController.items.isNotEmpty, timeout: timeout);

  // Pump a few more frames to let the resize settle before restoring the error handler.
  await tester.pump(const Duration(milliseconds: 500));
  FlutterError.onError = oldHandler;
}

Future<void> sendWindowsKeyboardEvent({required String type, required bool isAltPressed}) async {
  if (!Platform.isWindows) {
    return;
  }

  final data = const StandardMethodCodec().encodeMethodCall(
    MethodCall('onKeyboardEvent', {
      'type': type,
      'keyCode': _windowsAltVirtualKey,
      'scanCode': _windowsAltScanCode,
      'isShiftPressed': false,
      'isControlPressed': false,
      'isAltPressed': isAltPressed,
      'isMetaPressed': false,
    }),
  );

  await ServicesBinding.instance.defaultBinaryMessenger.handlePlatformMessage('com.wox.windows_window_manager', data, (_) {});
}

Future<void> holdQuickSelectModifier(WidgetTester tester, {Duration holdDuration = const Duration(milliseconds: 350)}) async {
  if (Platform.isWindows) {
    await sendWindowsKeyboardEvent(type: 'keydown', isAltPressed: true);
  }

  await tester.sendKeyDownEvent(LogicalKeyboardKey.altLeft);
  await tester.pump(holdDuration);
}

Future<void> releaseQuickSelectModifier(WidgetTester tester) async {
  await tester.sendKeyUpEvent(LogicalKeyboardKey.altLeft);

  if (Platform.isWindows) {
    await sendWindowsKeyboardEvent(type: 'keyup', isAltPressed: false);
  }

  await tester.pump(const Duration(milliseconds: 200));
}

Future<WoxSettingController> openSettings(WidgetTester tester, WoxLauncherController launcherController, String path) async {
  await triggerTestOpenSetting(tester, path: path);

  await pumpUntil(tester, () => launcherController.isInSettingView.value && find.byType(WoxSettingView).evaluate().isNotEmpty, timeout: const Duration(seconds: 30));

  expect(launcherController.isInSettingView.value, isTrue);
  expect(find.byType(WoxSettingView), findsOneWidget);
  return Get.find<WoxSettingController>();
}

Future<void> closeSettings(WidgetTester tester, WoxSettingController settingController, WoxLauncherController launcherController) async {
  final backButtonFinder = find.byKey(const ValueKey('settings-back-button'));
  expect(backButtonFinder, findsOneWidget);
  await tester.tap(backButtonFinder);
  await tester.pump(const Duration(milliseconds: 500));
  await pumpUntil(tester, () => launcherController.isInSettingView.value == false, timeout: const Duration(seconds: 30));
}

Future<void> tapSettingNavItem(WidgetTester tester, String navPath, {Duration timeout = const Duration(seconds: 30)}) async {
  final navItemFinder = find.byKey(ValueKey('settings-nav-$navPath'));
  expect(navItemFinder, findsOneWidget);
  await tester.tap(navItemFinder);
  await tester.pump(const Duration(milliseconds: 500));
  await pumpUntil(tester, () => navItemFinder.evaluate().isNotEmpty, timeout: timeout);
}

Future<void> queryAndWaitForResults(WidgetTester tester, WoxLauncherController controller, String query, {Duration timeout = const Duration(seconds: 30)}) async {
  await enterQueryAndWaitForResults(tester, controller, query, timeout: timeout);
}

Future<void> waitForQueryBoxFocus(WidgetTester tester, WoxLauncherController controller, {Duration timeout = const Duration(seconds: 30)}) async {
  await pumpUntil(tester, () => controller.queryBoxFocusNode.hasFocus, timeout: timeout);
}

Future<void> waitForQueryBoxText(WidgetTester tester, WoxLauncherController controller, String expectedText, {Duration timeout = const Duration(seconds: 30)}) async {
  await pumpUntil(tester, () => controller.queryBoxTextFieldController.text == expectedText, timeout: timeout);
}

Future<void> waitForNoResults(WidgetTester tester, WoxLauncherController controller, {Duration timeout = const Duration(seconds: 30)}) async {
  await pumpUntil(tester, () => controller.resultListViewController.items.isEmpty && controller.resultGridViewController.items.isEmpty, timeout: timeout);
}

Future<void> waitForNoActiveResults(WidgetTester tester, WoxLauncherController controller, {Duration timeout = const Duration(seconds: 30)}) async {
  await pumpUntil(tester, () => controller.activeResultViewController.items.isEmpty, timeout: timeout);
}

Future<void> pumpUntil(WidgetTester tester, bool Function() condition, {required Duration timeout}) async {
  final deadline = DateTime.now().add(timeout);
  while (DateTime.now().isBefore(deadline)) {
    await tester.pump(const Duration(milliseconds: 200));
    if (condition()) {
      return;
    }
  }

  fail('Condition not met within $timeout.');
}

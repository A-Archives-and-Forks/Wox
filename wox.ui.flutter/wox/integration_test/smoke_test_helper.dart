import 'dart:io';

import 'package:extended_text_field/extended_text_field.dart';
import 'package:flutter/services.dart';
import 'package:flutter/widgets.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:get/get.dart';
import 'package:uuid/v4.dart';
import 'package:wox/api/wox_api.dart';
import 'package:wox/controllers/wox_launcher_controller.dart';
import 'package:wox/controllers/wox_setting_controller.dart';
import 'package:wox/entity/wox_query.dart';
import 'package:wox/enums/wox_launch_mode_enum.dart';
import 'package:wox/enums/wox_position_type_enum.dart';
import 'package:wox/enums/wox_start_page_enum.dart';
import 'package:wox/main.dart' as app;
import 'package:wox/modules/launcher/views/wox_launcher_view.dart';
import 'package:wox/modules/setting/views/wox_setting_view.dart';
import 'package:wox/utils/wox_http_util.dart';
import 'package:wox/utils/heartbeat_checker.dart';
import 'package:wox/utils/wox_setting_util.dart';
import 'package:wox/utils/wox_theme_util.dart';
import 'package:wox/utils/wox_websocket_msg_util.dart';
import 'package:wox/utils/test/wox_test_config.dart';
import 'package:wox/utils/windows/window_manager.dart';

const Size smokeLargeWindowSize = Size(1200, 900);
const double smokeWindowPositionTolerance = 1;
const int _windowsAltVirtualKey = 18;
const int _windowsAltScanCode = 56;
const Size _smokeBootstrapWindowSize = Size(800, 600);

class SmokeLaunchResult {
  const SmokeLaunchResult({required this.controller, required this.elapsed});

  final WoxLauncherController controller;
  final Duration elapsed;
}

class ScreenWorkArea {
  const ScreenWorkArea({required this.x, required this.y, required this.width, required this.height});

  final int x;
  final int y;
  final int width;
  final int height;

  factory ScreenWorkArea.fromJson(Map<String, dynamic> json) {
    return ScreenWorkArea(
      x: (json['x'] ?? json['X']) as int,
      y: (json['y'] ?? json['Y']) as int,
      width: (json['width'] ?? json['Width']) as int,
      height: (json['height'] ?? json['Height']) as int,
    );
  }
}

Future<void> resetSmokeAppState() async {
  HeartbeatChecker().init();
  await WoxWebsocketMsgUtil.instance.init();
  Get.reset();
}

void registerLauncherTestCleanup(WidgetTester tester, WoxLauncherController controller) {
  addTearDown(() async {
    await controller.resetForIntegrationTest();

    await restoreSmokeWindowStateForNextTest();

    // Hide the window so the backend resets its visibility state.  Use
    // windowManager.hide() directly — controller.hideApp() involves async API
    // calls that may hang during tearDown.
    if (await windowManager.isVisible()) {
      await windowManager.hide();
    }

    // On Windows, fully unmount the previous app tree so that focus listeners
    // are disposed during teardown instead of surviving until the next key event.
    // This is NOT needed on macOS, and calling pumpWidget during teardown on macOS can cause
    // issues with the hidden window's vsync signals, causing pump() to block indefinitely.  Only do this on Windows
    if (Platform.isWindows) {
      await tester.pumpWidget(const SizedBox.shrink());
      await tester.pump();
    }
  });
}

Future<WoxLauncherController> launchLauncherApp(WidgetTester tester) async {
  // Ensure the window is visible before any pump() call.  On macOS, a hidden
  // window stops delivering vsync signals, which causes pump() to block.
  // The previous test's tearDown hides the window for backend state cleanup.
  if (!await windowManager.isVisible()) {
    await windowManager.show();
  }

  await resetSmokeAppState();
  app.main([WoxTestConfig.serverPort.toString(), '-1', 'true']);

  final launcherFinder = find.byType(WoxLauncherView);
  await pumpUntil(tester, () => launcherFinder.evaluate().isNotEmpty, timeout: const Duration(seconds: 30));
  expect(launcherFinder, findsOneWidget);

  final controller = Get.find<WoxLauncherController>();
  registerLauncherTestCleanup(tester, controller);
  return controller;
}

Future<SmokeLaunchResult> launchLauncherAppAndMeasureStartup(WidgetTester tester, {Duration timeout = const Duration(seconds: 5)}) async {
  if (!await windowManager.isVisible()) {
    await windowManager.show();
  }

  await resetSmokeAppState();

  final stopwatch = Stopwatch()..start();
  app.main([WoxTestConfig.serverPort.toString(), '-1', 'true']);

  final launcherFinder = find.byType(WoxLauncherView);
  await pumpUntil(tester, () => launcherFinder.evaluate().isNotEmpty, timeout: timeout);

  final remaining = timeout - stopwatch.elapsed;
  if (remaining.isNegative) {
    fail('Launcher widget appeared after ${stopwatch.elapsed}, exceeding timeout $timeout.');
  }

  await waitForWindowVisibility(tester, true, timeout: remaining);
  stopwatch.stop();

  expect(launcherFinder, findsOneWidget);
  final controller = Get.find<WoxLauncherController>();
  registerLauncherTestCleanup(tester, controller);
  return SmokeLaunchResult(controller: controller, elapsed: stopwatch.elapsed);
}

Future<WoxLauncherController> launchAndShowLauncher(WidgetTester tester, {Size? windowSize}) async {
  final controller = await launchLauncherApp(tester);

  await updateSettingDirect('LangCode', 'en_US');
  await updateSettingDirect('LaunchMode', WoxLaunchModeEnum.WOX_LAUNCH_MODE_FRESH.code);
  await updateSettingDirect('StartPage', WoxStartPageEnum.WOX_START_PAGE_BLANK.code);
  await triggerBackendShowApp(tester);
  // Use a bounded pump instead of pumpAndSettle because showApp() calls
  // focusQueryBox() which starts the cursor blink timer. The periodic blink
  // keeps scheduling frames, preventing pumpAndSettle from ever settling.
  await tester.pump(const Duration(milliseconds: 500));

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

Future<void> restoreSmokeWindowStateForNextTest() async {
  await updateSettingDirect('ShowPosition', WoxPositionTypeEnum.POSITION_TYPE_MOUSE_SCREEN.code);
  await saveLastWindowPosition(-1, -1);

  if (!Platform.isWindows && !Platform.isMacOS) {
    return;
  }

  final screen = await getMouseScreenWorkArea();
  final position = getCenteredTopLeftForWindowSize(screen, _smokeBootstrapWindowSize);
  await windowManager.setBounds(position, _smokeBootstrapWindowSize);
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

Offset getCenteredTopLeftForWindowSize(ScreenWorkArea screen, Size windowSize) {
  final expectedX = screen.x + ((screen.width - windowSize.width) / 2).round();
  final expectedY = screen.y + ((screen.height - windowSize.height) / 2).round();
  return Offset(expectedX.toDouble(), expectedY.toDouble());
}

bool isOffsetClose(Offset actual, Offset expected, {double tolerance = smokeWindowPositionTolerance}) {
  return (actual.dx - expected.dx).abs() <= tolerance && (actual.dy - expected.dy).abs() <= tolerance;
}

Future<ScreenWorkArea> getMouseScreenWorkArea() async {
  final response = await WoxHttpUtil.instance.getData<Map<String, dynamic>>(const UuidV4().generate(), '/test/screen/mouse');
  return ScreenWorkArea.fromJson(response);
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
  // Do NOT use systemInput.keyPress(escape) here.  Native OS-level key events
  // travel through the macOS event pipeline asynchronously — the KeyUpEvent
  // for Escape can arrive after Get.reset() clears Flutter's HardwareKeyboard
  // state in the next test, triggering a "physical key is not pressed"
  // assertion failure.  Calling hideApp directly is reliable and avoids the
  // async keyboard state mismatch.
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

  await waitForActiveResults(tester, controller, timeout: timeout);

  // Pump a few more frames to let the resize settle before restoring the error handler.
  await tester.pump(const Duration(milliseconds: 500));
  FlutterError.onError = oldHandler;
}

String normalizeSmokeText(String value) {
  return value.trim().toLowerCase();
}

List<WoxQueryResult> getActiveResults(WoxLauncherController controller) {
  return controller.activeResultViewController.items.map((item) => item.value.data).toList();
}

WoxQueryResult expectActiveResult(WoxLauncherController controller) {
  final activeResults = getActiveResults(controller);
  expect(activeResults, isNotEmpty);
  return controller.activeResultViewController.activeItem.data;
}

List<WoxResultAction> findResultActionsByName(WoxQueryResult result, String actionName, {bool exactMatch = false}) {
  final normalizedActionName = normalizeSmokeText(actionName);
  return result.actions.where((action) {
    final normalizedName = normalizeSmokeText(action.name);
    if (exactMatch) {
      return normalizedName == normalizedActionName;
    }
    return normalizedName.contains(normalizedActionName);
  }).toList();
}

WoxResultAction expectResultActionByName(WoxQueryResult result, String actionName, {bool exactMatch = false}) {
  final actions = findResultActionsByName(result, actionName, exactMatch: exactMatch);
  expect(actions, isNotEmpty, reason: 'Expected action "$actionName" in result "${result.title}".');
  return actions.first;
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
    // Keep the Windows bridge in sync while still driving Flutter's keyboard
    // pipeline, because quick select listens through onKeyEvent and
    // HardwareKeyboard.
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
  // Avoid tester.ensureVisible which calls pumpAndSettle (10-min timeout).
  // If the cursor blink timer is still active, pumpAndSettle never settles.
  // The back button is always visible at the bottom of the fixed sidebar.
  await tester.pump();
  await tester.tap(backButtonFinder, warnIfMissed: false);
  await tester.pump(const Duration(milliseconds: 500));

  final fallbackDeadline = DateTime.now().add(const Duration(seconds: 2));
  while (DateTime.now().isBefore(fallbackDeadline)) {
    await tester.pump(const Duration(milliseconds: 200));
    if (!launcherController.isInSettingView.value) {
      return;
    }
  }

  settingController.hideWindow(const UuidV4().generate());
  await pumpUntil(tester, () => launcherController.isInSettingView.value == false, timeout: const Duration(seconds: 30));
}

Future<void> tapSettingNavItem(WidgetTester tester, WoxSettingController settingController, String navPath, {Duration timeout = const Duration(seconds: 30)}) async {
  final navItemFinder = find.byKey(ValueKey('settings-nav-$navPath'));
  expect(navItemFinder, findsOneWidget);
  // Avoid tester.ensureVisible which calls pumpAndSettle (10-min timeout).
  // If the cursor blink timer is still active from the query box, pumpAndSettle
  // will never settle. Nav items are always visible in the fixed sidebar.
  await tester.pump();
  await tester.tap(navItemFinder, warnIfMissed: false);
  await tester.pump(const Duration(milliseconds: 500));
  await pumpUntil(tester, () => settingController.activeNavPath.value == navPath, timeout: timeout);
}

Future<void> queryAndWaitForResults(WidgetTester tester, WoxLauncherController controller, String query, {Duration timeout = const Duration(seconds: 30)}) async {
  await enterQueryAndWaitForResults(tester, controller, query, timeout: timeout);
}

Future<WoxQueryResult> queryAndWaitForActiveResult(WidgetTester tester, WoxLauncherController controller, String query, {Duration timeout = const Duration(seconds: 30)}) async {
  await queryAndWaitForResults(tester, controller, query, timeout: timeout);
  return expectActiveResult(controller);
}

Future<void> waitForActiveResults(WidgetTester tester, WoxLauncherController controller, {Duration timeout = const Duration(seconds: 30)}) async {
  await pumpUntil(tester, () => controller.activeResultViewController.items.isNotEmpty, timeout: timeout);
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

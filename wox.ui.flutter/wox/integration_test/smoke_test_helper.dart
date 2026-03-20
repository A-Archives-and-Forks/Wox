import 'dart:ui';

import 'package:flutter_test/flutter_test.dart';
import 'package:get/get.dart';
import 'package:uuid/v4.dart';
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
import 'package:wox/utils/test/wox_test_config.dart';
import 'package:wox/utils/windows/window_manager.dart';

const Size smokeLargeWindowSize = Size(1200, 900);

void resetSmokeAppState() {
  Get.reset();
}

Future<WoxLauncherController> launchAndShowLauncher(WidgetTester tester, {Size? windowSize}) async {
  resetSmokeAppState();
  app.main([WoxTestConfig.serverPort.toString(), '-1', 'true']);

  final launcherFinder = find.byType(WoxLauncherView);
  await pumpUntil(tester, () => launcherFinder.evaluate().isNotEmpty, timeout: const Duration(seconds: 30));
  expect(launcherFinder, findsOneWidget);

  final controller = Get.find<WoxLauncherController>();
  await controller.showApp(
    const UuidV4().generate(),
    ShowAppParams(
      selectAll: true,
      position: Position(type: WoxPositionTypeEnum.POSITION_TYPE_ACTIVE_SCREEN.code, x: 120, y: 120),
      queryHistories: [],
      launchMode: WoxLaunchModeEnum.WOX_LAUNCH_MODE_FRESH.code,
      startPage: WoxStartPageEnum.WOX_START_PAGE_BLANK.code,
      isQueryFocus: false,
      showQueryBox: true,
    ),
  );
  await tester.pumpAndSettle();

  if (windowSize != null) {
    await ensureWindowSize(tester, windowSize);
  }

  expect(await windowManager.isVisible(), isTrue);
  return controller;
}

Future<void> ensureWindowSize(WidgetTester tester, Size size) async {
  await windowManager.setSize(size);
  await tester.pumpAndSettle();
}

Future<WoxSettingController> openSettings(WidgetTester tester, WoxLauncherController launcherController, String path) async {
  launcherController.openSetting(const UuidV4().generate(), SettingWindowContext(path: path, param: ''));
  await tester.pumpAndSettle();

  expect(launcherController.isInSettingView.value, isTrue);
  expect(find.byType(WoxSettingView), findsOneWidget);
  return Get.find<WoxSettingController>();
}

Future<void> closeSettings(WidgetTester tester, WoxSettingController settingController, WoxLauncherController launcherController) async {
  settingController.hideWindow(const UuidV4().generate());
  await tester.pumpAndSettle();
  expect(launcherController.isInSettingView.value, isFalse);
}

Future<void> queryAndWaitForResults(WidgetTester tester, WoxLauncherController controller, String query, {Duration timeout = const Duration(seconds: 30)}) async {
  controller.queryBoxTextFieldController.text = query;
  controller.onQueryBoxTextChanged(query);
  await tester.pump();

  await pumpUntil(tester, () => controller.resultListViewController.items.isNotEmpty || controller.resultGridViewController.items.isNotEmpty, timeout: timeout);
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

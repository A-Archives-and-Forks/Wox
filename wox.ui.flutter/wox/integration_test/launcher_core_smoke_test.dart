import 'dart:io';

import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:uuid/v4.dart';
import 'package:wox/enums/wox_launch_mode_enum.dart';
import 'package:wox/enums/wox_position_type_enum.dart';
import 'package:wox/enums/wox_query_type_enum.dart';
import 'package:wox/enums/wox_selection_type_enum.dart';
import 'package:wox/modules/launcher/views/wox_launcher_view.dart';
import 'package:wox/modules/setting/views/wox_setting_view.dart';
import 'package:wox/utils/windows/window_manager.dart';

import 'smoke_test_helper.dart';

void registerLauncherCoreSmokeTests() {
  group('P0-SMK: Core Smoke Tests', () {
    testWidgets('P0-SMK-01: Launch main window and verify UI elements', (tester) async {
      final controller = await launchAndShowLauncher(tester);

      expect(find.byType(WoxLauncherView), findsOneWidget);
      expect(await windowManager.isVisible(), isTrue);
      expect(controller.isQueryBoxVisible.value, isTrue);
    });

    testWidgets('P0-SMK-02: ShowPosition mouse_screen centers the launcher on the current screen', (tester) async {
      if (!Platform.isWindows && !Platform.isMacOS) {
        return;
      }

      final controller = await launchLauncherApp(tester);
      await hideLauncherIfVisible(tester, controller);

      await updateSettingDirect('ShowPosition', WoxPositionTypeEnum.POSITION_TYPE_MOUSE_SCREEN.code);
      final expectedPosition = await getExpectedMouseScreenCenterTopLeft();
      await triggerBackendShowApp(tester);

      final actualPosition = await waitForWindowPosition(tester, expectedPosition);
      expect(isOffsetClose(actualPosition, expectedPosition), isTrue);
    });

    testWidgets('P0-SMK-07: ShowPosition last_location restores the saved window coordinates exactly', (tester) async {
      final controller = await launchLauncherApp(tester);
      await hideLauncherIfVisible(tester, controller);

      const expectedPosition = Offset(240, 180);
      await updateSettingDirect('ShowPosition', WoxPositionTypeEnum.POSITION_TYPE_LAST_LOCATION.code);
      await saveLastWindowPosition(expectedPosition.dx.toInt(), expectedPosition.dy.toInt());

      await triggerBackendShowApp(tester);

      final actualPosition = await waitForWindowPosition(tester, expectedPosition);
      expect(isOffsetClose(actualPosition, expectedPosition), isTrue);
    });

    testWidgets('P0-SMK-03: Keyboard navigation works', (tester) async {
      final controller = await launchAndShowLauncher(tester, windowSize: smokeLargeWindowSize);

      await queryAndWaitForResults(tester, controller, 'wox launcher test xyz123');
      await waitForQueryBoxFocus(tester, controller);

      final initialIndex = controller.activeResultViewController.activeIndex.value;

      controller.handleQueryBoxArrowDown();
      await tester.pump();
      expect(controller.activeResultViewController.activeIndex.value, equals(initialIndex + 1));

      controller.handleQueryBoxArrowUp();
      await tester.pump();
      expect(controller.activeResultViewController.activeIndex.value, equals(initialIndex));

      await controller.hideApp(const UuidV4().generate());
      await waitForWindowVisibility(tester, false);
      expect(await windowManager.isVisible(), isFalse);
    });

    testWidgets('P0-SMK-08: Long press Alt shows quick select labels', (tester) async {
      if (!Platform.isWindows) {
        return;
      }

      final controller = await launchAndShowLauncher(tester, windowSize: smokeLargeWindowSize);

      await queryAndWaitForResults(tester, controller, 'wox launcher test xyz123');
      controller.focusQueryBox();
      await tester.pump(const Duration(milliseconds: 200));

      expect(controller.isQuickSelectMode.value, isFalse);
      expect(controller.activeResultViewController.items.any((item) => item.value.isShowQuickSelect), isFalse);

      await holdQuickSelectModifier(tester);

      expect(controller.isQuickSelectMode.value, isTrue);
      final quickSelectItems = controller.activeResultViewController.items.where((item) => item.value.isShowQuickSelect).toList();
      expect(quickSelectItems, isNotEmpty);
      expect(quickSelectItems.first.value.quickSelectNumber, equals('1'));

      await releaseQuickSelectModifier(tester);

      expect(controller.isQuickSelectMode.value, isFalse);
      expect(controller.activeResultViewController.items.any((item) => item.value.isShowQuickSelect), isFalse);
    });

    testWidgets('P0-SMK-09: Closing settings returns focus to the launcher query box', (tester) async {
      final controller = await launchAndShowLauncher(tester, windowSize: smokeLargeWindowSize);
      final settingController = await openSettings(tester, controller, 'general');

      expect(controller.isInSettingView.value, isTrue);

      await closeSettings(tester, settingController, controller);
      await waitForQueryBoxFocus(tester, controller);

      expect(await windowManager.isVisible(), isTrue);
      expect(controller.isInSettingView.value, isFalse);
      expect(controller.queryBoxFocusNode.hasFocus, isTrue);
    });

    testWidgets('P0-SMK-10: Re-show restores query box focus for immediate typing', (tester) async {
      final controller = await launchAndShowLauncher(tester, windowSize: smokeLargeWindowSize);

      await waitForQueryBoxFocus(tester, controller);
      expect(controller.queryBoxFocusNode.hasFocus, isTrue);

      await hideLauncherByEscape(tester, controller);

      await triggerBackendShowApp(tester);
      await waitForQueryBoxFocus(tester, controller);

      expect(await windowManager.isVisible(), isTrue);
      expect(controller.queryBoxFocusNode.hasFocus, isTrue);
      expect(controller.isInSettingView.value, isFalse);
    });

    testWidgets('P0-SMK-11: Fresh launch clears stale query when shown from the default source', (tester) async {
      final controller = await launchAndShowLauncher(tester, windowSize: smokeLargeWindowSize);

      await queryAndWaitForResults(tester, controller, 'wox launcher test xyz123');
      expect(controller.queryBoxTextFieldController.text, equals('wox launcher test xyz123'));

      await updateSettingDirect('LaunchMode', WoxLaunchModeEnum.WOX_LAUNCH_MODE_FRESH.code);

      await hideLauncherByEscape(tester, controller);

      await triggerBackendShowApp(tester);
      await waitForQueryBoxFocus(tester, controller);

      expect(controller.queryBoxTextFieldController.text, isEmpty);
      expect(controller.queryBoxFocusNode.hasFocus, isTrue);
    });

    testWidgets('P0-SMK-12: Fresh launch preserves a query-hotkey query source', (tester) async {
      final controller = await launchLauncherApp(tester);
      await updateSettingDirect('LaunchMode', WoxLaunchModeEnum.WOX_LAUNCH_MODE_FRESH.code);

      await triggerTestQueryHotkey(tester, 'wox launcher test xyz123');
      await waitForQueryBoxText(tester, controller, 'wox launcher test xyz123');

      expect(await windowManager.isVisible(), isTrue);
      expect(controller.queryBoxTextFieldController.text, equals('wox launcher test xyz123'));
    });

    testWidgets('P0-SMK-13: Fresh launch preserves a selection query source payload', (tester) async {
      final controller = await launchLauncherApp(tester);
      await updateSettingDirect('LaunchMode', WoxLaunchModeEnum.WOX_LAUNCH_MODE_FRESH.code);

      await triggerTestSelectionHotkey(tester, type: WoxSelectionTypeEnum.WOX_SELECTION_TYPE_TEXT.code, text: 'selected smoke text');
      await pumpUntil(
        tester,
        () =>
            controller.currentQuery.value.queryType == WoxQueryTypeEnum.WOX_QUERY_TYPE_SELECTION.code &&
            controller.currentQuery.value.querySelection.type == WoxSelectionTypeEnum.WOX_SELECTION_TYPE_TEXT.code &&
            controller.currentQuery.value.querySelection.text == 'selected smoke text',
        timeout: const Duration(seconds: 30),
      );

      expect(await windowManager.isVisible(), isTrue);
      expect(controller.currentQuery.value.queryType, equals(WoxQueryTypeEnum.WOX_QUERY_TYPE_SELECTION.code));
      expect(controller.currentQuery.value.querySelection.type, equals(WoxSelectionTypeEnum.WOX_SELECTION_TYPE_TEXT.code));
      expect(controller.currentQuery.value.querySelection.text, equals('selected smoke text'));
      expect(controller.queryBoxTextFieldController.text, isEmpty);
    });

    testWidgets('P0-SMK-14: Fresh launch preserves a tray-query source payload', (tester) async {
      final controller = await launchLauncherApp(tester);
      await updateSettingDirect('LaunchMode', WoxLaunchModeEnum.WOX_LAUNCH_MODE_FRESH.code);

      await triggerTestTrayQuery(tester, query: 'tray smoke query', showQueryBox: true);
      await waitForQueryBoxText(tester, controller, 'tray smoke query');

      expect(await windowManager.isVisible(), isTrue);
      expect(controller.queryBoxTextFieldController.text, equals('tray smoke query'));
      expect(controller.isQueryBoxVisible.value, isTrue);
      expect(controller.isToolbarHiddenForce.value, isTrue);
    });

    testWidgets('P0-SMK-04: Action panel opens with Alt+J', (tester) async {
      final controller = await launchAndShowLauncher(tester, windowSize: smokeLargeWindowSize);

      await queryAndWaitForResults(tester, controller, 'wox launcher test xyz123');

      controller.openActionPanelForActiveResult(const UuidV4().generate());
      await tester.pump(const Duration(milliseconds: 500));

      expect(controller.isShowActionPanel.value, isTrue);
    }, skip: true);

    testWidgets('P0-SMK-05: Settings entry is reachable via openSetting', (tester) async {
      final launcherController = await launchAndShowLauncher(tester);

      await openSettings(tester, launcherController, 'general');

      expect(launcherController.isInSettingView.value, isTrue);
      expect(find.byType(WoxSettingView), findsOneWidget);
    });

    testWidgets('P0-SMK-06: Settings page basic navigation', (tester) async {
      final launcherController = await launchAndShowLauncher(tester);
      final settingController = await openSettings(tester, launcherController, 'general');

      expect(find.byType(WoxSettingView), findsOneWidget);

      await tapSettingNavItem(tester, settingController, 'general');
      expect(find.byType(WoxSettingView), findsOneWidget);

      await tapSettingNavItem(tester, settingController, 'ui');
      expect(find.byType(WoxSettingView), findsOneWidget);

      await tapSettingNavItem(tester, settingController, 'data');
      expect(find.byType(WoxSettingView), findsOneWidget);

      await closeSettings(tester, settingController, launcherController);
    });
  });
}

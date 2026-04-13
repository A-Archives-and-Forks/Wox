import 'dart:io';

import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:get/get.dart';
import 'package:uuid/v4.dart';
import 'package:wox/controllers/wox_setting_controller.dart';
import 'package:wox/enums/wox_launch_mode_enum.dart';
import 'package:wox/enums/wox_position_type_enum.dart';
import 'package:wox/enums/wox_query_type_enum.dart';
import 'package:wox/enums/wox_selection_type_enum.dart';
import 'package:wox/modules/launcher/views/wox_launcher_view.dart';
import 'package:wox/modules/setting/views/wox_setting_view.dart';
import 'package:wox/utils/windows/window_manager.dart';

import 'smoke_test_helper.dart';

void registerLauncherCoreSmokeTests() {
  group('T2: Core Smoke Tests', () {
    testWidgets('T2-01: Launch main window and verify UI elements', (tester) async {
      final controller = await launchAndShowLauncher(tester);

      expect(find.byType(WoxLauncherView), findsOneWidget);
      expect(await windowManager.isVisible(), isTrue);
      expect(controller.isQueryBoxVisible.value, isTrue);
    });

    testWidgets('T2-02: ShowPosition mouse_screen centers the launcher on the current screen', (tester) async {
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

    testWidgets('T2-03: ShowPosition last_location restores the saved window coordinates exactly', (tester) async {
      final controller = await launchLauncherApp(tester);
      await hideLauncherIfVisible(tester, controller);

      const expectedPosition = Offset(240, 180);
      await updateSettingDirect('ShowPosition', WoxPositionTypeEnum.POSITION_TYPE_LAST_LOCATION.code);
      await saveLastWindowPosition(expectedPosition.dx.toInt(), expectedPosition.dy.toInt());

      await triggerBackendShowApp(tester);

      final actualPosition = await waitForWindowPosition(tester, expectedPosition);
      expect(isOffsetClose(actualPosition, expectedPosition), isTrue);
    });

    testWidgets('T2-04: Keyboard navigation works', (tester) async {
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

    testWidgets('T2-05: Long press Alt shows quick select labels', (tester) async {
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

    testWidgets('T2-06: Closing settings returns focus to the launcher query box', (tester) async {
      final controller = await launchAndShowLauncher(tester, windowSize: smokeLargeWindowSize);
      final settingController = await openSettings(tester, controller, 'general');

      expect(controller.isInSettingView.value, isTrue);

      await closeSettings(tester, settingController, controller);
      await waitForQueryBoxFocus(tester, controller);

      expect(await windowManager.isVisible(), isTrue);
      expect(controller.isInSettingView.value, isFalse);
      expect(controller.queryBoxFocusNode.hasFocus, isTrue);
    });

    testWidgets('T2-07: Re-show restores query box focus for immediate typing', (tester) async {
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

    testWidgets('T2-08: Fresh launch clears stale query when shown from the default source', (tester) async {
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

    testWidgets('T2-09: Fresh launch preserves a query-hotkey query source', (tester) async {
      final controller = await launchLauncherApp(tester);
      await updateSettingDirect('LaunchMode', WoxLaunchModeEnum.WOX_LAUNCH_MODE_FRESH.code);

      await triggerTestQueryHotkey(tester, 'wox launcher test xyz123');
      await waitForQueryBoxText(tester, controller, 'wox launcher test xyz123');

      expect(await windowManager.isVisible(), isTrue);
      expect(controller.queryBoxTextFieldController.text, equals('wox launcher test xyz123'));
    });

    testWidgets('T2-10: Fresh launch preserves a selection query source payload', (tester) async {
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

    testWidgets('T2-11: Fresh launch preserves tray-query query and layout payloads', (tester) async {
      final controller = await launchLauncherApp(tester);
      await updateSettingDirect('LaunchMode', WoxLaunchModeEnum.WOX_LAUNCH_MODE_FRESH.code);

      await triggerTestTrayQuery(tester, query: 'tray smoke query', hideQueryBox: false, hideToolbar: true);
      await waitForQueryBoxText(tester, controller, 'tray smoke query');

      expect(await windowManager.isVisible(), isTrue);
      expect(controller.queryBoxTextFieldController.text, equals('tray smoke query'));
      expect(controller.isQueryBoxVisible.value, isTrue);
      expect(controller.isToolbarHiddenForce.value, isTrue);
    });

    testWidgets('T2-12: Continue launch restores the main query after a query hotkey session', (tester) async {
      final controller = await launchAndShowLauncher(tester, windowSize: smokeLargeWindowSize);
      await updateSettingDirect('LaunchMode', WoxLaunchModeEnum.WOX_LAUNCH_MODE_CONTINUE.code);

      await queryAndWaitForResults(tester, controller, 'main query xyz123');
      expect(controller.queryBoxTextFieldController.text, equals('main query xyz123'));

      await hideLauncherByEscape(tester, controller);

      await triggerTestQueryHotkey(tester, 'hotkey query abc456');
      await waitForQueryBoxText(tester, controller, 'hotkey query abc456');
      expect(controller.queryBoxTextFieldController.text, equals('hotkey query abc456'));

      await hideLauncherByEscape(tester, controller);

      await triggerBackendShowApp(tester);
      await waitForQueryBoxText(tester, controller, 'main query xyz123');

      expect(controller.queryBoxTextFieldController.text, equals('main query xyz123'));
    });

    testWidgets('T2-13: Action panel opens with Alt+J', (tester) async {
      final controller = await launchAndShowLauncher(tester, windowSize: smokeLargeWindowSize);

      await queryAndWaitForResults(tester, controller, 'wox launcher test xyz123');

      controller.openActionPanelForActiveResult(const UuidV4().generate());
      await tester.pump(const Duration(milliseconds: 500));

      expect(controller.isShowActionPanel.value, isTrue);
    }, skip: true);

    testWidgets('T2-14: Settings entry is reachable via openSetting', (tester) async {
      final launcherController = await launchAndShowLauncher(tester);

      await openSettings(tester, launcherController, 'general');

      expect(launcherController.isInSettingView.value, isTrue);
      expect(find.byType(WoxSettingView), findsOneWidget);
    });

    testWidgets('T2-15: Settings page basic navigation', (tester) async {
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

    testWidgets('T2-16: LaunchMode switch via settings syncs hide and show behavior immediately', (tester) async {
      final controller = await launchAndShowLauncher(tester, windowSize: smokeLargeWindowSize);

      // Open settings general page.
      final settingController = await openSettings(tester, controller, 'general');

      // --- Phase 1: Switch fresh → continue via the UI dropdown ---
      final freshLabel = settingController.tr('ui_launch_mode_fresh');
      final continueLabel = settingController.tr('ui_launch_mode_continue');

      // Tap the dropdown (currently showing "fresh") to open its menu.
      await tester.tap(find.text(freshLabel));
      await tester.pump(const Duration(milliseconds: 300));

      // Tap the "continue" option in the opened dropdown menu.
      // DropdownButton renders two text widgets for the selected item (one in
      // the button, one in the menu), so use .last to tap the menu item.
      await tester.tap(find.text(continueLabel).last);
      await tester.pump(const Duration(milliseconds: 500));

      await closeSettings(tester, settingController, controller);

      // Verify lastLaunchMode was synced immediately.
      expect(controller.lastLaunchMode, equals(WoxLaunchModeEnum.WOX_LAUNCH_MODE_CONTINUE.code));

      // Query and get results.
      await queryAndWaitForResults(tester, controller, 'wox launcher test xyz123');
      expect(controller.activeResultViewController.items, isNotEmpty);
      final sizeWithResults = await windowManager.getSize();

      // Hide and re-show — continue mode should preserve results and height.
      await hideLauncherByEscape(tester, controller);
      await triggerBackendShowApp(tester);
      await tester.pump(const Duration(milliseconds: 500));

      expect(controller.activeResultViewController.items, isNotEmpty, reason: 'Continue mode should preserve results');
      expect(controller.queryBoxTextFieldController.text, equals('wox launcher test xyz123'));
      final sizeAfterContinueReshow = await windowManager.getSize();
      expect(
        (sizeAfterContinueReshow.height - sizeWithResults.height).abs(),
        lessThanOrEqualTo(2),
        reason: 'Continue mode: window height should match (was ${sizeWithResults.height}, got ${sizeAfterContinueReshow.height})',
      );

      // --- Phase 2: Switch continue → fresh via the UI dropdown ---
      final settingController2 = await openSettings(tester, controller, 'general');

      // Dropdown now shows "continue". Tap it to open the menu.
      await tester.tap(find.text(continueLabel));
      await tester.pump(const Duration(milliseconds: 300));

      // Tap the "fresh" option.
      await tester.tap(find.text(freshLabel).last);
      await tester.pump(const Duration(milliseconds: 500));

      await closeSettings(tester, settingController2, controller);

      // Verify lastLaunchMode was synced back to fresh.
      expect(controller.lastLaunchMode, equals(WoxLaunchModeEnum.WOX_LAUNCH_MODE_FRESH.code));

      // Query and get results again.
      await queryAndWaitForResults(tester, controller, 'wox launcher test xyz123');
      expect(controller.activeResultViewController.items, isNotEmpty);

      // Hide and re-show — fresh mode should clear results.
      await hideLauncherByEscape(tester, controller);
      await triggerBackendShowApp(tester);
      await tester.pump(const Duration(milliseconds: 500));

      expect(controller.activeResultViewController.items, isEmpty, reason: 'Fresh mode should clear results on hide');
      expect(controller.queryBoxTextFieldController.text, isEmpty, reason: 'Fresh mode should clear query text on hide');
    });

    testWidgets('T2-17: Continue launch keeps result actions executable after hide and re-show', (tester) async {
      final controller = await launchAndShowLauncher(tester, windowSize: smokeLargeWindowSize);
      await updateSettingDirect('LaunchMode', WoxLaunchModeEnum.WOX_LAUNCH_MODE_CONTINUE.code);

      final result = await queryAndWaitForActiveResult(tester, controller, 'wox settings');
      expect(result.title, equals('Open Wox Settings'));
      expectResultActionByName(result, 'execute');

      controller.executeDefaultAction(const UuidV4().generate());
      await pumpUntil(tester, () => controller.isInSettingView.value && find.byType(WoxSettingView).evaluate().isNotEmpty, timeout: const Duration(seconds: 30));

      final settingController = Get.find<WoxSettingController>();
      await closeSettings(tester, settingController, controller);
      await waitForQueryBoxText(tester, controller, 'wox settings');
      expect(controller.activeResultViewController.items, isNotEmpty);

      await hideLauncherByEscape(tester, controller);
      await triggerBackendShowApp(tester);
      await waitForQueryBoxText(tester, controller, 'wox settings');
      expect(controller.activeResultViewController.items, isNotEmpty, reason: 'Continue mode should preserve prior results on re-show');

      controller.executeDefaultAction(const UuidV4().generate());
      await pumpUntil(tester, () => controller.isInSettingView.value && find.byType(WoxSettingView).evaluate().isNotEmpty, timeout: const Duration(seconds: 30));

      final settingControllerAfterReshow = Get.find<WoxSettingController>();
      await closeSettings(tester, settingControllerAfterReshow, controller);
    });
  });
}

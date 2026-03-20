import 'package:flutter_test/flutter_test.dart';
import 'package:uuid/v4.dart';
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

    testWidgets('P0-SMK-03: Keyboard navigation works', (tester) async {
      final controller = await launchAndShowLauncher(tester);

      await queryAndWaitForResults(tester, controller, 'wox launcher test xyz123');

      final initialIndex = controller.activeResultViewController.activeIndex.value;

      controller.handleQueryBoxArrowDown();
      await tester.pump();
      expect(controller.activeResultViewController.activeIndex.value, equals(initialIndex + 1));

      controller.handleQueryBoxArrowUp();
      await tester.pump();
      expect(controller.activeResultViewController.activeIndex.value, equals(initialIndex));

      controller.hideApp(const UuidV4().generate());
      await tester.pumpAndSettle();
      expect(await windowManager.isVisible(), isFalse);
    });

    testWidgets('P0-SMK-04: Action panel opens with Alt+J', (tester) async {
      final controller = await launchAndShowLauncher(tester, windowSize: smokeLargeWindowSize);

      await queryAndWaitForResults(tester, controller, 'wox launcher test xyz123');

      controller.openActionPanelForActiveResult(const UuidV4().generate());
      await tester.pumpAndSettle();

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

      for (final page in ['general', 'ui', 'data']) {
        settingController.activeNavPath.value = page;
        await tester.pumpAndSettle();
        expect(find.byType(WoxSettingView), findsOneWidget);
      }

      await closeSettings(tester, settingController, launcherController);
    });
  });
}

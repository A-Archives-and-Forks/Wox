import 'package:flutter_test/flutter_test.dart';
import 'package:wox/modules/setting/views/wox_setting_view.dart';

import 'smoke_test_helper.dart';

void registerLauncherKeyFunctionalitySmokeTests() {
  group('P1-SMK: Key Functionality Smoke Tests', () {
    testWidgets('P1-SMK-17: Theme settings accessible', (tester) async {
      final launcherController = await launchAndShowLauncher(tester);
      final settingController = await openSettings(tester, launcherController, 'general');

      await tapSettingNavItem(tester, settingController, 'ui_ui');

      expect(find.byType(WoxSettingView), findsOneWidget);

      await closeSettings(tester, settingController, launcherController);
    });

    testWidgets('P1-SMK-18: Data backup entry accessible', (tester) async {
      final launcherController = await launchAndShowLauncher(tester);
      final settingController = await openSettings(tester, launcherController, 'general');

      await tapSettingNavItem(tester, settingController, 'ui_data');

      expect(find.byType(WoxSettingView), findsOneWidget);

      await closeSettings(tester, settingController, launcherController);
    });

    testWidgets('P1-SMK-19: Usage and About pages load', (tester) async {
      final launcherController = await launchAndShowLauncher(tester);
      final settingController = await openSettings(tester, launcherController, 'general');

      await tapSettingNavItem(tester, settingController, 'ui_usage');

      expect(find.byType(WoxSettingView), findsOneWidget);

      await tapSettingNavItem(tester, settingController, 'ui_about');
      expect(find.byType(WoxSettingView), findsOneWidget);

      await closeSettings(tester, settingController, launcherController);
    });
  });
}

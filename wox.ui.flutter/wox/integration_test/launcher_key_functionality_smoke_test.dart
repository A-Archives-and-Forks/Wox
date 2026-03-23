import 'package:flutter_test/flutter_test.dart';
import 'package:wox/modules/setting/views/wox_setting_view.dart';

import 'smoke_test_helper.dart';

void registerLauncherKeyFunctionalitySmokeTests() {
  group('P1-SMK: Key Functionality Smoke Tests', () {
    testWidgets('P1-SMK-15: Theme settings accessible', (tester) async {
      final launcherController = await launchAndShowLauncher(tester);
      final settingController = await openSettings(tester, launcherController, 'general');

      await tapSettingNavItem(tester, settingController, 'ui');

      expect(find.byType(WoxSettingView), findsOneWidget);

      await closeSettings(tester, settingController, launcherController);
    });

    testWidgets('P1-SMK-16: Data backup entry accessible', (tester) async {
      final launcherController = await launchAndShowLauncher(tester);
      final settingController = await openSettings(tester, launcherController, 'general');

      await tapSettingNavItem(tester, settingController, 'data');

      expect(find.byType(WoxSettingView), findsOneWidget);

      await closeSettings(tester, settingController, launcherController);
    });

    testWidgets('P1-SMK-17: Usage and About pages load', (tester) async {
      final launcherController = await launchAndShowLauncher(tester);
      final settingController = await openSettings(tester, launcherController, 'general');

      await tapSettingNavItem(tester, settingController, 'usage');

      expect(find.byType(WoxSettingView), findsOneWidget);

      await tapSettingNavItem(tester, settingController, 'about');
      expect(find.byType(WoxSettingView), findsOneWidget);

      await closeSettings(tester, settingController, launcherController);
    });
  });
}

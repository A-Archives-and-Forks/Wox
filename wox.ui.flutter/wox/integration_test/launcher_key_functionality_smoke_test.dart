import 'package:flutter_test/flutter_test.dart';
import 'package:wox/modules/setting/views/wox_setting_view.dart';

import 'smoke_test_helper.dart';

void registerLauncherKeyFunctionalitySmokeTests() {
  group('T3: Key Functionality Smoke Tests', () {
    testWidgets('T3-01: Theme settings accessible', (tester) async {
      final launcherController = await launchAndShowLauncher(tester);
      final settingController = await openSettings(tester, launcherController, 'general');

      await tapSettingNavItem(tester, settingController, 'ui');

      expect(find.byType(WoxSettingView), findsOneWidget);

      await closeSettings(tester, settingController, launcherController);
    });

    testWidgets('T3-02: Data backup entry accessible', (tester) async {
      final launcherController = await launchAndShowLauncher(tester);
      final settingController = await openSettings(tester, launcherController, 'general');

      await tapSettingNavItem(tester, settingController, 'data');

      expect(find.byType(WoxSettingView), findsOneWidget);

      await closeSettings(tester, settingController, launcherController);
    });

    testWidgets('T3-03: Usage and About pages load', (tester) async {
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

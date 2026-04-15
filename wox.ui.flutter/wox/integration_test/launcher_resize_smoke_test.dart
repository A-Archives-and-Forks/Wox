import 'dart:io';

import 'package:flutter_test/flutter_test.dart';
import 'package:wox/utils/windows/window_manager.dart';

import 'smoke_test_helper.dart';

void registerLauncherResizeSmokeTests() {
  group('T7: Resize Smoke Tests', () {
    testWidgets('T7-01: repeated result grow/shrink cycles keep native window height in sync', (tester) async {
      if (!Platform.isWindows) {
        return;
      }

      final controller = await launchAndShowLauncher(tester, windowSize: smokeLargeWindowSize);

      for (var i = 0; i < 10; i++) {
        await queryAndWaitForResults(tester, controller, 'wox launcher test xyz123');
        await waitForWindowHeightToMatchController(tester, controller);
        final heightWithResults = (await windowManager.getSize()).height;

        tester.testTextInput.enterText('');
        await tester.pump();
        await waitForNoActiveResults(tester, controller);
        await waitForWindowHeightToMatchController(tester, controller);
        final heightWithoutResults = (await windowManager.getSize()).height;

        expect(heightWithResults, greaterThan(heightWithoutResults));
      }
    });
  });
}

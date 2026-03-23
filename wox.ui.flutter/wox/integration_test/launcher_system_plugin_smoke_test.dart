import 'package:flutter_test/flutter_test.dart';

import 'smoke_test_helper.dart';

void registerSystemPluginSmokeTests() {
  group('P2-SMK: System Plugin Smoke Tests - Tier 1 (Deterministic)', () {
    testWidgets('P2-SMK-20: Calculator plugin basic arithmetic', (tester) async {
      final controller = await launchAndShowLauncher(tester, windowSize: smokeLargeWindowSize);
      await queryAndWaitForResults(tester, controller, '1+1');

      expect(controller.activeResultViewController.items, isNotEmpty);
      final result = controller.activeResultViewController.activeItem.data;

      expect(result.title, equals('2'));
      final copyActions = result.actions.where((action) => action.name.toLowerCase().contains('copy')).toList();
      expect(copyActions, isNotEmpty);
    });

    testWidgets('P2-SMK-21: Calculator plugin sqrt function', (tester) async {
      final controller = await launchAndShowLauncher(tester, windowSize: smokeLargeWindowSize);
      await queryAndWaitForResults(tester, controller, 'sqrt(16)');

      expect(controller.activeResultViewController.items, isNotEmpty);
      final result = controller.activeResultViewController.activeItem.data;

      expect(result.title, equals('4'));
    });

    testWidgets('P2-SMK-22: URL plugin opens URLs', (tester) async {
      final controller = await launchAndShowLauncher(tester, windowSize: smokeLargeWindowSize);
      await queryAndWaitForResults(tester, controller, 'https://githubgithugithub.com');

      expect(controller.activeResultViewController.items, isNotEmpty);
      final result = controller.activeResultViewController.activeItem.data;

      expect(result.title, equals('https://githubgithugithub.com'));
      final openActions = result.actions.where((action) => action.name.toLowerCase().contains('open')).toList();
      expect(openActions, isNotEmpty);
    });

    testWidgets('P2-SMK-23: System plugin lock command', (tester) async {
      final controller = await launchAndShowLauncher(tester, windowSize: smokeLargeWindowSize);
      await queryAndWaitForResults(tester, controller, 'lock');

      expect(controller.activeResultViewController.items, isNotEmpty);
      final result = controller.activeResultViewController.activeItem.data;

      expect(result.title.toLowerCase(), contains('lock'));
      final executeActions = result.actions.where((action) => action.name.toLowerCase().contains('execute')).toList();
      expect(executeActions, isNotEmpty);
    });

    testWidgets('P2-SMK-24: System plugin settings command', (tester) async {
      final controller = await launchAndShowLauncher(tester, windowSize: smokeLargeWindowSize);
      await queryAndWaitForResults(tester, controller, 'wox settings');

      expect(controller.activeResultViewController.items, isNotEmpty);
      final result = controller.activeResultViewController.activeItem.data;

      expect(result.title, equals('Open Wox Settings'));
      final executeActions = result.actions.where((action) => action.name.toLowerCase().contains('execute')).toList();
      expect(executeActions, isNotEmpty);
    });

    testWidgets('P2-SMK-25: Doctor plugin returns diagnostic info', (tester) async {
      final controller = await launchAndShowLauncher(tester, windowSize: smokeLargeWindowSize);
      await queryAndWaitForResults(tester, controller, 'doctor');

      expect(controller.activeResultViewController.items, isNotEmpty);
      final result = controller.activeResultViewController.activeItem.data;

      expect(result.title, isNotEmpty);
      expect(result.subTitle, isNotEmpty);
    });

    testWidgets('P2-SMK-26: Emoji plugin returns emoji results', (tester) async {
      final controller = await launchAndShowLauncher(tester, windowSize: smokeLargeWindowSize);
      await queryAndWaitForResults(tester, controller, 'emoji smile');

      // Emoji plugin shows results in grid or list view
      final hasGridItems = controller.resultGridViewController.items.isNotEmpty;
      final hasListItems = controller.resultListViewController.items.isNotEmpty;
      expect(hasGridItems || hasListItems, isTrue);
    });

    testWidgets('P2-SMK-27: Indicator plugin shows plugin hints', (tester) async {
      final controller = await launchAndShowLauncher(tester, windowSize: smokeLargeWindowSize);
      await queryAndWaitForResults(tester, controller, '*');

      // Indicator plugin should show plugin suggestions
      expect(controller.activeResultViewController.items, isNotEmpty);
      final result = controller.activeResultViewController.activeItem.data;
      // Result should contain some indicator text
      expect(result.title.isNotEmpty || result.subTitle.isNotEmpty, isTrue);
    });

    testWidgets('P2-SMK-28: Converter plugin time conversion', (tester) async {
      final controller = await launchAndShowLauncher(tester, windowSize: smokeLargeWindowSize);
      await queryAndWaitForResults(tester, controller, '1h to minutes');

      expect(controller.activeResultViewController.items, isNotEmpty);
      final result = controller.activeResultViewController.activeItem.data;

      expect(result.title, contains('60'));
    });

    testWidgets('P2-SMK-29: Theme plugin returns theme options', (tester) async {
      final controller = await launchAndShowLauncher(tester, windowSize: smokeLargeWindowSize);
      await queryAndWaitForResults(tester, controller, 'theme');

      expect(controller.activeResultViewController.items, isNotEmpty);
      // Should have at least a default theme option
      expect(controller.activeResultViewController.items.length, greaterThanOrEqualTo(1));
    });
  });

  group('P2-SMK: System Plugin Smoke Tests - Tier 2 (Conditional - requires environment)', () {
    testWidgets('P2-SMK-30: WebSearch plugin searches Google - requires default Google config', (tester) async {
      final controller = await launchAndShowLauncher(tester, windowSize: smokeLargeWindowSize);
      await queryAndWaitForResults(tester, controller, 'g wox launcher');

      expect(controller.activeResultViewController.items, isNotEmpty);
      final result = controller.activeResultViewController.activeItem.data;

      expect(result.title, equals('Search Google for wox launcher'));
    }, skip: true);

    testWidgets('P2-SMK-31: Shell plugin executes shell commands - requires shell', (tester) async {
      final controller = await launchAndShowLauncher(tester, windowSize: smokeLargeWindowSize);
      await queryAndWaitForResults(tester, controller, '> echo hello');

      expect(controller.activeResultViewController.items, isNotEmpty);
      final result = controller.activeResultViewController.activeItem.data;

      expect(result.title.toLowerCase(), contains('echo hello'));
    }, skip: true);

    testWidgets('P2-SMK-32: WPM plugin returns word count - requires typing session', (tester) async {
      final controller = await launchAndShowLauncher(tester, windowSize: smokeLargeWindowSize);
      await queryAndWaitForResults(tester, controller, 'wpm');

      expect(controller.activeResultViewController.items, isNotEmpty);
    }, skip: true);

    testWidgets('P2-SMK-33: Backup plugin returns backup options - requires filesystem', (tester) async {
      final controller = await launchAndShowLauncher(tester, windowSize: smokeLargeWindowSize);
      await queryAndWaitForResults(tester, controller, 'backup');

      expect(controller.activeResultViewController.items, isNotEmpty);
    }, skip: true);

    testWidgets('P2-SMK-34: Update plugin checks for updates - requires network', (tester) async {
      final controller = await launchAndShowLauncher(tester, windowSize: smokeLargeWindowSize);
      await queryAndWaitForResults(tester, controller, 'update');

      expect(controller.activeResultViewController.items, isNotEmpty);
    }, skip: true);

    testWidgets('P2-SMK-35: Query History plugin does not crash on empty history - fresh install', (tester) async {
      final controller = await launchAndShowLauncher(tester, windowSize: smokeLargeWindowSize);
      await queryAndWaitForResults(tester, controller, 'h');

      // Should not crash - empty results are acceptable
      // Just verify no exception is thrown and window is still responsive
      expect(controller.activeResultViewController.items.isEmpty, isTrue);
    }, skip: true);

    testWidgets('P2-SMK-36: Application plugin finds platform applications - macOS only', (tester) async {
      final controller = await launchAndShowLauncher(tester, windowSize: smokeLargeWindowSize);
      // Use a common application name based on platform
      await queryAndWaitForResults(tester, controller, 'Finder');

      expect(controller.activeResultViewController.items, isNotEmpty);
    }, skip: true);

    testWidgets('P2-SMK-37: Clipboard plugin handles clipboard history - no clipboard data', (tester) async {
      final controller = await launchAndShowLauncher(tester, windowSize: smokeLargeWindowSize);
      await queryAndWaitForResults(tester, controller, 'cb');

      // Should not crash - empty clipboard history is acceptable
      // Just verify no exception is thrown
      expect(controller.activeResultViewController.items.isEmpty, isTrue);
    }, skip: true);
  });
}

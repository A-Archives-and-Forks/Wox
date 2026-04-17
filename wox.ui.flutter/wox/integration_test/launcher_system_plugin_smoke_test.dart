import 'dart:convert';
import 'dart:io';

import 'package:flutter_test/flutter_test.dart';
import 'package:wox/enums/wox_selection_type_enum.dart';

import 'smoke_test_helper.dart';

const String _appPluginId = 'ea2b6859-14bc-4c89-9c88-627da7379141';
const String _shellPluginId = '8a4b5c6d-7e8f-9a0b-1c2d-3e4f5a6b7c8d';
const String _appDirectoriesSettingKey = 'AppDirectories';
const String _shellCommandsSettingKey = 'shellCommands';

Future<Directory> _ensureGlobalTriggerSmokeFixtureDirectory() async {
  // The global app plugin normally indexes a large Windows surface area. These
  // speed smoke tests need one deterministic app candidate that can be queried
  // without depending on the developer's real Start Menu contents.
  final userDir = Platform.environment['WOX_TEST_USER_DIR'];
  if (userDir == null || userDir.isEmpty) {
    throw StateError('WOX_TEST_USER_DIR is required for smoke fixture setup.');
  }

  final fixtureDir = Directory('$userDir${Platform.pathSeparator}global-trigger-speed-fixtures');
  await fixtureDir.create(recursive: true);

  final smokeApp = File('${fixtureDir.path}${Platform.pathSeparator}SmokeApp.exe');
  if (await smokeApp.exists()) {
    return fixtureDir;
  }

  final systemRoot = Platform.environment['SystemRoot'] ?? r'C:\Windows';
  final sourceApp = File('$systemRoot${Platform.pathSeparator}System32${Platform.pathSeparator}cmd.exe');
  if (!await sourceApp.exists()) {
    throw StateError('Failed to find a stable Windows executable for app smoke fixtures.');
  }

  await sourceApp.copy(smokeApp.path);
  return fixtureDir;
}

void registerSystemPluginSmokeTests() {
  group('T6: System Plugin Smoke Tests - Tier 1 (Deterministic)', () {
    testWidgets('T6-01: Calculator plugin basic arithmetic', (tester) async {
      final controller = await launchAndShowLauncher(tester, windowSize: smokeLargeWindowSize);
      final result = await queryAndWaitForActiveResult(tester, controller, '1+1');

      expect(result.title, equals('2'));
      expect(result.isGroup, isFalse);
      expectResultActionByName(result, 'copy');
      expectQueryLatencyWithinThreshold(result);
    });

    testWidgets('T6-02: Calculator plugin sqrt function', (tester) async {
      final controller = await launchAndShowLauncher(tester, windowSize: smokeLargeWindowSize);
      final result = await queryAndWaitForActiveResult(tester, controller, 'sqrt(16)');

      expect(result.title, equals('4'));
      expect(result.isGroup, isFalse);
      expectResultActionByName(result, 'copy');
    });

    testWidgets('T6-03: Calculator plugin respects operator precedence', (tester) async {
      final controller = await launchAndShowLauncher(tester, windowSize: smokeLargeWindowSize);
      final result = await queryAndWaitForActiveResult(tester, controller, '2+3*4');

      expect(result.title, equals('14'));
      expect(result.isGroup, isFalse);
      expectResultActionByName(result, 'copy');
    });

    testWidgets('T6-04: URL plugin opens URLs', (tester) async {
      final controller = await launchAndShowLauncher(tester, windowSize: smokeLargeWindowSize);
      final result = await queryAndWaitForActiveResult(tester, controller, 'https://githubgithugithub.com');

      expect(result.title, equals('https://githubgithugithub.com'));
      expect(result.isGroup, isFalse);
      expectResultActionByName(result, 'open');
      expectQueryLatencyWithinThreshold(result);
    });

    testWidgets('T6-05: System plugin lock command', (tester) async {
      final controller = await launchAndShowLauncher(tester, windowSize: smokeLargeWindowSize);
      final result = await queryAndWaitForActiveResult(tester, controller, 'lock');
      final executeAction = expectResultActionByName(result, 'execute');

      expect(result.title.toLowerCase(), contains('lock'));
      expect(result.isGroup, isFalse);
      expect(executeAction.preventHideAfterAction, isFalse);
      expectQueryLatencyWithinThreshold(result);
    });

    testWidgets('T6-06: System plugin settings command', (tester) async {
      final controller = await launchAndShowLauncher(tester, windowSize: smokeLargeWindowSize);
      final result = await queryAndWaitForActiveResult(tester, controller, 'wox settings');
      final executeAction = expectResultActionByName(result, 'execute');

      expect(result.title, equals('Open Wox Settings'));
      expect(result.isGroup, isFalse);
      expect(executeAction.preventHideAfterAction, isTrue);
      expectQueryLatencyWithinThreshold(result);
    });

    testWidgets('T6-07: Doctor plugin returns diagnostic info', (tester) async {
      final controller = await launchAndShowLauncher(tester, windowSize: smokeLargeWindowSize);
      final result = await queryAndWaitForActiveResult(tester, controller, 'doctor');

      expect(result.title, isNotEmpty);
      expect(result.subTitle, isNotEmpty);
      expect(result.isGroup, isFalse);
      expect(result.actions, isNotEmpty);
    });

    testWidgets('T6-08: Emoji plugin returns emoji results', (tester) async {
      final controller = await launchAndShowLauncher(tester, windowSize: smokeLargeWindowSize);
      final result = await queryAndWaitForActiveResult(tester, controller, 'emoji smile');

      expect(controller.activeResultViewController.items, isNotEmpty);
      expect(result.title, isNotEmpty);
      expect(result.isGroup, isFalse);
    });

    testWidgets('T6-09: Indicator plugin shows plugin hints', (tester) async {
      final controller = await launchAndShowLauncher(tester, windowSize: smokeLargeWindowSize);
      final result = await queryAndWaitForActiveResult(tester, controller, '*');

      expect(controller.activeResultViewController.items, isNotEmpty);
      expect(result.title, isNotEmpty);
      expect(result.isGroup, isFalse);
      expectQueryLatencyWithinThreshold(result);
    });

    testWidgets('T6-10: Converter plugin time conversion', (tester) async {
      final controller = await launchAndShowLauncher(tester, windowSize: smokeLargeWindowSize);
      final result = await queryAndWaitForActiveResult(tester, controller, '1h to minutes');

      expect(result.title, contains('60'));
      expect(result.isGroup, isFalse);
      expectResultActionByName(result, 'copy');
      expectQueryLatencyWithinThreshold(result);
    });

    testWidgets('T6-11: Theme plugin returns theme options', (tester) async {
      final controller = await launchAndShowLauncher(tester, windowSize: smokeLargeWindowSize);
      final result = await queryAndWaitForActiveResult(tester, controller, 'theme');

      expect(controller.activeResultViewController.items, isNotEmpty);
      expect(controller.activeResultViewController.items.length, greaterThanOrEqualTo(1));
      expect(result.title, isNotEmpty);
      expect(result.isGroup, isFalse);
    });

    testWidgets('T6-12: File search plugin empty query keeps result list empty', (tester) async {
      final controller = await launchAndShowLauncher(tester, windowSize: smokeLargeWindowSize);

      await triggerTestQueryHotkey(tester, 'f ');
      await waitForQueryBoxText(tester, controller, 'f ');
      await waitForNoActiveResults(tester, controller);
      await tester.pump(const Duration(milliseconds: 500));

      expect(controller.activeResultViewController.items, isEmpty);

      if (controller.hasVisibleToolbarMsg) {
        expect(
          controller.resolvedToolbarText,
          anyOf([
            equals('File search is ready'),
            equals('Indexing files'),
            startsWith('Analyzing folders'),
            startsWith('Scanning folders'),
            startsWith('Writing index'),
            equals('Finalizing index'),
            equals('File search needs file access'),
            equals('File search needs attention'),
          ]),
        );
        expect(controller.isToolbarShowedWithoutResults, isTrue);
      } else {
        expect(controller.isToolbarShowedWithoutResults, isFalse);
      }
    });
  });

  group('T6: Global Trigger Plugin Speed Smoke Tests', () {
    testWidgets('T6-21: Selection global trigger returns within 10ms', (tester) async {
      final controller = await launchAndShowLauncher(tester, windowSize: smokeLargeWindowSize);

      await triggerTestSelectionHotkey(tester, type: WoxSelectionTypeEnum.WOX_SELECTION_TYPE_TEXT.code, text: 'global selection speed smoke');
      await waitForActiveResults(tester, controller);

      final result = expectActiveResultWhere(
        controller,
        (candidate) => normalizeSmokeText(candidate.title) == 'copy',
        description: 'Expected the selection plugin copy result to be visible.',
      );

      expectResultActionByName(result, 'copy');
      expectQueryLatencyWithinThreshold(result);
    });

    testWidgets('T6-22: Shell global alias query returns within 10ms', (tester) async {
      final controller = await launchAndShowLauncher(tester, windowSize: smokeLargeWindowSize);

      // The shell plugin only exposes global results for configured aliases, so
      // seed one explicit command before querying instead of depending on local user settings.
      await updatePluginSettingDirect(
        _shellPluginId,
        _shellCommandsSettingKey,
        jsonEncode([
          {'Alias': 'smokeshell', 'Command': 'echo smoke', 'Enabled': true, 'Silent': false},
        ]),
      );
      await tester.pump(const Duration(milliseconds: 500));

      final result = await queryAndWaitForResultWhere(
        tester,
        controller,
        'smokeshell',
        (candidate) => normalizeSmokeText(candidate.title) == 'smokeshell',
        description: 'Expected the seeded global shell alias result to be visible.',
      );

      expectResultActionByName(result, 'execute');
      expectQueryLatencyWithinThreshold(result);
    });

    testWidgets('T6-23: WebSearch global fallback returns within 10ms', (tester) async {
      final controller = await launchAndShowLauncher(tester, windowSize: smokeLargeWindowSize);
      const query = 'zzglobalwebsearchspeedsmoke';

      final result = await queryAndWaitForResultWhere(
        tester,
        controller,
        query,
        (candidate) => candidate.title == 'Search Google for $query',
        description: 'Expected the WebSearch fallback result to be visible.',
      );

      expectResultActionByName(result, 'search');
      expectQueryLatencyWithinThreshold(result);
    });

    testWidgets('T6-24: App global query returns within 10ms', (tester) async {
      final controller = await launchAndShowLauncher(tester, windowSize: smokeLargeWindowSize);
      final fixtureDir = await _ensureGlobalTriggerSmokeFixtureDirectory();

      await updatePluginSettingDirect(
        _appPluginId,
        _appDirectoriesSettingKey,
        jsonEncode([
          {'Path': fixtureDir.path},
        ]),
      );
      await tester.pump(const Duration(milliseconds: 500));

      final result = await queryAndWaitForResultWhere(
        tester,
        controller,
        'smokeapp',
        (candidate) => candidate.subTitle.toLowerCase().contains('smokeapp.exe'),
        description: 'Expected the seeded fixture app result to be visible.',
        timeout: const Duration(seconds: 60),
      );

      expectResultActionByName(result, 'open');
      expectQueryLatencyWithinThreshold(result);
    });
  });

  group('T6: System Plugin Smoke Tests - Tier 2 (Conditional - requires environment)', () {
    // Requires deterministic default Google configuration and network access.
    testWidgets('T6-13: WebSearch plugin searches Google - requires default Google config', (tester) async {
      final controller = await launchAndShowLauncher(tester, windowSize: smokeLargeWindowSize);
      await queryAndWaitForResults(tester, controller, 'g wox launcher');

      expect(controller.activeResultViewController.items, isNotEmpty);
      final result = controller.activeResultViewController.activeItem.data;

      expect(result.title, equals('Search Google for wox launcher'));
    }, skip: true);

    // Requires a stable shell environment and command execution support.
    testWidgets('T6-14: Shell plugin executes shell commands - requires shell', (tester) async {
      final controller = await launchAndShowLauncher(tester, windowSize: smokeLargeWindowSize);
      await queryAndWaitForResults(tester, controller, '> echo hello');

      expect(controller.activeResultViewController.items, isNotEmpty);
      final result = controller.activeResultViewController.activeItem.data;

      expect(result.title.toLowerCase(), contains('echo hello'));
    }, skip: true);

    // Requires seeded typing history to make the plugin deterministic.
    testWidgets('T6-15: WPM plugin returns word count - requires typing session', (tester) async {
      final controller = await launchAndShowLauncher(tester, windowSize: smokeLargeWindowSize);
      await queryAndWaitForResults(tester, controller, 'wpm');

      expect(controller.activeResultViewController.items, isNotEmpty);
    }, skip: true);

    // Requires filesystem fixtures and predictable backup state.
    testWidgets('T6-16: Backup plugin returns backup options - requires filesystem', (tester) async {
      final controller = await launchAndShowLauncher(tester, windowSize: smokeLargeWindowSize);
      await queryAndWaitForResults(tester, controller, 'backup');

      expect(controller.activeResultViewController.items, isNotEmpty);
    }, skip: true);

    // Requires network access and a stable update endpoint response.
    testWidgets('T6-17: Update plugin checks for updates - requires network', (tester) async {
      final controller = await launchAndShowLauncher(tester, windowSize: smokeLargeWindowSize);
      await queryAndWaitForResults(tester, controller, 'update');

      expect(controller.activeResultViewController.items, isNotEmpty);
    }, skip: true);

    // Requires control over query history persistence for a fresh-install baseline.
    testWidgets('T6-18: Query History plugin does not crash on empty history - fresh install', (tester) async {
      final controller = await launchAndShowLauncher(tester, windowSize: smokeLargeWindowSize);
      await queryAndWaitForResults(tester, controller, 'h');

      // Should not crash - empty results are acceptable
      // Just verify no exception is thrown and window is still responsive
      expect(controller.activeResultViewController.items.isEmpty, isTrue);
    }, skip: true);

    // Requires platform application discovery fixtures and macOS-specific availability.
    testWidgets('T6-19: Application plugin finds platform applications - macOS only', (tester) async {
      final controller = await launchAndShowLauncher(tester, windowSize: smokeLargeWindowSize);
      // Use a common application name based on platform
      await queryAndWaitForResults(tester, controller, 'Finder');

      expect(controller.activeResultViewController.items, isNotEmpty);
    }, skip: true);

    // Requires deterministic clipboard history state.
    testWidgets('T6-20: Clipboard plugin handles clipboard history - no clipboard data', (tester) async {
      final controller = await launchAndShowLauncher(tester, windowSize: smokeLargeWindowSize);
      await queryAndWaitForResults(tester, controller, 'cb');

      // Should not crash - empty clipboard history is acceptable
      // Just verify no exception is thrown
      expect(controller.activeResultViewController.items.isEmpty, isTrue);
    }, skip: true);
  });
}

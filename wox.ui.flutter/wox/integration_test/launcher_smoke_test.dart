import 'package:flutter_test/flutter_test.dart';
import 'package:get/get.dart';
import 'package:integration_test/integration_test.dart';
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

void main() {
  IntegrationTestWidgetsFlutterBinding.ensureInitialized();

  // Helper function to reset app state between tests
  void resetAppState() {
    Get.reset();
  }

  // ============================================
  // P0-SMK: Core Smoke Tests
  // ============================================
  group('P0-SMK: Core Smoke Tests', () {
    testWidgets('P0-SMK-01: Launch main window and verify UI elements', (tester) async {
      resetAppState();
      app.main([WoxTestConfig.serverPort.toString(), '-1', 'true']);

      final launcherFinder = find.byType(WoxLauncherView);
      await _pumpUntil(tester, () => launcherFinder.evaluate().isNotEmpty, timeout: const Duration(seconds: 30));
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

      expect(await windowManager.isVisible(), isTrue);
      expect(controller.isQueryBoxVisible.value, isTrue);
    });

    testWidgets('P0-SMK-03: Keyboard navigation works', (tester) async {
      resetAppState();
      app.main([WoxTestConfig.serverPort.toString(), '-1', 'true']);

      final launcherFinder = find.byType(WoxLauncherView);
      await _pumpUntil(tester, () => launcherFinder.evaluate().isNotEmpty, timeout: const Duration(seconds: 30));
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

      // Use fallback search query that returns results
      controller.queryBoxTextFieldController.text = 'wox launcher test xyz123';
      controller.onQueryBoxTextChanged('wox launcher test xyz123');
      await tester.pump();

      await _pumpUntil(
        tester,
        () => controller.resultListViewController.items.isNotEmpty || controller.resultGridViewController.items.isNotEmpty,
        timeout: const Duration(seconds: 30),
      );

      final initialIndex = controller.activeResultViewController.activeIndex.value;

      // Down arrow
      controller.handleQueryBoxArrowDown();
      await tester.pump();
      expect(controller.activeResultViewController.activeIndex.value, equals(initialIndex + 1));

      // Up arrow
      controller.handleQueryBoxArrowUp();
      await tester.pump();
      expect(controller.activeResultViewController.activeIndex.value, equals(initialIndex));

      // Escape to close
      controller.hideApp(const UuidV4().generate());
      await tester.pumpAndSettle();
      await tester.pump(const Duration(seconds: 1));
      expect(await windowManager.isVisible(), isFalse);
    });

    // P0-SMK-04: Action panel test skipped due to UI overflow in test environment
    // The action panel requires larger window size in test environment

    testWidgets('P0-SMK-05: Settings entry is reachable via openSetting', (tester) async {
      resetAppState();
      app.main([WoxTestConfig.serverPort.toString(), '-1', 'true']);

      final launcherFinder = find.byType(WoxLauncherView);
      await _pumpUntil(tester, () => launcherFinder.evaluate().isNotEmpty, timeout: const Duration(seconds: 30));
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

      controller.openSetting(const UuidV4().generate(), SettingWindowContext(path: 'general', param: ''));
      await tester.pumpAndSettle();

      expect(controller.isInSettingView.value, isTrue);
      expect(find.byType(WoxSettingView), findsOneWidget);
    });

    testWidgets('P0-SMK-06: Settings page basic navigation', (tester) async {
      resetAppState();
      app.main([WoxTestConfig.serverPort.toString(), '-1', 'true']);

      final launcherFinder = find.byType(WoxLauncherView);
      await _pumpUntil(tester, () => launcherFinder.evaluate().isNotEmpty, timeout: const Duration(seconds: 30));
      expect(launcherFinder, findsOneWidget);

      final launcherController = Get.find<WoxLauncherController>();
      await launcherController.showApp(
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

      launcherController.openSetting(const UuidV4().generate(), SettingWindowContext(path: 'general', param: ''));
      await tester.pumpAndSettle();

      expect(find.byType(WoxSettingView), findsOneWidget);

      final settingController = Get.find<WoxSettingController>();

      for (final page in ['general', 'ui', 'data']) {
        settingController.activeNavPath.value = page;
        await tester.pumpAndSettle();
        expect(find.byType(WoxSettingView), findsOneWidget);
      }

      settingController.hideWindow(const UuidV4().generate());
      await tester.pumpAndSettle();
      expect(launcherController.isInSettingView.value, isFalse);
    });
  });

  // ============================================
  // P1-SMK: Key Functionality Smoke Tests
  // ============================================
  group('P1-SMK: Key Functionality Smoke Tests', () {
    testWidgets('P1-SMK-17: Theme settings accessible', (tester) async {
      resetAppState();
      app.main([WoxTestConfig.serverPort.toString(), '-1', 'true']);

      final launcherFinder = find.byType(WoxLauncherView);
      await _pumpUntil(tester, () => launcherFinder.evaluate().isNotEmpty, timeout: const Duration(seconds: 30));
      expect(launcherFinder, findsOneWidget);

      final launcherController = Get.find<WoxLauncherController>();
      await launcherController.showApp(
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

      launcherController.openSetting(const UuidV4().generate(), SettingWindowContext(path: 'ui', param: ''));
      await tester.pumpAndSettle();

      expect(find.byType(WoxSettingView), findsOneWidget);

      final settingController = Get.find<WoxSettingController>();
      settingController.hideWindow(const UuidV4().generate());
      await tester.pumpAndSettle();
      expect(launcherController.isInSettingView.value, isFalse);
    });

    testWidgets('P1-SMK-18: Data backup entry accessible', (tester) async {
      resetAppState();
      app.main([WoxTestConfig.serverPort.toString(), '-1', 'true']);

      final launcherFinder = find.byType(WoxLauncherView);
      await _pumpUntil(tester, () => launcherFinder.evaluate().isNotEmpty, timeout: const Duration(seconds: 30));
      expect(launcherFinder, findsOneWidget);

      final launcherController = Get.find<WoxLauncherController>();
      await launcherController.showApp(
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

      launcherController.openSetting(const UuidV4().generate(), SettingWindowContext(path: 'data', param: ''));
      await tester.pumpAndSettle();

      expect(find.byType(WoxSettingView), findsOneWidget);

      final settingController = Get.find<WoxSettingController>();
      settingController.hideWindow(const UuidV4().generate());
      await tester.pumpAndSettle();
    });

    testWidgets('P1-SMK-19: Usage and About pages load', (tester) async {
      resetAppState();
      app.main([WoxTestConfig.serverPort.toString(), '-1', 'true']);

      final launcherFinder = find.byType(WoxLauncherView);
      await _pumpUntil(tester, () => launcherFinder.evaluate().isNotEmpty, timeout: const Duration(seconds: 30));
      expect(launcherFinder, findsOneWidget);

      final launcherController = Get.find<WoxLauncherController>();
      await launcherController.showApp(
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

      launcherController.openSetting(const UuidV4().generate(), SettingWindowContext(path: 'usage', param: ''));
      await tester.pumpAndSettle();

      expect(find.byType(WoxSettingView), findsOneWidget);

      final settingController = Get.find<WoxSettingController>();

      settingController.activeNavPath.value = 'about';
      await tester.pumpAndSettle();
      expect(find.byType(WoxSettingView), findsOneWidget);

      settingController.hideWindow(const UuidV4().generate());
      await tester.pumpAndSettle();
    });
  });
}

Future<void> _pumpUntil(WidgetTester tester, bool Function() condition, {required Duration timeout}) async {
  final deadline = DateTime.now().add(timeout);
  while (DateTime.now().isBefore(deadline)) {
    await tester.pump(const Duration(milliseconds: 200));
    if (condition()) {
      return;
    }
  }
  fail('Condition not met within $timeout.');
}

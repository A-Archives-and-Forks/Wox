import 'package:extended_text_field/extended_text_field.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:get/get.dart';
import 'package:integration_test/integration_test.dart';
import 'package:uuid/v4.dart';
import 'package:wox/controllers/wox_launcher_controller.dart';
import 'package:wox/entity/wox_query.dart';
import 'package:wox/enums/wox_launch_mode_enum.dart';
import 'package:wox/enums/wox_position_type_enum.dart';
import 'package:wox/enums/wox_start_page_enum.dart';
import 'package:wox/main.dart' as app;
import 'package:wox/modules/launcher/views/wox_launcher_view.dart';
import 'package:wox/utils/windows/window_manager.dart';

const testServerPort = String.fromEnvironment('WOX_TEST_SERVER_PORT', defaultValue: '34987');

void main() {
  IntegrationTestWidgetsFlutterBinding.ensureInitialized();

  testWidgets('launcher shows and returns results for app query', (tester) async {
    app.main([testServerPort, '-1', 'true']);

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

    final queryBoxFinder = find.byType(ExtendedTextField);
    expect(queryBoxFinder, findsOneWidget);
    await tester.tap(queryBoxFinder);
    await tester.enterText(queryBoxFinder, 'app');
    await tester.pump();

    await _pumpUntil(
      tester,
      () => controller.resultListViewController.items.any((item) => !item.value.data.isGroup) || controller.resultGridViewController.items.any((item) => !item.value.data.isGroup),
      timeout: const Duration(seconds: 30),
    );

    final nonGroupResults =
        controller.resultListViewController.items.where((item) => !item.value.data.isGroup).length +
        controller.resultGridViewController.items.where((item) => !item.value.data.isGroup).length;
    expect(nonGroupResults, greaterThan(0));
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

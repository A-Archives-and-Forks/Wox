import 'package:flutter_test/flutter_test.dart';
import 'package:uuid/v4.dart';
import 'package:wox/entity/wox_toolbar.dart';

import 'smoke_test_helper.dart';

void registerLauncherToolbarStatusSmokeTests() {
  group('P1-SMK: Toolbar Status Smoke Tests', () {
    testWidgets('P1-SMK-20: Toolbar status stays visible without results and notify cannot override it', (tester) async {
      final controller = await launchAndShowLauncher(tester, windowSize: smokeLargeWindowSize);
      final traceId = const UuidV4().generate();
      final status = ToolbarStatusInfo(id: 'indexing', title: 'Indexing files', icon: null, progress: 40, indeterminate: false, actions: const []);

      await controller.showToolbarStatus(traceId, status);
      await tester.pump();

      expect(controller.activeResultViewController.items, isEmpty);
      expect(controller.isShowToolbar, isTrue);
      expect(controller.isToolbarShowedWithoutResults, isTrue);
      expect(controller.resolvedToolbarText, equals('Indexing files'));
      expect(controller.resolvedToolbarProgress, equals(40));

      controller.showToolbarMsg(traceId, ToolbarMsg(text: 'notify should not win'));
      await tester.pump();

      expect(controller.resolvedToolbarText, equals('Indexing files'));
      expect(controller.toolbar.value.text == 'notify should not win', isFalse);
    });

    testWidgets('P1-SMK-21: Status actions override conflicting result hotkeys and restore after clear', (tester) async {
      final controller = await launchAndShowLauncher(tester, windowSize: smokeLargeWindowSize);
      final result = await queryAndWaitForActiveResult(tester, controller, '1+1');
      final copyAction = expectResultActionByName(result, 'copy');

      expect(copyAction.hotkey, isNotEmpty);

      final traceId = const UuidV4().generate();
      final status = ToolbarStatusInfo(
        id: 'calculator-status',
        title: 'Calculating',
        icon: null,
        progress: null,
        indeterminate: true,
        actions: [
          ToolbarStatusActionInfo(id: 'retry', name: 'Retry', icon: null, hotkey: copyAction.hotkey, isDefault: false, preventHideAfterAction: true, contextData: const {}),
        ],
      );

      await controller.showToolbarStatus(traceId, status);
      await tester.pump();

      final statusWinner = controller.getActionByToolbarHotkey(result, copyAction.hotkey);
      expect(statusWinner, isNotNull);
      expect(statusWinner!.name, equals('Retry'));

      final unifiedActionsWithStatus = controller.buildUnifiedActions(traceId, result);
      final restoredCopyWhileStatusVisible = unifiedActionsWithStatus.firstWhere((action) => action.name == copyAction.name);
      expect(restoredCopyWhileStatusVisible.hotkey, isEmpty);

      await controller.clearToolbarStatus(traceId);
      await tester.pump();

      final restoredWinner = controller.getActionByToolbarHotkey(result, copyAction.hotkey);
      expect(restoredWinner, isNotNull);
      expect(restoredWinner!.name, equals(copyAction.name));
      expect(controller.buildUnifiedActions(traceId, result).any((action) => action.name == 'Retry'), isFalse);
    });
  });
}

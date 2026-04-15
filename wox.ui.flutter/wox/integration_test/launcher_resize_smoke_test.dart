import 'dart:io';

import 'package:integration_test/integration_test.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:wox/controllers/wox_launcher_controller.dart';
import 'package:wox/entity/wox_image.dart';
import 'package:wox/entity/wox_preview.dart';
import 'package:wox/entity/wox_query.dart';
import 'package:wox/enums/wox_query_type_enum.dart';
import 'package:wox/utils/windows/window_manager.dart';

import 'smoke_test_helper.dart';

void main() {
  IntegrationTestWidgetsFlutterBinding.ensureInitialized();
  registerLauncherResizeSmokeTests();
}

void registerLauncherResizeSmokeTests() {
  group('T7: Resize Smoke Tests', () {
    testWidgets(
      'T7-01: repeated result grow/shrink cycles keep native window height in sync',
      (tester) async {
        if (!Platform.isWindows) {
          return;
        }

        final controller = await launchAndShowLauncher(
          tester,
          windowSize: smokeLargeWindowSize,
        );

        for (var i = 0; i < 10; i++) {
          await queryAndWaitForResults(
            tester,
            controller,
            'wox launcher test xyz123',
          );
          await waitForWindowHeightToMatchController(tester, controller);
          final heightWithResults = (await windowManager.getSize()).height;

          tester.testTextInput.enterText('');
          await tester.pump();
          await waitForNoActiveResults(tester, controller);
          await waitForWindowHeightToMatchController(tester, controller);
          final heightWithoutResults = (await windowManager.getSize()).height;

          expect(heightWithResults, greaterThan(heightWithoutResults));
        }
      },
    );

    testWidgets(
      'T7-02: partial query updates do not shrink the visible window before the final snapshot arrives',
      (tester) async {
        if (!Platform.isWindows) {
          return;
        }

        final controller = await launchAndShowLauncher(
          tester,
          windowSize: smokeLargeWindowSize,
        );
        const traceId = 'resize-smoke-partial-results';
        const initialQueryId = 'resize-smoke-initial-query';
        const updatedQueryId = 'resize-smoke-updated-query';

        controller.currentQuery.value = PlainQuery(
          queryId: initialQueryId,
          queryType: WoxQueryTypeEnum.WOX_QUERY_TYPE_INPUT.code,
          queryText: 'settin',
          querySelection: Selection.empty(),
        );
        controller.onReceivedQueryResults(
          traceId,
          initialQueryId,
          buildSyntheticResults(initialQueryId, 4),
        );
        await waitForWindowHeightToMatchController(tester, controller);
        final stableHeight = (await windowManager.getSize()).height;

        controller.currentQuery.value = PlainQuery(
          queryId: updatedQueryId,
          queryType: WoxQueryTypeEnum.WOX_QUERY_TYPE_INPUT.code,
          queryText: 'setting',
          querySelection: Selection.empty(),
        );
        controller.isCurrentQueryReturned = false;
        controller.isCurrentQuerySettled = false;
        controller.preserveVisibleQueryWindowHeight(
          traceId,
          updatedQueryId,
          stableHeight,
        );
        controller.onReceivedQueryResults(
          traceId,
          updatedQueryId,
          buildSyntheticResults(updatedQueryId, 1),
          isFinal: false,
        );

        final preservationDeadline = DateTime.now().add(
          WoxLauncherController.visibleQueryHeightPreservationDuration -
              const Duration(milliseconds: 40),
        );
        while (DateTime.now().isBefore(preservationDeadline)) {
          await tester.pump(const Duration(milliseconds: 20));
          final actualHeight = (await windowManager.getSize()).height;
          if ((actualHeight - stableHeight).abs() <= 2) {
            break;
          }
        }
        final heightDuringPartialResults =
            (await windowManager.getSize()).height;
        expect(
          heightDuringPartialResults,
          greaterThanOrEqualTo(stableHeight - 2),
        );

        controller.onReceivedQueryResults(
          traceId,
          updatedQueryId,
          buildSyntheticResults(updatedQueryId, 4),
          isFinal: true,
        );
        await waitForWindowHeightToMatchController(tester, controller);
        final finalHeight = (await windowManager.getSize()).height;
        expect((finalHeight - stableHeight).abs(), lessThanOrEqualTo(2));
      },
    );

    testWidgets(
      'T7-03: partial query updates release preserved window height after the grace window expires',
      (tester) async {
        if (!Platform.isWindows) {
          return;
        }

        final controller = await launchAndShowLauncher(
          tester,
          windowSize: smokeLargeWindowSize,
        );
        const traceId = 'resize-smoke-partial-results-timeout';
        const initialQueryId = 'resize-smoke-timeout-initial-query';
        const updatedQueryId = 'resize-smoke-timeout-updated-query';

        controller.currentQuery.value = PlainQuery(
          queryId: initialQueryId,
          queryType: WoxQueryTypeEnum.WOX_QUERY_TYPE_INPUT.code,
          queryText: 'settin',
          querySelection: Selection.empty(),
        );
        controller.onReceivedQueryResults(
          traceId,
          initialQueryId,
          buildSyntheticResults(initialQueryId, 4),
        );
        await waitForWindowHeightToMatchController(tester, controller);
        final stableHeight = (await windowManager.getSize()).height;

        controller.currentQuery.value = PlainQuery(
          queryId: updatedQueryId,
          queryType: WoxQueryTypeEnum.WOX_QUERY_TYPE_INPUT.code,
          queryText: 'setting',
          querySelection: Selection.empty(),
        );
        controller.isCurrentQueryReturned = false;
        controller.isCurrentQuerySettled = false;
        controller.preserveVisibleQueryWindowHeight(
          traceId,
          updatedQueryId,
          stableHeight,
        );
        controller.onReceivedQueryResults(
          traceId,
          updatedQueryId,
          buildSyntheticResults(updatedQueryId, 1),
          isFinal: false,
        );

        await tester.pump(
          WoxLauncherController.visibleQueryHeightPreservationDuration +
              const Duration(milliseconds: 250),
        );
        await waitForWindowHeightToMatchController(tester, controller);
        final heightAfterGraceWindow = (await windowManager.getSize()).height;

        expect(heightAfterGraceWindow, lessThan(stableHeight - 2));
      },
    );
  });
}

List<WoxQueryResult> buildSyntheticResults(String queryId, int count) {
  return List.generate(count, (index) {
    return WoxQueryResult(
      queryId: queryId,
      id: '$queryId-$index',
      title: 'Synthetic Result $index',
      subTitle: 'Synthetic Subtitle $index',
      icon: WoxImage.empty(),
      preview: WoxPreview.empty(),
      score: 100 - index,
      group: '',
      groupScore: 0,
      tails: const [],
      actions: const [],
      isGroup: false,
    );
  });
}

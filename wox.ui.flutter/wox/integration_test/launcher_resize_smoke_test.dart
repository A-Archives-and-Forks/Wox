import 'dart:io';

import 'package:integration_test/integration_test.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:wox/entity/wox_image.dart';
import 'package:wox/entity/wox_preview.dart';
import 'package:wox/entity/wox_query.dart';
import 'package:wox/enums/wox_query_type_enum.dart';
import 'package:wox/enums/wox_start_page_enum.dart';
import 'package:wox/utils/windows/window_manager.dart';

import 'smoke_test_helper.dart';

void main() {
  IntegrationTestWidgetsFlutterBinding.ensureInitialized();
  registerLauncherResizeSmokeTests();
}

void registerLauncherResizeSmokeTests() {
  group('T7: Resize Smoke Tests', () {
    testWidgets('T7-01: non-empty query keeps one expanded height across result-count changes', (tester) async {
      if (!Platform.isWindows) {
        return;
      }

      final controller = await launchAndShowLauncher(tester, windowSize: smokeLargeWindowSize);
      const traceId = 'resize-smoke-expanded-height-stability';
      const queryId = 'resize-smoke-expanded-query';

      final compactHeight = controller.calculateWindowHeight();
      await controller.onQueryChanged(
        traceId,
        PlainQuery(queryId: queryId, queryType: WoxQueryTypeEnum.WOX_QUERY_TYPE_INPUT.code, queryText: 'setting', querySelection: Selection.empty()),
        'resize smoke test',
      );
      await waitForWindowHeightToMatchController(tester, controller);

      final observedHeights = <double>[];
      for (final resultCount in [4, 1, 0, 6]) {
        controller.onReceivedQueryResults(traceId, queryId, resultCount == 0 ? const [] : buildSyntheticResults(queryId, resultCount));
        await waitForWindowHeightToMatchController(tester, controller);
        observedHeights.add((await windowManager.getSize()).height);
      }

      final expandedHeight = observedHeights.first;
      expect(expandedHeight, greaterThan(compactHeight + 2));
      for (final height in observedHeights.skip(1)) {
        expect((height - expandedHeight).abs(), lessThanOrEqualTo(2));
      }
    });

    testWidgets('T7-02: clearing a non-empty query returns blank start page to compact height', (tester) async {
      if (!Platform.isWindows) {
        return;
      }

      final controller = await launchAndShowLauncher(tester, windowSize: smokeLargeWindowSize);
      const traceId = 'resize-smoke-blank-start-page';
      const queryId = 'resize-smoke-blank-query';
      final compactHeight = controller.calculateWindowHeight();
      await controller.onQueryChanged(
        traceId,
        PlainQuery(queryId: queryId, queryType: WoxQueryTypeEnum.WOX_QUERY_TYPE_INPUT.code, queryText: 'setting', querySelection: Selection.empty()),
        'resize smoke test',
      );
      controller.onReceivedQueryResults(traceId, queryId, buildSyntheticResults(queryId, 4));
      await waitForWindowHeightToMatchController(tester, controller);
      final expandedHeight = (await windowManager.getSize()).height;

      controller.lastStartPage = WoxStartPageEnum.WOX_START_PAGE_BLANK.code;
      await controller.onQueryChanged(traceId, PlainQuery.emptyInput(), 'resize smoke clear query');
      await waitForWindowHeightToMatchController(tester, controller);
      final compactHeightAfterClear = (await windowManager.getSize()).height;

      expect(expandedHeight, greaterThan(compactHeight + 2));
      expect((compactHeightAfterClear - compactHeight).abs(), lessThanOrEqualTo(2));
    });

    testWidgets('T7-03: empty MRU start page keeps expanded height even without results', (tester) async {
      if (!Platform.isWindows) {
        return;
      }

      final controller = await launchAndShowLauncher(tester, windowSize: smokeLargeWindowSize);
      const traceId = 'resize-smoke-empty-mru';
      const queryId = 'resize-smoke-mru-query';
      final compactHeight = controller.calculateWindowHeight();
      await controller.onQueryChanged(
        traceId,
        PlainQuery(queryId: queryId, queryType: WoxQueryTypeEnum.WOX_QUERY_TYPE_INPUT.code, queryText: 'setting', querySelection: Selection.empty()),
        'resize smoke test',
      );
      controller.onReceivedQueryResults(traceId, queryId, buildSyntheticResults(queryId, 4));
      await waitForWindowHeightToMatchController(tester, controller);
      final expandedHeight = (await windowManager.getSize()).height;

      controller.lastStartPage = WoxStartPageEnum.WOX_START_PAGE_MRU.code;
      await controller.onQueryChanged(traceId, PlainQuery.emptyInput(), 'resize smoke clear query');
      await waitForWindowHeightToMatchController(tester, controller);
      final expandedHeightWithoutResults = (await windowManager.getSize()).height;

      expect(expandedHeight, greaterThan(compactHeight + 2));
      expect((expandedHeightWithoutResults - expandedHeight).abs(), lessThanOrEqualTo(2));
    });
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

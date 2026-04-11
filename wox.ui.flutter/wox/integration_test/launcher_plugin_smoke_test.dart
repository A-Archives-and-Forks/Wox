import 'dart:io';

import 'package:flutter_test/flutter_test.dart';
import 'package:uuid/v4.dart';
import 'package:wox/api/wox_api.dart';
import 'package:wox/enums/wox_preview_type_enum.dart';

import 'smoke_test_helper.dart';

const String _testNodeTemplatePluginIdEnv = 'WOX_TEST_NODE_TEMPLATE_PLUGIN_ID';
const String _testNodeTemplatePluginNameEnv = 'WOX_TEST_NODE_TEMPLATE_PLUGIN_NAME';
const String _testNodeTemplatePluginTriggerKeywordEnv = 'WOX_TEST_NODE_TEMPLATE_PLUGIN_TRIGGER_KEYWORD';
const String _testPythonTemplatePluginIdEnv = 'WOX_TEST_PYTHON_TEMPLATE_PLUGIN_ID';
const String _testPythonTemplatePluginNameEnv = 'WOX_TEST_PYTHON_TEMPLATE_PLUGIN_NAME';
const String _testPythonTemplatePluginTriggerKeywordEnv = 'WOX_TEST_PYTHON_TEMPLATE_PLUGIN_TRIGGER_KEYWORD';

class _SmokeTemplatePluginConfig {
  const _SmokeTemplatePluginConfig({required this.id, required this.name, required this.triggerKeyword, required this.runtime});

  final String id;
  final String name;
  final String triggerKeyword;
  final String runtime;

  static _SmokeTemplatePluginConfig? fromEnvironment({required String runtime, required String idEnvKey, required String nameEnvKey, required String triggerKeywordEnvKey}) {
    final id = Platform.environment[idEnvKey]?.trim() ?? '';
    final name = Platform.environment[nameEnvKey]?.trim() ?? '';
    final triggerKeyword = Platform.environment[triggerKeywordEnvKey]?.trim() ?? '';
    if (id.isEmpty || name.isEmpty || triggerKeyword.isEmpty) {
      return null;
    }

    return _SmokeTemplatePluginConfig(id: id, name: name, triggerKeyword: triggerKeyword, runtime: runtime);
  }
}

void registerLauncherPluginSmokeTests() {
  group('T4: Template Plugin Smoke Tests', () {
    testWidgets('T4-01: Packaged Nodejs template plugin loads and basic behaviors work', (tester) async {
      final config = _SmokeTemplatePluginConfig.fromEnvironment(
        runtime: 'nodejs',
        idEnvKey: _testNodeTemplatePluginIdEnv,
        nameEnvKey: _testNodeTemplatePluginNameEnv,
        triggerKeywordEnvKey: _testNodeTemplatePluginTriggerKeywordEnv,
      );
      if (config == null) {
        return;
      }

      final controller = await launchAndShowLauncher(tester, windowSize: smokeLargeWindowSize);
      final installedPlugins = await WoxApi.instance.findInstalledPlugins(const UuidV4().generate());
      final installedPlugin = installedPlugins.where((plugin) => plugin.id == config.id).toList();
      expect(installedPlugin, hasLength(1));
      expect(installedPlugin.first.name, equals(config.name));
      expect(installedPlugin.first.runtime, equals(config.runtime));
      expect(installedPlugin.first.isInstalled, isTrue);
      expect(installedPlugin.first.triggerKeywords, contains(config.triggerKeyword));

      const search = 'smoke-check';

      await queryAndWaitForResults(tester, controller, '${config.triggerKeyword} $search');

      expect(controller.activeResultViewController.items, isNotEmpty);
      final result = controller.activeResultViewController.activeItem.data;

      expect(result.title, equals('Hello World $search'));
      expect(result.subTitle, equals('This is a subtitle'));
      expect(result.preview.previewType, equals(WoxPreviewTypeEnum.WOX_PREVIEW_TYPE_TEXT.code));
      expect(result.preview.previewData, equals('This is a preview'));
      expect(result.preview.previewProperties['Property1'], equals('Hello World'));
      expect(result.preview.previewProperties['Property2'], equals('This is a property'));
      expect(result.tails, hasLength(1));
      expect(result.tails.first.text, equals('This is a tail'));
      final openActions = result.actions.where((action) => action.name == 'Open').toList();
      expect(openActions, isNotEmpty);
      expect(openActions.first.contextData['search'], equals(search));

      controller.executeDefaultAction(const UuidV4().generate());
      await waitForWindowVisibility(tester, false);
    });

    testWidgets('T4-02: Packaged Python template plugin loads and basic behaviors work', (tester) async {
      final config = _SmokeTemplatePluginConfig.fromEnvironment(
        runtime: 'python',
        idEnvKey: _testPythonTemplatePluginIdEnv,
        nameEnvKey: _testPythonTemplatePluginNameEnv,
        triggerKeywordEnvKey: _testPythonTemplatePluginTriggerKeywordEnv,
      );
      if (config == null) {
        return;
      }

      final controller = await launchAndShowLauncher(tester, windowSize: smokeLargeWindowSize);
      final installedPlugins = await WoxApi.instance.findInstalledPlugins(const UuidV4().generate());
      final installedPlugin = installedPlugins.where((plugin) => plugin.id == config.id).toList();
      expect(installedPlugin, hasLength(1));
      expect(installedPlugin.first.name, equals(config.name));
      expect(installedPlugin.first.runtime, equals(config.runtime));
      expect(installedPlugin.first.isInstalled, isTrue);
      expect(installedPlugin.first.triggerKeywords, contains(config.triggerKeyword));

      const search = 'SmOkE-ChEcK';

      await queryAndWaitForResults(tester, controller, '${config.triggerKeyword} $search');

      expect(controller.activeResultViewController.items, isNotEmpty);
      final result = controller.activeResultViewController.activeItem.data;

      expect(result.title, equals('you typed smoke-check'));
      expect(result.subTitle, equals('this is subsitle'));
      expect(result.tails, isEmpty);

      final myActions = result.actions.where((action) => action.name == 'My Action').toList();
      expect(myActions, isNotEmpty);
      expect(myActions.first.contextData['search_term'], equals('smoke-check'));
      expect(myActions.first.preventHideAfterAction, isTrue);

      controller.executeDefaultAction(const UuidV4().generate());
      await waitForWindowVisibility(tester, true);
    });
  });
}

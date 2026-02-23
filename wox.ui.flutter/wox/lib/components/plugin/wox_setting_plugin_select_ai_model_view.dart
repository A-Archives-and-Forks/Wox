import 'package:flutter/material.dart';
import 'package:wox/components/wox_ai_model_selector_view.dart';
import 'package:wox/entity/setting/wox_plugin_setting_select_ai_model.dart';

import 'wox_setting_plugin_item_view.dart';

class WoxSettingPluginSelectAIModel extends WoxSettingPluginItem {
  final PluginSettingValueSelectAIModel item;

  const WoxSettingPluginSelectAIModel({super.key, required this.item, required super.value, required super.onUpdate, required super.labelWidth});

  @override
  Widget build(BuildContext context) {
    return layout(
      label: item.label,
      child: Wrap(
        crossAxisAlignment: WrapCrossAlignment.center,
        children: [
          Padding(
            padding: const EdgeInsets.only(top: 6),
            child: WoxAIModelSelectorView(
              initialValue: value,
              onModelSelected: (modelJson) {
                updateConfig(item.key, modelJson);
              },
            ),
          ),
          suffix(item.suffix),
        ],
      ),
      style: item.style,
      tooltip: item.tooltip,
    );
  }
}

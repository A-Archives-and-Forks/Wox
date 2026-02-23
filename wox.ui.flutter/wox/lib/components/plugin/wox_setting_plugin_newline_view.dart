import 'package:flutter/material.dart';
import 'package:wox/entity/setting/wox_plugin_setting_newline.dart';
import 'package:wox/utils/consts.dart';

import 'wox_setting_plugin_item_view.dart';

class WoxSettingPluginNewLine extends WoxSettingPluginItem {
  final PluginSettingValueNewLine item;

  const WoxSettingPluginNewLine({super.key, required this.item, required super.value, required super.onUpdate, super.labelWidth = SETTING_LABEL_DEAULT_WIDTH});

  @override
  Widget build(BuildContext context) {
    return layout(label: "", child: const Padding(padding: EdgeInsets.all(4), child: Row(children: [SizedBox(width: 1)])), style: item.style, includeBottomSpacing: false);
  }
}

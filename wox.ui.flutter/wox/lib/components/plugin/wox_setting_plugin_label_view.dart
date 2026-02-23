import 'package:flutter/material.dart';
import 'package:wox/entity/setting/wox_plugin_setting_label.dart';
import 'package:wox/utils/colors.dart';
import 'package:wox/utils/consts.dart';

import 'wox_setting_plugin_item_view.dart';

class WoxSettingPluginLabel extends WoxSettingPluginItem {
  final PluginSettingValueLabel item;

  const WoxSettingPluginLabel({super.key, required this.item, required super.value, required super.onUpdate, super.labelWidth = SETTING_LABEL_DEAULT_WIDTH});

  @override
  Widget build(BuildContext context) {
    return layout(
      label: "",
      child: Text(item.content, style: TextStyle(color: getThemeTextColor(), fontSize: 13)),
      style: item.style,
      tooltip: item.tooltip,
      includeBottomSpacing: false,
    );
  }
}

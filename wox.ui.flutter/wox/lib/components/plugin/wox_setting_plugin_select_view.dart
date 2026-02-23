import 'package:flutter/material.dart';
import 'package:wox/components/wox_dropdown_button.dart';
import 'package:wox/entity/setting/wox_plugin_setting_select.dart';

import 'wox_setting_plugin_item_view.dart';

class WoxSettingPluginSelect extends WoxSettingPluginItem {
  final PluginSettingValueSelect item;

  const WoxSettingPluginSelect({super.key, required this.item, required super.value, required super.onUpdate, required super.labelWidth});

  @override
  Widget build(BuildContext context) {
    final dropdownWidth = item.style.width > 0 ? item.style.width.toDouble() : null;
    return layout(
      label: item.label,
      child: Wrap(
        crossAxisAlignment: WrapCrossAlignment.center,
        children: [
          WoxDropdownButton<String>(
            value: getSetting(item.key),
            isExpanded: true,
            width: dropdownWidth,
            items:
                item.options.map((e) {
                  return WoxDropdownItem(value: e.value, label: e.label);
                }).toList(),
            onChanged: (v) {
              updateConfig(item.key, v ?? "");
            },
          ),
          suffix(item.suffix),
        ],
      ),
      style: item.style,
      tooltip: item.tooltip,
    );
  }
}

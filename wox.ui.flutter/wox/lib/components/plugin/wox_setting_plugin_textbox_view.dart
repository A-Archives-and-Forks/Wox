import 'package:flutter/material.dart';
import 'package:wox/components/wox_textfield.dart';
import 'package:wox/entity/setting/wox_plugin_setting_textbox.dart';

import 'wox_setting_plugin_item_view.dart';

class WoxSettingPluginTextBox extends WoxSettingPluginItem {
  final PluginSettingValueTextBox item;
  final controller = TextEditingController();

  WoxSettingPluginTextBox({super.key, required this.item, required super.value, required super.onUpdate, required super.labelWidth}) {
    controller.text = getSetting(item.key);
    if (item.maxLines < 1) {
      item.maxLines = 1;
    }
  }

  @override
  Widget build(BuildContext context) {
    final inputWidth = item.style.width > 0 ? item.style.width.toDouble() : 100.0;
    return layout(
      label: item.label,
      child: Wrap(
        crossAxisAlignment: WrapCrossAlignment.center,
        children: [
          Focus(
            onFocusChange: (hasFocus) {
              if (!hasFocus) {
                for (var element in item.validators) {
                  var errMsg = element.validator.validate(controller.text);
                  item.tooltip = errMsg;
                  if (errMsg != "") {
                    return;
                  }
                }

                updateConfig(item.key, controller.text);
              }
            },
            child: WoxTextField(
              maxLines: item.maxLines,
              controller: controller,
              width: inputWidth,
              onChanged: (value) {
                for (var element in item.validators) {
                  var errMsg = element.validator.validate(value);
                  item.tooltip = errMsg;
                  break;
                }
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

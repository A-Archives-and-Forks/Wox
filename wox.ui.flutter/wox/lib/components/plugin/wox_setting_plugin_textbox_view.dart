import 'package:fluent_ui/fluent_ui.dart';
import 'package:wox/components/wox_tooltip_view.dart';
import 'package:wox/entity/setting/wox_plugin_setting_textbox.dart';
import 'package:wox/utils/wox_theme_util.dart';
import 'package:wox/utils/color_util.dart';

import 'wox_setting_plugin_item_view.dart';

class WoxSettingPluginTextBox extends WoxSettingPluginItem {
  final PluginSettingValueTextBox item;
  final controller = TextEditingController();

  WoxSettingPluginTextBox({super.key, required this.item, required super.value, required super.onUpdate}) {
    controller.text = getSetting(item.key);
    if (item.maxLines < 1) {
      item.maxLines = 1;
    }
  }

  @override
  Widget build(BuildContext context) {
    return layout(
      children: [
        label(item.label, item.style),
        if (item.tooltip != "") WoxTooltipView(tooltip: item.tooltip, paddingLeft: 0, color: safeFromCssColor(WoxThemeUtil.instance.currentTheme.value.resultItemTitleColor)),
        SizedBox(
          width: item.style.width > 0 ? item.style.width.toDouble() : 100,
          child: Focus(
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
            child: TextBox(
              maxLines: item.maxLines,
              controller: controller,
              onChanged: (value) {
                for (var element in item.validators) {
                  var errMsg = element.validator.validate(value);
                  item.tooltip = errMsg;
                  break;
                }
              },
            ),
          ),
        ),
        suffix(item.suffix),
      ],
      style: item.style,
    );
  }
}

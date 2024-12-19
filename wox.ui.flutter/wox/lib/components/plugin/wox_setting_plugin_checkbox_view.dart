import 'package:fluent_ui/fluent_ui.dart';
import 'package:wox/entity/setting/wox_plugin_setting_checkbox.dart';

import 'wox_setting_plugin_item_view.dart';

class WoxSettingPluginCheckbox extends WoxSettingPluginItem {
  final PluginSettingValueCheckBox item;

  const WoxSettingPluginCheckbox({super.key, required this.item, required super.value, required super.onUpdate});

  @override
  Widget build(BuildContext context) {
    return layout(
      children: [
        label(item.label, item.style),
        ToggleSwitch(
          checked: getSetting(item.key) == "true",
          onChanged: (value) {
            if (value == true) {
              updateConfig(item.key, "true");
            } else {
              updateConfig(item.key, "false");
            }
          },
        ),
      ],
      style: item.style,
    );
  }
}

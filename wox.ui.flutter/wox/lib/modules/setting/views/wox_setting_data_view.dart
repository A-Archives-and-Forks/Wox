import 'package:flutter/material.dart' hide DataTable;
import 'package:get/get.dart';
import 'package:uuid/v4.dart';
import 'package:wox/components/wox_button.dart';
import 'package:wox/components/wox_dropdown_button.dart';
import 'package:wox/components/wox_loading_indicator.dart';
import 'package:wox/components/wox_switch.dart';
import 'package:wox/modules/setting/views/wox_setting_base.dart';
import 'package:wox/utils/colors.dart';
import 'package:wox/utils/consts.dart';
import 'package:flutter/material.dart' as material;
import 'package:wox/api/wox_api.dart';
import 'package:wox/utils/picker.dart';
import 'package:wox/utils/wox_setting_focus_util.dart';

class WoxSettingDataView extends WoxSettingBaseView {
  const WoxSettingDataView({super.key});

  Widget _buildAutoBackupTips() {
    return Wrap(
      crossAxisAlignment: WrapCrossAlignment.center,
      children: [
        Text(controller.tr("ui_data_backup_auto_tips_prefix"), style: TextStyle(color: getThemeSubTextColor(), fontSize: 13)),
        WoxButton.text(
          text: controller.tr("ui_data_backup_folder_link"),
          onPressed: () async {
            try {
              final backupPath = await WoxApi.instance.getBackupFolder(const UuidV4().generate());
              await controller.openFolder(backupPath);
            } catch (e) {
              // Handle error silently or show a notification
            }
          },
        ),
        Text(controller.tr("ui_data_backup_auto_tips_suffix"), style: TextStyle(color: getThemeSubTextColor(), fontSize: 13)),
      ],
    );
  }

  Widget _buildBackupListTable(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.only(bottom: 28),
      child: SizedBox(
        width: GENERAL_SETTING_TABLE_WIDTH,
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Row(
              children: [
                Text(controller.tr("ui_data_backup_list_title"), style: TextStyle(color: getThemeTextColor(), fontSize: 13, fontWeight: FontWeight.w500)),
                const Spacer(),
                // Keep backup creation visually aligned with table Add buttons: compact, outlined, and anchored to the table header edge.
                Obx(() {
                  final isBackingUp = controller.isBackingUp.value;
                  // Manual backups can run long enough for users to click again. Disabling the
                  // button uses the existing gray disabled style, while the spinner shows that
                  // the click was accepted without adding another dialog or background state.
                  return WoxButton.secondary(
                    text: controller.tr("ui_data_backup_now"),
                    icon: isBackingUp ? WoxLoadingIndicator(size: 14, color: getThemeTextColor().withValues(alpha: 0.5)) : Icon(Icons.add, color: getThemeSubTextColor()),
                    height: 30,
                    padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 6),
                    onPressed:
                        isBackingUp
                            ? null
                            : () {
                              controller.backupNow();
                            },
                  );
                }),
              ],
            ),
            const SizedBox(height: 10),
            Obx(() {
              if (controller.backups.isEmpty) {
                return Padding(padding: const EdgeInsets.symmetric(vertical: 20), child: Center(child: Text(controller.tr("ui_data_backup_empty"))));
              }

              // Material DataTable sizes itself to its intrinsic content by default. Pinning it to
              // the section width keeps backup history aligned with other full-width table settings.
              return SizedBox(
                width: double.infinity,
                child: material.DataTable(
                  columnSpacing: 10,
                  horizontalMargin: 5,
                  headingRowHeight: 36,
                  dataRowMinHeight: 36,
                  dataRowMaxHeight: 36,
                  headingRowColor: material.WidgetStateProperty.resolveWith((states) => getThemeTextColor().withValues(alpha: 0.055)),
                  dataRowColor: material.WidgetStateProperty.resolveWith((states) => getThemeTextColor().withValues(alpha: 0.018)),
                  border: TableBorder.all(color: getThemeDividerColor().withValues(alpha: 0.58)),
                  columns: [
                    material.DataColumn(
                      label: Expanded(
                        child: Text(
                          controller.tr("ui_data_backup_date"),
                          style: TextStyle(overflow: TextOverflow.ellipsis, color: getThemeTextColor().withValues(alpha: 0.88), fontSize: 13, fontWeight: FontWeight.w600),
                        ),
                      ),
                    ),
                    material.DataColumn(
                      label: Expanded(
                        child: Text(
                          controller.tr("ui_data_backup_type"),
                          style: TextStyle(overflow: TextOverflow.ellipsis, color: getThemeTextColor().withValues(alpha: 0.88), fontSize: 13, fontWeight: FontWeight.w600),
                        ),
                      ),
                    ),
                    material.DataColumn(
                      label: Text(
                        controller.tr("ui_operation"),
                        style: TextStyle(overflow: TextOverflow.ellipsis, color: getThemeTextColor().withValues(alpha: 0.88), fontSize: 13, fontWeight: FontWeight.w600),
                      ),
                    ),
                  ],
                  rows:
                      controller.backups.map((backup) {
                        final date = DateTime.fromMillisecondsSinceEpoch(backup.timestamp);
                        final dateStr =
                            '${date.year}-${date.month.toString().padLeft(2, '0')}-${date.day.toString().padLeft(2, '0')} ${date.hour.toString().padLeft(2, '0')}:${date.minute.toString().padLeft(2, '0')}:${date.second.toString().padLeft(2, '0')}';

                        return material.DataRow(
                          cells: [
                            material.DataCell(Text(dateStr, style: TextStyle(overflow: TextOverflow.ellipsis, color: getThemeTextColor()))),
                            material.DataCell(
                              Text(
                                backup.type == "auto" ? controller.tr("ui_data_backup_type_auto") : controller.tr("ui_data_backup_type_manual"),
                                style: TextStyle(overflow: TextOverflow.ellipsis, color: getThemeTextColor()),
                              ),
                            ),
                            material.DataCell(
                              Row(
                                children: [
                                  WoxButton.text(
                                    text: controller.tr("ui_data_backup_restore"),
                                    onPressed: () async {
                                      await showDialog(
                                        context: context,
                                        barrierColor: getThemePopupBarrierColor(),
                                        builder: (context) {
                                          return AlertDialog(
                                            backgroundColor: getThemePopupSurfaceColor(),
                                            surfaceTintColor: Colors.transparent,
                                            shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(20), side: BorderSide(color: getThemePopupOutlineColor())),
                                            title: Text(controller.tr("ui_data_backup_restore_confirm_title")),
                                            content: Text(controller.tr("ui_data_backup_restore_confirm_message")),
                                            actions: [
                                              WoxButton.secondary(
                                                text: controller.tr("ui_data_backup_restore_cancel"),
                                                onPressed: () {
                                                  Navigator.pop(context);
                                                },
                                              ),
                                              WoxButton.primary(
                                                text: controller.tr("ui_data_backup_restore_confirm"),
                                                onPressed: () {
                                                  Navigator.pop(context);
                                                  controller.restoreBackup(backup.id);
                                                },
                                              ),
                                            ],
                                          );
                                        },
                                      );
                                      WoxSettingFocusUtil.restoreIfInSettingView();
                                    },
                                  ),
                                  WoxButton.text(
                                    text: controller.tr("plugin_file_open"),
                                    onPressed: () {
                                      controller.openFolder(backup.path);
                                    },
                                  ),
                                ],
                              ),
                            ),
                          ],
                        );
                      }).toList(),
                ),
              );
            }),
          ],
        ),
      ),
    );
  }

  @override
  Widget build(BuildContext context) {
    return form(
      width: GENERAL_SETTING_WIDE_FORM_WIDTH,
      title: controller.tr("ui_data"),
      description: controller.tr("ui_data_description"),
      children: [
        formSection(
          title: controller.tr("ui_data_section_storage"),
          children: [
            formField(
              label: controller.tr("ui_data_config_location"),
              labelWidth: GENERAL_SETTING_WIDE_LABEL_WIDTH,
              child: Obx(
                () => Row(
                  mainAxisSize: MainAxisSize.min,
                  children: [
                    // The full path is noisy in this dense settings page. Keep the two actions
                    // users need and leave the explanatory copy to describe what location changes do.
                    WoxButton.secondary(
                      text: controller.tr("plugin_file_open"),
                      onPressed: () {
                        controller.openFolder(controller.userDataLocation.value);
                      },
                    ),
                    const SizedBox(width: 10),
                    WoxButton.primary(
                      text: controller.tr("ui_data_config_location_change"),
                      onPressed: () async {
                        final selectedDirectory = await FileSelector.pick(const UuidV4().generate(), FileSelectorParams(isDirectory: true));
                        if (selectedDirectory.isEmpty || !context.mounted) {
                          return;
                        }

                        final picked = selectedDirectory[0];
                        // The compact Data layout no longer embeds WoxPathFinder, so it keeps the
                        // same confirmation flow here before moving Wox's storage location.
                        await showDialog(
                          context: context,
                          barrierColor: getThemePopupBarrierColor(),
                          builder:
                              (dialogContext) => AlertDialog(
                                backgroundColor: getThemePopupSurfaceColor(),
                                surfaceTintColor: Colors.transparent,
                                shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(20), side: BorderSide(color: getThemePopupOutlineColor())),
                                content: Text(controller.tr("ui_data_config_location_change_confirm").replaceAll("{0}", picked)),
                                actions: [
                                  WoxButton.secondary(text: controller.tr("ui_data_config_location_change_cancel"), onPressed: () => Navigator.pop(dialogContext)),
                                  WoxButton.primary(
                                    text: controller.tr("ui_data_config_location_change_confirm_button"),
                                    onPressed: () {
                                      Navigator.pop(dialogContext);
                                      controller.updateUserDataLocation(picked);
                                    },
                                  ),
                                ],
                              ),
                        );
                        WoxSettingFocusUtil.restoreIfInSettingView();
                      },
                    ),
                  ],
                ),
              ),
              tips: controller.tr("ui_data_config_location_tips"),
            ),
          ],
        ),
        formSection(
          title: controller.tr("ui_data_section_backup"),
          children: [
            formField(
              label: controller.tr("ui_data_backup_auto_title"),
              labelWidth: GENERAL_SETTING_WIDE_LABEL_WIDTH,
              child: Obx(() {
                return WoxSwitch(
                  value: controller.woxSetting.value.enableAutoBackup,
                  onChanged: (value) {
                    controller.updateConfig("EnableAutoBackup", value.toString());
                  },
                );
              }),
              tipsWidget: _buildAutoBackupTips(),
            ),
            _buildBackupListTable(context),
          ],
        ),
        formSection(
          title: controller.tr("ui_data_section_logs"),
          children: [
            formField(
              label: controller.tr("ui_data_log_level_title"),
              labelWidth: GENERAL_SETTING_WIDE_LABEL_WIDTH,
              child: Obx(() {
                final logLevel = controller.woxSetting.value.logLevel.toUpperCase();
                final selectedLogLevel = logLevel == "DEBUG" ? "DEBUG" : "INFO";
                final isUpdatingLogLevel = controller.isUpdatingLogLevel.value;
                return WoxDropdownButton<String>(
                  value: selectedLogLevel,
                  items: [
                    WoxDropdownItem(value: "INFO", label: controller.tr("ui_data_log_level_info")),
                    WoxDropdownItem(value: "DEBUG", label: controller.tr("ui_data_log_level_debug")),
                  ],
                  onChanged:
                      isUpdatingLogLevel
                          ? null
                          : (value) {
                            if (value != null) {
                              controller.updateLogLevel(value);
                            }
                          },
                  isExpanded: true,
                );
              }),
              tips: controller.tr("ui_data_log_level_tips"),
            ),
            formField(
              label: controller.tr("ui_data_log_clear_title"),
              labelWidth: GENERAL_SETTING_WIDE_LABEL_WIDTH,
              child: Obx(() {
                final isClearing = controller.isClearingLogs.value;
                return Row(
                  mainAxisSize: MainAxisSize.min,
                  children: [
                    WoxButton.secondary(
                      text: controller.tr("ui_data_log_clear_button"),
                      icon: isClearing ? WoxLoadingIndicator(size: 14, color: getThemeTextColor()) : null,
                      onPressed:
                          isClearing
                              ? null
                              : () async {
                                await showDialog(
                                  context: context,
                                  barrierColor: getThemePopupBarrierColor(),
                                  builder: (dialogContext) {
                                    return AlertDialog(
                                      backgroundColor: getThemePopupSurfaceColor(),
                                      surfaceTintColor: Colors.transparent,
                                      shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(20), side: BorderSide(color: getThemePopupOutlineColor())),
                                      title: Text(controller.tr("ui_data_log_clear_confirm_title")),
                                      content: Text(controller.tr("ui_data_log_clear_confirm_message")),
                                      actions: [
                                        WoxButton.secondary(
                                          text: controller.tr("ui_data_log_clear_cancel"),
                                          onPressed: () {
                                            Navigator.pop(dialogContext);
                                          },
                                        ),
                                        WoxButton.primary(
                                          text: controller.tr("ui_data_log_clear_confirm"),
                                          onPressed: () {
                                            Navigator.pop(dialogContext);
                                            controller.clearLogs();
                                          },
                                        ),
                                      ],
                                    );
                                  },
                                );
                                WoxSettingFocusUtil.restoreIfInSettingView();
                              },
                    ),
                    const SizedBox(width: 10),
                    WoxButton.secondary(
                      text: controller.tr("ui_data_log_open_button"),
                      onPressed:
                          isClearing
                              ? null
                              : () {
                                controller.openLogFile();
                              },
                    ),
                  ],
                );
              }),
              tips: controller.tr("ui_data_log_clear_tips"),
            ),
          ],
        ),
      ],
    );
  }
}

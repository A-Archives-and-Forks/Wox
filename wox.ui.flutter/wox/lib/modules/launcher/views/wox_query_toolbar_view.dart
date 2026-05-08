import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:get/get.dart';
import 'package:uuid/v4.dart';
import 'package:wox/components/wox_hotkey_view.dart';
import 'package:wox/components/wox_image_view.dart';
import 'package:wox/controllers/wox_launcher_controller.dart';
import 'package:wox/entity/wox_hotkey.dart';
import 'package:wox/entity/wox_toolbar.dart';
import 'package:wox/utils/log.dart';
import 'package:wox/utils/wox_theme_util.dart';
import 'package:wox/utils/wox_interface_size_util.dart';
import 'package:wox/api/wox_api.dart';
import 'package:wox/controllers/wox_setting_controller.dart';
import 'package:wox/utils/color_util.dart';
import 'package:wox/utils/wox_text_measure_util.dart';

class WoxQueryToolbarView extends GetView<WoxLauncherController> {
  const WoxQueryToolbarView({super.key});

  bool get hasResultItems => controller.resultListViewController.items.isNotEmpty;

  bool get hasLeftMessage {
    final text = controller.resolvedToolbarText;
    return text != null && text.isNotEmpty;
  }

  Widget leftPart(double maxLeftWidth) {
    if (LoggerSwitch.enablePaintLog) Logger.instance.debug(const UuidV4().generate(), "repaint: toolbar view - left part");

    return Obx(() {
      final text = controller.resolvedToolbarText;
      final hasToolbarProgress = controller.hasVisibleToolbarMsg && (controller.resolvedToolbarProgress != null || controller.resolvedToolbarIndeterminate);
      final metrics = WoxInterfaceSizeUtil.instance.current;

      // If no message, return empty widget
      if (text == null || text.isEmpty) {
        return const SizedBox.shrink();
      }

      // Cap the left section width while allowing it to shrink to content size.
      return ConstrainedBox(
        constraints: BoxConstraints(maxWidth: maxLeftWidth),
        child: Row(
          mainAxisSize: MainAxisSize.min,
          children: [
            if (controller.resolvedToolbarIcon != null)
              Padding(padding: EdgeInsets.only(right: metrics.scaledSpacing(8)), child: WoxImageView(woxImage: controller.resolvedToolbarIcon!, width: metrics.toolbarIconSize, height: metrics.toolbarIconSize)),
            // Text area flexes inside the capped max width and will ellipsize when needed
            Flexible(
              child: LayoutBuilder(
                builder: (context, constraints) {
                  final textStyle = TextStyle(color: safeFromCssColor(WoxThemeUtil.instance.currentTheme.value.toolbarFontColor), fontSize: metrics.toolbarFontSize);
                  final isTextOverflow = WoxTextMeasureUtil.isTextOverflow(context: context, text: text, style: textStyle, maxWidth: constraints.maxWidth);

                  return Row(
                    mainAxisSize: MainAxisSize.min,
                    children: [
                      Flexible(
                        child: Text(
                          text,
                          style: textStyle,
                          overflow: TextOverflow.ellipsis,
                          maxLines: 1,
                        ),
                      ),
                      if (hasToolbarProgress)
                        Padding(
                          padding: EdgeInsets.only(left: metrics.scaledSpacing(8)),
                          child: SizedBox(
                            width: metrics.scaledSpacing(14),
                            height: metrics.scaledSpacing(14),
                            child: CircularProgressIndicator(
                              strokeWidth: metrics.scaledSpacing(2),
                              value: controller.resolvedToolbarIndeterminate ? null : (controller.resolvedToolbarProgress ?? 0).clamp(0, 100) / 100,
                              valueColor: AlwaysStoppedAnimation<Color>(safeFromCssColor(WoxThemeUtil.instance.currentTheme.value.toolbarFontColor)),
                              backgroundColor: safeFromCssColor(WoxThemeUtil.instance.currentTheme.value.toolbarFontColor).withValues(alpha: 0.2),
                            ),
                          ),
                        ),
                      if (isTextOverflow && !controller.hasVisibleToolbarMsg)
                        MouseRegion(
                          cursor: SystemMouseCursors.click,
                          child: GestureDetector(
                            onTap: () {
                              Clipboard.setData(ClipboardData(text: text));
                              controller.toolbarCopyText.value = 'toolbar_copied';
                              Future.delayed(const Duration(seconds: 3), () {
                                controller.toolbarCopyText.value = 'toolbar_copy';
                              });
                            },
                            child: Padding(
                              padding: EdgeInsets.only(left: metrics.scaledSpacing(8)),
                              child: Obx(() {
                                final settingController = Get.find<WoxSettingController>();
                                return Text(
                                  settingController.tr(controller.toolbarCopyText.value),
                                  style: TextStyle(
                                    color: safeFromCssColor(WoxThemeUtil.instance.currentTheme.value.toolbarFontColor),
                                    fontSize: metrics.toolbarFontSize,
                                    decoration: TextDecoration.underline,
                                  ),
                                );
                              }),
                            ),
                          ),
                        ),
                      if (isTextOverflow && !controller.hasVisibleToolbarMsg) ...[
                        SizedBox(width: metrics.scaledSpacing(8)),
                        Theme(
                          data: Theme.of(context).copyWith(
                            popupMenuTheme: PopupMenuThemeData(
                              color: safeFromCssColor(WoxThemeUtil.instance.currentTheme.value.toolbarBackgroundColor),
                              textStyle: TextStyle(color: safeFromCssColor(WoxThemeUtil.instance.currentTheme.value.toolbarFontColor), fontSize: metrics.toolbarFontSize),
                            ),
                          ),
                          child: PopupMenuButton<String>(
                            padding: EdgeInsets.zero,
                            tooltip: '',
                            onSelected: (value) async {
                              await WoxApi.instance.toolbarSnooze(const UuidV4().generate(), text, value);
                              // Hide current toolbar message immediately
                              controller.toolbar.value = controller.toolbar.value.emptyLeftSide();
                            },
                            itemBuilder: (context) {
                              final settingController = Get.find<WoxSettingController>();
                              return [
                                PopupMenuItem(value: '3d', child: Text(settingController.tr('toolbar_snooze_3d'))),
                                PopupMenuItem(value: '7d', child: Text(settingController.tr('toolbar_snooze_7d'))),
                                PopupMenuItem(value: '1m', child: Text(settingController.tr('toolbar_snooze_1m'))),
                                PopupMenuItem(value: 'forever', child: Text(settingController.tr('toolbar_snooze_forever'))),
                              ];
                            },
                            child: Builder(
                              builder: (context) {
                                final settingController = Get.find<WoxSettingController>();
                                return Text(
                                  settingController.tr('toolbar_snooze'),
                                  style: TextStyle(
                                    color: safeFromCssColor(WoxThemeUtil.instance.currentTheme.value.toolbarFontColor),
                                    fontSize: metrics.toolbarFontSize,
                                    decoration: TextDecoration.underline,
                                  ),
                                );
                              },
                            ),
                          ),
                        ),
                      ],
                    ],
                  );
                },
              ),
            ),
          ],
        ),
      );
    });
  }

  /// Calculate the precise width of a single action (name + hotkey + spacing)
  double _calculateActionWidth(BuildContext context, String actionName, HotkeyX hotkey) {
    final nameWidth = WoxTextMeasureUtil.measureTextWidth(
      context: context,
      text: actionName,
      style: TextStyle(color: safeFromCssColor(WoxThemeUtil.instance.currentTheme.value.toolbarFontColor), fontSize: WoxInterfaceSizeUtil.instance.current.toolbarFontSize),
    );

    // Calculate hotkey width
    final metrics = WoxInterfaceSizeUtil.instance.current;
    double hotkeyWidth = 0;
    if (hotkey.isNormalHotkey) {
      // Hotkey chip measurements mirror WoxHotkeyView's density-scaled key box
      // sizes so overflow decisions match the rendered toolbar actions.
      final keyCount = (hotkey.normalHotkey!.modifiers?.length ?? 0) + 1;
      hotkeyWidth = keyCount * metrics.scaledSpacing(28) + (keyCount - 1) * metrics.scaledSpacing(4);
    } else if (hotkey.isDoubleHotkey) {
      hotkeyWidth = metrics.scaledSpacing(28) * 2 + metrics.scaledSpacing(4);
    } else if (hotkey.isSingleHotkey) {
      hotkeyWidth = metrics.scaledSpacing(28);
    }

    return nameWidth + metrics.scaledSpacing(8) + hotkeyWidth + metrics.scaledSpacing(16);
  }

  Widget rightPart() {
    if (LoggerSwitch.enablePaintLog) Logger.instance.debug(const UuidV4().generate(), "repaint: toolbar view  - right part");

    return Obx(() {
      final toolbarInfo = controller.toolbar.value;

      // Show all actions with hotkeys
      if (toolbarInfo.actions == null || toolbarInfo.actions!.isEmpty) {
        return const SizedBox();
      }

      return LayoutBuilder(
        builder: (context, constraints) {
          final availableWidth = constraints.maxWidth;

          // Parse all actions and calculate their widths
          final actionData = <Map<String, dynamic>>[];
          for (var actionInfo in toolbarInfo.actions!) {
            var hotkey = WoxHotkey.parseHotkeyFromString(actionInfo.hotkey);
            if (hotkey != null) {
              final calculatedWidth = _calculateActionWidth(context, actionInfo.name, hotkey);
              actionData.add({'info': actionInfo, 'hotkey': hotkey, 'width': calculatedWidth});
            }
          }

          if (actionData.isEmpty) {
            return const SizedBox();
          }

          // Determine how many actions to show from right to left
          // Start from the rightmost action and work backwards
          final actionsToShow = <Map<String, dynamic>>[];
          double totalWidth = 0;

          // Iterate from right to left (reverse order)
          for (int i = actionData.length - 1; i >= 0; i--) {
            final action = actionData[i];
            final actionWidth = action['width'] as double;

            // Check if adding this action would exceed available width
            if (totalWidth + actionWidth <= availableWidth) {
              actionsToShow.insert(0, action); // Insert at beginning to maintain order
              totalWidth += actionWidth;
            } else {
              // No more space, stop adding actions
              break;
            }
          }

          // When there's a left message, ensure at least one action is shown (the rightmost one)
          if (hasLeftMessage && actionsToShow.isEmpty && actionData.isNotEmpty) {
            actionsToShow.add(actionData.last);
          }

          // Build widgets for the actions to show
          List<Widget> actionWidgets = [];
          for (var actionData in actionsToShow) {
            final actionInfo = actionData['info'] as ToolbarActionInfo;
            final hotkey = actionData['hotkey'] as HotkeyX;

            if (actionWidgets.isNotEmpty) {
              actionWidgets.add(SizedBox(width: WoxInterfaceSizeUtil.instance.current.scaledSpacing(16)));
            }

            actionWidgets.add(_buildClickableToolbarAction(actionInfo, hotkey));
          }

          return Align(alignment: Alignment.centerRight, child: Row(mainAxisSize: MainAxisSize.min, children: actionWidgets));
        },
      );
    });
  }

  Widget _buildClickableToolbarAction(ToolbarActionInfo actionInfo, HotkeyX hotkey) {
    return MouseRegion(
      cursor: SystemMouseCursors.click,
      child: GestureDetector(
        behavior: HitTestBehavior.opaque,
        onTap: () {
          controller.handleToolbarActionTap(const UuidV4().generate(), actionInfo);
        },
        child: Row(
          mainAxisSize: MainAxisSize.min,
          children: [
            Text(
              actionInfo.name,
              style: TextStyle(color: safeFromCssColor(WoxThemeUtil.instance.currentTheme.value.toolbarFontColor), fontSize: WoxInterfaceSizeUtil.instance.current.toolbarFontSize),
              overflow: TextOverflow.ellipsis,
              maxLines: 1,
            ),
            SizedBox(width: WoxInterfaceSizeUtil.instance.current.scaledSpacing(8)),
            WoxHotkeyView(
              hotkey: hotkey,
              backgroundColor:
                  hasResultItems
                      ? safeFromCssColor(WoxThemeUtil.instance.currentTheme.value.toolbarBackgroundColor)
                      : safeFromCssColor(WoxThemeUtil.instance.currentTheme.value.appBackgroundColor).withValues(alpha: 0.1),
              borderColor: safeFromCssColor(WoxThemeUtil.instance.currentTheme.value.toolbarFontColor),
              textColor: safeFromCssColor(WoxThemeUtil.instance.currentTheme.value.toolbarFontColor),
            ),
          ],
        ),
      ),
    );
  }

  @override
  Widget build(BuildContext context) {
    if (LoggerSwitch.enablePaintLog) Logger.instance.debug(const UuidV4().generate(), "repaint: query toolbar view - container");

    return Obx(() {
      final metrics = WoxInterfaceSizeUtil.instance.metrics.value;
      return SizedBox(
        height: WoxThemeUtil.instance.getToolbarHeight(),
        child: Container(
          decoration: BoxDecoration(
            color: hasResultItems ? safeFromCssColor(WoxThemeUtil.instance.currentTheme.value.toolbarBackgroundColor) : Colors.transparent,
            border: Border(
              top: BorderSide(
                color: hasResultItems ? safeFromCssColor(WoxThemeUtil.instance.currentTheme.value.toolbarFontColor).withValues(alpha: 0.1) : Colors.transparent,
                width: 1,
              ),
            ),
          ),
          child: Padding(
            padding: EdgeInsets.only(
              left: WoxThemeUtil.instance.currentTheme.value.toolbarPaddingLeft.toDouble(),
              right: WoxThemeUtil.instance.currentTheme.value.toolbarPaddingRight.toDouble(),
            ),
            child: LayoutBuilder(
              builder: (context, constraints) {
                // Limit left message to a max fraction so right side always has room
                // Toolbar text, icons, and hotkey chips use density metrics now,
                // so reserve the right-action area with the same scale instead
                // of the old normal-only 200px estimate.
                final double leftMaxWidth = (constraints.maxWidth - metrics.scaledSpacing(200)).clamp(0.0, constraints.maxWidth).toDouble();
                return Row(
                  crossAxisAlignment: CrossAxisAlignment.center,
                  children: [
                    // Left part takes only the space it needs up to leftMaxWidth
                    leftPart(leftMaxWidth),
                    if (hasLeftMessage) SizedBox(width: metrics.scaledSpacing(16)),
                    // Right part fills remaining space and aligns content to the right
                    Expanded(child: rightPart()),
                  ],
                );
              },
            ),
          ),
        ),
      );
    });
  }
}

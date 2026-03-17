import 'package:desktop_drop/desktop_drop.dart';
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:get/get.dart';
import 'package:wox/components/wox_platform_focus.dart';
import 'package:wox/controllers/wox_launcher_controller.dart';
import 'package:wox/modules/launcher/views/wox_query_box_view.dart';
import 'package:wox/modules/launcher/views/wox_query_result_view.dart';
import 'package:wox/modules/launcher/views/wox_query_toolbar_view.dart';
import 'package:wox/utils/wox_theme_util.dart';
import 'package:wox/utils/color_util.dart';

class WoxLauncherView extends GetView<WoxLauncherController> {
  const WoxLauncherView({super.key});

  @override
  Widget build(BuildContext context) {
    return Obx(() {
      final theme = WoxThemeUtil.instance.currentTheme.value;
      final isQueryBoxVisible = controller.isQueryBoxVisible.value;
      final isToolbarShowedWithoutResults = controller.isToolbarShowedWithoutResults;
      final queryBoxView = const WoxQueryBoxView();
      final resultView = const WoxQueryResultView();
      final topPadding = isQueryBoxVisible ? theme.appPaddingTop.toDouble() : 0.0;

      double bottomPadding = theme.appPaddingBottom.toDouble();
      if (isQueryBoxVisible && isToolbarShowedWithoutResults) {
        bottomPadding = 0.0;
      }

      Widget content = resultView;
      if (isQueryBoxVisible) {
        content = Column(
          children: [
            if (controller.isQueryBoxAtBottom.value) const Expanded(child: WoxQueryResultView()),
            queryBoxView,
            if (!controller.isQueryBoxAtBottom.value) const Expanded(child: WoxQueryResultView()),
          ],
        );
      }

      return WoxPlatformFocus(
        onKeyEvent: (node, event) {
          if (event is! KeyDownEvent || event.logicalKey != LogicalKeyboardKey.escape) {
            return KeyEventResult.ignored;
          }

          if (controller.queryBoxFocusNode.hasFocus) {
            return KeyEventResult.ignored;
          }

          controller.focusQueryBox();
          return KeyEventResult.handled;
        },
        child: Scaffold(
          backgroundColor: safeFromCssColor(theme.appBackgroundColor),
          body: DropTarget(
            onDragDone: (DropDoneDetails details) {
              controller.handleDropFiles(details);
            },
            child: Column(
              children: [
                if (!isQueryBoxVisible) const Offstage(offstage: true, child: WoxQueryBoxView()),
                Flexible(
                  fit: isQueryBoxVisible ? FlexFit.tight : FlexFit.loose,
                  child: Padding(
                    padding: EdgeInsets.only(top: topPadding, right: theme.appPaddingRight.toDouble(), bottom: bottomPadding, left: theme.appPaddingLeft.toDouble()),
                    child: content,
                  ),
                ),
                if (controller.isShowToolbar && !controller.isToolbarHiddenForce.value) const SizedBox(height: 40, child: WoxQueryToolbarView()),
              ],
            ),
          ),
        ),
      );
    });
  }
}

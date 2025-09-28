import 'package:flutter/material.dart';
import 'package:uuid/v4.dart';
import 'package:wox/components/wox_image_view.dart';
import 'package:wox/entity/wox_image.dart';
import 'package:wox/entity/wox_list_item.dart';
import 'package:wox/entity/wox_theme.dart';
import 'package:wox/enums/wox_image_type_enum.dart';
import 'package:wox/enums/wox_list_view_type_enum.dart';
import 'package:wox/enums/wox_result_tail_type_enum.dart';
import 'package:wox/utils/log.dart';
import 'package:wox/utils/wox_setting_util.dart';
import 'package:wox/utils/color_util.dart';

import 'wox_hotkey_view.dart';

class WoxListItemView extends StatelessWidget {
  final WoxListItem item;
  final WoxTheme woxTheme;
  final bool isActive;
  final bool isHovered;
  final WoxListViewType listViewType;

  const WoxListItemView({
    super.key,
    required this.item,
    required this.woxTheme,
    required this.isActive,
    required this.isHovered,
    required this.listViewType,
  });

  double getImageSize(WoxImage img, double defaultSize) {
    if (img.imageType == WoxImageTypeEnum.WOX_IMAGE_TYPE_EMOJI.code) {
      return defaultSize - 10;
    } else {
      return defaultSize;
    }
  }

  Widget buildQuickSelectNumber() {
    return Padding(
      padding: const EdgeInsets.only(left: 10.0, right: 5.0),
      child: Container(
        width: 24,
        height: 24,
        decoration: BoxDecoration(
          color: safeFromCssColor(isActive ? woxTheme.resultItemActiveTailTextColor : woxTheme.resultItemTailTextColor),
          borderRadius: BorderRadius.circular(4),
          border: Border.all(
            color: safeFromCssColor(isActive ? woxTheme.resultItemActiveTailTextColor : woxTheme.resultItemTailTextColor).withValues(alpha: 0.3),
            width: 1,
          ),
        ),
        child: Center(
          child: Text(
            item.quickSelectNumber,
            style: TextStyle(
              color: safeFromCssColor(isActive ? woxTheme.resultItemActiveBackgroundColor : woxTheme.appBackgroundColor),
              fontSize: 12,
              fontWeight: FontWeight.bold,
            ),
          ),
        ),
      ),
    );
  }

  Widget buildTails() {
    return ConstrainedBox(
      constraints: BoxConstraints(maxWidth: WoxSettingUtil.instance.currentSetting.appWidth / 2),
      child: Padding(
        padding: const EdgeInsets.only(left: 10.0, right: 5.0),
        child: SingleChildScrollView(
          scrollDirection: Axis.horizontal,
          child: Row(
            children: [
              for (final tail in item.tails)
                if (tail.type == WoxListItemTailTypeEnum.WOX_LIST_ITEM_TAIL_TYPE_TEXT.code && tail.text != null)
                  Padding(
                    padding: const EdgeInsets.only(left: 10.0),
                    child: Text(
                      tail.text!,
                      style: TextStyle(
                        color: safeFromCssColor(isActive ? woxTheme.resultItemActiveTailTextColor : woxTheme.resultItemTailTextColor),
                        fontSize: 12,
                      ),
                      maxLines: 1,
                      overflow: TextOverflow.ellipsis,
                      strutStyle: const StrutStyle(
                        forceStrutHeight: true,
                      ),
                    ),
                  )
                else if (tail.type == WoxListItemTailTypeEnum.WOX_LIST_ITEM_TAIL_TYPE_HOTKEY.code && tail.hotkey != null)
                  Padding(
                    padding: const EdgeInsets.only(left: 10.0),
                    child: WoxHotkeyView(
                      hotkey: tail.hotkey!,
                      backgroundColor: isActive ? safeFromCssColor(woxTheme.resultItemActiveBackgroundColor) : safeFromCssColor(woxTheme.actionContainerBackgroundColor),
                      borderColor: safeFromCssColor(isActive ? woxTheme.resultItemActiveTailTextColor : woxTheme.resultItemTailTextColor),
                      textColor: safeFromCssColor(isActive ? woxTheme.resultItemActiveTailTextColor : woxTheme.resultItemTailTextColor),
                    ),
                  )
                else if (tail.type == WoxListItemTailTypeEnum.WOX_LIST_ITEM_TAIL_TYPE_IMAGE.code && tail.image != null && tail.image!.imageData.isNotEmpty)
                  Padding(
                    padding: const EdgeInsets.only(left: 10.0),
                    child: WoxImageView(
                      woxImage: tail.image!,
                      width: getImageSize(tail.image!, 24),
                      height: getImageSize(tail.image!, 24),
                    ),
                  ),
            ],
          ),
        ),
      ),
    );
  }

  Color getBackgroundColor() {
    if (isActive) {
      if (listViewType == WoxListViewTypeEnum.WOX_LIST_VIEW_TYPE_ACTION.code) {
        return safeFromCssColor(woxTheme.actionItemActiveBackgroundColor);
      }
      if (listViewType == WoxListViewTypeEnum.WOX_LIST_VIEW_TYPE_CHAT.code) {
        return safeFromCssColor(woxTheme.resultItemActiveBackgroundColor);
      }
      if (listViewType == WoxListViewTypeEnum.WOX_LIST_VIEW_TYPE_RESULT.code) {
        return safeFromCssColor(woxTheme.resultItemActiveBackgroundColor);
      }
    } else if (isHovered) {
      // Use a lighter version of the active background color for hover state
      if (listViewType == WoxListViewTypeEnum.WOX_LIST_VIEW_TYPE_ACTION.code) {
        return safeFromCssColor(woxTheme.actionItemActiveBackgroundColor).withValues(alpha: 0.3);
      }
      if (listViewType == WoxListViewTypeEnum.WOX_LIST_VIEW_TYPE_CHAT.code) {
        return safeFromCssColor(woxTheme.resultItemActiveBackgroundColor).withValues(alpha: 0.3);
      }
      if (listViewType == WoxListViewTypeEnum.WOX_LIST_VIEW_TYPE_RESULT.code) {
        return safeFromCssColor(woxTheme.resultItemActiveBackgroundColor).withValues(alpha: 0.3);
      }
    }

    return Colors.transparent;
  }

  @override
  Widget build(BuildContext context) {
    if (LoggerSwitch.enablePaintLog) Logger.instance.debug(const UuidV4().generate(), "repaint: list item view ${item.title} - container");

    final bool isResultList = listViewType == WoxListViewTypeEnum.WOX_LIST_VIEW_TYPE_RESULT.code;
    final BorderRadius borderRadius = isResultList && woxTheme.resultItemBorderRadius > 0 ? BorderRadius.circular(woxTheme.resultItemBorderRadius.toDouble()) : BorderRadius.zero;

    Widget content = Container(
      decoration: BoxDecoration(
        color: getBackgroundColor(),
        border: Border(
            left: listViewType == WoxListViewTypeEnum.WOX_LIST_VIEW_TYPE_ACTION.code
                ? BorderSide.none
                : BorderSide(
                    color: isActive ? safeFromCssColor(woxTheme.resultItemActiveBackgroundColor) : Colors.transparent,
                    width: isActive ? woxTheme.resultItemActiveBorderLeftWidth.toDouble() : woxTheme.resultItemBorderLeftWidth.toDouble(),
                  )),
      ),
      padding: isResultList
          ? EdgeInsets.only(
              top: woxTheme.resultItemPaddingTop.toDouble(),
              right: woxTheme.resultItemPaddingRight.toDouble(),
              bottom: woxTheme.resultItemPaddingBottom.toDouble(),
              left: woxTheme.resultItemPaddingLeft.toDouble(),
            )
          : EdgeInsets.zero,
      child: Row(
        children: [
          item.isGroup
              ? const SizedBox()
              : Padding(
                  padding: const EdgeInsets.only(left: 5.0, right: 10.0),
                  child: WoxImageView(
                    woxImage: item.icon,
                    width: getImageSize(item.icon, 30),
                    height: getImageSize(item.icon, 30),
                  ),
                ),
          Expanded(
            child: Column(crossAxisAlignment: CrossAxisAlignment.start, mainAxisAlignment: MainAxisAlignment.center, children: [
              Text(
                item.title,
                style: TextStyle(
                  fontSize: 16,
                  color: listViewType == WoxListViewTypeEnum.WOX_LIST_VIEW_TYPE_ACTION.code
                      ? safeFromCssColor(isActive ? woxTheme.actionItemActiveFontColor : woxTheme.actionItemFontColor)
                      : safeFromCssColor(isActive ? woxTheme.resultItemActiveTitleColor : woxTheme.resultItemTitleColor),
                ),
                maxLines: 1,
                overflow: TextOverflow.ellipsis,
                strutStyle: const StrutStyle(
                  forceStrutHeight: true,
                ),
              ),
              item.subTitle.isNotEmpty
                  ? Padding(
                      padding: const EdgeInsets.only(top: 2.0),
                      child: Text(
                        item.subTitle,
                        style: TextStyle(
                          color: safeFromCssColor(isActive ? woxTheme.resultItemActiveSubTitleColor : woxTheme.resultItemSubTitleColor),
                          fontSize: 13,
                        ),
                        maxLines: 1,
                        overflow: TextOverflow.ellipsis,
                        strutStyle: const StrutStyle(
                          forceStrutHeight: true,
                        ),
                      ),
                    )
                  : const SizedBox(),
            ]),
          ),
          // Tails
          if (item.tails.isNotEmpty) buildTails() else const SizedBox(),
          // Quick select number
          if (item.isShowQuickSelect && item.quickSelectNumber.isNotEmpty) buildQuickSelectNumber(),
        ],
      ),
    );

    if (borderRadius != BorderRadius.zero) {
      content = ClipRRect(
        borderRadius: borderRadius,
        child: content,
      );
    }

    return content;
  }
}

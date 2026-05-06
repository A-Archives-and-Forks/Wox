import 'package:flutter/material.dart';
import 'package:wox/components/wox_image_view.dart';
import 'package:wox/components/wox_tooltip.dart';
import 'package:wox/entity/wox_list_item.dart';
import 'package:wox/entity/wox_preview_list.dart';
import 'package:wox/entity/wox_theme.dart';
import 'package:wox/enums/wox_result_tail_text_category_enum.dart';
import 'package:wox/enums/wox_result_tail_type_enum.dart';
import 'package:wox/utils/color_util.dart';

class WoxListPreviewView extends StatelessWidget {
  final WoxPreviewListData data;
  final WoxTheme woxTheme;

  const WoxListPreviewView({super.key, required this.data, required this.woxTheme});

  @override
  Widget build(BuildContext context) {
    final fontColor = safeFromCssColor(woxTheme.previewFontColor);
    final splitLineColor = safeFromCssColor(woxTheme.previewSplitLineColor);

    // A generic list preview can represent selected files, compression
    // progress, or other status rows. Rendering only row data here prevents the
    // preview from leaking file-specific assumptions back into plugin payloads.
    if (data.items.isEmpty) {
      return Center(child: Text("No items", style: TextStyle(color: fontColor.withValues(alpha: 0.62), fontSize: 14)));
    }

    return Padding(
      padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 10),
      child: Column(
        children: [
          for (var index = 0; index < data.items.length; index++) ...[
            _ListPreviewRow(item: data.items[index], woxTheme: woxTheme),
            if (index != data.items.length - 1) Divider(height: 1, color: splitLineColor.withValues(alpha: 0.28)),
          ],
        ],
      ),
    );
  }
}

class _ListPreviewRow extends StatelessWidget {
  final WoxPreviewListItem item;
  final WoxTheme woxTheme;

  const _ListPreviewRow({required this.item, required this.woxTheme});

  static const double _iconSize = 34;
  static const double _tailImageSize = 18;

  @override
  Widget build(BuildContext context) {
    final fontColor = safeFromCssColor(woxTheme.previewFontColor);
    final splitLineColor = safeFromCssColor(woxTheme.previewSplitLineColor);
    final propertyColor = safeFromCssColor(woxTheme.previewPropertyContentColor);

    return Padding(
      padding: const EdgeInsets.symmetric(vertical: 10),
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.center,
        children: [
          Container(
            width: _iconSize,
            height: _iconSize,
            decoration: BoxDecoration(
              color: propertyColor.withValues(alpha: 0.1),
              borderRadius: BorderRadius.circular(8),
              border: Border.all(color: splitLineColor.withValues(alpha: 0.38)),
            ),
            child: _buildIcon(propertyColor),
          ),
          const SizedBox(width: 12),
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              mainAxisSize: MainAxisSize.min,
              children: [
                WoxTooltip(
                  message: item.title,
                  child: Text(
                    item.title,
                    maxLines: 1,
                    overflow: TextOverflow.ellipsis,
                    style: TextStyle(color: fontColor.withValues(alpha: 0.92), fontSize: 14.5, fontWeight: FontWeight.w600, height: 1.2),
                  ),
                ),
                if (item.subtitle.isNotEmpty) ...[
                  const SizedBox(height: 4),
                  WoxTooltip(
                    message: item.subtitle,
                    child: Text(
                      item.subtitle,
                      maxLines: 1,
                      overflow: TextOverflow.ellipsis,
                      style: TextStyle(color: fontColor.withValues(alpha: 0.56), fontSize: 12.5, height: 1.2),
                    ),
                  ),
                ],
              ],
            ),
          ),
          if (item.tails.isNotEmpty) ...[
            const SizedBox(width: 10),
            ConstrainedBox(
              constraints: const BoxConstraints(maxWidth: 150),
              child: SingleChildScrollView(scrollDirection: Axis.horizontal, child: Row(children: item.tails.map((tail) => _buildTail(tail, fontColor, splitLineColor)).toList())),
            ),
          ],
        ],
      ),
    );
  }

  Widget _buildIcon(Color color) {
    final icon = item.icon;
    if (icon == null || icon.imageData.isEmpty) {
      return Icon(Icons.list_alt_outlined, color: color.withValues(alpha: 0.88), size: 18);
    }

    return Center(child: WoxImageView(woxImage: icon, width: 20, height: 20));
  }

  Widget _buildTail(WoxListItemTail tail, Color fontColor, Color splitLineColor) {
    if (tail.type == WoxListItemTailTypeEnum.WOX_LIST_ITEM_TAIL_TYPE_IMAGE.code && tail.image != null && tail.image!.imageData.isNotEmpty) {
      return Padding(
        padding: const EdgeInsets.only(left: 6),
        child: WoxImageView(woxImage: tail.image!, width: tail.imageWidth ?? _tailImageSize, height: tail.imageHeight ?? _tailImageSize),
      );
    }

    if (tail.type != WoxListItemTailTypeEnum.WOX_LIST_ITEM_TAIL_TYPE_TEXT.code || tail.text == null) {
      return const SizedBox.shrink();
    }

    final style = _tailStyle(tail.textCategory, fontColor, splitLineColor);
    return Padding(
      padding: const EdgeInsets.only(left: 6),
      child: Container(
        constraints: const BoxConstraints(maxWidth: 92),
        padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 4),
        decoration: BoxDecoration(color: style.backgroundColor, borderRadius: BorderRadius.circular(8), border: Border.all(color: style.borderColor)),
        child: Text(tail.text!, maxLines: 1, overflow: TextOverflow.ellipsis, style: TextStyle(color: style.textColor, fontSize: 11, fontWeight: FontWeight.w600, height: 1.1)),
      ),
    );
  }

  _TailStyle _tailStyle(String textCategory, Color fontColor, Color splitLineColor) {
    final normalizedCategory = WoxListItemTailTextCategoryEnum.ensureCode(textCategory);
    final semanticColor = switch (normalizedCategory) {
      woxListItemTailTextCategoryDanger => const Color(0xFFE5484D),
      woxListItemTailTextCategoryWarning => const Color(0xFFF5A524),
      woxListItemTailTextCategorySuccess => const Color(0xFF30A46C),
      _ => fontColor.withValues(alpha: 0.62),
    };

    // Tails reuse result-row semantic categories, but preview rows need their
    // own compact chip styling because the surrounding panel has different
    // background and density from the result list.
    return _TailStyle(
      textColor: semanticColor,
      backgroundColor: semanticColor.withValues(alpha: normalizedCategory == woxListItemTailTextCategoryDefault ? 0.035 : 0.1),
      borderColor: normalizedCategory == woxListItemTailTextCategoryDefault ? splitLineColor.withValues(alpha: 0.42) : semanticColor.withValues(alpha: 0.28),
    );
  }
}

class _TailStyle {
  final Color textColor;
  final Color backgroundColor;
  final Color borderColor;

  const _TailStyle({required this.textColor, required this.backgroundColor, required this.borderColor});
}

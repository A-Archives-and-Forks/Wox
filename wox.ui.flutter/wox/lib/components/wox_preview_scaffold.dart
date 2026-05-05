import 'package:flutter/material.dart';
import 'package:wox/components/wox_tooltip.dart';
import 'package:wox/entity/wox_theme.dart';
import 'package:wox/utils/color_util.dart';

class WoxPreviewScaffold extends StatelessWidget {
  final WoxTheme woxTheme;
  final Widget child;
  final Map<String, String> properties;
  final ScrollController scrollController;
  final bool contentHandlesScrolling;

  const WoxPreviewScaffold({
    super.key,
    required this.woxTheme,
    required this.child,
    required this.properties,
    required this.scrollController,
    this.contentHandlesScrolling = false,
  });

  @override
  Widget build(BuildContext context) {
    // The preview shell keeps every plugin preview content-first. Older rendering
    // placed properties in table rows that competed with the preview body, so the
    // new structure reserves the main area for PreviewData and moves metadata into
    // lightweight pills at the bottom.
    return Container(
      padding: const EdgeInsets.only(top: 12, bottom: 10, left: 14, right: 12),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          Expanded(child: _buildContent(context)),
          if (properties.isNotEmpty) ...[
            // The preview body now has its own framed surface, so an additional
            // divider above the metadata pills is visual clutter instead of
            // useful separation.
            Padding(padding: const EdgeInsets.only(top: 10), child: _PreviewPropertyPills(woxTheme: woxTheme, properties: properties)),
          ],
        ],
      ),
    );
  }

  Widget _buildContent(BuildContext context) {
    final themedContent = Theme(data: ThemeData(textSelectionTheme: TextSelectionThemeData(selectionColor: safeFromCssColor(woxTheme.previewTextSelectionColor))), child: child);

    if (contentHandlesScrolling) {
      return LayoutBuilder(builder: (context, constraints) => SizedBox(width: constraints.maxWidth, height: constraints.maxHeight, child: themedContent));
    }

    // The shell owns scrolling for simple preview types so text, markdown, and
    // unsupported-file messages share the same scrollbar placement and padding.
    // The viewport height is passed as a minimum child height so short preview
    // renderers can opt into vertical centering without reading an unbounded
    // height from SingleChildScrollView.
    return LayoutBuilder(
      builder:
          (context, viewportConstraints) => Scrollbar(
            thumbVisibility: true,
            controller: scrollController,
            child: SingleChildScrollView(
              controller: scrollController,
              child: ConstrainedBox(
                constraints: BoxConstraints(minWidth: viewportConstraints.maxWidth, maxWidth: viewportConstraints.maxWidth, minHeight: viewportConstraints.maxHeight),
                child: themedContent,
              ),
            ),
          ),
    );
  }
}

class _PreviewPropertyPills extends StatelessWidget {
  final WoxTheme woxTheme;
  final Map<String, String> properties;

  const _PreviewPropertyPills({required this.woxTheme, required this.properties});

  @override
  Widget build(BuildContext context) {
    final fontColor = safeFromCssColor(woxTheme.previewFontColor);
    final contentColor = safeFromCssColor(woxTheme.previewPropertyContentColor);
    final borderColor = safeFromCssColor(woxTheme.previewSplitLineColor);

    // Pills are horizontally scrollable instead of wrapping so metadata never
    // steals height from the preview body on compact launcher windows. They are
    // styled as quiet metadata, not controls, because actions already live in
    // the launcher toolbar.
    // Only the value is shown by default to keep the metadata strip compact; the
    // title remains available in the tooltip for users who need the exact field.
    return SizedBox(
      height: 26,
      child: ListView.separated(
        scrollDirection: Axis.horizontal,
        itemCount: properties.length,
        separatorBuilder: (context, index) => const SizedBox(width: 8),
        itemBuilder: (context, index) {
          final entry = properties.entries.elementAt(index);
          final value = entry.value.trim();
          final label = value.isEmpty ? entry.key : "${entry.key}: $value";
          final visibleText = value.isEmpty ? entry.key : value;

          return WoxTooltip(
            message: label,
            child: Container(
              constraints: const BoxConstraints(maxWidth: 220),
              padding: const EdgeInsets.symmetric(horizontal: 9, vertical: 4),
              decoration: BoxDecoration(
                color: fontColor.withValues(alpha: 0.035),
                borderRadius: BorderRadius.circular(8),
                border: Border.all(color: borderColor.withValues(alpha: 0.48)),
              ),
              child: Text(
                visibleText,
                overflow: TextOverflow.ellipsis,
                maxLines: 1,
                style: TextStyle(color: contentColor.withValues(alpha: 0.9), fontSize: 11.5, height: 1.2, fontWeight: FontWeight.w600),
              ),
            ),
          );
        },
      ),
    );
  }
}

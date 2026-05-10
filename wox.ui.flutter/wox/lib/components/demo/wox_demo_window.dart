part of 'wox_demo.dart';

class WoxDemoResult {
  const WoxDemoResult({required this.title, required this.icon, this.subtitle, this.tail, this.selected = false});

  final String title;
  final String? subtitle;
  final Widget icon;
  final bool selected;
  final String? tail;
}

class WoxDemoWindow extends StatelessWidget {
  const WoxDemoWindow({
    super.key,
    required this.accent,
    required this.query,
    this.results = const [
      WoxDemoResult(title: 'Open Wox Settings', subtitle: r'C:\Users\qianl\AppData\Roaming\Wox', icon: WoxDemoLogoMark(), tail: '2 day ago', selected: true),
      WoxDemoResult(
        title: 'Open URL settings',
        subtitle: 'Configure URL open rules and browser targets',
        icon: Icon(Icons.link_rounded, color: Color(0xFF38BDF8), size: 24),
        tail: 'Settings',
      ),
      WoxDemoResult(title: 'Open WebView settings', subtitle: 'Inspect and tune embedded preview behavior', icon: Icon(Icons.language_rounded, color: Color(0xFF60A5FA), size: 24)),
      WoxDemoResult(
        title: 'Open Update settings',
        subtitle: 'Check update channel and release status',
        icon: Icon(Icons.sync_rounded, color: Color(0xFF3B82F6), size: 24),
        tail: 'Update',
      ),
    ],
    this.queryAccessory,
    this.footerHotkey,
    this.isFooterHotkeyPressed = false,
    this.actionPanel,
    this.actionPanelProgress = 0,
    this.opaqueBackground = false,
    this.showQueryBox = true,
    this.showToolbar = true,
  });

  final Color accent;
  final String query;
  final List<WoxDemoResult> results;
  final Widget? queryAccessory;
  final String? footerHotkey;
  final bool isFooterHotkeyPressed;
  final Widget? actionPanel;
  final double actionPanelProgress;
  final bool opaqueBackground;
  final bool showQueryBox;
  final bool showToolbar;

  @override
  Widget build(BuildContext context) {
    return LayoutBuilder(
      builder: (context, constraints) {
        final metrics = WoxInterfaceSizeUtil.instance.current;
        // Bug fix: the preview is now a supporting element with less vertical
        // weight. Respecting the parent height prevents small onboarding
        // windows from overflowing while still capping tall windows so the demo
        // does not dominate the current setup task.
        final maxPreviewHeight = constraints.maxHeight.clamp(0.0, 320.0).toDouble();
        final previewWidth = constraints.maxWidth;
        // Feature refinement: the shared demo now defaults to the full Wox
        // launcher chrome. Most previews should show both the query box and the
        // toolbar; special entry points such as Tray Query opt out explicitly so
        // callers do not have to remember to add chrome for every normal demo.
        final hasFooter = showToolbar;
        final effectiveFooterHotkey = footerHotkey ?? _demoActionPanelHotkey();
        final footerHeight = hasFooter ? WoxThemeUtil.instance.getToolbarHeight() : 0.0;
        final actionPanelWidth = (previewWidth * 0.42).clamp(250.0, 320.0).toDouble();
        final queryTop = 12.0;
        final resultTop = showQueryBox ? queryTop + metrics.queryBoxBaseHeight + 10 : queryTop;
        final bottomPadding = hasFooter ? 0.0 : 12.0;
        final maxResultListHeight = (maxPreviewHeight - resultTop - footerHeight - bottomPadding).clamp(0.0, double.infinity).toDouble();
        final resultListHeight = (results.length * WoxThemeUtil.instance.getResultItemHeight()).clamp(0.0, maxResultListHeight).toDouble();
        // Bug fix: toolbar-enabled previews should shrink to their visible
        // results like the real launcher. The old full-height result list left
        // a dead band under short demos, so the footer is now placed directly
        // after the rendered rows unless the parent height forces clipping.
        final shouldShrinkToContent = hasFooter || !showQueryBox;
        final previewHeight = shouldShrinkToContent ? (resultTop + resultListHeight + footerHeight + bottomPadding).clamp(0.0, maxPreviewHeight).toDouble() : maxPreviewHeight;

        return Center(
          child: SizedBox(
            width: previewWidth,
            height: previewHeight,
            child: Container(
              decoration: BoxDecoration(
                // Feature refinement: the main-hotkey desktop scene needs an
                // opaque launcher surface so the simulated desktop does not
                // wash through Wox. Other onboarding previews keep their
                // existing translucent treatment unless they opt in.
                color: opaqueBackground ? getThemeBackgroundColor().withValues(alpha: 1) : getThemeBackgroundColor().withValues(alpha: 0.86),
                border: Border.all(color: getThemeTextColor().withValues(alpha: 0.10)),
                borderRadius: BorderRadius.circular(8),
              ),
              child: ClipRRect(
                borderRadius: BorderRadius.circular(8),
                child: Stack(
                  children: [
                    Positioned.fill(
                      child: DecoratedBox(
                        decoration: BoxDecoration(
                          gradient: RadialGradient(center: const Alignment(0.6, -0.5), radius: 1.05, colors: [accent.withValues(alpha: 0.12), Colors.transparent]),
                        ),
                      ),
                    ),
                    if (showQueryBox) _MiniSearchBar(query: query, trailing: queryAccessory),
                    // Feature refinement: the mock launcher now follows the
                    // production query/result vertical rhythm instead of using
                    // compact onboarding-only row spacing. This keeps fonts,
                    // padding, and density comparable to the real Wox window.
                    Positioned(
                      left: 12,
                      right: 12,
                      top: resultTop,
                      bottom: shouldShrinkToContent ? null : bottomPadding,
                      height: shouldShrinkToContent ? resultListHeight : null,
                      child: _MiniResultList(results: results),
                    ),
                    if (hasFooter) _MiniFooter(accent: accent, hotkey: effectiveFooterHotkey, isPressed: isFooterHotkeyPressed),
                    if (actionPanel != null)
                      Positioned(
                        right: 16,
                        bottom: footerHeight + 12,
                        width: actionPanelWidth,
                        child: Opacity(
                          opacity: actionPanelProgress,
                          child: Transform.translate(
                            offset: Offset(18 * (1 - actionPanelProgress), 10 * (1 - actionPanelProgress)),
                            child: Transform.scale(alignment: Alignment.bottomRight, scale: 0.96 + (0.04 * actionPanelProgress), child: actionPanel),
                          ),
                        ),
                      ),
                  ],
                ),
              ),
            ),
          ),
        );
      },
    );
  }
}

class _MiniSearchBar extends StatelessWidget {
  const _MiniSearchBar({required this.query, this.trailing});

  final String query;
  final Widget? trailing;

  @override
  Widget build(BuildContext context) {
    final metrics = WoxInterfaceSizeUtil.instance.current;
    final woxTheme = WoxThemeUtil.instance.currentTheme.value;

    return Positioned(
      left: 12,
      right: 12,
      top: 12,
      child: Container(
        height: metrics.queryBoxBaseHeight,
        padding: const EdgeInsets.only(left: 8, right: 8, top: QUERY_BOX_CONTENT_PADDING_TOP, bottom: QUERY_BOX_CONTENT_PADDING_BOTTOM),
        decoration: BoxDecoration(color: woxTheme.queryBoxBackgroundColorParsed, borderRadius: BorderRadius.circular(woxTheme.queryBoxBorderRadius.toDouble())),
        child: Row(
          children: [
            Expanded(
              child: Text(query, maxLines: 1, overflow: TextOverflow.ellipsis, style: TextStyle(color: woxTheme.queryBoxFontColorParsed, fontSize: metrics.queryBoxFontSize)),
            ),
            // Feature refinement: the search row has no implicit Glance item.
            // Callers opt in only after the Glance step has introduced and
            // enabled it, which keeps earlier examples from leaking later
            // onboarding concepts.
            if (trailing != null) ...[const SizedBox(width: 10), trailing!],
          ],
        ),
      ),
    );
  }
}

class _MiniResultList extends StatelessWidget {
  const _MiniResultList({required this.results});

  final List<WoxDemoResult> results;

  @override
  Widget build(BuildContext context) {
    // Bug fix: the preview rows now use the real Wox result height, and the
    // Action Panel demo also reserves toolbar space. A fixed Column can exceed
    // the remaining preview height, while the production result view is a
    // clipped list. Using a non-scrollable ListView preserves real row sizing
    // and clips overflow instead of rendering Flutter's overflow warning.
    return ListView.builder(
      padding: EdgeInsets.zero,
      physics: const NeverScrollableScrollPhysics(),
      itemCount: results.length,
      itemBuilder: (context, index) {
        final result = results[index];
        return _MiniResultRow(title: result.title, subtitle: result.subtitle, icon: result.icon, selected: result.selected, tail: result.tail);
      },
    );
  }
}

class _MiniResultRow extends StatelessWidget {
  const _MiniResultRow({required this.title, required this.icon, this.subtitle, this.selected = false, this.tail});

  final String title;
  final String? subtitle;
  final Widget icon;
  final bool selected;
  final String? tail;

  @override
  Widget build(BuildContext context) {
    // Bug fix: rows must keep launcher-like density even when a preview passes
    // fewer entries. The previous Expanded row made two-result demos stretch
    // into oversized blocks, so each mock result now has a stable row height.
    // Feature refinement: rows now also model Wox's subtitle and tail affordance
    // so the shared preview shows file paths, result descriptions, and status
    // chips instead of flattening every result into a single title line.
    // Feature refinement: result row metrics now come from the production
    // launcher sizing/theme utilities. The onboarding-specific padding and
    // bold text made the preview look unlike Wox, while reusing these values
    // keeps the example aligned with real query results across densities.
    final metrics = WoxInterfaceSizeUtil.instance.current;
    final woxTheme = WoxThemeUtil.instance.currentTheme.value;
    final hasSubtitle = subtitle != null && subtitle!.isNotEmpty;
    final borderRadius = woxTheme.resultItemBorderRadius > 0 ? BorderRadius.circular(woxTheme.resultItemBorderRadius.toDouble()) : BorderRadius.zero;
    final maxBorderWidth =
        (woxTheme.resultItemActiveBorderLeftWidth > woxTheme.resultItemBorderLeftWidth ? woxTheme.resultItemActiveBorderLeftWidth : woxTheme.resultItemBorderLeftWidth).toDouble();
    final actualBorderWidth = selected ? woxTheme.resultItemActiveBorderLeftWidth.toDouble() : woxTheme.resultItemBorderLeftWidth.toDouble();
    final titleColor = selected ? woxTheme.resultItemActiveTitleColorParsed : woxTheme.resultItemTitleColorParsed;
    final subtitleColor = selected ? woxTheme.resultItemActiveSubTitleColorParsed : woxTheme.resultItemSubTitleColorParsed;
    final tailColor = selected ? woxTheme.resultItemActiveTailTextColorParsed : woxTheme.resultItemTailTextColorParsed;

    Widget content = Container(
      decoration: BoxDecoration(color: selected ? woxTheme.resultItemActiveBackgroundColorParsed : Colors.transparent),
      padding: EdgeInsets.only(
        top: metrics.scaledSpacing(woxTheme.resultItemPaddingTop.toDouble()),
        right: metrics.scaledSpacing(woxTheme.resultItemPaddingRight.toDouble()),
        bottom: metrics.scaledSpacing(woxTheme.resultItemPaddingBottom.toDouble()),
        left: metrics.scaledSpacing(woxTheme.resultItemPaddingLeft.toDouble() + maxBorderWidth),
      ),
      child: Row(
        children: [
          Padding(
            padding: EdgeInsets.only(left: metrics.resultItemIconPaddingLeft, right: metrics.resultItemIconPaddingRight),
            child: SizedBox(width: metrics.resultIconSize, height: metrics.resultIconSize, child: FittedBox(fit: BoxFit.contain, child: icon)),
          ),
          Expanded(
            child: Column(
              mainAxisAlignment: MainAxisAlignment.center,
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text(title, maxLines: 1, overflow: TextOverflow.ellipsis, style: TextStyle(color: titleColor, fontSize: metrics.resultTitleFontSize)),
                if (hasSubtitle)
                  Padding(
                    padding: EdgeInsets.only(top: metrics.resultItemSubtitlePaddingTop),
                    child: Text(subtitle!, maxLines: 1, overflow: TextOverflow.ellipsis, style: TextStyle(color: subtitleColor, fontSize: metrics.resultSubtitleFontSize)),
                  ),
              ],
            ),
          ),
          if (tail != null && tail!.isNotEmpty)
            Padding(
              padding: EdgeInsets.only(left: metrics.resultItemTailPaddingLeft, right: metrics.resultItemTailPaddingRight),
              child: Padding(
                padding: EdgeInsets.only(left: metrics.resultItemTailItemPaddingLeft),
                child: ConstrainedBox(
                  constraints: const BoxConstraints(maxWidth: 132),
                  child: DecoratedBox(
                    decoration: BoxDecoration(
                      color: Colors.transparent,
                      border: Border.all(color: tailColor.withValues(alpha: selected ? 0.34 : 0.2)),
                      borderRadius: BorderRadius.circular(999),
                    ),
                    child: Padding(
                      padding: EdgeInsets.symmetric(horizontal: metrics.resultItemTextTailHPadding, vertical: metrics.resultItemTextTailVPadding),
                      child: Text(tail!, maxLines: 1, overflow: TextOverflow.ellipsis, style: TextStyle(color: tailColor, fontSize: metrics.tailHotkeyFontSize)),
                    ),
                  ),
                ),
              ),
            ),
        ],
      ),
    );

    if (borderRadius != BorderRadius.zero) {
      content = ClipRRect(borderRadius: borderRadius, child: content);
    }

    if (actualBorderWidth > 0) {
      content = Stack(
        children: [
          content,
          Positioned(
            left: 0,
            top: 0,
            bottom: 0,
            child: Container(
              width: actualBorderWidth,
              decoration: BoxDecoration(
                color: woxTheme.resultItemActiveBackgroundColorParsed,
                borderRadius: borderRadius != BorderRadius.zero ? BorderRadius.only(topLeft: borderRadius.topLeft, bottomLeft: borderRadius.bottomLeft) : BorderRadius.zero,
              ),
            ),
          ),
        ],
      );
    }

    return SizedBox(height: WoxThemeUtil.instance.getResultItemHeight(), child: content);
  }
}

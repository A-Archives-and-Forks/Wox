part of 'wox_demo.dart';

// Animated demo for the welcome onboarding step.
//
// Phase timeline (total 5600ms, looping):
//   0.00–0.30  Concept card visible — static; users read the anatomy labels.
//   0.30–0.52  Card slides up + fades; three colored chip ghosts fly from their
//              card positions toward the Wox query bar.
//   0.50–0.68  Wox window rises in; query bar assembles in three stages:
//              '' → 'wpm' → 'wpm install' → 'wpm install everything'.
//              Each token keeps the color it had on the anatomy card.
//   0.68–0.88  Three plugin-store results stagger in, showing the search is done.
//   0.88–1.00  Hold, then loop.
//
// The flying chip overlay uses approximate chip positions computed from
// estimated font metrics at fontSize 15 w700. Small inaccuracies are invisible
// because the chips shrink to ~0.4× before reaching the bar.
class WoxQueryConceptDemo extends StatefulWidget {
  const WoxQueryConceptDemo({super.key, required this.accent, required this.tr});

  final Color accent;
  final String Function(String key) tr;

  @override
  State<WoxQueryConceptDemo> createState() => _WoxQueryConceptDemoState();
}

class _WoxQueryConceptDemoState extends State<WoxQueryConceptDemo> with SingleTickerProviderStateMixin {
  late final AnimationController _controller;

  // Fixed token colors: the anatomy card chips and the Wox query bar spans use
  // identical tints so users visually connect each label to its query segment.
  static const _commandColor = Color(0xFFFACC15);
  static const _searchTermColor = Color(0xFF4ADE80);

  @override
  void initState() {
    super.initState();
    _controller = AnimationController(vsync: this, duration: const Duration(milliseconds: 5600))..repeat();
  }

  @override
  void dispose() {
    _controller.dispose();
    super.dispose();
  }

  double _interval(double start, double end, Curve curve) {
    final t = ((_controller.value - start) / (end - start)).clamp(0.0, 1.0);
    return curve.transform(t.toDouble());
  }

  // ── Concept card ─────────────────────────────────────────────────────────
  double get _cardOpacity {
    if (_controller.value < 0.30) return 1.0;
    if (_controller.value < 0.50) return 1.0 - _interval(0.30, 0.50, Curves.easeInCubic);
    return 0.0;
  }

  double get _cardDY {
    if (_controller.value < 0.30) return 0.0;
    return -30.0 * _interval(0.30, 0.50, Curves.easeInCubic);
  }

  // ── Flying chip overlay ───────────────────────────────────────────────────
  // Chips are only rendered during the crossfade window (0.30–0.52).
  bool get _showFlyingChips => _controller.value >= 0.30 && _controller.value < 0.52;

  double get _flyProgress {
    if (_controller.value < 0.30) return 0.0;
    if (_controller.value < 0.52) return _interval(0.30, 0.52, Curves.easeInCubic);
    return 1.0;
  }

  // Chips fade in at lift-off and fade out near the query bar so they vanish
  // just before the colored text spans appear in the bar.
  double _chipAlpha(double fly) {
    if (fly < 0.08) return fly / 0.08;
    if (fly > 0.70) return 1.0 - ((fly - 0.70) / 0.30);
    return 1.0;
  }

  // ── Wox window ───────────────────────────────────────────────────────────
  double get _woxOpacity {
    if (_controller.value < 0.38) return 0.0;
    if (_controller.value < 0.55) return _interval(0.38, 0.55, Curves.easeOutCubic);
    return 1.0;
  }

  double get _woxDY {
    if (_controller.value < 0.38) return 22.0;
    if (_controller.value < 0.55) return 22.0 * (1.0 - _interval(0.38, 0.55, Curves.easeOutCubic));
    return 0.0;
  }

  // 0=empty  1='wpm'  2='wpm install'  3='wpm install everything'
  int get _queryStage {
    if (_controller.value < 0.52) return 0;
    if (_controller.value < 0.60) return 1;
    if (_controller.value < 0.66) return 2;
    return 3;
  }

  // ── Results ───────────────────────────────────────────────────────────────
  // All three results appear together immediately after the query is complete.
  // Staggered reveals were removed because they added complexity without
  // teaching anything new; the important moment is the query completing.
  double get _resultsOpacity {
    if (_controller.value < 0.68) return 0.0;
    if (_controller.value < 0.76) return _interval(0.68, 0.76, Curves.easeOutCubic);
    return 1.0;
  }

  @override
  Widget build(BuildContext context) {
    return AnimatedBuilder(
      key: const ValueKey('onboarding-query-concept-demo'),
      animation: _controller,
      builder: (context, child) {
        final showChips = _showFlyingChips;
        final fly = _flyProgress;

        return LayoutBuilder(
          builder: (context, constraints) {
            final w = constraints.maxWidth;
            final h = constraints.maxHeight;

            // ── Chip source positions ───────────────────────────────────────
            // The concept card is centered in the area padded 40px per side.
            // Its inner padding is 20px. Chip widths are estimated from the
            // token text at fontSize 15 w700 on a typical sans-serif font.
            // Vertical position targets the chip text row, which sits roughly
            // at 42% of the demo height (card is vertically centered, chip row
            // is above the label area).
            const estWpmW = 53.0;
            const estInstallW = 84.0;
            const estEverythingW = 110.0;
            const gap = 10.0;
            const estRowW = estWpmW + gap + estInstallW + gap + estEverythingW; // ≈257
            const cardInnerPad = 20.0;
            // Chip row left: card centered in (w-80) with 40px outer offset.
            final chipRowLeft = (w - estRowW - 2 * cardInnerPad) / 2 + cardInnerPad;
            final wpmSrcX = chipRowLeft + estWpmW / 2;
            final instSrcX = chipRowLeft + estWpmW + gap + estInstallW / 2;
            final evSrcX = chipRowLeft + estWpmW + gap + estInstallW + gap + estEverythingW / 2;
            final srcY = h * 0.42;

            // ── Chip destination: query bar text area ───────────────────────
            // Wox window: left-padding 48, top-padding 40 + current woxDY.
            // Query bar: inset left 12 + bar left-padding 8 → text starts at x=68.
            // Query bar center-Y: 40 + woxDY + 12 (bar top inset) + 18 (half ~36px bar height).
            const queryTextX = 48.0 + 12.0 + 8.0; // 68
            final queryBarCenterY = 40.0 + _woxDY + 12.0 + 18.0;
            // All three chips converge toward the query bar start; the slight
            // x-offsets spread them so they don't collapse to a single point.
            const dstBaseX = queryTextX + 16.0;
            final dstY = queryBarCenterY;

            double lerp(double a, double b, double t) => a + (b - a) * t;

            Widget flyChip(double srcX, double srcY2, double dstX, Color color, String text) {
              final cx = lerp(srcX, dstX, fly);
              final cy = lerp(srcY2, dstY, fly);
              final scale = lerp(1.0, 0.40, fly);
              final alpha = _chipAlpha(fly);
              return Positioned(
                left: cx,
                top: cy,
                child: FractionalTranslation(
                  translation: const Offset(-0.5, -0.5),
                  child: IgnorePointer(
                    child: Opacity(
                      opacity: alpha,
                      child: Transform.scale(
                        scale: scale,
                        child: Container(
                          padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 7),
                          decoration: BoxDecoration(color: color.withValues(alpha: 0.18), borderRadius: BorderRadius.circular(6)),
                          child: Text(text, style: TextStyle(color: color, fontSize: 15, fontWeight: FontWeight.w700)),
                        ),
                      ),
                    ),
                  ),
                ),
              );
            }

            return ClipRRect(
              borderRadius: BorderRadius.circular(8),
              child: Stack(
                clipBehavior: Clip.hardEdge,
                children: [
                  Positioned.fill(child: WoxDemoDesktopBackground(accent: widget.accent, isMac: Platform.isMacOS, showDefaultIcons: false)),

                  // Concept card – slides up and fades out during phase 2.
                  if (_cardOpacity > 0.01)
                    Positioned.fill(
                      child: Center(
                        child: Padding(
                          padding: const EdgeInsets.symmetric(horizontal: 40, vertical: 24),
                          child: Opacity(
                            opacity: _cardOpacity,
                            child: Transform.translate(
                              offset: Offset(0, _cardDY),
                              child: _QueryConceptCard(accent: widget.accent, commandColor: _commandColor, searchTermColor: _searchTermColor, tr: widget.tr),
                            ),
                          ),
                        ),
                      ),
                    ),

                  // Flying chip ghosts – rendered only during crossfade window.
                  if (showChips) ...[
                    flyChip(wpmSrcX, srcY, dstBaseX, widget.accent, 'wpm'),
                    flyChip(instSrcX, srcY, dstBaseX + 44.0, _commandColor, 'install'),
                    flyChip(evSrcX, srcY, dstBaseX + 104.0, _searchTermColor, 'everything'),
                  ],

                  // Wox window – rises in and shows the assembled query + results.
                  if (_woxOpacity > 0.01)
                    Positioned.fill(
                      child: Padding(
                        padding: const EdgeInsets.fromLTRB(48, 40, 52, 36),
                        child: Opacity(
                          opacity: _woxOpacity,
                          child: Transform.translate(
                            offset: Offset(0, _woxDY),
                            child: _ConceptDemoWindow(
                              accent: widget.accent,
                              queryStage: _queryStage,
                              resultsOpacity: _resultsOpacity,
                              triggerKeywordColor: widget.accent,
                              commandColor: _commandColor,
                              searchTermColor: _searchTermColor,
                              tr: widget.tr,
                            ),
                          ),
                        ),
                      ),
                    ),
                ],
              ),
            );
          },
        );
      },
    );
  }
}

// The card itself: a rounded panel with a title, three annotated tokens, and
// connector lines linking each token to its semantic label below.
class _QueryConceptCard extends StatelessWidget {
  const _QueryConceptCard({required this.accent, required this.commandColor, required this.searchTermColor, required this.tr});

  final Color accent;
  final Color commandColor;
  final Color searchTermColor;
  final String Function(String key) tr;

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.all(20),
      decoration: BoxDecoration(
        color: getThemeBackgroundColor().withValues(alpha: 0.88),
        border: Border.all(color: getThemeTextColor().withValues(alpha: 0.09)),
        borderRadius: BorderRadius.circular(10),
        boxShadow: [BoxShadow(color: Colors.black.withValues(alpha: 0.14), blurRadius: 24, offset: const Offset(0, 10))],
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        mainAxisSize: MainAxisSize.min,
        children: [
          Text(tr('onboarding_query_concept_title'), style: TextStyle(color: getThemeSubTextColor(), fontSize: 11, letterSpacing: 0.6, fontWeight: FontWeight.w600)),
          const SizedBox(height: 16),
          // Each token is placed in its own column so the chip and label are
          // always center-aligned with each other regardless of text length.
          Row(
            crossAxisAlignment: CrossAxisAlignment.start,
            mainAxisSize: MainAxisSize.min,
            children: [
              _ConceptToken(token: 'wpm', label: tr('onboarding_query_concept_trigger_keyword'), color: accent),
              const SizedBox(width: 10),
              _ConceptToken(token: 'install', label: tr('onboarding_query_concept_command'), color: commandColor),
              const SizedBox(width: 10),
              _ConceptToken(token: 'everything', label: tr('onboarding_query_concept_search_term'), color: searchTermColor),
            ],
          ),
        ],
      ),
    );
  }
}

// A single token chip with a short vertical connector and the semantic label
// below it. IntrinsicWidth ensures the chip and label share the same center
// regardless of which one is wider.
class _ConceptToken extends StatelessWidget {
  const _ConceptToken({required this.token, required this.label, required this.color, this.subLabel});

  final String token;
  final String label;
  final String? subLabel;
  final Color color;

  @override
  Widget build(BuildContext context) {
    return IntrinsicWidth(
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.center,
        children: [
          // Token chip: tinted background keeps the color mark readable on
          // both light and dark themes without relying on full opacity.
          Container(
            width: double.infinity,
            padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 7),
            decoration: BoxDecoration(color: color.withValues(alpha: 0.14), borderRadius: BorderRadius.circular(6)),
            child: Text(token, textAlign: TextAlign.center, style: TextStyle(color: color, fontSize: 15, fontWeight: FontWeight.w700)),
          ),
          const SizedBox(height: 6),
          // Short vertical line connecting the chip to the label below.
          Container(width: 1, height: 12, color: color.withValues(alpha: 0.35)),
          const SizedBox(height: 6),
          Text(label, textAlign: TextAlign.center, style: TextStyle(color: color, fontSize: 11, fontWeight: FontWeight.w600)),
          if (subLabel != null) ...[const SizedBox(height: 2), Text(subLabel!, textAlign: TextAlign.center, style: TextStyle(color: color.withValues(alpha: 0.65), fontSize: 10))],
        ],
      ),
    );
  }
}

// A stripped-down Wox window rendered only during the query-concept demo.
// Uses a RichText query bar so each token segment keeps its anatomy card
// color: accent for trigger keyword, amber for command, green for search term.
//
// Layout: a Stack that fills its parent, matching the structure of
// WoxDemoWindow. _MiniFooter and _MiniSearchBar are both Positioned widgets
// that must live inside a Stack — placing them in a Column caused the toolbar
// to float in the center of the window instead of anchoring to the bottom.
class _ConceptDemoWindow extends StatelessWidget {
  const _ConceptDemoWindow({
    required this.accent,
    required this.queryStage,
    required this.resultsOpacity,
    required this.triggerKeywordColor,
    required this.commandColor,
    required this.searchTermColor,
    required this.tr,
  });

  final Color accent;
  final int queryStage;
  // All three results share one opacity value — they appear simultaneously
  // once the query is fully assembled, so no stagger is needed here.
  final double resultsOpacity;
  final Color triggerKeywordColor;
  final Color commandColor;
  final Color searchTermColor;
  final String Function(String key) tr;

  List<TextSpan> _buildSpans(double fontSize) {
    if (queryStage == 0) return [];
    return [
      TextSpan(text: 'wpm', style: TextStyle(color: triggerKeywordColor, fontWeight: FontWeight.w700)),
      if (queryStage >= 2) TextSpan(text: ' install', style: TextStyle(color: commandColor, fontWeight: FontWeight.w700)),
      if (queryStage >= 3) TextSpan(text: ' everything', style: TextStyle(color: searchTermColor)),
    ];
  }

  @override
  Widget build(BuildContext context) {
    final metrics = WoxInterfaceSizeUtil.instance.current;
    final woxTheme = WoxThemeUtil.instance.currentTheme.value;
    // Vertical layout mirrors WoxDemoWindow: query bar at top, results below,
    // toolbar at the absolute bottom via _MiniFooter's own Positioned.
    final queryTop = 12.0;
    final resultTop = queryTop + metrics.queryBoxBaseHeight + 10.0;
    final footerHeight = WoxThemeUtil.instance.getToolbarHeight();

    return Container(
      decoration: BoxDecoration(
        color: getThemeBackgroundColor().withValues(alpha: 0.86),
        border: Border.all(color: getThemeTextColor().withValues(alpha: 0.10)),
        borderRadius: BorderRadius.circular(8),
      ),
      child: ClipRRect(
        borderRadius: BorderRadius.circular(8),
        // Stack fills the parent so every Positioned child (query bar, footer)
        // uses the same coordinate space as the real WoxDemoWindow.
        child: Stack(
          fit: StackFit.expand,
          children: [
            // Accent radial gradient sits behind all content.
            Positioned.fill(
              child: DecoratedBox(
                decoration: BoxDecoration(gradient: RadialGradient(center: const Alignment(0.6, -0.5), radius: 1.05, colors: [accent.withValues(alpha: 0.12), Colors.transparent])),
              ),
            ),
            // Query bar – RichText keeps each token in its anatomy color.
            Positioned(
              left: 12,
              right: 12,
              top: queryTop,
              height: metrics.queryBoxBaseHeight,
              child: Container(
                padding: const EdgeInsets.symmetric(horizontal: 8),
                decoration: BoxDecoration(color: woxTheme.queryBoxBackgroundColorParsed, borderRadius: BorderRadius.circular(woxTheme.queryBoxBorderRadius.toDouble())),
                child: Align(
                  alignment: Alignment.centerLeft,
                  child: RichText(
                    maxLines: 1,
                    overflow: TextOverflow.ellipsis,
                    text: TextSpan(style: TextStyle(fontSize: metrics.queryBoxFontSize), children: _buildSpans(metrics.queryBoxFontSize)),
                  ),
                ),
              ),
            ),
            // Three plugin-store results, all fading in together.
            if (resultsOpacity > 0.01)
              Positioned(
                left: 12,
                right: 12,
                top: resultTop,
                bottom: footerHeight,
                child: Opacity(
                  opacity: resultsOpacity,
                  child: Column(
                    mainAxisSize: MainAxisSize.min,
                    children: [
                      _MiniResultRow(
                        title: 'Everything',
                        subtitle: tr('onboarding_query_concept_result1_subtitle'),
                        icon: Icon(Icons.search_rounded, color: accent, size: 23),
                        selected: true,
                      ),
                      _MiniResultRow(
                        title: 'Everything (portable)',
                        subtitle: tr('onboarding_query_concept_result2_subtitle'),
                        icon: Icon(Icons.search_outlined, color: accent.withValues(alpha: 0.65), size: 23),
                      ),
                      _MiniResultRow(
                        title: 'Everything-cli',
                        subtitle: tr('onboarding_query_concept_result3_subtitle'),
                        icon: const Icon(Icons.terminal_rounded, color: Color(0xFF94A3B8), size: 23),
                      ),
                    ],
                  ),
                ),
              ),
            // Toolbar – _MiniFooter is a Positioned widget that anchors itself
            // to bottom:0, left:0, right:0 inside the Stack.
            _MiniFooter(accent: accent, hotkey: _demoActionPanelHotkey(), isPressed: false),
          ],
        ),
      ),
    );
  }
}

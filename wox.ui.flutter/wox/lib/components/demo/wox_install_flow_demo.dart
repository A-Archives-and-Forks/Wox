part of 'wox_demo.dart';

class _InstallFlowDemo extends StatefulWidget {
  const _InstallFlowDemo({
    required this.demoKey,
    required this.accent,
    required this.icon,
    required this.title,
    required this.hintFrom,
    required this.hintTo,
    required this.queryStages,
    required this.installLabel,
    required this.installingLabel,
    required this.installedLabel,
    required this.primaryTitle,
    required this.primarySubtitle,
    required this.primaryIcon,
    required this.secondaryResults,
    // Optional theme data shown after installation completes. When set the demo
    // window crossfades to these colors to show the "applied theme" result.
    this.appliedTheme,
  });

  final ValueKey<String> demoKey;
  final Color accent;
  final IconData icon;
  final String title;
  final String hintFrom;
  final String hintTo;
  final List<String> queryStages;
  final String installLabel;
  final String installingLabel;
  final String installedLabel;
  final String primaryTitle;
  final String primarySubtitle;
  final Widget primaryIcon;
  final List<WoxDemoResult> secondaryResults;
  final _DemoThemeData? appliedTheme;

  @override
  State<_InstallFlowDemo> createState() => _InstallFlowDemoState();
}

class _InstallFlowDemoState extends State<_InstallFlowDemo> with SingleTickerProviderStateMixin {
  late final AnimationController _controller;

  @override
  void initState() {
    super.initState();
    // Duration extended from 4600ms to 5600ms so the applied-theme window
    // stays visible for ~1.7s (was ~0.7s) before the loop restarts.
    _controller = AnimationController(vsync: this, duration: const Duration(milliseconds: 5600))..repeat();
  }

  @override
  void dispose() {
    _controller.dispose();
    super.dispose();
  }

  double _interval(double start, double end, Curve curve) {
    final value = ((_controller.value - start) / (end - start)).clamp(0.0, 1.0).toDouble();
    return curve.transform(value);
  }

  // Timeline (5600ms total, thresholds derived from keeping all absolute
  // durations the same as before except the applied-theme window which gains
  // ~1000ms so users can read the result before the loop restarts):
  //   0 –  460ms (0.00–0.08): initial pause
  //   460 – 2576ms (0.08–0.46): typing query stages
  //   2576 – 2944ms (0.46–0.53): full query visible, pre-install pause
  //   2944 – 3496ms (0.53–0.62): install progress ramp
  //   3496 – 3588ms (0.62–0.64): installed label
  //   3588 – 5324ms (0.64–0.95): applied theme shown (~1736ms)
  //   5324 – 5600ms (0.95–1.00): fade out install progress

  String _queryText() {
    // Bug fix: the previous easeOutCubic curve over a stage list caused early
    // characters to rush by and later ones to linger, and each stage had
    // multi-character jumps (e.g., '' → 'w' → 'wpm'). Typing the final query
    // string one character at a time with linear speed gives equal per-character
    // duration (~56ms/char for 'wpm install clipboard', ~74ms for 'theme ocean dark').
    final target = widget.queryStages.last;
    if (target.isEmpty) return '';
    final t = _interval(0.08, 0.29, Curves.linear);
    return target.substring(0, (t * target.length).floor().clamp(0, target.length));
  }

  double _installProgress() {
    if (_controller.value < 0.53) {
      return 0;
    }
    if (_controller.value < 0.64) {
      return _interval(0.53, 0.64, Curves.easeOutCubic);
    }
    if (_controller.value < 0.95) {
      return 1;
    }
    return 1 - _interval(0.95, 1, Curves.easeInCubic);
  }

  // Returns true during the window where the theme has been fully applied and
  // the demo window should show the new theme's appearance (~1736ms).
  bool _isThemeApplied() => _controller.value >= 0.64 && _controller.value < 0.95;

  String _primaryTail() {
    if (_controller.value >= 0.53 && _controller.value < 0.63) {
      return widget.installingLabel;
    }
    if (_controller.value >= 0.63 && _controller.value < 0.95) {
      return widget.installedLabel;
    }
    return widget.installLabel;
  }

  @override
  Widget build(BuildContext context) {
    // Feature change: WPM and theme installation use the same compact desktop
    // teaching pattern as query shortcuts. The shared animation keeps the top
    // hint strip stable while the launcher demonstrates typing, selecting a
    // store result, and reaching the install action.
    return AnimatedBuilder(
      key: widget.demoKey,
      animation: _controller,
      builder: (context, child) {
        final installProgress = _installProgress();

        return ClipRRect(
          borderRadius: BorderRadius.circular(8),
          child: Stack(
            children: [
              Positioned.fill(child: WoxDemoDesktopBackground(accent: widget.accent, isMac: Platform.isMacOS, showDefaultIcons: false)),
              Positioned.fill(
                child: Padding(
                  padding: const EdgeInsets.fromLTRB(48, 18, 52, 36),
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.stretch,
                    children: [
                      WoxDemoHintCard(accent: widget.accent, icon: widget.icon, title: widget.title, from: widget.hintFrom, to: widget.hintTo),
                      const SizedBox(height: 12),
                      Expanded(
                        // Theme install feature: when the animation reaches the
                        // "applied" window, crossfade to a WoxDemoWindow wrapped
                        // with _InheritedDemoTheme so all descendant colors
                        // (query bar, results, toolbar) switch to the new theme
                        // without touching any global WoxThemeUtil state.
                        child: AnimatedSwitcher(
                          duration: const Duration(milliseconds: 500),
                          child: KeyedSubtree(
                            key: ValueKey(_isThemeApplied() && widget.appliedTheme != null),
                            child: Builder(
                              builder: (ctx) {
                                final isApplied = _isThemeApplied() && widget.appliedTheme != null;
                                final effectiveAccent = isApplied ? widget.appliedTheme!.accent : widget.accent;
                                final window = WoxDemoWindow(
                                  accent: effectiveAccent,
                                  query: _queryText(),
                                  opaqueBackground: true,
                                  results: [
                                    WoxDemoResult(title: widget.primaryTitle, subtitle: widget.primarySubtitle, icon: widget.primaryIcon, selected: true, tail: _primaryTail()),
                                    ...widget.secondaryResults,
                                  ],
                                );
                                if (isApplied) {
                                  return _InheritedDemoTheme(data: widget.appliedTheme!, child: window);
                                }
                                return window;
                              },
                            ),
                          ),
                        ),
                      ),
                    ],
                  ),
                ),
              ),
            ],
          ),
        );
      },
    );
  }
}

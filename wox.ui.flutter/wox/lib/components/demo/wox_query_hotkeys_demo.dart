part of 'wox_demo.dart';

class WoxQueryHotkeysDemo extends StatefulWidget {
  const WoxQueryHotkeysDemo({super.key, required this.accent, required this.tr});

  final Color accent;
  final String Function(String key) tr;

  @override
  State<WoxQueryHotkeysDemo> createState() => _WoxQueryHotkeysDemoState();
}

class _WoxQueryHotkeysDemoState extends State<WoxQueryHotkeysDemo> with SingleTickerProviderStateMixin {
  late final AnimationController _controller;

  @override
  void initState() {
    super.initState();
    _controller = AnimationController(vsync: this, duration: const Duration(milliseconds: 4600))..repeat();
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

  double _shortcutProgress() {
    if (_controller.value < 0.18) return 0;
    if (_controller.value < 0.30) {
      return _interval(0.18, 0.30, Curves.easeOutCubic);
    }
    if (_controller.value < 0.50) return 1;
    return 1 - _interval(0.50, 0.62, Curves.easeInCubic);
  }

  double _windowProgress() {
    if (_controller.value < 0.40) return 0;
    if (_controller.value < 0.58) {
      return _interval(0.40, 0.58, Curves.easeOutCubic);
    }
    if (_controller.value < 0.90) return 1;
    return 1 - _interval(0.90, 1, Curves.easeInCubic);
  }

  bool _isShortcutPressed() {
    return _controller.value >= 0.30 && _controller.value <= 0.42;
  }

  @override
  Widget build(BuildContext context) {
    final hotkey = _formatDemoHotkey('', fallback: Platform.isMacOS ? 'cmd+shift+g' : 'ctrl+shift+g');

    // Feature change: Query Hotkeys now get their own onboarding motion. The
    // demo starts from a configured binding, then shows the hotkey opening Wox
    // directly with the bound query instead of sharing the old summary list.
    return AnimatedBuilder(
      key: const ValueKey('onboarding-query-hotkeys-demo'),
      animation: _controller,
      builder: (context, child) {
        final shortcutProgress = _shortcutProgress();
        final windowProgress = _windowProgress();

        return ClipRRect(
          borderRadius: BorderRadius.circular(8),
          child: Stack(
            children: [
              Positioned.fill(child: WoxDemoDesktopBackground(accent: widget.accent, isMac: Platform.isMacOS, showDefaultIcons: false)),
              Positioned.fill(
                child: Padding(
                  // Feature refinement: query feature demos use a shared top
                  // hint strip and reserve the rest of the scene for the actual
                  // launcher animation. This keeps explanation and demo from
                  // competing for the same vertical space.
                  padding: const EdgeInsets.fromLTRB(48, 18, 52, 36),
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.stretch,
                    children: [
                      WoxDemoHintCard(accent: widget.accent, icon: Icons.keyboard_command_key, title: widget.tr('onboarding_query_hotkeys_title'), from: hotkey, to: 'github repo'),
                      const SizedBox(height: 12),
                      Expanded(
                        child: Stack(
                          children: [
                            Positioned.fill(
                              child: Opacity(
                                opacity: shortcutProgress,
                                child: Transform.translate(
                                  offset: Offset(0, 8 * (1 - shortcutProgress)),
                                  child: _HotkeyPressOverlay(hotkey: hotkey, accent: widget.accent, pressed: _isShortcutPressed()),
                                ),
                              ),
                            ),
                            if (windowProgress > 0.01)
                              Positioned.fill(
                                child: Transform.translate(
                                  offset: Offset(0, 20 * (1 - windowProgress)),
                                  child: Transform.scale(
                                    scale: 0.95 + (0.05 * windowProgress),
                                    // Feature refinement: keep Query Hotkeys
                                    // aligned with Query Shortcuts. The
                                    // previous extra top inset left an empty
                                    // band between the hint strip and Wox,
                                    // while the shared remaining-area layout
                                    // gives the demo more usable space.
                                    child: WoxDemoWindow(
                                      accent: widget.accent,
                                      query: 'github repo',
                                      opaqueBackground: true,
                                      footerHotkey: _demoActionPanelHotkey(),
                                      results: [
                                        WoxDemoResult(
                                          title: 'Wox repository',
                                          subtitle: 'Open Wox-launcher/Wox on GitHub',
                                          icon: const Icon(Icons.code_rounded, color: Colors.white, size: 23),
                                          selected: true,
                                          tail: hotkey,
                                        ),
                                        WoxDemoResult(
                                          title: widget.tr('onboarding_query_hotkeys_title'),
                                          subtitle: widget.tr('onboarding_query_hotkeys_body'),
                                          icon: Icon(Icons.bolt_outlined, color: widget.accent, size: 23),
                                          tail: widget.tr('ui_query_hotkeys'),
                                        ),
                                        const WoxDemoResult(
                                          title: 'Issues',
                                          subtitle: 'github repo issues',
                                          icon: Icon(Icons.bug_report_outlined, color: Color(0xFFFACC15), size: 23),
                                          tail: 'GitHub',
                                        ),
                                      ],
                                    ),
                                  ),
                                ),
                              ),
                          ],
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

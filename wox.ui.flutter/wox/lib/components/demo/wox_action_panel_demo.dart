part of 'wox_demo.dart';

class WoxActionPanelDemo extends StatefulWidget {
  const WoxActionPanelDemo({super.key, required this.accent, required this.hotkey, required this.queryAccessory, required this.tr});

  final Color accent;
  final String hotkey;
  final Widget? queryAccessory;
  final String Function(String key) tr;

  @override
  State<WoxActionPanelDemo> createState() => _WoxActionPanelDemoState();
}

class _WoxActionPanelDemoState extends State<WoxActionPanelDemo> with SingleTickerProviderStateMixin {
  late final AnimationController _controller;

  @override
  void initState() {
    super.initState();
    _controller = AnimationController(vsync: this, duration: const Duration(milliseconds: 3600))..repeat();
  }

  @override
  void dispose() {
    _controller.dispose();
    super.dispose();
  }

  double _curvedPhase(double start, double end, Curve curve) {
    final value = ((_controller.value - start) / (end - start)).clamp(0.0, 1.0).toDouble();
    return curve.transform(value);
  }

  double _panelProgress() {
    if (_controller.value < 0.26) {
      return 0;
    }
    if (_controller.value < 0.48) {
      return _curvedPhase(0.26, 0.48, Curves.easeOutCubic);
    }
    if (_controller.value < 0.88) {
      return 1;
    }
    return 1 - _curvedPhase(0.88, 1, Curves.easeInCubic);
  }

  bool _isShortcutPressed() {
    return _controller.value >= 0.18 && _controller.value <= 0.42;
  }

  @override
  Widget build(BuildContext context) {
    // Feature change: the Action Panel onboarding preview is now shaped like the
    // real launcher instead of a static two-column sketch. The old version
    // named the actions but did not teach the Alt+J transition, so this compact
    // animation keeps the query, result list, footer shortcuts, and floating
    // action panel recognizable while staying cheap to render in the guide.
    return AnimatedBuilder(
      animation: _controller,
      builder: (context, child) {
        final panelProgress = _panelProgress();
        final shortcutPressed = _isShortcutPressed();

        // Feature refinement: Action Panel now runs inside the shared desktop
        // frame. The previous standalone Wox card lacked the same system
        // context as the other onboarding demos, so wrapping the real launcher
        // mock keeps the visual language consistent without changing behavior.
        return WoxDemoFramedDesktop(
          accent: widget.accent,
          child: Padding(
            // Symmetric vertical padding centres the Wox window within the
            // desktop frame. The previous 76/44 split was a leftover from
            // when a hint strip occupied the top, which this demo does not have.
            padding: const EdgeInsets.fromLTRB(48, 44, 52, 44),
            child: WoxDemoWindow(
              accent: widget.accent,
              query: 'sett',
              queryAccessory: widget.queryAccessory,
              footerHotkey: widget.hotkey,
              isFooterHotkeyPressed: shortcutPressed,
              actionPanelProgress: panelProgress,
              actionPanel: WoxDemoActionPanel(accent: widget.accent, tr: widget.tr),
              opaqueBackground: true,
            ),
          ),
        );
      },
    );
  }
}

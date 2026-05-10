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

  @override
  State<_InstallFlowDemo> createState() => _InstallFlowDemoState();
}

class _InstallFlowDemoState extends State<_InstallFlowDemo> with SingleTickerProviderStateMixin {
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

  String _queryText() {
    final typingProgress = _interval(0.10, 0.56, Curves.easeOutCubic);
    final rawStage = (typingProgress * (widget.queryStages.length - 1)).floor();
    final stage = rawStage.clamp(0, widget.queryStages.length - 1).toInt();
    return widget.queryStages[stage];
  }

  double _installProgress() {
    if (_controller.value < 0.64) {
      return 0;
    }
    if (_controller.value < 0.78) {
      return _interval(0.64, 0.78, Curves.easeOutCubic);
    }
    if (_controller.value < 0.94) {
      return 1;
    }
    return 1 - _interval(0.94, 1, Curves.easeInCubic);
  }

  String _primaryTail() {
    if (_controller.value >= 0.64 && _controller.value < 0.76) {
      return widget.installingLabel;
    }
    if (_controller.value >= 0.76 && _controller.value < 0.94) {
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
                      WoxDemoHintCard(
                        accent: widget.accent,
                        icon: widget.icon,
                        title: widget.title,
                        from: widget.hintFrom,
                        to: widget.hintTo,
                        progress: 0.45 + (0.55 * installProgress),
                      ),
                      const SizedBox(height: 12),
                      Expanded(
                        child: WoxDemoWindow(
                          accent: widget.accent,
                          query: _queryText(),
                          opaqueBackground: true,
                          results: [
                            WoxDemoResult(title: widget.primaryTitle, subtitle: widget.primarySubtitle, icon: widget.primaryIcon, selected: true, tail: _primaryTail()),
                            ...widget.secondaryResults,
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

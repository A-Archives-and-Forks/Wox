part of 'wox_demo.dart';

class WoxSelectionHotkeyDemo extends StatefulWidget {
  const WoxSelectionHotkeyDemo({super.key, required this.accent, required this.hotkey, required this.tr});

  final Color accent;
  final String hotkey;
  final String Function(String key) tr;

  @override
  State<WoxSelectionHotkeyDemo> createState() => _WoxSelectionHotkeyDemoState();
}

class _WoxSelectionHotkeyDemoState extends State<WoxSelectionHotkeyDemo> with SingleTickerProviderStateMixin {
  static const String _selectedFileName = 'Quarterly plan.pdf';

  late final AnimationController _controller;

  @override
  void initState() {
    super.initState();
    _controller = AnimationController(vsync: this, duration: const Duration(milliseconds: 5200))..repeat();
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

  double _cursorProgress() {
    if (_controller.value < 0.08) {
      return 0;
    }
    if (_controller.value < 0.34) {
      return _interval(0.08, 0.34, Curves.easeInOutCubic);
    }
    return 1;
  }

  double _shortcutProgress() {
    if (_controller.value < 0.36) {
      return 0;
    }
    if (_controller.value < 0.46) {
      return _interval(0.36, 0.46, Curves.easeOutCubic);
    }
    if (_controller.value < 0.66) {
      return 1;
    }
    return 1 - _interval(0.66, 0.78, Curves.easeInCubic);
  }

  double _windowProgress() {
    if (_controller.value < 0.56) {
      return 0;
    }
    if (_controller.value < 0.74) {
      return _interval(0.56, 0.74, Curves.easeOutCubic);
    }
    if (_controller.value < 0.92) {
      return 1;
    }
    return 1 - _interval(0.92, 1, Curves.easeInCubic);
  }

  bool _isFileSelected() {
    return _controller.value >= 0.30 && _controller.value < 0.95;
  }

  bool _isShortcutPressed() {
    return _controller.value >= 0.46 && _controller.value <= 0.58;
  }

  String _displayHotkey() {
    return _formatDemoHotkey(widget.hotkey, fallback: Platform.isMacOS ? 'cmd+option+space' : 'ctrl+alt+space');
  }

  @override
  Widget build(BuildContext context) {
    final hotkey = _displayHotkey();
    final desktopIsMac = Platform.isMacOS;

    // Feature change: the selection-hotkey preview now demonstrates the real
    // workflow: choose something on the desktop, press the configured shortcut,
    // and open Wox with context-specific actions for that selection.
    return AnimatedBuilder(
      key: const ValueKey('onboarding-selection-hotkey-demo'),
      animation: _controller,
      builder: (context, child) {
        final cursorProgress = _cursorProgress();
        final shortcutProgress = _shortcutProgress();
        final windowProgress = _windowProgress();
        final fileSelected = _isFileSelected();

        return LayoutBuilder(
          builder: (context, constraints) {
            final startCursor = Offset(constraints.maxWidth - 96, constraints.maxHeight - 86);
            final targetCursor = Offset(186, 112);
            final cursorOffset = Offset.lerp(startCursor, targetCursor, cursorProgress)!;
            final cursorOpacity = 1 - _interval(0.70, 0.86, Curves.easeInCubic);

            return ClipRRect(
              borderRadius: BorderRadius.circular(8),
              child: Stack(
                children: [
                  Positioned.fill(child: WoxDemoDesktopBackground(accent: widget.accent, isMac: desktopIsMac, showDefaultIcons: false)),
                  Positioned(left: 42, top: 54, child: WoxDemoDesktopFileIcon(label: 'Roadmap.md', icon: Icons.article_outlined, accent: const Color(0xFF60A5FA))),
                  Positioned(
                    left: 150,
                    top: 54,
                    child: WoxDemoDesktopFileIcon(label: _selectedFileName, icon: Icons.picture_as_pdf_outlined, accent: widget.accent, selected: fileSelected),
                  ),
                  Positioned(left: 258, top: 54, child: WoxDemoDesktopFileIcon(label: 'Screenshots', icon: Icons.folder_outlined, accent: const Color(0xFFFACC15))),
                  Positioned(left: 64, top: 150, child: WoxDemoDesktopFileIcon(label: 'Release notes.txt', icon: Icons.description_outlined, accent: const Color(0xFF34D399))),
                  if (cursorOpacity > 0.01)
                    Positioned(left: cursorOffset.dx, top: cursorOffset.dy, child: Opacity(opacity: cursorOpacity, child: _DemoCursor(accent: widget.accent))),
                  Positioned.fill(
                    child: Opacity(
                      opacity: shortcutProgress,
                      child: Transform.translate(
                        offset: Offset(0, 8 * (1 - shortcutProgress)),
                        child: _HotkeyPressOverlay(hotkey: hotkey, accent: widget.accent, pressed: _isShortcutPressed()),
                      ),
                    ),
                  ),
                  // Feature refinement: the selection launcher appears fully
                  // opaque, matching the main-hotkey demo and making the file
                  // action rows readable over the simulated desktop.
                  if (windowProgress > 0.01)
                    Positioned.fill(
                      child: Transform.translate(
                        offset: Offset(0, 20 * (1 - windowProgress)),
                        child: Transform.scale(
                          scale: 0.95 + (0.05 * windowProgress),
                          child: Padding(
                            padding: const EdgeInsets.fromLTRB(52, 134, 52, 38),
                            child: WoxDemoWindow(
                              accent: widget.accent,
                              query: _selectedFileName,
                              opaqueBackground: true,
                              results: [
                                WoxDemoResult(
                                  title: 'Quick Actions',
                                  subtitle: _selectedFileName,
                                  icon: const Icon(Icons.touch_app_outlined, color: Colors.white, size: 23),
                                  selected: true,
                                  tail: hotkey,
                                ),
                                WoxDemoResult(
                                  title: 'Open file',
                                  subtitle: 'Open the selected desktop file',
                                  icon: Icon(Icons.open_in_new_rounded, color: widget.accent, size: 23),
                                  tail: 'Enter',
                                ),
                                const WoxDemoResult(
                                  title: 'Copy file path',
                                  subtitle: r'C:\Users\qianl\Desktop\Quarterly plan.pdf',
                                  icon: Icon(Icons.copy_rounded, color: Color(0xFF38BDF8), size: 23),
                                  tail: 'Copy',
                                ),
                                const WoxDemoResult(
                                  title: 'Show in folder',
                                  subtitle: 'Reveal the selected file in its location',
                                  icon: Icon(Icons.folder_open_outlined, color: Color(0xFFFACC15), size: 23),
                                ),
                              ],
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

part of 'wox_demo.dart';

class WoxDemoDesktopFileIcon extends StatelessWidget {
  const WoxDemoDesktopFileIcon({super.key, required this.label, required this.icon, required this.accent, this.selected = false});

  final String label;
  final IconData icon;
  final Color accent;
  final bool selected;

  @override
  Widget build(BuildContext context) {
    final textColor = getThemeTextColor();

    return AnimatedContainer(
      duration: const Duration(milliseconds: 180),
      width: 86,
      padding: const EdgeInsets.symmetric(horizontal: 7, vertical: 8),
      decoration: BoxDecoration(
        color: selected ? getThemeActiveBackgroundColor().withValues(alpha: 0.16) : Colors.transparent,
        border: Border.all(color: selected ? accent.withValues(alpha: 0.62) : Colors.transparent),
        borderRadius: BorderRadius.circular(8),
      ),
      child: Column(
        mainAxisSize: MainAxisSize.min,
        children: [
          AnimatedContainer(
            duration: const Duration(milliseconds: 180),
            width: 42,
            height: 42,
            decoration: BoxDecoration(
              color: accent.withValues(alpha: selected ? 0.90 : 0.72),
              borderRadius: BorderRadius.circular(10),
              boxShadow: [BoxShadow(color: accent.withValues(alpha: selected ? 0.32 : 0.16), blurRadius: selected ? 18 : 10)],
            ),
            child: Icon(icon, color: Colors.white.withValues(alpha: 0.96), size: 25),
          ),
          const SizedBox(height: 7),
          Text(
            label,
            textAlign: TextAlign.center,
            maxLines: 2,
            overflow: TextOverflow.ellipsis,
            style: TextStyle(color: textColor.withValues(alpha: selected ? 0.96 : 0.82), fontSize: 10, fontWeight: selected ? FontWeight.w700 : FontWeight.w600, height: 1.12),
          ),
        ],
      ),
    );
  }
}

class _DemoCursor extends StatelessWidget {
  const _DemoCursor({required this.accent});

  final Color accent;

  @override
  Widget build(BuildContext context) {
    return Transform.rotate(
      angle: -0.55,
      child: Icon(
        Icons.navigation_rounded,
        color: getThemeTextColor(),
        size: 28,
        shadows: [Shadow(color: Colors.black.withValues(alpha: 0.34), blurRadius: 8, offset: const Offset(0, 3)), Shadow(color: accent.withValues(alpha: 0.16), blurRadius: 14)],
      ),
    );
  }
}

class WoxDemoDesktopBackground extends StatelessWidget {
  const WoxDemoDesktopBackground({super.key, required this.accent, required this.isMac, this.showDefaultIcons = true});

  final Color accent;
  final bool isMac;
  final bool showDefaultIcons;

  @override
  Widget build(BuildContext context) {
    final textColor = getThemeTextColor();
    final backgroundColor = getThemeBackgroundColor();
    final desktopTint = Color.lerp(backgroundColor, accent, 0.10)!;

    return DecoratedBox(
      decoration: BoxDecoration(
        gradient: LinearGradient(begin: Alignment.topLeft, end: Alignment.bottomRight, colors: [desktopTint, backgroundColor, textColor.withValues(alpha: 0.08)]),
      ),
      child: Stack(
        children: [
          // Feature refinement: selection demos provide their own desktop files,
          // while the main-hotkey demo keeps generic icons to suggest an idle
          // system desktop. The toggle avoids duplicate icons in shared chrome.
          if (showDefaultIcons) ...[
            Positioned(left: 28, top: 34, child: _DesktopFolderIcon(label: 'Apps', accent: accent)),
            Positioned(left: 28, top: 112, child: _DesktopFolderIcon(label: 'Files', accent: const Color(0xFFFACC15))),
          ],
          if (isMac) ...[
            const Positioned(left: 0, right: 0, top: 0, child: _MacMenuBar()),
            const Positioned(left: 0, right: 0, bottom: 14, child: _MacDock()),
          ] else ...[
            const Positioned(left: 0, right: 0, bottom: 0, child: _WindowsTaskbar()),
          ],
        ],
      ),
    );
  }
}

class WoxDemoFramedDesktop extends StatelessWidget {
  const WoxDemoFramedDesktop({super.key, required this.accent, required this.child});

  final Color accent;
  final Widget child;

  @override
  Widget build(BuildContext context) {
    // Feature refinement: several onboarding demos now share the same simulated
    // desktop frame. The previous standalone cards made Glance and Action Panel
    // feel visually detached, while this wrapper keeps their preview chrome
    // consistent with the hotkey and query sections. It intentionally keeps
    // desktop file icons hidden so feature-specific demos control their own
    // foreground content without visual clutter.
    return ClipRRect(
      borderRadius: BorderRadius.circular(8),
      child: Stack(children: [Positioned.fill(child: WoxDemoDesktopBackground(accent: accent, isMac: Platform.isMacOS, showDefaultIcons: false)), Positioned.fill(child: child)]),
    );
  }
}

class _DesktopFolderIcon extends StatelessWidget {
  const _DesktopFolderIcon({required this.label, required this.accent});

  final String label;
  final Color accent;

  @override
  Widget build(BuildContext context) {
    return SizedBox(
      width: 58,
      child: Column(
        children: [
          Container(
            width: 38,
            height: 30,
            decoration: BoxDecoration(
              color: accent.withValues(alpha: 0.72),
              borderRadius: BorderRadius.circular(7),
              boxShadow: [BoxShadow(color: accent.withValues(alpha: 0.22), blurRadius: 12)],
            ),
            child: Icon(Icons.folder_rounded, color: Colors.white.withValues(alpha: 0.94), size: 23),
          ),
          const SizedBox(height: 5),
          Text(
            label,
            maxLines: 1,
            overflow: TextOverflow.ellipsis,
            style: TextStyle(color: getThemeTextColor().withValues(alpha: 0.82), fontSize: 10, fontWeight: FontWeight.w600),
          ),
        ],
      ),
    );
  }
}

class _MacMenuBar extends StatelessWidget {
  const _MacMenuBar();

  @override
  Widget build(BuildContext context) {
    return Container(
      height: 28,
      padding: const EdgeInsets.symmetric(horizontal: 14),
      decoration: BoxDecoration(color: getThemeBackgroundColor().withValues(alpha: 0.72), border: Border(bottom: BorderSide(color: getThemeTextColor().withValues(alpha: 0.08)))),
      child: Row(
        children: [
          Icon(Icons.apple, color: getThemeTextColor(), size: 16),
          const SizedBox(width: 12),
          Text('Finder', style: TextStyle(color: getThemeTextColor(), fontSize: 11, fontWeight: FontWeight.w700)),
          const SizedBox(width: 12),
          Text('File', style: TextStyle(color: getThemeTextColor().withValues(alpha: 0.74), fontSize: 11)),
          const Spacer(),
          Icon(Icons.search_rounded, color: getThemeTextColor().withValues(alpha: 0.72), size: 15),
          const SizedBox(width: 12),
          Text('09:41', style: TextStyle(color: getThemeTextColor().withValues(alpha: 0.78), fontSize: 11, fontWeight: FontWeight.w600)),
        ],
      ),
    );
  }
}

class _MacDock extends StatelessWidget {
  const _MacDock();

  @override
  Widget build(BuildContext context) {
    final colors = [const Color(0xFF60A5FA), const Color(0xFF34D399), const Color(0xFFF97316), const Color(0xFFF43F5E), const Color(0xFFA78BFA)];
    return Center(
      child: Container(
        height: 44,
        padding: const EdgeInsets.symmetric(horizontal: 12),
        decoration: BoxDecoration(
          color: getThemeBackgroundColor().withValues(alpha: 0.68),
          border: Border.all(color: getThemeTextColor().withValues(alpha: 0.10)),
          borderRadius: BorderRadius.circular(16),
          boxShadow: [BoxShadow(color: Colors.black.withValues(alpha: 0.10), blurRadius: 24, offset: const Offset(0, 10))],
        ),
        child: Row(
          mainAxisSize: MainAxisSize.min,
          children: [
            for (final color in colors)
              Container(
                width: 26,
                height: 26,
                margin: const EdgeInsets.symmetric(horizontal: 4),
                decoration: BoxDecoration(color: color.withValues(alpha: 0.82), borderRadius: BorderRadius.circular(7)),
              ),
          ],
        ),
      ),
    );
  }
}

class _WindowsTaskbar extends StatelessWidget {
  const _WindowsTaskbar();

  @override
  Widget build(BuildContext context) {
    return Container(
      height: 42,
      padding: const EdgeInsets.symmetric(horizontal: 12),
      decoration: BoxDecoration(color: getThemeBackgroundColor().withValues(alpha: 0.78), border: Border(top: BorderSide(color: getThemeTextColor().withValues(alpha: 0.08)))),
      child: Row(
        children: [
          Container(
            width: 24,
            height: 24,
            decoration: BoxDecoration(color: const Color(0xFF3B82F6), borderRadius: BorderRadius.circular(5)),
            child: const Icon(Icons.window_rounded, color: Colors.white, size: 15),
          ),
          const SizedBox(width: 10),
          Container(
            width: 110,
            height: 25,
            padding: const EdgeInsets.symmetric(horizontal: 9),
            decoration: BoxDecoration(color: getThemeTextColor().withValues(alpha: 0.08), borderRadius: BorderRadius.circular(999)),
            child: Row(
              children: [
                Icon(Icons.search_rounded, color: getThemeTextColor().withValues(alpha: 0.58), size: 14),
                const SizedBox(width: 5),
                Text('Search', style: TextStyle(color: getThemeTextColor().withValues(alpha: 0.54), fontSize: 10)),
              ],
            ),
          ),
          const Spacer(),
          Icon(Icons.wifi_rounded, color: getThemeTextColor().withValues(alpha: 0.70), size: 14),
          const SizedBox(width: 10),
          Text('09:41', style: TextStyle(color: getThemeTextColor().withValues(alpha: 0.76), fontSize: 10, fontWeight: FontWeight.w600)),
        ],
      ),
    );
  }
}

class _HotkeyPressOverlay extends StatelessWidget {
  const _HotkeyPressOverlay({required this.hotkey, required this.accent, required this.pressed});

  final String hotkey;
  final Color accent;
  final bool pressed;

  @override
  Widget build(BuildContext context) {
    final parts = hotkey.split('+');
    return Center(
      child: Container(
        padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 13),
        decoration: BoxDecoration(
          color: getThemeBackgroundColor().withValues(alpha: 0.78),
          border: Border.all(color: pressed ? accent.withValues(alpha: 0.88) : getThemeTextColor().withValues(alpha: 0.12)),
          borderRadius: BorderRadius.circular(14),
          boxShadow: [
            BoxShadow(color: Colors.black.withValues(alpha: 0.16), blurRadius: 28, offset: const Offset(0, 14)),
            BoxShadow(color: accent.withValues(alpha: pressed ? 0.24 : 0.10), blurRadius: 28),
          ],
        ),
        child: Row(
          mainAxisSize: MainAxisSize.min,
          children: [
            for (var index = 0; index < parts.length; index++) ...[
              _DemoKeycap(label: parts[index], accent: accent, pressed: pressed),
              if (index < parts.length - 1)
                Padding(
                  padding: const EdgeInsets.symmetric(horizontal: 8),
                  child: Text('+', style: TextStyle(color: getThemeTextColor().withValues(alpha: 0.50), fontSize: 16, fontWeight: FontWeight.w800)),
                ),
            ],
          ],
        ),
      ),
    );
  }
}

class _DemoKeycap extends StatelessWidget {
  const _DemoKeycap({required this.label, required this.accent, required this.pressed});

  final String label;
  final Color accent;
  final bool pressed;

  @override
  Widget build(BuildContext context) {
    return AnimatedScale(
      duration: const Duration(milliseconds: 120),
      curve: Curves.easeOutCubic,
      scale: pressed ? 0.94 : 1,
      child: AnimatedContainer(
        duration: const Duration(milliseconds: 120),
        curve: Curves.easeOutCubic,
        height: 34,
        constraints: const BoxConstraints(minWidth: 58),
        padding: const EdgeInsets.symmetric(horizontal: 12),
        alignment: Alignment.center,
        decoration: BoxDecoration(
          color: pressed ? accent.withValues(alpha: 0.22) : getThemeTextColor().withValues(alpha: 0.06),
          border: Border.all(color: pressed ? accent : getThemeTextColor().withValues(alpha: 0.24)),
          borderRadius: BorderRadius.circular(7),
        ),
        child: Text(
          label,
          maxLines: 1,
          overflow: TextOverflow.ellipsis,
          style: TextStyle(color: pressed ? accent : getThemeTextColor(), fontSize: 12, fontWeight: FontWeight.w800),
        ),
      ),
    );
  }
}

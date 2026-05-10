part of 'wox_demo.dart';

class _TrayQueryIcon extends StatelessWidget {
  const _TrayQueryIcon({required this.accent, required this.pressed, this.size = 28});

  final Color accent;
  final bool pressed;
  final double size;

  @override
  Widget build(BuildContext context) {
    // Feature refinement: the tray query affordance should read as a real
    // system-tray glyph, not a launcher-sized button. Keeping the size explicit
    // also lets the tray demo anchor Wox geometry to the same visual bounds.
    return AnimatedContainer(
      duration: const Duration(milliseconds: 140),
      width: size,
      height: size,
      decoration: BoxDecoration(
        color: pressed ? accent.withValues(alpha: 0.90) : getThemeBackgroundColor().withValues(alpha: 0.88),
        border: Border.all(color: pressed ? accent : getThemeTextColor().withValues(alpha: 0.14)),
        borderRadius: BorderRadius.circular(8),
        boxShadow: [BoxShadow(color: accent.withValues(alpha: pressed ? 0.24 : 0.08), blurRadius: pressed ? 14 : 8)],
      ),
      child: Icon(Icons.wb_sunny_outlined, color: pressed ? Colors.white : accent, size: size * 0.56),
    );
  }
}

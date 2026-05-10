part of 'wox_demo.dart';

class _ExpansionBadge extends StatelessWidget {
  const _ExpansionBadge({required this.accent, required this.from, required this.to});

  final Color accent;
  final String from;
  final String to;

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 8),
      decoration: BoxDecoration(
        color: getThemeBackgroundColor().withValues(alpha: 0.94),
        border: Border.all(color: accent.withValues(alpha: 0.32)),
        borderRadius: BorderRadius.circular(8),
        boxShadow: [BoxShadow(color: Colors.black.withValues(alpha: 0.12), blurRadius: 20, offset: const Offset(0, 10))],
      ),
      child: Row(
        mainAxisSize: MainAxisSize.min,
        children: [
          Text(from, style: TextStyle(color: accent, fontSize: 12, fontWeight: FontWeight.w800)),
          Padding(padding: const EdgeInsets.symmetric(horizontal: 8), child: Icon(Icons.arrow_forward_rounded, color: getThemeSubTextColor(), size: 16)),
          Text(to, style: TextStyle(color: getThemeTextColor(), fontSize: 12, fontWeight: FontWeight.w700)),
        ],
      ),
    );
  }
}

class WoxDemoHintCard extends StatelessWidget {
  const WoxDemoHintCard({super.key, required this.accent, required this.icon, required this.title, required this.from, required this.to, this.progress = 1});

  final Color accent;
  final IconData icon;
  final String title;
  final String from;
  final String to;
  final double progress;

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 11),
      decoration: BoxDecoration(
        color: getThemeBackgroundColor().withValues(alpha: 0.92),
        border: Border.all(color: getThemeTextColor().withValues(alpha: 0.10)),
        borderRadius: BorderRadius.circular(8),
        boxShadow: [BoxShadow(color: Colors.black.withValues(alpha: 0.12), blurRadius: 22, offset: const Offset(0, 10))],
      ),
      child: Row(
        children: [
          Icon(icon, color: accent, size: 20),
          const SizedBox(width: 8),
          Flexible(
            flex: 2,
            child: Text(title, maxLines: 1, overflow: TextOverflow.ellipsis, style: TextStyle(color: getThemeTextColor(), fontSize: 13, fontWeight: FontWeight.w700)),
          ),
          const Spacer(),
          Opacity(
            opacity: progress.clamp(0.0, 1.0).toDouble(),
            child: Transform.translate(offset: Offset(0, 6 * (1 - progress.clamp(0.0, 1.0).toDouble())), child: _ExpansionBadge(accent: accent, from: from, to: to)),
          ),
        ],
      ),
    );
  }
}

part of 'wox_demo.dart';

class WoxDemoActionPanel extends StatelessWidget {
  const WoxDemoActionPanel({super.key, required this.accent, required this.tr});

  final Color accent;
  final String Function(String key) tr;

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.all(12),
      decoration: BoxDecoration(
        color: getThemeBackgroundColor().withValues(alpha: 0.94),
        borderRadius: BorderRadius.circular(8),
        border: Border.all(color: getThemeTextColor().withValues(alpha: 0.07)),
        boxShadow: [BoxShadow(color: Colors.black.withValues(alpha: 0.18), blurRadius: 28, offset: const Offset(0, 16))],
      ),
      child: Column(
        mainAxisSize: MainAxisSize.min,
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text('Actions', style: TextStyle(color: getThemeTextColor(), fontSize: 15, fontWeight: FontWeight.w700)),
          const SizedBox(height: 9),
          Container(height: 1, color: getThemeTextColor().withValues(alpha: 0.54)),
          const SizedBox(height: 8),
          _MiniActionRow(accent: accent, icon: Icons.play_arrow_rounded, title: 'Execute', selected: true),
          const SizedBox(height: 8),
          _MiniActionRow(accent: accent, icon: Icons.push_pin_outlined, title: tr('onboarding_action_panel_copy')),
          const SizedBox(height: 8),
          _MiniActionRow(accent: accent, icon: Icons.more_horiz, title: tr('onboarding_action_panel_more')),
        ],
      ),
    );
  }
}

class _MiniActionRow extends StatelessWidget {
  const _MiniActionRow({required this.accent, required this.icon, required this.title, this.selected = false});

  final Color accent;
  final IconData icon;
  final String title;
  final bool selected;

  @override
  Widget build(BuildContext context) {
    return Container(
      height: 32,
      padding: const EdgeInsets.symmetric(horizontal: 9),
      decoration: BoxDecoration(color: selected ? accent.withValues(alpha: 0.82) : getThemeTextColor().withValues(alpha: 0.055), borderRadius: BorderRadius.circular(7)),
      child: Row(
        children: [
          Icon(icon, size: 17, color: selected ? Colors.white : accent),
          const SizedBox(width: 9),
          Expanded(
            child: Text(
              title,
              maxLines: 1,
              overflow: TextOverflow.ellipsis,
              style: TextStyle(color: selected ? Colors.white : getThemeTextColor(), fontSize: 12, fontWeight: FontWeight.w700),
            ),
          ),
        ],
      ),
    );
  }
}

class _MiniFooter extends StatelessWidget {
  const _MiniFooter({required this.accent, required this.hotkey, required this.isPressed});

  final Color accent;
  final String hotkey;
  final bool isPressed;

  @override
  Widget build(BuildContext context) {
    final keyLabels = hotkey.split('+');
    final metrics = WoxInterfaceSizeUtil.instance.current;

    return Positioned(
      left: 0,
      right: 0,
      bottom: 0,
      child: Container(
        height: metrics.toolbarHeight,
        padding: EdgeInsets.symmetric(horizontal: metrics.scaledSpacing(12)),
        decoration: BoxDecoration(color: getThemeTextColor().withValues(alpha: 0.035), border: Border(top: BorderSide(color: getThemeTextColor().withValues(alpha: 0.07)))),
        child: FittedBox(
          alignment: Alignment.centerRight,
          fit: BoxFit.scaleDown,
          child: Row(
            mainAxisSize: MainAxisSize.min,
            children: [
              Text('Execute', style: TextStyle(color: getThemeTextColor(), fontSize: metrics.toolbarFontSize)),
              SizedBox(width: metrics.toolbarActionNameHotkeySpacing),
              _MiniShortcutKey(label: 'Enter', accent: accent, active: false),
              SizedBox(width: metrics.toolbarActionSpacing),
              Text('More Actions', style: TextStyle(color: isPressed ? accent : getThemeTextColor(), fontSize: metrics.toolbarFontSize)),
              SizedBox(width: metrics.toolbarActionNameHotkeySpacing),
              for (var index = 0; index < keyLabels.length; index++) ...[
                _MiniShortcutKey(label: keyLabels[index], accent: accent, active: isPressed),
                if (index < keyLabels.length - 1) SizedBox(width: metrics.toolbarHotkeyKeySpacing),
              ],
            ],
          ),
        ),
      ),
    );
  }
}

class _MiniShortcutKey extends StatelessWidget {
  const _MiniShortcutKey({required this.label, required this.accent, required this.active});

  final String label;
  final Color accent;
  final bool active;

  @override
  Widget build(BuildContext context) {
    final metrics = WoxInterfaceSizeUtil.instance.current;

    return AnimatedContainer(
      duration: const Duration(milliseconds: 140),
      height: metrics.scaledSpacing(22),
      constraints: BoxConstraints(minWidth: metrics.scaledSpacing(28)),
      padding: EdgeInsets.symmetric(horizontal: metrics.scaledSpacing(7)),
      decoration: BoxDecoration(
        color: active ? accent.withValues(alpha: 0.20) : Colors.transparent,
        border: Border.all(color: active ? accent : getThemeTextColor().withValues(alpha: 0.66)),
        borderRadius: BorderRadius.circular(4),
      ),
      alignment: Alignment.center,
      child: Text(
        label,
        maxLines: 1,
        overflow: TextOverflow.ellipsis,
        style: TextStyle(color: active ? accent : getThemeTextColor(), fontSize: metrics.tailHotkeyFontSize, fontWeight: FontWeight.w500),
      ),
    );
  }
}

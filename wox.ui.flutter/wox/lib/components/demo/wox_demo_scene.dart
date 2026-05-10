part of 'wox_demo.dart';

class WoxDemoDesktopFileItem {
  const WoxDemoDesktopFileItem({required this.label, required this.icon, required this.accent, required this.left, required this.top, this.selected = false});

  final String label;
  final IconData icon;
  final Color accent;
  final double left;
  final double top;
  final bool selected;
}

class WoxDemoScene extends StatelessWidget {
  const WoxDemoScene({
    super.key,
    required this.accent,
    required this.child,
    this.hint,
    this.desktopFiles = const [],
    this.showDefaultIcons = false,
    this.contentPadding = const EdgeInsets.fromLTRB(48, 82, 52, 44),
  });

  final Color accent;
  final Widget child;
  final Widget? hint;
  final List<WoxDemoDesktopFileItem> desktopFiles;
  final bool showDefaultIcons;
  final EdgeInsetsGeometry contentPadding;

  @override
  Widget build(BuildContext context) {
    // Feature extraction: onboarding, settings, and future tooltips need the same simulated desktop shell. Keeping that shell here avoids each feature demo reimplementing file icons, OS chrome, and padding rules differently.
    return ClipRRect(
      borderRadius: BorderRadius.circular(8),
      child: Stack(
        children: [
          Positioned.fill(child: WoxDemoDesktopBackground(accent: accent, isMac: Platform.isMacOS, showDefaultIcons: showDefaultIcons)),
          for (final file in desktopFiles)
            Positioned(left: file.left, top: file.top, child: WoxDemoDesktopFileIcon(label: file.label, icon: file.icon, accent: file.accent, selected: file.selected)),
          Positioned.fill(
            child: Padding(
              padding: contentPadding,
              child: Column(
                children: [
                  if (hint != null) ...[hint!, const SizedBox(height: 12)],
                  Expanded(child: child),
                ],
              ),
            ),
          ),
        ],
      ),
    );
  }
}

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
              // When a hint card is present the default top padding (82px) is
              // intentionally larger than the bottom (44px) to leave room for
              // the hint strip. Without a hint the asymmetry pushes the Wox
              // window below the visual center, so we equalise both sides.
              padding: hint != null ? contentPadding : contentPadding.resolve(TextDirection.ltr).copyWith(top: contentPadding.resolve(TextDirection.ltr).bottom),
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

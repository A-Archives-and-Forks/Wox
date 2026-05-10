part of 'wox_demo.dart';

class WoxWpmInstallDemo extends StatelessWidget {
  const WoxWpmInstallDemo({super.key, required this.accent, required this.tr});

  final Color accent;
  final String Function(String key) tr;

  @override
  Widget build(BuildContext context) {
    return _InstallFlowDemo(
      demoKey: const ValueKey('onboarding-wpm-install-demo'),
      accent: accent,
      icon: Icons.extension_outlined,
      title: tr('onboarding_wpm_install_title'),
      hintFrom: 'wpm install',
      hintTo: tr('onboarding_wpm_install_hint_target'),
      queryStages: const ['', 'w', 'wpm', 'wpm install', 'wpm install clip', 'wpm install clipboard'],
      installLabel: tr('plugin_wpm_install'),
      installingLabel: tr('plugin_wpm_installing'),
      installedLabel: tr('plugin_wpm_start_using'),
      primaryTitle: 'Clipboard History',
      primarySubtitle: tr('onboarding_wpm_install_result_subtitle'),
      primaryIcon: const Icon(Icons.content_paste_search_outlined, color: Colors.white, size: 23),
      secondaryResults: [
        WoxDemoResult(
          title: 'Browser Bookmarks',
          subtitle: tr('plugin_wpm_command_install'),
          icon: const Icon(Icons.bookmark_outline_rounded, color: Color(0xFFFACC15), size: 23),
          tail: tr('plugin_wpm_install'),
        ),
        const WoxDemoResult(title: 'ChatGPT', subtitle: 'AI assistant plugin', icon: Icon(Icons.auto_awesome_outlined, color: Color(0xFFA78BFA), size: 23), tail: 'AI'),
      ],
    );
  }
}

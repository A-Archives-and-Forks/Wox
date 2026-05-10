part of 'wox_demo.dart';

class WoxThemeInstallDemo extends StatelessWidget {
  const WoxThemeInstallDemo({super.key, required this.accent, required this.tr});

  final Color accent;
  final String Function(String key) tr;

  @override
  Widget build(BuildContext context) {
    return _InstallFlowDemo(
      demoKey: const ValueKey('onboarding-theme-install-demo'),
      accent: accent,
      icon: Icons.palette_outlined,
      title: tr('onboarding_theme_install_title'),
      hintFrom: 'theme',
      hintTo: tr('onboarding_theme_install_hint_target'),
      queryStages: const ['', 't', 'theme', 'theme ocean', 'theme ocean dark'],
      installLabel: tr('plugin_theme_install_theme'),
      installingLabel: tr('plugin_wpm_installing'),
      installedLabel: tr('ui_setting_theme_apply'),
      primaryTitle: 'Ocean Dark',
      primarySubtitle: tr('onboarding_theme_install_result_subtitle'),
      primaryIcon: const _ThemeSwatchIcon(background: Color(0xFF0F172A), accent: Color(0xFF38BDF8), highlight: Color(0xFF22C55E)),
      secondaryResults: [
        WoxDemoResult(
          title: 'Aurora',
          subtitle: tr('plugin_theme_group_store'),
          icon: const _ThemeSwatchIcon(background: Color(0xFF261A3D), accent: Color(0xFFE879F9), highlight: Color(0xFFFACC15)),
          tail: tr('plugin_theme_install_theme'),
        ),
        WoxDemoResult(
          title: 'Default Dark',
          subtitle: tr('plugin_theme_group_current'),
          icon: const _ThemeSwatchIcon(background: Color(0xFF1F2937), accent: Color(0xFF60A5FA), highlight: Color(0xFF94A3B8)),
          tail: tr('ui_setting_theme_system_tag'),
        ),
      ],
    );
  }
}

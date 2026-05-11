import 'package:flutter/material.dart';
import 'package:wox/components/demo/wox_demo.dart';
import 'package:wox/components/onboarding/wox_onboarding_step_layout.dart';
import 'package:wox/components/wox_hotkey_recorder_view.dart';
import 'package:wox/entity/wox_hotkey.dart';
import 'package:wox/utils/colors.dart';

class WoxMainHotkeyOnboarding extends StatelessWidget {
  const WoxMainHotkeyOnboarding({super.key, required this.accent, required this.hotkey, required this.tr, required this.onHotkeyChanged});

  final Color accent;
  final String hotkey;
  final String Function(String key) tr;
  final void Function(String hotkey) onHotkeyChanged;

  @override
  Widget build(BuildContext context) {
    return WoxOnboardingStepLayout(
      previewKey: ValueKey('onboarding-media-mainHotkey-$hotkey'),
      content: _HotkeyContent(body: tr('onboarding_main_hotkey_tip'), hotkey: hotkey, onHotkeyChanged: onHotkeyChanged),
      demo: WoxMainHotkeyDemo(accent: accent, hotkey: hotkey, tr: tr),
    );
  }
}

class WoxSelectionHotkeyOnboarding extends StatelessWidget {
  const WoxSelectionHotkeyOnboarding({super.key, required this.accent, required this.hotkey, required this.tr, required this.onHotkeyChanged});

  final Color accent;
  final String hotkey;
  final String Function(String key) tr;
  final void Function(String hotkey) onHotkeyChanged;

  @override
  Widget build(BuildContext context) {
    return WoxOnboardingStepLayout(
      previewKey: ValueKey('onboarding-media-selectionHotkey-$hotkey'),
      content: _HotkeyContent(body: tr('onboarding_selection_hotkey_description'), hotkey: hotkey, onHotkeyChanged: onHotkeyChanged),
      demo: WoxSelectionHotkeyDemo(accent: accent, hotkey: hotkey, tr: tr),
    );
  }
}

class _HotkeyContent extends StatelessWidget {
  const _HotkeyContent({required this.body, required this.hotkey, required this.onHotkeyChanged});

  final String body;
  final String hotkey;
  final void Function(String hotkey) onHotkeyChanged;

  @override
  Widget build(BuildContext context) {
    return WoxOnboardingSettingsPanel(
      children: [
        Text(body, style: TextStyle(color: getThemeSubTextColor(), fontSize: 14, height: 1.45)),
        const SizedBox(height: 26),
        Align(
          alignment: Alignment.centerLeft,
          child: WoxHotkeyRecorder(
            hotkey: WoxHotkey.parseHotkeyFromString(hotkey),
            tipPosition: WoxHotkeyRecorderTipPosition.right,
            onHotKeyRecorded: (value) {
              // Step extraction: the recorder UI is reusable for both hotkey
              // steps, but the parent still decides which setting key is saved.
              onHotkeyChanged(value);
            },
          ),
        ),
      ],
    );
  }
}

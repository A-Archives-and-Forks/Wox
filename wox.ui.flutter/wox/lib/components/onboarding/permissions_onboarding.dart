import 'dart:io';

import 'package:flutter/material.dart';
import 'package:uuid/v4.dart';
import 'package:wox/api/wox_api.dart';
import 'package:wox/components/demo/wox_demo.dart';
import 'package:wox/components/onboarding/wox_onboarding_step_layout.dart';
import 'package:wox/components/wox_button.dart';

class WoxPermissionsOnboarding extends StatelessWidget {
  const WoxPermissionsOnboarding({super.key, required this.accent, required this.tr, required this.isPermissionLoading, required this.accessibilityPassed});

  final Color accent;
  final String Function(String key) tr;
  final bool isPermissionLoading;
  final bool? accessibilityPassed;

  @override
  Widget build(BuildContext context) {
    return WoxOnboardingStepLayout(
      previewKey: const ValueKey('onboarding-media-permissions'),
      content: _buildContent(),
      demo: WoxDemoWindow(
        accent: accent,
        query: 'permissions',
        results: [
          WoxDemoResult(
            title: tr('onboarding_permission_accessibility_title'),
            subtitle: tr('onboarding_permission_accessibility_body'),
            icon: const Icon(Icons.accessibility_new_outlined, color: Colors.white, size: 22),
            selected: true,
            tail: tr('onboarding_permission_needs_action'),
          ),
          WoxDemoResult(
            title: tr('onboarding_permission_disk_title'),
            subtitle: tr('onboarding_permission_disk_body'),
            icon: Icon(Icons.folder_open_outlined, color: accent, size: 22),
            tail: tr('onboarding_permission_optional'),
          ),
          WoxDemoResult(
            title: tr('onboarding_permission_privacy_card'),
            subtitle: Platform.isMacOS ? tr('onboarding_permission_open_privacy') : tr('onboarding_permissions_lite_body'),
            icon: Icon(Icons.security_outlined, color: accent, size: 22),
            tail: tr('onboarding_permission_ready'),
          ),
          WoxDemoResult(
            title: tr('onboarding_permissions_lite_title'),
            subtitle: tr('onboarding_permissions_lite_body'),
            icon: Icon(Icons.verified_user_outlined, color: accent, size: 22),
            tail: Platform.operatingSystem,
          ),
        ],
      ),
    );
  }

  Widget _buildContent() {
    if (!Platform.isMacOS) {
      return WoxOnboardingInfoPanel(
        key: const ValueKey('onboarding-permission-lite'),
        title: tr('onboarding_permissions_lite_title'),
        body: tr('onboarding_permissions_lite_body'),
        badge: Platform.operatingSystem,
      );
    }

    final statusText =
        isPermissionLoading ? tr('onboarding_permission_checking') : (accessibilityPassed == true ? tr('onboarding_permission_ready') : tr('onboarding_permission_needs_action'));
    return Column(
      key: const ValueKey('onboarding-permission-macos'),
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        WoxOnboardingInfoPanel(title: tr('onboarding_permission_accessibility_title'), body: tr('onboarding_permission_accessibility_body'), badge: statusText),
        const SizedBox(height: 14),
        WoxOnboardingInfoPanel(title: tr('onboarding_permission_disk_title'), body: tr('onboarding_permission_disk_body'), badge: tr('onboarding_permission_optional')),
        const SizedBox(height: 20),
        Wrap(
          spacing: 12,
          runSpacing: 10,
          children: [
            // Step extraction: permission buttons remain in this step because
            // they are macOS-only UI, while WoxApi keeps the platform action
            // behind the existing bridge.
            WoxButton.secondary(text: tr('onboarding_permission_open_accessibility'), onPressed: () => WoxApi.instance.openAccessibilityPermission(const UuidV4().generate())),
            WoxButton.secondary(text: tr('onboarding_permission_open_privacy'), onPressed: () => WoxApi.instance.openPrivacyPermission(const UuidV4().generate())),
          ],
        ),
      ],
    );
  }
}

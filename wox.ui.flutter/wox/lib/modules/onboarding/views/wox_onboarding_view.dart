import 'dart:async';
import 'dart:convert';
import 'dart:io';

import 'package:flutter/material.dart';
import 'package:get/get.dart';
import 'package:uuid/v4.dart';
import 'package:wox/api/wox_api.dart';
import 'package:wox/components/wox_button.dart';
import 'package:wox/components/wox_dropdown_button.dart';
import 'package:wox/components/wox_hotkey_recorder_view.dart';
import 'package:wox/components/wox_image_view.dart';
import 'package:wox/components/wox_switch.dart';
import 'package:wox/controllers/wox_launcher_controller.dart';
import 'package:wox/controllers/wox_setting_controller.dart';
import 'package:wox/entity/wox_glance.dart';
import 'package:wox/entity/wox_hotkey.dart';
import 'package:wox/entity/wox_image.dart';
import 'package:wox/entity/wox_lang.dart';
import 'package:wox/utils/colors.dart';
import 'package:wox/utils/consts.dart';
import 'package:wox/utils/wox_interface_size_util.dart';
import 'package:wox/utils/wox_theme_util.dart';

const double _onboardingSidebarWidth = 256;
const double _onboardingFooterHeight = 72;

class WoxOnboardingView extends StatefulWidget {
  const WoxOnboardingView({super.key});

  @override
  State<WoxOnboardingView> createState() => _WoxOnboardingViewState();
}

class _OnboardingStep {
  const _OnboardingStep({required this.id, required this.titleKey, required this.descriptionKey});

  final String id;
  final String titleKey;
  final String descriptionKey;
}

class _WoxOnboardingViewState extends State<WoxOnboardingView> {
  final launcherController = Get.find<WoxLauncherController>();
  final settingController = Get.find<WoxSettingController>();
  int activeStepIndex = 0;
  bool isGlanceLoading = false;
  bool isGlanceLoadFailed = false;
  bool hasRequestedGlanceLoad = false;
  bool isPermissionLoading = false;
  bool? accessibilityPassed;
  late final Future<List<WoxLang>> availableLanguagesFuture = WoxApi.instance.getAllLanguages(const UuidV4().generate());

  late final List<_OnboardingStep> steps = [
    const _OnboardingStep(id: 'welcome', titleKey: 'onboarding_welcome_title', descriptionKey: 'onboarding_welcome_description'),
    // Permission setup is macOS-only because Windows and Linux do not need a
    // first-run system permission page for the Wox features introduced here.
    // Keeping the step out of the list also keeps numbering and progress honest.
    if (Platform.isMacOS) const _OnboardingStep(id: 'permissions', titleKey: 'onboarding_permissions_title', descriptionKey: 'onboarding_permissions_description'),
    const _OnboardingStep(id: 'mainHotkey', titleKey: 'onboarding_main_hotkey_title', descriptionKey: 'onboarding_main_hotkey_description'),
    const _OnboardingStep(id: 'selectionHotkey', titleKey: 'onboarding_selection_hotkey_title', descriptionKey: 'onboarding_selection_hotkey_description'),
    // Feature change: width and density tuning made onboarding feel longer than
    // the core first-run flow. Removing that dedicated page keeps the tour
    // focused while the shared Wox preview still demonstrates the real layout.
    const _OnboardingStep(id: 'glance', titleKey: 'onboarding_glance_title', descriptionKey: 'onboarding_glance_description'),
    const _OnboardingStep(id: 'actionPanel', titleKey: 'onboarding_action_panel_title', descriptionKey: 'onboarding_action_panel_description'),
    // Feature change: the previous Advanced Queries page bundled three
    // unrelated workflows. Splitting them into dedicated steps lets each query
    // feature get its own explanation and animated demo.
    const _OnboardingStep(id: 'queryHotkeys', titleKey: 'onboarding_query_hotkeys_title', descriptionKey: 'onboarding_query_hotkeys_body'),
    const _OnboardingStep(id: 'queryShortcuts', titleKey: 'onboarding_query_shortcuts_title', descriptionKey: 'onboarding_query_shortcuts_body'),
    const _OnboardingStep(id: 'trayQueries', titleKey: 'onboarding_tray_queries_title', descriptionKey: 'onboarding_tray_queries_body'),
    // Feature change: plugin and theme installation are common first-run
    // workflows, so they are taught as standalone sections instead of being
    // hidden behind generic query examples.
    const _OnboardingStep(id: 'wpmInstall', titleKey: 'onboarding_wpm_install_title', descriptionKey: 'onboarding_wpm_install_body'),
    const _OnboardingStep(id: 'themeInstall', titleKey: 'onboarding_theme_install_title', descriptionKey: 'onboarding_theme_install_body'),
    const _OnboardingStep(id: 'finish', titleKey: 'onboarding_finish_title', descriptionKey: 'onboarding_finish_description'),
  ];

  String tr(String key) => settingController.tr(key);

  _OnboardingStep get activeStep => steps[activeStepIndex];

  bool get isLastStep => activeStepIndex == steps.length - 1;

  Color get activeAccent => _stepAccentColor(activeStep.id);

  WoxImage? _resolveGlanceIcon(MetadataGlance glance, GlanceItem? preview) {
    if (preview != null && preview.icon.imageData.isNotEmpty) {
      // Feature change: the runtime Glance API can return a state-specific icon
      // such as AC power instead of the static metadata glyph, so onboarding uses
      // that live icon first and only falls back when the API has no snapshot yet.
      return preview.icon;
    }

    final metadataIcon = WoxImage.parse(glance.icon);
    return metadataIcon?.imageData.isNotEmpty == true ? metadataIcon : null;
  }

  Color _stepAccentColor(String stepId) {
    // Feature refinement: accents are tied to feature identity instead of list
    // position. Removing the window/interface section should not shift the
    // colors users already saw for Glance, Action Panel, or later sections.
    return switch (stepId) {
      'welcome' => const Color(0xFF2DD4BF),
      'permissions' => const Color(0xFFF97316),
      'mainHotkey' => const Color(0xFFF97316),
      'selectionHotkey' => const Color(0xFF60A5FA),
      'glance' => const Color(0xFFFACC15),
      'actionPanel' => const Color(0xFF34D399),
      'queryHotkeys' => const Color(0xFFF43F5E),
      'queryShortcuts' => const Color(0xFFA78BFA),
      'trayQueries' => const Color(0xFF22C55E),
      'wpmInstall' => const Color(0xFF38BDF8),
      'themeInstall' => const Color(0xFFE879F9),
      'finish' => const Color(0xFF2DD4BF),
      _ => const Color(0xFF22C55E),
    };
  }

  @override
  void initState() {
    super.initState();
    _handleStepEntered();
  }

  void _goToStep(int index) {
    if (index < 0 || index >= steps.length) {
      return;
    }

    setState(() {
      activeStepIndex = index;
    });
    _handleStepEntered();
  }

  void _handleStepEntered() {
    if (activeStep.id == 'glance' && !hasRequestedGlanceLoad) {
      hasRequestedGlanceLoad = true;
      unawaited(_loadGlanceChoices());
    }
    if (activeStep.id == 'permissions' && Platform.isMacOS && accessibilityPassed == null && !isPermissionLoading) {
      unawaited(_loadPermissionStatus());
    }
  }

  Future<void> _loadPermissionStatus() async {
    setState(() {
      isPermissionLoading = true;
    });
    try {
      final results = await WoxApi.instance.doctorCheck(const UuidV4().generate());
      final accessibility = results.where((item) => item.type.toLowerCase() == 'accessibility').toList();
      if (!mounted) return;
      setState(() {
        accessibilityPassed = accessibility.isEmpty ? true : accessibility.first.passed;
      });
    } catch (_) {
      if (!mounted) return;
      setState(() {
        accessibilityPassed = null;
      });
    } finally {
      if (mounted) {
        setState(() {
          isPermissionLoading = false;
        });
      }
    }
  }

  Future<void> _loadGlanceChoices() async {
    setState(() {
      isGlanceLoading = true;
      isGlanceLoadFailed = false;
    });

    try {
      final traceId = const UuidV4().generate();
      // Plugin and Glance metadata can still be loading during first install.
      // The onboarding page explicitly waits here and renders loading/empty
      // states so users never see a blank selector.
      await settingController.reloadPlugins(traceId);
      settingController.settingGlancePreviewItems.clear();
      await settingController.refreshSettingGlancePreviews(traceId);
    } catch (_) {
      if (!mounted) return;
      setState(() {
        isGlanceLoadFailed = true;
      });
    } finally {
      if (mounted) {
        setState(() {
          isGlanceLoading = false;
        });
      }
    }
  }

  Future<void> _finish({required bool markFinished}) async {
    await launcherController.finishOnboarding(const UuidV4().generate(), markFinished: markFinished);
  }

  @override
  Widget build(BuildContext context) {
    return Obx(() {
      final background = getThemeBackgroundColor();
      final accent = activeAccent;
      return Material(
        // Onboarding uses InkWell, buttons, and dropdowns outside the normal
        // launcher/setting subtree. Providing a local Material ancestor prevents
        // Flutter's debug error surface and gives text the expected app style.
        key: const ValueKey('onboarding-view'),
        color: background,
        child: DefaultTextStyle(
          style: TextStyle(color: getThemeTextColor(), fontSize: 13),
          child: Stack(
            children: [
              Positioned.fill(child: _OnboardingBackdrop(accent: accent)),
              Column(
                children: [
                  Expanded(child: Row(children: [_buildSidebar(accent), Expanded(child: _buildStepBody(accent))])),
                  _buildFooter(accent),
                ],
              ),
            ],
          ),
        ),
      );
    });
  }

  Widget _buildSidebar(Color accent) {
    return Container(
      width: _onboardingSidebarWidth,
      padding: const EdgeInsets.fromLTRB(24, 30, 18, 22),
      decoration: BoxDecoration(border: Border(right: BorderSide(color: getThemeSubTextColor().withValues(alpha: 0.18)))),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(
            children: [
              Container(
                width: 28,
                height: 28,
                decoration: BoxDecoration(borderRadius: BorderRadius.circular(6), boxShadow: [BoxShadow(color: accent.withValues(alpha: 0.18), blurRadius: 14)]),
                clipBehavior: Clip.antiAlias,
                child: WoxImageView(woxImage: WoxImage.newBase64(WOX_ICON), width: 28, height: 28),
              ),
              const SizedBox(width: 11),
              // The onboarding rail now follows the settings page's text
              // hierarchy instead of using oversized promotional weights, so
              // the guide feels like part of the same management surface.
              Text(tr('onboarding_title'), style: TextStyle(color: getThemeTextColor(), fontSize: 22, fontWeight: FontWeight.w700)),
            ],
          ),
          const SizedBox(height: 10),
          Text(tr('onboarding_subtitle'), style: TextStyle(color: getThemeSubTextColor(), fontSize: 13, height: 1.45)),
          const SizedBox(height: 24),
          Expanded(
            child: ListView.builder(
              itemCount: steps.length,
              itemBuilder: (context, index) {
                final step = steps[index];
                final isActive = index == activeStepIndex;
                final isDone = index < activeStepIndex;
                final nodeColor = isActive ? accent : (isDone ? getThemeActiveBackgroundColor() : getThemeSubTextColor().withValues(alpha: 0.42));
                const rowHeight = 58.0;
                const nodeCenterY = rowHeight / 2;
                final nodeSize = isActive ? 24.0 : 18.0;
                final nodeRadius = nodeSize / 2;
                return InkWell(
                  key: ValueKey('onboarding-step-${step.id}'),
                  borderRadius: BorderRadius.circular(10),
                  hoverColor: getThemeTextColor().withValues(alpha: 0.04),
                  splashColor: Colors.transparent,
                  onTap: () => _goToStep(index),
                  child: SizedBox(
                    // A fixed row height lets the step node and label share the
                    // same visual center. The previous top-aligned Row made the
                    // active label drift below the numbered circle.
                    height: rowHeight,
                    child: Row(
                      crossAxisAlignment: CrossAxisAlignment.center,
                      children: [
                        SizedBox(
                          width: 30,
                          height: rowHeight,
                          child: Stack(
                            clipBehavior: Clip.none,
                            children: [
                              if (index != 0)
                                Positioned(
                                  left: 14.5,
                                  top: 0,
                                  height: nodeCenterY - nodeRadius - 3,
                                  child: Container(width: 1, color: getThemeSubTextColor().withValues(alpha: 0.16)),
                                ),
                              if (index != steps.length - 1)
                                Positioned(
                                  left: 14.5,
                                  top: nodeCenterY + nodeRadius + 3,
                                  bottom: 0,
                                  child: Container(width: 1, color: getThemeSubTextColor().withValues(alpha: 0.16)),
                                ),
                              Positioned(
                                left: (30 - nodeSize) / 2,
                                top: nodeCenterY - nodeRadius,
                                child: AnimatedContainer(
                                  duration: const Duration(milliseconds: 220),
                                  width: nodeSize,
                                  height: nodeSize,
                                  alignment: Alignment.center,
                                  decoration: BoxDecoration(
                                    color: isActive ? nodeColor.withValues(alpha: 0.18) : nodeColor.withValues(alpha: 0.12),
                                    shape: BoxShape.circle,
                                    border: Border.all(color: nodeColor.withValues(alpha: isActive ? 0.82 : 0.38)),
                                    boxShadow: isActive ? [BoxShadow(color: nodeColor.withValues(alpha: 0.28), blurRadius: 18, spreadRadius: 1)] : const [],
                                  ),
                                  child:
                                      isDone
                                          ? Icon(Icons.check_rounded, size: 12, color: nodeColor)
                                          : Text('${index + 1}', style: TextStyle(color: nodeColor, fontSize: 10, fontWeight: FontWeight.w800)),
                                ),
                              ),
                            ],
                          ),
                        ),
                        const SizedBox(width: 10),
                        Expanded(
                          child: AnimatedContainer(
                            duration: const Duration(milliseconds: 220),
                            height: 38,
                            alignment: Alignment.centerLeft,
                            padding: const EdgeInsets.symmetric(horizontal: 10),
                            decoration: BoxDecoration(
                              color: isActive ? accent.withValues(alpha: 0.11) : Colors.transparent,
                              border: Border.all(color: isActive ? accent.withValues(alpha: 0.28) : Colors.transparent),
                              borderRadius: BorderRadius.circular(8),
                            ),
                            child: Text(
                              tr(step.titleKey),
                              maxLines: 1,
                              overflow: TextOverflow.ellipsis,
                              style: TextStyle(
                                color: isActive ? getThemeTextColor() : getThemeTextColor().withValues(alpha: 0.78),
                                fontSize: 13,
                                fontWeight: isActive ? FontWeight.w600 : FontWeight.normal,
                              ),
                            ),
                          ),
                        ),
                      ],
                    ),
                  ),
                );
              },
            ),
          ),
        ],
      ),
    );
  }

  Widget _buildStepBody(Color accent) {
    Widget buildStepIntro() {
      return Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text(tr(activeStep.titleKey), style: TextStyle(color: getThemeTextColor(), fontSize: 32, fontWeight: FontWeight.w800, height: 1.05)),
          const SizedBox(height: 12),
          ConstrainedBox(
            constraints: const BoxConstraints(maxWidth: 640),
            child: Text(tr(activeStep.descriptionKey), style: TextStyle(color: getThemeSubTextColor(), fontSize: 15, height: 1.52)),
          ),
          const SizedBox(height: 24),
          Flexible(child: SingleChildScrollView(child: Align(alignment: Alignment.topLeft, child: _buildStepContent()))),
        ],
      );
    }

    Widget buildMediaSlot() {
      return _OnboardingMediaSlot(
        stepId: activeStep.id,
        tr: tr,
        accent: accent,
        mainHotkey: settingController.woxSetting.value.mainHotkey,
        selectionHotkey: settingController.woxSetting.value.selectionHotkey,
        glanceEnabled: settingController.woxSetting.value.enableGlance,
        glanceLabel: _currentGlanceLabel(),
        glanceValue: _currentGlanceValue(),
        glanceIcon: _currentGlanceIcon(),
      );
    }

    return Padding(
      padding: const EdgeInsets.fromLTRB(38, 34, 38, 28),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Expanded(
            child: AnimatedSwitcher(
              duration: const Duration(milliseconds: 280),
              switchInCurve: Curves.easeOutCubic,
              switchOutCurve: Curves.easeInCubic,
              transitionBuilder: (child, animation) {
                final offset = Tween<Offset>(begin: const Offset(0.025, 0), end: Offset.zero).animate(animation);
                return FadeTransition(opacity: animation, child: SlideTransition(position: offset, child: child));
              },
              child: Column(
                key: ValueKey('onboarding-stage-${activeStep.id}'),
                crossAxisAlignment: CrossAxisAlignment.stretch,
                children: [
                  // Feature refinement: every onboarding section now follows
                  // the main-hotkey 4:6 structure. Keeping the demo area
                  // consistently taller gives animated examples enough room
                  // without changing the left progress rail or footer.
                  Flexible(flex: 4, child: buildStepIntro()),
                  const SizedBox(height: 10),
                  Flexible(flex: 6, child: buildMediaSlot()),
                ],
              ),
            ),
          ),
        ],
      ),
    );
  }

  Widget _buildStepContent() {
    switch (activeStep.id) {
      case 'permissions':
        return _buildPermissionsStep();
      case 'mainHotkey':
        return _buildHotkeyStep(settingKey: 'MainHotkey', currentValue: settingController.woxSetting.value.mainHotkey, tipKey: 'onboarding_main_hotkey_tip');
      case 'selectionHotkey':
        return _buildHotkeyStep(settingKey: 'SelectionHotkey', currentValue: settingController.woxSetting.value.selectionHotkey, tipKey: 'onboarding_selection_hotkey_tip');
      case 'glance':
        return _buildGlanceStep();
      case 'actionPanel':
        return _buildFeatureStep(
          key: const ValueKey('onboarding-action-panel-page'),
          titleKey: 'onboarding_action_panel_card_title',
          bodyKey: 'onboarding_action_panel_card_body',
          badge: Platform.isMacOS ? 'Cmd+J' : 'Alt+J',
        );
      case 'queryHotkeys':
        return _buildFeatureStep(
          key: const ValueKey('onboarding-query-hotkeys-page'),
          titleKey: 'onboarding_query_hotkeys_title',
          bodyKey: 'onboarding_query_hotkeys_body',
          badge: tr('ui_query_hotkeys'),
        );
      case 'queryShortcuts':
        return _buildFeatureStep(
          key: const ValueKey('onboarding-query-shortcuts-page'),
          titleKey: 'onboarding_query_shortcuts_title',
          bodyKey: 'onboarding_query_shortcuts_body',
          badge: tr('ui_query_shortcuts'),
        );
      case 'trayQueries':
        return _buildFeatureStep(
          key: const ValueKey('onboarding-tray-queries-page'),
          titleKey: 'onboarding_tray_queries_title',
          bodyKey: 'onboarding_tray_queries_body',
          badge: tr('ui_tray_queries'),
        );
      case 'wpmInstall':
        return _buildFeatureStep(
          key: const ValueKey('onboarding-wpm-install-page'),
          titleKey: 'onboarding_wpm_install_title',
          bodyKey: 'onboarding_wpm_install_body',
          badge: 'wpm install',
        );
      case 'themeInstall':
        return _buildFeatureStep(
          key: const ValueKey('onboarding-theme-install-page'),
          titleKey: 'onboarding_theme_install_title',
          bodyKey: 'onboarding_theme_install_body',
          badge: tr('plugin_theme_install_theme'),
        );
      case 'finish':
        return _buildFeatureStep(
          key: const ValueKey('onboarding-finish-page'),
          titleKey: 'onboarding_finish_card_title',
          bodyKey: 'onboarding_finish_card_body',
          badge: tr('onboarding_finish_badge'),
        );
      default:
        return _buildWelcomeStep();
    }
  }

  Widget _buildWelcomeStep() {
    return _SettingsPanel(
      key: const ValueKey('onboarding-welcome-page'),
      children: [
        Text(tr('onboarding_welcome_card_body'), style: TextStyle(color: getThemeSubTextColor(), fontSize: 14, height: 1.5)),
        const SizedBox(height: 20),
        Container(height: 1, color: getThemeSubTextColor().withValues(alpha: 0.14)),
        const SizedBox(height: 18),
        Row(
          crossAxisAlignment: CrossAxisAlignment.center,
          children: [
            Text(tr('ui_lang'), style: TextStyle(color: getThemeTextColor(), fontSize: 14, fontWeight: FontWeight.w600)),
            const SizedBox(width: 18),
            Expanded(
              child: Align(
                alignment: Alignment.centerRight,
                child: ConstrainedBox(
                  constraints: const BoxConstraints(maxWidth: 320),
                  child: FutureBuilder<List<WoxLang>>(
                    future: availableLanguagesFuture,
                    builder: (context, snapshot) {
                      if (snapshot.connectionState != ConnectionState.done) {
                        return Text(tr('onboarding_loading'), textAlign: TextAlign.right, style: TextStyle(color: getThemeSubTextColor(), fontSize: 13));
                      }

                      final languages = snapshot.data ?? const <WoxLang>[];
                      if (languages.isEmpty) {
                        return Text(settingController.woxSetting.value.langCode, textAlign: TextAlign.right, style: TextStyle(color: getThemeSubTextColor(), fontSize: 13));
                      }

                      // Feature refinement: language selection is now an inline
                      // setting row instead of a stacked block. The dropdown
                      // keeps a bounded width so the welcome copy remains the
                      // dominant content while the control still uses the same
                      // updateLang path as settings.
                      return WoxDropdownButton<String>(
                        key: const ValueKey('onboarding-language-dropdown'),
                        items: languages.map((language) => WoxDropdownItem(value: language.code, label: language.name)).toList(),
                        value: settingController.woxSetting.value.langCode,
                        onChanged: (value) {
                          if (value != null) {
                            unawaited(settingController.updateLang(value));
                          }
                        },
                        isExpanded: true,
                      );
                    },
                  ),
                ),
              ),
            ),
          ],
        ),
      ],
    );
  }

  Widget _buildPermissionsStep() {
    if (!Platform.isMacOS) {
      return _InfoPanel(
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
        _InfoPanel(title: tr('onboarding_permission_accessibility_title'), body: tr('onboarding_permission_accessibility_body'), badge: statusText),
        const SizedBox(height: 14),
        _InfoPanel(title: tr('onboarding_permission_disk_title'), body: tr('onboarding_permission_disk_body'), badge: tr('onboarding_permission_optional')),
        const SizedBox(height: 20),
        Wrap(
          spacing: 12,
          runSpacing: 10,
          children: [
            WoxButton.secondary(text: tr('onboarding_permission_open_accessibility'), onPressed: () => WoxApi.instance.openAccessibilityPermission(const UuidV4().generate())),
            WoxButton.secondary(text: tr('onboarding_permission_open_privacy'), onPressed: () => WoxApi.instance.openPrivacyPermission(const UuidV4().generate())),
          ],
        ),
      ],
    );
  }

  Widget _buildHotkeyStep({required String settingKey, required String currentValue, required String tipKey}) {
    return _SettingsPanel(
      children: [
        Text(tr(tipKey), style: TextStyle(color: getThemeSubTextColor(), fontSize: 14, height: 1.45)),
        const SizedBox(height: 26),
        Align(
          alignment: Alignment.centerLeft,
          child: WoxHotkeyRecorder(
            hotkey: WoxHotkey.parseHotkeyFromString(currentValue),
            tipPosition: WoxHotkeyRecorderTipPosition.right,
            onHotKeyRecorded: (hotkey) {
              // Hotkey setup is saved immediately so leaving the guide after
              // this step preserves the user's chosen launch behavior.
              settingController.updateConfig(settingKey, hotkey);
            },
          ),
        ),
      ],
    );
  }

  Widget _buildGlanceStep() {
    final isEnabled = settingController.woxSetting.value.enableGlance;
    final children = <Widget>[
      Row(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text(tr('ui_glance_enable'), style: TextStyle(color: getThemeTextColor(), fontSize: 14, fontWeight: FontWeight.w700)),
                const SizedBox(height: 7),
                Text(tr('ui_glance_enable_tips'), style: TextStyle(color: getThemeSubTextColor(), fontSize: 13, height: 1.45)),
              ],
            ),
          ),
          const SizedBox(width: 18),
          WoxSwitch(
            key: const ValueKey('onboarding-glance-enable-switch'),
            value: isEnabled,
            onChanged: (value) {
              // Glance is optional during onboarding. Save the enable flag
              // independently so users can leave this step without selecting
              // a provider when they do not want the accessory value.
              settingController.updateConfig('EnableGlance', value.toString());
            },
          ),
        ],
      ),
    ];

    if (!isEnabled) {
      return _SettingsPanel(key: const ValueKey('onboarding-glance-disabled'), children: children);
    }

    if (isGlanceLoading) {
      children.addAll([
        const SizedBox(height: 22),
        _InfoPanel(
          key: const ValueKey('onboarding-glance-loading'),
          title: tr('onboarding_glance_loading_title'),
          body: tr('onboarding_glance_loading_body'),
          badge: tr('onboarding_loading'),
        ),
      ]);
      return _SettingsPanel(children: children);
    }

    final items = _buildGlanceDropdownItems();
    if (isGlanceLoadFailed || items.isEmpty) {
      children.addAll([
        const SizedBox(height: 22),
        _InfoPanel(
          key: const ValueKey('onboarding-glance-empty'),
          title: tr('onboarding_glance_empty_title'),
          body: tr('onboarding_glance_empty_body'),
          badge: tr('onboarding_can_skip'),
        ),
      ]);
      return _SettingsPanel(children: children);
    }

    final currentRef = settingController.woxSetting.value.primaryGlance;
    final currentValue = items.any((item) => item.value == currentRef.key) ? currentRef.key : items.first.value;
    return _SettingsPanel(
      key: const ValueKey('onboarding-glance-picker'),
      children: [
        ...children,
        const SizedBox(height: 24),
        Row(
          crossAxisAlignment: CrossAxisAlignment.center,
          children: [
            Text(tr('onboarding_glance_picker_label'), style: TextStyle(color: getThemeTextColor(), fontSize: 14, fontWeight: FontWeight.w600)),
            const SizedBox(width: 18),
            Expanded(
              child: Align(
                alignment: Alignment.centerRight,
                child: ConstrainedBox(
                  constraints: const BoxConstraints(maxWidth: 420),
                  // Feature refinement: the primary Glance selector now uses
                  // the same inline setting-row layout as the welcome language
                  // picker. The previous stacked label made the card taller
                  // than necessary, while a bounded dropdown keeps the setting
                  // compact without letting long provider names dominate.
                  child: WoxDropdownButton<String>(
                    value: currentValue,
                    items: items,
                    onChanged: (value) {
                      if (value == null) return;
                      final ref = _parseGlanceKey(value);
                      settingController.updateConfig('EnableGlance', 'true');
                      settingController.updateConfig('PrimaryGlance', jsonEncode(ref.toJson()));
                    },
                    isExpanded: true,
                  ),
                ),
              ),
            ),
          ],
        ),
      ],
    );
  }

  List<WoxDropdownItem<String>> _buildGlanceDropdownItems() {
    final items = <WoxDropdownItem<String>>[];
    for (final plugin in settingController.installedPlugins) {
      for (final glance in plugin.glances) {
        final key = GlanceRef(pluginId: plugin.id, glanceId: glance.id).key;
        final preview = settingController.settingGlancePreviewItems[key];
        final icon = _resolveGlanceIcon(glance, preview);
        items.add(
          WoxDropdownItem(
            value: key,
            label: glance.name,
            subtitle: plugin.name,
            leading: icon == null ? null : WoxImageView(woxImage: icon, width: 18, height: 18, svgColor: getThemeTextColor()),
            trailing: Text(preview?.text ?? '', maxLines: 1, overflow: TextOverflow.ellipsis, style: TextStyle(color: getThemeSubTextColor(), fontSize: 12)),
          ),
        );
      }
    }
    return items;
  }

  String _currentGlanceLabel() {
    final currentRef = settingController.woxSetting.value.primaryGlance;
    for (final plugin in settingController.installedPlugins) {
      for (final glance in plugin.glances) {
        if (GlanceRef(pluginId: plugin.id, glanceId: glance.id).key == currentRef.key) {
          return glance.name;
        }
      }
    }
    return tr('onboarding_glance_sample_time');
  }

  String _currentGlanceValue() {
    final currentRef = settingController.woxSetting.value.primaryGlance;
    final preview = settingController.settingGlancePreviewItems[currentRef.key]?.text;
    if (preview != null && preview.isNotEmpty) {
      return preview;
    }
    return tr('onboarding_glance_sample_value');
  }

  WoxImage? _currentGlanceIcon() {
    final currentRef = settingController.woxSetting.value.primaryGlance;
    final preview = settingController.settingGlancePreviewItems[currentRef.key];
    for (final plugin in settingController.installedPlugins) {
      for (final glance in plugin.glances) {
        if (GlanceRef(pluginId: plugin.id, glanceId: glance.id).key == currentRef.key) {
          return _resolveGlanceIcon(glance, preview);
        }
      }
    }
    return preview != null && preview.icon.imageData.isNotEmpty ? preview.icon : null;
  }

  GlanceRef _parseGlanceKey(String key) {
    final parts = key.split('\x00');
    if (parts.length != 2) {
      return GlanceRef.empty();
    }
    return GlanceRef(pluginId: parts[0], glanceId: parts[1]);
  }

  Widget _buildFeatureStep({required Key key, required String titleKey, required String bodyKey, required String badge}) {
    return _InfoPanel(key: key, title: tr(titleKey), body: tr(bodyKey), badge: badge);
  }

  Widget _buildFooter(Color accent) {
    return Container(
      height: _onboardingFooterHeight,
      padding: const EdgeInsets.symmetric(horizontal: 28),
      decoration: BoxDecoration(color: getThemeBackgroundColor().withValues(alpha: 0.52), border: Border(top: BorderSide(color: getThemeSubTextColor().withValues(alpha: 0.16)))),
      child: Row(
        children: [
          WoxButton.text(key: const ValueKey('onboarding-skip-button'), text: tr('onboarding_skip'), onPressed: () => _finish(markFinished: true)),
          // Feature refinement: progress is represented by the step rail only.
          // Removing the footer progress bar keeps the action area focused on
          // navigation and avoids showing two competing progress indicators.
          const Spacer(),
          WoxButton.secondary(
            key: const ValueKey('onboarding-back-button'),
            text: tr('onboarding_back'),
            onPressed: activeStepIndex == 0 ? null : () => _goToStep(activeStepIndex - 1),
          ),
          const SizedBox(width: 12),
          WoxButton.primary(
            key: ValueKey(isLastStep ? 'onboarding-finish-button' : 'onboarding-next-button'),
            text: tr(isLastStep ? 'onboarding_finish' : 'onboarding_next'),
            onPressed: isLastStep ? () => _finish(markFinished: true) : () => _goToStep(activeStepIndex + 1),
          ),
        ],
      ),
    );
  }
}

class _OnboardingBackdrop extends StatelessWidget {
  const _OnboardingBackdrop({required this.accent});

  final Color accent;

  @override
  Widget build(BuildContext context) {
    return CustomPaint(
      // The onboarding background now carries the product-tour feeling instead
      // of reusing the settings page's flat surface. Keeping it as a painter
      // avoids extra layout widgets and makes the visual layer independent of
      // the step content.
      painter: _OnboardingBackdropPainter(accent: accent, textColor: getThemeTextColor(), backgroundColor: getThemeBackgroundColor()),
    );
  }
}

class _OnboardingBackdropPainter extends CustomPainter {
  const _OnboardingBackdropPainter({required this.accent, required this.textColor, required this.backgroundColor});

  final Color accent;
  final Color textColor;
  final Color backgroundColor;

  @override
  void paint(Canvas canvas, Size size) {
    final base = Paint()..color = backgroundColor;
    canvas.drawRect(Offset.zero & size, base);

    final gridPaint =
        Paint()
          ..color = textColor.withValues(alpha: 0.022)
          ..strokeWidth = 1;
    for (double x = _onboardingSidebarWidth; x < size.width; x += 52) {
      canvas.drawLine(Offset(x, 0), Offset(x, size.height - _onboardingFooterHeight), gridPaint);
    }
    for (double y = 42; y < size.height - _onboardingFooterHeight; y += 52) {
      canvas.drawLine(Offset(_onboardingSidebarWidth, y), Offset(size.width, y), gridPaint);
    }

    final sweepPaint =
        Paint()
          ..shader = LinearGradient(
            begin: Alignment.topLeft,
            end: Alignment.bottomRight,
            colors: [accent.withValues(alpha: 0.12), textColor.withValues(alpha: 0.014), Colors.transparent],
            stops: const [0, 0.45, 1],
          ).createShader(Rect.fromLTWH(_onboardingSidebarWidth, 0, size.width - _onboardingSidebarWidth, size.height - _onboardingFooterHeight));
    final path =
        Path()
          ..moveTo(_onboardingSidebarWidth, 0)
          ..lineTo(size.width, 0)
          ..lineTo(size.width, 230)
          ..lineTo(_onboardingSidebarWidth, 520)
          ..close();
    canvas.drawPath(path, sweepPaint);

    // Feature refinement: the backdrop now stays behind the task content. The
    // earlier grid and accent shards competed with form controls, so the same
    // visual language is kept at a lower opacity and aligned to shared layout
    // constants.
    final shardPaint = Paint()..color = accent.withValues(alpha: 0.035);
    canvas.drawRRect(RRect.fromRectAndRadius(Rect.fromLTWH(size.width - 500, 92, 420, 120), const Radius.circular(18)), shardPaint);
    canvas.drawRRect(RRect.fromRectAndRadius(Rect.fromLTWH(356, size.height - 238, 360, 86), const Radius.circular(18)), shardPaint);
  }

  @override
  bool shouldRepaint(covariant _OnboardingBackdropPainter oldDelegate) {
    return oldDelegate.accent != accent || oldDelegate.textColor != textColor || oldDelegate.backgroundColor != backgroundColor;
  }
}

class _SettingsPanel extends StatelessWidget {
  const _SettingsPanel({super.key, required this.children});

  final List<Widget> children;

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.all(22),
      decoration: BoxDecoration(
        color: getThemeTextColor().withValues(alpha: 0.04),
        border: Border.all(color: getThemeSubTextColor().withValues(alpha: 0.18)),
        borderRadius: BorderRadius.circular(8),
      ),
      child: Column(crossAxisAlignment: CrossAxisAlignment.start, mainAxisSize: MainAxisSize.min, children: children),
    );
  }
}

class _InfoPanel extends StatelessWidget {
  const _InfoPanel({super.key, required this.title, required this.body, required this.badge});

  final String title;
  final String body;
  final String badge;

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.all(22),
      decoration: BoxDecoration(
        color: getThemeTextColor().withValues(alpha: 0.04),
        border: Border.all(color: getThemeSubTextColor().withValues(alpha: 0.18)),
        borderRadius: BorderRadius.circular(8),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        mainAxisSize: MainAxisSize.min,
        children: [
          Row(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Expanded(child: Text(title, style: TextStyle(color: getThemeTextColor(), fontSize: 17, fontWeight: FontWeight.w600))),
              const SizedBox(width: 14),
              Container(
                padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 5),
                decoration: BoxDecoration(color: getThemeActiveBackgroundColor().withValues(alpha: 0.16), borderRadius: BorderRadius.circular(16)),
                child: Text(badge, style: TextStyle(color: getThemeActiveBackgroundColor(), fontSize: 12, fontWeight: FontWeight.w600)),
              ),
            ],
          ),
          const SizedBox(height: 12),
          Text(body, style: TextStyle(color: getThemeSubTextColor(), fontSize: 14, height: 1.5)),
        ],
      ),
    );
  }
}

class _OnboardingMediaSlot extends StatelessWidget {
  const _OnboardingMediaSlot({
    required this.stepId,
    required this.tr,
    required this.accent,
    required this.mainHotkey,
    required this.selectionHotkey,
    required this.glanceEnabled,
    required this.glanceLabel,
    required this.glanceValue,
    required this.glanceIcon,
  });

  final String stepId;
  final String Function(String key) tr;
  final Color accent;
  final String mainHotkey;
  final String selectionHotkey;
  final bool glanceEnabled;
  final String glanceLabel;
  final String glanceValue;
  final WoxImage? glanceIcon;

  @override
  Widget build(BuildContext context) {
    // Feature refinement: all sections now use the same taller demo cap as the
    // main-hotkey flow. This keeps later animated examples consistent instead
    // of shrinking them back to the old compact preview height.
    const maxPreviewHeight = 520.0;

    return ConstrainedBox(
      constraints: BoxConstraints(maxHeight: maxPreviewHeight),
      child: ClipRRect(
        borderRadius: BorderRadius.circular(8),
        // The right-side guide surface is intentionally Flutter-native now.
        // It reacts to onboarding state immediately, while final GIF assets
        // would lag behind settings such as Glance enablement and selection.
        child: _OnboardingMediaCard(
          stepId: stepId,
          tr: tr,
          accent: accent,
          mainHotkey: mainHotkey,
          selectionHotkey: selectionHotkey,
          glanceEnabled: glanceEnabled,
          glanceLabel: glanceLabel,
          glanceValue: glanceValue,
          glanceIcon: glanceIcon,
        ),
      ),
    );
  }
}

class _OnboardingMediaCard extends StatelessWidget {
  const _OnboardingMediaCard({
    required this.stepId,
    required this.tr,
    required this.accent,
    required this.mainHotkey,
    required this.selectionHotkey,
    required this.glanceEnabled,
    required this.glanceLabel,
    required this.glanceValue,
    required this.glanceIcon,
  });

  final String stepId;
  final String Function(String key) tr;
  final Color accent;
  final String mainHotkey;
  final String selectionHotkey;
  final bool glanceEnabled;
  final String glanceLabel;
  final String glanceValue;
  final WoxImage? glanceIcon;

  @override
  Widget build(BuildContext context) {
    return TweenAnimationBuilder<double>(
      key: ValueKey('onboarding-media-$stepId'),
      duration: const Duration(milliseconds: 300),
      curve: Curves.easeOutCubic,
      tween: Tween(begin: 0, end: 1),
      builder: (context, value, child) {
        // Animation optimization: the previous longer slide made each step feel
        // heavy. A shorter travel distance keeps the card responsive without
        // adding bounce to the settings-style onboarding surface.
        return Opacity(opacity: value, child: Transform.translate(offset: Offset(0, 8 * (1 - value)), child: child));
      },
      // Feature refinement: every step already has its title and explanation
      // above the media area. The previous generic chrome duplicated labels,
      // dots, icons, padding, and borders, so this focused surface keeps the
      // examples compact and consistent with the Action Panel step.
      child: _buildFocusedPreviewCard(),
    );
  }

  Widget _buildFocusedPreviewCard() {
    return Container(
      padding: const EdgeInsets.all(8),
      decoration: BoxDecoration(color: getThemeTextColor().withValues(alpha: 0.028), borderRadius: BorderRadius.circular(8)),
      child: _buildPreviewSwitcher(),
    );
  }

  Widget _buildPreviewSwitcher() {
    return AnimatedSwitcher(
      duration: const Duration(milliseconds: 180),
      reverseDuration: const Duration(milliseconds: 120),
      switchInCurve: Curves.easeOutCubic,
      switchOutCurve: Curves.easeInQuad,
      transitionBuilder: (child, animation) {
        // Animation optimization: preview content still crossfades between
        // steps, but the scale range is now nearly flat so text and controls
        // feel snappier instead of popping.
        return FadeTransition(opacity: animation, child: ScaleTransition(scale: Tween<double>(begin: 0.995, end: 1).animate(animation), child: child));
      },
      child: KeyedSubtree(
        key: ValueKey('preview-$stepId-$mainHotkey-$selectionHotkey-$glanceEnabled-$glanceLabel-$glanceValue-${glanceIcon?.imageType}-${glanceIcon?.imageData}'),
        child: _buildPreviewContent(),
      ),
    );
  }

  Widget? _buildGlanceAccessory() {
    if (!glanceEnabled) {
      return null;
    }

    // Feature refinement: Glance is a learned capability, so the shared Wox
    // preview only shows the live query accessory on the Glance step and later
    // sections after the user enables it. Earlier steps stay clean and do not
    // reveal the feature before onboarding explains it.
    return _GlanceAccessory(
      key: ValueKey('mini-glance-pill-$glanceLabel-$glanceValue-${glanceIcon?.imageType}-${glanceIcon?.imageData}'),
      label: glanceLabel,
      value: glanceValue,
      icon: glanceIcon,
    );
  }

  Widget _buildPreviewContent() {
    // Feature refinement: every section preview now uses the same mini Wox
    // shell. Earlier custom sketches made Glance, hotkeys, and advanced
    // queries look like separate products, so the data varies while the Wox
    // window structure stays shared and launcher-like.
    switch (stepId) {
      case 'permissions':
        return _MiniWoxWindow(
          accent: accent,
          query: 'permissions',
          results: [
            _MiniResultEntry(
              title: tr('onboarding_permission_accessibility_title'),
              subtitle: tr('onboarding_permission_accessibility_body'),
              icon: const Icon(Icons.accessibility_new_outlined, color: Colors.white, size: 22),
              selected: true,
              tail: tr('onboarding_permission_needs_action'),
            ),
            _MiniResultEntry(
              title: tr('onboarding_permission_disk_title'),
              subtitle: tr('onboarding_permission_disk_body'),
              icon: Icon(Icons.folder_open_outlined, color: accent, size: 22),
              tail: tr('onboarding_permission_optional'),
            ),
            _MiniResultEntry(
              title: tr('onboarding_permission_privacy_card'),
              subtitle: Platform.isMacOS ? tr('onboarding_permission_open_privacy') : tr('onboarding_permissions_lite_body'),
              icon: Icon(Icons.security_outlined, color: accent, size: 22),
              tail: tr('onboarding_permission_ready'),
            ),
            _MiniResultEntry(
              title: tr('onboarding_permissions_lite_title'),
              subtitle: tr('onboarding_permissions_lite_body'),
              icon: Icon(Icons.verified_user_outlined, color: accent, size: 22),
              tail: Platform.operatingSystem,
            ),
          ],
        );
      case 'mainHotkey':
        return _MainHotkeyDemo(accent: accent, hotkey: mainHotkey, tr: tr);
      case 'selectionHotkey':
        return _SelectionHotkeyDemo(accent: accent, hotkey: selectionHotkey, tr: tr);
      case 'glance':
        // Feature refinement: Glance now uses the same simulated desktop frame
        // as the other onboarding demos, so the feature tour feels continuous
        // instead of switching between standalone cards and desktop scenes.
        return _DesktopFramedDemo(
          accent: accent,
          child: Padding(
            padding: const EdgeInsets.fromLTRB(48, 82, 52, 44),
            child: _MiniWoxWindow(
              accent: accent,
              query: 'wox',
              queryAccessory: _buildGlanceAccessory(),
              opaqueBackground: true,
              results: [
                _MiniResultEntry(
                  title: glanceEnabled ? glanceLabel : tr('ui_glance_enable'),
                  subtitle: glanceEnabled ? tr('onboarding_glance_description') : tr('ui_glance_enable_tips'),
                  icon: _GlanceInlineIcon(
                    icon: glanceEnabled ? glanceIcon : null,
                    fallback: glanceEnabled ? Icons.remove_red_eye_outlined : Icons.visibility_off_outlined,
                    color: Colors.white,
                    size: 22,
                  ),
                  selected: true,
                  tail: glanceEnabled ? glanceValue : '',
                ),
                _MiniResultEntry(
                  title: tr('onboarding_glance_sample_provider'),
                  subtitle: tr('onboarding_glance_loading_body'),
                  icon: Icon(Icons.bolt_outlined, color: accent, size: 22),
                  tail: 'Glance',
                ),
                _MiniResultEntry(
                  title: tr('ui_glance_primary'),
                  subtitle: tr('onboarding_glance_picker_label'),
                  icon: const Icon(Icons.push_pin_outlined, color: Color(0xFF60A5FA), size: 22),
                  tail: glanceEnabled ? glanceLabel : '',
                ),
                _MiniResultEntry(
                  title: tr('onboarding_glance_empty_title'),
                  subtitle: tr('onboarding_glance_empty_body'),
                  icon: const Icon(Icons.visibility_outlined, color: Color(0xFFFACC15), size: 22),
                  tail: glanceEnabled ? '' : tr('onboarding_can_skip'),
                ),
              ],
            ),
          ),
        );
      case 'actionPanel':
        return _ActionPanelDemo(accent: accent, hotkey: Platform.isMacOS ? 'Cmd+J' : 'Alt+J', queryAccessory: _buildGlanceAccessory(), tr: tr);
      case 'queryHotkeys':
        return _QueryHotkeysDemo(accent: accent, tr: tr);
      case 'queryShortcuts':
        return _QueryShortcutsDemo(accent: accent, tr: tr);
      case 'trayQueries':
        return _TrayQueriesDemo(accent: accent, tr: tr);
      case 'wpmInstall':
        return _WpmInstallDemo(accent: accent, tr: tr);
      case 'themeInstall':
        return _ThemeInstallDemo(accent: accent, tr: tr);
      case 'finish':
        return _MiniWoxWindow(
          accent: accent,
          query: 'ready',
          queryAccessory: _buildGlanceAccessory(),
          results: [
            _MiniResultEntry(
              title: tr('onboarding_finish_card_title'),
              subtitle: tr('onboarding_finish_card_body'),
              icon: const Icon(Icons.check_rounded, color: Colors.white, size: 24),
              selected: true,
              tail: tr('onboarding_finish_badge'),
            ),
            const _MiniResultEntry(title: 'Open Wox Settings', subtitle: r'C:\Users\qianl\AppData\Roaming\Wox', icon: _WoxLogoMark()),
            _MiniResultEntry(
              title: tr('onboarding_action_panel_title'),
              subtitle: tr('onboarding_action_panel_description'),
              icon: Icon(Icons.play_arrow_rounded, color: accent, size: 23),
              tail: Platform.isMacOS ? 'Cmd+J' : 'Alt+J',
            ),
            _MiniResultEntry(
              title: tr('onboarding_query_hotkeys_title'),
              subtitle: tr('onboarding_query_shortcuts_title'),
              icon: const Icon(Icons.manage_search_rounded, color: Color(0xFFA78BFA), size: 23),
              tail: tr('ui_tray_queries'),
            ),
          ],
        );
      default:
        return _MiniWoxWindow(accent: accent, query: 'wox');
    }
  }
}

String _formatDemoHotkey(String hotkey, {required String fallback}) {
  final configuredHotkey = hotkey.trim();
  final rawHotkey = configuredHotkey.isEmpty ? fallback : configuredHotkey;
  // Feature refinement: onboarding demos render the persisted hotkey values
  // rather than static labels. Formatting is limited to display casing so the
  // animation always mirrors the recorder shown above it.
  return rawHotkey.split('+').map(_formatDemoHotkeyPart).join('+');
}

String _formatDemoHotkeyPart(String part) {
  final normalized = part.trim().toLowerCase();
  return switch (normalized) {
    'alt' || 'option' => Platform.isMacOS ? 'Option' : 'Alt',
    'control' || 'ctrl' => 'Ctrl',
    'shift' => 'Shift',
    'meta' || 'command' || 'cmd' => Platform.isMacOS ? 'Cmd' : 'Win',
    'windows' || 'win' => 'Win',
    'space' => 'Space',
    'enter' => 'Enter',
    'escape' || 'esc' => 'Esc',
    'backspace' => 'Backspace',
    'delete' => 'Delete',
    'tab' => 'Tab',
    'arrowup' || 'up' => 'Up',
    'arrowdown' || 'down' => 'Down',
    'arrowleft' || 'left' => 'Left',
    'arrowright' || 'right' => 'Right',
    _ => normalized.isEmpty ? part : '${normalized[0].toUpperCase()}${normalized.substring(1)}',
  };
}

class _MainHotkeyDemo extends StatefulWidget {
  const _MainHotkeyDemo({required this.accent, required this.hotkey, required this.tr});

  final Color accent;
  final String hotkey;
  final String Function(String key) tr;

  @override
  State<_MainHotkeyDemo> createState() => _MainHotkeyDemoState();
}

class _MainHotkeyDemoState extends State<_MainHotkeyDemo> with SingleTickerProviderStateMixin {
  late final AnimationController _controller;

  @override
  void initState() {
    super.initState();
    _controller = AnimationController(vsync: this, duration: const Duration(milliseconds: 4200))..repeat();
  }

  @override
  void dispose() {
    _controller.dispose();
    super.dispose();
  }

  double _interval(double start, double end, Curve curve) {
    final value = ((_controller.value - start) / (end - start)).clamp(0.0, 1.0).toDouble();
    return curve.transform(value);
  }

  double _windowProgress() {
    if (_controller.value < 0.28) {
      return 0;
    }
    if (_controller.value < 0.46) {
      return _interval(0.28, 0.46, Curves.easeOutCubic);
    }
    if (_controller.value < 0.88) {
      return 1;
    }
    return 1 - _interval(0.88, 1, Curves.easeInCubic);
  }

  double _shortcutProgress() {
    if (_controller.value < 0.10) {
      return 0;
    }
    if (_controller.value < 0.22) {
      return _interval(0.10, 0.22, Curves.easeOutCubic);
    }
    if (_controller.value < 0.54) {
      return 1;
    }
    return 1 - _interval(0.54, 0.68, Curves.easeInCubic);
  }

  bool _isShortcutPressed() {
    return _controller.value >= 0.20 && _controller.value <= 0.34;
  }

  String _queryText() {
    if (_controller.value < 0.52) {
      return '';
    }
    if (_controller.value < 0.60) {
      return 'a';
    }
    if (_controller.value < 0.68) {
      return 'ap';
    }
    return 'app';
  }

  String _displayHotkey() {
    return _formatDemoHotkey(widget.hotkey, fallback: Platform.isMacOS ? 'option+space' : 'alt+space');
  }

  @override
  Widget build(BuildContext context) {
    final hotkey = _displayHotkey();
    final desktopIsMac = Platform.isMacOS;

    // Feature change: the main-hotkey preview now teaches the real launch
    // moment instead of showing an already-open Wox window. A scripted Flutter
    // scene keeps the demo theme-aware and platform-aware without shipping
    // separate recorded videos for macOS, Windows, and Linux.
    return AnimatedBuilder(
      key: const ValueKey('onboarding-main-hotkey-demo'),
      animation: _controller,
      builder: (context, child) {
        final shortcutProgress = _shortcutProgress();
        final windowProgress = _windowProgress();

        return ClipRRect(
          borderRadius: BorderRadius.circular(8),
          child: Stack(
            children: [
              Positioned.fill(child: _DesktopDemoBackground(accent: widget.accent, isMac: desktopIsMac)),
              Positioned.fill(
                child: Opacity(
                  opacity: shortcutProgress,
                  child: Transform.translate(
                    offset: Offset(0, 8 * (1 - shortcutProgress)),
                    child: _HotkeyPressOverlay(hotkey: hotkey, accent: widget.accent, pressed: _isShortcutPressed()),
                  ),
                ),
              ),
              // Feature refinement: Wox now enters by position and scale only.
              // Fading the whole launcher made the opened window look
              // translucent against the desktop, which weakened the demo.
              if (windowProgress > 0.01)
                Positioned.fill(
                  child: Transform.translate(
                    offset: Offset(0, 22 * (1 - windowProgress)),
                    child: Transform.scale(
                      scale: 0.95 + (0.05 * windowProgress),
                      child: Padding(
                        // Feature refinement: keep the opened Wox preview
                        // centered inside the now-taller demo area. The height
                        // fix belongs to the media slot, not to an artificial
                        // upward offset inside the desktop scene.
                        padding: const EdgeInsets.fromLTRB(34, 42, 34, 42),
                        child: _MiniWoxWindow(
                          accent: widget.accent,
                          query: _queryText(),
                          opaqueBackground: true,
                          results: [
                            _MiniResultEntry(
                              title: widget.tr('onboarding_main_hotkey_title'),
                              subtitle: widget.tr('onboarding_main_hotkey_tip'),
                              icon: const Icon(Icons.keyboard_alt_outlined, color: Colors.white, size: 23),
                              selected: true,
                              tail: hotkey,
                            ),
                            _MiniResultEntry(
                              title: 'Applications',
                              subtitle: widget.tr('onboarding_media_app_result_subtitle'),
                              icon: Icon(Icons.apps_rounded, color: widget.accent, size: 23),
                              tail: 'Apps',
                            ),
                            _MiniResultEntry(
                              title: 'Files',
                              subtitle: widget.tr('onboarding_media_file_result_subtitle'),
                              icon: const Icon(Icons.folder_outlined, color: Color(0xFFFACC15), size: 23),
                              tail: 'Files',
                            ),
                            const _MiniResultEntry(
                              title: 'Plugins',
                              subtitle: r'C:\Users\qianl\dev\Wox.Plugin.Template.Nodejs',
                              icon: Icon(Icons.extension_outlined, color: Color(0xFF60A5FA), size: 23),
                              tail: '51 day ago',
                            ),
                          ],
                        ),
                      ),
                    ),
                  ),
                ),
            ],
          ),
        );
      },
    );
  }
}

class _SelectionHotkeyDemo extends StatefulWidget {
  const _SelectionHotkeyDemo({required this.accent, required this.hotkey, required this.tr});

  final Color accent;
  final String hotkey;
  final String Function(String key) tr;

  @override
  State<_SelectionHotkeyDemo> createState() => _SelectionHotkeyDemoState();
}

class _SelectionHotkeyDemoState extends State<_SelectionHotkeyDemo> with SingleTickerProviderStateMixin {
  static const String _selectedFileName = 'Quarterly plan.pdf';

  late final AnimationController _controller;

  @override
  void initState() {
    super.initState();
    _controller = AnimationController(vsync: this, duration: const Duration(milliseconds: 5200))..repeat();
  }

  @override
  void dispose() {
    _controller.dispose();
    super.dispose();
  }

  double _interval(double start, double end, Curve curve) {
    final value = ((_controller.value - start) / (end - start)).clamp(0.0, 1.0).toDouble();
    return curve.transform(value);
  }

  double _cursorProgress() {
    if (_controller.value < 0.08) {
      return 0;
    }
    if (_controller.value < 0.34) {
      return _interval(0.08, 0.34, Curves.easeInOutCubic);
    }
    return 1;
  }

  double _shortcutProgress() {
    if (_controller.value < 0.36) {
      return 0;
    }
    if (_controller.value < 0.46) {
      return _interval(0.36, 0.46, Curves.easeOutCubic);
    }
    if (_controller.value < 0.66) {
      return 1;
    }
    return 1 - _interval(0.66, 0.78, Curves.easeInCubic);
  }

  double _windowProgress() {
    if (_controller.value < 0.56) {
      return 0;
    }
    if (_controller.value < 0.74) {
      return _interval(0.56, 0.74, Curves.easeOutCubic);
    }
    if (_controller.value < 0.92) {
      return 1;
    }
    return 1 - _interval(0.92, 1, Curves.easeInCubic);
  }

  bool _isFileSelected() {
    return _controller.value >= 0.30 && _controller.value < 0.95;
  }

  bool _isShortcutPressed() {
    return _controller.value >= 0.46 && _controller.value <= 0.58;
  }

  String _displayHotkey() {
    return _formatDemoHotkey(widget.hotkey, fallback: Platform.isMacOS ? 'cmd+option+space' : 'ctrl+alt+space');
  }

  @override
  Widget build(BuildContext context) {
    final hotkey = _displayHotkey();
    final desktopIsMac = Platform.isMacOS;

    // Feature change: the selection-hotkey preview now demonstrates the real
    // workflow: choose something on the desktop, press the configured shortcut,
    // and open Wox with context-specific actions for that selection.
    return AnimatedBuilder(
      key: const ValueKey('onboarding-selection-hotkey-demo'),
      animation: _controller,
      builder: (context, child) {
        final cursorProgress = _cursorProgress();
        final shortcutProgress = _shortcutProgress();
        final windowProgress = _windowProgress();
        final fileSelected = _isFileSelected();

        return LayoutBuilder(
          builder: (context, constraints) {
            final startCursor = Offset(constraints.maxWidth - 96, constraints.maxHeight - 86);
            final targetCursor = Offset(186, 112);
            final cursorOffset = Offset.lerp(startCursor, targetCursor, cursorProgress)!;
            final cursorOpacity = 1 - _interval(0.70, 0.86, Curves.easeInCubic);

            return ClipRRect(
              borderRadius: BorderRadius.circular(8),
              child: Stack(
                children: [
                  Positioned.fill(child: _DesktopDemoBackground(accent: widget.accent, isMac: desktopIsMac, showDefaultIcons: false)),
                  Positioned(left: 42, top: 54, child: _SelectionDesktopFile(label: 'Roadmap.md', icon: Icons.article_outlined, accent: const Color(0xFF60A5FA))),
                  Positioned(
                    left: 150,
                    top: 54,
                    child: _SelectionDesktopFile(label: _selectedFileName, icon: Icons.picture_as_pdf_outlined, accent: widget.accent, selected: fileSelected),
                  ),
                  Positioned(left: 258, top: 54, child: _SelectionDesktopFile(label: 'Screenshots', icon: Icons.folder_outlined, accent: const Color(0xFFFACC15))),
                  Positioned(left: 64, top: 150, child: _SelectionDesktopFile(label: 'Release notes.txt', icon: Icons.description_outlined, accent: const Color(0xFF34D399))),
                  if (cursorOpacity > 0.01)
                    Positioned(left: cursorOffset.dx, top: cursorOffset.dy, child: Opacity(opacity: cursorOpacity, child: _DemoCursor(accent: widget.accent))),
                  Positioned.fill(
                    child: Opacity(
                      opacity: shortcutProgress,
                      child: Transform.translate(
                        offset: Offset(0, 8 * (1 - shortcutProgress)),
                        child: _HotkeyPressOverlay(hotkey: hotkey, accent: widget.accent, pressed: _isShortcutPressed()),
                      ),
                    ),
                  ),
                  // Feature refinement: the selection launcher appears fully
                  // opaque, matching the main-hotkey demo and making the file
                  // action rows readable over the simulated desktop.
                  if (windowProgress > 0.01)
                    Positioned.fill(
                      child: Transform.translate(
                        offset: Offset(0, 20 * (1 - windowProgress)),
                        child: Transform.scale(
                          scale: 0.95 + (0.05 * windowProgress),
                          child: Padding(
                            padding: const EdgeInsets.fromLTRB(52, 134, 52, 38),
                            child: _MiniWoxWindow(
                              accent: widget.accent,
                              query: _selectedFileName,
                              opaqueBackground: true,
                              results: [
                                _MiniResultEntry(
                                  title: 'Quick Actions',
                                  subtitle: _selectedFileName,
                                  icon: const Icon(Icons.touch_app_outlined, color: Colors.white, size: 23),
                                  selected: true,
                                  tail: hotkey,
                                ),
                                _MiniResultEntry(
                                  title: 'Open file',
                                  subtitle: 'Open the selected desktop file',
                                  icon: Icon(Icons.open_in_new_rounded, color: widget.accent, size: 23),
                                  tail: 'Enter',
                                ),
                                const _MiniResultEntry(
                                  title: 'Copy file path',
                                  subtitle: r'C:\Users\qianl\Desktop\Quarterly plan.pdf',
                                  icon: Icon(Icons.copy_rounded, color: Color(0xFF38BDF8), size: 23),
                                  tail: 'Copy',
                                ),
                                const _MiniResultEntry(
                                  title: 'Show in folder',
                                  subtitle: 'Reveal the selected file in its location',
                                  icon: Icon(Icons.folder_open_outlined, color: Color(0xFFFACC15), size: 23),
                                ),
                              ],
                            ),
                          ),
                        ),
                      ),
                    ),
                ],
              ),
            );
          },
        );
      },
    );
  }
}

class _QueryHotkeysDemo extends StatefulWidget {
  const _QueryHotkeysDemo({required this.accent, required this.tr});

  final Color accent;
  final String Function(String key) tr;

  @override
  State<_QueryHotkeysDemo> createState() => _QueryHotkeysDemoState();
}

class _QueryHotkeysDemoState extends State<_QueryHotkeysDemo> with SingleTickerProviderStateMixin {
  late final AnimationController _controller;

  @override
  void initState() {
    super.initState();
    _controller = AnimationController(vsync: this, duration: const Duration(milliseconds: 4600))..repeat();
  }

  @override
  void dispose() {
    _controller.dispose();
    super.dispose();
  }

  double _interval(double start, double end, Curve curve) {
    final value = ((_controller.value - start) / (end - start)).clamp(0.0, 1.0).toDouble();
    return curve.transform(value);
  }

  double _shortcutProgress() {
    if (_controller.value < 0.18) return 0;
    if (_controller.value < 0.30) {
      return _interval(0.18, 0.30, Curves.easeOutCubic);
    }
    if (_controller.value < 0.50) return 1;
    return 1 - _interval(0.50, 0.62, Curves.easeInCubic);
  }

  double _windowProgress() {
    if (_controller.value < 0.40) return 0;
    if (_controller.value < 0.58) {
      return _interval(0.40, 0.58, Curves.easeOutCubic);
    }
    if (_controller.value < 0.90) return 1;
    return 1 - _interval(0.90, 1, Curves.easeInCubic);
  }

  bool _isShortcutPressed() {
    return _controller.value >= 0.30 && _controller.value <= 0.42;
  }

  @override
  Widget build(BuildContext context) {
    final hotkey = _formatDemoHotkey('', fallback: Platform.isMacOS ? 'cmd+shift+g' : 'ctrl+shift+g');

    // Feature change: Query Hotkeys now get their own onboarding motion. The
    // demo starts from a configured binding, then shows the hotkey opening Wox
    // directly with the bound query instead of sharing the old summary list.
    return AnimatedBuilder(
      key: const ValueKey('onboarding-query-hotkeys-demo'),
      animation: _controller,
      builder: (context, child) {
        final shortcutProgress = _shortcutProgress();
        final windowProgress = _windowProgress();

        return ClipRRect(
          borderRadius: BorderRadius.circular(8),
          child: Stack(
            children: [
              Positioned.fill(child: _DesktopDemoBackground(accent: widget.accent, isMac: Platform.isMacOS, showDefaultIcons: false)),
              Positioned.fill(
                child: Padding(
                  // Feature refinement: query feature demos use a shared top
                  // hint strip and reserve the rest of the scene for the actual
                  // launcher animation. This keeps explanation and demo from
                  // competing for the same vertical space.
                  padding: const EdgeInsets.fromLTRB(48, 18, 52, 36),
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.stretch,
                    children: [
                      _QueryDemoHintStrip(
                        accent: widget.accent,
                        icon: Icons.keyboard_command_key,
                        title: widget.tr('onboarding_query_hotkeys_title'),
                        from: hotkey,
                        to: 'github repo',
                      ),
                      const SizedBox(height: 12),
                      Expanded(
                        child: Stack(
                          children: [
                            Positioned.fill(
                              child: Opacity(
                                opacity: shortcutProgress,
                                child: Transform.translate(
                                  offset: Offset(0, 8 * (1 - shortcutProgress)),
                                  child: _HotkeyPressOverlay(hotkey: hotkey, accent: widget.accent, pressed: _isShortcutPressed()),
                                ),
                              ),
                            ),
                            if (windowProgress > 0.01)
                              Positioned.fill(
                                child: Transform.translate(
                                  offset: Offset(0, 20 * (1 - windowProgress)),
                                  child: Transform.scale(
                                    scale: 0.95 + (0.05 * windowProgress),
                                    // Feature refinement: keep Query Hotkeys
                                    // aligned with Query Shortcuts. The
                                    // previous extra top inset left an empty
                                    // band between the hint strip and Wox,
                                    // while the shared remaining-area layout
                                    // gives the demo more usable space.
                                    child: _MiniWoxWindow(
                                      accent: widget.accent,
                                      query: 'github repo',
                                      opaqueBackground: true,
                                      results: [
                                        _MiniResultEntry(
                                          title: 'Wox repository',
                                          subtitle: 'Open Wox-launcher/Wox on GitHub',
                                          icon: const Icon(Icons.code_rounded, color: Colors.white, size: 23),
                                          selected: true,
                                          tail: hotkey,
                                        ),
                                        _MiniResultEntry(
                                          title: widget.tr('onboarding_query_hotkeys_title'),
                                          subtitle: widget.tr('onboarding_query_hotkeys_body'),
                                          icon: Icon(Icons.bolt_outlined, color: widget.accent, size: 23),
                                          tail: widget.tr('ui_query_hotkeys'),
                                        ),
                                        const _MiniResultEntry(
                                          title: 'Issues',
                                          subtitle: 'github repo issues',
                                          icon: Icon(Icons.bug_report_outlined, color: Color(0xFFFACC15), size: 23),
                                          tail: 'GitHub',
                                        ),
                                      ],
                                    ),
                                  ),
                                ),
                              ),
                          ],
                        ),
                      ),
                    ],
                  ),
                ),
              ),
            ],
          ),
        );
      },
    );
  }
}

class _QueryShortcutsDemo extends StatefulWidget {
  const _QueryShortcutsDemo({required this.accent, required this.tr});

  final Color accent;
  final String Function(String key) tr;

  @override
  State<_QueryShortcutsDemo> createState() => _QueryShortcutsDemoState();
}

class _QueryShortcutsDemoState extends State<_QueryShortcutsDemo> with SingleTickerProviderStateMixin {
  late final AnimationController _controller;

  @override
  void initState() {
    super.initState();
    _controller = AnimationController(vsync: this, duration: const Duration(milliseconds: 4400))..repeat();
  }

  @override
  void dispose() {
    _controller.dispose();
    super.dispose();
  }

  double _interval(double start, double end, Curve curve) {
    final value = ((_controller.value - start) / (end - start)).clamp(0.0, 1.0).toDouble();
    return curve.transform(value);
  }

  String _queryText() {
    if (_controller.value < 0.18) return '';
    if (_controller.value < 0.30) return 'g';
    if (_controller.value < 0.48) return 'gh';
    if (_controller.value < 0.58) return 'gh ';
    if (_controller.value < 0.68) return 'gh r';
    return 'gh repo';
  }

  bool _isExpanded() {
    return _controller.value >= 0.68 && _controller.value < 0.94;
  }

  @override
  Widget build(BuildContext context) {
    // Feature change: Query Shortcuts are now shown as a typing workflow. The
    // animation makes the alias expansion visible, which the old combined
    // advanced-query page could only describe in text.
    return AnimatedBuilder(
      key: const ValueKey('onboarding-query-shortcuts-demo'),
      animation: _controller,
      builder: (context, child) {
        final expandedProgress = _isExpanded() ? _interval(0.68, 0.80, Curves.easeOutCubic) : 0.0;

        return ClipRRect(
          borderRadius: BorderRadius.circular(8),
          child: Stack(
            children: [
              Positioned.fill(child: _DesktopDemoBackground(accent: widget.accent, isMac: Platform.isMacOS, showDefaultIcons: false)),
              Positioned.fill(
                child: Padding(
                  // Feature fix: hint content stays in a horizontal strip above
                  // Wox. This preserves the top/bottom demo rhythm while keeping
                  // the alias mapping visible instead of letting the launcher
                  // overlap the teaching content.
                  padding: const EdgeInsets.fromLTRB(48, 18, 52, 36),
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.stretch,
                    children: [
                      _QueryDemoHintStrip(
                        accent: widget.accent,
                        icon: Icons.short_text_outlined,
                        title: widget.tr('onboarding_query_shortcuts_title'),
                        from: 'gh repo',
                        to: 'github repo',
                        progress: 0.35 + (0.65 * expandedProgress),
                      ),
                      const SizedBox(height: 12),
                      Expanded(
                        child: _MiniWoxWindow(
                          accent: widget.accent,
                          query: _queryText(),
                          opaqueBackground: true,
                          results: [
                            _MiniResultEntry(
                              // Feature fix: Wox keeps the visible query as
                              // "gh repo"; only the internal query sent to
                              // providers expands to "github repo".
                              title: 'Open repository',
                              subtitle: _isExpanded() ? 'github repo' : widget.tr('onboarding_query_shortcuts_body'),
                              icon: const Icon(Icons.short_text_outlined, color: Colors.white, size: 23),
                              selected: true,
                              tail: _isExpanded() ? 'gh' : widget.tr('ui_query_shortcuts'),
                            ),
                            _MiniResultEntry(
                              title: 'Repository search',
                              subtitle: 'github repo',
                              icon: Icon(Icons.open_in_new_rounded, color: widget.accent, size: 23),
                              tail: 'Enter',
                            ),
                            const _MiniResultEntry(title: 'Search issues', subtitle: 'github issues', icon: Icon(Icons.search_rounded, color: Color(0xFF60A5FA), size: 23)),
                          ],
                        ),
                      ),
                    ],
                  ),
                ),
              ),
            ],
          ),
        );
      },
    );
  }
}

class _TrayQueriesDemo extends StatefulWidget {
  const _TrayQueriesDemo({required this.accent, required this.tr});

  final Color accent;
  final String Function(String key) tr;

  @override
  State<_TrayQueriesDemo> createState() => _TrayQueriesDemoState();
}

class _TrayQueriesDemoState extends State<_TrayQueriesDemo> with SingleTickerProviderStateMixin {
  late final AnimationController _controller;

  @override
  void initState() {
    super.initState();
    _controller = AnimationController(vsync: this, duration: const Duration(milliseconds: 5000))..repeat();
  }

  @override
  void dispose() {
    _controller.dispose();
    super.dispose();
  }

  double _interval(double start, double end, Curve curve) {
    final value = ((_controller.value - start) / (end - start)).clamp(0.0, 1.0).toDouble();
    return curve.transform(value);
  }

  double _cursorProgress() {
    if (_controller.value < 0.10) return 0;
    if (_controller.value < 0.38) {
      return _interval(0.10, 0.38, Curves.easeInOutCubic);
    }
    return 1;
  }

  double _windowProgress() {
    if (_controller.value < 0.48) return 0;
    if (_controller.value < 0.66) {
      return _interval(0.48, 0.66, Curves.easeOutCubic);
    }
    if (_controller.value < 0.92) return 1;
    return 1 - _interval(0.92, 1, Curves.easeInCubic);
  }

  bool _isTrayPressed() {
    return _controller.value >= 0.38 && _controller.value <= 0.50;
  }

  @override
  Widget build(BuildContext context) {
    final isMac = Platform.isMacOS;

    // Feature change: Tray Queries now show the click path from tray/menu-bar
    // icon to a query window near that system area. This separates the tray
    // mental model from keyboard-triggered query features.
    return AnimatedBuilder(
      key: const ValueKey('onboarding-tray-queries-demo'),
      animation: _controller,
      builder: (context, child) {
        final cursorProgress = _cursorProgress();
        final windowProgress = _windowProgress();

        return LayoutBuilder(
          builder: (context, constraints) {
            final trayAnchor = isMac ? Offset(constraints.maxWidth - 84, 17) : Offset(constraints.maxWidth - 120, constraints.maxHeight - 23);
            final startCursor = Offset(60, constraints.maxHeight - 82);
            final cursorOffset = Offset.lerp(startCursor, trayAnchor.translate(-6, -10), cursorProgress)!;

            return ClipRRect(
              borderRadius: BorderRadius.circular(8),
              child: Stack(
                children: [
                  Positioned.fill(child: _DesktopDemoBackground(accent: widget.accent, isMac: isMac, showDefaultIcons: false)),
                  Positioned(
                    left: 48,
                    right: 52,
                    top: 18,
                    child: _QueryDemoHintStrip(
                      accent: widget.accent,
                      icon: Icons.ads_click_rounded,
                      title: widget.tr('onboarding_tray_queries_title'),
                      from: 'tray icon',
                      to: 'weather',
                    ),
                  ),
                  Positioned(left: trayAnchor.dx - 18, top: trayAnchor.dy - 18, child: _TrayQueryIcon(accent: widget.accent, pressed: _isTrayPressed())),
                  Positioned(left: cursorOffset.dx, top: cursorOffset.dy, child: _DemoCursor(accent: widget.accent)),
                  if (windowProgress > 0.01)
                    Positioned(
                      right: 48,
                      top: 96,
                      width: 420,
                      height: 240,
                      child: Transform.translate(
                        offset: Offset(0, 18 * (1 - windowProgress)),
                        child: Transform.scale(
                          scale: 0.95 + (0.05 * windowProgress),
                          alignment: isMac ? Alignment.topRight : Alignment.bottomRight,
                          child: _MiniWoxWindow(
                            accent: widget.accent,
                            query: 'weather',
                            opaqueBackground: true,
                            results: [
                              _MiniResultEntry(
                                title: 'Weather',
                                subtitle: 'Sunny, 24 C',
                                icon: const Icon(Icons.wb_sunny_outlined, color: Colors.white, size: 23),
                                selected: true,
                                tail: widget.tr('ui_tray_queries'),
                              ),
                              _MiniResultEntry(
                                title: widget.tr('onboarding_tray_queries_title'),
                                subtitle: widget.tr('onboarding_tray_queries_body'),
                                icon: Icon(Icons.ads_click_rounded, color: widget.accent, size: 23),
                                tail: 'Tray',
                              ),
                              const _MiniResultEntry(
                                title: 'Calendar',
                                subtitle: 'Next meeting in 25 minutes',
                                icon: Icon(Icons.calendar_month_outlined, color: Color(0xFF60A5FA), size: 23),
                              ),
                            ],
                          ),
                        ),
                      ),
                    ),
                ],
              ),
            );
          },
        );
      },
    );
  }
}

class _WpmInstallDemo extends StatelessWidget {
  const _WpmInstallDemo({required this.accent, required this.tr});

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
        _MiniResultEntry(
          title: 'Browser Bookmarks',
          subtitle: tr('plugin_wpm_command_install'),
          icon: const Icon(Icons.bookmark_outline_rounded, color: Color(0xFFFACC15), size: 23),
          tail: tr('plugin_wpm_install'),
        ),
        const _MiniResultEntry(title: 'ChatGPT', subtitle: 'AI assistant plugin', icon: Icon(Icons.auto_awesome_outlined, color: Color(0xFFA78BFA), size: 23), tail: 'AI'),
      ],
    );
  }
}

class _ThemeInstallDemo extends StatelessWidget {
  const _ThemeInstallDemo({required this.accent, required this.tr});

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
        _MiniResultEntry(
          title: 'Aurora',
          subtitle: tr('plugin_theme_group_store'),
          icon: const _ThemeSwatchIcon(background: Color(0xFF261A3D), accent: Color(0xFFE879F9), highlight: Color(0xFFFACC15)),
          tail: tr('plugin_theme_install_theme'),
        ),
        _MiniResultEntry(
          title: 'Default Dark',
          subtitle: tr('plugin_theme_group_current'),
          icon: const _ThemeSwatchIcon(background: Color(0xFF1F2937), accent: Color(0xFF60A5FA), highlight: Color(0xFF94A3B8)),
          tail: tr('ui_setting_theme_system_tag'),
        ),
      ],
    );
  }
}

class _InstallFlowDemo extends StatefulWidget {
  const _InstallFlowDemo({
    required this.demoKey,
    required this.accent,
    required this.icon,
    required this.title,
    required this.hintFrom,
    required this.hintTo,
    required this.queryStages,
    required this.installLabel,
    required this.installingLabel,
    required this.installedLabel,
    required this.primaryTitle,
    required this.primarySubtitle,
    required this.primaryIcon,
    required this.secondaryResults,
  });

  final ValueKey<String> demoKey;
  final Color accent;
  final IconData icon;
  final String title;
  final String hintFrom;
  final String hintTo;
  final List<String> queryStages;
  final String installLabel;
  final String installingLabel;
  final String installedLabel;
  final String primaryTitle;
  final String primarySubtitle;
  final Widget primaryIcon;
  final List<_MiniResultEntry> secondaryResults;

  @override
  State<_InstallFlowDemo> createState() => _InstallFlowDemoState();
}

class _InstallFlowDemoState extends State<_InstallFlowDemo> with SingleTickerProviderStateMixin {
  late final AnimationController _controller;

  @override
  void initState() {
    super.initState();
    _controller = AnimationController(vsync: this, duration: const Duration(milliseconds: 4600))..repeat();
  }

  @override
  void dispose() {
    _controller.dispose();
    super.dispose();
  }

  double _interval(double start, double end, Curve curve) {
    final value = ((_controller.value - start) / (end - start)).clamp(0.0, 1.0).toDouble();
    return curve.transform(value);
  }

  String _queryText() {
    final typingProgress = _interval(0.10, 0.56, Curves.easeOutCubic);
    final rawStage = (typingProgress * (widget.queryStages.length - 1)).floor();
    final stage = rawStage.clamp(0, widget.queryStages.length - 1).toInt();
    return widget.queryStages[stage];
  }

  double _installProgress() {
    if (_controller.value < 0.64) {
      return 0;
    }
    if (_controller.value < 0.78) {
      return _interval(0.64, 0.78, Curves.easeOutCubic);
    }
    if (_controller.value < 0.94) {
      return 1;
    }
    return 1 - _interval(0.94, 1, Curves.easeInCubic);
  }

  String _primaryTail() {
    if (_controller.value >= 0.64 && _controller.value < 0.76) {
      return widget.installingLabel;
    }
    if (_controller.value >= 0.76 && _controller.value < 0.94) {
      return widget.installedLabel;
    }
    return widget.installLabel;
  }

  @override
  Widget build(BuildContext context) {
    // Feature change: WPM and theme installation use the same compact desktop
    // teaching pattern as query shortcuts. The shared animation keeps the top
    // hint strip stable while the launcher demonstrates typing, selecting a
    // store result, and reaching the install action.
    return AnimatedBuilder(
      key: widget.demoKey,
      animation: _controller,
      builder: (context, child) {
        final installProgress = _installProgress();

        return ClipRRect(
          borderRadius: BorderRadius.circular(8),
          child: Stack(
            children: [
              Positioned.fill(child: _DesktopDemoBackground(accent: widget.accent, isMac: Platform.isMacOS, showDefaultIcons: false)),
              Positioned.fill(
                child: Padding(
                  padding: const EdgeInsets.fromLTRB(48, 18, 52, 36),
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.stretch,
                    children: [
                      _QueryDemoHintStrip(
                        accent: widget.accent,
                        icon: widget.icon,
                        title: widget.title,
                        from: widget.hintFrom,
                        to: widget.hintTo,
                        progress: 0.45 + (0.55 * installProgress),
                      ),
                      const SizedBox(height: 12),
                      Expanded(
                        child: _MiniWoxWindow(
                          accent: widget.accent,
                          query: _queryText(),
                          opaqueBackground: true,
                          results: [
                            _MiniResultEntry(title: widget.primaryTitle, subtitle: widget.primarySubtitle, icon: widget.primaryIcon, selected: true, tail: _primaryTail()),
                            ...widget.secondaryResults,
                          ],
                        ),
                      ),
                    ],
                  ),
                ),
              ),
            ],
          ),
        );
      },
    );
  }
}

class _ThemeSwatchIcon extends StatelessWidget {
  const _ThemeSwatchIcon({required this.background, required this.accent, required this.highlight});

  final Color background;
  final Color accent;
  final Color highlight;

  @override
  Widget build(BuildContext context) {
    return Container(
      width: 28,
      height: 28,
      decoration: BoxDecoration(color: background, borderRadius: BorderRadius.circular(7), border: Border.all(color: Colors.white.withValues(alpha: 0.16))),
      child: Stack(
        children: [
          Positioned(left: 5, right: 5, top: 7, child: Container(height: 4, decoration: BoxDecoration(color: accent, borderRadius: BorderRadius.circular(999)))),
          Positioned(left: 5, right: 11, top: 14, child: Container(height: 4, decoration: BoxDecoration(color: highlight, borderRadius: BorderRadius.circular(999)))),
          Positioned(
            left: 5,
            right: 15,
            top: 20,
            child: Container(height: 3, decoration: BoxDecoration(color: Colors.white.withValues(alpha: 0.72), borderRadius: BorderRadius.circular(999))),
          ),
        ],
      ),
    );
  }
}

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

class _QueryDemoHintStrip extends StatelessWidget {
  const _QueryDemoHintStrip({required this.accent, required this.icon, required this.title, required this.from, required this.to, this.progress = 1});

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

class _TrayQueryIcon extends StatelessWidget {
  const _TrayQueryIcon({required this.accent, required this.pressed});

  final Color accent;
  final bool pressed;

  @override
  Widget build(BuildContext context) {
    return AnimatedContainer(
      duration: const Duration(milliseconds: 140),
      width: 36,
      height: 36,
      decoration: BoxDecoration(
        color: pressed ? accent.withValues(alpha: 0.90) : getThemeBackgroundColor().withValues(alpha: 0.88),
        border: Border.all(color: pressed ? accent : getThemeTextColor().withValues(alpha: 0.14)),
        borderRadius: BorderRadius.circular(10),
        boxShadow: [BoxShadow(color: accent.withValues(alpha: pressed ? 0.28 : 0.10), blurRadius: pressed ? 20 : 12)],
      ),
      child: Icon(Icons.wb_sunny_outlined, color: pressed ? Colors.white : accent, size: 20),
    );
  }
}

class _SelectionDesktopFile extends StatelessWidget {
  const _SelectionDesktopFile({required this.label, required this.icon, required this.accent, this.selected = false});

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

class _DesktopDemoBackground extends StatelessWidget {
  const _DesktopDemoBackground({required this.accent, required this.isMac, this.showDefaultIcons = true});

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

class _DesktopFramedDemo extends StatelessWidget {
  const _DesktopFramedDemo({required this.accent, required this.child});

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
      child: Stack(children: [Positioned.fill(child: _DesktopDemoBackground(accent: accent, isMac: Platform.isMacOS, showDefaultIcons: false)), Positioned.fill(child: child)]),
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

class _ActionPanelDemo extends StatefulWidget {
  const _ActionPanelDemo({required this.accent, required this.hotkey, required this.queryAccessory, required this.tr});

  final Color accent;
  final String hotkey;
  final Widget? queryAccessory;
  final String Function(String key) tr;

  @override
  State<_ActionPanelDemo> createState() => _ActionPanelDemoState();
}

class _ActionPanelDemoState extends State<_ActionPanelDemo> with SingleTickerProviderStateMixin {
  late final AnimationController _controller;

  @override
  void initState() {
    super.initState();
    _controller = AnimationController(vsync: this, duration: const Duration(milliseconds: 3600))..repeat();
  }

  @override
  void dispose() {
    _controller.dispose();
    super.dispose();
  }

  double _curvedPhase(double start, double end, Curve curve) {
    final value = ((_controller.value - start) / (end - start)).clamp(0.0, 1.0).toDouble();
    return curve.transform(value);
  }

  double _panelProgress() {
    if (_controller.value < 0.26) {
      return 0;
    }
    if (_controller.value < 0.48) {
      return _curvedPhase(0.26, 0.48, Curves.easeOutCubic);
    }
    if (_controller.value < 0.88) {
      return 1;
    }
    return 1 - _curvedPhase(0.88, 1, Curves.easeInCubic);
  }

  bool _isShortcutPressed() {
    return _controller.value >= 0.18 && _controller.value <= 0.42;
  }

  @override
  Widget build(BuildContext context) {
    // Feature change: the Action Panel onboarding preview is now shaped like the
    // real launcher instead of a static two-column sketch. The old version
    // named the actions but did not teach the Alt+J transition, so this compact
    // animation keeps the query, result list, footer shortcuts, and floating
    // action panel recognizable while staying cheap to render in the guide.
    return AnimatedBuilder(
      animation: _controller,
      builder: (context, child) {
        final panelProgress = _panelProgress();
        final shortcutPressed = _isShortcutPressed();

        // Feature refinement: Action Panel now runs inside the shared desktop
        // frame. The previous standalone Wox card lacked the same system
        // context as the other onboarding demos, so wrapping the real launcher
        // mock keeps the visual language consistent without changing behavior.
        return _DesktopFramedDemo(
          accent: widget.accent,
          child: Padding(
            padding: const EdgeInsets.fromLTRB(48, 76, 52, 44),
            child: _MiniWoxWindow(
              accent: widget.accent,
              query: 'sett',
              queryAccessory: widget.queryAccessory,
              footerHotkey: widget.hotkey,
              isFooterHotkeyPressed: shortcutPressed,
              actionPanelProgress: panelProgress,
              actionPanel: _MiniActionPanel(accent: widget.accent, tr: widget.tr),
              opaqueBackground: true,
            ),
          ),
        );
      },
    );
  }
}

class _MiniResultEntry {
  const _MiniResultEntry({required this.title, required this.icon, this.subtitle, this.tail, this.selected = false});

  final String title;
  final String? subtitle;
  final Widget icon;
  final bool selected;
  final String? tail;
}

class _MiniWoxWindow extends StatelessWidget {
  const _MiniWoxWindow({
    required this.accent,
    required this.query,
    this.results = const [
      _MiniResultEntry(title: 'Open Wox Settings', subtitle: r'C:\Users\qianl\AppData\Roaming\Wox', icon: _WoxLogoMark(), tail: '2 day ago', selected: true),
      _MiniResultEntry(
        title: 'Open URL settings',
        subtitle: 'Configure URL open rules and browser targets',
        icon: Icon(Icons.link_rounded, color: Color(0xFF38BDF8), size: 24),
        tail: 'Settings',
      ),
      _MiniResultEntry(
        title: 'Open WebView settings',
        subtitle: 'Inspect and tune embedded preview behavior',
        icon: Icon(Icons.language_rounded, color: Color(0xFF60A5FA), size: 24),
      ),
      _MiniResultEntry(
        title: 'Open Update settings',
        subtitle: 'Check update channel and release status',
        icon: Icon(Icons.sync_rounded, color: Color(0xFF3B82F6), size: 24),
        tail: 'Update',
      ),
    ],
    this.queryAccessory,
    this.footerHotkey,
    this.isFooterHotkeyPressed = false,
    this.actionPanel,
    this.actionPanelProgress = 0,
    this.opaqueBackground = false,
  });

  final Color accent;
  final String query;
  final List<_MiniResultEntry> results;
  final Widget? queryAccessory;
  final String? footerHotkey;
  final bool isFooterHotkeyPressed;
  final Widget? actionPanel;
  final double actionPanelProgress;
  final bool opaqueBackground;

  @override
  Widget build(BuildContext context) {
    return LayoutBuilder(
      builder: (context, constraints) {
        final metrics = WoxInterfaceSizeUtil.instance.current;
        // Bug fix: the preview is now a supporting element with less vertical
        // weight. Respecting the parent height prevents small onboarding
        // windows from overflowing while still capping tall windows so the demo
        // does not dominate the current setup task.
        final previewHeight = constraints.maxHeight.clamp(0.0, 320.0).toDouble();
        final previewWidth = constraints.maxWidth;
        final hasFooter = footerHotkey != null;
        final footerHeight = hasFooter ? WoxThemeUtil.instance.getToolbarHeight() : 0.0;
        final actionPanelWidth = (previewWidth * 0.42).clamp(250.0, 320.0).toDouble();
        final queryTop = 12.0;
        final resultTop = queryTop + metrics.queryBoxBaseHeight + 10;

        return Center(
          child: SizedBox(
            width: previewWidth,
            height: previewHeight,
            child: Container(
              decoration: BoxDecoration(
                // Feature refinement: the main-hotkey desktop scene needs an
                // opaque launcher surface so the simulated desktop does not
                // wash through Wox. Other onboarding previews keep their
                // existing translucent treatment unless they opt in.
                color: opaqueBackground ? getThemeBackgroundColor().withValues(alpha: 1) : getThemeBackgroundColor().withValues(alpha: 0.86),
                border: Border.all(color: getThemeTextColor().withValues(alpha: 0.10)),
                borderRadius: BorderRadius.circular(8),
              ),
              child: ClipRRect(
                borderRadius: BorderRadius.circular(8),
                child: Stack(
                  children: [
                    Positioned.fill(
                      child: DecoratedBox(
                        decoration: BoxDecoration(
                          gradient: RadialGradient(center: const Alignment(0.6, -0.5), radius: 1.05, colors: [accent.withValues(alpha: 0.12), Colors.transparent]),
                        ),
                      ),
                    ),
                    _MiniSearchBar(query: query, trailing: queryAccessory),
                    // Feature refinement: the mock launcher now follows the
                    // production query/result vertical rhythm instead of using
                    // compact onboarding-only row spacing. This keeps fonts,
                    // padding, and density comparable to the real Wox window.
                    Positioned(left: 12, right: 12, top: resultTop, bottom: footerHeight + 10, child: _MiniResultList(results: results)),
                    if (hasFooter) _MiniFooter(accent: accent, hotkey: footerHotkey!, isPressed: isFooterHotkeyPressed),
                    if (actionPanel != null)
                      Positioned(
                        right: 16,
                        bottom: footerHeight + 12,
                        width: actionPanelWidth,
                        child: Opacity(
                          opacity: actionPanelProgress,
                          child: Transform.translate(
                            offset: Offset(18 * (1 - actionPanelProgress), 10 * (1 - actionPanelProgress)),
                            child: Transform.scale(alignment: Alignment.bottomRight, scale: 0.96 + (0.04 * actionPanelProgress), child: actionPanel),
                          ),
                        ),
                      ),
                  ],
                ),
              ),
            ),
          ),
        );
      },
    );
  }
}

class _MiniSearchBar extends StatelessWidget {
  const _MiniSearchBar({required this.query, this.trailing});

  final String query;
  final Widget? trailing;

  @override
  Widget build(BuildContext context) {
    final metrics = WoxInterfaceSizeUtil.instance.current;
    final woxTheme = WoxThemeUtil.instance.currentTheme.value;

    return Positioned(
      left: 12,
      right: 12,
      top: 12,
      child: Container(
        height: metrics.queryBoxBaseHeight,
        padding: const EdgeInsets.only(left: 8, right: 8, top: QUERY_BOX_CONTENT_PADDING_TOP, bottom: QUERY_BOX_CONTENT_PADDING_BOTTOM),
        decoration: BoxDecoration(color: woxTheme.queryBoxBackgroundColorParsed, borderRadius: BorderRadius.circular(woxTheme.queryBoxBorderRadius.toDouble())),
        child: Row(
          children: [
            Expanded(
              child: Text(query, maxLines: 1, overflow: TextOverflow.ellipsis, style: TextStyle(color: woxTheme.queryBoxFontColorParsed, fontSize: metrics.queryBoxFontSize)),
            ),
            // Feature refinement: the search row has no implicit Glance item.
            // Callers opt in only after the Glance step has introduced and
            // enabled it, which keeps earlier examples from leaking later
            // onboarding concepts.
            if (trailing != null) ...[const SizedBox(width: 10), trailing!],
          ],
        ),
      ),
    );
  }
}

class _MiniResultList extends StatelessWidget {
  const _MiniResultList({required this.results});

  final List<_MiniResultEntry> results;

  @override
  Widget build(BuildContext context) {
    // Bug fix: the preview rows now use the real Wox result height, and the
    // Action Panel demo also reserves toolbar space. A fixed Column can exceed
    // the remaining preview height, while the production result view is a
    // clipped list. Using a non-scrollable ListView preserves real row sizing
    // and clips overflow instead of rendering Flutter's overflow warning.
    return ListView.builder(
      padding: EdgeInsets.zero,
      physics: const NeverScrollableScrollPhysics(),
      itemCount: results.length,
      itemBuilder: (context, index) {
        final result = results[index];
        return _MiniResultRow(title: result.title, subtitle: result.subtitle, icon: result.icon, selected: result.selected, tail: result.tail);
      },
    );
  }
}

class _MiniResultRow extends StatelessWidget {
  const _MiniResultRow({required this.title, required this.icon, this.subtitle, this.selected = false, this.tail});

  final String title;
  final String? subtitle;
  final Widget icon;
  final bool selected;
  final String? tail;

  @override
  Widget build(BuildContext context) {
    // Bug fix: rows must keep launcher-like density even when a preview passes
    // fewer entries. The previous Expanded row made two-result demos stretch
    // into oversized blocks, so each mock result now has a stable row height.
    // Feature refinement: rows now also model Wox's subtitle and tail affordance
    // so the shared preview shows file paths, result descriptions, and status
    // chips instead of flattening every result into a single title line.
    // Feature refinement: result row metrics now come from the production
    // launcher sizing/theme utilities. The onboarding-specific padding and
    // bold text made the preview look unlike Wox, while reusing these values
    // keeps the example aligned with real query results across densities.
    final metrics = WoxInterfaceSizeUtil.instance.current;
    final woxTheme = WoxThemeUtil.instance.currentTheme.value;
    final hasSubtitle = subtitle != null && subtitle!.isNotEmpty;
    final borderRadius = woxTheme.resultItemBorderRadius > 0 ? BorderRadius.circular(woxTheme.resultItemBorderRadius.toDouble()) : BorderRadius.zero;
    final maxBorderWidth =
        (woxTheme.resultItemActiveBorderLeftWidth > woxTheme.resultItemBorderLeftWidth ? woxTheme.resultItemActiveBorderLeftWidth : woxTheme.resultItemBorderLeftWidth).toDouble();
    final actualBorderWidth = selected ? woxTheme.resultItemActiveBorderLeftWidth.toDouble() : woxTheme.resultItemBorderLeftWidth.toDouble();
    final titleColor = selected ? woxTheme.resultItemActiveTitleColorParsed : woxTheme.resultItemTitleColorParsed;
    final subtitleColor = selected ? woxTheme.resultItemActiveSubTitleColorParsed : woxTheme.resultItemSubTitleColorParsed;
    final tailColor = selected ? woxTheme.resultItemActiveTailTextColorParsed : woxTheme.resultItemTailTextColorParsed;

    Widget content = Container(
      decoration: BoxDecoration(color: selected ? woxTheme.resultItemActiveBackgroundColorParsed : Colors.transparent),
      padding: EdgeInsets.only(
        top: metrics.scaledSpacing(woxTheme.resultItemPaddingTop.toDouble()),
        right: metrics.scaledSpacing(woxTheme.resultItemPaddingRight.toDouble()),
        bottom: metrics.scaledSpacing(woxTheme.resultItemPaddingBottom.toDouble()),
        left: metrics.scaledSpacing(woxTheme.resultItemPaddingLeft.toDouble() + maxBorderWidth),
      ),
      child: Row(
        children: [
          Padding(
            padding: EdgeInsets.only(left: metrics.resultItemIconPaddingLeft, right: metrics.resultItemIconPaddingRight),
            child: SizedBox(width: metrics.resultIconSize, height: metrics.resultIconSize, child: FittedBox(fit: BoxFit.contain, child: icon)),
          ),
          Expanded(
            child: Column(
              mainAxisAlignment: MainAxisAlignment.center,
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text(title, maxLines: 1, overflow: TextOverflow.ellipsis, style: TextStyle(color: titleColor, fontSize: metrics.resultTitleFontSize)),
                if (hasSubtitle)
                  Padding(
                    padding: EdgeInsets.only(top: metrics.resultItemSubtitlePaddingTop),
                    child: Text(subtitle!, maxLines: 1, overflow: TextOverflow.ellipsis, style: TextStyle(color: subtitleColor, fontSize: metrics.resultSubtitleFontSize)),
                  ),
              ],
            ),
          ),
          if (tail != null && tail!.isNotEmpty)
            Padding(
              padding: EdgeInsets.only(left: metrics.resultItemTailPaddingLeft, right: metrics.resultItemTailPaddingRight),
              child: Padding(
                padding: EdgeInsets.only(left: metrics.resultItemTailItemPaddingLeft),
                child: ConstrainedBox(
                  constraints: const BoxConstraints(maxWidth: 132),
                  child: DecoratedBox(
                    decoration: BoxDecoration(
                      color: Colors.transparent,
                      border: Border.all(color: tailColor.withValues(alpha: selected ? 0.34 : 0.2)),
                      borderRadius: BorderRadius.circular(999),
                    ),
                    child: Padding(
                      padding: EdgeInsets.symmetric(horizontal: metrics.resultItemTextTailHPadding, vertical: metrics.resultItemTextTailVPadding),
                      child: Text(tail!, maxLines: 1, overflow: TextOverflow.ellipsis, style: TextStyle(color: tailColor, fontSize: metrics.tailHotkeyFontSize)),
                    ),
                  ),
                ),
              ),
            ),
        ],
      ),
    );

    if (borderRadius != BorderRadius.zero) {
      content = ClipRRect(borderRadius: borderRadius, child: content);
    }

    if (actualBorderWidth > 0) {
      content = Stack(
        children: [
          content,
          Positioned(
            left: 0,
            top: 0,
            bottom: 0,
            child: Container(
              width: actualBorderWidth,
              decoration: BoxDecoration(
                color: woxTheme.resultItemActiveBackgroundColorParsed,
                borderRadius: borderRadius != BorderRadius.zero ? BorderRadius.only(topLeft: borderRadius.topLeft, bottomLeft: borderRadius.bottomLeft) : BorderRadius.zero,
              ),
            ),
          ),
        ],
      );
    }

    return SizedBox(height: WoxThemeUtil.instance.getResultItemHeight(), child: content);
  }
}

class _MiniActionPanel extends StatelessWidget {
  const _MiniActionPanel({required this.accent, required this.tr});

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

class _WoxLogoMark extends StatelessWidget {
  const _WoxLogoMark();

  @override
  Widget build(BuildContext context) {
    return Container(
      width: 28,
      height: 28,
      decoration: BoxDecoration(color: Colors.white, borderRadius: BorderRadius.circular(6)),
      alignment: Alignment.center,
      child: Text('W', style: TextStyle(color: Colors.black.withValues(alpha: 0.92), fontSize: 20, fontWeight: FontWeight.w900, height: 1)),
    );
  }
}

class _GlanceAccessory extends StatelessWidget {
  const _GlanceAccessory({super.key, required this.label, required this.value, required this.icon});

  final String label;
  final String value;
  final WoxImage? icon;

  @override
  Widget build(BuildContext context) {
    final metrics = WoxInterfaceSizeUtil.instance.current;
    final baseTextColor = WoxThemeUtil.instance.currentTheme.value.queryBoxFontColorParsed;
    final accessoryColor = baseTextColor.withValues(alpha: 0.8);

    // Feature refinement: the real launcher renders Glance as a lightweight
    // inline query accessory, not as a bordered badge. Matching that shape here
    // keeps the onboarding preview aligned with the production window chrome.
    return ConstrainedBox(
      constraints: BoxConstraints(maxWidth: metrics.queryBoxGlanceMaxWidth),
      child: Container(
        height: metrics.scaledSpacing(30),
        padding: EdgeInsets.symmetric(horizontal: metrics.scaledSpacing(8)),
        decoration: BoxDecoration(color: Colors.transparent, borderRadius: BorderRadius.circular(5)),
        child: Row(
          mainAxisSize: MainAxisSize.min,
          children: [
            _GlanceInlineIcon(icon: icon, fallback: Icons.schedule_outlined, color: accessoryColor, size: metrics.scaledSpacing(16)),
            SizedBox(width: metrics.scaledSpacing(5)),
            Flexible(
              child: Text(
                value.isEmpty ? label : value,
                maxLines: 1,
                overflow: TextOverflow.ellipsis,
                style: TextStyle(color: accessoryColor, fontSize: metrics.queryBoxGlanceFontSize),
              ),
            ),
          ],
        ),
      ),
    );
  }
}

class _GlanceInlineIcon extends StatelessWidget {
  const _GlanceInlineIcon({required this.icon, required this.fallback, required this.color, required this.size});

  final WoxImage? icon;
  final IconData fallback;
  final Color color;
  final double size;

  @override
  Widget build(BuildContext context) {
    if (icon != null && icon!.imageData.isNotEmpty) {
      // Feature change: inline Glance previews render the icon returned by the
      // API, which preserves state-specific glyphs while retaining the fallback
      // eye for missing or not-yet-loaded responses.
      return WoxImageView(woxImage: icon!, width: size, height: size, svgColor: color);
    }

    return Icon(fallback, color: color, size: size);
  }
}

import 'dart:async';
import 'dart:convert';
import 'dart:io';

import 'package:flutter/material.dart';
import 'package:get/get.dart';
import 'package:uuid/v4.dart';
import 'package:wox/api/wox_api.dart';
import 'package:wox/components/demo/wox_demo.dart';
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

const double _onboardingSidebarWidth = 256;
const double _onboardingFooterHeight = 72;

class WoxOnboardingView extends StatefulWidget {
  const WoxOnboardingView({super.key});

  @override
  State<WoxOnboardingView> createState() => _WoxOnboardingViewState();
}

class _OnboardingStep {
  const _OnboardingStep({required this.id, required this.titleKey});

  final String id;
  final String titleKey;
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
    const _OnboardingStep(id: 'welcome', titleKey: 'onboarding_welcome_title'),
    // Permission setup is macOS-only because Windows and Linux do not need a
    // first-run system permission page for the Wox features introduced here.
    // Keeping the step out of the list also keeps numbering and progress honest.
    if (Platform.isMacOS) const _OnboardingStep(id: 'permissions', titleKey: 'onboarding_permissions_title'),
    const _OnboardingStep(id: 'mainHotkey', titleKey: 'onboarding_main_hotkey_title'),
    const _OnboardingStep(id: 'selectionHotkey', titleKey: 'onboarding_selection_hotkey_title'),
    // Feature change: width and density tuning made onboarding feel longer than
    // the core first-run flow. Removing that dedicated page keeps the tour
    // focused while the shared Wox preview still demonstrates the real layout.
    const _OnboardingStep(id: 'glance', titleKey: 'onboarding_glance_title'),
    const _OnboardingStep(id: 'actionPanel', titleKey: 'onboarding_action_panel_title'),
    // Feature change: the previous Advanced Queries page bundled three
    // unrelated workflows. Splitting them into dedicated steps lets each query
    // feature get its own explanation and animated demo.
    const _OnboardingStep(id: 'queryHotkeys', titleKey: 'onboarding_query_hotkeys_title'),
    const _OnboardingStep(id: 'queryShortcuts', titleKey: 'onboarding_query_shortcuts_title'),
    const _OnboardingStep(id: 'trayQueries', titleKey: 'onboarding_tray_queries_title'),
    // Feature change: plugin and theme installation are common first-run
    // workflows, so they are taught as standalone sections instead of being
    // hidden behind generic query examples.
    const _OnboardingStep(id: 'wpmInstall', titleKey: 'onboarding_wpm_install_title'),
    const _OnboardingStep(id: 'themeInstall', titleKey: 'onboarding_theme_install_title'),
    const _OnboardingStep(id: 'finish', titleKey: 'onboarding_finish_title'),
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

    double settingsHeightForStep(String stepId) {
      // Layout change: the general step description moved out of the right pane
      // so the animated demo can become the primary teaching surface. Each
      // settings area gets a fixed, scrollable height instead of sharing a flex
      // ratio with the demo, which keeps the demo size predictable per viewport.
      return switch (stepId) {
        'welcome' => 150,
        'permissions' => 146,
        'mainHotkey' || 'selectionHotkey' => 126,
        'glance' => 148,
        'finish' => 112,
        _ => 108,
      };
    }

    return Padding(
      padding: const EdgeInsets.fromLTRB(38, 30, 38, 20),
      child: LayoutBuilder(
        builder: (context, constraints) {
          const titleHeight = 44.0;
          const titleToSettingsGap = 16.0;
          const settingsToDemoGap = 18.0;
          final availableHeight = constraints.maxHeight;
          final preferredSettingsHeight = settingsHeightForStep(activeStep.id);
          final maxSettingsHeight = (availableHeight - titleHeight - titleToSettingsGap - settingsToDemoGap - 360).clamp(84.0, preferredSettingsHeight).toDouble();
          final settingsHeight = preferredSettingsHeight.clamp(84.0, maxSettingsHeight).toDouble();
          final demoHeight = (availableHeight - titleHeight - titleToSettingsGap - settingsHeight - settingsToDemoGap).clamp(280.0, double.infinity).toDouble();

          return AnimatedSwitcher(
            duration: const Duration(milliseconds: 280),
            switchInCurve: Curves.easeOutCubic,
            switchOutCurve: Curves.easeInCubic,
            transitionBuilder: (child, animation) {
              final offset = Tween<Offset>(begin: const Offset(0.025, 0), end: Offset.zero).animate(animation);
              return FadeTransition(opacity: animation, child: SlideTransition(position: offset, child: child));
            },
            child: SizedBox(
              key: ValueKey('onboarding-stage-${activeStep.id}'),
              height: availableHeight,
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.stretch,
                children: [
                  SizedBox(
                    height: titleHeight,
                    child: Align(
                      alignment: Alignment.topLeft,
                      child: Text(
                        tr(activeStep.titleKey),
                        maxLines: 1,
                        overflow: TextOverflow.ellipsis,
                        style: TextStyle(color: getThemeTextColor(), fontSize: 32, fontWeight: FontWeight.w800, height: 1.05),
                      ),
                    ),
                  ),
                  const SizedBox(height: titleToSettingsGap),
                  SizedBox(height: settingsHeight, child: SingleChildScrollView(child: Align(alignment: Alignment.topLeft, child: _buildStepContent()))),
                  const SizedBox(height: settingsToDemoGap),
                  SizedBox(height: demoHeight, child: buildMediaSlot()),
                ],
              ),
            ),
          );
        },
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
    return ClipRRect(
      borderRadius: BorderRadius.circular(8),
      // Layout change: the parent now calculates an explicit demo height, so
      // this slot fills that space instead of applying its own cap. Keeping the
      // size decision in one place makes every onboarding section allocate as
      // much room as possible to the animated walkthrough.
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
    return WoxDemoGlanceAccessory(
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
        return WoxDemoWindow(
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
        );
      case 'mainHotkey':
        return WoxMainHotkeyDemo(accent: accent, hotkey: mainHotkey, tr: tr);
      case 'selectionHotkey':
        return WoxSelectionHotkeyDemo(accent: accent, hotkey: selectionHotkey, tr: tr);
      case 'glance':
        return WoxGlanceDemo(accent: accent, enabled: glanceEnabled, label: glanceLabel, value: glanceValue, icon: glanceIcon, tr: tr);
      case 'actionPanel':
        return WoxActionPanelDemo(accent: accent, hotkey: Platform.isMacOS ? 'Cmd+J' : 'Alt+J', queryAccessory: _buildGlanceAccessory(), tr: tr);
      case 'queryHotkeys':
        return WoxQueryHotkeysDemo(accent: accent, tr: tr);
      case 'queryShortcuts':
        return WoxQueryShortcutsDemo(accent: accent, tr: tr);
      case 'trayQueries':
        return WoxTrayQueriesDemo(accent: accent, tr: tr);
      case 'wpmInstall':
        return WoxWpmInstallDemo(accent: accent, tr: tr);
      case 'themeInstall':
        return WoxThemeInstallDemo(accent: accent, tr: tr);
      case 'finish':
        return WoxDemoWindow(
          accent: accent,
          query: 'ready',
          queryAccessory: _buildGlanceAccessory(),
          results: [
            WoxDemoResult(
              title: tr('onboarding_finish_card_title'),
              subtitle: tr('onboarding_finish_card_body'),
              icon: const Icon(Icons.check_rounded, color: Colors.white, size: 24),
              selected: true,
              tail: tr('onboarding_finish_badge'),
            ),
            const WoxDemoResult(title: 'Open Wox Settings', subtitle: r'C:\Users\qianl\AppData\Roaming\Wox', icon: WoxDemoLogoMark()),
            WoxDemoResult(
              title: tr('onboarding_action_panel_title'),
              subtitle: tr('onboarding_action_panel_description'),
              icon: Icon(Icons.play_arrow_rounded, color: accent, size: 23),
              tail: Platform.isMacOS ? 'Cmd+J' : 'Alt+J',
            ),
            WoxDemoResult(
              title: tr('onboarding_query_hotkeys_title'),
              subtitle: tr('onboarding_query_shortcuts_title'),
              icon: const Icon(Icons.manage_search_rounded, color: Color(0xFFA78BFA), size: 23),
              tail: tr('ui_tray_queries'),
            ),
          ],
        );
      default:
        return WoxDemoWindow(accent: accent, query: 'wox');
    }
  }
}

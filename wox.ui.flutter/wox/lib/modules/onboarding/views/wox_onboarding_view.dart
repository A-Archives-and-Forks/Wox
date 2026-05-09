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
import 'package:wox/utils/colors.dart';
import 'package:wox/utils/consts.dart';

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

  late final List<_OnboardingStep> steps = [
    const _OnboardingStep(id: 'welcome', titleKey: 'onboarding_welcome_title', descriptionKey: 'onboarding_welcome_description'),
    // Permission setup is macOS-only because Windows and Linux do not need a
    // first-run system permission page for the Wox features introduced here.
    // Keeping the step out of the list also keeps numbering and progress honest.
    if (Platform.isMacOS) const _OnboardingStep(id: 'permissions', titleKey: 'onboarding_permissions_title', descriptionKey: 'onboarding_permissions_description'),
    const _OnboardingStep(id: 'mainHotkey', titleKey: 'onboarding_main_hotkey_title', descriptionKey: 'onboarding_main_hotkey_description'),
    const _OnboardingStep(id: 'selectionHotkey', titleKey: 'onboarding_selection_hotkey_title', descriptionKey: 'onboarding_selection_hotkey_description'),
    const _OnboardingStep(id: 'appearance', titleKey: 'onboarding_appearance_title', descriptionKey: 'onboarding_appearance_description'),
    const _OnboardingStep(id: 'glance', titleKey: 'onboarding_glance_title', descriptionKey: 'onboarding_glance_description'),
    const _OnboardingStep(id: 'actionPanel', titleKey: 'onboarding_action_panel_title', descriptionKey: 'onboarding_action_panel_description'),
    const _OnboardingStep(id: 'advancedQueries', titleKey: 'onboarding_advanced_queries_title', descriptionKey: 'onboarding_advanced_queries_description'),
    const _OnboardingStep(id: 'finish', titleKey: 'onboarding_finish_title', descriptionKey: 'onboarding_finish_description'),
  ];

  String tr(String key) => settingController.tr(key);

  _OnboardingStep get activeStep => steps[activeStepIndex];

  bool get isLastStep => activeStepIndex == steps.length - 1;

  Color get activeAccent => _stepAccentColor(activeStepIndex);

  double get activeProgress => (activeStepIndex + 1) / steps.length;

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

  Color _stepAccentColor(int index) {
    const accents = [
      Color(0xFF2DD4BF),
      Color(0xFFF97316),
      Color(0xFF60A5FA),
      Color(0xFFE879F9),
      Color(0xFFFACC15),
      Color(0xFF34D399),
      Color(0xFFF43F5E),
      Color(0xFFA78BFA),
      Color(0xFF22C55E),
    ];
    return accents[index % accents.length];
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
      width: 276,
      padding: const EdgeInsets.fromLTRB(30, 34, 22, 24),
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
          const SizedBox(height: 28),
          ClipRRect(
            borderRadius: BorderRadius.circular(999),
            child: LinearProgressIndicator(
              minHeight: 4,
              value: activeProgress,
              backgroundColor: getThemeTextColor().withValues(alpha: 0.08),
              valueColor: AlwaysStoppedAnimation<Color>(accent),
            ),
          ),
          const SizedBox(height: 26),
          Expanded(
            child: ListView.builder(
              itemCount: steps.length,
              itemBuilder: (context, index) {
                final step = steps[index];
                final isActive = index == activeStepIndex;
                final isDone = index < activeStepIndex;
                final nodeColor = isActive ? accent : (isDone ? getThemeActiveBackgroundColor() : getThemeSubTextColor().withValues(alpha: 0.42));
                const rowHeight = 64.0;
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
                            height: 40,
                            alignment: Alignment.centerLeft,
                            padding: const EdgeInsets.symmetric(horizontal: 10),
                            decoration: BoxDecoration(
                              color: isActive ? accent.withValues(alpha: 0.11) : Colors.transparent,
                              border: Border.all(color: isActive ? accent.withValues(alpha: 0.28) : Colors.transparent),
                              borderRadius: BorderRadius.circular(10),
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
          Text(tr(activeStep.titleKey), style: TextStyle(color: getThemeTextColor(), fontSize: 34, fontWeight: FontWeight.w800, height: 1.05)),
          const SizedBox(height: 12),
          ConstrainedBox(
            constraints: const BoxConstraints(maxWidth: 680),
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
        glanceEnabled: settingController.woxSetting.value.enableGlance,
        glanceLabel: _currentGlanceLabel(),
        glanceValue: _currentGlanceValue(),
        glanceIcon: _currentGlanceIcon(),
        appWidth: settingController.woxSetting.value.appWidth,
        interfaceSize: settingController.woxSetting.value.uiDensity,
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
                  // Feature refinement: every onboarding step now follows the
                  // same vertical rhythm as the Action Panel step. The left
                  // rail already owns progress, so the body can spend its width
                  // on compact explanation first and a shared Wox preview below.
                  Flexible(flex: 5, child: buildStepIntro()),
                  const SizedBox(height: 22),
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
      case 'appearance':
        return _buildAppearanceStep();
      case 'glance':
        return _buildGlanceStep();
      case 'actionPanel':
        return _buildFeatureStep(
          key: const ValueKey('onboarding-action-panel-page'),
          titleKey: 'onboarding_action_panel_card_title',
          bodyKey: 'onboarding_action_panel_card_body',
          badge: Platform.isMacOS ? 'Cmd+J' : 'Alt+J',
        );
      case 'advancedQueries':
        return _buildAdvancedQueriesStep();
      case 'finish':
        return _buildFeatureStep(
          key: const ValueKey('onboarding-finish-page'),
          titleKey: 'onboarding_finish_card_title',
          bodyKey: 'onboarding_finish_card_body',
          badge: tr('onboarding_finish_badge'),
        );
      default:
        return _buildFeatureStep(key: const ValueKey('onboarding-welcome-page'), titleKey: 'onboarding_welcome_card_title', bodyKey: 'onboarding_welcome_card_body', badge: 'Wox');
    }
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

  Widget _buildAppearanceStep() {
    final currentWidth = settingController.woxSetting.value.appWidth;
    final currentInterfaceSize = settingController.woxSetting.value.uiDensity;
    return _SettingsPanel(
      children: [
        Text(tr('onboarding_appearance_width_label'), style: TextStyle(color: getThemeTextColor(), fontSize: 14, fontWeight: FontWeight.w600)),
        const SizedBox(height: 12),
        Wrap(
          spacing: 10,
          runSpacing: 10,
          children:
              [750, 900, 1050]
                  .map(
                    (width) => _ChoiceButton<int>(
                      key: ValueKey('onboarding-width-$width'),
                      value: width,
                      selected: currentWidth == width,
                      label: '$width',
                      onSelected: (value) => settingController.updateConfig('AppWidth', value.toString()),
                    ),
                  )
                  .toList(),
        ),
        const SizedBox(height: 28),
        Text(tr('onboarding_appearance_interface_label'), style: TextStyle(color: getThemeTextColor(), fontSize: 14, fontWeight: FontWeight.w600)),
        const SizedBox(height: 12),
        Wrap(
          spacing: 10,
          runSpacing: 10,
          children: [
            _ChoiceButton<String>(
              key: const ValueKey('onboarding-interface-compact'),
              value: 'compact',
              selected: currentInterfaceSize == 'compact',
              label: tr('ui_interface_size_compact'),
              onSelected: (value) => settingController.updateConfig('UiDensity', value),
            ),
            _ChoiceButton<String>(
              key: const ValueKey('onboarding-interface-normal'),
              value: 'normal',
              selected: currentInterfaceSize == 'normal',
              label: tr('ui_interface_size_normal'),
              onSelected: (value) => settingController.updateConfig('UiDensity', value),
            ),
            _ChoiceButton<String>(
              key: const ValueKey('onboarding-interface-comfortable'),
              value: 'comfortable',
              selected: currentInterfaceSize == 'comfortable',
              label: tr('ui_interface_size_comfortable'),
              onSelected: (value) => settingController.updateConfig('UiDensity', value),
            ),
          ],
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
        Text(tr('onboarding_glance_picker_label'), style: TextStyle(color: getThemeTextColor(), fontSize: 14, fontWeight: FontWeight.w600)),
        const SizedBox(height: 12),
        WoxDropdownButton<String>(
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

  Widget _buildAdvancedQueriesStep() {
    return Column(
      key: const ValueKey('onboarding-advanced-query-page'),
      children: [
        _InfoPanel(title: tr('onboarding_query_hotkeys_title'), body: tr('onboarding_query_hotkeys_body'), badge: tr('ui_query_hotkeys')),
        const SizedBox(height: 12),
        _InfoPanel(title: tr('onboarding_query_shortcuts_title'), body: tr('onboarding_query_shortcuts_body'), badge: tr('ui_query_shortcuts')),
        const SizedBox(height: 12),
        _InfoPanel(title: tr('onboarding_tray_queries_title'), body: tr('onboarding_tray_queries_body'), badge: tr('ui_tray_queries')),
      ],
    );
  }

  Widget _buildFeatureStep({required Key key, required String titleKey, required String bodyKey, required String badge}) {
    return _InfoPanel(key: key, title: tr(titleKey), body: tr(bodyKey), badge: badge);
  }

  Widget _buildFooter(Color accent) {
    return Container(
      height: 76,
      padding: const EdgeInsets.symmetric(horizontal: 28),
      decoration: BoxDecoration(color: getThemeBackgroundColor().withValues(alpha: 0.52), border: Border(top: BorderSide(color: getThemeSubTextColor().withValues(alpha: 0.16)))),
      child: Row(
        children: [
          WoxButton.text(key: const ValueKey('onboarding-skip-button'), text: tr('onboarding_skip'), onPressed: () => _finish(markFinished: true)),
          const SizedBox(width: 18),
          Expanded(
            child: LayoutBuilder(
              builder: (context, constraints) {
                final completedWidth = constraints.maxWidth * activeProgress;
                return Stack(
                  alignment: Alignment.centerLeft,
                  children: [
                    Container(height: 2, decoration: BoxDecoration(color: getThemeTextColor().withValues(alpha: 0.08), borderRadius: BorderRadius.circular(999))),
                    AnimatedContainer(
                      duration: const Duration(milliseconds: 240),
                      curve: Curves.easeOutCubic,
                      width: completedWidth,
                      height: 2,
                      decoration: BoxDecoration(
                        color: accent,
                        borderRadius: BorderRadius.circular(999),
                        boxShadow: [BoxShadow(color: accent.withValues(alpha: 0.42), blurRadius: 12)],
                      ),
                    ),
                  ],
                );
              },
            ),
          ),
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
          ..color = textColor.withValues(alpha: 0.035)
          ..strokeWidth = 1;
    for (double x = 276; x < size.width; x += 52) {
      canvas.drawLine(Offset(x, 0), Offset(x, size.height - 76), gridPaint);
    }
    for (double y = 42; y < size.height - 76; y += 52) {
      canvas.drawLine(Offset(276, y), Offset(size.width, y), gridPaint);
    }

    final sweepPaint =
        Paint()
          ..shader = LinearGradient(
            begin: Alignment.topLeft,
            end: Alignment.bottomRight,
            colors: [accent.withValues(alpha: 0.18), textColor.withValues(alpha: 0.018), Colors.transparent],
            stops: const [0, 0.45, 1],
          ).createShader(Rect.fromLTWH(276, 0, size.width - 276, size.height - 76));
    final path =
        Path()
          ..moveTo(276, 0)
          ..lineTo(size.width, 0)
          ..lineTo(size.width, 230)
          ..lineTo(276, 520)
          ..close();
    canvas.drawPath(path, sweepPaint);

    final shardPaint = Paint()..color = accent.withValues(alpha: 0.055);
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

class _ChoiceButton<T> extends StatelessWidget {
  const _ChoiceButton({super.key, required this.value, required this.selected, required this.label, required this.onSelected});

  final T value;
  final bool selected;
  final String label;
  final ValueChanged<T> onSelected;

  @override
  Widget build(BuildContext context) {
    return OutlinedButton(
      onPressed: () => onSelected(value),
      style: OutlinedButton.styleFrom(
        // Animation optimization: the choice highlight changes on the same
        // short cadence as the preview, making the setting and preview feel
        // connected without adding extra state machinery.
        animationDuration: const Duration(milliseconds: 160),
        padding: const EdgeInsets.symmetric(horizontal: 18, vertical: 13),
        side: BorderSide(color: selected ? getThemeActiveBackgroundColor() : getThemeSubTextColor().withValues(alpha: 0.35)),
        backgroundColor: selected ? getThemeActiveBackgroundColor().withValues(alpha: 0.16) : Colors.transparent,
        shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(6)),
      ),
      child: Text(label, style: TextStyle(color: selected ? getThemeActiveBackgroundColor() : getThemeTextColor(), fontSize: 13, fontWeight: FontWeight.w600)),
    );
  }
}

class _OnboardingMediaSlot extends StatelessWidget {
  const _OnboardingMediaSlot({
    required this.stepId,
    required this.tr,
    required this.accent,
    required this.glanceEnabled,
    required this.glanceLabel,
    required this.glanceValue,
    required this.glanceIcon,
    required this.appWidth,
    required this.interfaceSize,
  });

  final String stepId;
  final String Function(String key) tr;
  final Color accent;
  final bool glanceEnabled;
  final String glanceLabel;
  final String glanceValue;
  final WoxImage? glanceIcon;
  final int appWidth;
  final String interfaceSize;

  @override
  Widget build(BuildContext context) {
    return ConstrainedBox(
      constraints: const BoxConstraints(maxHeight: 500),
      child: ClipRRect(
        borderRadius: BorderRadius.circular(8),
        // The right-side guide surface is intentionally Flutter-native now.
        // It reacts to onboarding state immediately, while final GIF assets
        // would lag behind settings such as Glance enablement and selection.
        child: _OnboardingMediaCard(
          stepId: stepId,
          tr: tr,
          accent: accent,
          glanceEnabled: glanceEnabled,
          glanceLabel: glanceLabel,
          glanceValue: glanceValue,
          glanceIcon: glanceIcon,
          appWidth: appWidth,
          interfaceSize: interfaceSize,
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
    required this.glanceEnabled,
    required this.glanceLabel,
    required this.glanceValue,
    required this.glanceIcon,
    required this.appWidth,
    required this.interfaceSize,
  });

  final String stepId;
  final String Function(String key) tr;
  final Color accent;
  final bool glanceEnabled;
  final String glanceLabel;
  final String glanceValue;
  final WoxImage? glanceIcon;
  final int appWidth;
  final String interfaceSize;

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
        key: ValueKey('preview-$stepId-$glanceEnabled-$glanceLabel-$glanceValue-${glanceIcon?.imageType}-${glanceIcon?.imageData}-$appWidth-$interfaceSize'),
        child: _buildPreviewContent(),
      ),
    );
  }

  double _appearanceWidthFactor() {
    const widths = [750, 900, 1050];
    final normalized = ((appWidth - widths.first) / (widths.last - widths.first)).clamp(0.0, 1.0).toDouble();
    return 0.72 + (normalized * 0.28);
  }

  String _interfaceSizeLabel() {
    switch (interfaceSize) {
      case 'compact':
        return tr('ui_interface_size_compact');
      case 'comfortable':
        return tr('ui_interface_size_comfortable');
      default:
        return tr('ui_interface_size_normal');
    }
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
              icon: const Icon(Icons.accessibility_new_outlined, color: Colors.white, size: 22),
              selected: true,
              trailing: tr('onboarding_permission_needs_action'),
            ),
            _MiniResultEntry(
              title: tr('onboarding_permission_disk_title'),
              icon: Icon(Icons.folder_open_outlined, color: accent, size: 22),
              trailing: tr('onboarding_permission_optional'),
            ),
            _MiniResultEntry(
              title: tr('onboarding_permission_privacy_card'),
              icon: Icon(Icons.security_outlined, color: accent, size: 22),
              trailing: tr('onboarding_permission_ready'),
            ),
            _MiniResultEntry(title: tr('onboarding_permissions_lite_title'), icon: Icon(Icons.verified_user_outlined, color: accent, size: 22), trailing: Platform.operatingSystem),
          ],
        );
      case 'mainHotkey':
        return _MiniWoxWindow(
          accent: accent,
          query: 'launch',
          results: [
            _MiniResultEntry(
              title: tr('onboarding_main_hotkey_title'),
              icon: const Icon(Icons.keyboard_alt_outlined, color: Colors.white, size: 23),
              selected: true,
              trailing: Platform.isMacOS ? 'Option+Space' : 'Alt+Space',
            ),
            _MiniResultEntry(title: 'Applications', icon: Icon(Icons.apps_rounded, color: accent, size: 23)),
            _MiniResultEntry(title: 'Files', icon: const Icon(Icons.folder_outlined, color: Color(0xFFFACC15), size: 23)),
            _MiniResultEntry(title: 'Plugins', icon: const Icon(Icons.extension_outlined, color: Color(0xFF60A5FA), size: 23)),
          ],
        );
      case 'selectionHotkey':
        return _MiniWoxWindow(
          accent: accent,
          query: 'selected text',
          results: [
            _MiniResultEntry(
              title: tr('onboarding_selection_hotkey_title'),
              icon: const Icon(Icons.text_fields_rounded, color: Colors.white, size: 23),
              selected: true,
              trailing: Platform.isMacOS ? 'Cmd+Option+Space' : 'Ctrl+Alt+Space',
            ),
            _MiniResultEntry(title: 'Search selection', icon: Icon(Icons.search_rounded, color: accent, size: 23)),
            _MiniResultEntry(title: 'Open selection URL', icon: const Icon(Icons.link_rounded, color: Color(0xFF38BDF8), size: 23)),
            _MiniResultEntry(title: 'Run selection query', icon: const Icon(Icons.bolt_outlined, color: Color(0xFF34D399), size: 23)),
          ],
        );
      case 'appearance':
        return _MiniWoxWindow(
          accent: accent,
          query: 'appearance',
          widthFactor: _appearanceWidthFactor(),
          results: [
            _MiniResultEntry(
              title: tr('onboarding_appearance_width_label'),
              icon: const Icon(Icons.width_normal_outlined, color: Colors.white, size: 23),
              selected: true,
              trailing: '$appWidth',
            ),
            _MiniResultEntry(
              title: tr('onboarding_appearance_interface_label'),
              icon: Icon(Icons.density_medium_outlined, color: accent, size: 23),
              trailing: _interfaceSizeLabel(),
            ),
            _MiniResultEntry(title: tr('ui_interface_size_compact'), icon: const Icon(Icons.view_headline_rounded, color: Color(0xFF60A5FA), size: 23)),
            _MiniResultEntry(title: tr('ui_interface_size_comfortable'), icon: const Icon(Icons.view_stream_rounded, color: Color(0xFFA78BFA), size: 23)),
          ],
        );
      case 'glance':
        return _MiniWoxWindow(
          accent: accent,
          query: 'wox',
          queryAccessory: _buildGlanceAccessory(),
          results: [
            _MiniResultEntry(
              title: glanceEnabled ? glanceLabel : tr('ui_glance_enable'),
              icon: _GlanceInlineIcon(
                icon: glanceEnabled ? glanceIcon : null,
                fallback: glanceEnabled ? Icons.remove_red_eye_outlined : Icons.visibility_off_outlined,
                color: Colors.white,
                size: 22,
              ),
              selected: true,
              trailing: glanceEnabled ? glanceValue : '',
            ),
            _MiniResultEntry(title: tr('onboarding_glance_sample_provider'), icon: Icon(Icons.bolt_outlined, color: accent, size: 22), trailing: 'Glance'),
            _MiniResultEntry(
              title: tr('ui_glance_primary'),
              icon: const Icon(Icons.push_pin_outlined, color: Color(0xFF60A5FA), size: 22),
              trailing: glanceEnabled ? glanceLabel : '',
            ),
            _MiniResultEntry(
              title: tr('onboarding_glance_empty_title'),
              icon: const Icon(Icons.visibility_outlined, color: Color(0xFFFACC15), size: 22),
              trailing: glanceEnabled ? '' : tr('onboarding_can_skip'),
            ),
          ],
        );
      case 'actionPanel':
        return _ActionPanelDemo(accent: accent, hotkey: Platform.isMacOS ? 'Cmd+J' : 'Alt+J', queryAccessory: _buildGlanceAccessory(), tr: tr);
      case 'advancedQueries':
        return _MiniWoxWindow(
          accent: accent,
          query: 'github repo',
          queryAccessory: _buildGlanceAccessory(),
          results: [
            _MiniResultEntry(
              title: tr('onboarding_query_hotkeys_title'),
              icon: const Icon(Icons.keyboard_command_key, color: Colors.white, size: 22),
              selected: true,
              trailing: tr('ui_query_hotkeys'),
            ),
            _MiniResultEntry(title: tr('onboarding_query_shortcuts_title'), icon: Icon(Icons.short_text_outlined, color: accent, size: 22), trailing: tr('ui_query_shortcuts')),
            _MiniResultEntry(
              title: tr('onboarding_tray_queries_title'),
              icon: const Icon(Icons.room_service_outlined, color: Color(0xFF60A5FA), size: 22),
              trailing: tr('ui_tray_queries'),
            ),
            _MiniResultEntry(title: tr('onboarding_advanced_queries_title'), icon: const Icon(Icons.manage_search_rounded, color: Color(0xFFA78BFA), size: 22), trailing: 'Wox'),
          ],
        );
      case 'finish':
        return _MiniWoxWindow(
          accent: accent,
          query: 'ready',
          queryAccessory: _buildGlanceAccessory(),
          results: [
            _MiniResultEntry(
              title: tr('onboarding_finish_card_title'),
              icon: const Icon(Icons.check_rounded, color: Colors.white, size: 24),
              selected: true,
              trailing: tr('onboarding_finish_badge'),
            ),
            _MiniResultEntry(title: 'Open Wox Settings', icon: const _WoxLogoMark()),
            _MiniResultEntry(
              title: tr('onboarding_action_panel_title'),
              icon: Icon(Icons.play_arrow_rounded, color: accent, size: 23),
              trailing: Platform.isMacOS ? 'Cmd+J' : 'Alt+J',
            ),
            _MiniResultEntry(title: tr('onboarding_advanced_queries_title'), icon: const Icon(Icons.manage_search_rounded, color: Color(0xFFA78BFA), size: 23)),
          ],
        );
      default:
        return _MiniWoxWindow(accent: accent, query: 'wox');
    }
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

        return _MiniWoxWindow(
          accent: widget.accent,
          query: 'sett',
          queryAccessory: widget.queryAccessory,
          footerHotkey: widget.hotkey,
          isFooterHotkeyPressed: shortcutPressed,
          actionPanelProgress: panelProgress,
          actionPanel: _MiniActionPanel(accent: widget.accent, tr: widget.tr),
        );
      },
    );
  }
}

class _MiniResultEntry {
  const _MiniResultEntry({required this.title, required this.icon, this.selected = false, this.trailing});

  final String title;
  final Widget icon;
  final bool selected;
  final String? trailing;
}

class _MiniWoxWindow extends StatelessWidget {
  const _MiniWoxWindow({
    required this.accent,
    required this.query,
    this.results = const [
      _MiniResultEntry(title: 'Open Wox Settings', icon: _WoxLogoMark(), selected: true),
      _MiniResultEntry(title: 'Open URL settings', icon: Icon(Icons.link_rounded, color: Color(0xFF38BDF8), size: 24)),
      _MiniResultEntry(title: 'Open WebView settings', icon: Icon(Icons.language_rounded, color: Color(0xFF60A5FA), size: 24)),
      _MiniResultEntry(title: 'Open Update settings', icon: Icon(Icons.sync_rounded, color: Color(0xFF3B82F6), size: 24)),
    ],
    this.queryAccessory,
    this.widthFactor = 1,
    this.footerHotkey,
    this.isFooterHotkeyPressed = false,
    this.actionPanel,
    this.actionPanelProgress = 0,
  });

  final Color accent;
  final String query;
  final List<_MiniResultEntry> results;
  final Widget? queryAccessory;
  final double widthFactor;
  final String? footerHotkey;
  final bool isFooterHotkeyPressed;
  final Widget? actionPanel;
  final double actionPanelProgress;

  @override
  Widget build(BuildContext context) {
    return LayoutBuilder(
      builder: (context, constraints) {
        final previewHeight = constraints.maxHeight.clamp(236.0, 330.0).toDouble();
        // Feature refinement: the shared Wox preview can still demonstrate
        // launcher width choices, but it keeps one shell implementation instead
        // of returning a separate appearance-only mock.
        final previewWidth = constraints.maxWidth * widthFactor.clamp(0.64, 1.0);
        final hasFooter = footerHotkey != null;
        final footerHeight = hasFooter ? 48.0 : 0.0;
        final actionPanelWidth = (previewWidth * 0.42).clamp(250.0, 320.0).toDouble();

        return Center(
          child: SizedBox(
            width: previewWidth,
            height: previewHeight,
            child: Container(
              decoration: BoxDecoration(
                color: getThemeBackgroundColor().withValues(alpha: 0.86),
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
                    Positioned(left: 12, right: 12, top: 66, bottom: footerHeight + 12, child: _MiniResultList(accent: accent, results: results)),
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
    return Positioned(
      left: 12,
      right: 12,
      top: 12,
      child: Container(
        height: 42,
        padding: const EdgeInsets.symmetric(horizontal: 13),
        decoration: BoxDecoration(color: getThemeTextColor().withValues(alpha: 0.075), borderRadius: BorderRadius.circular(8)),
        child: Row(
          children: [
            Expanded(
              child: Text(query, maxLines: 1, overflow: TextOverflow.ellipsis, style: TextStyle(color: getThemeTextColor(), fontSize: 26, fontWeight: FontWeight.w400, height: 1)),
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
  const _MiniResultList({required this.accent, required this.results});

  final Color accent;
  final List<_MiniResultEntry> results;

  @override
  Widget build(BuildContext context) {
    return Column(
      children: [
        for (var index = 0; index < results.length; index++) ...[
          _MiniResultRow(accent: accent, title: results[index].title, icon: results[index].icon, selected: results[index].selected, trailing: results[index].trailing),
          if (index < results.length - 1) const SizedBox(height: 7),
        ],
      ],
    );
  }
}

class _MiniResultRow extends StatelessWidget {
  const _MiniResultRow({required this.accent, required this.title, required this.icon, this.selected = false, this.trailing});

  final Color accent;
  final String title;
  final Widget icon;
  final bool selected;
  final String? trailing;

  @override
  Widget build(BuildContext context) {
    // Bug fix: rows must keep launcher-like density even when a preview passes
    // fewer entries. The previous Expanded row made two-result demos stretch
    // into oversized blocks, so each mock result now has a stable row height.
    return SizedBox(
      height: 40,
      child: Container(
        padding: const EdgeInsets.symmetric(horizontal: 10),
        decoration: BoxDecoration(color: selected ? accent.withValues(alpha: 0.76) : getThemeTextColor().withValues(alpha: 0.018), borderRadius: BorderRadius.circular(7)),
        child: Row(
          children: [
            SizedBox(width: 30, height: 30, child: Center(child: icon)),
            const SizedBox(width: 11),
            Expanded(
              child: Text(
                title,
                maxLines: 1,
                overflow: TextOverflow.ellipsis,
                style: TextStyle(color: selected ? Colors.white : getThemeTextColor(), fontSize: 13, fontWeight: FontWeight.w600),
              ),
            ),
            if (trailing != null && trailing!.isNotEmpty) ...[
              const SizedBox(width: 10),
              ConstrainedBox(
                constraints: const BoxConstraints(maxWidth: 132),
                child: Text(
                  trailing!,
                  maxLines: 1,
                  overflow: TextOverflow.ellipsis,
                  style: TextStyle(color: selected ? Colors.white.withValues(alpha: 0.90) : getThemeSubTextColor(), fontSize: 12, fontWeight: FontWeight.w600),
                ),
              ),
            ],
          ],
        ),
      ),
    );
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

    return Positioned(
      left: 0,
      right: 0,
      bottom: 0,
      child: Container(
        height: 42,
        padding: const EdgeInsets.symmetric(horizontal: 12),
        decoration: BoxDecoration(color: getThemeTextColor().withValues(alpha: 0.035), border: Border(top: BorderSide(color: getThemeTextColor().withValues(alpha: 0.07)))),
        child: FittedBox(
          alignment: Alignment.centerRight,
          fit: BoxFit.scaleDown,
          child: Row(
            mainAxisSize: MainAxisSize.min,
            children: [
              Text('Execute', style: TextStyle(color: getThemeTextColor(), fontSize: 12, fontWeight: FontWeight.w600)),
              const SizedBox(width: 8),
              _MiniShortcutKey(label: 'Enter', accent: accent, active: false),
              const SizedBox(width: 14),
              Text('More Actions', style: TextStyle(color: isPressed ? accent : getThemeTextColor(), fontSize: 12, fontWeight: FontWeight.w700)),
              const SizedBox(width: 8),
              for (var index = 0; index < keyLabels.length; index++) ...[
                _MiniShortcutKey(label: keyLabels[index], accent: accent, active: isPressed),
                if (index < keyLabels.length - 1) const SizedBox(width: 5),
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
    return AnimatedContainer(
      duration: const Duration(milliseconds: 140),
      height: 24,
      constraints: const BoxConstraints(minWidth: 28),
      padding: const EdgeInsets.symmetric(horizontal: 7),
      decoration: BoxDecoration(
        color: active ? accent.withValues(alpha: 0.20) : Colors.transparent,
        border: Border.all(color: active ? accent : getThemeTextColor().withValues(alpha: 0.66)),
        borderRadius: BorderRadius.circular(5),
      ),
      alignment: Alignment.center,
      child: Text(label, maxLines: 1, overflow: TextOverflow.ellipsis, style: TextStyle(color: active ? accent : getThemeTextColor(), fontSize: 11, fontWeight: FontWeight.w800)),
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
    final accessoryColor = getThemeSubTextColor();

    // Feature refinement: the real launcher renders Glance as a lightweight
    // inline query accessory, not as a bordered badge. Matching that shape here
    // keeps the onboarding preview aligned with the production window chrome.
    return ConstrainedBox(
      constraints: const BoxConstraints(maxWidth: 190),
      child: Row(
        mainAxisSize: MainAxisSize.min,
        children: [
          _GlanceInlineIcon(icon: icon, fallback: Icons.schedule_outlined, color: accessoryColor, size: 16),
          const SizedBox(width: 7),
          Flexible(
            child: Text(
              value.isEmpty ? label : value,
              maxLines: 1,
              overflow: TextOverflow.ellipsis,
              style: TextStyle(color: accessoryColor, fontSize: 14, fontWeight: FontWeight.w600),
            ),
          ),
        ],
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

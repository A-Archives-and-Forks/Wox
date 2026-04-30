import 'dart:async';
import 'dart:io';
import 'package:flutter/material.dart';
import 'package:get/get.dart';
import 'package:uuid/v4.dart';
import 'package:wox/components/wox_button.dart';
import 'package:wox/components/wox_image_view.dart';
import 'package:wox/components/wox_loading_indicator.dart';
import 'package:wox/components/wox_textfield.dart';
import 'package:wox/entity/wox_image.dart';
import 'package:wox/entity/wox_runtime_status.dart';
import 'package:wox/enums/wox_image_type_enum.dart';
import 'package:wox/modules/setting/views/wox_setting_base.dart';
import 'package:wox/utils/colors.dart';
import 'package:wox/utils/consts.dart';
import 'package:wox/utils/picker.dart';

// ignore: must_be_immutable
class WoxSettingRuntimeView extends WoxSettingBaseView {
  WoxSettingRuntimeView({super.key});

  String _runtimeDisplayName(String runtime) {
    switch (runtime.toUpperCase()) {
      case 'PYTHON':
        return controller.tr("ui_runtime_name_python");
      case 'NODEJS':
        return controller.tr("ui_runtime_name_nodejs");
      case 'SCRIPT':
        return controller.tr("ui_runtime_name_script");
      case 'GO':
        return controller.tr("ui_runtime_name_go");
      default:
        return runtime;
    }
  }

  String _runtimeIcon(String runtime) {
    switch (runtime.toUpperCase()) {
      case 'PYTHON':
        return PYTHON_ICON;
      case 'NODEJS':
        return NODEJS_ICON;
      case 'SCRIPT':
        return SCRIPT_ICON;
      default:
        return SCRIPT_ICON;
    }
  }

  Widget _buildRuntimeStatusCard(BuildContext context, WoxRuntimeStatus status) {
    final bool isRunning = status.isStarted;
    final Color baseBackground = getThemeBackgroundColor();
    final bool isDarkTheme = baseBackground.computeLuminance() < 0.5;
    final Color textColor = getThemeTextColor();
    final Color subTextColor = getThemeSubTextColor();
    final Color statusColor = isRunning ? Colors.green : Colors.red;
    final Color outlineColor = getThemeDividerColor().withValues(alpha: isDarkTheme ? 0.45 : 0.28);
    final Color panelColor = getThemePanelBackgroundColor();
    final Color blendedPanelColor = panelColor.a < 1 ? Color.alphaBlend(panelColor, baseBackground) : panelColor;
    final Color tileColor = isDarkTheme ? blendedPanelColor.lighter(8) : blendedPanelColor.darker(6);
    final Color iconBackgroundColor = isDarkTheme ? blendedPanelColor.lighter(16) : blendedPanelColor.darker(2);

    final String stateLabel = isRunning ? controller.tr("ui_runtime_status_running") : controller.tr("ui_runtime_status_stopped");
    final String pluginCountLabel = controller.tr("ui_runtime_status_plugin_count").replaceAll("{count}", status.loadedPluginCount.toString());
    final String hostVersionLabel = status.hostVersion.isNotEmpty && !status.hostVersion.toLowerCase().startsWith('v') ? 'v${status.hostVersion}' : status.hostVersion;
    final WoxImage runtimeIcon = WoxImage(imageType: WoxImageTypeEnum.WOX_IMAGE_TYPE_SVG.code, imageData: _runtimeIcon(status.runtime));

    return Container(
      padding: const EdgeInsets.all(14),
      decoration: BoxDecoration(
        // Keep each runtime tile lightweight but distinct; the first pass was too close to the page background and lost the grouped status shape.
        color: tileColor,
        borderRadius: BorderRadius.circular(8),
        border: Border.all(color: outlineColor),
      ),
      child: SizedBox(
        height: 88,
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Row(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Container(
                  width: 34,
                  height: 34,
                  decoration: BoxDecoration(
                    // The leading mark identifies the runtime itself; status now lives in the pill so the icon no longer changes meaning between running and stopped.
                    color: iconBackgroundColor,
                    borderRadius: BorderRadius.circular(8),
                  ),
                  child: Center(child: WoxImageView(woxImage: runtimeIcon, width: 22, height: 22)),
                ),
                const SizedBox(width: 12),
                Expanded(
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      Row(
                        children: [
                          Expanded(child: Text(_runtimeDisplayName(status.runtime), style: TextStyle(fontSize: 15, fontWeight: FontWeight.w600, color: textColor))),
                          if (hostVersionLabel.isNotEmpty) Text(hostVersionLabel, style: TextStyle(color: subTextColor, fontSize: 12)),
                        ],
                      ),
                      const SizedBox(height: 4),
                      Container(
                        padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 2),
                        decoration: BoxDecoration(color: statusColor.withValues(alpha: isDarkTheme ? 0.22 : 0.12), borderRadius: BorderRadius.circular(999)),
                        child: Text(stateLabel, style: TextStyle(fontSize: 12, fontWeight: FontWeight.w600, color: statusColor)),
                      ),
                    ],
                  ),
                ),
              ],
            ),
            const Spacer(),
            Padding(padding: const EdgeInsets.only(left: 46), child: Text(pluginCountLabel, style: TextStyle(color: subTextColor, fontSize: 13))),
          ],
        ),
      ),
    );
  }

  Widget _buildRuntimeStatusCards(List<WoxRuntimeStatus> visibleStatuses) {
    return LayoutBuilder(
      builder: (context, constraints) {
        final double availableWidth = constraints.maxWidth.isFinite ? constraints.maxWidth : GENERAL_SETTING_FORM_WIDTH;
        final double spacing = 12;
        final int columnCount =
            availableWidth >= 860
                ? 3
                : availableWidth >= 560
                ? 2
                : 1;
        final double cardWidth = columnCount == 1 ? availableWidth : (availableWidth - spacing * (columnCount - 1)) / columnCount;

        // Runtime status is a summary, not a normal label/control pair, so use the full page width and keep all three runtimes visually grouped.
        return Wrap(
          spacing: spacing,
          runSpacing: spacing,
          children: visibleStatuses.map((status) => SizedBox(width: cardWidth, child: _buildRuntimeStatusCard(context, status))).toList(),
        );
      },
    );
  }

  // Validation states
  final RxString pythonValidationMessage = ''.obs;
  final RxString nodejsValidationMessage = ''.obs;
  final RxBool isPythonValidating = false.obs;
  final RxBool isNodejsValidating = false.obs;

  // Text controllers for immediate updates
  TextEditingController? pythonController;
  TextEditingController? nodejsController;

  // Debounce timers for validation
  Timer? _pythonValidationTimer;
  Timer? _nodejsValidationTimer;

  // Validation methods
  Future<void> validatePythonPath(String path) async {
    if (path.isEmpty) {
      pythonValidationMessage.value = '';
      return;
    }

    isPythonValidating.value = true;
    try {
      final result = await Process.run(path, ['--version']);
      if (result.exitCode == 0) {
        final version = result.stdout.toString().trim();
        pythonValidationMessage.value = '✓ $version';
      } else {
        pythonValidationMessage.value = '✗ ${controller.tr("ui_runtime_validation_failed")}';
      }
    } catch (e) {
      pythonValidationMessage.value = '✗ ${controller.tr("ui_runtime_validation_error")}: ${e.toString()}';
    } finally {
      isPythonValidating.value = false;
    }
  }

  Future<void> validateNodejsPath(String path) async {
    if (path.isEmpty) {
      nodejsValidationMessage.value = '';
      return;
    }

    isNodejsValidating.value = true;
    try {
      final result = await Process.run(path, ['-v']);
      if (result.exitCode == 0) {
        final version = result.stdout.toString().trim();
        nodejsValidationMessage.value = '✓ $version';
      } else {
        nodejsValidationMessage.value = '✗ ${controller.tr("ui_runtime_validation_failed")}';
      }
    } catch (e) {
      nodejsValidationMessage.value = '✗ ${controller.tr("ui_runtime_validation_error")}: ${e.toString()}';
    } finally {
      isNodejsValidating.value = false;
    }
  }

  void updatePythonPath(String value) {
    controller.updateConfig("CustomPythonPath", value);

    // Cancel previous timer
    _pythonValidationTimer?.cancel();

    // Start new timer for debounced validation
    _pythonValidationTimer = Timer(const Duration(milliseconds: 500), () {
      validatePythonPath(value);
    });
  }

  void updateNodejsPath(String value) {
    controller.updateConfig("CustomNodejsPath", value);

    // Cancel previous timer
    _nodejsValidationTimer?.cancel();

    // Start new timer for debounced validation
    _nodejsValidationTimer = Timer(const Duration(milliseconds: 500), () {
      validateNodejsPath(value);
    });
  }

  void dispose() {
    _pythonValidationTimer?.cancel();
    _nodejsValidationTimer?.cancel();
    pythonController?.dispose();
    nodejsController?.dispose();
  }

  @override
  Widget build(BuildContext context) {
    // Initialize controllers with current values only if not already initialized
    pythonController ??= TextEditingController(text: controller.woxSetting.value.customPythonPath);
    nodejsController ??= TextEditingController(text: controller.woxSetting.value.customNodejsPath);

    // Initial validation
    if (pythonController!.text.isNotEmpty) {
      validatePythonPath(pythonController!.text);
    }
    if (nodejsController!.text.isNotEmpty) {
      validateNodejsPath(nodejsController!.text);
    }
    return Obx(() {
      final statuses = controller.runtimeStatuses;
      final bool isLoadingStatuses = controller.isRuntimeStatusLoading.value;
      final String runtimeStatusError = controller.runtimeStatusError.value;
      final List<WoxRuntimeStatus> visibleStatuses = statuses.where((status) => status.runtime.toUpperCase() != 'SCRIPT' || status.loadedPluginCount > 0).toList();

      return form(
        title: controller.tr("ui_runtime_settings"),
        description: controller.tr("ui_runtime_settings_description"),
        children: [
          formField(
            label: controller.tr("ui_runtime_status"),
            fullWidth: true,
            tips: null,
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Row(children: [if (isLoadingStatuses) const WoxLoadingIndicator(size: 16)]),
                const SizedBox(height: 12),
                if (runtimeStatusError.isNotEmpty) ...[Text(runtimeStatusError, style: TextStyle(color: Colors.red, fontSize: 12)), const SizedBox(height: 4)],
                if (!isLoadingStatuses && runtimeStatusError.isEmpty && visibleStatuses.isEmpty)
                  Text(controller.tr("ui_runtime_status_empty"), style: TextStyle(color: Colors.grey[120])),
                if (visibleStatuses.isNotEmpty) _buildRuntimeStatusCards(visibleStatuses),
              ],
            ),
          ),
          formSection(
            title: controller.tr("ui_runtime_executable_paths"),
            children: [
              formField(
                label: controller.tr("ui_runtime_python_path"),
                labelWidth: GENERAL_SETTING_LABEL_WIDTH,
                tips: controller.tr("ui_runtime_python_path_tips"),
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Row(
                      children: [
                        Expanded(
                          child: WoxTextField(
                            controller: pythonController!,
                            hintText: controller.tr("ui_runtime_python_path_placeholder"),
                            onChanged: (value) {
                              updatePythonPath(value);
                            },
                          ),
                        ),
                        const SizedBox(width: 10),
                        WoxButton.primary(
                          text: controller.tr("ui_runtime_browse"),
                          onPressed: () async {
                            final result = await FileSelector.pick(const UuidV4().generate(), FileSelectorParams(isDirectory: false));
                            if (result.isNotEmpty) {
                              pythonController!.text = result.first;
                              updatePythonPath(result.first);
                            }
                          },
                        ),
                        const SizedBox(width: 10),
                        WoxButton.secondary(
                          text: controller.tr("ui_runtime_clear"),
                          onPressed: () {
                            pythonController!.clear();
                            updatePythonPath("");
                          },
                        ),
                      ],
                    ),
                    const SizedBox(height: 5),
                    Obx(() {
                      if (isPythonValidating.value) {
                        return Row(children: [const WoxLoadingIndicator(size: 16), const SizedBox(width: 8), Text(controller.tr("ui_runtime_validating"))]);
                      } else if (pythonValidationMessage.value.isNotEmpty) {
                        return Text(
                          pythonValidationMessage.value,
                          style: TextStyle(color: pythonValidationMessage.value.startsWith('✓') ? Colors.green : Colors.red, fontSize: 12),
                        );
                      }
                      return const SizedBox.shrink();
                    }),
                  ],
                ),
              ),
              formField(
                label: controller.tr("ui_runtime_nodejs_path"),
                labelWidth: GENERAL_SETTING_LABEL_WIDTH,
                tips: controller.tr("ui_runtime_nodejs_path_tips"),
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Row(
                      children: [
                        Expanded(
                          child: WoxTextField(
                            controller: nodejsController!,
                            hintText: controller.tr("ui_runtime_nodejs_path_placeholder"),
                            onChanged: (value) {
                              updateNodejsPath(value);
                            },
                          ),
                        ),
                        const SizedBox(width: 10),
                        WoxButton.primary(
                          text: controller.tr("ui_runtime_browse"),
                          onPressed: () async {
                            final result = await FileSelector.pick(const UuidV4().generate(), FileSelectorParams(isDirectory: false));
                            if (result.isNotEmpty) {
                              nodejsController!.text = result.first;
                              updateNodejsPath(result.first);
                            }
                          },
                        ),
                        const SizedBox(width: 10),
                        WoxButton.secondary(
                          text: controller.tr("ui_runtime_clear"),
                          onPressed: () {
                            nodejsController!.clear();
                            updateNodejsPath("");
                          },
                        ),
                      ],
                    ),
                    const SizedBox(height: 5),
                    Obx(() {
                      if (isNodejsValidating.value) {
                        return Row(children: [const WoxLoadingIndicator(size: 16), const SizedBox(width: 8), Text(controller.tr("ui_runtime_validating"))]);
                      } else if (nodejsValidationMessage.value.isNotEmpty) {
                        return Text(
                          nodejsValidationMessage.value,
                          style: TextStyle(color: nodejsValidationMessage.value.startsWith('✓') ? Colors.green : Colors.red, fontSize: 12),
                        );
                      }
                      return const SizedBox.shrink();
                    }),
                  ],
                ),
              ),
            ],
          ),
        ],
      );
    });
  }
}

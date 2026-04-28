import 'dart:async';
import 'dart:io';
import 'dart:ui' as ui;

import 'package:flutter/material.dart';
import 'package:flutter/rendering.dart';
import 'package:get/get.dart';
import 'package:path_provider/path_provider.dart';
import 'package:url_launcher/url_launcher.dart';
import 'package:uuid/v4.dart';
import 'package:wox/components/wox_button.dart';
import 'package:wox/components/wox_loading_indicator.dart';
import 'package:wox/controllers/wox_setting_controller.dart';
import 'package:wox/entity/wox_usage_stats.dart';
import 'package:wox/modules/setting/views/wox_usage_share_card.dart';
import 'package:wox/utils/colors.dart';
import 'package:wox/utils/color_util.dart';
import 'package:wox/utils/log.dart';
import 'package:wox/utils/screenshot/screenshot_platform_bridge.dart';
import 'package:wox/utils/wox_theme_util.dart';

class WoxSettingUsageView extends StatefulWidget {
  const WoxSettingUsageView({super.key});

  @override
  State<WoxSettingUsageView> createState() => _WoxSettingUsageViewState();
}

class _WoxSettingUsageViewState extends State<WoxSettingUsageView> {
  final WoxSettingController controller = Get.find<WoxSettingController>();
  bool _isSharingUsage = false;
  String _shareStatusMessage = '';
  bool _shareStatusIsError = false;

  @override
  void initState() {
    super.initState();
    // Usage numbers can change while the settings window is open. Refreshing when this tab is
    // mounted keeps the page current without exposing a manual refresh button for a passive report.
    WidgetsBinding.instance.addPostFrameCallback((_) {
      unawaited(controller.refreshUsageStats());
    });
  }

  String _weekdayLabel(int weekday) {
    switch (weekday) {
      case 0:
        return controller.tr('ui_weekday_sun');
      case 1:
        return controller.tr('ui_weekday_mon');
      case 2:
        return controller.tr('ui_weekday_tue');
      case 3:
        return controller.tr('ui_weekday_wed');
      case 4:
        return controller.tr('ui_weekday_thu');
      case 5:
        return controller.tr('ui_weekday_fri');
      case 6:
        return controller.tr('ui_weekday_sat');
      default:
        return weekday.toString();
    }
  }

  Widget _form({double width = 960, required List<Widget> children}) {
    return Align(
      alignment: Alignment.topLeft,
      child: SingleChildScrollView(
        child: Padding(
          padding: const EdgeInsets.only(left: 20, right: 40, bottom: 20, top: 20),
          child: Column(crossAxisAlignment: CrossAxisAlignment.start, mainAxisSize: MainAxisSize.min, children: [...children.map((e) => SizedBox(width: width, child: e))]),
        ),
      ),
    );
  }

  Widget _statCard({required String title, required String value, required IconData icon}) {
    final bg = safeFromCssColor(WoxThemeUtil.instance.currentTheme.value.actionItemActiveBackgroundColor).withValues(alpha: 0.4);
    return Container(
      padding: const EdgeInsets.all(14),
      decoration: BoxDecoration(color: bg, borderRadius: BorderRadius.circular(10), border: Border.all(color: getThemeTextColor().withValues(alpha: 0.08))),
      child: Row(
        children: [
          Container(
            width: 34,
            height: 34,
            decoration: BoxDecoration(color: getThemeTextColor().withValues(alpha: 0.08), borderRadius: BorderRadius.circular(10)),
            child: Icon(icon, size: 18, color: getThemeTextColor()),
          ),
          const SizedBox(width: 12),
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text(title, style: TextStyle(color: getThemeSubTextColor(), fontSize: 12)),
                const SizedBox(height: 6),
                Text(value, style: TextStyle(color: getThemeTextColor(), fontSize: 20, fontWeight: FontWeight.w600)),
              ],
            ),
          ),
        ],
      ),
    );
  }

  Widget _barChart({required List<int> data, required List<String> labels, double height = 120}) {
    final maxValue = data.isEmpty ? 0 : data.reduce((a, b) => a > b ? a : b);
    final barColor = safeFromCssColor(WoxThemeUtil.instance.currentTheme.value.resultItemTitleColor);
    final bgLine = getThemeTextColor().withValues(alpha: 0.06);

    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 12),
      decoration: BoxDecoration(
        borderRadius: BorderRadius.circular(10),
        border: Border.all(color: getThemeTextColor().withValues(alpha: 0.08)),
        color: getThemeTextColor().withValues(alpha: 0.03),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          SizedBox(
            height: height,
            child: Row(
              crossAxisAlignment: CrossAxisAlignment.end,
              children: List.generate(data.length, (i) {
                final v = data[i];
                final ratio = maxValue == 0 ? 0.0 : v / maxValue;
                return Expanded(
                  child: Padding(
                    padding: const EdgeInsets.symmetric(horizontal: 2),
                    child: Container(
                      height: (height - 12) * ratio + 4,
                      decoration: BoxDecoration(color: barColor.withValues(alpha: 0.65), borderRadius: BorderRadius.circular(6), border: Border.all(color: bgLine)),
                    ),
                  ),
                );
              }),
            ),
          ),
          const SizedBox(height: 8),
          Row(
            children: List.generate(labels.length, (i) {
              return Expanded(child: Text(labels[i], textAlign: TextAlign.center, style: TextStyle(color: getThemeSubTextColor(), fontSize: 11), overflow: TextOverflow.ellipsis));
            }),
          ),
        ],
      ),
    );
  }

  Widget _topList({required String title, required List<WoxUsageStatsItem> items, required String emptyText}) {
    return Container(
      padding: const EdgeInsets.all(12),
      decoration: BoxDecoration(
        borderRadius: BorderRadius.circular(10),
        border: Border.all(color: getThemeTextColor().withValues(alpha: 0.08)),
        color: getThemeTextColor().withValues(alpha: 0.03),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text(title, style: TextStyle(color: getThemeTextColor(), fontSize: 13, fontWeight: FontWeight.w600)),
          const SizedBox(height: 8),
          if (items.isEmpty)
            Text(emptyText, style: TextStyle(color: getThemeSubTextColor(), fontSize: 12))
          else
            Column(
              children:
                  items.take(10).map((e) {
                    return Padding(
                      padding: const EdgeInsets.symmetric(vertical: 6),
                      child: Row(
                        children: [
                          Expanded(child: Text(e.name.isNotEmpty ? e.name : e.id, style: TextStyle(color: getThemeTextColor(), fontSize: 13), overflow: TextOverflow.ellipsis)),
                          const SizedBox(width: 12),
                          Text(e.count.toString(), style: TextStyle(color: getThemeSubTextColor(), fontSize: 12)),
                        ],
                      ),
                    );
                  }).toList(),
            ),
        ],
      ),
    );
  }

  Future<ui.Image> _captureShareCardImage(WoxUsageStats stats) async {
    final overlay = Overlay.of(context);
    final captureKey = GlobalKey();
    late final OverlayEntry entry;

    entry = OverlayEntry(
      builder: (context) {
        return Positioned(
          left: 0,
          top: 0,
          child: IgnorePointer(
            child: Opacity(
              opacity: 0.01,
              child: Material(type: MaterialType.transparency, child: RepaintBoundary(key: captureKey, child: WoxUsageShareCard(stats: stats, tr: controller.tr))),
            ),
          ),
        );
      },
    );

    overlay.insert(entry);
    try {
      // A temporary overlay gives the share card its own paint pass. Capturing the in-page hidden
      // widget was timing-sensitive because settings rebuilds and scroll containers can leave the
      // boundary dirty when the click handler tries to call toImage.
      RenderRepaintBoundary? boundary;
      for (var attempt = 0; attempt < 6; attempt++) {
        WidgetsBinding.instance.ensureVisualUpdate();
        await Future<void>.delayed(const Duration(milliseconds: 16));
        await WidgetsBinding.instance.endOfFrame;
        boundary = captureKey.currentContext?.findRenderObject() as RenderRepaintBoundary?;
        if (boundary != null && !boundary.debugNeedsPaint) {
          break;
        }
      }

      final readyBoundary = boundary;
      if (readyBoundary == null || readyBoundary.debugNeedsPaint) {
        throw StateError('Usage share card is not ready');
      }
      return readyBoundary.toImage(pixelRatio: 2);
    } finally {
      entry.remove();
    }
  }

  String _buildXShareText() {
    // Keep the compose text in i18n so the X draft follows the user's Wox language. The image
    // itself is still copied separately because X intent URLs cannot attach clipboard images.
    return controller.tr('ui_usage_share_tweet_text');
  }

  Future<void> _shareUsageToX(WoxUsageStats stats) async {
    final traceId = const UuidV4().generate();
    setState(() {
      _isSharingUsage = true;
      _shareStatusMessage = '';
      _shareStatusIsError = false;
    });

    try {
      // The share image is rendered through a temporary overlay so the PNG uses the polished
      // share-card layout without changing the visible settings page or inventing data.
      final image = await _captureShareCardImage(stats);
      final byteData = await image.toByteData(format: ui.ImageByteFormat.png);
      image.dispose();
      if (byteData == null) {
        throw StateError('Failed to encode usage share image');
      }

      final directory = await getTemporaryDirectory();
      final file = File('${directory.path}/wox-usage-share.png');
      await file.writeAsBytes(byteData.buffer.asUint8List(), flush: true);

      var statusMessage = controller.tr('ui_usage_share_success');
      var statusIsError = false;
      try {
        await ScreenshotPlatformBridge.instance.writeClipboardImageFile(filePath: file.path);
      } catch (e) {
        // Clipboard support is platform-specific. The generated image file is still useful, so the
        // share flow continues to X and reports a warning instead of failing the whole action.
        Logger.instance.warn(traceId, 'Usage share image generated but clipboard copy failed: $e');
        statusMessage = controller.tr('ui_usage_share_clipboard_unsupported');
      }

      final text = Uri.encodeQueryComponent(_buildXShareText());
      final uri = Uri.parse('https://x.com/intent/tweet?text=$text');
      final opened = await launchUrl(uri, mode: LaunchMode.externalApplication);
      if (!opened) {
        statusMessage = controller.tr('ui_usage_share_failed');
        statusIsError = true;
      }

      if (mounted) {
        setState(() {
          _shareStatusMessage = statusMessage;
          _shareStatusIsError = statusIsError;
        });
      }
    } catch (e) {
      Logger.instance.error(traceId, 'Failed to share usage stats to X: $e');
      if (mounted) {
        setState(() {
          _shareStatusMessage = '${controller.tr('ui_usage_share_failed')}: $e';
          _shareStatusIsError = true;
        });
      }
    } finally {
      if (mounted) {
        setState(() {
          _isSharingUsage = false;
        });
      }
    }
  }

  @override
  Widget build(BuildContext context) {
    return Obx(() {
      final isLoading = controller.isUsageStatsLoading.value;
      final error = controller.usageStatsError.value;
      final stats = controller.usageStats.value;

      final mostHour = stats.mostActiveHour < 0 ? '-' : '${stats.mostActiveHour.toString().padLeft(2, '0')}:00';
      final mostDay = stats.mostActiveDay < 0 ? '-' : _weekdayLabel(stats.mostActiveDay);

      return Stack(
        children: [
          _form(
            children: [
              Row(
                children: [
                  Text(controller.tr('ui_usage'), style: TextStyle(color: getThemeTextColor(), fontSize: 16, fontWeight: FontWeight.w600)),
                  const SizedBox(width: 12),
                  if (isLoading) const WoxLoadingIndicator(size: 16),
                  const Spacer(),
                  if (_isSharingUsage)
                    const WoxLoadingIndicator(size: 16)
                  else
                    WoxButton.text(
                      text: controller.tr('ui_usage_share_x'),
                      icon: Icon(Icons.ios_share, size: 15, color: getThemeTextColor()),
                      onPressed: isLoading ? null : () => _shareUsageToX(stats),
                    ),
                ],
              ),
              const SizedBox(height: 14),
              if (_shareStatusMessage.isNotEmpty)
                Padding(
                  padding: const EdgeInsets.only(bottom: 10),
                  child: Text(_shareStatusMessage, style: TextStyle(color: _shareStatusIsError ? Colors.red : getThemeSubTextColor(), fontSize: 12)),
                ),
              if (error.isNotEmpty) Padding(padding: const EdgeInsets.only(bottom: 10), child: Text(error, style: const TextStyle(color: Colors.red, fontSize: 12))),
              LayoutBuilder(
                builder: (context, constraints) {
                  final width = constraints.maxWidth.isFinite ? constraints.maxWidth : 960.0;
                  final spacing = 12.0;
                  final columns = width >= 760 ? 4 : 2;
                  final cardWidth = (width - (columns - 1) * spacing) / columns;

                  return Wrap(
                    spacing: spacing,
                    runSpacing: spacing,
                    children: [
                      SizedBox(width: cardWidth, child: _statCard(title: controller.tr('ui_usage_opened'), value: stats.totalOpened.toString(), icon: Icons.visibility_outlined)),
                      SizedBox(
                        width: cardWidth,
                        child: _statCard(title: controller.tr('ui_usage_app_launches'), value: stats.totalAppLaunch.toString(), icon: Icons.rocket_launch_outlined),
                      ),
                      SizedBox(width: cardWidth, child: _statCard(title: controller.tr('ui_usage_apps_used'), value: stats.totalAppsUsed.toString(), icon: Icons.apps_outlined)),
                      SizedBox(width: cardWidth, child: _statCard(title: controller.tr('ui_usage_actions'), value: stats.totalActions.toString(), icon: Icons.bolt_outlined)),
                    ],
                  );
                },
              ),
              const SizedBox(height: 18),
              LayoutBuilder(
                builder: (context, constraints) {
                  final width = constraints.maxWidth.isFinite ? constraints.maxWidth : 960.0;
                  final spacing = 12.0;
                  final columns = width >= 760 ? 2 : 1;
                  final blockWidth = columns == 1 ? width : (width - spacing) / columns;

                  final hourLabels = List<String>.generate(24, (i) => i % 6 == 0 ? i.toString().padLeft(2, '0') : '');
                  final weekdayLabels = List<String>.generate(7, (i) => _weekdayLabel(i));

                  return Wrap(
                    spacing: spacing,
                    runSpacing: spacing,
                    children: [
                      SizedBox(
                        width: blockWidth,
                        child: Column(
                          crossAxisAlignment: CrossAxisAlignment.start,
                          children: [
                            Text(controller.tr('ui_usage_opened_by_hour'), style: TextStyle(color: getThemeTextColor(), fontSize: 13, fontWeight: FontWeight.w600)),
                            const SizedBox(height: 8),
                            _barChart(data: stats.openedByHour, labels: hourLabels),
                            const SizedBox(height: 8),
                            Text('${controller.tr('ui_usage_most_active_hour')}: $mostHour', style: TextStyle(color: getThemeSubTextColor(), fontSize: 12)),
                          ],
                        ),
                      ),
                      SizedBox(
                        width: blockWidth,
                        child: Column(
                          crossAxisAlignment: CrossAxisAlignment.start,
                          children: [
                            Text(controller.tr('ui_usage_opened_by_weekday'), style: TextStyle(color: getThemeTextColor(), fontSize: 13, fontWeight: FontWeight.w600)),
                            const SizedBox(height: 8),
                            _barChart(data: stats.openedByWeekday, labels: weekdayLabels),
                            const SizedBox(height: 8),
                            Text('${controller.tr('ui_usage_most_active_day')}: $mostDay', style: TextStyle(color: getThemeSubTextColor(), fontSize: 12)),
                          ],
                        ),
                      ),
                    ],
                  );
                },
              ),
              const SizedBox(height: 18),
              LayoutBuilder(
                builder: (context, constraints) {
                  final width = constraints.maxWidth.isFinite ? constraints.maxWidth : 960.0;
                  final spacing = 12.0;
                  final columns = width >= 760 ? 2 : 1;
                  final blockWidth = columns == 1 ? width : (width - spacing) / columns;
                  final emptyText = controller.tr('ui_usage_no_data');

                  return Wrap(
                    spacing: spacing,
                    runSpacing: spacing,
                    children: [
                      SizedBox(width: blockWidth, child: _topList(title: controller.tr('ui_usage_top_apps'), items: stats.topApps, emptyText: emptyText)),
                      SizedBox(width: blockWidth, child: _topList(title: controller.tr('ui_usage_top_plugins'), items: stats.topPlugins, emptyText: emptyText)),
                    ],
                  );
                },
              ),
            ],
          ),
        ],
      );
    });
  }
}

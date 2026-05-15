import 'dart:async';
import 'dart:convert';
import 'dart:io';
import 'dart:math' as math;
import 'dart:ui' as ui;

import 'package:flutter/material.dart';
import 'package:flutter/rendering.dart';
import 'package:get/get.dart';
import 'package:path_provider/path_provider.dart';
import 'package:url_launcher/url_launcher.dart';
import 'package:uuid/v4.dart';
import 'package:wox/components/wox_image_view.dart';
import 'package:wox/components/wox_loading_indicator.dart';
import 'package:wox/components/wox_panel.dart';
import 'package:wox/components/wox_tooltip.dart';
import 'package:wox/controllers/wox_setting_controller.dart';
import 'package:wox/entity/wox_usage_stats.dart';
import 'package:wox/utils/colors.dart';
import 'package:wox/utils/consts.dart';
import 'package:wox/utils/log.dart';
import 'package:wox/utils/screenshot/screenshot_platform_bridge.dart';

class WoxSettingUsageView extends StatefulWidget {
  const WoxSettingUsageView({super.key});

  @override
  State<WoxSettingUsageView> createState() => _WoxSettingUsageViewState();
}

class _WoxSettingUsageViewState extends State<WoxSettingUsageView> {
  final WoxSettingController controller = Get.find<WoxSettingController>();
  late final MemoryImage _woxIconImage = MemoryImage(base64Decode(WOX_ICON.split(';base64,').last));
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

  Widget _form({double width = GENERAL_SETTING_COMPACT_FORM_WIDTH, required List<Widget> children}) {
    return Align(
      alignment: Alignment.topLeft,
      child: SingleChildScrollView(
        child: SizedBox(
          width: width,
          child: Padding(
            padding: const EdgeInsets.only(left: 38, right: 44, bottom: 30, top: 34),
            child: Column(crossAxisAlignment: CrossAxisAlignment.start, mainAxisSize: MainAxisSize.min, children: children),
          ),
        ),
      ),
    );
  }

  Color _pageBackgroundColor() {
    // The dashboard redesign originally used a fixed light analytics background, but settings tabs
    // must share the active theme's app background. Reading the theme here keeps Usage visually
    // aligned with every other settings tab while preserving the redesigned card layout above it.
    return getThemeBackgroundColor();
  }

  Color _panelColor() {
    if (isThemeDark()) {
      return getThemePanelBackgroundColor().lighter(4);
    }
    return Colors.white;
  }

  Color _outlineColor() {
    return isThemeDark() ? Colors.white.withValues(alpha: 0.08) : const Color(0xFFE6EAF0);
  }

  Color _shareImageBackgroundColor() {
    // The capture overlay must use an opaque color. Hiding the overlay with opacity previously made
    // the exported PNG translucent, which looked wrong after pasting into X or image viewers.
    return isThemeDark() ? const Color(0xFF202733) : const Color(0xFFF6F8FB);
  }

  Widget _shareButton({required bool disabled, required VoidCallback onPressed}) {
    final textColor = disabled ? getThemeSubTextColor().withValues(alpha: 0.55) : getThemeTextColor();
    return Material(
      color: Colors.transparent,
      child: InkWell(
        onTap: disabled ? null : onPressed,
        borderRadius: BorderRadius.circular(8),
        child: Container(
          padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 8),
          decoration: BoxDecoration(color: _panelColor(), borderRadius: BorderRadius.circular(8), border: Border.all(color: _outlineColor())),
          child: Row(
            mainAxisSize: MainAxisSize.min,
            children: [
              Icon(Icons.ios_share, size: 15, color: textColor),
              const SizedBox(width: 8),
              Text(controller.tr('ui_usage_share_x'), style: TextStyle(color: textColor, fontSize: 13, fontWeight: FontWeight.w600)),
            ],
          ),
        ),
      ),
    );
  }

  List<_UsagePeriodOption> get _periodOptions {
    return [
      _UsagePeriodOption(code: '7d', label: controller.tr('ui_usage_period_7d')),
      _UsagePeriodOption(code: '30d', label: controller.tr('ui_usage_period_30d')),
      _UsagePeriodOption(code: '365d', label: controller.tr('ui_usage_period_365d')),
      _UsagePeriodOption(code: 'all', label: controller.tr('ui_usage_period_all')),
    ];
  }

  String _periodLabel(String period) {
    for (final option in _periodOptions) {
      if (option.code == period) {
        return option.label;
      }
    }
    return controller.tr('ui_usage_period_30d');
  }

  String _overviewLabel(String period) {
    return controller.tr('ui_usage_overview').replaceAll('{period}', _periodLabel(period));
  }

  String _compareLabel(WoxUsageStats stats) {
    if (stats.period == 'all') {
      return controller.tr('ui_usage_period_all');
    }
    return controller.tr('ui_usage_compare_previous_period').replaceAll('{days}', stats.periodDays.toString());
  }

  _UsageTrend _buildUsageTrend({required double? changePercent, required int current, required int previous, required Color accentColor, bool allTime = false}) {
    if (allTime) {
      return _UsageTrend(label: controller.tr('ui_usage_metric_cumulative'), color: accentColor, icon: Icons.all_inclusive);
    }

    if (changePercent == null) {
      return _UsageTrend(label: controller.tr('ui_usage_metric_new'), color: accentColor, icon: Icons.trending_up);
    }

    if (changePercent.abs() < 0.5 || current == previous) {
      return _UsageTrend(label: controller.tr('ui_usage_metric_stable'), color: getThemeSubTextColor(), icon: Icons.remove);
    }

    final rounded = changePercent.abs().round();
    return _UsageTrend(
      label: '${changePercent > 0 ? '+' : '-'}$rounded%',
      color: changePercent > 0 ? accentColor : const Color(0xFFEF4444),
      icon: changePercent > 0 ? Icons.arrow_upward : Icons.arrow_downward,
    );
  }

  Widget _periodSelector({required String selectedPeriod, required bool disabled}) {
    return Container(
      padding: const EdgeInsets.all(3),
      decoration: BoxDecoration(color: _panelColor(), borderRadius: BorderRadius.circular(8), border: Border.all(color: _outlineColor())),
      child: Row(
        mainAxisSize: MainAxisSize.min,
        children:
            _periodOptions.map((option) {
              final selected = option.code == selectedPeriod;
              final textColor = selected ? getThemeTextColor() : getThemeSubTextColor();
              return Material(
                color: Colors.transparent,
                child: InkWell(
                  onTap: disabled || selected ? null : () => unawaited(controller.refreshUsageStats(period: option.code)),
                  borderRadius: BorderRadius.circular(6),
                  child: AnimatedContainer(
                    duration: const Duration(milliseconds: 120),
                    padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 6),
                    decoration: BoxDecoration(
                      // Period is a dashboard filter, not a navigation tab. A segmented control keeps the
                      // range choice close to the share action while avoiding a bulky dropdown that would
                      // hide the available reporting scopes.
                      color: selected ? getThemeActiveBackgroundColor().withValues(alpha: isThemeDark() ? 0.36 : 0.16) : Colors.transparent,
                      borderRadius: BorderRadius.circular(6),
                    ),
                    child: Text(
                      option.label,
                      style: TextStyle(
                        color: disabled && !selected ? textColor.withValues(alpha: 0.5) : textColor,
                        fontSize: 12,
                        fontWeight: selected ? FontWeight.w700 : FontWeight.w600,
                      ),
                    ),
                  ),
                ),
              );
            }).toList(),
      ),
    );
  }

  Widget _statCard({required String title, required String value, required IconData icon, required Color accentColor, required _UsageTrend trend, required String compareLabel}) {
    return WoxPanel(
      height: 116,
      padding: const EdgeInsets.all(14),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(
            children: [
              Container(
                width: 46,
                height: 46,
                decoration: BoxDecoration(color: accentColor.withValues(alpha: isThemeDark() ? 0.22 : 0.12), borderRadius: BorderRadius.circular(8)),
                child: Icon(icon, size: 22, color: accentColor),
              ),
              const SizedBox(width: 12),
              Expanded(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  mainAxisAlignment: MainAxisAlignment.center,
                  children: [
                    Text(title, style: TextStyle(color: getThemeSubTextColor(), fontSize: 12, fontWeight: FontWeight.w600), overflow: TextOverflow.ellipsis),
                    const SizedBox(height: 6),
                    Text(value, style: TextStyle(color: getThemeTextColor(), fontSize: 22, fontWeight: FontWeight.w700, height: 1.0)),
                  ],
                ),
              ),
            ],
          ),
          const Spacer(),
          // The comparison row belongs to the whole KPI card, not just the text column. Placing it
          // at the bottom separates auxiliary trend context from the primary icon/value content and
          // gives localized labels the full card width without truncating common English text.
          Row(
            children: [
              Container(
                padding: const EdgeInsets.symmetric(horizontal: 7, vertical: 3),
                decoration: BoxDecoration(color: trend.color.withValues(alpha: isThemeDark() ? 0.16 : 0.09), borderRadius: BorderRadius.circular(6)),
                child: Row(
                  mainAxisSize: MainAxisSize.min,
                  children: [
                    Icon(trend.icon, size: 10, color: trend.color),
                    const SizedBox(width: 3),
                    Text(trend.label, style: TextStyle(color: trend.color, fontSize: 10, fontWeight: FontWeight.w700)),
                  ],
                ),
              ),
              const SizedBox(width: 8),
              Expanded(
                child: SizedBox(
                  height: 16,
                  child: FittedBox(
                    alignment: Alignment.centerLeft,
                    fit: BoxFit.scaleDown,
                    child: Text(compareLabel, style: TextStyle(color: getThemeSubTextColor(), fontSize: 11, fontWeight: FontWeight.w500), maxLines: 1),
                  ),
                ),
              ),
            ],
          ),
        ],
      ),
    );
  }

  Widget _dashboardPanel({required String title, required IconData icon, required Widget child, String? footer}) {
    return WoxPanel(
      padding: const EdgeInsets.all(16),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(
            children: [
              Icon(icon, size: 16, color: getThemeTextColor()),
              const SizedBox(width: 8),
              Expanded(child: Text(title, style: TextStyle(color: getThemeTextColor(), fontSize: 14, fontWeight: FontWeight.w700), overflow: TextOverflow.ellipsis)),
            ],
          ),
          const SizedBox(height: 14),
          child,
          if (footer != null) ...[const SizedBox(height: 12), Text(footer, style: TextStyle(color: getThemeSubTextColor(), fontSize: 12, fontWeight: FontWeight.w500))],
        ],
      ),
    );
  }

  Widget _barChart({
    required List<int> data,
    required List<String> labels,
    required List<String> tooltipLabels,
    required int highlightIndex,
    required Color accentColor,
    double height = 150,
  }) {
    final maxValue = data.isEmpty ? 0 : data.reduce(math.max);
    final axisMax = _chartAxisMax(maxValue);
    final tickValues = List<int>.generate(5, (i) => (axisMax - axisMax * i / 4).round());
    final neutralBar = getThemeSubTextColor().withValues(alpha: isThemeDark() ? 0.42 : 0.30);
    final gridColor = getThemeTextColor().withValues(alpha: isThemeDark() ? 0.09 : 0.055);

    // Chart rendering stays widget-based instead of introducing a painter so the setting page keeps
    // straightforward layout behavior and still gains modern gridlines, rounded bars, and active
    // bucket highlighting from the real usage summary. Hover state is local to the chart so bars can
    // expose exact values without adding global dashboard state or changing the API shape again.
    int? hoveredIndex;
    return StatefulBuilder(
      builder: (context, setChartState) {
        return Column(
          children: [
            Row(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                // The first dashboard pass drew the bars without a y-axis, which made hover values
                // the only way to understand scale. A compact axis keeps the chart readable at a
                // glance while staying inside the existing settings card layout.
                SizedBox(
                  width: 28,
                  height: height,
                  child: Column(
                    mainAxisAlignment: MainAxisAlignment.spaceBetween,
                    crossAxisAlignment: CrossAxisAlignment.end,
                    children:
                        tickValues.map((value) {
                          return Text(_formatAxisValue(value), style: TextStyle(color: getThemeSubTextColor(), fontSize: 10, fontWeight: FontWeight.w500));
                        }).toList(),
                  ),
                ),
                const SizedBox(width: 8),
                Expanded(
                  child: SizedBox(
                    height: height,
                    child: Stack(
                      children: [
                        Positioned.fill(
                          child: Column(mainAxisAlignment: MainAxisAlignment.spaceBetween, children: List.generate(5, (_) => Container(height: 1, color: gridColor))),
                        ),
                        Positioned.fill(
                          child: Row(
                            crossAxisAlignment: CrossAxisAlignment.end,
                            children: List.generate(data.length, (i) {
                              final value = data[i];
                              final ratio = axisMax == 0 ? 0.0 : value / axisMax;
                              final barHeight = value == 0 ? 5.0 : math.max(8.0, (height - 10) * ratio);
                              final isActive = (hoveredIndex ?? highlightIndex) == i && value > 0;
                              final tooltipLabel = i < tooltipLabels.length ? tooltipLabels[i] : labels[i];
                              return Expanded(
                                // Usage chart bars keep their short hover delay, but
                                // render through WoxTooltip so chart hints match other
                                // text overlays and remain selectable when needed.
                                child: WoxTooltip(
                                  message: '$tooltipLabel · $value',
                                  waitDuration: const Duration(milliseconds: 180),
                                  child: MouseRegion(
                                    cursor: SystemMouseCursors.click,
                                    onEnter: (_) => setChartState(() => hoveredIndex = i),
                                    onExit: (_) => setChartState(() => hoveredIndex = null),
                                    child: Padding(
                                      padding: EdgeInsets.symmetric(horizontal: data.length > 12 ? 3 : 6),
                                      child: Align(
                                        alignment: Alignment.bottomCenter,
                                        child: AnimatedContainer(
                                          duration: const Duration(milliseconds: 140),
                                          height: barHeight,
                                          decoration: BoxDecoration(color: isActive ? accentColor : neutralBar, borderRadius: BorderRadius.circular(7)),
                                        ),
                                      ),
                                    ),
                                  ),
                                ),
                              );
                            }),
                          ),
                        ),
                      ],
                    ),
                  ),
                ),
              ],
            ),
            const SizedBox(height: 10),
            Row(
              children: [
                const SizedBox(width: 36),
                Expanded(
                  child: Row(
                    children: List.generate(labels.length, (i) {
                      final isActive = (hoveredIndex ?? highlightIndex) == i && (data.isNotEmpty && i < data.length && data[i] > 0);
                      return Expanded(
                        child: Text(
                          labels[i],
                          textAlign: TextAlign.center,
                          style: TextStyle(color: isActive ? accentColor : getThemeSubTextColor(), fontSize: 11, fontWeight: isActive ? FontWeight.w700 : FontWeight.w500),
                          overflow: TextOverflow.ellipsis,
                        ),
                      );
                    }),
                  ),
                ),
              ],
            ),
          ],
        );
      },
    );
  }

  int _chartAxisMax(int maxValue) {
    if (maxValue <= 0) {
      return 4;
    }

    final magnitude = math.pow(10, maxValue.toString().length - 1).toInt();
    for (final factor in const [1, 2, 5, 10]) {
      final candidate = magnitude * factor;
      if (candidate >= maxValue) {
        return candidate;
      }
    }
    return magnitude * 10;
  }

  String _formatAxisValue(int value) {
    if (value >= 1000 && value % 1000 == 0) {
      return '${value ~/ 1000}k';
    }
    return value.toString();
  }

  Widget _itemIcon(WoxUsageStatsItem item, Color accentColor) {
    if (item.icon.imageData.isNotEmpty) {
      return ClipRRect(borderRadius: BorderRadius.circular(4), child: WoxImageView(woxImage: item.icon, width: 18, height: 18));
    }

    return Container(
      width: 18,
      height: 18,
      decoration: BoxDecoration(color: accentColor.withValues(alpha: isThemeDark() ? 0.18 : 0.10), borderRadius: BorderRadius.circular(4)),
      child: Icon(Icons.apps_outlined, size: 13, color: accentColor),
    );
  }

  Widget _topList({
    required String title,
    required IconData titleIcon,
    required List<WoxUsageStatsItem> items,
    required String emptyText,
    required Color accentColor,
    bool showItemIcons = false,
  }) {
    final maxCount = items.isEmpty ? 0 : items.map((e) => e.count).reduce(math.max);

    return _dashboardPanel(
      title: title,
      icon: titleIcon,
      child:
          items.isEmpty
              ? SizedBox(height: 72, child: Align(alignment: Alignment.centerLeft, child: Text(emptyText, style: TextStyle(color: getThemeSubTextColor(), fontSize: 12))))
              : Column(
                children:
                    items.take(10).toList().asMap().entries.map((entry) {
                      final rank = entry.key + 1;
                      final e = entry.value;
                      final name = e.name.isNotEmpty ? e.name : e.id;
                      final progress = maxCount == 0 ? 0.0 : e.count / maxCount;
                      return Padding(
                        padding: const EdgeInsets.symmetric(vertical: 5),
                        child: SizedBox(
                          height: 24,
                          child: Row(
                            children: [
                              SizedBox(
                                width: 24,
                                child: Text(
                                  _rankLabel(rank),
                                  textAlign: TextAlign.center,
                                  style: TextStyle(color: getThemeSubTextColor(), fontSize: rank <= 3 ? 14 : 12, fontWeight: FontWeight.w600),
                                ),
                              ),
                              if (showItemIcons) ...[_itemIcon(e, accentColor), const SizedBox(width: 8)],
                              Expanded(
                                flex: 5,
                                child: Text(name, style: TextStyle(color: getThemeTextColor(), fontSize: 13, fontWeight: FontWeight.w500), overflow: TextOverflow.ellipsis),
                              ),
                              const SizedBox(width: 12),
                              // The accepted design puts ranking text on the left and uses a thin
                              // progress meter on the right. Keeping progress out of the row
                              // background prevents long app names from fighting with colored bars.
                              Expanded(
                                flex: 4,
                                child: ClipRRect(
                                  borderRadius: BorderRadius.circular(99),
                                  child: Stack(
                                    children: [
                                      Container(
                                        height: 3,
                                        margin: const EdgeInsets.symmetric(vertical: 10.5),
                                        color: getThemeTextColor().withValues(alpha: isThemeDark() ? 0.08 : 0.055),
                                      ),
                                      FractionallySizedBox(
                                        widthFactor: progress.clamp(0.0, 1.0),
                                        child: Container(
                                          height: 3,
                                          margin: const EdgeInsets.symmetric(vertical: 10.5),
                                          decoration: BoxDecoration(color: accentColor.withValues(alpha: isThemeDark() ? 0.72 : 0.68), borderRadius: BorderRadius.circular(99)),
                                        ),
                                      ),
                                    ],
                                  ),
                                ),
                              ),
                              const SizedBox(width: 10),
                              SizedBox(
                                width: 32,
                                child: Text(
                                  e.count.toString(),
                                  textAlign: TextAlign.right,
                                  style: TextStyle(color: getThemeSubTextColor(), fontSize: 12, fontWeight: FontWeight.w700),
                                  overflow: TextOverflow.ellipsis,
                                ),
                              ),
                            ],
                          ),
                        ),
                      );
                    }).toList(),
              ),
    );
  }

  String _rankLabel(int rank) {
    switch (rank) {
      case 1:
        return '🏆';
      case 2:
        return '🥈';
      case 3:
        return '🥉';
      default:
        return rank.toString();
    }
  }

  String _shareImageTitle() {
    return controller.tr('ui_usage_share_image_title');
  }

  Widget _usageDashboardBody({required WoxUsageStats stats, required String selectedPeriod, required String error}) {
    final allTime = stats.period == 'all' || selectedPeriod == 'all';
    final mostHour = stats.mostActiveHour < 0 ? '-' : '${stats.mostActiveHour.toString().padLeft(2, '0')}:00';
    final mostDay = stats.mostActiveDay < 0 ? '-' : _weekdayLabel(stats.mostActiveDay);
    const blueAccent = Color(0xFF3B82F6);
    const tealAccent = Color(0xFF14B8A6);
    const amberAccent = Color(0xFFF59E0B);
    const violetAccent = Color(0xFF8B5CF6);
    // KPI cards can keep their semantic accents, but the lower analytics panels need a calmer
    // reading rhythm. Reusing blue for app/time panels and violet for plugin panels matches the
    // accepted mockup more closely than giving every card its own dominant color.
    const appPanelAccent = blueAccent;
    const pluginPanelAccent = violetAccent;
    final compareLabel = _compareLabel(stats);

    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      mainAxisSize: MainAxisSize.min,
      children: [
        if (error.isNotEmpty) Padding(padding: const EdgeInsets.only(bottom: 10), child: Text(error, style: const TextStyle(color: Colors.red, fontSize: 12))),
        LayoutBuilder(
          builder: (context, constraints) {
            final width = constraints.maxWidth.isFinite ? constraints.maxWidth : GENERAL_SETTING_COMPACT_FORM_WIDTH;
            final spacing = 12.0;
            final columns = width >= 760 ? 4 : 2;
            final cardWidth = (width - (columns - 1) * spacing) / columns;

            return Wrap(
              spacing: spacing,
              runSpacing: spacing,
              children: [
                SizedBox(
                  width: cardWidth,
                  child: _statCard(
                    title: controller.tr('ui_usage_opened'),
                    value: stats.periodOpened.toString(),
                    icon: Icons.visibility_outlined,
                    accentColor: blueAccent,
                    trend: _buildUsageTrend(
                      changePercent: stats.openedChangePercent,
                      current: stats.periodOpened,
                      previous: stats.previousPeriodOpened,
                      accentColor: blueAccent,
                      allTime: allTime,
                    ),
                    compareLabel: compareLabel,
                  ),
                ),
                SizedBox(
                  width: cardWidth,
                  child: _statCard(
                    title: controller.tr('ui_usage_app_launches'),
                    value: stats.periodAppLaunch.toString(),
                    icon: Icons.rocket_launch_outlined,
                    accentColor: tealAccent,
                    trend: _buildUsageTrend(
                      changePercent: stats.appLaunchChangePercent,
                      current: stats.periodAppLaunch,
                      previous: stats.previousPeriodAppLaunch,
                      accentColor: tealAccent,
                      allTime: allTime,
                    ),
                    compareLabel: compareLabel,
                  ),
                ),
                SizedBox(
                  width: cardWidth,
                  child: _statCard(
                    title: controller.tr('ui_usage_apps_used'),
                    value: stats.periodAppsUsed.toString(),
                    icon: Icons.apps_outlined,
                    accentColor: amberAccent,
                    trend: _buildUsageTrend(
                      changePercent: stats.appsUsedChangePercent,
                      current: stats.periodAppsUsed,
                      previous: stats.previousPeriodAppsUsed,
                      accentColor: amberAccent,
                      allTime: allTime,
                    ),
                    compareLabel: compareLabel,
                  ),
                ),
                SizedBox(
                  width: cardWidth,
                  child: _statCard(
                    title: controller.tr('ui_usage_actions'),
                    value: stats.periodActions.toString(),
                    icon: Icons.bolt_outlined,
                    accentColor: violetAccent,
                    trend: _buildUsageTrend(
                      changePercent: stats.actionsChangePercent,
                      current: stats.periodActions,
                      previous: stats.previousPeriodActions,
                      accentColor: violetAccent,
                      allTime: allTime,
                    ),
                    compareLabel: compareLabel,
                  ),
                ),
              ],
            );
          },
        ),
        const SizedBox(height: 18),
        LayoutBuilder(
          builder: (context, constraints) {
            final width = constraints.maxWidth.isFinite ? constraints.maxWidth : GENERAL_SETTING_COMPACT_FORM_WIDTH;
            final spacing = 12.0;
            final columns = width >= 760 ? 2 : 1;
            final blockWidth = columns == 1 ? width : (width - spacing) / columns;

            final hourLabels = List<String>.generate(24, (i) => i % 6 == 0 ? i.toString().padLeft(2, '0') : '');
            final hourTooltipLabels = List<String>.generate(24, (i) => '${i.toString().padLeft(2, '0')}:00');
            final weekdayLabels = List<String>.generate(7, (i) => _weekdayLabel(i));

            return Wrap(
              spacing: spacing,
              runSpacing: spacing,
              children: [
                SizedBox(
                  width: blockWidth,
                  child: _dashboardPanel(
                    title: controller.tr('ui_usage_opened_by_hour'),
                    icon: Icons.schedule_outlined,
                    child: _barChart(data: stats.openedByHour, labels: hourLabels, tooltipLabels: hourTooltipLabels, highlightIndex: stats.mostActiveHour, accentColor: blueAccent),
                    footer: '${controller.tr('ui_usage_most_active_hour')} · $mostHour',
                  ),
                ),
                SizedBox(
                  width: blockWidth,
                  child: _dashboardPanel(
                    title: controller.tr('ui_usage_opened_by_weekday'),
                    icon: Icons.calendar_today_outlined,
                    child: _barChart(
                      data: stats.openedByWeekday,
                      labels: weekdayLabels,
                      tooltipLabels: weekdayLabels,
                      highlightIndex: stats.mostActiveDay,
                      accentColor: appPanelAccent,
                    ),
                    footer: '${controller.tr('ui_usage_most_active_day')} · $mostDay',
                  ),
                ),
              ],
            );
          },
        ),
        const SizedBox(height: 18),
        LayoutBuilder(
          builder: (context, constraints) {
            final width = constraints.maxWidth.isFinite ? constraints.maxWidth : GENERAL_SETTING_COMPACT_FORM_WIDTH;
            final spacing = 12.0;
            final columns = width >= 760 ? 2 : 1;
            final blockWidth = columns == 1 ? width : (width - spacing) / columns;
            final emptyText = controller.tr('ui_usage_no_data');

            return Wrap(
              spacing: spacing,
              runSpacing: spacing,
              children: [
                SizedBox(
                  width: blockWidth,
                  child: _topList(
                    title: controller.tr('ui_usage_top_apps'),
                    titleIcon: Icons.apps_outlined,
                    items: stats.topApps,
                    emptyText: emptyText,
                    accentColor: appPanelAccent,
                    showItemIcons: true,
                  ),
                ),
                SizedBox(
                  width: blockWidth,
                  child: _topList(
                    title: controller.tr('ui_usage_top_plugins'),
                    titleIcon: Icons.extension_outlined,
                    items: stats.topPlugins,
                    emptyText: emptyText,
                    accentColor: pluginPanelAccent,
                  ),
                ),
              ],
            );
          },
        ),
      ],
    );
  }

  Widget _usageSummaryHeader({required bool isLoading, required bool disableShare, required String selectedPeriod}) {
    final titleBlock = Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Row(
          mainAxisSize: MainAxisSize.min,
          children: [
            Text(controller.tr('ui_usage'), style: TextStyle(color: getThemeTextColor(), fontSize: 21, fontWeight: FontWeight.w800, height: 1.1)),
            if (isLoading) ...[const SizedBox(width: 10), const WoxLoadingIndicator(size: 16)],
          ],
        ),
        const SizedBox(height: 6),
        Text(_overviewLabel(selectedPeriod), style: TextStyle(color: getThemeSubTextColor(), fontSize: 13, fontWeight: FontWeight.w500)),
      ],
    );

    final shareAction =
        _isSharingUsage ? const Padding(padding: EdgeInsets.only(top: 6), child: WoxLoadingIndicator(size: 16)) : _shareButton(disabled: disableShare, onPressed: _shareUsageToX);
    final periodSelector = _periodSelector(selectedPeriod: selectedPeriod, disabled: isLoading);

    // The period selector is a page-level filter, not part of the share action. It stays centered in
    // the whole header instead of participating in a Wrap; the previous width threshold was too
    // conservative and pushed the selector onto a second row on normal settings-window sizes.
    return SizedBox(
      height: 54,
      child: Stack(
        alignment: Alignment.topCenter,
        children: [
          Align(alignment: Alignment.topLeft, child: titleBlock),
          Align(alignment: Alignment.topCenter, child: periodSelector),
          Align(alignment: Alignment.topRight, child: shareAction),
        ],
      ),
    );
  }

  Widget _usageShareHeader({required String selectedPeriod}) {
    // The share image header is rendered only in the temporary capture overlay. The visible settings
    // page keeps its regular title/subtitle, while the exported image gets a more editorial header.
    return SizedBox(
      width: double.infinity,
      height: 72,
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                // Keep the logo in the brand line instead of beside the whole title stack. The
                // previous lockup made the large title start after the icon, which looked visually
                // offset even though the widget edges were technically aligned.
                Row(
                  children: [
                    ClipRRect(borderRadius: BorderRadius.circular(8), child: Image(image: _woxIconImage, width: 30, height: 30, fit: BoxFit.cover)),
                    const SizedBox(width: 10),
                    Text('Wox Launcher', style: TextStyle(color: getThemeSubTextColor(), fontSize: 11, fontWeight: FontWeight.w800, letterSpacing: 1.1)),
                  ],
                ),
                const SizedBox(height: 9),
                Text(
                  _shareImageTitle(),
                  style: TextStyle(color: getThemeTextColor(), fontSize: 30, fontWeight: FontWeight.w900, height: 1.0),
                  maxLines: 1,
                  overflow: TextOverflow.ellipsis,
                ),
              ],
            ),
          ),
          const SizedBox(width: 20),
          // A small period pill gives the top-right corner useful context without competing with
          // the Wox logo. The previous card-like badge was visually heavier than the report title.
          Container(
            padding: const EdgeInsets.symmetric(horizontal: 11, vertical: 7),
            decoration: BoxDecoration(color: _panelColor().withValues(alpha: 0.72), borderRadius: BorderRadius.circular(999), border: Border.all(color: _outlineColor())),
            child: Row(
              mainAxisSize: MainAxisSize.min,
              children: [
                Icon(Icons.calendar_today_outlined, size: 13, color: getThemeSubTextColor()),
                const SizedBox(width: 7),
                Text(_periodLabel(selectedPeriod), style: TextStyle(color: getThemeSubTextColor(), fontSize: 12, fontWeight: FontWeight.w800)),
              ],
            ),
          ),
        ],
      ),
    );
  }

  Future<ui.Image> _captureUsagePageImage() async {
    final overlay = Overlay.of(context);
    final captureKey = GlobalKey();
    final selectedPeriod = controller.usageStatsPeriod.value;
    final stats = controller.usageStats.value;
    final error = controller.usageStatsError.value;
    late final OverlayEntry entry;

    // The capture overlay is rendered and read back immediately. Precache the bundled icon first so
    // the share image does not grab a frame where the logo slot has layout but no decoded pixels yet.
    await precacheImage(_woxIconImage, context);

    entry = OverlayEntry(
      builder: (context) {
        return Positioned(
          left: -10000,
          top: -10000,
          child: IgnorePointer(
            child: Material(
              type: MaterialType.transparency,
              child: RepaintBoundary(
                key: captureKey,
                child: SizedBox(
                  width: GENERAL_SETTING_WIDE_FORM_WIDTH,
                  child: ColoredBox(
                    color: _shareImageBackgroundColor(),
                    // The exported image needs breathing room on every edge; the visible settings
                    // page keeps its dense layout, while only the capture overlay gets this padding.
                    child: Padding(
                      padding: const EdgeInsets.all(32),
                      child: Column(
                        crossAxisAlignment: CrossAxisAlignment.start,
                        mainAxisSize: MainAxisSize.min,
                        children: [
                          _usageShareHeader(selectedPeriod: selectedPeriod),
                          const SizedBox(height: 22),
                          _usageDashboardBody(stats: stats, selectedPeriod: selectedPeriod, error: error),
                        ],
                      ),
                    ),
                  ),
                ),
              ),
            ),
          ),
        );
      },
    );

    overlay.insert(entry);
    try {
      // The capture target is hidden in an overlay so the screenshot can have a share-only title and
      // outer padding without mutating the visible settings page or capturing interactive controls.
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
        throw StateError('Usage page is not ready for sharing');
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

  Future<void> _shareUsageToX() async {
    final traceId = const UuidV4().generate();
    setState(() {
      _isSharingUsage = true;
      _shareStatusMessage = '';
      _shareStatusIsError = false;
    });

    try {
      // Exporting the live dashboard preserves the user's selected period and current theme. The
      // previous off-screen share card was visually disconnected from the page the user chose to
      // share, so the capture now targets the dashboard body directly.
      final image = await _captureUsagePageImage();
      final byteData = await image.toByteData(format: ui.ImageByteFormat.png);
      image.dispose();
      if (byteData == null) {
        throw StateError('Failed to encode usage share image');
      }

      final directory = await getTemporaryDirectory();
      final file = File('${directory.path}/wox-usage-share.png');
      await file.writeAsBytes(byteData.buffer.asUint8List(), flush: true);

      var statusMessage = '';
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
      final selectedPeriod = controller.usageStatsPeriod.value;

      return Container(
        color: _pageBackgroundColor(),
        child: Stack(
          children: [
            _form(
              width: GENERAL_SETTING_WIDE_FORM_WIDTH,
              children: [
                if (_shareStatusMessage.isNotEmpty)
                  Padding(
                    padding: const EdgeInsets.only(bottom: 10),
                    child: Text(_shareStatusMessage, style: TextStyle(color: _shareStatusIsError ? Colors.red : getThemeSubTextColor(), fontSize: 12)),
                  ),
                // Normal settings view keeps its original title and subtitle. The share-only title
                // and outer padding are rendered in a temporary overlay when the user clicks Share.
                _usageSummaryHeader(isLoading: isLoading, disableShare: isLoading, selectedPeriod: selectedPeriod),
                const SizedBox(height: 18),
                _usageDashboardBody(stats: stats, selectedPeriod: selectedPeriod, error: error),
              ],
            ),
          ],
        ),
      );
    });
  }
}

class _UsageTrend {
  const _UsageTrend({required this.label, required this.color, required this.icon});

  final String label;
  final Color color;
  final IconData icon;
}

class _UsagePeriodOption {
  const _UsagePeriodOption({required this.code, required this.label});

  final String code;
  final String label;
}

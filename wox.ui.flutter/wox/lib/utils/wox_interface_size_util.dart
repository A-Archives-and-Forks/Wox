import 'package:get/get.dart';
import 'package:wox/utils/consts.dart';

class WoxInterfaceSizeMetrics {
  final String density;
  final double scale;
  final double resultTitleFontSize;
  final double resultSubtitleFontSize;
  final double tailHotkeyFontSize;
  final double resultIconSize;
  final double tailImageSize;
  final double quickSelectSize;
  final double resultItemBaseHeight;
  final double queryBoxBaseHeight;
  final double queryBoxFontSize;
  final double queryBoxIconSize;
  final double toolbarHeight;
  final double toolbarFontSize;
  final double toolbarIconSize;
  final double actionItemBaseHeight;
  final double actionHeaderFontSize;

  const WoxInterfaceSizeMetrics({
    required this.density,
    required this.scale,
    required this.resultTitleFontSize,
    required this.resultSubtitleFontSize,
    required this.tailHotkeyFontSize,
    required this.resultIconSize,
    required this.tailImageSize,
    required this.quickSelectSize,
    required this.resultItemBaseHeight,
    required this.queryBoxBaseHeight,
    required this.queryBoxFontSize,
    required this.queryBoxIconSize,
    required this.toolbarHeight,
    required this.toolbarFontSize,
    required this.toolbarIconSize,
    required this.actionItemBaseHeight,
    required this.actionHeaderFontSize,
  });

  factory WoxInterfaceSizeMetrics.fromDensity(String value) {
    final density = WoxInterfaceSizeUtil.normalizeDensity(value);
    final scale = switch (density) {
      WoxInterfaceSizeUtil.compact => 0.875,
      WoxInterfaceSizeUtil.comfortable => 1.125,
      _ => 1.0,
    };

    double scaled(double base) => (base * scale).roundToDouble();

    return WoxInterfaceSizeMetrics(
      density: density,
      scale: scale,
      resultTitleFontSize: scaled(16),
      resultSubtitleFontSize: scaled(13),
      tailHotkeyFontSize: scaled(11),
      resultIconSize: scaled(30),
      tailImageSize: scaled(20),
      quickSelectSize: scaled(24),
      resultItemBaseHeight: scaled(RESULT_ITEM_BASE_HEIGHT),
      queryBoxBaseHeight: scaled(QUERY_BOX_BASE_HEIGHT),
      queryBoxFontSize: scaled(28),
      queryBoxIconSize: scaled(30),
      toolbarHeight: scaled(TOOLBAR_HEIGHT),
      toolbarFontSize: scaled(12),
      toolbarIconSize: scaled(24),
      actionItemBaseHeight: scaled(ACTION_ITEM_BASE_HEIGHT),
      actionHeaderFontSize: scaled(16),
    );
  }

  double scaledSpacing(double base) => (base * scale).roundToDouble();

  double get queryBoxLineHeight => queryBoxBaseHeight - QUERY_BOX_CONTENT_PADDING_TOP - QUERY_BOX_CONTENT_PADDING_BOTTOM;
}

class WoxInterfaceSizeUtil {
  static const compact = 'compact';
  static const normal = 'normal';
  static const comfortable = 'comfortable';

  WoxInterfaceSizeUtil._privateConstructor();

  static final WoxInterfaceSizeUtil _instance = WoxInterfaceSizeUtil._privateConstructor();

  static WoxInterfaceSizeUtil get instance => _instance;

  final Rx<WoxInterfaceSizeMetrics> metrics = WoxInterfaceSizeMetrics.fromDensity(normal).obs;

  WoxInterfaceSizeMetrics get current => metrics.value;

  static String normalizeDensity(String value) {
    switch (value.trim().toLowerCase()) {
      case compact:
        return compact;
      case comfortable:
        return comfortable;
      default:
        return normal;
    }
  }

  void refreshFromDensity(String density) {
    final nextMetrics = WoxInterfaceSizeMetrics.fromDensity(density);
    if (nextMetrics.density == metrics.value.density) {
      return;
    }

    // Density changes affect launcher-only measurements. Keeping the metrics
    // in one observable avoids adding per-size fields to the backend DTO while
    // still letting the launcher rebuild and resize immediately after reload.
    metrics.value = nextMetrics;
  }
}

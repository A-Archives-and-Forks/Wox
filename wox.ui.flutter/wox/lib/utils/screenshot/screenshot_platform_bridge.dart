import 'dart:io';

import 'package:flutter/services.dart';
import 'package:wox/entity/screenshot_session.dart';

abstract class ScreenshotPlatformBridge {
  static ScreenshotPlatformBridge _instance =
      MethodChannelScreenshotPlatformBridge();

  static ScreenshotPlatformBridge get instance => _instance;

  static void setInstanceForTest(ScreenshotPlatformBridge bridge) {
    _instance = bridge;
  }

  static void resetInstance() {
    _instance = MethodChannelScreenshotPlatformBridge();
  }

  Future<List<DisplaySnapshot>> captureAllDisplays();
}

class MethodChannelScreenshotPlatformBridge
    implements ScreenshotPlatformBridge {
  static const String _windowsChannelName = 'com.wox.windows_window_manager';
  static const String _macosChannelName = 'com.wox.macos_window_manager';
  static const String _linuxChannelName = 'com.wox.linux_window_manager';

  late final MethodChannel _channel = MethodChannel(_resolveChannelName());

  String _resolveChannelName() {
    if (Platform.isWindows) {
      return _windowsChannelName;
    }
    if (Platform.isMacOS) {
      return _macosChannelName;
    }
    if (Platform.isLinux) {
      return _linuxChannelName;
    }
    throw UnsupportedError('Unsupported platform: ${Platform.operatingSystem}');
  }

  @override
  Future<List<DisplaySnapshot>> captureAllDisplays() async {
    final response = await _channel.invokeMethod<List<dynamic>>(
      'captureAllDisplays',
    );
    final snapshots = response ?? const <dynamic>[];

    // The native bridge returns JSON-like maps so Flutter can keep the screenshot session platform-agnostic.
    return snapshots.whereType<Map<dynamic, dynamic>>().map((item) {
      return DisplaySnapshot.fromJson(
        item.map((key, value) => MapEntry(key.toString(), value)),
      );
    }).toList();
  }
}

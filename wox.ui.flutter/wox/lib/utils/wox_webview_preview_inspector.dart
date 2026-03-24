import 'dart:io';

import 'package:flutter/services.dart';

class WoxWebViewPreviewInspector {
  static const MethodChannel _channel = MethodChannel('com.wox.webview_preview');

  static Future<bool> openInspector() async {
    if (!Platform.isMacOS) {
      return false;
    }

    final result = await _channel.invokeMethod<bool>('openInspector');
    return result ?? false;
  }
}

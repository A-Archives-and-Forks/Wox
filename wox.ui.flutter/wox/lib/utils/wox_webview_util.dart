import 'dart:io';

import 'package:flutter/services.dart';

class WoxWebViewUtil {
  static const MethodChannel _channel = MethodChannel('com.wox.webview_preview');

  static Future<bool> openInspector() async {
    return _invoke('openInspector');
  }

  static Future<bool> refresh() async {
    return _invoke('refresh');
  }

  static Future<bool> goBack() async {
    return _invoke('goBack');
  }

  static Future<bool> goForward() async {
    return _invoke('goForward');
  }

  static Future<bool> _invoke(String method) async {
    if (!Platform.isMacOS) {
      return false;
    }

    final result = await _channel.invokeMethod<bool>(method);
    return result ?? false;
  }
}

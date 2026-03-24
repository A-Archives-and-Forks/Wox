import 'dart:io';

import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:path_provider/path_provider.dart';
import 'package:webview_windows/webview_windows.dart';
import 'package:wox/entity/wox_preview_webview_data.dart';

class WoxWebViewUtil {
  static const MethodChannel _channel = MethodChannel(
    'com.wox.webview_preview',
  );
  static const String mobileUserAgent =
      "Mozilla/5.0 (iPhone; CPU iPhone OS 18_7_6 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/26.0 Mobile/15E148 Safari/604.1";

  static final Map<String, WoxWindowsWebViewSession> _cachedWindowsSessions =
      {};

  static WebviewController? _activeWindowsController;
  static Future<void>? _windowsEnvironmentInitialization;
  static bool _windowsRuntimeChecked = false;
  static bool _windowsRuntimeAvailable = false;

  static bool get supportsNativePreviewActions =>
      Platform.isMacOS || Platform.isWindows;

  static Future<bool> openInspector() async {
    if (Platform.isWindows) {
      final controller = _activeWindowsController;
      if (controller == null) {
        return false;
      }

      await controller.openDevTools();
      return true;
    }

    return _invoke('openInspector');
  }

  static Future<bool> refresh() async {
    if (Platform.isWindows) {
      final controller = _activeWindowsController;
      if (controller == null) {
        return false;
      }

      await controller.reload();
      return true;
    }

    return _invoke('refresh');
  }

  static Future<bool> goBack() async {
    if (Platform.isWindows) {
      final controller = _activeWindowsController;
      if (controller == null) {
        return false;
      }

      await controller.goBack();
      return true;
    }

    return _invoke('goBack');
  }

  static Future<bool> goForward() async {
    if (Platform.isWindows) {
      final controller = _activeWindowsController;
      if (controller == null) {
        return false;
      }

      await controller.goForward();
      return true;
    }

    return _invoke('goForward');
  }

  static void setActiveWindowsController(WebviewController? controller) {
    _activeWindowsController = controller;
  }

  static void clearActiveWindowsController(WebviewController controller) {
    if (identical(_activeWindowsController, controller)) {
      _activeWindowsController = null;
    }
  }

  static Future<WoxWindowsWebViewSession?> acquireWindowsSession(
    WoxPreviewWebviewData previewData,
  ) async {
    if (!Platform.isWindows) {
      return null;
    }

    final runtimeReady = await ensureWindowsWebViewReady();
    if (!runtimeReady) {
      return null;
    }

    final cacheKey = previewData.resolvedCacheKey;
    final shouldCache = cacheKey.isNotEmpty;
    final session =
        shouldCache
            ? (_cachedWindowsSessions[cacheKey] ??=
                WoxWindowsWebViewSession.cached(cacheKey: cacheKey))
            : WoxWindowsWebViewSession.transient();
    await session.ensureInitialized();
    await session.apply(previewData);
    return session;
  }

  static Future<void> releaseWindowsSession(
    WoxWindowsWebViewSession? session,
  ) async {
    if (session == null || session.isCached) {
      return;
    }

    await session.dispose();
  }

  static Future<bool> ensureWindowsWebViewReady() async {
    if (!Platform.isWindows) {
      return false;
    }

    if (_windowsRuntimeChecked) {
      return _windowsRuntimeAvailable;
    }

    final version = await WebviewController.getWebViewVersion();
    _windowsRuntimeChecked = true;
    _windowsRuntimeAvailable = version != null;
    if (!_windowsRuntimeAvailable) {
      return false;
    }

    _windowsEnvironmentInitialization ??= _initializeWindowsEnvironment();
    await _windowsEnvironmentInitialization;
    return true;
  }

  static Future<void> _initializeWindowsEnvironment() async {
    final supportDirectory = await getApplicationSupportDirectory();
    final userDataPath =
        "${supportDirectory.path}${Platform.pathSeparator}webview_windows";

    try {
      await WebviewController.initializeEnvironment(userDataPath: userDataPath);
    } on PlatformException catch (error) {
      final message = error.message?.toLowerCase() ?? "";
      if (!message.contains("initialized")) {
        rethrow;
      }
    }
  }

  static String buildInjectCssScript(String css) {
    final cssLiteral = _encodeJsString(css);
    return """
(() => {
  const css = $cssLiteral;
  if (!css) {
    return;
  }

  const styleId = "wox-webview-preview-style";
  let style = document.getElementById(styleId);
  if (!style) {
    style = document.createElement("style");
    style.id = styleId;
    (document.head || document.documentElement).appendChild(style);
  }
  style.textContent = css;
})();
""";
  }

  static String _encodeJsString(String input) {
    final escaped = input
        .replaceAll(r'\', r'\\')
        .replaceAll("'", r"\'")
        .replaceAll('\r', r'\r')
        .replaceAll('\n', r'\n')
        .replaceAll('\u2028', r'\u2028')
        .replaceAll('\u2029', r'\u2029');
    return "'$escaped'";
  }

  static Future<bool> _invoke(String method) async {
    if (!Platform.isMacOS) {
      return false;
    }

    final result = await _channel.invokeMethod<bool>(method);
    return result ?? false;
  }
}

class WoxWindowsWebViewSession {
  final bool isCached;
  final String? cacheKey;
  final WebviewController controller = WebviewController();

  Future<void>? _initialization;
  String _currentUrl = "";
  String _currentCss = "";
  String? _currentScriptId;
  bool _disposed = false;

  WoxWindowsWebViewSession.cached({required this.cacheKey}) : isCached = true;

  WoxWindowsWebViewSession.transient() : isCached = false, cacheKey = null;

  Future<void> ensureInitialized() {
    _initialization ??= _initialize();
    return _initialization!;
  }

  Future<void> apply(WoxPreviewWebviewData previewData) async {
    if (_disposed) {
      return;
    }

    await ensureInitialized();

    final injectCssChanged = _currentCss != previewData.injectCss;
    if (injectCssChanged && _currentScriptId != null) {
      await controller.removeScriptToExecuteOnDocumentCreated(
        _currentScriptId!,
      );
      _currentScriptId = null;
    }

    if (injectCssChanged && previewData.injectCss.isNotEmpty) {
      _currentScriptId = await controller.addScriptToExecuteOnDocumentCreated(
        WoxWebViewUtil.buildInjectCssScript(previewData.injectCss),
      );
    }

    final shouldReload = _currentUrl != previewData.url || injectCssChanged;
    _currentCss = previewData.injectCss;

    if (shouldReload && previewData.url.isNotEmpty) {
      await controller.loadUrl(previewData.url);
      _currentUrl = previewData.url;
    }
  }

  Future<void> dispose() async {
    if (_disposed) {
      return;
    }

    _disposed = true;
    await controller.dispose();
  }

  Future<void> _initialize() async {
    await controller.initialize();
    await controller.setBackgroundColor(Colors.transparent);
    await controller.setPopupWindowPolicy(WebviewPopupWindowPolicy.sameWindow);
    await controller.setUserAgent(WoxWebViewUtil.mobileUserAgent);
    await controller.setCacheDisabled(!isCached);
  }
}

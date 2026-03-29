import 'dart:async';

import 'package:flutter/foundation.dart';
import 'package:flutter/material.dart';
import 'package:wox/entity/wox_preview_webview_data.dart';
import 'package:wox/utils/webview/windows/webview.dart';
import 'package:wox/utils/webview/wox_webview_session.dart';
import 'package:wox/utils/webview/wox_webview_support.dart';

class WoxWindowsWebViewSession implements WoxWebViewSession {
  @override
  final bool isCached;

  @override
  final String? cacheKey;

  final WebviewController controller = WebviewController();
  final StreamController<WoxWebViewSessionAction> _actions = StreamController<WoxWebViewSessionAction>.broadcast();
  final ValueNotifier<WoxWebViewNavigationState> _navigationState = ValueNotifier(const WoxWebViewNavigationState());

  Future<void>? _initialization;
  StreamSubscription<AcceleratorKeyPressedEvent>? _acceleratorKeySubscription;
  StreamSubscription<HistoryChanged>? _historyChangedSubscription;
  StreamSubscription<dynamic>? _webMessageSubscription;
  String _currentUrl = "";
  String _currentCss = "";
  String? _currentScriptId;
  bool _disposed = false;

  WoxWindowsWebViewSession.cached({required this.cacheKey}) : isCached = true;

  WoxWindowsWebViewSession.transient() : isCached = false, cacheKey = null;

  @override
  Stream<WoxWebViewSessionAction> get actions => _actions.stream;

  @override
  ValueListenable<WoxWebViewNavigationState> get navigationState => _navigationState;

  @override
  Widget buildWidget() => SizedBox.expand(child: Webview(controller));

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
      await controller.removeScriptToExecuteOnDocumentCreated(_currentScriptId!);
      _currentScriptId = null;
    }

    if (injectCssChanged && previewData.injectCss.isNotEmpty) {
      _currentScriptId = await controller.addScriptToExecuteOnDocumentCreated(WoxWebViewSupport.buildInjectCssScript(previewData.injectCss));
    }

    final shouldReload = _currentUrl != previewData.url || injectCssChanged;
    _currentCss = previewData.injectCss;

    if (shouldReload && previewData.url.isNotEmpty) {
      await controller.loadUrl(previewData.url);
      _currentUrl = previewData.url;
    }
  }

  @override
  Future<void> dispose() async {
    if (_disposed) {
      return;
    }

    _disposed = true;
    await _acceleratorKeySubscription?.cancel();
    await _historyChangedSubscription?.cancel();
    await _webMessageSubscription?.cancel();
    await _actions.close();
    _navigationState.dispose();
    await controller.dispose();
  }

  Future<void> _initialize() async {
    await controller.initialize();
    await controller.addScriptToExecuteOnDocumentCreated(WoxWebViewSupport.buildUnhandledEscapeScript(postMessageExpression: "window.chrome.webview.postMessage"));
    _acceleratorKeySubscription = controller.acceleratorKeyPressed.listen((event) {
      final isAltJ = event.keyEventKind == 2 && event.virtualKey == 0x4A;

      if (isAltJ) {
        _actions.add(WoxWebViewSessionAction.toggleActionPanel);
      }
    });
    _historyChangedSubscription = controller.historyChanged.listen((event) {
      _navigationState.value = WoxWebViewNavigationState(canGoBack: event.canGoBack, canGoForward: event.canGoForward);
    });
    _webMessageSubscription = controller.webMessage.listen((message) {
      if (message is! Map) {
        return;
      }

      if (message["type"] == WoxWebViewSupport.unhandledEscapeMessageType) {
        _actions.add(WoxWebViewSessionAction.fallbackEscape);
      }
    });
    await controller.setBackgroundColor(Colors.transparent);
    await controller.setPopupWindowPolicy(WebviewPopupWindowPolicy.sameWindow);
    await controller.setUserAgent(WoxWebViewSupport.mobileUserAgent);
    await controller.setCacheDisabled(!isCached);
  }
}

import 'dart:async';
import 'dart:io';

import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:get/get.dart';
import 'package:uuid/v4.dart';
import 'package:wox/components/wox_loading_indicator.dart';
import 'package:wox/controllers/wox_launcher_controller.dart';
import 'package:wox/entity/wox_preview_webview_data.dart';
import 'package:wox/utils/webview/wox_webview_util.dart';
import 'package:wox/utils/webview/wox_webview_session.dart';

class WoxWebViewPreview extends StatefulWidget {
  final String previewData;

  const WoxWebViewPreview({super.key, required this.previewData});

  @override
  State<WoxWebViewPreview> createState() => _WoxWebViewPreviewState();
}

class _WoxWebViewPreviewState extends State<WoxWebViewPreview> {
  Future<WoxWebViewSession?>? _windowsSessionFuture;
  WoxWebViewSession? _session;
  StreamSubscription<WoxWebViewSessionAction>? _sessionActionSubscription;
  String? _windowsErrorMessage;
  final launcherController = Get.find<WoxLauncherController>();

  WoxPreviewWebviewData get webviewData {
    return WoxPreviewWebviewData.fromPreviewData(widget.previewData);
  }

  @override
  void initState() {
    super.initState();
    _refreshWindowsSession();
  }

  @override
  void didUpdateWidget(covariant WoxWebViewPreview oldWidget) {
    super.didUpdateWidget(oldWidget);
    if (oldWidget.previewData != widget.previewData) {
      unawaited(_releaseCurrentSession());
      _refreshWindowsSession();
    }
  }

  @override
  void dispose() {
    unawaited(_releaseCurrentSession());
    super.dispose();
  }

  void _refreshWindowsSession() {
    if (!Platform.isWindows) {
      return;
    }

    _windowsErrorMessage = null;
    _windowsSessionFuture = WoxWebViewUtil.acquireSession(webviewData)
        .then((session) {
          _session = session;
          WoxWebViewUtil.setActiveSession(session);
          _subscribeSessionActions(session);
          return session;
        })
        .catchError((error) {
          _windowsErrorMessage = error.toString();
          return null;
        });
  }

  Future<void> _releaseCurrentSession() async {
    await _sessionActionSubscription?.cancel();
    _sessionActionSubscription = null;

    final session = _session;
    if (session == null) {
      return;
    }

    WoxWebViewUtil.clearActiveSession(session);
    _session = null;
    await WoxWebViewUtil.releaseSession(session);
  }

  void _subscribeSessionActions(WoxWebViewSession? session) {
    _sessionActionSubscription?.cancel();
    _sessionActionSubscription = null;

    if (session == null) {
      return;
    }

    _sessionActionSubscription = session.actions.listen((action) {
      switch (action) {
        case WoxWebViewSessionAction.toggleActionPanel:
          launcherController.toggleActionPanel(const UuidV4().generate());
          break;
        case WoxWebViewSessionAction.focusQueryBox:
          launcherController.focusQueryBox();
          break;
      }
    });
  }

  Widget _buildWindowsPreview(WoxPreviewWebviewData preview) {
    final future = _windowsSessionFuture;
    if (future == null) {
      return SelectableText("WebView preview is not initialized on Windows.\nURL: ${preview.url}");
    }

    return FutureBuilder<WoxWebViewSession?>(
      future: future,
      builder: (context, snapshot) {
        if (snapshot.connectionState != ConnectionState.done) {
          return const Center(child: WoxLoadingIndicator(size: 20));
        }

        final session = snapshot.data;
        if (session == null) {
          final message = _windowsErrorMessage ?? "WebView2 Runtime is not available on this system.";
          return SelectableText("$message\nURL: ${preview.url}");
        }

        return session.buildWidget();
      },
    );
  }

  @override
  Widget build(BuildContext context) {
    final preview = webviewData;

    if (Platform.isWindows) {
      return _buildWindowsPreview(preview);
    }

    if (Platform.isMacOS) {
      return SizedBox.expand(
        child: AppKitView(key: ValueKey(widget.previewData), viewType: "wox/webview_preview", creationParams: preview.toJson(), creationParamsCodec: const StandardMessageCodec()),
      );
    }

    return SelectableText("WebView preview is currently only available on macOS and Windows.\nURL: ${preview.url}");
  }
}

import 'dart:async';
import 'dart:io';

import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:wox/components/wox_loading_indicator.dart';
import 'package:wox/entity/wox_preview_webview_data.dart';
import 'package:wox/utils/wox_webview_util.dart';
import 'package:webview_windows/webview_windows.dart';

class WoxWebViewPreview extends StatefulWidget {
  static const String viewType = "wox/webview_preview";

  final String previewData;

  const WoxWebViewPreview({super.key, required this.previewData});

  @override
  State<WoxWebViewPreview> createState() => _WoxWebViewPreviewState();
}

class _WoxWebViewPreviewState extends State<WoxWebViewPreview> {
  Future<WoxWindowsWebViewSession?>? _windowsSessionFuture;
  WoxWindowsWebViewSession? _session;
  String? _windowsErrorMessage;

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
    _windowsSessionFuture = WoxWebViewUtil.acquireWindowsSession(webviewData)
        .then((session) {
          _session = session;
          WoxWebViewUtil.setActiveWindowsController(session?.controller);
          return session;
        })
        .catchError((error) {
          _windowsErrorMessage = error.toString();
          return null;
        });
  }

  Future<void> _releaseCurrentSession() async {
    final session = _session;
    if (session == null) {
      return;
    }

    WoxWebViewUtil.clearActiveWindowsController(session.controller);
    _session = null;
    await WoxWebViewUtil.releaseWindowsSession(session);
  }

  Widget _buildWindowsPreview(WoxPreviewWebviewData preview) {
    final future = _windowsSessionFuture;
    if (future == null) {
      return SelectableText("WebView preview is not initialized on Windows.\nURL: ${preview.url}");
    }

    return FutureBuilder<WoxWindowsWebViewSession?>(
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

        return SizedBox.expand(child: Webview(session.controller));
      },
    );
  }

  @override
  Widget build(BuildContext context) {
    final preview = webviewData;

    if (Platform.isWindows) {
      return _buildWindowsPreview(preview);
    }

    if (!Platform.isMacOS) {
      return SelectableText("WebView preview is currently only available on macOS and Windows.\nURL: ${preview.url}");
    }

    return SizedBox.expand(
      child: AppKitView(
        key: ValueKey(widget.previewData),
        viewType: WoxWebViewPreview.viewType,
        creationParams: preview.toJson(),
        creationParamsCodec: const StandardMessageCodec(),
      ),
    );
  }
}

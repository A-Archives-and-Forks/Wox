import 'dart:io';

import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:wox/entity/wox_preview_webview_data.dart';

class WoxWebViewPreview extends StatelessWidget {
  static const String viewType = "wox/webview_preview";

  final String previewData;

  const WoxWebViewPreview({super.key, required this.previewData});

  WoxPreviewWebviewData get webviewData {
    return WoxPreviewWebviewData.fromPreviewData(previewData);
  }

  @override
  Widget build(BuildContext context) {
    final preview = webviewData;

    if (!Platform.isMacOS) {
      return SelectableText("WebView preview prototype is currently only available on macOS.\nURL: ${preview.url}");
    }

    return SizedBox.expand(child: AppKitView(key: ValueKey(previewData), viewType: viewType, creationParams: preview.toJson(), creationParamsCodec: const StandardMessageCodec()));
  }
}

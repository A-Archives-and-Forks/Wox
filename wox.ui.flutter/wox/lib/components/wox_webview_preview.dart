import 'dart:convert';
import 'dart:io';

import 'package:flutter/material.dart';
import 'package:flutter/services.dart';

class WoxWebViewPreview extends StatelessWidget {
  static const String viewType = "wox/webview_preview";

  final String previewData;

  const WoxWebViewPreview({super.key, required this.previewData});

  Map<String, dynamic> get creationParams {
    try {
      final decoded = jsonDecode(previewData);
      if (decoded is Map<String, dynamic> && decoded["url"] is String) {
        return decoded;
      }
    } catch (_) {
      // Keep backward compatibility with plain URL payloads.
    }

    return {"url": previewData};
  }

  @override
  Widget build(BuildContext context) {
    if (!Platform.isMacOS) {
      return SelectableText("WebView preview prototype is currently only available on macOS.\nURL: ${creationParams["url"] ?? previewData}");
    }

    return SizedBox.expand(child: AppKitView(key: ValueKey(previewData), viewType: viewType, creationParams: creationParams, creationParamsCodec: const StandardMessageCodec()));
  }
}

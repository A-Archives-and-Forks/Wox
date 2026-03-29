import 'dart:convert';

class WoxPreviewWebviewData {
  late String url;
  late String injectCss;
  late bool cacheDisabled;

  WoxPreviewWebviewData({required this.url, this.injectCss = "", this.cacheDisabled = false});

  factory WoxPreviewWebviewData.fromJson(Map<String, dynamic> json) {
    return WoxPreviewWebviewData(url: json["url"]?.toString() ?? "", injectCss: json["injectCss"]?.toString() ?? "", cacheDisabled: json["cacheDisabled"] == true);
  }

  factory WoxPreviewWebviewData.fromPreviewData(String previewData) {
    try {
      final decoded = jsonDecode(previewData);
      if (decoded is Map) {
        final json = Map<String, dynamic>.from(decoded);
        if (json["url"] is String) {
          return WoxPreviewWebviewData.fromJson(json);
        }
      }
    } catch (_) {
      // Keep backward compatibility with plain URL payloads.
    }

    return WoxPreviewWebviewData(url: previewData);
  }

  Map<String, dynamic> toJson() {
    return {"url": url, "injectCss": injectCss, "cacheDisabled": cacheDisabled, "cacheKey": resolvedCacheKey};
  }

  String get resolvedCacheKey {
    if (cacheDisabled) {
      return "";
    }

    return url.trim();
  }
}

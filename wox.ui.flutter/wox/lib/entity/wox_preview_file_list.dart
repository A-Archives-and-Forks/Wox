import 'dart:convert';

class WoxPreviewFileListData {
  final List<String> filePaths;

  const WoxPreviewFileListData({required this.filePaths});

  factory WoxPreviewFileListData.fromJson(Map<String, dynamic> json) {
    final rawPaths = json["filePaths"];

    // The renderer consumes the same public preview contract as SDK plugins.
    // Older code decoded this shape inline, which hid the expected field names
    // from readers and made malformed payload handling inconsistent.
    return WoxPreviewFileListData(filePaths: rawPaths is List ? rawPaths.map((item) => item.toString()).toList() : const []);
  }

  factory WoxPreviewFileListData.fromPreviewData(String previewData) {
    final decoded = jsonDecode(previewData);

    return WoxPreviewFileListData.fromJson(decoded is Map<String, dynamic> ? decoded : const {});
  }

  Map<String, dynamic> toJson() {
    return {"filePaths": filePaths};
  }
}

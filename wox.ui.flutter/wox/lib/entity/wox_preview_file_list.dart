import 'dart:convert';

class WoxPreviewFileList {
  final List<String> filePaths;

  const WoxPreviewFileList({required this.filePaths});

  factory WoxPreviewFileList.fromPreviewData(String previewData) {
    final decoded = jsonDecode(previewData);
    final rawPaths = decoded is Map<String, dynamic> ? decoded["filePaths"] : null;

    // File-list preview data is intentionally structured instead of markdown so
    // the UI can preserve filename, directory, and type affordances. Unknown
    // payloads degrade to an empty list instead of throwing from the renderer.
    return WoxPreviewFileList(filePaths: rawPaths is List ? rawPaths.map((item) => item.toString()).toList() : const []);
  }
}

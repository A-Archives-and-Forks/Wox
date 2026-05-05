import 'package:flutter/material.dart';
import 'package:path/path.dart' as path;
import 'package:wox/components/wox_tooltip.dart';
import 'package:wox/entity/wox_preview_file_list.dart';
import 'package:wox/entity/wox_theme.dart';
import 'package:wox/utils/color_util.dart';

class WoxFileListPreviewView extends StatelessWidget {
  final WoxPreviewFileList data;
  final WoxTheme woxTheme;

  const WoxFileListPreviewView({super.key, required this.data, required this.woxTheme});

  @override
  Widget build(BuildContext context) {
    final fontColor = safeFromCssColor(woxTheme.previewFontColor);
    final splitLineColor = safeFromCssColor(woxTheme.previewSplitLineColor);
    final surfaceColor = fontColor.withValues(alpha: 0.035);
    final borderColor = splitLineColor.withValues(alpha: 0.48);

    // File selection previews used to arrive as markdown, which made paths look
    // like raw debug text. A native file-list surface gives each path a clear
    // filename, parent folder, and type chip without adding action controls.
    return Container(
      decoration: BoxDecoration(color: surfaceColor, borderRadius: BorderRadius.circular(8), border: Border.all(color: borderColor)),
      child:
          data.filePaths.isEmpty
              ? Center(child: Text("No files", style: TextStyle(color: fontColor.withValues(alpha: 0.62), fontSize: 14)))
              : Padding(
                padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 10),
                // Scrolling is owned by WoxPreviewScaffold. Keeping this view as
                // a plain column avoids nested scrollables and the unbounded
                // height layout failures that happen when preview metadata
                // changes the available content area.
                child: Column(
                  children: [
                    for (var index = 0; index < data.filePaths.length; index++) ...[
                      _FileListPreviewRow(filePath: data.filePaths[index], woxTheme: woxTheme),
                      if (index != data.filePaths.length - 1) Divider(height: 1, color: splitLineColor.withValues(alpha: 0.28)),
                    ],
                  ],
                ),
              ),
    );
  }
}

class _FileListPreviewRow extends StatelessWidget {
  final String filePath;
  final WoxTheme woxTheme;

  const _FileListPreviewRow({required this.filePath, required this.woxTheme});

  @override
  Widget build(BuildContext context) {
    final fontColor = safeFromCssColor(woxTheme.previewFontColor);
    final splitLineColor = safeFromCssColor(woxTheme.previewSplitLineColor);
    final propertyColor = safeFromCssColor(woxTheme.previewPropertyContentColor);
    final fileName = path.basename(filePath);
    final parentPath = path.dirname(filePath);
    final extension = path.extension(filePath).replaceFirst(".", "").toUpperCase();
    final typeLabel = extension.isEmpty ? "FILE" : extension;

    return Padding(
      padding: const EdgeInsets.symmetric(vertical: 10),
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.center,
        children: [
          Container(
            width: 34,
            height: 34,
            decoration: BoxDecoration(
              color: propertyColor.withValues(alpha: 0.1),
              borderRadius: BorderRadius.circular(8),
              border: Border.all(color: splitLineColor.withValues(alpha: 0.38)),
            ),
            child: Icon(_iconForExtension(extension), color: propertyColor.withValues(alpha: 0.88), size: 18),
          ),
          const SizedBox(width: 12),
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              mainAxisSize: MainAxisSize.min,
              children: [
                WoxTooltip(
                  message: filePath,
                  child: Text(
                    fileName,
                    maxLines: 1,
                    overflow: TextOverflow.ellipsis,
                    style: TextStyle(color: fontColor.withValues(alpha: 0.92), fontSize: 14.5, fontWeight: FontWeight.w600, height: 1.2),
                  ),
                ),
                const SizedBox(height: 4),
                WoxTooltip(
                  message: parentPath,
                  child: Text(parentPath, maxLines: 1, overflow: TextOverflow.ellipsis, style: TextStyle(color: fontColor.withValues(alpha: 0.56), fontSize: 12.5, height: 1.2)),
                ),
              ],
            ),
          ),
          const SizedBox(width: 10),
          Container(
            constraints: const BoxConstraints(maxWidth: 78),
            padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 4),
            decoration: BoxDecoration(
              color: fontColor.withValues(alpha: 0.035),
              borderRadius: BorderRadius.circular(8),
              border: Border.all(color: splitLineColor.withValues(alpha: 0.42)),
            ),
            child: Text(
              typeLabel,
              maxLines: 1,
              overflow: TextOverflow.ellipsis,
              style: TextStyle(color: fontColor.withValues(alpha: 0.62), fontSize: 11, fontWeight: FontWeight.w600, height: 1.1),
            ),
          ),
        ],
      ),
    );
  }

  IconData _iconForExtension(String extension) {
    switch (extension.toLowerCase()) {
      case "png":
      case "jpg":
      case "jpeg":
      case "webp":
      case "gif":
      case "bmp":
      case "svg":
        return Icons.image_outlined;
      case "dmg":
      case "zip":
      case "rar":
      case "7z":
      case "tar":
      case "gz":
        return Icons.archive_outlined;
      case "pdf":
        return Icons.picture_as_pdf_outlined;
      default:
        return Icons.insert_drive_file_outlined;
    }
  }
}

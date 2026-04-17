import 'dart:convert';
import 'dart:typed_data';
import 'dart:ui';

import 'package:flutter/material.dart';

enum ScreenshotSessionStage { idle, loading, selecting, annotating, exporting, done, cancelled, failed }

enum ScreenshotTool { select, rect, ellipse, arrow, text }

enum ScreenshotAnnotationType { rect, ellipse, arrow, text }

Map<String, dynamic>? _normalizeJsonMap(dynamic value) {
  if (value is Map<String, dynamic>) {
    return value;
  }
  if (value is! Map) {
    return null;
  }

  // MethodChannel payloads arrive as JSON-like maps whose nested values are usually typed as
  // Map<Object?, Object?>. Normalizing the structure at the entity boundary keeps screenshot
  // parsing tolerant to native bridge responses instead of crashing before the session starts.
  return value.map<String, dynamic>((key, entryValue) => MapEntry(key.toString(), _normalizeJsonValue(entryValue)));
}

dynamic _normalizeJsonValue(dynamic value) {
  if (value is Map) {
    return _normalizeJsonMap(value);
  }
  if (value is List) {
    return value.map(_normalizeJsonValue).toList();
  }
  return value;
}

class ScreenshotPoint {
  const ScreenshotPoint({required this.x, required this.y});

  final double x;
  final double y;

  factory ScreenshotPoint.fromOffset(Offset offset) => ScreenshotPoint(x: offset.dx, y: offset.dy);

  Offset toOffset() => Offset(x, y);
}

class ScreenshotRect {
  const ScreenshotRect({required this.x, required this.y, required this.width, required this.height});

  final double x;
  final double y;
  final double width;
  final double height;

  factory ScreenshotRect.fromJson(Map<String, dynamic> json) {
    return ScreenshotRect(
      x: (json['x'] ?? json['X'] ?? 0).toDouble(),
      y: (json['y'] ?? json['Y'] ?? 0).toDouble(),
      width: (json['width'] ?? json['Width'] ?? 0).toDouble(),
      height: (json['height'] ?? json['Height'] ?? 0).toDouble(),
    );
  }

  factory ScreenshotRect.fromRect(Rect rect) {
    return ScreenshotRect(x: rect.left, y: rect.top, width: rect.width, height: rect.height);
  }

  Rect toRect() => Rect.fromLTWH(x, y, width, height);

  Map<String, dynamic> toJson() {
    return {'x': x, 'y': y, 'width': width, 'height': height};
  }
}

class CaptureScreenshotRequest {
  const CaptureScreenshotRequest({required this.sessionId, required this.trigger, required this.scope, required this.output, required this.tools});

  final String sessionId;
  final String trigger;
  final String scope;
  final String output;
  final List<String> tools;

  factory CaptureScreenshotRequest.fromJson(Map<String, dynamic> json) {
    return CaptureScreenshotRequest(
      sessionId: json['SessionId'] as String? ?? json['sessionId'] as String? ?? '',
      trigger: json['Trigger'] as String? ?? json['trigger'] as String? ?? 'plugin',
      scope: json['Scope'] as String? ?? json['scope'] as String? ?? 'all_displays',
      output: json['Output'] as String? ?? json['output'] as String? ?? 'clipboard',
      tools: ((json['Tools'] ?? json['tools']) as List<dynamic>? ?? const []).map((tool) => tool.toString()).toList(),
    );
  }

  Map<String, dynamic> toJson() {
    return {'sessionId': sessionId, 'trigger': trigger, 'scope': scope, 'output': output, 'tools': tools};
  }
}

class CaptureScreenshotResult {
  const CaptureScreenshotResult({required this.status, this.pngBase64, this.logicalSelectionRect, this.errorCode, this.errorMessage});

  final String status;
  final String? pngBase64;
  final ScreenshotRect? logicalSelectionRect;
  final String? errorCode;
  final String? errorMessage;

  factory CaptureScreenshotResult.completed(Uint8List pngBytes, Rect selectionRect) {
    return CaptureScreenshotResult(status: 'completed', pngBase64: base64Encode(pngBytes), logicalSelectionRect: ScreenshotRect.fromRect(selectionRect));
  }

  factory CaptureScreenshotResult.cancelled() => const CaptureScreenshotResult(status: 'cancelled');

  factory CaptureScreenshotResult.failed({String? errorCode, String? errorMessage}) {
    return CaptureScreenshotResult(status: 'failed', errorCode: errorCode, errorMessage: errorMessage);
  }

  factory CaptureScreenshotResult.fromJson(Map<String, dynamic> json) {
    final logicalSelectionRect = _normalizeJsonMap(json['logicalSelectionRect'] ?? json['LogicalSelectionRect']);

    return CaptureScreenshotResult(
      status: json['status'] as String? ?? json['Status'] as String? ?? 'failed',
      pngBase64: json['pngBase64'] as String? ?? json['PngBase64'] as String?,
      logicalSelectionRect: logicalSelectionRect != null ? ScreenshotRect.fromJson(logicalSelectionRect) : null,
      errorCode: json['errorCode'] as String? ?? json['ErrorCode'] as String?,
      errorMessage: json['errorMessage'] as String? ?? json['ErrorMessage'] as String?,
    );
  }

  Map<String, dynamic> toJson() {
    return {'status': status, 'pngBase64': pngBase64, 'logicalSelectionRect': logicalSelectionRect?.toJson(), 'errorCode': errorCode, 'errorMessage': errorMessage};
  }
}

class DisplaySnapshot {
  DisplaySnapshot({required this.displayId, required this.logicalBounds, required this.pixelBounds, required this.scale, required this.rotation, required this.imageBytesBase64});

  final String displayId;
  final ScreenshotRect logicalBounds;
  final ScreenshotRect pixelBounds;
  final double scale;
  final int rotation;
  final String imageBytesBase64;

  Uint8List get imageBytes => base64Decode(imageBytesBase64);

  factory DisplaySnapshot.fromJson(Map<String, dynamic> json) {
    final logicalBounds = _normalizeJsonMap(json['logicalBounds'] ?? json['LogicalBounds']);
    final pixelBounds = _normalizeJsonMap(json['pixelBounds'] ?? json['PixelBounds']);

    return DisplaySnapshot(
      displayId: json['displayId'] as String? ?? json['DisplayId'] as String? ?? '',
      logicalBounds: ScreenshotRect.fromJson(logicalBounds ?? const <String, dynamic>{}),
      pixelBounds: ScreenshotRect.fromJson(pixelBounds ?? const <String, dynamic>{}),
      scale: (json['scale'] ?? json['Scale'] ?? 1).toDouble(),
      rotation: (json['rotation'] ?? json['Rotation'] ?? 0) as int,
      imageBytesBase64: json['imageBytesBase64'] as String? ?? json['ImageBytesBase64'] as String? ?? '',
    );
  }
}

class ScreenshotAnnotation {
  const ScreenshotAnnotation({
    required this.id,
    required this.type,
    this.rect,
    this.start,
    this.end,
    this.text,
    this.color = const Color(0xFFFF5B36),
    this.strokeWidth = 3,
    this.fontSize = 20,
  });

  final String id;
  final ScreenshotAnnotationType type;
  final Rect? rect;
  final Offset? start;
  final Offset? end;
  final String? text;
  final Color color;
  final double strokeWidth;
  final double fontSize;
}

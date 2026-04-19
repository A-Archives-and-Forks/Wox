import 'dart:async';
import 'dart:io';

import 'package:flutter/services.dart';
import 'package:wox/entity/screenshot_session.dart';

abstract class ScreenshotPlatformBridge {
  static ScreenshotPlatformBridge _instance = MethodChannelScreenshotPlatformBridge();

  static ScreenshotPlatformBridge get instance => _instance;

  static void setInstanceForTest(ScreenshotPlatformBridge bridge) {
    _instance = bridge;
  }

  static void resetInstance() {
    _instance = MethodChannelScreenshotPlatformBridge();
  }

  Future<List<DisplaySnapshot>> captureAllDisplays();

  Future<ScreenshotNativeSelectionResult> selectCaptureRegion(ScreenshotRect nativeWorkspaceBounds);

  Future<ScreenshotWorkspacePresentation> presentCaptureWorkspace(ScreenshotRect nativeWorkspaceBounds);

  Future<ScreenshotWorkspacePresentation> prepareCaptureWorkspace(ScreenshotRect nativeWorkspaceBounds) {
    return presentCaptureWorkspace(nativeWorkspaceBounds);
  }

  Future<void> revealPreparedCaptureWorkspace() async {}

  Stream<ScreenshotSelectionDisplayHint> selectionDisplayHints() => const Stream<ScreenshotSelectionDisplayHint>.empty();

  Future<void> dismissCaptureWorkspacePresentation();

  Future<void> dismissNativeSelectionOverlays();

  Future<Map<String, dynamic>> debugCaptureWorkspaceState();
}

class MethodChannelScreenshotPlatformBridge implements ScreenshotPlatformBridge {
  static const String _windowsChannelName = 'com.wox.windows_window_manager';
  static const String _macosChannelName = 'com.wox.macos_window_manager';
  static const String _macosScreenshotEventChannelName = 'com.wox.macos_screenshot_events';
  static const String _linuxChannelName = 'com.wox.linux_window_manager';

  late final MethodChannel _channel = MethodChannel(_resolveChannelName());
  late final StreamController<ScreenshotSelectionDisplayHint> _selectionDisplayHintController = StreamController<ScreenshotSelectionDisplayHint>.broadcast();

  MethodChannelScreenshotPlatformBridge() {
    if (Platform.isMacOS) {
      const MethodChannel(_macosScreenshotEventChannelName).setMethodCallHandler(_handleMacOSScreenshotEvent);
    }
  }

  String _resolveChannelName() {
    if (Platform.isWindows) {
      return _windowsChannelName;
    }
    if (Platform.isMacOS) {
      return _macosChannelName;
    }
    if (Platform.isLinux) {
      return _linuxChannelName;
    }
    throw UnsupportedError('Unsupported platform: ${Platform.operatingSystem}');
  }

  @override
  Future<List<DisplaySnapshot>> captureAllDisplays() async {
    final response = await _channel.invokeMethod<List<dynamic>>('captureAllDisplays');
    final snapshots = response ?? const <dynamic>[];

    // The native bridge returns JSON-like maps so Flutter can keep the screenshot session platform-agnostic.
    return snapshots.whereType<Map<dynamic, dynamic>>().map((item) {
      return DisplaySnapshot.fromJson(item.map((key, value) => MapEntry(key.toString(), value)));
    }).toList();
  }

  @override
  Future<ScreenshotNativeSelectionResult> selectCaptureRegion(ScreenshotRect nativeWorkspaceBounds) async {
    try {
      final response = await _channel.invokeMethod<Map<dynamic, dynamic>>('selectCaptureRegion', nativeWorkspaceBounds.toJson());
      if (response == null) {
        return const ScreenshotNativeSelectionResult(wasHandled: false);
      }

      return ScreenshotNativeSelectionResult.fromJson(response.map((key, value) => MapEntry(key.toString(), value)));
    } on MissingPluginException {
      // Only the macOS runner currently installs the native region selector. Returning an
      // unhandled response keeps the existing Flutter workspace path active everywhere else.
      return const ScreenshotNativeSelectionResult(wasHandled: false);
    }
  }

  @override
  Future<ScreenshotWorkspacePresentation> presentCaptureWorkspace(ScreenshotRect nativeWorkspaceBounds) async {
    try {
      final response = await _channel.invokeMethod<Map<dynamic, dynamic>>('presentCaptureWorkspace', nativeWorkspaceBounds.toJson());
      if (response == null) {
        return ScreenshotWorkspacePresentation(workspaceBounds: nativeWorkspaceBounds, workspaceScale: 1, presentedByPlatform: false);
      }

      return ScreenshotWorkspacePresentation.fromJson(response.map((key, value) => MapEntry(key.toString(), value)));
    } on MissingPluginException {
      // Linux and older runners do not implement screenshot-only presentation. Falling back to the
      // generic window-manager path keeps the existing single-window workflow available there.
      return ScreenshotWorkspacePresentation(workspaceBounds: nativeWorkspaceBounds, workspaceScale: 1, presentedByPlatform: false);
    }
  }

  @override
  Future<ScreenshotWorkspacePresentation> prepareCaptureWorkspace(ScreenshotRect nativeWorkspaceBounds) async {
    try {
      final response = await _channel.invokeMethod<Map<dynamic, dynamic>>('prepareCaptureWorkspace', nativeWorkspaceBounds.toJson());
      if (response == null) {
        return ScreenshotWorkspacePresentation(workspaceBounds: nativeWorkspaceBounds, workspaceScale: 1, presentedByPlatform: false);
      }

      return ScreenshotWorkspacePresentation.fromJson(response.map((key, value) => MapEntry(key.toString(), value)));
    } on MissingPluginException {
      // Only macOS currently separates screenshot preparation from reveal. Older/native-simple
      // runners can still satisfy the contract through the original present call.
      return presentCaptureWorkspace(nativeWorkspaceBounds);
    }
  }

  @override
  Future<void> revealPreparedCaptureWorkspace() async {
    try {
      await _channel.invokeMethod<void>('revealPreparedCaptureWorkspace');
    } on MissingPluginException {
      return;
    }
  }

  @override
  Stream<ScreenshotSelectionDisplayHint> selectionDisplayHints() => _selectionDisplayHintController.stream;

  @override
  Future<void> dismissCaptureWorkspacePresentation() async {
    try {
      await _channel.invokeMethod<void>('dismissCaptureWorkspacePresentation');
    } on MissingPluginException {
      return;
    }
  }

  @override
  Future<void> dismissNativeSelectionOverlays() async {
    try {
      await _channel.invokeMethod<void>('dismissNativeSelectionOverlays');
    } on MissingPluginException {
      return;
    }
  }

  @override
  Future<Map<String, dynamic>> debugCaptureWorkspaceState() async {
    try {
      final response = await _channel.invokeMethod<Map<dynamic, dynamic>>('debugCaptureWorkspaceState');
      if (response == null) {
        return const <String, dynamic>{};
      }
      return response.map((key, value) => MapEntry(key.toString(), value));
    } on MissingPluginException {
      return const <String, dynamic>{};
    }
  }

  Future<void> _handleMacOSScreenshotEvent(MethodCall call) async {
    if (call.method != 'onSelectionDisplayHint') {
      return;
    }

    final arguments = call.arguments;
    if (arguments is! Map) {
      return;
    }

    // Native drag-time hints arrive as loosely typed method-channel maps. Parsing them here keeps
    // the controller focused on prewarm orchestration instead of transport-specific null checks.
    final normalized = arguments.map<String, dynamic>((key, value) => MapEntry(key.toString(), value));
    _selectionDisplayHintController.add(ScreenshotSelectionDisplayHint.fromJson(normalized));
  }
}

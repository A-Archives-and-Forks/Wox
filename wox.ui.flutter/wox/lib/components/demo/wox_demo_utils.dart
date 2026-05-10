part of 'wox_demo.dart';

String _formatDemoHotkey(String hotkey, {required String fallback}) {
  final configuredHotkey = hotkey.trim();
  final rawHotkey = configuredHotkey.isEmpty ? fallback : configuredHotkey;
  // Feature refinement: onboarding demos render the persisted hotkey values
  // rather than static labels. Formatting is limited to display casing so the
  // animation always mirrors the recorder shown above it.
  return rawHotkey.split('+').map(_formatDemoHotkeyPart).join('+');
}

String _formatDemoHotkeyPart(String part) {
  final normalized = part.trim().toLowerCase();
  return switch (normalized) {
    'alt' || 'option' => Platform.isMacOS ? 'Option' : 'Alt',
    'control' || 'ctrl' => 'Ctrl',
    'shift' => 'Shift',
    'meta' || 'command' || 'cmd' => Platform.isMacOS ? 'Cmd' : 'Win',
    'windows' || 'win' => 'Win',
    'space' => 'Space',
    'enter' => 'Enter',
    'escape' || 'esc' => 'Esc',
    'backspace' => 'Backspace',
    'delete' => 'Delete',
    'tab' => 'Tab',
    'arrowup' || 'up' => 'Up',
    'arrowdown' || 'down' => 'Down',
    'arrowleft' || 'left' => 'Left',
    'arrowright' || 'right' => 'Right',
    _ => normalized.isEmpty ? part : '${normalized[0].toUpperCase()}${normalized.substring(1)}',
  };
}

String _demoActionPanelHotkey() {
  // Feature fix: settings popovers reuse the same query demos without access to
  // onboarding's configured Action Panel hotkey. Using the platform default
  // keeps the launcher toolbar visible and truthful enough for feature previews
  // while avoiding a dependency on the onboarding controller.
  return _formatDemoHotkey('', fallback: Platform.isMacOS ? 'cmd+j' : 'alt+j');
}

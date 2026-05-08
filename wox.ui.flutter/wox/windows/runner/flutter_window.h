#ifndef RUNNER_FLUTTER_WINDOW_H_
#define RUNNER_FLUTTER_WINDOW_H_

#include <flutter/dart_project.h>
#include <flutter/flutter_view_controller.h>
#include <flutter/method_channel.h>
#include <flutter/standard_method_codec.h>

#include <memory>
#include <optional>
#include <string>
#include <unordered_set>
#include <vector>

#include "win32_window.h"

// A window that does nothing but host a Flutter view.
class FlutterWindow : public Win32Window
{
public:
  // Creates a new FlutterWindow hosting a Flutter view running |project|.
  explicit FlutterWindow(const flutter::DartProject &project);
  virtual ~FlutterWindow();

  // Log message to console and Flutter
  void Log(const std::string &message);

protected:
  // Win32Window:
  bool OnCreate() override;
  void OnDestroy() override;
  LRESULT MessageHandler(HWND window, UINT const message, WPARAM const wparam,
                         LPARAM const lparam) noexcept override;

private:
  // The project to run.
  flutter::DartProject project_;

  // The Flutter instance hosted by this window.
  std::unique_ptr<flutter::FlutterViewController> flutter_controller_;

  // Window manager method channel
  std::unique_ptr<flutter::MethodChannel<flutter::EncodableValue>> window_manager_channel_;

  // Original window procedure
  WNDPROC original_window_proc_;

  // Original child window procedure for the Flutter view hwnd.
  WNDPROC original_child_window_proc_ = nullptr;

  // Flutter view child window handle.
  HWND child_window_ = nullptr;

  // Previous active window handle
  HWND previous_active_window_;

  // Only restore the saved foreground window when Wox has stayed focused since
  // the last show/focus request.
  bool restore_previous_window_on_hide_ = false;

  // Guard transient WM_ACTIVATE/WA_INACTIVE blur events between show() and focus().
  // show() sets this to true; focus() and hide() clear it.
  bool blur_guard_active_ = false;

  // Extra blur grace period (GetTickCount64 deadline) after show/focus to absorb
  // short-lived foreground steals from other apps. see issue #4346
  ULONGLONG blur_guard_until_tick_ = 0;

  struct ScreenshotPresentationState
  {
    bool active = false;
    bool prepared = false;
    double workspace_scale = 1.0;
    RECT native_workspace_bounds{0, 0, 0, 0};
  } screenshot_presentation_state_;

  struct ScrollingCaptureOverlayState
  {
    bool active = false;
    HWND overlay_window = nullptr;
    HHOOK mouse_hook = nullptr;
    RECT selection_bounds{0, 0, 0, 0};
  } scrolling_capture_overlay_state_;

  struct CachedDisplayCapture
  {
    std::wstring display_id;
    RECT monitor_bounds{0, 0, 0, 0};
    double scale = 1.0;
    int rotation = 0;
    HBITMAP bitmap = nullptr;
  };

  std::vector<CachedDisplayCapture> cached_display_captures_;

  // Save/restore the previously focused window (Windows focus rules require explicit restore)
  void SavePreviousActiveWindow(HWND selfHwnd);
  void RestorePreviousActiveWindow(HWND selfHwnd);
  HWND NormalizeToRootWindow(HWND hwnd) const;
  bool ShouldSuppressBlurForActivatedWindow(HWND selfHwnd, HWND activatedHwnd);

  // Get the DPI scaling factor for the window
  float GetDpiScale(HWND hwnd);

  // Sync the hosted Flutter child window with the root client area.
  void SyncFlutterChildWindowToClientArea(HWND hwnd, const char *source, bool engine_handled);
  void FocusFlutterViewOrRoot(HWND hwnd);

  // Helpers for logging native geometry.
  std::string RectToString(const RECT &rect) const;
  RECT GetWindowRectSafe(HWND hwnd) const;
  void ReleaseDisplayCaptures(std::vector<CachedDisplayCapture> *captures);
  void ClearCachedDisplayCaptures();
  bool CaptureDisplaySnapshots(std::vector<CachedDisplayCapture> *captures_out, std::string *error_out, const std::optional<RECT> &logical_selection = std::nullopt);
  bool BuildDisplaySnapshotPayloads(const std::vector<CachedDisplayCapture> &captures, bool include_image_bytes, flutter::EncodableList *snapshots_out, std::string *error_out);
  const CachedDisplayCapture *FindCachedDisplayCapture(const std::string &display_id) const;
  bool CachedDisplayCapturesMatch(const std::vector<std::string> &display_ids) const;
  void PrepareCaptureWorkspace(HWND hwnd, const RECT &native_workspace_bounds);
  void RevealPreparedCaptureWorkspace(HWND hwnd);
  flutter::EncodableMap BuildCaptureWorkspaceResponse(const RECT &native_workspace_bounds) const;
  void BeginScrollingCaptureOverlay(HWND hwnd, const RECT &workspace_bounds, const RECT &selection_bounds, const RECT &controls_bounds);
  void DismissScrollingCaptureOverlay();
  void MoveScrollingCaptureControlsWindow(HWND hwnd, const RECT &controls_bounds);
  void SetScrollingCaptureControlsBackdrop(HWND hwnd, bool compact);
  HRGN CreateScrollingCaptureControlsRegion(int width, int height) const;
  void ApplyScrollingCaptureControlsRegion(HWND hwnd);
  void ClearScrollingCaptureControlsRegion();
  void PaintScrollingCaptureOverlay(HWND hwnd);
  void EmitScrollingCaptureWheelEvent();
  bool IsPointInScrollingCaptureSelection(POINT point) const;

  // Send window event to Flutter
  void SendWindowEvent(const std::string &eventName);

  // Handle method calls from Flutter
  void HandleWindowManagerMethodCall(
      const flutter::MethodCall<flutter::EncodableValue> &method_call,
      std::unique_ptr<flutter::MethodResult<flutter::EncodableValue>> result);

  // Dismiss the Windows Start Menu if it is currently open.
  // SetForegroundWindow requires no menus to be active.
  void DismissStartMenuIfOpen();

  // Static window procedure for handling window events
  static LRESULT CALLBACK WindowProc(HWND hwnd, UINT message, WPARAM wparam, LPARAM lparam);

  // Static child window procedure for observing the Flutter view hwnd.
  static LRESULT CALLBACK ChildWindowProc(HWND hwnd, UINT message, WPARAM wparam, LPARAM lparam);

  // Static overlay procedure for the passive scrolling screenshot mask.
  static LRESULT CALLBACK ScrollingCaptureOverlayWindowProc(HWND hwnd, UINT message, WPARAM wparam, LPARAM lparam);

  // Static low-level mouse hook for native scrolling screenshot wheel observation.
  static LRESULT CALLBACK ScrollingCaptureMouseHookProc(int code, WPARAM wparam, LPARAM lparam);

  // Track non-repeat keydowns that reached the Flutter child window. If the
  // matching keyup later lands on the root window and Flutter ignores it, we
  // use this set to decide whether the release should be sent back to the
  // child window.
  void TrackChildKeyDown(UINT message, WPARAM wparam, LPARAM lparam);
  void ClearTrackedChildKeyDown(UINT message, WPARAM wparam, LPARAM lparam);
  bool HasTrackedChildKeyDown(UINT message, WPARAM wparam, LPARAM lparam) const;
  bool RerouteIgnoredRootKeyUp(HWND hwnd, UINT message, WPARAM wparam, LPARAM lparam);
  void FlushPendingChildKeyUps();
  static uint64_t MakeKeyboardMessageSignature(UINT message, WPARAM wparam, LPARAM lparam);

  std::unordered_set<uint64_t> pending_child_keydowns_;
};

#endif // RUNNER_FLUTTER_WINDOW_H_

#include "flutter_window.h"

#include <cmath>
#include <cctype>
#include <optional>
#include <thread>
#include <string>
#include <flutter/plugin_registrar_windows.h>
#include <windows.h>
#include <dwmapi.h>

#include "flutter/generated_plugin_registrant.h"
#include "wox_webview/wox_webview_plugin.h"

#ifndef DWMWA_USE_IMMERSIVE_DARK_MODE
#define DWMWA_USE_IMMERSIVE_DARK_MODE 20
#endif

// After SW_HIDE, Windows may activate another window asynchronously.
// Retry restoring the previous foreground window shortly after hide.
static constexpr UINT_PTR kRestoreForegroundTimerId1 = 0xA11;
static constexpr UINT_PTR kRestoreForegroundTimerId2 = 0xA12;
static constexpr ULONGLONG kPostShowBlurGraceMs = 300;

// Store window instance for window procedure
FlutterWindow *g_window_instance = nullptr;

// Global log function
void LogMessage(const std::string &message)
{
  if (g_window_instance)
  {
    g_window_instance->Log(message);
  }
}

static std::optional<WORD> ParseWindowsVirtualKey(const std::string &key)
{
  if (key.size() == 1)
  {
    const unsigned char ch = static_cast<unsigned char>(key[0]);
    if (std::isalpha(ch))
    {
      return static_cast<WORD>(std::toupper(ch));
    }

    if (std::isdigit(ch))
    {
      return static_cast<WORD>(ch);
    }
  }

  if (key == "alt")
    return static_cast<WORD>(VK_LMENU);
  if (key == "control")
    return static_cast<WORD>(VK_LCONTROL);
  if (key == "shift")
    return static_cast<WORD>(VK_LSHIFT);
  if (key == "meta")
    return static_cast<WORD>(VK_LWIN);
  if (key == "escape")
    return static_cast<WORD>(VK_ESCAPE);
  if (key == "enter")
    return static_cast<WORD>(VK_RETURN);
  if (key == "tab")
    return static_cast<WORD>(VK_TAB);
  if (key == "space")
    return static_cast<WORD>(VK_SPACE);
  if (key == "arrowUp")
    return static_cast<WORD>(VK_UP);
  if (key == "arrowDown")
    return static_cast<WORD>(VK_DOWN);
  if (key == "arrowLeft")
    return static_cast<WORD>(VK_LEFT);
  if (key == "arrowRight")
    return static_cast<WORD>(VK_RIGHT);

  return std::nullopt;
}

static bool PostWindowsKeyMessage(HWND hwnd, WORD virtual_key, bool key_up, bool system_key)
{
  if (hwnd == nullptr)
  {
    return false;
  }

  UINT message = key_up ? (system_key ? WM_SYSKEYUP : WM_KEYUP) : (system_key ? WM_SYSKEYDOWN : WM_KEYDOWN);
  LPARAM lparam = 1;
  lparam |= static_cast<LPARAM>(MapVirtualKey(virtual_key, MAPVK_VK_TO_VSC)) << 16;
  if (system_key)
  {
    lparam |= static_cast<LPARAM>(1) << 29;
  }
  if (key_up)
  {
    lparam |= static_cast<LPARAM>(1) << 30;
    lparam |= static_cast<LPARAM>(1) << 31;
  }

  return PostMessage(hwnd, message, virtual_key, lparam) != 0;
}

static std::optional<DWORD> ParseWindowsMouseFlag(const std::string &button, bool button_up)
{
  if (button == "left")
    return button_up ? MOUSEEVENTF_LEFTUP : MOUSEEVENTF_LEFTDOWN;
  if (button == "right")
    return button_up ? MOUSEEVENTF_RIGHTUP : MOUSEEVENTF_RIGHTDOWN;
  if (button == "middle")
    return button_up ? MOUSEEVENTF_MIDDLEUP : MOUSEEVENTF_MIDDLEDOWN;
  return std::nullopt;
}

static bool SendWindowsMouseButtonInput(DWORD mouse_flag)
{
  INPUT input = {};
  input.type = INPUT_MOUSE;
  input.mi.dwFlags = mouse_flag;
  return SendInput(1, &input, sizeof(INPUT)) == 1;
}

FlutterWindow::FlutterWindow(const flutter::DartProject &project)
    : project_(project),
      original_window_proc_(nullptr),
      previous_active_window_(nullptr)
{
  g_window_instance = this;
}

FlutterWindow::~FlutterWindow()
{
  // Clear global instance
  if (g_window_instance == this)
  {
    g_window_instance = nullptr;
  }
}

void FlutterWindow::Log(const std::string &message)
{
  if (window_manager_channel_)
  {
    window_manager_channel_->InvokeMethod("log", std::make_unique<flutter::EncodableValue>(message));
  }
}

HWND FlutterWindow::NormalizeToRootWindow(HWND hwnd) const
{
  if (hwnd == nullptr)
  {
    return nullptr;
  }

  HWND root = GetAncestor(hwnd, GA_ROOTOWNER);
  if (root == nullptr)
  {
    root = GetAncestor(hwnd, GA_ROOT);
  }
  if (root == nullptr)
  {
    root = hwnd;
  }

  return root;
}

bool FlutterWindow::ShouldSuppressBlurForActivatedWindow(HWND selfHwnd, HWND activatedHwnd)
{
  if (selfHwnd == nullptr || activatedHwnd == nullptr)
  {
    return false;
  }

  HWND selfRoot = NormalizeToRootWindow(selfHwnd);
  if (selfRoot == nullptr)
  {
    selfRoot = selfHwnd;
  }

  HWND activatedRoot = NormalizeToRootWindow(activatedHwnd);
  if (activatedRoot == nullptr)
  {
    activatedRoot = activatedHwnd;
  }

  if (activatedRoot == selfRoot || IsChild(selfRoot, activatedHwnd) || IsChild(selfRoot, activatedRoot))
  {
    Log("WM_ACTIVATE: WA_INACTIVE suppressed (same Wox window tree)");
    return true;
  }

  DWORD selfPid = 0;
  DWORD activatedPid = 0;
  GetWindowThreadProcessId(selfRoot, &selfPid);
  GetWindowThreadProcessId(activatedRoot, &activatedPid);
  if (selfPid != 0 && selfPid == activatedPid)
  {
    Log("WM_ACTIVATE: WA_INACTIVE suppressed (same process native host)");
    return true;
  }

  return false;
}

void FlutterWindow::SavePreviousActiveWindow(HWND selfHwnd)
{
  if (selfHwnd == nullptr)
  {
    return;
  }

  HWND fg = GetForegroundWindow();
  if (fg == nullptr)
  {
    return;
  }

  // Normalize to root window (avoid saving child controls)
  HWND root = NormalizeToRootWindow(fg);
  if (root == nullptr)
  {
    root = fg;
  }

  if (root == selfHwnd)
  {
    return;
  }

  if (!IsWindow(root) || !IsWindowVisible(root))
  {
    return;
  }

  previous_active_window_ = root;
  restore_previous_window_on_hide_ = true;

  char fgStr[32];
  sprintf_s(fgStr, "%p", previous_active_window_);
  Log(std::string("Window: saved previous foreground hwnd=") + fgStr);
}

void FlutterWindow::RestorePreviousActiveWindow(HWND selfHwnd)
{
  if (selfHwnd == nullptr)
  {
    return;
  }

  HWND prev = previous_active_window_;
  if (prev == nullptr)
  {
    Log("Window: no previous foreground window saved");
    return;
  }

  // Normalize again (in case we saved a non-root window in the past)
  HWND root = NormalizeToRootWindow(prev);
  if (root != nullptr)
  {
    prev = root;
  }

  if (prev == selfHwnd)
  {
    Log("Window: previous foreground is self, skip restore");
    return;
  }

  if (!IsWindow(prev))
  {
    Log("Window: previous foreground hwnd is invalid (destroyed?)");
    previous_active_window_ = nullptr;
    return;
  }

  char prevStr[32];
  sprintf_s(prevStr, "%p", prev);
  Log(std::string("Window: restoring previous foreground hwnd=") + prevStr);

  // If the previous window is minimized, do not restore it.
  // The user might have minimized it explicitly, and Wox being an overlay shouldn't change the window layout.
  if (IsIconic(prev))
  {
    Log("Window: previous foreground is minimized, skipping restore");
    previous_active_window_ = nullptr;
    return;
  }

  // Fast path: try directly.
  if (SetForegroundWindow(prev))
  {
    BringWindowToTop(prev);
    return;
  }

  // Fallback: Attach input queues temporarily.
  DWORD curTid = GetCurrentThreadId();
  DWORD prevTid = GetWindowThreadProcessId(prev, nullptr);
  bool attached = false;
  if (prevTid != 0 && prevTid != curTid)
  {
    attached = AttachThreadInput(prevTid, curTid, TRUE);
  }

  SetForegroundWindow(prev);
  BringWindowToTop(prev);

  if (attached)
  {
    AttachThreadInput(prevTid, curTid, FALSE);
  }

  if (GetForegroundWindow() == prev)
  {
    Log("Window: restore foreground succeeded (AttachThreadInput)");
    return;
  }

  // Last try: relax foreground restrictions.
  AllowSetForegroundWindow(ASFW_ANY);
  SetForegroundWindow(prev);
  BringWindowToTop(prev);
  Log("Window: restore foreground final attempt completed");
}

void FlutterWindow::DismissStartMenuIfOpen()
{
  HWND fg = GetForegroundWindow();
  if (!fg)
    return;

  WCHAR className[256] = {0};
  GetClassNameW(fg, className, 256);

  DWORD pid = 0;
  GetWindowThreadProcessId(fg, &pid);

  // Get process name for detection
  WCHAR exePath[MAX_PATH] = {0};
  WCHAR *fileName = nullptr;
  bool gotProcessName = false;

  if (pid != 0)
  {
    HANDLE hProcess = OpenProcess(PROCESS_QUERY_LIMITED_INFORMATION, FALSE, pid);
    if (hProcess)
    {
      DWORD pathLen = MAX_PATH;
      if (QueryFullProcessImageNameW(hProcess, 0, exePath, &pathLen))
      {
        gotProcessName = true;
        fileName = wcsrchr(exePath, L'\\');
        if (fileName)
          fileName++;
        else
          fileName = exePath;
      }
      CloseHandle(hProcess);
    }
  }

  // Detect Start Menu / Search overlay by window class or process name
  bool isStartMenu = false;

  // UWP apps (Start Menu, Search) use this window class
  if (wcscmp(className, L"Windows.UI.Core.CoreWindow") == 0)
  {
    isStartMenu = true;
  }

  if (!isStartMenu && gotProcessName && fileName)
  {
    if (_wcsicmp(fileName, L"StartMenuExperienceHost.exe") == 0 ||
        _wcsicmp(fileName, L"SearchHost.exe") == 0 ||
        _wcsicmp(fileName, L"SearchApp.exe") == 0 ||
        _wcsicmp(fileName, L"ShellExperienceHost.exe") == 0)
    {
      isStartMenu = true;
    }
  }

  if (!isStartMenu)
    return;

  Log("Focus: Start Menu detected, dismissing with WM_CLOSE");

  // Clear saved previous window if it was the Start Menu -- we don't want to
  // restore it when Wox hides.
  if (previous_active_window_ == fg || previous_active_window_ == GetAncestor(fg, GA_ROOT))
  {
    previous_active_window_ = nullptr;
  }

  // Post WM_CLOSE to dismiss the Start Menu window.
  // PostMessage bypasses UIPI restrictions that block SendInput.
  PostMessage(fg, WM_CLOSE, 0, 0);
  Sleep(200);
}

// Get the DPI scaling factor for the window
float FlutterWindow::GetDpiScale(HWND hwnd)
{
  // Default DPI is 96
  float dpiScale = 1.0f;

  // Try to use GetDpiForWindow which is available on Windows 10 1607 and later
  HMODULE user32 = GetModuleHandle(TEXT("user32.dll"));
  if (user32)
  {
    typedef UINT(WINAPI * GetDpiForWindowFunc)(HWND);
    GetDpiForWindowFunc getDpiForWindow =
        reinterpret_cast<GetDpiForWindowFunc>(GetProcAddress(user32, "GetDpiForWindow"));

    if (getDpiForWindow)
    {
      UINT dpi = getDpiForWindow(hwnd);
      dpiScale = dpi / 96.0f;
    }
    else
    {
      // Fallback for older Windows versions
      HDC hdc = GetDC(hwnd);
      if (hdc)
      {
        int dpiX = GetDeviceCaps(hdc, LOGPIXELSX);
        dpiScale = dpiX / 96.0f;
        ReleaseDC(hwnd, hdc);
      }
    }
  }

  return dpiScale;
}

bool FlutterWindow::OnCreate()
{
  if (!Win32Window::OnCreate())
  {
    return false;
  }

  RECT frame = GetClientArea();

  // The size here must match the window dimensions to avoid unnecessary surface
  // creation / destruction in the startup path.
  flutter_controller_ = std::make_unique<flutter::FlutterViewController>(frame.right - frame.left, frame.bottom - frame.top, project_);
  // Ensure that basic setup of the controller was successful.
  if (!flutter_controller_->engine() || !flutter_controller_->view())
  {
    return false;
  }
  RegisterPlugins(flutter_controller_->engine());
  RegisterWoxWebviewPlugin(flutter_controller_->engine()->GetRegistrarForPlugin("WoxWebviewPlugin"));

  // Set up window manager method channel
  window_manager_channel_ = std::make_unique<flutter::MethodChannel<flutter::EncodableValue>>(
      flutter_controller_->engine()->messenger(), "com.wox.windows_window_manager",
      &flutter::StandardMethodCodec::GetInstance());

  window_manager_channel_->SetMethodCallHandler(
      [this](const auto &call, auto result)
      {
        HandleWindowManagerMethodCall(call, std::move(result));
      });

  // Replace the window procedure to capture window events
  HWND hwnd = GetHandle();
  if (hwnd != nullptr)
  {
    original_window_proc_ = reinterpret_cast<WNDPROC>(GetWindowLongPtr(hwnd, GWLP_WNDPROC));
    SetWindowLongPtr(hwnd, GWLP_WNDPROC, reinterpret_cast<LONG_PTR>(WindowProc));
  }

  SetChildContent(flutter_controller_->view()->GetNativeWindow());

  flutter_controller_->engine()->SetNextFrameCallback([&]()
                                                      {
                                                        // hidden-at-launch
                                                        // this->Show();
                                                      });

  // Flutter can complete the first frame before the "show window" callback is
  // registered. The following call ensures a frame is pending to ensure the
  // window is shown. It is a no-op if the first frame hasn't completed yet.
  flutter_controller_->ForceRedraw();

  return true;
}

void FlutterWindow::OnDestroy()
{
  // Restore original window procedure
  HWND hwnd = GetHandle();
  if (hwnd != nullptr && original_window_proc_ != nullptr)
  {
    SetWindowLongPtr(hwnd, GWLP_WNDPROC, reinterpret_cast<LONG_PTR>(original_window_proc_));
  }

  if (flutter_controller_)
  {
    flutter_controller_ = nullptr;
  }

  Win32Window::OnDestroy();
}

LRESULT
FlutterWindow::MessageHandler(HWND hwnd, UINT const message, WPARAM const wparam, LPARAM const lparam) noexcept
{
  // Give Flutter, including plugins, an opportunity to handle window messages.
  if (flutter_controller_)
  {
    std::optional<LRESULT> result = flutter_controller_->HandleTopLevelWindowProc(hwnd, message, wparam, lparam);

    if (result)
    {
      return *result;
    }
  }

  switch (message)
  {
  case WM_TIMER:
    if (wparam == kRestoreForegroundTimerId1 || wparam == kRestoreForegroundTimerId2)
    {
      KillTimer(hwnd, static_cast<UINT_PTR>(wparam));
      // Only restore when this window is still hidden.
      if (IsWindowVisible(hwnd) == 0)
      {
        RestorePreviousActiveWindow(hwnd);
      }
      return 0;
    }
    break;
  case WM_FONTCHANGE:
    flutter_controller_->engine()->ReloadSystemFonts();
    break;
  }

  return Win32Window::MessageHandler(hwnd, message, wparam, lparam);
}

void FlutterWindow::SendWindowEvent(const std::string &eventName)
{
  if (window_manager_channel_)
  {
    window_manager_channel_->InvokeMethod(eventName, std::make_unique<flutter::EncodableValue>(flutter::EncodableMap()));
  }
}

LRESULT CALLBACK FlutterWindow::WindowProc(HWND hwnd, UINT message, WPARAM wparam, LPARAM lparam)
{
  // If window instance is not available, use default window procedure
  if (g_window_instance == nullptr || g_window_instance->original_window_proc_ == nullptr)
  {
    return DefWindowProc(hwnd, message, wparam, lparam);
  }

  // Handle window messages and send events to Flutter
  switch (message)
  {
  case WM_ACTIVATE:
    if (LOWORD(wparam) == WA_ACTIVE || LOWORD(wparam) == WA_CLICKACTIVE)
    {
      // g_window_instance->SendWindowEvent("onWindowFocus");
    }
    else
    {
      HWND activatedHwnd = reinterpret_cast<HWND>(lparam);
      if (!g_window_instance->ShouldSuppressBlurForActivatedWindow(hwnd, activatedHwnd))
      {
        const bool in_post_show_grace = GetTickCount64() < g_window_instance->blur_guard_until_tick_;
        if (g_window_instance->blur_guard_active_ || in_post_show_grace)
        {
          if (g_window_instance->blur_guard_active_)
          {
            g_window_instance->Log("WM_ACTIVATE: WA_INACTIVE suppressed (show-to-focus transition)");
          }
          else
          {
            g_window_instance->Log("WM_ACTIVATE: WA_INACTIVE suppressed (post-show grace)");
          }
        }
        else
        {
          g_window_instance->restore_previous_window_on_hide_ = false;
          g_window_instance->previous_active_window_ = nullptr;
          g_window_instance->SendWindowEvent("onWindowBlur");
        }
      }
    }
    break;
  }

  // Call the original window procedure
  return CallWindowProc(g_window_instance->original_window_proc_, hwnd, message, wparam, lparam);
}

void FlutterWindow::HandleWindowManagerMethodCall(
    const flutter::MethodCall<flutter::EncodableValue> &method_call,
    std::unique_ptr<flutter::MethodResult<flutter::EncodableValue>> result)
{
  const std::string &method_name = method_call.method_name();
  HWND hwnd = GetHandle();

  if (hwnd == nullptr)
  {
    result->Error("WINDOW_ERROR", "Failed to get window handle");
    return;
  }

  try
  {
    if (method_name == "inputKeyDown" || method_name == "inputKeyUp")
    {
      const auto *arguments = std::get_if<flutter::EncodableMap>(method_call.arguments());
      if (arguments == nullptr)
      {
        result->Error("INVALID_ARGUMENTS", "Missing arguments for keyboard input");
        return;
      }

      auto key_it = arguments->find(flutter::EncodableValue("key"));
      if (key_it == arguments->end())
      {
        result->Error("INVALID_ARGUMENTS", "Missing key for keyboard input");
        return;
      }

      const auto *key = std::get_if<std::string>(&key_it->second);
      if (key == nullptr)
      {
        result->Error("INVALID_ARGUMENTS", "Key must be a string");
        return;
      }

      auto virtual_key = ParseWindowsVirtualKey(*key);
      if (!virtual_key.has_value())
      {
        result->Error("UNSUPPORTED_KEY", "Unsupported key for Windows system input");
        return;
      }

      const bool key_up = method_name == "inputKeyUp";
      const bool is_alt = *virtual_key == VK_MENU || *virtual_key == VK_LMENU || *virtual_key == VK_RMENU;
      const bool handled = PostWindowsKeyMessage(hwnd, *virtual_key, key_up, is_alt);
      if (!handled)
      {
        result->Error("INPUT_ERROR", "Failed to send keyboard input");
        return;
      }

      result->Success();
    }
    else if (method_name == "inputMouseMove")
    {
      const auto *arguments = std::get_if<flutter::EncodableMap>(method_call.arguments());
      if (arguments == nullptr)
      {
        result->Error("INVALID_ARGUMENTS", "Missing arguments for mouse move");
        return;
      }

      auto x_it = arguments->find(flutter::EncodableValue("x"));
      auto y_it = arguments->find(flutter::EncodableValue("y"));
      if (x_it == arguments->end() || y_it == arguments->end())
      {
        result->Error("INVALID_ARGUMENTS", "Missing coordinates for mouse move");
        return;
      }

      double x = std::get<double>(x_it->second);
      double y = std::get<double>(y_it->second);
      if (!SetCursorPos(static_cast<int>(std::lround(x)), static_cast<int>(std::lround(y))))
      {
        result->Error("INPUT_ERROR", "Failed to move mouse cursor");
        return;
      }

      result->Success();
    }
    else if (method_name == "inputMouseDown" || method_name == "inputMouseUp")
    {
      const auto *arguments = std::get_if<flutter::EncodableMap>(method_call.arguments());
      if (arguments == nullptr)
      {
        result->Error("INVALID_ARGUMENTS", "Missing arguments for mouse button input");
        return;
      }

      auto button_it = arguments->find(flutter::EncodableValue("button"));
      if (button_it == arguments->end())
      {
        result->Error("INVALID_ARGUMENTS", "Missing mouse button");
        return;
      }

      const auto *button = std::get_if<std::string>(&button_it->second);
      if (button == nullptr)
      {
        result->Error("INVALID_ARGUMENTS", "Mouse button must be a string");
        return;
      }

      auto mouse_flag = ParseWindowsMouseFlag(*button, method_name == "inputMouseUp");
      if (!mouse_flag.has_value())
      {
        result->Error("UNSUPPORTED_BUTTON", "Unsupported mouse button for Windows system input");
        return;
      }

      if (!SendWindowsMouseButtonInput(*mouse_flag))
      {
        result->Error("INPUT_ERROR", "Failed to send mouse button input");
        return;
      }

      result->Success();
    }
    else if (method_name == "setSize")
    {
      const auto *arguments = std::get_if<flutter::EncodableMap>(method_call.arguments());
      if (arguments)
      {
        auto width_it = arguments->find(flutter::EncodableValue("width"));
        auto height_it = arguments->find(flutter::EncodableValue("height"));
        if (width_it != arguments->end() && height_it != arguments->end())
        {
          double width = std::get<double>(width_it->second);
          double height = std::get<double>(height_it->second);

          // Get DPI scale factor
          float dpiScale = GetDpiScale(hwnd);

          // Apply DPI scaling to get physical pixels
          int scaledWidth = static_cast<int>(width * dpiScale);
          int scaledHeight = static_cast<int>(height * dpiScale);

          RECT rect;
          GetWindowRect(hwnd, &rect);
          SetWindowPos(hwnd, nullptr, rect.left, rect.top, scaledWidth, scaledHeight, SWP_NOZORDER | SWP_FRAMECHANGED);

          // Force Flutter to redraw immediately to match the new window size
          if (flutter_controller_)
          {
            flutter_controller_->ForceRedraw();
          }

          result->Success();
        }
        else
        {
          result->Error("INVALID_ARGUMENTS", "Invalid arguments for setSize");
        }
      }
      else
      {
        result->Error("INVALID_ARGUMENTS", "Invalid arguments for setSize");
      }
    }
    else if (method_name == "setBounds")
    {
      const auto *arguments = std::get_if<flutter::EncodableMap>(method_call.arguments());
      if (arguments)
      {
        auto x_it = arguments->find(flutter::EncodableValue("x"));
        auto y_it = arguments->find(flutter::EncodableValue("y"));
        auto width_it = arguments->find(flutter::EncodableValue("width"));
        auto height_it = arguments->find(flutter::EncodableValue("height"));
        if (x_it != arguments->end() && y_it != arguments->end() && width_it != arguments->end() && height_it != arguments->end())
        {
          double x = std::get<double>(x_it->second);
          double y = std::get<double>(y_it->second);
          double width = std::get<double>(width_it->second);
          double height = std::get<double>(height_it->second);

          struct MonitorFindData
          {
            LONG targetX, targetY;
            HMONITOR foundMonitor;
            UINT foundDpi;
          } findData = {static_cast<LONG>(x), static_cast<LONG>(y), nullptr, 96};

          EnumDisplayMonitors(nullptr, nullptr, [](HMONITOR hMon, HDC, LPRECT, LPARAM lParam) -> BOOL
                              {
                                auto *data = reinterpret_cast<MonitorFindData *>(lParam);
                                MONITORINFO mi = {sizeof(mi)};
                                if (GetMonitorInfo(hMon, &mi))
                                {
                                  UINT dpi = FlutterDesktopGetDpiForMonitor(hMon);
                                  float scale = dpi / 96.0f;

                                  LONG logLeft = static_cast<LONG>(mi.rcMonitor.left / scale);
                                  LONG logTop = static_cast<LONG>(mi.rcMonitor.top / scale);
                                  LONG logRight = static_cast<LONG>(mi.rcMonitor.right / scale);
                                  LONG logBottom = static_cast<LONG>(mi.rcMonitor.bottom / scale);

                                  if (data->targetX >= logLeft && data->targetX < logRight &&
                                      data->targetY >= logTop && data->targetY < logBottom)
                                  {
                                    data->foundMonitor = hMon;
                                    data->foundDpi = dpi;
                                    return FALSE;
                                  }
                                }
                                return TRUE; }, reinterpret_cast<LPARAM>(&findData));

          if (findData.foundMonitor == nullptr)
          {
            findData.foundMonitor = MonitorFromPoint({0, 0}, MONITOR_DEFAULTTOPRIMARY);
            findData.foundDpi = FlutterDesktopGetDpiForMonitor(findData.foundMonitor);
          }

          float dpiScale = findData.foundDpi / 96.0f;
          int scaledX = static_cast<int>(x * dpiScale);
          int scaledY = static_cast<int>(y * dpiScale);
          int scaledWidth = static_cast<int>(width * dpiScale);
          int scaledHeight = static_cast<int>(height * dpiScale);

          SetWindowPos(hwnd, nullptr, scaledX, scaledY, scaledWidth, scaledHeight, SWP_NOZORDER | SWP_FRAMECHANGED);

          if (flutter_controller_)
          {
            flutter_controller_->ForceRedraw();
          }

          result->Success();
        }
        else
        {
          result->Error("INVALID_ARGUMENTS", "Invalid arguments for setBounds");
        }
      }
      else
      {
        result->Error("INVALID_ARGUMENTS", "Invalid arguments for setBounds");
      }
    }
    else if (method_name == "getPosition")
    {
      RECT rect;
      GetWindowRect(hwnd, &rect);

      // Get DPI scale factor
      float dpiScale = GetDpiScale(hwnd);

      // Apply DPI scaling to logical pixels (physical to logical)
      double scaledX = static_cast<double>(rect.left) / dpiScale;
      double scaledY = static_cast<double>(rect.top) / dpiScale;

      flutter::EncodableMap position;
      position[flutter::EncodableValue("x")] = flutter::EncodableValue(scaledX);
      position[flutter::EncodableValue("y")] = flutter::EncodableValue(scaledY);
      result->Success(flutter::EncodableValue(position));
    }
    else if (method_name == "getSize")
    {
      RECT rect;
      GetWindowRect(hwnd, &rect);

      // Get DPI scale factor
      float dpiScale = GetDpiScale(hwnd);

      // Convert physical pixels to logical pixels
      double width = static_cast<double>(rect.right - rect.left) / dpiScale;
      double height = static_cast<double>(rect.bottom - rect.top) / dpiScale;

      flutter::EncodableMap size;
      size[flutter::EncodableValue("width")] = flutter::EncodableValue(width);
      size[flutter::EncodableValue("height")] = flutter::EncodableValue(height);
      result->Success(flutter::EncodableValue(size));
    }
    else if (method_name == "setPosition")
    {
      const auto *arguments = std::get_if<flutter::EncodableMap>(method_call.arguments());
      if (arguments)
      {
        auto x_it = arguments->find(flutter::EncodableValue("x"));
        auto y_it = arguments->find(flutter::EncodableValue("y"));
        if (x_it != arguments->end() && y_it != arguments->end())
        {
          double x = std::get<double>(x_it->second);
          double y = std::get<double>(y_it->second);

          // COORDINATE SYSTEM EXPLANATION:
          // ... (existing logic) ...

          struct MonitorFindData
          {
            LONG targetX, targetY;
            HMONITOR foundMonitor;
            UINT foundDpi;
          } findData = {static_cast<LONG>(x), static_cast<LONG>(y), nullptr, 96};

          // Enumerate all monitors to find which one contains our logical point
          EnumDisplayMonitors(nullptr, nullptr, [](HMONITOR hMon, HDC, LPRECT, LPARAM lParam) -> BOOL
                              {
                                auto *data = reinterpret_cast<MonitorFindData *>(lParam);
                                MONITORINFO mi = {sizeof(mi)};
                                if (GetMonitorInfo(hMon, &mi))
                                {
                                  UINT dpi = FlutterDesktopGetDpiForMonitor(hMon);
                                  float scale = dpi / 96.0f;

                                  LONG logLeft = static_cast<LONG>(mi.rcMonitor.left / scale);
                                  LONG logTop = static_cast<LONG>(mi.rcMonitor.top / scale);
                                  LONG logRight = static_cast<LONG>(mi.rcMonitor.right / scale);
                                  LONG logBottom = static_cast<LONG>(mi.rcMonitor.bottom / scale);

                                  if (data->targetX >= logLeft && data->targetX < logRight &&
                                      data->targetY >= logTop && data->targetY < logBottom)
                                  {
                                    data->foundMonitor = hMon;
                                    data->foundDpi = dpi;
                                    return FALSE; // Found the correct monitor, stop enumeration
                                  }
                                }
                                return TRUE; // Not this monitor, continue searching
                              },
                              reinterpret_cast<LPARAM>(&findData));

          if (findData.foundMonitor == nullptr)
          {
            findData.foundMonitor = MonitorFromPoint({0, 0}, MONITOR_DEFAULTTOPRIMARY);
            findData.foundDpi = FlutterDesktopGetDpiForMonitor(findData.foundMonitor);
          }

          float dpiScale = findData.foundDpi / 96.0f;
          int scaledX = static_cast<int>(x * dpiScale);
          int scaledY = static_cast<int>(y * dpiScale);

          RECT rect;
          GetWindowRect(hwnd, &rect);
          int width = rect.right - rect.left;
          int height = rect.bottom - rect.top;
          SetWindowPos(hwnd, nullptr, scaledX, scaledY, width, height, SWP_NOZORDER | SWP_NOSIZE);
          result->Success();
        }
        else
        {
          result->Error("INVALID_ARGUMENTS", "Invalid arguments for setPosition");
        }
      }
      else
      {
        result->Error("INVALID_ARGUMENTS", "Invalid arguments for setPosition");
      }
    }
    else if (method_name == "center")
    {
      const auto *arguments = std::get_if<flutter::EncodableMap>(method_call.arguments());
      if (!arguments)
      {
        result->Error("INVALID_ARGUMENTS", "Arguments must be provided for center");
        return;
      }

      auto width_it = arguments->find(flutter::EncodableValue("width"));
      auto height_it = arguments->find(flutter::EncodableValue("height"));

      if (width_it == arguments->end() || height_it == arguments->end())
      {
        result->Error("INVALID_ARGUMENTS", "Both width and height must be provided for center");
        return;
      }

      double width = std::get<double>(width_it->second);
      double height = std::get<double>(height_it->second);

      // Get cursor position to determine which monitor to center on
      POINT cursorPos;
      GetCursorPos(&cursorPos);

      // Get the monitor where the cursor is located
      HMONITOR hMonitor = MonitorFromPoint(cursorPos, MONITOR_DEFAULTTONEAREST);
      MONITORINFO monitorInfo;
      monitorInfo.cbSize = sizeof(MONITORINFO);

      if (!GetMonitorInfo(hMonitor, &monitorInfo))
      {
        result->Error("MONITOR_ERROR", "Failed to get monitor info");
        return;
      }

      // Get DPI scale factor for the target monitor
      UINT dpi = FlutterDesktopGetDpiForMonitor(hMonitor);
      float dpiScale = dpi / 96.0f;

      // Apply DPI scaling to get physical pixels
      int scaledWidth = static_cast<int>(width * dpiScale);
      int scaledHeight = static_cast<int>(height * dpiScale);

      // Get monitor work area (physical coordinates), excluding taskbar
      int monitorLeft = monitorInfo.rcWork.left;
      int monitorTop = monitorInfo.rcWork.top;
      int monitorWidth = monitorInfo.rcWork.right - monitorInfo.rcWork.left;
      int monitorHeight = monitorInfo.rcWork.bottom - monitorInfo.rcWork.top;

      // Calculate center position on the mouse's monitor
      int x = monitorLeft + (monitorWidth - scaledWidth) / 2;
      int y = monitorTop + (monitorHeight - scaledHeight) / 2;

      Log("Center: window to " + std::to_string(x) + "," + std::to_string(y) + " with " + std::to_string(scaledWidth) + "," + std::to_string(scaledHeight) + " on monitor at " + std::to_string(monitorLeft) + "," + std::to_string(monitorTop));
      SetWindowPos(hwnd, nullptr, x, y, scaledWidth, scaledHeight, SWP_NOZORDER);
      result->Success();
    }
    else if (method_name == "show")
    {
      SavePreviousActiveWindow(hwnd);
      // Suppress transient blur events that fire between show() and the
      // subsequent focus() call from Dart.  Without this, Windows may
      // deactivate the newly-shown window (e.g. Explorer steals focus),
      // sending WM_ACTIVATE/WA_INACTIVE before focus() has a chance to
      // grab the foreground, which causes onWindowBlur -> hideApp().
      blur_guard_active_ = true;
      blur_guard_until_tick_ = GetTickCount64() + kPostShowBlurGraceMs;
      ShowWindow(hwnd, SW_SHOW);
      result->Success();
    }
    else if (method_name == "hide")
    {
      Log("[KEYLOG][NATIVE] Hide called, using ShowWindow(SW_HIDE)");
      blur_guard_active_ = false;
      blur_guard_until_tick_ = 0;

      HWND fg = GetForegroundWindow();
      bool isForeground = (fg == hwnd || fg == GetAncestor(hwnd, GA_ROOT));
      bool shouldRestorePreviousWindow = restore_previous_window_on_hide_;

      ShowWindow(hwnd, SW_HIDE);

      if (isForeground && shouldRestorePreviousWindow)
      {
        RestorePreviousActiveWindow(hwnd);

        // Retry restore after the system finishes processing activation changes.
        KillTimer(hwnd, kRestoreForegroundTimerId1);
        KillTimer(hwnd, kRestoreForegroundTimerId2);
        SetTimer(hwnd, kRestoreForegroundTimerId1, 30, nullptr);
        SetTimer(hwnd, kRestoreForegroundTimerId2, 200, nullptr);
      }
      else if (!shouldRestorePreviousWindow)
      {
        Log("Window: Wox already lost focus before hiding, skipping restore");
        previous_active_window_ = nullptr;
        KillTimer(hwnd, kRestoreForegroundTimerId1);
        KillTimer(hwnd, kRestoreForegroundTimerId2);
      }
      else
      {
        Log("Window: Wox is not foreground when hiding, skipping restore");
        previous_active_window_ = nullptr;
        KillTimer(hwnd, kRestoreForegroundTimerId1);
        KillTimer(hwnd, kRestoreForegroundTimerId2);
      }

      restore_previous_window_on_hide_ = false;

      result->Success();
    }
    else if (method_name == "focus")
    {
      // If the Start Menu or Search overlay is open, dismiss it first.
      // SetForegroundWindow requires "no menus are active" to succeed.
      DismissStartMenuIfOpen();

      // Save current foreground window before bringing Wox to front.
      SavePreviousActiveWindow(hwnd);

      // Optimization: Try SetForegroundWindow directly first.
      // If we already have permission or are in foreground, this avoids AttachThreadInput
      // which can block for seconds if the foreground window is hung.
      if (SetForegroundWindow(hwnd))
      {
        SetFocus(hwnd);
        BringWindowToTop(hwnd);
        blur_guard_active_ = false;
        result->Success();
        return;
      }

      HWND fg = GetForegroundWindow();
      DWORD curTid = GetCurrentThreadId();
      DWORD fgTid = 0;
      if (fg)
      {
        fgTid = GetWindowThreadProcessId(fg, nullptr);
      }

      bool attached = false;
      if (fg && fgTid != 0 && fgTid != curTid)
      {
        attached = AttachThreadInput(fgTid, curTid, TRUE);
      }

      SetForegroundWindow(hwnd);
      SetFocus(hwnd);
      BringWindowToTop(hwnd);

      if (attached)
      {
        AttachThreadInput(fgTid, curTid, FALSE);
      }

      if (GetForegroundWindow() == hwnd)
      {
        Log("Focus: use attach thread input");
        blur_guard_active_ = false;
        result->Success();
        return;
      }

      INPUT pInputs[2];
      ZeroMemory(pInputs, sizeof(INPUT));

      pInputs[0].type = INPUT_KEYBOARD;
      pInputs[0].ki.wVk = VK_MENU; // Alt down
      pInputs[0].ki.dwFlags = 0;

      pInputs[1].type = INPUT_KEYBOARD;
      pInputs[1].ki.wVk = VK_MENU; // Alt up
      pInputs[1].ki.dwFlags = KEYEVENTF_KEYUP;

      SendInput(2, pInputs, sizeof(INPUT));
      Sleep(10);

      SetForegroundWindow(hwnd);
      SetFocus(hwnd);
      BringWindowToTop(hwnd);

      if (GetForegroundWindow() == hwnd)
      {
        Log("Focus: use Alt key injection");
        blur_guard_active_ = false;
        result->Success();
        return;
      }

      Log("Focus: both methods failed, trying AllowSetForegroundWindow");
      AllowSetForegroundWindow(ASFW_ANY);
      SetForegroundWindow(hwnd);
      SetFocus(hwnd);

      Log("Focus: final attempt completed");
      blur_guard_active_ = false;
      result->Success();
    }
    else if (method_name == "isVisible")
    {
      bool is_visible = IsWindowVisible(hwnd) != 0;
      result->Success(flutter::EncodableValue(is_visible));
    }
    else if (method_name == "setAlwaysOnTop")
    {
      const auto *arguments = std::get_if<bool>(method_call.arguments());
      if (arguments)
      {
        bool always_on_top = *arguments;
        SetWindowPos(hwnd, always_on_top ? HWND_TOPMOST : HWND_NOTOPMOST, 0, 0, 0, 0, SWP_NOMOVE | SWP_NOSIZE);
        result->Success();
      }
      else
      {
        result->Error("INVALID_ARGUMENTS", "Invalid arguments for setAlwaysOnTop");
      }
    }
    else if (method_name == "setAppearance")
    {
      const auto *arguments = std::get_if<std::string>(method_call.arguments());
      if (arguments)
      {
        std::string appearance = *arguments;
        BOOL useDark = (appearance == "dark");
        DwmSetWindowAttribute(hwnd, DWMWA_USE_IMMERSIVE_DARK_MODE, &useDark, sizeof(useDark));
        result->Success();
      }
      else
      {
        result->Error("INVALID_ARGUMENTS", "Invalid arguments for setAppearance");
      }
    }
    else if (method_name == "startDragging")
    {
      ReleaseCapture();
      SendMessage(hwnd, WM_NCLBUTTONDOWN, HTCAPTION, 0);
      result->Success();
    }
    else if (method_name == "waitUntilReadyToShow")
    {
      result->Success();
    }
    else
    {
      result->NotImplemented();
    }
  }
  catch (const std::exception &e)
  {
    result->Error("EXCEPTION", std::string("Exception: ") + e.what());
  }
  catch (...)
  {
    result->Error("EXCEPTION", "Unknown exception occurred");
  }
}

#include "flutter_window.h"

#include <cmath>
#include <cctype>
#include <cstdint>
#include <optional>
#include <mutex>
#include <sstream>
#include <string>
#include <thread>
#include <vector>
#include <flutter/plugin_registrar_windows.h>
#include <windows.h>
#include <dwmapi.h>
#include <gdiplus.h>
#include <objidl.h>

#include "flutter/generated_plugin_registrant.h"
#include "utils.h"
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
static std::once_flag g_gdiplus_init_once;
static ULONG_PTR g_gdiplus_token = 0;

static void EnsureGdiplusInitialized()
{
  std::call_once(g_gdiplus_init_once, []() {
    Gdiplus::GdiplusStartupInput startup_input;
    Gdiplus::GdiplusStartup(&g_gdiplus_token, &startup_input, nullptr);
  });
}

static std::string Base64Encode(const std::vector<uint8_t> &data)
{
  static constexpr char kAlphabet[] = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/";
  std::string encoded;
  encoded.reserve(((data.size() + 2) / 3) * 4);

  size_t index = 0;
  while (index + 2 < data.size())
  {
    const uint32_t value = (static_cast<uint32_t>(data[index]) << 16) |
                           (static_cast<uint32_t>(data[index + 1]) << 8) |
                           static_cast<uint32_t>(data[index + 2]);
    encoded.push_back(kAlphabet[(value >> 18) & 0x3F]);
    encoded.push_back(kAlphabet[(value >> 12) & 0x3F]);
    encoded.push_back(kAlphabet[(value >> 6) & 0x3F]);
    encoded.push_back(kAlphabet[value & 0x3F]);
    index += 3;
  }

  if (index < data.size())
  {
    uint32_t value = static_cast<uint32_t>(data[index]) << 16;
    encoded.push_back(kAlphabet[(value >> 18) & 0x3F]);
    if (index + 1 < data.size())
    {
      value |= static_cast<uint32_t>(data[index + 1]) << 8;
      encoded.push_back(kAlphabet[(value >> 12) & 0x3F]);
      encoded.push_back(kAlphabet[(value >> 6) & 0x3F]);
      encoded.push_back('=');
    }
    else
    {
      encoded.push_back(kAlphabet[(value >> 12) & 0x3F]);
      encoded.push_back('=');
      encoded.push_back('=');
    }
  }

  return encoded;
}

static bool GetPngEncoderClsid(CLSID *out_clsid)
{
  EnsureGdiplusInitialized();

  UINT encoder_count = 0;
  UINT encoder_size = 0;
  if (Gdiplus::GetImageEncodersSize(&encoder_count, &encoder_size) != Gdiplus::Ok || encoder_size == 0)
  {
    return false;
  }

  std::vector<uint8_t> buffer(encoder_size);
  auto *encoders = reinterpret_cast<Gdiplus::ImageCodecInfo *>(buffer.data());
  if (Gdiplus::GetImageEncoders(encoder_count, encoder_size, encoders) != Gdiplus::Ok)
  {
    return false;
  }

  for (UINT i = 0; i < encoder_count; ++i)
  {
    if (wcscmp(encoders[i].MimeType, L"image/png") == 0)
    {
      *out_clsid = encoders[i].Clsid;
      return true;
    }
  }

  return false;
}

static bool EncodeBitmapToPngBase64(HBITMAP bitmap, std::string &png_base64, std::string &error)
{
  CLSID png_clsid{};
  if (!GetPngEncoderClsid(&png_clsid))
  {
    error = "Failed to find PNG encoder";
    return false;
  }

  Gdiplus::Bitmap image(bitmap, nullptr);
  IStream *stream = nullptr;
  if (CreateStreamOnHGlobal(nullptr, TRUE, &stream) != S_OK)
  {
    error = "Failed to create memory stream";
    return false;
  }

  const auto status = image.Save(stream, &png_clsid, nullptr);
  if (status != Gdiplus::Ok)
  {
    error = "Failed to encode monitor image as PNG";
    stream->Release();
    return false;
  }

  HGLOBAL global = nullptr;
  if (GetHGlobalFromStream(stream, &global) != S_OK || global == nullptr)
  {
    error = "Failed to access encoded PNG stream";
    stream->Release();
    return false;
  }

  const SIZE_T size = GlobalSize(global);
  auto *bytes = static_cast<uint8_t *>(GlobalLock(global));
  if (bytes == nullptr || size == 0)
  {
    error = "Failed to lock encoded PNG bytes";
    if (bytes != nullptr)
    {
      GlobalUnlock(global);
    }
    stream->Release();
    return false;
  }

  std::vector<uint8_t> copy(bytes, bytes + size);
  GlobalUnlock(global);
  stream->Release();
  png_base64 = Base64Encode(copy);
  return true;
}

static flutter::EncodableMap BuildRectValue(double x, double y, double width, double height)
{
  flutter::EncodableMap rect;
  rect[flutter::EncodableValue("x")] = flutter::EncodableValue(x);
  rect[flutter::EncodableValue("y")] = flutter::EncodableValue(y);
  rect[flutter::EncodableValue("width")] = flutter::EncodableValue(width);
  rect[flutter::EncodableValue("height")] = flutter::EncodableValue(height);
  return rect;
}

static flutter::EncodableMap BuildRectValue(const RECT &rect)
{
  return BuildRectValue(
      static_cast<double>(rect.left),
      static_cast<double>(rect.top),
      static_cast<double>(rect.right - rect.left),
      static_cast<double>(rect.bottom - rect.top));
}

static flutter::EncodableMap BuildScaledRectValue(const RECT &rect, double scale)
{
  const double safe_scale = scale <= 0 ? 1.0 : scale;
  return BuildRectValue(
      static_cast<double>(rect.left) / safe_scale,
      static_cast<double>(rect.top) / safe_scale,
      static_cast<double>(rect.right - rect.left) / safe_scale,
      static_cast<double>(rect.bottom - rect.top) / safe_scale);
}

struct MonitorSnapshotCapture
{
  std::vector<flutter::EncodableValue> snapshots;
  std::string error;
};

static bool IsKeyDownMessage(UINT message)
{
  return message == WM_KEYDOWN || message == WM_SYSKEYDOWN;
}

static bool IsKeyUpMessage(UINT message)
{
  return message == WM_KEYUP || message == WM_SYSKEYUP;
}

uint64_t FlutterWindow::MakeKeyboardMessageSignature(UINT message, WPARAM wparam, LPARAM lparam)
{
  const uint64_t virtual_key = static_cast<uint64_t>(wparam & 0xFFFF);
  const uint64_t scancode = static_cast<uint64_t>((lparam >> 16) & 0xFF);
  const uint64_t is_extended = static_cast<uint64_t>((lparam >> 24) & 0x1);
  const uint64_t is_system_key = static_cast<uint64_t>(message == WM_SYSKEYDOWN || message == WM_SYSKEYUP);

  return virtual_key | (scancode << 16) | (is_extended << 24) | (is_system_key << 25);
}

static UINT KeyboardKeyUpMessageFromSignature(uint64_t signature)
{
  const bool is_system_key = ((signature >> 25) & 0x1) != 0;
  return is_system_key ? WM_SYSKEYUP : WM_KEYUP;
}

static WPARAM KeyboardVirtualKeyFromSignature(uint64_t signature)
{
  return static_cast<WPARAM>(signature & 0xFFFF);
}

static LPARAM MakeKeyboardKeyUpLParamFromSignature(uint64_t signature)
{
  // Rebuild the keyup LPARAM from the tracked child keydown signature so the
  // synthetic release matches the original key as closely as possible.
  // Flutter's Windows keyboard path uses both WPARAM and LPARAM fields
  // (scancode / extended bit / system-key bit / transition bits) when mapping
  // the event, so sending only VK_ESCAPE/VK_RETURN without the original shape
  // risks clearing the wrong key or being ignored by the engine.
  LPARAM lparam = 1;
  lparam |= static_cast<LPARAM>((signature >> 16) & 0xFF) << 16;
  lparam |= static_cast<LPARAM>((signature >> 24) & 0x1) << 24;

  if (((signature >> 25) & 0x1) != 0)
  {
    lparam |= static_cast<LPARAM>(1) << 29;
  }

  lparam |= static_cast<LPARAM>(1) << 30;
  lparam |= static_cast<LPARAM>(1) << 31;
  return lparam;
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
      original_child_window_proc_(nullptr),
      child_window_(nullptr),
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

std::string FlutterWindow::RectToString(const RECT &rect) const
{
  std::ostringstream oss;
  oss << "(" << rect.left << "," << rect.top << ")-(" << rect.right << "," << rect.bottom << ")";
  return oss.str();
}

RECT FlutterWindow::GetWindowRectSafe(HWND hwnd) const
{
  RECT rect{};
  if (hwnd != nullptr && IsWindow(hwnd))
  {
    GetWindowRect(hwnd, &rect);
  }
  return rect;
}

void FlutterWindow::SyncFlutterChildWindowToClientArea(HWND hwnd, const char *source, bool engine_handled)
{
  if (child_window_ == nullptr || !IsWindow(child_window_))
  {
    return;
  }

  RECT client_rect{};
  GetClientRect(hwnd, &client_rect);
  const int width = client_rect.right - client_rect.left;
  const int height = client_rect.bottom - client_rect.top;

  MoveWindow(child_window_, client_rect.left, client_rect.top, width, height, TRUE);

  const RECT child_rect = GetWindowRectSafe(child_window_);
  std::ostringstream oss;
  oss << source << ": engineHandled=" << (engine_handled ? "true" : "false")
      << ", client=" << RectToString(client_rect)
      << ", child=" << RectToString(child_rect);
  Log(oss.str());
}

void FlutterWindow::TrackChildKeyDown(UINT message, WPARAM wparam, LPARAM lparam)
{
  if (!IsKeyDownMessage(message))
  {
    return;
  }

  // Repeat keydown messages should not create extra pending releases.
  if ((lparam & (static_cast<LPARAM>(1) << 30)) != 0)
  {
    return;
  }

  const uint64_t signature = MakeKeyboardMessageSignature(message, wparam, lparam);
  pending_child_keydowns_.insert(signature);
}

void FlutterWindow::ClearTrackedChildKeyDown(UINT message, WPARAM wparam, LPARAM lparam)
{
  if (!IsKeyUpMessage(message))
  {
    return;
  }

  const uint64_t signature = MakeKeyboardMessageSignature(message, wparam, lparam);
  pending_child_keydowns_.erase(signature);
}

bool FlutterWindow::HasTrackedChildKeyDown(UINT message, WPARAM wparam, LPARAM lparam) const
{
  if (!IsKeyUpMessage(message))
  {
    return false;
  }

  const uint64_t signature = MakeKeyboardMessageSignature(message, wparam, lparam);
  return pending_child_keydowns_.find(signature) != pending_child_keydowns_.end();
}

// Windows occasionally delivers the release for Enter/Escape-style actions
// to the top-level runner window after the keydown has already triggered a
// focus/view transition inside Flutter. In that case the engine sees the
// keydown on the child hwnd but ignores the matching keyup on the root hwnd,
// leaving HardwareKeyboard in a stale "pressed" state. The visible symptom is
// an every-other-press failure: one Enter works, the next one is ignored,
// then the following release clears the stale state again.

// Alternatives considered:
// 1. Reintroduce the old message-loop-to-Dart keyboard bridge.
//    Rejected because it duplicates the engine's keyboard pipeline and turns
//    a root/child routing bug into a broad Windows-only input hack.
// 2. Move all action execution to keyup in higher layers.
//    Rejected because it changes behavior outside Windows and spreads this
//    engine-specific issue into Dart UI code.
// 3. Fix the Flutter engine.
//    This is the ideal long-term solution, but it is outside the Wox runner.

// The chosen compromise is narrow and native: only when a non-repeat keydown
// definitely reached the child hwnd, and the matching keyup later lands on
// the root hwnd, send that release back to the child synchronously so the
// engine can clear its pressed state.
bool FlutterWindow::RerouteIgnoredRootKeyUp(HWND hwnd, UINT message, WPARAM wparam, LPARAM lparam)
{
  if (!IsKeyUpMessage(message) || child_window_ == nullptr || hwnd == nullptr)
  {
    return false;
  }

  if (!IsWindow(child_window_) || GetAncestor(child_window_, GA_ROOT) != hwnd)
  {
    return false;
  }

  if (!HasTrackedChildKeyDown(message, wparam, lparam))
  {
    return false;
  }

  SendMessage(child_window_, message, wparam, lparam);
  return true;
}

void FlutterWindow::FlushPendingChildKeyUps()
{
  if (pending_child_keydowns_.empty())
  {
    return;
  }

  if (child_window_ == nullptr || !IsWindow(child_window_))
  {
    return;
  }

  // Flush every still-pending child keydown as a synthetic keyup.
  //
  // This handles two scenarios:
  //   1. Hide-on-keydown (e.g. Escape): child receives keydown -> Dart hides
  //      the window immediately -> the real keyup is never delivered through
  //      Flutter -> HardwareKeyboard keeps the key marked as pressed.
  //   2. Defense-in-depth on show: if a previous hide-flush was ineffective
  //      (engine dropped the synthetic keyup), the show-flush retries.
  //
  // Take a snapshot for safe iteration: SendMessage below re-enters
  // ChildWindowProc which calls ClearTrackedChildKeyDown, removing
  // entries from pending_child_keydowns_ during iteration.
  const std::unordered_set<uint64_t> snapshot = pending_child_keydowns_;

  for (const uint64_t signature : snapshot)
  {
    const WPARAM vk = KeyboardVirtualKeyFromSignature(signature);

    // Only send a synthetic keyup if the OS says the key is NOT currently
    // pressed. This avoids incorrectly releasing a key the user is still
    // holding (e.g. while the window shows via a keyboard shortcut).
    if ((GetAsyncKeyState(static_cast<int>(vk)) & 0x8000) != 0)
    {
      continue;
    }

    SendMessage(
        child_window_,
        KeyboardKeyUpMessageFromSignature(signature),
        vk,
        MakeKeyboardKeyUpLParamFromSignature(signature));
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

  child_window_ = flutter_controller_->view()->GetNativeWindow();
  SetChildContent(child_window_);

  if (child_window_ != nullptr)
  {
    original_child_window_proc_ = reinterpret_cast<WNDPROC>(GetWindowLongPtr(child_window_, GWLP_WNDPROC));
    SetWindowLongPtr(child_window_, GWLP_WNDPROC, reinterpret_cast<LONG_PTR>(ChildWindowProc));
  }

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
  pending_child_keydowns_.clear();

  if (child_window_ != nullptr && original_child_window_proc_ != nullptr)
  {
    SetWindowLongPtr(child_window_, GWLP_WNDPROC, reinterpret_cast<LONG_PTR>(original_child_window_proc_));
    original_child_window_proc_ = nullptr;
    child_window_ = nullptr;
  }

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
  if (message == WM_SIZE)
  {
    std::optional<LRESULT> top_level_result;

    if (flutter_controller_)
    {
      // Keep the existing dispatch order for instrumentation so we can verify
      // whether the engine handles WM_SIZE before the base runner resizes the
      // hosted child window.
      top_level_result = flutter_controller_->HandleTopLevelWindowProc(hwnd, message, wparam, lparam);
    }

    std::ostringstream oss;
    oss << "WM_SIZE: engineHandled=" << (top_level_result.has_value() ? "true" : "false");
    Log(oss.str());

    // Keep the hosted Flutter child window in sync even when the engine
    // handles WM_SIZE before the base runner processes it.
    SyncFlutterChildWindowToClientArea(hwnd, "WM_SIZE", top_level_result.has_value());

    if (top_level_result)
    {
      return *top_level_result;
    }

    return Win32Window::MessageHandler(hwnd, message, wparam, lparam);
  }

  // Give Flutter, including plugins, an opportunity to handle window messages.
  if (flutter_controller_)
  {
    // Reroute BEFORE HandleTopLevelWindowProc. Otherwise the engine's
    // top-level handler consumes the WM_KEYUP at the root window without
    // generating a Dart KeyUpEvent, leaving HardwareKeyboard._pressedKeys
    // with a stale entry. The visible symptom is that the affected key
    // stops working until the next app restart.
    if (RerouteIgnoredRootKeyUp(hwnd, message, wparam, lparam))
    {
      return 0;
    }

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

LRESULT CALLBACK FlutterWindow::ChildWindowProc(HWND hwnd, UINT message, WPARAM wparam, LPARAM lparam)
{
  if (g_window_instance == nullptr || g_window_instance->original_child_window_proc_ == nullptr)
  {
    return DefWindowProc(hwnd, message, wparam, lparam);
  }

  if (IsKeyDownMessage(message))
  {
    g_window_instance->TrackChildKeyDown(message, wparam, lparam);
  }
  else if (IsKeyUpMessage(message))
  {
    g_window_instance->ClearTrackedChildKeyDown(message, wparam, lparam);
  }

  const LRESULT result = CallWindowProc(g_window_instance->original_child_window_proc_, hwnd, message, wparam, lparam);

  return result;
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
    else if (method_name == "captureAllDisplays")
    {
      MonitorSnapshotCapture monitor_capture;

      EnumDisplayMonitors(
          nullptr,
          nullptr,
          [](HMONITOR monitor, HDC, LPRECT, LPARAM data) -> BOOL
          {
            auto *capture = reinterpret_cast<MonitorSnapshotCapture *>(data);
            MONITORINFOEXW monitor_info{};
            monitor_info.cbSize = sizeof(MONITORINFOEXW);
            if (!GetMonitorInfoW(monitor, reinterpret_cast<MONITORINFO *>(&monitor_info)))
            {
              capture->error = "Failed to query monitor info";
              return FALSE;
            }

            const int width = monitor_info.rcMonitor.right - monitor_info.rcMonitor.left;
            const int height = monitor_info.rcMonitor.bottom - monitor_info.rcMonitor.top;
            if (width <= 0 || height <= 0)
            {
              capture->error = "Monitor has invalid bounds";
              return FALSE;
            }

            HDC screen_dc = GetDC(nullptr);
            if (screen_dc == nullptr)
            {
              capture->error = "Failed to access desktop device context";
              return FALSE;
            }

            HDC memory_dc = CreateCompatibleDC(screen_dc);
            HBITMAP bitmap = CreateCompatibleBitmap(screen_dc, width, height);
            if (memory_dc == nullptr || bitmap == nullptr)
            {
              if (bitmap != nullptr)
              {
                DeleteObject(bitmap);
              }
              if (memory_dc != nullptr)
              {
                DeleteDC(memory_dc);
              }
              ReleaseDC(nullptr, screen_dc);
              capture->error = "Failed to allocate monitor bitmap";
              return FALSE;
            }

            HGDIOBJ old_bitmap = SelectObject(memory_dc, bitmap);
            const BOOL copied = BitBlt(
                memory_dc,
                0,
                0,
                width,
                height,
                screen_dc,
                monitor_info.rcMonitor.left,
                monitor_info.rcMonitor.top,
                SRCCOPY | CAPTUREBLT);

            SelectObject(memory_dc, old_bitmap);
            DeleteDC(memory_dc);
            ReleaseDC(nullptr, screen_dc);

            if (!copied)
            {
              DeleteObject(bitmap);
              capture->error = "Failed to capture monitor bitmap";
              return FALSE;
            }

            std::string png_base64;
            std::string encode_error;
            const bool encoded = EncodeBitmapToPngBase64(bitmap, png_base64, encode_error);
            DeleteObject(bitmap);
            if (!encoded)
            {
              capture->error = encode_error;
              return FALSE;
            }

            DEVMODEW dev_mode{};
            dev_mode.dmSize = sizeof(DEVMODEW);
            int rotation = 0;
            if (EnumDisplaySettingsExW(monitor_info.szDevice, ENUM_CURRENT_SETTINGS, &dev_mode, 0))
            {
              switch (dev_mode.dmDisplayOrientation)
              {
              case DMDO_90:
                rotation = 90;
                break;
              case DMDO_180:
                rotation = 180;
                break;
              case DMDO_270:
                rotation = 270;
                break;
              default:
                rotation = 0;
                break;
              }
            }

            const UINT dpi = FlutterDesktopGetDpiForMonitor(monitor);
            const double scale = static_cast<double>(dpi) / 96.0;
            flutter::EncodableMap snapshot;
            snapshot[flutter::EncodableValue("displayId")] = flutter::EncodableValue(Utf8FromUtf16(monitor_info.szDevice));
            // Windows screenshot presentation now sizes one overlay window in native virtual-
            // desktop pixels. Mixed-DPI monitor layouts cannot be represented by dividing each
            // monitor rect by its own scale because the combined workspace would no longer share
            // one coordinate system. Flutter normalizes these native bounds after the platform
            // reports the final overlay window scale.
            snapshot[flutter::EncodableValue("logicalBounds")] = flutter::EncodableValue(
                BuildRectValue(
                    static_cast<double>(monitor_info.rcMonitor.left),
                    static_cast<double>(monitor_info.rcMonitor.top),
                    static_cast<double>(width),
                    static_cast<double>(height)));
            snapshot[flutter::EncodableValue("pixelBounds")] = flutter::EncodableValue(
                BuildRectValue(
                    static_cast<double>(monitor_info.rcMonitor.left),
                    static_cast<double>(monitor_info.rcMonitor.top),
                    static_cast<double>(width),
                    static_cast<double>(height)));
            snapshot[flutter::EncodableValue("scale")] = flutter::EncodableValue(scale);
            snapshot[flutter::EncodableValue("rotation")] = flutter::EncodableValue(rotation);
            snapshot[flutter::EncodableValue("imageBytesBase64")] = flutter::EncodableValue(png_base64);
            capture->snapshots.emplace_back(snapshot);
            return TRUE;
          },
          reinterpret_cast<LPARAM>(&monitor_capture));

      if (!monitor_capture.error.empty())
      {
        result->Error("CAPTURE_ERROR", monitor_capture.error);
        return;
      }

      flutter::EncodableList snapshots;
      for (const auto &snapshot : monitor_capture.snapshots)
      {
        snapshots.push_back(snapshot);
      }
      result->Success(flutter::EncodableValue(snapshots));
    }
    else if (method_name == "presentCaptureWorkspace")
    {
      const auto *arguments = std::get_if<flutter::EncodableMap>(method_call.arguments());
      if (arguments == nullptr)
      {
        result->Error("INVALID_ARGUMENTS", "Invalid arguments for presentCaptureWorkspace");
        return;
      }

      auto x_it = arguments->find(flutter::EncodableValue("x"));
      auto y_it = arguments->find(flutter::EncodableValue("y"));
      auto width_it = arguments->find(flutter::EncodableValue("width"));
      auto height_it = arguments->find(flutter::EncodableValue("height"));
      if (x_it == arguments->end() || y_it == arguments->end() || width_it == arguments->end() || height_it == arguments->end())
      {
        result->Error("INVALID_ARGUMENTS", "Invalid arguments for presentCaptureWorkspace");
        return;
      }

      const double x = std::get<double>(x_it->second);
      const double y = std::get<double>(y_it->second);
      const double width = std::get<double>(width_it->second);
      const double height = std::get<double>(height_it->second);
      const RECT native_workspace_bounds{
          static_cast<LONG>(std::lround(x)),
          static_cast<LONG>(std::lround(y)),
          static_cast<LONG>(std::lround(x + width)),
          static_cast<LONG>(std::lround(y + height))};

      SavePreviousActiveWindow(hwnd);
      FlushPendingChildKeyUps();
      blur_guard_active_ = true;
      blur_guard_until_tick_ = GetTickCount64() + kPostShowBlurGraceMs;

      SetWindowPos(
          hwnd,
          HWND_TOPMOST,
          native_workspace_bounds.left,
          native_workspace_bounds.top,
          native_workspace_bounds.right - native_workspace_bounds.left,
          native_workspace_bounds.bottom - native_workspace_bounds.top,
          SWP_FRAMECHANGED | SWP_SHOWWINDOW);

      if (flutter_controller_)
      {
        flutter_controller_->ForceRedraw();
      }
      SyncFlutterChildWindowToClientArea(hwnd, "presentCaptureWorkspace", false);

      // Screenshot capture replaces the standard show() -> focus() sequence on Windows because the
      // generic window-manager path assumes one monitor/DPI. Reapplying the focus restore steps
      // here keeps the capture overlay interactive without reusing the single-monitor geometry path.
      DismissStartMenuIfOpen();
      SavePreviousActiveWindow(hwnd);
      if (!SetForegroundWindow(hwnd))
      {
        AllowSetForegroundWindow(ASFW_ANY);
        SetForegroundWindow(hwnd);
      }
      SetFocus(hwnd);
      BringWindowToTop(hwnd);
      blur_guard_active_ = false;

      const double workspace_scale = static_cast<double>(GetDpiScale(hwnd));
      screenshot_presentation_state_.active = true;
      screenshot_presentation_state_.workspace_scale = workspace_scale;
      screenshot_presentation_state_.native_workspace_bounds = native_workspace_bounds;

      flutter::EncodableMap response;
      response[flutter::EncodableValue("workspaceBounds")] = flutter::EncodableValue(BuildScaledRectValue(native_workspace_bounds, workspace_scale));
      response[flutter::EncodableValue("workspaceScale")] = flutter::EncodableValue(workspace_scale);
      response[flutter::EncodableValue("presentedByPlatform")] = flutter::EncodableValue(true);
      result->Success(flutter::EncodableValue(response));
    }
    else if (method_name == "dismissCaptureWorkspacePresentation")
    {
      screenshot_presentation_state_.active = false;
      screenshot_presentation_state_.workspace_scale = 1.0;
      screenshot_presentation_state_.native_workspace_bounds = {0, 0, 0, 0};
      SyncFlutterChildWindowToClientArea(hwnd, "dismissCaptureWorkspacePresentation", false);
      result->Success();
    }
    else if (method_name == "debugCaptureWorkspaceState")
    {
      RECT root_rect{};
      GetWindowRect(hwnd, &root_rect);
      const double current_scale = static_cast<double>(GetDpiScale(hwnd));

      flutter::EncodableMap response;
      response[flutter::EncodableValue("isCapturePresentationActive")] = flutter::EncodableValue(screenshot_presentation_state_.active);
      response[flutter::EncodableValue("workspaceScale")] = flutter::EncodableValue(screenshot_presentation_state_.workspace_scale);
      response[flutter::EncodableValue("workspaceBounds")] = flutter::EncodableValue(
          BuildScaledRectValue(screenshot_presentation_state_.native_workspace_bounds, screenshot_presentation_state_.workspace_scale));
      response[flutter::EncodableValue("windowBounds")] = flutter::EncodableValue(BuildScaledRectValue(root_rect, current_scale));
      response[flutter::EncodableValue("nativeWorkspaceBounds")] = flutter::EncodableValue(BuildRectValue(screenshot_presentation_state_.native_workspace_bounds));
      result->Success(flutter::EncodableValue(response));
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

          RECT root_rect{};
          RECT client_rect{};
          GetWindowRect(hwnd, &root_rect);
          GetClientRect(hwnd, &client_rect);
          const RECT child_rect = GetWindowRectSafe(child_window_);
          std::ostringstream oss;
          oss << "setSize: logical=" << width << "x" << height
              << ", physical=" << scaledWidth << "x" << scaledHeight
              << ", root=" << RectToString(root_rect)
              << ", client=" << RectToString(client_rect)
              << ", child=" << RectToString(child_rect);
          Log(oss.str());

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

          RECT root_rect{};
          RECT client_rect{};
          GetWindowRect(hwnd, &root_rect);
          GetClientRect(hwnd, &client_rect);
          const RECT child_rect = GetWindowRectSafe(child_window_);
          std::ostringstream oss;
          oss << "setBounds: logicalPos=" << x << "," << y
              << ", logicalSize=" << width << "x" << height
              << ", physicalPos=" << scaledX << "," << scaledY
              << ", physicalSize=" << scaledWidth << "x" << scaledHeight
              << ", root=" << RectToString(root_rect)
              << ", client=" << RectToString(client_rect)
              << ", child=" << RectToString(child_rect);
          Log(oss.str());

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

      // Flush stale keyboard state before showing the window.
      // If the previous hide-flush was ineffective (e.g. the engine dropped
      // the synthetic keyup), retrying here clears any remaining entries so
      // the user doesn't encounter stuck keys after Wox reappears.
      FlushPendingChildKeyUps();

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
      blur_guard_active_ = false;
      blur_guard_until_tick_ = 0;

      // Flush before SW_HIDE. After the window is hidden, Windows may deliver
      // the physical keyup somewhere else (or not through Flutter at all),
      // which is exactly how Escape ended up stuck in HardwareKeyboard.
      FlushPendingChildKeyUps();

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

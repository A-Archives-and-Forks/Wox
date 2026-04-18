import ApplicationServices
import Cocoa
import FlutterMacOS
import ScreenCaptureKit

// The screenshot workspace and saved window positions both use a top-left logical desktop space
// that spans every monitor. The previous macOS conversion mixed that global contract with
// per-screen top edges, which left fullscreen screenshot overlays misaligned as soon as displays
// had different vertical offsets. Keeping every top-left/AppKit conversion anchored to the virtual
// desktop top fixes the multi-display capture bug without introducing a second coordinate system.
private func virtualDesktopTopInAppKit() -> CGFloat {
  return NSScreen.screens.map { $0.frame.maxY }.max() ?? 0
}

private func appKitY(fromTopLeftY y: CGFloat, height: CGFloat = 0) -> CGFloat {
  return virtualDesktopTopInAppKit() - y - height
}

private func topLeftY(fromAppKitY y: CGFloat, height: CGFloat) -> CGFloat {
  return virtualDesktopTopInAppKit() - y - height
}

@main
class AppDelegate: FlutterAppDelegate {
  // The screenshot capture helpers run inside Swift `throws` / `async throws` contexts. The
  // previous implementation threw `FlutterError` directly, but the macOS Flutter SDK exposes it
  // as an Objective-C channel payload rather than a Swift `Error`, which now fails compilation.
  // Keep a Swift-native error for internal control flow and convert it back at the channel
  // boundary so Dart still receives the same error codes and messages as before.
  private struct DisplayCaptureError: LocalizedError {
    let code: String
    let message: String
    let details: Any?

    var errorDescription: String? {
      return message
    }

    func asFlutterError() -> FlutterError {
      return FlutterError(code: code, message: message, details: details)
    }
  }

  private struct ScreenshotPresentationState {
    let collectionBehavior: NSWindow.CollectionBehavior
    let level: NSWindow.Level
    let hasShadow: Bool
  }

  // Store the previous active application
  private var previousActiveApp: NSRunningApplication?
  // Only restore the previous app when Wox has stayed focused since the last show/focus.
  private var shouldRestorePreviousAppOnHide = false
  // Flutter method channel for window events
  private var windowEventChannel: FlutterMethodChannel?
  // Current appearance (light/dark)
  private var currentAppearance: String = "light"
  private var screenshotPresentationState: ScreenshotPresentationState?
  private var isCapturePresentationActive = false
  private var captureWorkspaceBounds = NSRect.zero
  private var captureWorkspaceScale = 1.0

  private func savePreviousActiveAppIfNeeded() {
    if let frontApp = NSWorkspace.shared.frontmostApplication,
      frontApp != NSRunningApplication.current,
      !frontApp.isTerminated
    {
      log(
        "Saving previous active app: \(frontApp.localizedName ?? "Unknown") (bundleID: \(frontApp.bundleIdentifier ?? "Unknown"))"
      )
      previousActiveApp = frontApp
      shouldRestorePreviousAppOnHide = true
    } else {
      log("No new previous app to save, keeping existing restore state")
    }
  }

  private func log(_ message: String) {
    DispatchQueue.main.async { [weak self] in
      self?.windowEventChannel?.invokeMethod("log", arguments: message)
    }
  }

  private func keyCode(for key: String) -> CGKeyCode? {
    switch key.lowercased() {
    case "a": return 0
    case "s": return 1
    case "d": return 2
    case "f": return 3
    case "h": return 4
    case "g": return 5
    case "z": return 6
    case "x": return 7
    case "c": return 8
    case "v": return 9
    case "b": return 11
    case "q": return 12
    case "w": return 13
    case "e": return 14
    case "r": return 15
    case "y": return 16
    case "t": return 17
    case "1": return 18
    case "2": return 19
    case "3": return 20
    case "4": return 21
    case "6": return 22
    case "5": return 23
    case "9": return 25
    case "7": return 26
    case "8": return 28
    case "0": return 29
    case "o": return 31
    case "u": return 32
    case "i": return 34
    case "p": return 35
    case "l": return 37
    case "j": return 38
    case "k": return 40
    case "n": return 45
    case "m": return 46
    case "enter": return 36
    case "tab": return 48
    case "space": return 49
    case "escape": return 53
    case "meta": return 55
    case "shift": return 56
    case "alt": return 58
    case "control": return 59
    case "arrowleft": return 123
    case "arrowright": return 124
    case "arrowdown": return 125
    case "arrowup": return 126
    default: return nil
    }
  }

  private func mouseButton(for button: String) -> CGMouseButton? {
    switch button.lowercased() {
    case "left": return .left
    case "right": return .right
    case "middle": return .center
    default: return nil
    }
  }

  private func mouseEventTypes(for button: CGMouseButton) -> (down: CGEventType, up: CGEventType) {
    switch button {
    case .left:
      return (.leftMouseDown, .leftMouseUp)
    case .right:
      return (.rightMouseDown, .rightMouseUp)
    default:
      return (.otherMouseDown, .otherMouseUp)
    }
  }

  private func screenPoint(fromTopLeft point: CGPoint) -> CGPoint {
    return CGPoint(x: point.x, y: appKitY(fromTopLeftY: point.y))
  }

  private func parseRect(arguments: [String: Any]) -> NSRect? {
    guard
      let x = arguments["x"] as? Double,
      let y = arguments["y"] as? Double,
      let width = arguments["width"] as? Double,
      let height = arguments["height"] as? Double
    else {
      return nil
    }

    return NSRect(x: x, y: y, width: width, height: height)
  }

  private func buildRectPayload(_ rect: NSRect) -> [String: Any] {
    return [
      "x": rect.origin.x,
      "y": rect.origin.y,
      "width": rect.size.width,
      "height": rect.size.height,
    ]
  }

  private func presentCaptureWorkspace(on window: NSWindow, bounds: NSRect) -> [String: Any] {
    if screenshotPresentationState == nil {
      screenshotPresentationState = ScreenshotPresentationState(
        collectionBehavior: window.collectionBehavior,
        level: window.level,
        hasShadow: window.hasShadow
      )
    }

    let frameRect = window.frameRect(forContentRect: NSRect(x: 0, y: 0, width: bounds.width, height: bounds.height))
    let flippedY = appKitY(fromTopLeftY: bounds.origin.y, height: frameRect.height)

    // The launcher window's default collection behavior is tuned for a compact app panel. Screenshot
    // capture needs a system-overlay style contract instead so one window can stay pinned across the
    // full virtual desktop without inheriting the main launcher panel's single-display assumptions.
    var collectionBehavior: NSWindow.CollectionBehavior = [
      .canJoinAllSpaces,
      .stationary,
      .ignoresCycle,
      .fullScreenNone,
    ]
    if #available(macOS 13.0, *) {
      collectionBehavior.insert(.canJoinAllApplications)
    }
    window.collectionBehavior = collectionBehavior
    window.level = .popUpMenu
    window.hasShadow = false
    window.setFrame(NSRect(x: bounds.origin.x, y: flippedY, width: frameRect.width, height: frameRect.height), display: true)
    window.contentView?.needsLayout = true
    window.contentView?.layoutSubtreeIfNeeded()
    window.displayIfNeeded()
    savePreviousActiveAppIfNeeded()
    window.makeKeyAndOrderFront(nil)
    NSApp.activate(ignoringOtherApps: true)

    isCapturePresentationActive = true
    captureWorkspaceBounds = bounds
    captureWorkspaceScale = 1
    return [
      "workspaceBounds": buildRectPayload(bounds),
      "workspaceScale": 1,
      "presentedByPlatform": true,
    ]
  }

  private func dismissCaptureWorkspacePresentation(on window: NSWindow) {
    guard let savedState = screenshotPresentationState else {
      isCapturePresentationActive = false
      captureWorkspaceBounds = .zero
      captureWorkspaceScale = 1
      return
    }

    window.collectionBehavior = savedState.collectionBehavior
    window.level = savedState.level
    window.hasShadow = savedState.hasShadow
    screenshotPresentationState = nil
    isCapturePresentationActive = false
    captureWorkspaceBounds = .zero
    captureWorkspaceScale = 1
  }

  private func debugCaptureWorkspaceState(for window: NSWindow) -> [String: Any] {
    let contentRect = window.contentRect(forFrameRect: window.frame)
    let windowTopLeftRect = NSRect(
      x: window.frame.origin.x,
      y: topLeftY(fromAppKitY: window.frame.origin.y, height: contentRect.height),
      width: contentRect.width,
      height: contentRect.height
    )

    return [
      "isCapturePresentationActive": isCapturePresentationActive,
      "workspaceScale": captureWorkspaceScale,
      "workspaceBounds": buildRectPayload(captureWorkspaceBounds),
      "windowBounds": buildRectPayload(windowTopLeftRect),
      "collectionBehavior": window.collectionBehavior.rawValue,
    ]
  }

  private func currentMouseLocation() -> CGPoint {
    return CGEvent(source: nil)?.location ?? NSEvent.mouseLocation
  }

  private func captureAllDisplaysLegacy() throws -> [[String: Any]] {
    if !CGPreflightScreenCaptureAccess() {
      throw DisplayCaptureError(code: "permission_denied", message: "Screen recording permission is required", details: nil)
    }

    let screens = NSScreen.screens
    if screens.isEmpty {
      throw DisplayCaptureError(code: "capture_failed", message: "No screens are available for capture", details: nil)
    }

    let globalTop = screens.map { $0.frame.origin.y + $0.frame.height }.max() ?? 0
    var snapshots: [[String: Any]] = []

    for screen in screens {
      guard let screenNumber = screen.deviceDescription[NSDeviceDescriptionKey("NSScreenNumber")] as? NSNumber else {
        throw DisplayCaptureError(code: "capture_failed", message: "Failed to resolve macOS display id", details: nil)
      }

      let displayId = CGDirectDisplayID(screenNumber.uint32Value)
      guard let cgImage = CGDisplayCreateImage(displayId) else {
        throw DisplayCaptureError(code: "capture_failed", message: "Failed to capture macOS display image", details: nil)
      }

      let bitmap = NSBitmapImageRep(cgImage: cgImage)
      guard let pngData = bitmap.representation(using: .png, properties: [:]) else {
        throw DisplayCaptureError(code: "capture_failed", message: "Failed to encode macOS display image", details: nil)
      }

      let frame = screen.frame
      let scale = screen.backingScaleFactor
      let logicalX = frame.origin.x
      let logicalY = globalTop - frame.origin.y - frame.height
      let rotation = Int(CGDisplayRotation(displayId).rounded())

      snapshots.append(
        [
          "displayId": String(displayId),
          "logicalBounds": [
            "x": logicalX,
            "y": logicalY,
            "width": frame.width,
            "height": frame.height,
          ],
          "pixelBounds": [
            "x": logicalX * scale,
            "y": logicalY * scale,
            "width": CGFloat(cgImage.width),
            "height": CGFloat(cgImage.height),
          ],
          "scale": scale,
          "rotation": rotation,
          "imageBytesBase64": pngData.base64EncodedString(),
        ])
    }

    return snapshots
  }

  @available(macOS 14.0, *)
  private func captureAllDisplaysWithScreenCaptureKit() async throws -> [[String: Any]] {
    if !CGPreflightScreenCaptureAccess() {
      throw DisplayCaptureError(code: "permission_denied", message: "Screen recording permission is required", details: nil)
    }

    let shareableContent = try await SCShareableContent.excludingDesktopWindows(
      false,
      onScreenWindowsOnly: true
    )
    if shareableContent.displays.isEmpty {
      throw DisplayCaptureError(
        code: "capture_failed",
        message: "No displays are available for ScreenCaptureKit capture",
        details: nil
      )
    }

    let globalTop = shareableContent.displays.map { $0.frame.maxY }.max() ?? 0
    let excludedApplications = shareableContent.applications.filter {
      $0.bundleIdentifier == Bundle.main.bundleIdentifier
    }
    var snapshots: [[String: Any]] = []

    for display in shareableContent.displays {
      // ScreenCaptureKit replaces CGDisplayCreateImage on modern macOS. We exclude the current
      // Wox process here so the screenshot workspace does not appear in the captured background
      // after the launcher window is hidden and resized across the virtual desktop.
      let contentFilter = SCContentFilter(
        display: display,
        excludingApplications: excludedApplications,
        exceptingWindows: []
      )
      let scale = CGFloat(contentFilter.pointPixelScale)
      let streamConfiguration = SCStreamConfiguration()
      streamConfiguration.width = Int((display.frame.width * scale).rounded())
      streamConfiguration.height = Int((display.frame.height * scale).rounded())

      let cgImage = try await SCScreenshotManager.captureImage(
        contentFilter: contentFilter,
        configuration: streamConfiguration
      )
      let bitmap = NSBitmapImageRep(cgImage: cgImage)
      guard let pngData = bitmap.representation(using: .png, properties: [:]) else {
        throw DisplayCaptureError(
          code: "capture_failed",
          message: "Failed to encode ScreenCaptureKit display image",
          details: nil
        )
      }

      let logicalFrame = display.frame
      let logicalY = globalTop - logicalFrame.origin.y - logicalFrame.height
      let rotation = Int(CGDisplayRotation(display.displayID).rounded())

      snapshots.append(
        [
          "displayId": String(display.displayID),
          "logicalBounds": [
            "x": logicalFrame.origin.x,
            "y": logicalY,
            "width": logicalFrame.width,
            "height": logicalFrame.height,
          ],
          "pixelBounds": [
            "x": logicalFrame.origin.x * scale,
            "y": logicalY * scale,
            "width": CGFloat(cgImage.width),
            "height": CGFloat(cgImage.height),
          ],
          "scale": scale,
          "rotation": rotation,
          "imageBytesBase64": pngData.base64EncodedString(),
        ])
    }

    return snapshots
  }

  private func captureAllDisplays() async throws -> [[String: Any]] {
    if #available(macOS 14.0, *) {
      do {
        return try await captureAllDisplaysWithScreenCaptureKit()
      } catch let error as DisplayCaptureError {
        if error.code == "permission_denied" {
          throw error
        }

        // Keep the existing CGDisplay fallback for older runners or partial ScreenCaptureKit
        // failures so screenshot capture still works on supported macOS builds that haven't
        // fully transitioned to the newer API surface yet.
        log("ScreenCaptureKit capture failed, falling back to CGDisplayCreateImage: \(error.message)")
      } catch {
        // Surface unexpected native failures in the fallback log as well so we still capture
        // evidence when the newer API fails before the legacy path is attempted.
        log("ScreenCaptureKit capture failed, falling back to CGDisplayCreateImage: \(error.localizedDescription)")
      }
    }

    return try captureAllDisplaysLegacy()
  }

  private func postKeyboardEvent(key: String, isDown: Bool) -> FlutterError? {
    guard let keyCode = keyCode(for: key) else {
      return FlutterError(code: "UNSUPPORTED_KEY", message: "Unsupported key for macOS system input", details: key)
    }

    guard let event = CGEvent(keyboardEventSource: CGEventSource(stateID: .hidSystemState), virtualKey: keyCode, keyDown: isDown) else {
      return FlutterError(code: "INPUT_ERROR", message: "Failed to create macOS keyboard event", details: key)
    }

    event.post(tap: .cghidEventTap)
    return nil
  }

  private func postMouseButtonEvent(button: String, isDown: Bool) -> FlutterError? {
    guard let mouseButton = mouseButton(for: button) else {
      return FlutterError(code: "UNSUPPORTED_BUTTON", message: "Unsupported mouse button for macOS system input", details: button)
    }

    let eventTypes = mouseEventTypes(for: mouseButton)
    let eventType = isDown ? eventTypes.down : eventTypes.up
    guard
      let event = CGEvent(mouseEventSource: CGEventSource(stateID: .hidSystemState), mouseType: eventType, mouseCursorPosition: currentMouseLocation(), mouseButton: mouseButton)
    else {
      return FlutterError(code: "INPUT_ERROR", message: "Failed to create macOS mouse event", details: button)
    }

    event.post(tap: .cghidEventTap)
    return nil
  }

  private func moveMouse(to point: CGPoint) -> FlutterError? {
    CGWarpMouseCursorPosition(screenPoint(fromTopLeft: point))
    return nil
  }

  override func applicationShouldTerminateAfterLastWindowClosed(_: NSApplication) -> Bool {
    return false
  }

  override func applicationSupportsSecureRestorableState(_: NSApplication) -> Bool {
    return true
  }

  /// Apply acrylic effect to window
  private func applyAcrylicEffect(to window: NSWindow) {
    // Set appearance based on current theme
    if currentAppearance == "dark" {
      window.appearance = NSAppearance(named: .darkAqua)
    } else {
      window.appearance = NSAppearance(named: .aqua)
    }

    if let contentView = window.contentView {
      // Remove existing visual effect view if any to avoid stacking
      for subview in contentView.subviews {
        if subview is NSVisualEffectView {
          subview.removeFromSuperview()
        }
      }

      let effectView = NSVisualEffectView(frame: contentView.bounds)
      effectView.material = .popover
      effectView.state = .active
      effectView.blendingMode = .behindWindow
      // Ensure the effect view resizes with the window
      effectView.autoresizingMask = [.width, .height]
      contentView.addSubview(effectView, positioned: .below, relativeTo: nil)

      // Try to make all Flutter-related views transparent
      for subview in contentView.subviews where !(subview is NSVisualEffectView) {
        subview.wantsLayer = true
        subview.layer?.backgroundColor = NSColor.clear.cgColor
      }
    }
  }

  // Setup notification for window blur event
  private func setupWindowBlurNotification() {
    guard let window = mainFlutterWindow else { return }

    NotificationCenter.default.addObserver(
      self,
      selector: #selector(windowDidResignKey),
      name: NSWindow.didResignKeyNotification,
      object: window
    )
  }

  // Handle window loss of focus
  @objc private func windowDidResignKey(_: Notification) {
    log("Window did resign key (blur)")
    if mainFlutterWindow?.isVisible == true {
      shouldRestorePreviousAppOnHide = false
    }
    // Notify Flutter about the window blur event
    DispatchQueue.main.async {
      self.windowEventChannel?.invokeMethod("onWindowBlur", arguments: nil)
    }
  }

  override func applicationDidFinishLaunching(_ notification: Notification) {
    let controller = mainFlutterWindow?.contentViewController as! FlutterViewController

    // Try to make Flutter view background transparent
    let flutterView = controller.view
    flutterView.wantsLayer = true
    flutterView.layer?.backgroundColor = NSColor.clear.cgColor

    let channel = FlutterMethodChannel(
      name: "com.wox.macos_window_manager",
      binaryMessenger: controller.engine.binaryMessenger
    )

    // Store window event channel for use in window events
    windowEventChannel = channel

    // Setup window blur notification
    setupWindowBlurNotification()

    channel.setMethodCallHandler { [weak self] call, result in
      guard let window = self?.mainFlutterWindow else {
        result(FlutterError(code: "NO_WINDOW", message: "No window found", details: nil))
        return
      }

      DispatchQueue.main.async {
        switch call.method {
        case "captureAllDisplays":
          Task { @MainActor in
            do {
              result(try await self?.captureAllDisplays())
            } catch let error as DisplayCaptureError {
              // Convert the Swift-native capture error back to `FlutterError` only when returning
              // through the method channel so the Dart side keeps the existing error contract.
              result(error.asFlutterError())
            } catch {
              result(
                FlutterError(
                  code: "capture_failed",
                  message: error.localizedDescription,
                  details: nil
                ))
            }
          }
          return

        case "presentCaptureWorkspace":
          if let args = call.arguments as? [String: Any], let bounds = self?.parseRect(arguments: args) {
            result(self?.presentCaptureWorkspace(on: window, bounds: bounds))
          } else {
            result(
              FlutterError(
                code: "INVALID_ARGS",
                message: "Invalid arguments for presentCaptureWorkspace",
                details: nil
              )
            )
          }

        case "dismissCaptureWorkspacePresentation":
          self?.dismissCaptureWorkspacePresentation(on: window)
          result(nil)

        case "debugCaptureWorkspaceState":
          result(self?.debugCaptureWorkspaceState(for: window))

        case "setSize":
          if let args = call.arguments as? [String: Any],
            let width = args["width"] as? Double,
            let height = args["height"] as? Double
          {
            // Keep top-left stable when resizing; direct setContentSize can shift Y on macOS.
            let currentFrame = window.frame
            let currentTop = currentFrame.origin.y + currentFrame.height
            let targetFrame = window.frameRect(forContentRect: NSRect(x: 0, y: 0, width: width, height: height))
            let newOriginY = currentTop - targetFrame.height
            window.setFrame(
              NSRect(
                x: currentFrame.origin.x,
                y: newOriginY,
                width: targetFrame.width,
                height: targetFrame.height
              ),
              display: true
            )
            result(nil)
          } else {
            result(
              FlutterError(
                code: "INVALID_ARGS", message: "Invalid arguments for setSize", details: nil
              ))
          }

        case "inputKeyDown":
          if let args = call.arguments as? [String: Any],
            let key = args["key"] as? String
          {
            if let error = self?.postKeyboardEvent(key: key, isDown: true) {
              result(error)
            } else {
              result(nil)
            }
          } else {
            result(FlutterError(code: "INVALID_ARGS", message: "Missing key for keyboard input", details: nil))
          }

        case "inputKeyUp":
          if let args = call.arguments as? [String: Any],
            let key = args["key"] as? String
          {
            if let error = self?.postKeyboardEvent(key: key, isDown: false) {
              result(error)
            } else {
              result(nil)
            }
          } else {
            result(FlutterError(code: "INVALID_ARGS", message: "Missing key for keyboard input", details: nil))
          }

        case "inputMouseMove":
          if let args = call.arguments as? [String: Any],
            let x = args["x"] as? Double,
            let y = args["y"] as? Double
          {
            if let error = self?.moveMouse(to: CGPoint(x: x, y: y)) {
              result(error)
            } else {
              result(nil)
            }
          } else {
            result(FlutterError(code: "INVALID_ARGS", message: "Missing coordinates for mouse move", details: nil))
          }

        case "inputMouseDown":
          if let args = call.arguments as? [String: Any],
            let button = args["button"] as? String
          {
            if let error = self?.postMouseButtonEvent(button: button, isDown: true) {
              result(error)
            } else {
              result(nil)
            }
          } else {
            result(FlutterError(code: "INVALID_ARGS", message: "Missing mouse button", details: nil))
          }

        case "inputMouseUp":
          if let args = call.arguments as? [String: Any],
            let button = args["button"] as? String
          {
            if let error = self?.postMouseButtonEvent(button: button, isDown: false) {
              result(error)
            } else {
              result(nil)
            }
          } else {
            result(FlutterError(code: "INVALID_ARGS", message: "Missing mouse button", details: nil))
          }

        case "setBounds":
          if let args = call.arguments as? [String: Any],
            let x = args["x"] as? Double,
            let y = args["y"] as? Double,
            let width = args["width"] as? Double,
            let height = args["height"] as? Double
          {
            let frameRect = window.frameRect(forContentRect: NSRect(x: 0, y: 0, width: width, height: height))
            // Screenshot capture stretches one window across the virtual desktop, so screen-local
            // Y conversion is not enough once monitors sit at different heights.
            let flippedY = appKitY(fromTopLeftY: y, height: frameRect.height)
            window.setFrame(NSRect(x: x, y: flippedY, width: frameRect.width, height: frameRect.height), display: true)
            result(nil)
          } else {
            result(
              FlutterError(
                code: "INVALID_ARGS", message: "Invalid arguments for setBounds", details: nil
              ))
          }

        case "getPosition":
          let frame = window.frame
          // Return the shared virtual-desktop top-left position so saved window locations round-trip
          // correctly even after the window temporarily spans multiple displays for screenshot capture.
          let x = frame.origin.x
          let y = topLeftY(fromAppKitY: frame.origin.y, height: frame.height)
          result(["x": x, "y": y])

        case "getSize":
          // Keep getSize consistent with setSize/setBounds, which both accept
          // content rect dimensions from Dart. Returning frame size here makes
          // the controller think the window is taller than the actual Flutter
          // content area on macOS, which can skip needed resizes and cause
          // transient RenderFlex overflows in smoke tests.
          let contentRect = window.contentRect(forFrameRect: window.frame)
          result(["width": contentRect.width, "height": contentRect.height])

        case "setPosition":
          if let args = call.arguments as? [String: Any],
            let x = args["x"] as? Double,
            let y = args["y"] as? Double
          {
            // Keep launcher positioning on the same virtual-desktop contract as screenshot capture.
            // The previous screen-relative conversion restored windows to the wrong Y whenever the
            // saved position belonged to a display that was vertically offset from the main screen.
            let flippedY = appKitY(fromTopLeftY: y, height: window.frame.height)

            window.setFrameOrigin(NSPoint(x: x, y: flippedY))
            result(nil)
          } else {
            result(
              FlutterError(
                code: "INVALID_ARGS", message: "Invalid arguments for setPosition", details: nil
              ))
          }

        case "center":
          // Get the screen where the mouse cursor is located
          let mouseLocation = NSEvent.mouseLocation
          var targetScreen: NSScreen? = nil
          for screen in NSScreen.screens {
            if screen.frame.contains(mouseLocation) {
              targetScreen = screen
              break
            }
          }
          let screenFrame = targetScreen?.frame ?? NSScreen.main?.frame ?? NSRect.zero

          var windowWidth: CGFloat = window.frame.width
          var windowHeight: CGFloat = window.frame.height
          if let args = call.arguments as? [String: Any] {
            if let width = args["width"] as? Double {
              windowWidth = CGFloat(width)
            }
            if let height = args["height"] as? Double {
              windowHeight = CGFloat(height)
            }
          }

          let x = (screenFrame.width - windowWidth) / 2 + screenFrame.minX
          let y = (screenFrame.height - windowHeight) / 2 + screenFrame.minY

          self?.log("Center: window to \(x),\(y) on screen at \(screenFrame.minX),\(screenFrame.minY)")
          let newFrame = NSRect(x: x, y: y, width: windowWidth, height: windowHeight)
          window.setFrame(newFrame, display: true)
          result(nil)

        case "show":
          self?.log("Showing Wox window")
          self?.savePreviousActiveAppIfNeeded()

          window.makeKeyAndOrderFront(nil)
          NSApp.activate(ignoringOtherApps: true)
          result(nil)

        case "hide":
          self?.log("Hiding Wox window")
          let isWoxFrontmost = NSApp.isActive || NSWorkspace.shared.frontmostApplication == NSRunningApplication.current
          let shouldRestorePreviousApp = self?.shouldRestorePreviousAppOnHide == true
          window.orderOut(nil)
          // Only restore the previous app when Wox stayed focused since the last show/focus.
          if isWoxFrontmost && shouldRestorePreviousApp {
            if let prevApp = self?.previousActiveApp, prevApp != NSRunningApplication.current, !prevApp.isTerminated {
              self?.log("Activating previous app: \(prevApp.localizedName ?? "Unknown") (bundleID: \(prevApp.bundleIdentifier ?? "Unknown"))")
              prevApp.activate(options: .activateIgnoringOtherApps)
            } else {
              self?.log("No valid previous app saved for activation")
            }
          } else if !shouldRestorePreviousApp {
            self?.log("Skipping previous app activation because Wox already lost focus before hiding")
          } else {
            self?.log("Wox is not frontmost when hiding, skipping previous app activation")
          }
          self?.previousActiveApp = nil
          self?.shouldRestorePreviousAppOnHide = false
          result(nil)

        case "focus":
          self?.savePreviousActiveAppIfNeeded()
          window.makeKeyAndOrderFront(nil)
          NSApp.activate(ignoringOtherApps: true)
          result(nil)

        case "isVisible":
          result(window.isVisible)

        case "setAlwaysOnTop":
          if let alwaysOnTop = call.arguments as? Bool {
            if alwaysOnTop {
              window.level = .popUpMenu
            } else {
              window.level = .normal
            }

            result(nil)
          } else {
            result(
              FlutterError(
                code: "INVALID_ARGS", message: "Invalid arguments for setAlwaysOnTop", details: nil
              )
            )
          }

        case "setAppearance":
          if let appearance = call.arguments as? String {
            self?.currentAppearance = appearance
            self?.applyAcrylicEffect(to: window)
            result(nil)
          } else {
            result(
              FlutterError(
                code: "INVALID_ARGS", message: "Invalid arguments for setAppearance", details: nil
              )
            )
          }

        case "startDragging":
          if let currentEvent = window.currentEvent {
            self?.log("Performing drag with event: \(currentEvent)")
            window.performDrag(with: currentEvent)
          }
          result(nil)

        case "waitUntilReadyToShow":
          // Set app appearance based on current theme
          if self?.currentAppearance == "dark" {
            NSApp.appearance = NSAppearance(named: .darkAqua)
          } else {
            NSApp.appearance = NSAppearance(named: .aqua)
          }

          window.level = .popUpMenu
          window.titlebarAppearsTransparent = true
          window.styleMask.insert(.fullSizeContentView)
          window.styleMask.insert(.nonactivatingPanel)
          window.styleMask.remove(.resizable)

          // Hide windows buttons
          window.titleVisibility = .hidden
          window.standardWindowButton(.closeButton)?.isHidden = true
          window.standardWindowButton(.miniaturizeButton)?.isHidden = true
          window.standardWindowButton(.zoomButton)?.isHidden = true

          // Make window can join all spaces
          window.collectionBehavior.insert(.canJoinAllSpaces)
          window.collectionBehavior.insert(.fullScreenAuxiliary)
          window.styleMask.insert(.nonactivatingPanel)
          self?.applyAcrylicEffect(to: window)

          if let mainWindow = window as? MainFlutterWindow {
            mainWindow.isReadyToShow = true
          }

          result(nil)

        default:
          result(FlutterMethodNotImplemented)
        }
      }
    }

    super.applicationDidFinishLaunching(notification)
  }
}

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

private func topLeftPoint(fromAppKit point: CGPoint) -> CGPoint {
  return CGPoint(x: point.x, y: topLeftY(fromAppKitY: point.y, height: 0))
}

private func topLeftRect(fromAppKitRect rect: NSRect) -> NSRect {
  return NSRect(
    x: rect.origin.x,
    y: topLeftY(fromAppKitY: rect.origin.y, height: rect.height),
    width: rect.width,
    height: rect.height
  )
}

private func appKitRect(fromTopLeftRect rect: NSRect) -> NSRect {
  return NSRect(
    x: rect.origin.x,
    y: appKitY(fromTopLeftY: rect.origin.y, height: rect.height),
    width: rect.width,
    height: rect.height
  )
}

private func clampPoint(_ point: CGPoint, to bounds: NSRect) -> CGPoint {
  return CGPoint(
    x: min(max(point.x, bounds.minX), bounds.maxX),
    y: min(max(point.y, bounds.minY), bounds.maxY)
  )
}

private func rectFromPoints(_ start: CGPoint, _ end: CGPoint) -> NSRect {
  return NSRect(
    x: min(start.x, end.x),
    y: min(start.y, end.y),
    width: abs(end.x - start.x),
    height: abs(end.y - start.y)
  )
}

private func intersectionArea(_ lhs: NSRect, _ rhs: NSRect) -> CGFloat {
  let intersection = lhs.intersection(rhs)
  if intersection.isEmpty {
    return 0
  }

  return intersection.width * intersection.height
}

private struct CachedDisplayCapture {
  let displayId: String
  let logicalBounds: NSRect
  let visibleBounds: NSRect
  let scale: CGFloat
  let rotation: Int
  let image: CGImage
}

private struct NativeSelectionOverlayResult {
  let selection: NSRect?
  let editorVisibleBounds: NSRect?
}

private final class ScreenshotOverlayView: NSView {
  private static let selectionBorderColor = NSColor(red: 41 / 255, green: 1, blue: 114 / 255, alpha: 1)
  private static let overlayColor = NSColor(calibratedWhite: 0, alpha: 0.46)
  private static let labelBackgroundColor = NSColor(calibratedWhite: 0.09, alpha: 0.9)
  private static let selectionShadowColor = NSColor(calibratedWhite: 0, alpha: 0.2)

  let capture: CachedDisplayCapture

  var globalSelection: NSRect? {
    didSet {
      needsDisplay = true
    }
  }

  var shouldDrawSizeLabel = false {
    didSet {
      needsDisplay = true
    }
  }

  override var isFlipped: Bool {
    return true
  }

  init(capture: CachedDisplayCapture) {
    self.capture = capture
    super.init(frame: NSRect(origin: .zero, size: capture.logicalBounds.size))
    wantsLayer = true
    layer?.backgroundColor = NSColor.clear.cgColor
  }

  @available(*, unavailable)
  required init?(coder: NSCoder) {
    fatalError("init(coder:) has not been implemented")
  }

  override func acceptsFirstMouse(for event: NSEvent?) -> Bool {
    return true
  }

  override func resetCursorRects() {
    discardCursorRects()
    addCursorRect(bounds, cursor: .crosshair)
  }

  override func draw(_ dirtyRect: NSRect) {
    super.draw(dirtyRect)

    guard let context = NSGraphicsContext.current?.cgContext else {
      return
    }

    context.saveGState()
    context.interpolationQuality = .high
    // `CGContext.draw` still uses a bottom-left image coordinate system even when the NSView is
    // flipped. Drawing without an explicit Y flip makes every monitor preview appear upside down.
    context.translateBy(x: 0, y: bounds.height)
    context.scaleBy(x: 1, y: -1)
    context.draw(capture.image, in: NSRect(origin: .zero, size: bounds.size))
    context.scaleBy(x: 1, y: -1)
    context.translateBy(x: 0, y: -bounds.height)

    let overlayPath = NSBezierPath(rect: bounds)
    if let localSelection = localSelectionRect() {
      overlayPath.appendRect(localSelection)
      overlayPath.windingRule = .evenOdd
    }
    ScreenshotOverlayView.overlayColor.setFill()
    overlayPath.fill()

    if let localSelection = localSelectionRect() {
      drawSelectionBorder(in: localSelection)
      if shouldDrawSizeLabel, let globalSelection = globalSelection {
        drawSelectionSizeLabel(for: globalSelection)
      }
    }

    context.restoreGState()
  }

  private func localSelectionRect() -> NSRect? {
    guard let globalSelection = globalSelection, !globalSelection.isEmpty else {
      return nil
    }

    let intersection = capture.logicalBounds.intersection(globalSelection)
    if intersection.isEmpty {
      return nil
    }

    return NSRect(
      x: intersection.minX - capture.logicalBounds.minX,
      y: intersection.minY - capture.logicalBounds.minY,
      width: intersection.width,
      height: intersection.height
    )
  }

  private func drawSelectionBorder(in localSelection: NSRect) {
    let borderRect = localSelection.insetBy(dx: 1, dy: 1)
    let shadow = NSShadow()
    shadow.shadowBlurRadius = 18
    shadow.shadowOffset = NSSize(width: 0, height: 0)
    shadow.shadowColor = ScreenshotOverlayView.selectionShadowColor
    shadow.set()

    let borderPath = NSBezierPath(rect: borderRect)
    borderPath.lineWidth = 2
    ScreenshotOverlayView.selectionBorderColor.setStroke()
    borderPath.stroke()
  }

  private func drawSelectionSizeLabel(for globalSelection: NSRect) {
    let label = "\(Int(globalSelection.width.rounded())) x \(Int(globalSelection.height.rounded()))"
    let attributes: [NSAttributedString.Key: Any] = [
      .font: NSFont.systemFont(ofSize: 14, weight: .bold),
      .foregroundColor: NSColor.white,
    ]
    let labelSize = label.size(withAttributes: attributes)
    let labelWidth = labelSize.width + 16
    let labelHeight = labelSize.height + 8
    let preferredAboveY = globalSelection.minY - labelHeight - 10
    let labelY =
      preferredAboveY >= capture.logicalBounds.minY + 8
      ? preferredAboveY
      : min(globalSelection.maxY + 10, capture.logicalBounds.maxY - labelHeight - 8)
    let labelX = min(
      max(globalSelection.minX + 12, capture.logicalBounds.minX + 8),
      capture.logicalBounds.maxX - labelWidth - 8
    )
    let localRect = NSRect(
      x: labelX - capture.logicalBounds.minX,
      y: labelY - capture.logicalBounds.minY,
      width: labelWidth,
      height: labelHeight
    )

    let backgroundPath = NSBezierPath(roundedRect: localRect, xRadius: 10, yRadius: 10)
    ScreenshotOverlayView.labelBackgroundColor.setFill()
    backgroundPath.fill()
    label.draw(
      at: CGPoint(x: localRect.minX + 8, y: localRect.minY + 4),
      withAttributes: attributes
    )
  }
}

private final class ScreenshotOverlayWindow: NSWindow {
  let overlayView: ScreenshotOverlayView

  override var canBecomeKey: Bool {
    return true
  }

  override var canBecomeMain: Bool {
    return true
  }

  init(capture: CachedDisplayCapture) {
    overlayView = ScreenshotOverlayView(capture: capture)
    super.init(
      contentRect: appKitRect(fromTopLeftRect: capture.logicalBounds),
      styleMask: .borderless,
      backing: .buffered,
      defer: false
    )

    isOpaque = false
    backgroundColor = .clear
    hasShadow = false
    // The overlay session owns these windows explicitly. Releasing them as a side effect of close()
    // makes lifetime depend on AppKit event timing and was the most likely source of the drag-time crash.
    isReleasedWhenClosed = false
    level = .popUpMenu
    ignoresMouseEvents = false
    acceptsMouseMovedEvents = true
    titleVisibility = .hidden
    titlebarAppearsTransparent = true
    animationBehavior = .none
    collectionBehavior = [
      .canJoinAllSpaces,
      .fullScreenAuxiliary,
      .stationary,
      .ignoresCycle,
    ]
    if #available(macOS 13.0, *) {
      collectionBehavior.insert(.canJoinAllApplications)
    }

    contentView = overlayView
  }
}

private final class ScreenshotOverlaySession {
  private let workspaceBounds: NSRect
  private let captures: [CachedDisplayCapture]
  private let windows: [ScreenshotOverlayWindow]
  private let onComplete: (NativeSelectionOverlayResult) -> Void
  private var localEventMonitor: Any?
  private var dragStart: CGPoint?
  private var isCompleting = false
  private var overlaysDismissed = false

  init(
    workspaceBounds: NSRect,
    captures: [CachedDisplayCapture],
    onComplete: @escaping (NativeSelectionOverlayResult) -> Void
  ) {
    self.workspaceBounds = workspaceBounds
    self.captures = captures
    self.windows = captures.map(ScreenshotOverlayWindow.init)
    self.onComplete = onComplete
  }

  func begin() {
    installEventMonitor()
    for window in windows {
      window.orderFrontRegardless()
    }
    windows.first?.makeKeyAndOrderFront(nil)
    NSApp.activate(ignoringOtherApps: true)
  }

  func cancel() {
    complete(with: NativeSelectionOverlayResult(selection: nil, editorVisibleBounds: nil))
  }

  func dismissOverlays() {
    if overlaysDismissed {
      return
    }

    overlaysDismissed = true
    let windowsToDismiss = windows
    DispatchQueue.main.async {
      for window in windowsToDismiss {
        window.orderOut(nil)
        window.contentView = nil
        window.close()
      }
    }
  }

  private func installEventMonitor() {
    localEventMonitor = NSEvent.addLocalMonitorForEvents(matching: [.leftMouseDown, .leftMouseDragged, .leftMouseUp, .keyDown]) { [weak self] event in
      return self?.handle(event: event) ?? event
    }
  }

  private func handle(event: NSEvent) -> NSEvent? {
    if isCompleting {
      return nil
    }

    switch event.type {
    case .keyDown:
      if event.keyCode == 53 {
        cancel()
        return nil
      }

      return nil

    case .leftMouseDown:
      let point = clampPoint(topLeftPoint(fromAppKit: NSEvent.mouseLocation), to: workspaceBounds)
      dragStart = point
      updateSelection(rectFromPoints(point, point))
      return nil

    case .leftMouseDragged:
      guard let dragStart else {
        return nil
      }

      let point = clampPoint(topLeftPoint(fromAppKit: NSEvent.mouseLocation), to: workspaceBounds)
      updateSelection(rectFromPoints(dragStart, point))
      return nil

    case .leftMouseUp:
      guard let dragStart else {
        return nil
      }

      let point = clampPoint(topLeftPoint(fromAppKit: NSEvent.mouseLocation), to: workspaceBounds)
      let selection = rectFromPoints(dragStart, point)
      completeSelection(selection)
      return nil

    default:
      return event
    }
  }

  private func updateSelection(_ selection: NSRect?) {
    let labelDisplayId = selection.flatMap(selectionLabelDisplayId(for:))
    for window in windows {
      window.overlayView.globalSelection = selection
      window.overlayView.shouldDrawSizeLabel = window.overlayView.capture.displayId == labelDisplayId
    }
  }

  private func completeSelection(_ selection: NSRect) {
    if selection.width < 1 || selection.height < 1 {
      cancel()
      return
    }

    let editorVisibleBounds = preferredEditorVisibleBounds(for: selection)
    complete(with: NativeSelectionOverlayResult(selection: selection, editorVisibleBounds: editorVisibleBounds))
  }

  private func complete(with result: NativeSelectionOverlayResult) {
    if isCompleting {
      return
    }

    isCompleting = true
    dragStart = nil
    if let localEventMonitor {
      NSEvent.removeMonitor(localEventMonitor)
      self.localEventMonitor = nil
    }

    // The drag phase ends here, but successful handoff keeps the overlay windows alive so Flutter
    // can reuse the exact same captured backdrop underneath its annotation controls. Cancel still
    // closes immediately because there is no follow-up editor that needs the native background.
    if result.selection == nil {
      dismissOverlays()
    }

    let onComplete = self.onComplete
    DispatchQueue.main.async {
      onComplete(result)
    }
  }

  private func selectionLabelDisplayId(for selection: NSRect) -> String? {
    let anchorPoint = CGPoint(x: selection.minX + 12, y: selection.minY + 12)
    if let containingCapture = captures.first(where: { $0.logicalBounds.contains(anchorPoint) }) {
      return containingCapture.displayId
    }

    if let originCapture = captures.first(where: { $0.logicalBounds.contains(selection.origin) }) {
      return originCapture.displayId
    }

    return preferredCapture(for: selection)?.displayId
  }

  private func preferredEditorVisibleBounds(for selection: NSRect) -> NSRect? {
    return preferredCapture(for: selection)?.visibleBounds
  }

  private func preferredCapture(for selection: NSRect) -> CachedDisplayCapture? {
    let selectionCenter = CGPoint(x: selection.midX, y: selection.midY)
    if let centeredCapture = captures.first(where: { $0.logicalBounds.contains(selectionCenter) }) {
      return centeredCapture
    }

    return captures.max(by: { intersectionArea($0.logicalBounds, selection) < intersectionArea($1.logicalBounds, selection) })
  }
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
    let styleMask: NSWindow.StyleMask
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
  // Native multi-display selection reuses the already captured CGImages so the selector can draw
  // one full-resolution background per monitor without sending large image payloads back to Swift.
  private var cachedDisplayCaptures: [CachedDisplayCapture] = []
  private var activeOverlaySession: ScreenshotOverlaySession?
  private var nativeOverlayDismissTimeoutWorkItem: DispatchWorkItem?

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

  private func screen(for displayId: CGDirectDisplayID) -> NSScreen? {
    return NSScreen.screens.first { screen in
      guard let screenNumber = screen.deviceDescription[NSDeviceDescriptionKey("NSScreenNumber")] as? NSNumber else {
        return false
      }

      return CGDirectDisplayID(screenNumber.uint32Value) == displayId
    }
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

  private func cancelNativeOverlayDismissTimeout() {
    nativeOverlayDismissTimeoutWorkItem?.cancel()
    nativeOverlayDismissTimeoutWorkItem = nil
  }

  private func dismissNativeSelectionOverlays() {
    cancelNativeOverlayDismissTimeout()
    guard let activeOverlaySession else {
      return
    }

    self.activeOverlaySession = nil
    activeOverlaySession.dismissOverlays()
  }

  private func scheduleNativeOverlayDismissTimeout() {
    cancelNativeOverlayDismissTimeout()
    guard activeOverlaySession != nil else {
      return
    }

    let workItem = DispatchWorkItem { [weak self] in
      // Flutter should acknowledge the handoff quickly by converting the selector into a passive
      // backdrop. This timeout only exists to prevent a leaked topmost drag overlay if that
      // handoff fails before Flutter starts driving the session.
      self?.log("Native selection overlay timed out while waiting for Flutter to activate the backdrop handoff; dismissing overlays")
      self?.dismissNativeSelectionOverlays()
    }
    nativeOverlayDismissTimeoutWorkItem = workItem
    DispatchQueue.main.asyncAfter(deadline: .now() + 10, execute: workItem)
  }

  private func selectCaptureRegion(
    workspaceBounds: NSRect,
    completion: @escaping (Result<[String: Any], DisplayCaptureError>) -> Void
  ) {
    guard activeOverlaySession == nil else {
      completion(
        .failure(
          DisplayCaptureError(
            code: "busy",
            message: "A screenshot selection session is already active",
            details: nil
          )
        )
      )
      return
    }

    let captures = cachedDisplayCaptures.filter { !$0.logicalBounds.intersection(workspaceBounds).isEmpty }
    guard captures.count >= 2 else {
      completion(.success(["wasHandled": false]))
      return
    }

    // The native selector uses one borderless window per display because the single Flutter window
    // path only renders reliably on the primary monitor. Matching Apple's per-screen overlay model
    // keeps the drag interaction stable across mixed-resolution and fullscreen spaces.
    let overlaySession = ScreenshotOverlaySession(workspaceBounds: workspaceBounds, captures: captures) { [weak self] result in
      guard let self else {
        completion(.success(["wasHandled": false]))
        return
      }

      if result.selection == nil {
        self.cancelNativeOverlayDismissTimeout()
        self.activeOverlaySession = nil
      } else {
        self.scheduleNativeOverlayDismissTimeout()
      }
      let payload: [String: Any] = [
        "wasHandled": true,
        "selection": result.selection.map { self.buildRectPayload($0) } ?? NSNull(),
        "editorVisibleBounds": result.editorVisibleBounds.map { self.buildRectPayload($0) } ?? NSNull(),
      ]
      completion(
        .success(payload)
      )
    }

    activeOverlaySession = overlaySession
    // Native selection activates Wox again after the launcher window hid itself. Saving the current
    // frontmost app here preserves the same focus-restore behavior that the regular window show/hide
    // path already has when the screenshot session later cancels or finishes from a hidden state.
    savePreviousActiveAppIfNeeded()
    overlaySession.begin()
  }

  private func presentCaptureWorkspace(on window: NSWindow, bounds: NSRect) -> [String: Any] {
    if screenshotPresentationState == nil {
      screenshotPresentationState = ScreenshotPresentationState(
        collectionBehavior: window.collectionBehavior,
        level: window.level,
        hasShadow: window.hasShadow,
        styleMask: window.styleMask
      )
    }

    // Making the window borderless ensures the content rect equals the frame rect, so the
    // captured background fills the entire monitor area without a title bar offset pushing
    // the content down and exposing the macOS menubar underneath.
    window.styleMask = .borderless
    let flippedY = appKitY(fromTopLeftY: bounds.origin.y, height: bounds.height)

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
    window.setFrame(NSRect(x: bounds.origin.x, y: flippedY, width: bounds.width, height: bounds.height), display: true)
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
    window.styleMask = savedState.styleMask
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

    var snapshots: [[String: Any]] = []
    var cachedCaptures: [CachedDisplayCapture] = []

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

      let frame = topLeftRect(fromAppKitRect: screen.frame)
      let visibleFrame = topLeftRect(fromAppKitRect: screen.visibleFrame)
      let scale = screen.backingScaleFactor
      let rotation = Int(CGDisplayRotation(displayId).rounded())

      cachedCaptures.append(
        CachedDisplayCapture(
          displayId: String(displayId),
          logicalBounds: frame,
          visibleBounds: visibleFrame,
          scale: scale,
          rotation: rotation,
          image: cgImage
        )
      )
      snapshots.append(
        [
          "displayId": String(displayId),
          "logicalBounds": [
            "x": frame.origin.x,
            "y": frame.origin.y,
            "width": frame.width,
            "height": frame.height,
          ],
          "pixelBounds": [
            "x": frame.origin.x * scale,
            "y": frame.origin.y * scale,
            "width": CGFloat(cgImage.width),
            "height": CGFloat(cgImage.height),
          ],
          "scale": scale,
          "rotation": rotation,
          "imageBytesBase64": pngData.base64EncodedString(),
        ])
    }

    cachedDisplayCaptures = cachedCaptures
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

    let excludedApplications = shareableContent.applications.filter {
      $0.bundleIdentifier == Bundle.main.bundleIdentifier
    }
    var snapshots: [[String: Any]] = []
    var cachedCaptures: [CachedDisplayCapture] = []

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

      let matchedScreen = screen(for: display.displayID)
      // Mixed-resolution desktops exposed that ScreenCaptureKit's display frame can drift from the
      // AppKit window server layout that actually decides where NSWindow overlays appear. Basing the
      // overlay geometry on the matching NSScreen frame keeps the shaded window aligned with the
      // real monitor bounds even when one display uses a different native resolution or scale.
      let logicalFrame = matchedScreen.map { topLeftRect(fromAppKitRect: $0.frame) } ?? topLeftRect(fromAppKitRect: display.frame)
      let visibleFrame = matchedScreen.map { topLeftRect(fromAppKitRect: $0.visibleFrame) } ?? logicalFrame
      let rotation = Int(CGDisplayRotation(display.displayID).rounded())

      cachedCaptures.append(
        CachedDisplayCapture(
          displayId: String(display.displayID),
          logicalBounds: logicalFrame,
          visibleBounds: visibleFrame,
          scale: scale,
          rotation: rotation,
          image: cgImage
        )
      )
      snapshots.append(
        [
          "displayId": String(display.displayID),
          "logicalBounds": [
            "x": logicalFrame.origin.x,
            "y": logicalFrame.origin.y,
            "width": logicalFrame.width,
            "height": logicalFrame.height,
          ],
          "pixelBounds": [
            "x": logicalFrame.origin.x * scale,
            "y": logicalFrame.origin.y * scale,
            "width": CGFloat(cgImage.width),
            "height": CGFloat(cgImage.height),
          ],
          "scale": scale,
          "rotation": rotation,
          "imageBytesBase64": pngData.base64EncodedString(),
        ])
    }

    cachedDisplayCaptures = cachedCaptures
    return snapshots
  }

  private func captureAllDisplays() async throws -> [[String: Any]] {
    cachedDisplayCaptures = []
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

        case "selectCaptureRegion":
          if let args = call.arguments as? [String: Any], let bounds = self?.parseRect(arguments: args) {
            self?.selectCaptureRegion(workspaceBounds: bounds) { selectionResult in
              switch selectionResult {
              case .success(let payload):
                result(payload)
              case .failure(let error):
                result(error.asFlutterError())
              }
            }
          } else {
            result(
              FlutterError(
                code: "INVALID_ARGS",
                message: "Invalid arguments for selectCaptureRegion",
                details: nil
              )
            )
          }

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

        case "dismissNativeSelectionOverlays":
          self?.dismissNativeSelectionOverlays()
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

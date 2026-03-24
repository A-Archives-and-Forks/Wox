import Cocoa
import FlutterMacOS
import WebKit

private let mobileUserAgent =
  "Mozilla/5.0 (iPhone; CPU iPhone OS 18_7_6 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/26.0 Mobile/15E148 Safari/604.1"

private enum WoxWebViewSessionPolicy {
  case persistent
}

private struct WoxWebViewPreviewRequest {
  let urlString: String
  let injectCss: String
  let cacheEnabled: Bool
  let cacheKey: String

  init(args: [String: Any]) {
    urlString = args["url"] as? String ?? ""
    injectCss = args["injectCss"] as? String ?? ""
    cacheEnabled = args["cacheEnabled"] as? Bool ?? false
    cacheKey = args["cacheKey"] as? String ?? ""
  }

  var hasCache: Bool {
    cacheEnabled && !cacheKey.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty
  }

  var cacheSignature: String {
    "\(injectCss)|\(mobileUserAgent)"
  }
}

private final class WoxCachedWebViewEntry {
  let webView: WKWebView
  let signature: String
  var currentURL: String

  init(webView: WKWebView, signature: String, currentURL: String) {
    self.webView = webView
    self.signature = signature
    self.currentURL = currentURL
  }
}

private enum WoxWebViewStore {
  private static var entries: [String: WoxCachedWebViewEntry] = [:]

  static func resolveWebView(for request: WoxWebViewPreviewRequest) -> (webView: WKWebView, shouldReload: Bool) {
    guard request.hasCache else {
      return (makeWebView(for: request), true)
    }

    let normalizedKey = request.cacheKey.trimmingCharacters(in: .whitespacesAndNewlines)
    if let cached = entries[normalizedKey], cached.signature == request.cacheSignature {
      let shouldReload = cached.currentURL != request.urlString
      if shouldReload {
        cached.currentURL = request.urlString
      }
      return (cached.webView, shouldReload)
    }

    let webView = makeWebView(for: request)
    entries[normalizedKey] = WoxCachedWebViewEntry(
      webView: webView,
      signature: request.cacheSignature,
      currentURL: request.urlString
    )
    return (webView, true)
  }

  private static func makeWebView(for request: WoxWebViewPreviewRequest) -> WKWebView {
    let configuration = WoxWebViewPreviewNativeView.makeConfiguration(
      sessionPolicy: .persistent,
      injectCss: request.injectCss
    )
    let webView = WKWebView(frame: .zero, configuration: configuration)
    if #available(macOS 13.3, *) {
      webView.isInspectable = true
    }
    webView.customUserAgent = mobileUserAgent
    return webView
  }
}

class WoxWebViewPreviewPlugin: NSObject {
  private static weak var activeWebView: WKWebView?

  static func register(with registrar: FlutterPluginRegistrar) {
    let factory = WoxWebViewPreviewFactory()
    registrar.register(factory, withId: "wox/webview_preview")
  }

  static func setActiveWebView(_ webView: WKWebView) {
    activeWebView = webView
  }

  static func openInspector() -> Bool {
    guard let activeWebView else {
      return false
    }

    let showInspectorSelector = Selector(("_showWebInspector"))
    guard activeWebView.responds(to: showInspectorSelector) else {
      return false
    }

    activeWebView.perform(showInspectorSelector)
    return true
  }
}

class WoxWebViewPreviewFactory: NSObject, FlutterPlatformViewFactory {
  func create(withViewIdentifier viewId: Int64, arguments args: Any?) -> NSView {
    return WoxWebViewPreviewNativeView(frame: .zero, args: args)
  }

  func createArgsCodec() -> (FlutterMessageCodec & NSObjectProtocol)? {
    return FlutterStandardMessageCodec.sharedInstance()
  }
}

final class WoxWebViewPreviewNativeView: NSView, WKNavigationDelegate, WKUIDelegate {
  private let webView: WKWebView

  init(frame frameRect: NSRect, args: Any?) {
    let creationParams = args as? [String: Any] ?? [:]
    let request = WoxWebViewPreviewRequest(args: creationParams)
    let resolved = WoxWebViewStore.resolveWebView(for: request)

    webView = resolved.webView
    super.init(frame: frameRect)

    WoxWebViewPreviewPlugin.setActiveWebView(webView)
    webView.navigationDelegate = self
    webView.uiDelegate = self
    webView.autoresizingMask = [.width, .height]
    webView.frame = bounds
    webView.removeFromSuperview()
    addSubview(webView)

    wantsLayer = true
    layer?.backgroundColor = NSColor.clear.cgColor

    configure(with: request, shouldReload: resolved.shouldReload)
  }

  @available(*, unavailable)
  required init?(coder: NSCoder) {
    fatalError("init(coder:) has not been implemented")
  }

  fileprivate static func makeConfiguration(sessionPolicy: WoxWebViewSessionPolicy, injectCss: String?) -> WKWebViewConfiguration {
    let configuration = WKWebViewConfiguration()

    switch sessionPolicy {
    case .persistent:
      // Keep cookies and storage across Wox restarts.
      configuration.websiteDataStore = WKWebsiteDataStore.default()
    }

    if let injectCss, !injectCss.isEmpty {
      let userContentController = WKUserContentController()
      userContentController.addUserScript(
        WKUserScript(
          source: makeInjectCssScript(css: injectCss),
          injectionTime: .atDocumentEnd,
          forMainFrameOnly: true
        )
      )
      configuration.userContentController = userContentController
    }

    return configuration
  }

  private static func makeInjectCssScript(css: String) -> String {
    guard
      let cssData = try? JSONSerialization.data(withJSONObject: [css]),
      let cssArrayLiteral = String(data: cssData, encoding: .utf8)
    else {
      return ""
    }

    return """
      (() => {
        const css = \(cssArrayLiteral)[0];
        if (!css) {
          return;
        }

        const styleId = "wox-webview-preview-style";
        let style = document.getElementById(styleId);
        if (!style) {
          style = document.createElement("style");
          style.id = styleId;
          (document.head || document.documentElement).appendChild(style);
        }
        style.textContent = css;
      })();
      """
  }

  private func configure(with request: WoxWebViewPreviewRequest, shouldReload: Bool) {
    guard shouldReload, let url = URL(string: request.urlString) else {
      return
    }

    webView.load(URLRequest(url: url))
  }

  func webView(_ webView: WKWebView, createWebViewWith configuration: WKWebViewConfiguration, for navigationAction: WKNavigationAction, windowFeatures: WKWindowFeatures)
    -> WKWebView?
  {
    if navigationAction.targetFrame == nil, let url = navigationAction.request.url {
      webView.load(URLRequest(url: url))
    }

    return nil
  }
}

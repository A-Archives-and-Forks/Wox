import Cocoa
import FlutterMacOS

class MainFlutterWindow: NSPanel {
  var isReadyToShow: Bool = false
  private var webViewPreviewChannel: FlutterMethodChannel?

  override func awakeFromNib() {
    let flutterViewController = FlutterViewController()
    let windowFrame = self.frame
    self.contentViewController = flutterViewController
    self.setFrame(windowFrame, display: false)

    RegisterGeneratedPlugins(registry: flutterViewController)
    WoxWebViewPreviewPlugin.register(with: flutterViewController.registrar(forPlugin: "WoxWebViewPreviewPlugin"))

    let webViewPreviewChannel = FlutterMethodChannel(
      name: "com.wox.webview_preview",
      binaryMessenger: flutterViewController.engine.binaryMessenger
    )
    WoxWebViewPreviewPlugin.setMethodChannel(webViewPreviewChannel)
    webViewPreviewChannel.setMethodCallHandler { call, result in
      switch call.method {
      case "openInspector":
        result(WoxWebViewPreviewPlugin.openInspector())
      case "refresh":
        result(WoxWebViewPreviewPlugin.refresh())
      case "goBack":
        result(WoxWebViewPreviewPlugin.goBack())
      case "goForward":
        result(WoxWebViewPreviewPlugin.goForward())
      default:
        result(FlutterMethodNotImplemented)
      }
    }
    self.webViewPreviewChannel = webViewPreviewChannel

    super.awakeFromNib()
  }

  override public func order(_ place: NSWindow.OrderingMode, relativeTo otherWin: Int) {
    super.order(place, relativeTo: otherWin)

    if !isReadyToShow {
      setIsVisible(false)
    }
  }
}

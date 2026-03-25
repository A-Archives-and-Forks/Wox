class WoxWebViewSupport {
  static const String mobileUserAgent = "Mozilla/5.0 (iPhone; CPU iPhone OS 18_7_6 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/26.0 Mobile/15E148 Safari/604.1";

  static String buildInjectCssScript(String css) {
    final cssLiteral = _encodeJsString(css);
    return """
(() => {
  const css = $cssLiteral;
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
""";
  }

  static String _encodeJsString(String input) {
    final escaped = input
        .replaceAll(r'\', r'\\')
        .replaceAll("'", r"\'")
        .replaceAll('\r', r'\r')
        .replaceAll('\n', r'\n')
        .replaceAll('\u2028', r'\u2028')
        .replaceAll('\u2029', r'\u2029');
    return "'$escaped'";
  }
}

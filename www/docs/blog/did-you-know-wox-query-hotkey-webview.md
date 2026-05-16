---
title: "Did You Know: Wox Can Browse Websites with Query Hotkeys"
description: Use Query Hotkeys and the WebView plugin to open compact web panels for sites like X and Instagram.
date: 2026-05-16
---

# Did You Know: Wox Can Browse Websites with Query Hotkeys

Wox is usually treated as a launcher: open an app, find a file, run a command, move on. But it can also work as a small web panel when you combine two features: Query Hotkeys and the built-in WebView plugin.

The WebView plugin lets Wox preview configured websites inside the launcher. On first load, it includes two useful examples:

| Keyword | Site |
| --- | --- |
| `x` | `https://x.com` |
| `ig` | `https://www.instagram.com` |

That means `webview x` opens X in Wox, and `webview ig` opens Instagram in Wox.

![Wox WebView opened by a query hotkey](/images/did-you-know-wox-webview-hotkey.png)

The part that makes this feel fast is Query Hotkeys. Instead of opening Wox and typing `webview x` every time, bind a hotkey directly to that query:

| Query Hotkey field | Example value |
| --- | --- |
| Hotkey | Any available shortcut, such as `ctrl+shift+x` |
| Query | `webview x` |
| Position | A fixed position such as `top_right` |
| Hide Query Box | Enabled |
| Hide Toolbar | Enabled |
| Width | A compact width, such as `420` |
| Max Result Count | `5` is enough |

With the query box and toolbar hidden, the result preview becomes the focus. A quick hotkey opens the site as a compact, launcher-sized web panel instead of sending you to a full browser window.

Use the same pattern for Instagram:

| Query Hotkey field | Example value |
| --- | --- |
| Hotkey | Any available shortcut, such as `ctrl+shift+i` |
| Query | `webview ig` |
| Position | A fixed position such as `top_right` |
| Hide Query Box | Enabled |
| Hide Toolbar | Enabled |
| Width | A compact width, such as `420` |

You can also edit the WebView plugin settings to add your own site keyword and URL. After that, bind another Query Hotkey to `webview <keyword>`.

![Wox WebView action to open the page in a browser](/images/did-you-know-wox-webview-open-browser.png)

This does not replace a browser. It is better for quick checks: timelines, small dashboards, docs, status pages, or any site you want to glance at without changing your current workspace. When you do need the normal browser, use the WebView result action **Open in Browser**.

Platform note: the built-in WebView system plugin is currently available on macOS and Windows. Linux does not have a WebView preview implementation in Wox today.

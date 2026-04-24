# Changelog

## v2.0.3 -

- Add
  - [`Screenshot`] Add screenshot plugin with annotation, history, export path, clipboard handoff, keyboard confirmation, and multi-display handling. One more app to remove from startup list!  
    ![](https://raw.githubusercontent.com/Wox-launcher/Wox/refs/heads/master/screenshots/screenshot.png)  
  **Tips**: Use it with Query Hotkey to capture screenshots with a single shortcut
  - [`WebView`] Add configurable website previews with navigation actions, preview toolbar, cache controls, and Windows support.   
    ![](https://raw.githubusercontent.com/Wox-launcher/Wox/refs/heads/master/screenshots/webview_with_hotkey.png)  
  **Tips**: Use it with Query Hotkey to open frequently used websites with one shortcut, such as Ctrl+Shift+I to quickly check Instagram and hide it again
  - [`Converter`] Add unit conversion support for length, weight, and temperature #4390
  - [`File Search`] Add native indexed file search with database-backed scanning, wildcard search, incremental changefeed sync, startup restore, and cross-platform providers. Everything plugin has been moved to [here](https://github.com/qianlifeng/Wox.Plugin.Everything)
  - [`Toolbar`] Add plugin toolbar messages API so long-running tasks can show progress and actions in the launcher
  - [`App`] Add customizable ignore rules for app indexing #4375
  - [`System`] Add shutdown and restart commands with confirmation prompts
  - [`Query Hotkey`] Add per-hotkey position, query box, toolbar, width, and result count options
  - [`Tray`] Add context menus and configurable result limits for tray queries

- Improve
  - [`Launcher`] Improve query result handling, height preservation, and resize timing to reduce flicker and input lag
  - [`Query`] Improve temporary query restoration, debounced plugin fallback, and result tracking for more stable query transitions
  - [`Plugin`] Improve uninstall progress reporting and host cleanup
  - [`File Icon`] Improve Windows file icon retrieval with associated file type fallback
  - [`Updater`] Improve macOS app replacement and Linux updater logging
  - [`Settings`] Improve loading of settings and AI model data

- Fix
  - [`Launcher`] Fix resize regressions, first-result painting flicker, and delayed window hiding on Windows
  - [`Tray`] Fix preview padding when tray query results only contain a preview
  - [`App`] Fix application indexing issues and query handling edge cases


## v2.0.2 - 2026-03-23

- Add
  - [`Plugin`] Add action to open a plugin's settings directly from query results
  - [`Clipboard`] Add action to open directory paths directly from clipboard results
  - [`AI`] Add MiniMax provider support
  - [`Privacy`] Add optional anonymous usage statistics with privacy controls
  - [`Hotkey`] Add ignored application list for global hotkeys #4372
  - [`Tray`] Add `Show Query Box` option for tray queries

- Improve
  - [`App`] Improve app search metadata and indexing on macOS and Windows, including System32 apps, Windows `.url` shortcuts, and cleaner Windows icons #4291 #4367
  - [`Plugin Setting`] Improve required-value validation in plugin tables and make AI model selection fall back to available provider configs #4365
  - [`AI`] Improve provider setup with default host configs and clearer provider icons
  - [`URL`] Improve URL results with dynamic website icons
  - Improve restoring the previously active window when Wox hides, with better tray interaction and Quick Select behavior on Windows
  - Improve locale detection when choosing the app language #4371
  - Improve switching to existing windows by matching window titles more reliably

- Fix
  - Fix opening URLs and file paths on Windows when the target contains `&` or quotes #4360
  - [`Clipboard`] Fix cross-platform clipboard handling for text, images, and file paths #4309
  - Fix Windows DWM refresh and acrylic resize jitter issues
  - Fix Linux release bundles so bundled shared libraries can resolve their packaged dependencies correctly #4347
  - [`Python Plugin`] Fix compatibility on modern macOS by requiring a newer Python runtime #4374

## v2.0.1 - 2026-03-07

- Add
  - Add tray query feature. User can add custom queries to tray menu for quick access
    ![](https://raw.githubusercontent.com/Wox-launcher/Wox/refs/heads/master/screenshots/tray_query.png)
  - Add "App font family" setting to choose system font for Wox interface #4335
  - [`Plugin Setting`] Add image emoji selector for plugin table image fields
  - [`Plugin Setting`] Add `maxHeight` property support in plugin table setting value #4339
  - [`Plugin Store`] Add filter functionality and upgrade indicators for plugins #4356
  - [`Browser Bookmarks`] Add Firefox support #4354
  - [`Script Plugin`] Add missing runtime notifications with install actions #4357
  - Add secondary tap support for item actions in grid and list views #4358
  - [`Web Search`] Allow user to select custom browser #3597
  - [`Setting`] Add log management features including clearing logs and changing log level
  - [`File Explorer Search`] Add quick jump paths and enhance file dialog interactions

- Improve
  - [`Shell`] Enhance Shell plugin terminal preview to support search/full-screen/scroll-to-load functions
  - Improve query hotkey tooltips and add Wox Chrome extension link in settings #4333
  - Improve app process exit handling when shutting down Wox #4338
  - Improve the layout of the plugin settings page
  - [`Plugin Setting`] Improve focus management and validation
  - Improve preview functionality and local actions
  - Improve Windows Start Menu handling by dismissing it when Wox opens #4341
  - [`Calculator`] Improve history management and limit displayed history to top 100 entries #4340
  - Improve listview rendering performance

- Fix
  - [`File Explorer Search`] Fix an issue that file explorer search plugin cannot navigate on open/save dialog
  - [`Clipboard`] Fix self-triggering in clipboard watch #4309
  - Fix Windows hotkey recording so the Win key and modifier combinations can be captured correctly
  - Fix an issue that Wox setting table values can't be saved sometimes
  - Fix query results not being cleared correctly when app visibility changes
  - Fix transient focus loss when showing Wox window on Windows #4346
  - Fix Base64 JPEG decode issue in image preview
  - [`Plugin Setting`] Fix an issue with handling null and empty JSON responses in plugin table settings
  - [`File`] Fix Windows Phone Link automatic downloads occurring when fetching file icons #4352
  - [`Web Search`] Fix query URL formatting for escaped search text #4360
  - Fix image loading error handling in image view

## v2.0.0 - 2026-02-09

It's time to release the official 2.0 version! There are no major issues in everyday use anymore. Thank you to all users who tested the beta version and provided feedback!

- Add
  - [`Calculator`] Add comma separator support in Calculator plugin #4325
  - [`File Explorer Search`] Add type-to-search feature (experimental, default is off, user can enable this in plugin setting). When enabled, user can type to filter in finder/explorer windows.
    ![](https://raw.githubusercontent.com/Wox-launcher/Wox/refs/heads/master/screenshots/typetosearch.png)
    ![](https://raw.githubusercontent.com/Wox-launcher/Wox/refs/heads/master/screenshots/typetosearch_setting.png)

- Improve
  - [`File`] Improve everything sdk integration stability (with 1.5a support) #4317

- Fix
  - [`File Explorer Search`] Fix a issue that file explorer search plugin's settings do not load #4326
  - [`Clipboard`] Fix a issue that Clipboard plugin cannot paste to active window #4328
  - [`Wpm`] Fix a issue where WPM couldn't create script plugins #4330

## v2.0.0-beta.8 — 2026-01-10

- Add
  - [`Emoji`] Add ai search support for Emoji plugin (you need to enable AI feature in settings first)
    ![](https://raw.githubusercontent.com/Wox-launcher/Wox/refs/heads/master/screenshots/emoji_ai_search.png)
  - Add auto theme which changes theme based on system light/dark mode
    ![](https://raw.githubusercontent.com/Wox-launcher/Wox/refs/heads/master/screenshots/auto_theme.png)
  - [`Explorer`] Add Explorer plugin to quick switch paths in Open/Save dialog #3259, see [Explorer plugin guide](https://wox-launcher.github.io/Wox/guide/plugins/system/explorer.html) for more details
  - Add loading animation to query box during plugin metadata fetching to improve user experience

- Improve
  - Improve markdown preview rendering performance and stability
  - Critical deletion actions have been implemented to recycle bin, this will prevent accidental data loss #3958
  - Improve docs website [https://wox-launcher.github.io/Wox/guide/introduction.html](https://wox-launcher.github.io/Wox/guide/introduction.html)
  - Support multiple-line text in query input box #3797
    ![](https://github.com/user-attachments/assets/64040d63-5d9b-46b4-93a8-449becf70762)
  - Improve database recovery mechanism to prevent database corruption on cloud disk sync (icloud, onedrive, dropbox, etc.)

- Fix
  - Fix clipboard history cause windows copy mal-function #4309
  - Fix switching to application alway opens a new window instead of focusing existing one #1922
  - Fixed the parsing issue of lnk files on Windows #4315
  - Fix the issue where plugin configuration is lost after plugin upgrade

## v2.0.0-beta.7 — 2025-12-19

- Add
  - Add MCP Server for Wox plugin development (default enabled on port 29867, can be configured in settings)
  - Add thousands separator for numbers in Calculator plugin `#4299`
  - Add windows setting searches
  - Add usage page in settings
    ![](screenshots/usage.png)

- Improve
  - Improve fuzzy match based on fzf algorithm
  - Improve app searches on windows by

- Fix
  - Fix working directory issues, adding getWorkingDirectory function for command execution context, close `#4161`
  - Fix command line window display issue when executing Script Plugin
  - [`AI Chat`] Fix a render issue
  - [`Emoji`] Fix copy large image not working on windows
  - [`Clipboard`] Fix clipboard image paste issue on windows
  - Fix a theme regression released on beta.6 that causes crash on invalid theme colors `#4302`

## v2.0.0-beta.6 — 2025-12-05

- Add
  - Add Emoji plugin
  - Add Launch Mode and Start Page setting

- Improve
  - UI now uses safe color parsing (`safeFromCssColor`) to fall back gracefully when theme colors are invalid, preventing crashes and highlighting misconfigured themes.

## v2.0.0-beta.5 — 2025-09-24

- Fix
  - Fix a regression issue that some settings can't be changed on beta.4 @yougg

## v2.0.0-beta.4 — 2025-08-24

- Add
  - Quick Select to choose results via digits/letters
  - MRU for query mode, use can now display MRU results when opening Wox
  - Last Query Mode option (retain last query or always start fresh, #4234)
  - Custom Python and Node.js path configuration (#4220)
  - Edge bookmarks loading across platforms
  - Calculator plugin: add power operator (^) support

- Improve
  - Migrate settings from JSON to a unified, type-safe SQLite store
  - Reduce clipboard memory usage
  - Windows UX: app display details, Unicode handling, and UWP icon retrieval

- Fix
  - Key conflict when holding Ctrl and repeatedly pressing other keys
  - "Last display position" not restored after restart
  - Windows app extension checks (case-insensitive, #4251)
  - Image loading error handling in image view

## v2.0.0-beta.3 — 2025-06-23

- Add
  - Chat plugin: support multiple tool calls executed simultaneously in a single request
  - Chat plugin: support custom agents
  - ScriptPlugin support

- Fix
  - Windows sometimes cannot gain focus (#4198)

## v2.0.0-beta.2 — 2025-04-18

- Add
  - Chat plugin (supports MCP)
  - Double modifiers hotkey (e.g., double-click Ctrl)
  - [Windows] Everything (file plugin)

- Improve
  - Settings interface now follows the theme color
  - [Windows] Optimized transparent display effect

- Fix
  - [Windows] Focus not returning (#4144, #4166)

## v2.0.0-beta.1 — 2025-02-27

- Add
  - Cross-platform rewrite (macOS, Windows, Linux) with a single executable
  - Modern UI/UX with a new preview panel; AI-ready commands
  - Plugin system (JavaScript and Python); improved plugin store; better action filtering and result scoring
  - AI integrations (enhanced AI command processing; AI-powered theme creation)
  - Internationalization for settings
  - Enhanced deep linking

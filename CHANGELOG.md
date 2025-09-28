# Changelog

## v2.0.0-beta.6 — 2025-??-??

- Improve
  - UI now uses safe color parsing (`safeFromCssColor`) to fall back gracefully when theme colors are invalid, preventing crashes and highlighting misconfigured themes.

---

## v2.0.0-beta.5 — 2025-09-24

- Fix
  - Fix a regression issue that some settings can't be changed on beta.4 @yougg

---

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

---

## v2.0.0-beta.3 — 2025-06-23

- Add

  - Chat plugin: support multiple tool calls executed simultaneously in a single request
  - Chat plugin: support custom agents
  - ScriptPlugin support

- Fix
  - Windows sometimes cannot gain focus (#4198)

---

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

---

## v2.0.0-beta.1 — 2025-02-27

- Add
  - Cross-platform rewrite (macOS, Windows, Linux) with a single executable
  - Modern UI/UX with a new preview panel; AI-ready commands
  - Plugin system (JavaScript and Python); improved plugin store; better action filtering and result scoring
  - AI integrations (enhanced AI command processing; AI-powered theme creation)
  - Internationalization for settings
  - Enhanced deep linking

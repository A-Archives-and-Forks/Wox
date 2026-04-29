package dto

import (
	"wox/i18n"
	"wox/setting"
)

type WoxSettingDto struct {
	EnableAutostart           bool
	MainHotkey                string
	SelectionHotkey           string
	IgnoredHotkeyApps         []setting.IgnoredHotkeyApp
	LogLevel                  string
	UsePinYin                 bool
	SwitchInputMethodABC      bool
	HideOnStart               bool
	HideOnLostFocus           bool
	ShowTray                  bool
	LangCode                  i18n.LangCode
	QueryHotkeys              []setting.QueryHotkey
	QueryShortcuts            []setting.QueryShortcut
	TrayQueries               []setting.TrayQuery
	LaunchMode                setting.LaunchMode
	StartPage                 setting.StartPage
	AIProviders               []setting.AIProvider
	HttpProxyEnabled          bool
	HttpProxyUrl              string
	ShowPosition              setting.PositionType
	EnableAutoBackup          bool
	EnableAutoUpdate          bool
	EnableAnonymousUsageStats bool
	CustomPythonPath          string
	CustomNodejsPath          string

	// UI related
	AppWidth       int
	MaxResultCount int
	ThemeId        string
	AppFontFamily  string

	// Debug display switches are only shown by the dev UI, but the DTO keeps
	// them beside other settings so backend tail rendering and Flutter toggles
	// stay synchronized through the existing settings API.
	ShowScoreTail       bool
	ShowPerformanceTail bool
}
